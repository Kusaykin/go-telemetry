package repository

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"maps"
	"net"
	"slices"
	"strconv"
	"syscall"
	"time"

	"github.com/golang-migrate/migrate/v4"
	migratepg "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"

	models "github.com/Kusaykin/go-telemetry/internal/model"
	"github.com/Kusaykin/go-telemetry/internal/retry"
	"github.com/Kusaykin/go-telemetry/migrations"
)

func NewPostgres(dsn string) (*sqlx.DB, error) {
	db, err := sqlx.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}

	return db, nil
}

const migrationConnResetTimeout = 5 * time.Second

func Migrate(ctx context.Context, db *sqlx.DB) (err error) {
	driverCfg := &migratepg.Config{}

	deadline, hasDeadline := ctx.Deadline()

	var timeout time.Duration
	if hasDeadline {
		timeout = time.Until(deadline)
		if timeout <= 0 {
			return fmt.Errorf("apply migrations: %w", context.DeadlineExceeded)
		}

		driverCfg.StatementTimeout = timeout
	}

	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration connection: %w", err)
	}

	defer func() {
		err = errors.Join(err, releaseMigrationConn(conn, hasDeadline))
	}()

	if hasDeadline {
		lockTimeout := strconv.FormatInt(max(timeout.Milliseconds(), 1), 10)
		if _, err := conn.ExecContext(ctx, `SELECT set_config('lock_timeout', $1, false)`, lockTimeout); err != nil {
			return fmt.Errorf("set migration lock timeout: %w", err)
		}
	}

	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("load migrations: %w", err)
	}

	defer func() {
		err = errors.Join(err, src.Close())
	}()

	dbDriver, err := migratepg.WithConnection(ctx, conn, driverCfg)
	if err != nil {
		return fmt.Errorf("init migrate driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", src, "postgres", dbDriver)
	if err != nil {
		return fmt.Errorf("init migrate: %w", err)
	}

	if hasDeadline {
		m.LockTimeout = timeout
	}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}

	return nil
}

func releaseMigrationConn(conn *sql.Conn, resetSession bool) error {
	if !resetSession {
		return conn.Close()
	}

	ctx, cancel := context.WithTimeout(context.Background(), migrationConnResetTimeout)
	defer cancel()

	if _, err := conn.ExecContext(ctx, `RESET lock_timeout`); err != nil {
		_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		return fmt.Errorf("reset migration lock timeout: %w", err)
	}

	return conn.Close()
}

type PostgresStorage struct {
	db     *sqlx.DB
	delays []time.Duration

	stopping context.Context
	stop     context.CancelFunc
}

func NewPostgresStorage(db *sqlx.DB) *PostgresStorage {
	stopping, stop := context.WithCancel(context.Background())

	return &PostgresStorage{db: db, delays: retry.Delays(), stopping: stopping, stop: stop}
}

func IsRetriable(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgerrcode.IsConnectionException(pgErr.Code) || isServerUnavailable(pgErr.Code)
	}

	var connErr *pgconn.ConnectError
	if errors.As(err, &connErr) {
		return isTransientConnect(connErr)
	}

	return errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, driver.ErrBadConn)
}

func isTransientConnect(err *pgconn.ConnectError) bool {
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return dnsErr.IsTemporary || dnsErr.IsTimeout
	}

	var netErr net.Error
	if pgconn.Timeout(err) || errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}

	return errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.ECONNABORTED) ||
		errors.Is(err, syscall.EHOSTUNREACH) ||
		errors.Is(err, syscall.ENETUNREACH) ||
		errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrUnexpectedEOF)
}

func isServerUnavailable(code string) bool {
	switch code {
	case pgerrcode.AdminShutdown, pgerrcode.CrashShutdown, pgerrcode.CannotConnectNow:
		return true
	default:
		return false
	}
}

func isRetriableWrite(err error) bool {
	var connErr *pgconn.ConnectError
	return errors.As(err, &connErr) && IsRetriable(err)
}

var errCommit = errors.New("commit")

func isRetriableTx(err error) bool {
	return IsRetriable(err) && !errors.Is(err, errCommit)
}

func (s *PostgresStorage) retry(ctx context.Context, fn func(ctx context.Context) error) error {
	return s.retryIf(ctx, IsRetriable, fn)
}

func (s *PostgresStorage) retryIf(ctx context.Context, retriable func(error) bool, fn func(ctx context.Context) error) error {
	waitCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer context.AfterFunc(s.stopping, cancel)()

	return retry.Do(waitCtx, s.delays, retriable, func() error { return fn(ctx) })
}

const (
	upsertGauge = `INSERT INTO gauges (name, value) VALUES ($1, $2)
		 ON CONFLICT (name) DO UPDATE SET value = EXCLUDED.value`

	upsertCounter = `INSERT INTO counters (name, delta) VALUES ($1, $2)
		 ON CONFLICT (name) DO UPDATE SET delta = counters.delta + EXCLUDED.delta`
)

func (s *PostgresStorage) UpdateGauge(ctx context.Context, name string, value float64) error {
	err := s.retry(ctx, func(ctx context.Context) error {
		_, err := s.db.ExecContext(ctx, upsertGauge, name, value)
		return err
	})
	if err != nil {
		return fmt.Errorf("update gauge %s: %w", name, err)
	}

	return nil
}

func (s *PostgresStorage) UpdateCounter(ctx context.Context, name string, delta int64) (int64, error) {
	var value int64

	err := s.retryIf(ctx, isRetriableWrite, func(ctx context.Context) error {
		return s.db.GetContext(ctx, &value, upsertCounter+` RETURNING delta`, name, delta)
	})
	if err != nil {
		return 0, fmt.Errorf("update counter %s: %w", name, counterError(err))
	}

	return value, nil
}

func (s *PostgresStorage) UpdateBatch(ctx context.Context, metrics []models.Metrics) (err error) {
	gauges, counters, err := models.Collapse(metrics)
	if err != nil {
		return fmt.Errorf("update batch: %w", err)
	}

	if len(gauges) == 0 && len(counters) == 0 {
		return nil
	}

	if err := s.retryIf(ctx, isRetriableTx, func(ctx context.Context) error {
		return s.updateBatchTx(ctx, gauges, counters)
	}); err != nil {
		return fmt.Errorf("update batch: %w", err)
	}

	return nil
}

func (s *PostgresStorage) updateBatchTx(ctx context.Context, gauges map[string]float64, counters map[string]int64) (err error) {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire connection: %w", err)
	}

	defer func() {
		err = errors.Join(err, conn.Close())
	}()

	return conn.Raw(func(driverConn any) error {
		pc, ok := driverConn.(*stdlib.Conn)
		if !ok {
			return fmt.Errorf("unexpected driver connection %T", driverConn)
		}

		tx, err := pc.Conn().Begin(ctx)
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback(ctx) }()

		if err := upsertBatch(ctx, tx, gauges, counters); err != nil {
			return err
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("%w: %w", errCommit, err)
		}

		return nil
	})
}

func upsertBatch(ctx context.Context, tx pgx.Tx, gauges map[string]float64, counters map[string]int64) (err error) {
	gaugeNames := slices.Sorted(maps.Keys(gauges))
	counterNames := slices.Sorted(maps.Keys(counters))

	batch := &pgx.Batch{}
	for _, name := range gaugeNames {
		batch.Queue(upsertGauge, name, gauges[name])
	}
	for _, name := range counterNames {
		batch.Queue(upsertCounter, name, counters[name])
	}

	br := tx.SendBatch(ctx, batch)
	defer func() {
		err = errors.Join(err, br.Close())
	}()

	for _, name := range gaugeNames {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("gauge %s: %w", name, err)
		}
	}
	for _, name := range counterNames {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("counter %s: %w", name, counterError(err))
		}
	}

	return nil
}

func counterError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.NumericValueOutOfRange {
		return fmt.Errorf("%w: %w", models.ErrCounterOverflow, err)
	}

	return err
}

func (s *PostgresStorage) Gauge(ctx context.Context, name string) (float64, bool, error) {
	var value float64

	err := s.retry(ctx, func(ctx context.Context) error {
		return s.db.GetContext(ctx, &value, `SELECT value FROM gauges WHERE name = $1`, name)
	})
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}

	if err != nil {
		return 0, false, fmt.Errorf("get gauge %s: %w", name, err)
	}

	return value, true, nil
}

func (s *PostgresStorage) Counter(ctx context.Context, name string) (int64, bool, error) {
	var delta int64

	err := s.retry(ctx, func(ctx context.Context) error {
		return s.db.GetContext(ctx, &delta, `SELECT delta FROM counters WHERE name = $1`, name)
	})
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}

	if err != nil {
		return 0, false, fmt.Errorf("get counter %s: %w", name, err)
	}

	return delta, true, nil
}

func (s *PostgresStorage) Gauges(ctx context.Context) (map[string]float64, error) {
	var rows []struct {
		Name  string  `db:"name"`
		Value float64 `db:"value"`
	}

	err := s.retry(ctx, func(ctx context.Context) error {
		rows = nil
		return s.db.SelectContext(ctx, &rows, `SELECT name, value FROM gauges`)
	})
	if err != nil {
		return nil, fmt.Errorf("list gauges: %w", err)
	}

	gauges := make(map[string]float64, len(rows))
	for _, row := range rows {
		gauges[row.Name] = row.Value
	}

	return gauges, nil
}

func (s *PostgresStorage) Counters(ctx context.Context) (map[string]int64, error) {
	var rows []struct {
		Name  string `db:"name"`
		Delta int64  `db:"delta"`
	}

	err := s.retry(ctx, func(ctx context.Context) error {
		rows = nil
		return s.db.SelectContext(ctx, &rows, `SELECT name, delta FROM counters`)
	})
	if err != nil {
		return nil, fmt.Errorf("list counters: %w", err)
	}

	counters := make(map[string]int64, len(rows))
	for _, row := range rows {
		counters[row.Name] = row.Delta
	}

	return counters, nil
}

func (s *PostgresStorage) PingContext(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func (s *PostgresStorage) Run(ctx context.Context) error {
	<-ctx.Done()
	s.stop()

	return nil
}

func (s *PostgresStorage) Close() error {
	s.stop()

	return s.db.Close()
}
