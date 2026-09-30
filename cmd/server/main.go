package main

import (
	"context"
	"errors"
	"fmt"
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
)

const shutdownTimeout = 5 * time.Second

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

func run(ctx context.Context, cfg config.Server, log *zap.Logger) error {
	store, err := repository.NewFileStorage(cfg.FileStoragePath, cfg.StoreInterval, cfg.Restore, log)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(ctx)

	var wg sync.WaitGroup
	defer wg.Wait()
	defer cancel()

	wg.Go(func() { store.Run(ctx) })

	log.Info("starting server",
		zap.String("address", cfg.Address),
		zap.Duration("store_interval", cfg.StoreInterval),
		zap.String("file_storage_path", cfg.FileStoragePath),
		zap.Bool("restore", cfg.Restore),
	)

	srv := newServer(cfg, store, log)

	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.ListenAndServe() }()

	select {
	case err := <-serveErr:
		return errors.Join(err, store.Save())
	case <-ctx.Done():
	}

	log.Info("shutting down server")

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancelShutdown()

	shutdownErr := srv.Shutdown(shutdownCtx)

	cancel()
	wg.Wait()

	return errors.Join(shutdownErr, store.Save())
}

func newServer(cfg config.Server, store handler.Storage, log *zap.Logger) *http.Server {
	return &http.Server{
		Addr:    cfg.Address,
		Handler: handler.NewRouter(store, log),
	}
}
