package main

import (
	"log"
	"net/http"
	"os"

	"github.com/Kusaykin/go-telemetry/internal/config"
	"github.com/Kusaykin/go-telemetry/internal/handler"
	"github.com/Kusaykin/go-telemetry/internal/repository"
)

func main() {
	cfg, err := config.LoadServer(os.Args[1:], os.Stderr)
	if err != nil {
		os.Exit(config.ExitCode(err))
	}

	if err := run(cfg); err != nil {
		log.Fatal(err)
	}
}

func run(cfg config.Server) error {
	return newServer(cfg).ListenAndServe()
}

func newServer(cfg config.Server) *http.Server {
	store := repository.NewMemStorage()

	return &http.Server{
		Addr:    cfg.Address,
		Handler: handler.NewRouter(store),
	}
}
