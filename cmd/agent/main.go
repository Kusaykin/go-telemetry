package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/Kusaykin/go-telemetry/internal/agent"
	"github.com/Kusaykin/go-telemetry/internal/config"
	"github.com/Kusaykin/go-telemetry/internal/logger"
)

func main() {
	cfg, err := config.LoadAgent(os.Args[1:], os.LookupEnv, os.Stderr)
	if err != nil {
		os.Exit(config.ExitCode(err))
	}

	log, err := logger.NewConsole("info")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer log.Sync()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	agent.New(cfg, log).Run(ctx)
}
