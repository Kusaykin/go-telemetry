package repository

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"time"

	"github.com/golang-migrate/migrate/v4"
	migratepg "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"

	models "github.com/Kusaykin/go-telemetry/internal/model"
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
	db *sqlx.DB
}

func NewPostgresStorage(db *sqlx.DB) *PostgresStorage {
	return &PostgresStorage{db: db}
}

const (
	upsertGauge = `INSERT INTO gauges (name, value) VALUES ($1, $2)
		 ON CONFLICT (name) DO UPDATE SET value = EXCLUDED.value`

	upsertCounter = `INSERT INTO counters (name, delta) VALUES ($1, $2)
		 ON CONFLICT (name) DO UPDATE SET delta = counters.delta + EXCLUDED.delta`
)

func (s *PostgresStorage) UpdateGauge(ctx context.Context, name string, value float64) error {
	if _, err := s.db.ExecContext(ctx, upsertGauge, name, value); err != nil {
		return fmt.Errorf("update gauge %s: %w", name, err)
	}

	return nil
}

func (s *PostgresStorage) UpdateCounter(ctx context.Context, name string, delta int64) (int64, error) {
	var value int64

	if err := s.db.GetContext(ctx, &value, upsertCounter+` RETURNING delta`, name, delta); err != nil {
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

	conn, err := s.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("update batch: acquire connection: %w", err)
	}

	defer func() {
		err = errors.Join(err, conn.Close())
	}()

	// pgx.Batch доступен только на нативном соединении pgx, поэтому
	// достаём его из database/sql на время транзакции.
	err = conn.Raw(func(driverConn any) error {
		pc, ok := driverConn.(*stdlib.Conn)
		if !ok {
			return fmt.Errorf("unexpected driver connection %T", driverConn)
		}

		return pgx.BeginFunc(ctx, pc.Conn(), func(tx pgx.Tx) error {
			return upsertBatch(ctx, tx, gauges, counters)
		})
	})
	if err != nil {
		return fmt.Errorf("update batch: %w", err)
	}

	return nil
}

// upsertBatch отправляет все upsert-ы одним pgx.Batch — за один round-trip.
// Имена сортируются, чтобы параллельные батчи брали блокировки строк
// в одном порядке и не уходили в дедлок.
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

	// Результаты читаются по очереди, чтобы в ошибке было имя метрики.
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

// pgNumericValueOutOfRange — SQLSTATE 22003, в том числе переполнение bigint.
const pgNumericValueOutOfRange = "22003"

// counterError приводит переполнение bigint к ErrCounterOverflow,
// чтобы оно обрабатывалось так же, как в MemStorage.
func counterError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgNumericValueOutOfRange {
		return fmt.Errorf("%w: %w", models.ErrCounterOverflow, err)
	}

	return err
}

func (s *PostgresStorage) Gauge(ctx context.Context, name string) (float64, bool, error) {
	var value float64

	err := s.db.GetContext(ctx, &value, `SELECT value FROM gauges WHERE name = $1`, name)
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

	err := s.db.GetContext(ctx, &delta, `SELECT delta FROM counters WHERE name = $1`, name)
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

	if err := s.db.SelectContext(ctx, &rows, `SELECT name, value FROM gauges`); err != nil {
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

	if err := s.db.SelectContext(ctx, &rows, `SELECT name, delta FROM counters`); err != nil {
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

func (s *PostgresStorage) Run(context.Context) error {
	return nil
}

func (s *PostgresStorage) Close() error {
	return s.db.Close()
}
