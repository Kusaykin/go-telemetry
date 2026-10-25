package repository

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"slices"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4/database"
	migratepg "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	models "github.com/Kusaykin/go-telemetry/internal/model"
	"github.com/Kusaykin/go-telemetry/internal/retry"
)

func TestNewPostgres(t *testing.T) {
	t.Run("валидный DSN открывается без подключения к БД", func(t *testing.T) {
		db, err := NewPostgres("postgres://127.0.0.1:1/metrics?sslmode=disable")
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		assert.Equal(t, "pgx", db.DriverName())
	})

	t.Run("некорректный DSN проявляется при ping, а не при открытии", func(t *testing.T) {
		db, err := NewPostgres("host=localhost port=abc")
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		assert.Error(t, db.PingContext(t.Context()))
	})
}

func openPostgres(t *testing.T, dsn string) *sqlx.DB {
	t.Helper()

	db, err := NewPostgres(dsn)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	return db
}

func TestMigrateFailsWithoutDatabase(t *testing.T) {
	db := openPostgres(t, "postgres://127.0.0.1:1/metrics?sslmode=disable&connect_timeout=1")
	assert.Error(t, Migrate(t.Context(), db))
}

func TestMigrateFailsOnInvalidDSN(t *testing.T) {
	db := openPostgres(t, "host=localhost port=abc")
	assert.Error(t, Migrate(t.Context(), db))
}

func TestMigrateFailsOnExpiredContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), -time.Second)
	defer cancel()

	db := openPostgres(t, "postgres://127.0.0.1:1/metrics?sslmode=disable")
	assert.ErrorIs(t, Migrate(ctx, db), context.DeadlineExceeded)
}

func TestMigrateRespectsContextDeadline(t *testing.T) {
	// 10.255.255.1 не маршрутизируется: без дедлайна подключение висело бы
	// до TCP-таймаута ОС.
	ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	defer cancel()

	db := openPostgres(t, "postgres://10.255.255.1:5432/metrics?sslmode=disable")

	start := time.Now()
	err := Migrate(ctx, db)

	assert.Error(t, err)
	assert.Less(t, time.Since(start), 3*time.Second)
}

func TestMigrateRespectsDeadlineWhileLockIsHeld(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("TEST_DATABASE_DSN не задана")
	}

	db := openPostgres(t, dsn)

	// Держим тот же advisory lock, что берёт golang-migrate, на отдельном
	// соединении: pg_advisory_lock привязан к сессии.
	conn, err := db.Conn(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })

	var dbName, schema string
	require.NoError(t, conn.QueryRowContext(t.Context(),
		`SELECT current_database(), current_schema()`).Scan(&dbName, &schema))

	lockID, err := database.GenerateAdvisoryLockId(dbName, schema, migratepg.DefaultMigrationsTable)
	require.NoError(t, err)

	_, err = conn.ExecContext(t.Context(), `SELECT pg_advisory_lock($1)`, lockID)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, lockID)
	})

	ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
	defer cancel()

	start := time.Now()
	err = Migrate(ctx, db)

	assert.Error(t, err)
	assert.Less(t, time.Since(start), 3*time.Second)
}

func TestMigrateKeepsPoolAndResetsSession(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("TEST_DATABASE_DSN не задана")
	}

	// Одно соединение в пуле: миграции и проверка ниже идут через одну сессию.
	db := openPostgres(t, dsn)
	db.SetMaxOpenConns(1)

	var defaultLockTimeout string
	require.NoError(t, db.GetContext(t.Context(), &defaultLockTimeout, `SHOW lock_timeout`))

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	require.NoError(t, Migrate(ctx, db))

	var lockTimeout string
	require.NoError(t, db.GetContext(t.Context(), &lockTimeout, `SHOW lock_timeout`),
		"пул должен остаться открытым после миграций")
	assert.Equal(t, defaultLockTimeout, lockTimeout, "lock_timeout миграций не должен оставаться в сессии")
}

// newPostgresStorage подключается к БД из TEST_DATABASE_DSN, применяет
// миграции и очищает таблицы после теста. Без переменной тест пропускается.
func newPostgresStorage(t *testing.T) *PostgresStorage {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("TEST_DATABASE_DSN не задана")
	}

	db := openPostgres(t, dsn)

	require.NoError(t, Migrate(t.Context(), db))
	require.NoError(t, Migrate(t.Context(), db), "повторный запуск миграций не должен падать")

	truncate := func() {
		_, err := db.Exec(`TRUNCATE gauges, counters`)
		require.NoError(t, err)
	}

	truncate()
	t.Cleanup(truncate)

	return NewPostgresStorage(db)
}

func TestPostgresStorageGauge(t *testing.T) {
	s := newPostgresStorage(t)

	mustUpdateGauge(t, s, "Alloc", 1.5)
	mustUpdateGauge(t, s, "Alloc", -2.25)
	mustUpdateGauge(t, s, "Zero", 0)

	value, ok := mustGauge(t, s, "Alloc")
	assert.True(t, ok)
	assert.Equal(t, -2.25, value)

	_, ok = mustGauge(t, s, "Zero")
	assert.True(t, ok)

	_, ok = mustGauge(t, s, "Missing")
	assert.False(t, ok)
}

func TestPostgresStorageCounter(t *testing.T) {
	s := newPostgresStorage(t)

	assert.Equal(t, int64(5), mustUpdateCounter(t, s, "PollCount", 5))
	assert.Equal(t, int64(15), mustUpdateCounter(t, s, "PollCount", 10))

	delta, ok := mustCounter(t, s, "PollCount")
	assert.True(t, ok)
	assert.Equal(t, int64(15), delta)

	_, ok = mustCounter(t, s, "Missing")
	assert.False(t, ok)
}

func TestPostgresStorageGaugeAndCounterWithSameName(t *testing.T) {
	s := newPostgresStorage(t)

	mustUpdateGauge(t, s, "Same", 1.5)
	mustUpdateCounter(t, s, "Same", 3)

	value, _ := mustGauge(t, s, "Same")
	assert.Equal(t, 1.5, value)

	delta, _ := mustCounter(t, s, "Same")
	assert.Equal(t, int64(3), delta)
}

func TestPostgresStorageList(t *testing.T) {
	s := newPostgresStorage(t)

	assert.Empty(t, mustGauges(t, s))
	assert.Empty(t, mustCounters(t, s))

	mustUpdateGauge(t, s, "Alloc", 12.5)
	mustUpdateGauge(t, s, "Sys", 1)
	mustUpdateCounter(t, s, "PollCount", 7)

	assert.Equal(t, map[string]float64{"Alloc": 12.5, "Sys": 1}, mustGauges(t, s))
	assert.Equal(t, map[string]int64{"PollCount": 7}, mustCounters(t, s))
}

func TestPostgresStorageUpdateBatch(t *testing.T) {
	checkUpdateBatch(t, newPostgresStorage(t))
}

func TestPostgresStorageUpdateBatchOverflow(t *testing.T) {
	checkUpdateBatchOverflow(t, newPostgresStorage(t))
}

func TestPostgresStorageUpdateBatchRollsBack(t *testing.T) {
	s := newPostgresStorage(t)

	mustUpdateCounter(t, s, "PollCount", 1)

	err := s.UpdateBatch(t.Context(), []models.Metrics{
		gaugeMetric("Alloc", 1.5),
		counterMetric("A", 1),
		counterMetric("PollCount", math.MaxInt64),
	})
	require.ErrorIs(t, err, models.ErrCounterOverflow)

	assert.Empty(t, mustGauges(t, s))
	assert.Equal(t, map[string]int64{"PollCount": 1}, mustCounters(t, s))
}

func TestPostgresStorageCounterOverflow(t *testing.T) {
	s := newPostgresStorage(t)

	mustUpdateCounter(t, s, "PollCount", math.MaxInt64)

	_, err := s.UpdateCounter(t.Context(), "PollCount", 1)

	require.ErrorIs(t, err, models.ErrCounterOverflow)
	delta, _ := mustCounter(t, s, "PollCount")
	assert.Equal(t, int64(math.MaxInt64), delta)
}

func TestPostgresStorageConcurrentBatches(t *testing.T) {
	s := newPostgresStorage(t)

	const workers, batches = 8, 20

	ctx := t.Context()

	var wg sync.WaitGroup
	for i := range workers {
		wg.Go(func() {
			batch := []models.Metrics{counterMetric("A", 1), counterMetric("B", 1)}
			if i%2 == 1 {
				slices.Reverse(batch)
			}

			for range batches {
				assert.NoError(t, s.UpdateBatch(ctx, batch))
			}
		})
	}
	wg.Wait()

	assert.Equal(t, map[string]int64{"A": workers * batches, "B": workers * batches}, mustCounters(t, s))
}

func TestPostgresStorageConcurrentCounter(t *testing.T) {
	s := newPostgresStorage(t)

	const workers, updates = 8, 20

	ctx := t.Context()

	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for range updates {
				_, err := s.UpdateCounter(ctx, "PollCount", 1)
				assert.NoError(t, err)
			}
		})
	}
	wg.Wait()

	delta, _ := mustCounter(t, s, "PollCount")
	assert.Equal(t, int64(workers*updates), delta)
}

func TestIsRetriable(t *testing.T) {
	pgErr := func(code string) error {
		return fmt.Errorf("wrapped: %w", &pgconn.PgError{Code: code})
	}

	t.Run("Class 08 повторяется", func(t *testing.T) {
		for _, code := range []string{
			pgerrcode.ConnectionException,
			pgerrcode.ConnectionDoesNotExist,
			pgerrcode.ConnectionFailure,
			pgerrcode.SQLClientUnableToEstablishSQLConnection,
		} {
			assert.True(t, IsRetriable(pgErr(code)), code)
		}
	})

	t.Run("перезапуск или неготовность сервера повторяются", func(t *testing.T) {
		for _, code := range []string{
			pgerrcode.AdminShutdown,
			pgerrcode.CrashShutdown,
			pgerrcode.CannotConnectNow,
		} {
			assert.True(t, IsRetriable(pgErr(code)), code)
		}
	})

	t.Run("обрыв соединения посреди запроса повторяется", func(t *testing.T) {
		for _, err := range []error{
			io.EOF,
			io.ErrUnexpectedEOF,
			&net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNRESET},
			driver.ErrBadConn,
		} {
			assert.True(t, IsRetriable(fmt.Errorf("wrapped: %w", err)), err.Error())
		}
	})

	t.Run("прочие ошибки PostgreSQL не повторяются", func(t *testing.T) {
		for _, code := range []string{
			pgerrcode.UniqueViolation,
			pgerrcode.NumericValueOutOfRange,
			pgerrcode.UndefinedTable,
		} {
			assert.False(t, IsRetriable(pgErr(code)), code)
		}
	})

	t.Run("неудачное подключение повторяется", func(t *testing.T) {
		db := openPostgres(t, "postgres://127.0.0.1:1/metrics?sslmode=disable&connect_timeout=1")
		assert.True(t, IsRetriable(db.PingContext(t.Context())))
	})

	t.Run("несуществующий хост не повторяется", func(t *testing.T) {
		db := openPostgres(t, "postgres://no-such-host.invalid:5432/metrics?sslmode=disable&connect_timeout=1")
		err := db.PingContext(t.Context())

		var dnsErr *net.DNSError
		require.ErrorAs(t, err, &dnsErr)
		assert.False(t, IsRetriable(err))
	})

	t.Run("прочие ошибки не повторяются", func(t *testing.T) {
		assert.False(t, IsRetriable(errors.New("boom")))
		assert.False(t, IsRetriable(context.Canceled))
	})
}

func TestPostgresStorageRetriesConnectionErrors(t *testing.T) {
	db := openPostgres(t, "postgres://127.0.0.1:1/metrics?sslmode=disable&connect_timeout=1")
	s := NewPostgresStorage(db)
	s.delays = []time.Duration{20 * time.Millisecond, 20 * time.Millisecond, 20 * time.Millisecond}

	start := time.Now()
	err := s.UpdateGauge(t.Context(), "Alloc", 1)

	require.Error(t, err)
	assert.True(t, IsRetriable(err))
	assert.GreaterOrEqual(t, time.Since(start), 60*time.Millisecond)
}

func TestPostgresStorageStopsRetryingOnCancel(t *testing.T) {
	db := openPostgres(t, "postgres://127.0.0.1:1/metrics?sslmode=disable&connect_timeout=1")
	s := NewPostgresStorage(db)

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := s.UpdateGauge(ctx, "Alloc", 1)

	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, time.Since(start), retry.Delays()[0])
}

func TestPostgresStorageStopsRetryingOnShutdown(t *testing.T) {
	db := openPostgres(t, "postgres://127.0.0.1:1/metrics?sslmode=disable&connect_timeout=1")
	s := NewPostgresStorage(db)

	runCtx, shutdown := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- s.Run(runCtx) }()

	time.AfterFunc(50*time.Millisecond, shutdown)

	start := time.Now()
	err := s.UpdateGauge(t.Context(), "Alloc", 1)

	require.ErrorIs(t, err, context.Canceled)
	assert.True(t, IsRetriable(err), "возвращается исходная ошибка подключения")
	assert.Less(t, time.Since(start), retry.Delays()[0])
	require.NoError(t, <-done)
}

func TestPostgresStoragePingDoesNotRetry(t *testing.T) {
	db := openPostgres(t, "postgres://127.0.0.1:1/metrics?sslmode=disable&connect_timeout=1")
	s := NewPostgresStorage(db)

	start := time.Now()
	err := s.PingContext(t.Context())

	require.Error(t, err)
	assert.Less(t, time.Since(start), retry.Delays()[0])
}
