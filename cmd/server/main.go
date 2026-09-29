package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/Kusaykin/go-telemetry/internal/config"
	"github.com/Kusaykin/go-telemetry/internal/handler"
	"github.com/Kusaykin/go-telemetry/internal/logger"
	"github.com/Kusaykin/go-telemetry/internal/repository"
	"go.uber.org/zap"
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

	if err := run(cfg, log); err != nil {
		log.Fatal("server stopped", zap.Error(err))
	}
}

func run(cfg config.Server, log *zap.Logger) error {
	log.Info("starting server", zap.String("address", cfg.Address))

	return newServer(cfg, log).ListenAndServe()
}

func newServer(cfg config.Server, log *zap.Logger) *http.Server {
	store := repository.NewMemStorage()

	return &http.Server{
		Addr:    cfg.Address,
		Handler: handler.NewRouter(store, log),
	}
}
