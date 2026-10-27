package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/Kusaykin/go-telemetry/internal/config"
	"github.com/Kusaykin/go-telemetry/internal/handler"
	"github.com/Kusaykin/go-telemetry/internal/logger"
	"github.com/Kusaykin/go-telemetry/internal/repository"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

const (
	shutdownTimeout = 5 * time.Second
	dbStartTimeout  = 10 * time.Second
)

func main() {
	cfg, err := config.LoadServer(os.Args[1:], os.LookupEnv, os.Stderr)
	if err != nil {
		os.Exit(config.ExitCode(err))
	}

	log, err := logger.NewJSON("info")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer log.Sync()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, cfg, log); err != nil {
		log.Fatal("server stopped", zap.Error(err))
	}
}

type storage interface {
	handler.Storage
	Run(ctx context.Context) error
	Close() error
}

func run(ctx context.Context, cfg config.Server, log *zap.Logger) error {
	store, db, err := openStorage(ctx, cfg, log)
	if err != nil {
		return err
	}

	logStartup(cfg, log)

	srv := newServer(cfg, store, db, log)

	var handlers inflight
	srv.Handler = handlers.wrap(srv.Handler)

	reqCtx, cancelRequests := context.WithCancel(context.Background())
	defer cancelRequests()
	srv.BaseContext = func(net.Listener) context.Context { return reqCtx }

	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	})

	g.Go(func() error {
		return store.Run(gctx)
	})

	g.Go(func() error {
		<-gctx.Done()
		log.Info("shutting down server")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		err := srv.Shutdown(shutdownCtx)
		if errors.Is(err, context.DeadlineExceeded) {
			log.Warn("shutdown timed out, cancelling in-flight requests")
			cancelRequests()
			err = errors.Join(err, srv.Close())
		}

		if err != nil {
			return fmt.Errorf("graceful shutdown: %w", err)
		}

		return nil
	})

	err = g.Wait()

	handlers.wait()

	return errors.Join(err, store.Close())
}

type inflight struct {
	wg sync.WaitGroup
}

func (f *inflight) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.wg.Add(1)
		defer f.wg.Done()

		next.ServeHTTP(w, r)
	})
}

func (f *inflight) wait() {
	f.wg.Wait()
}

type storageKind string

const (
	storageMemory   storageKind = "memory"
	storageFile     storageKind = "file"
	storagePostgres storageKind = "postgres"
)

func selectStorage(cfg config.Server) storageKind {
	switch {
	case cfg.DatabaseDSN != "":
		return storagePostgres
	case cfg.FileStoragePath != "":
		return storageFile
	default:
		return storageMemory
	}
}

func openStorage(ctx context.Context, cfg config.Server, log *zap.Logger) (storage, handler.Pinger, error) {
	if selectStorage(cfg) != storagePostgres {
		store, err := repository.NewFileStorage(cfg.FileStoragePath, cfg.StoreInterval, cfg.Restore, log)
		if err != nil {
			return nil, nil, err
		}

		return store, store, nil
	}

	db, err := repository.NewPostgres(cfg.DatabaseDSN)
	if err != nil {
		return nil, nil, err
	}

	startCtx, cancel := context.WithTimeout(ctx, dbStartTimeout)
	defer cancel()

	if err := db.PingContext(startCtx); err != nil {
		return nil, nil, errors.Join(fmt.Errorf("connect postgres: %w", err), db.Close())
	}

	if err := repository.Migrate(startCtx, db); err != nil {
		return nil, nil, errors.Join(err, db.Close())
	}

	store := repository.NewPostgresStorage(db)

	return store, store, nil
}

func logStartup(cfg config.Server, log *zap.Logger) {
	kind := selectStorage(cfg)
	fields := []zap.Field{
		zap.String("address", cfg.Address),
		zap.String("storage", string(kind)),
	}

	fileFields := []zap.Field{
		zap.Duration("store_interval", cfg.StoreInterval),
		zap.String("file_storage_path", cfg.FileStoragePath),
		zap.Bool("restore", cfg.Restore),
	}

	switch kind {
	case storageFile:
		fields = append(fields, fileFields...)
	case storagePostgres:
		if fileSettingsChanged(cfg) {
			log.Warn("file storage settings (-i, -f, -r) are ignored in postgres mode", fileFields...)
		}
	}

	log.Info("starting server", fields...)
}

func fileSettingsChanged(cfg config.Server) bool {
	return cfg.StoreInterval != config.DefaultStoreInterval ||
		cfg.FileStoragePath != config.DefaultFileStoragePath ||
		cfg.Restore != config.DefaultRestore
}

func newServer(cfg config.Server, store handler.Storage, db handler.Pinger, log *zap.Logger) *http.Server {
	return &http.Server{
		Addr:    cfg.Address,
		Handler: handler.NewRouter(store, db, log),
	}
}
