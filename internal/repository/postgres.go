package repository

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/golang-migrate/migrate/v4"
	migratepg "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"

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

func (s *PostgresStorage) UpdateGauge(ctx context.Context, name string, value float64) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO gauges (name, value) VALUES ($1, $2)
		 ON CONFLICT (name) DO UPDATE SET value = EXCLUDED.value`,
		name, value)
	if err != nil {
		return fmt.Errorf("update gauge %s: %w", name, err)
	}

	return nil
}

func (s *PostgresStorage) UpdateCounter(ctx context.Context, name string, delta int64) (int64, error) {
	var value int64

	err := s.db.GetContext(ctx, &value,
		`INSERT INTO counters (name, delta) VALUES ($1, $2)
		 ON CONFLICT (name) DO UPDATE SET delta = counters.delta + EXCLUDED.delta
		 RETURNING delta`,
		name, delta)
	if err != nil {
		return 0, fmt.Errorf("update counter %s: %w", name, err)
	}

	return value, nil
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
