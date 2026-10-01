package main

import (
	"os"

	"github.com/Kusaykin/go-telemetry/internal/agent"
	"github.com/Kusaykin/go-telemetry/internal/config"
)

func main() {
	cfg, err := config.LoadAgent(os.Args[1:], os.Stderr)
	if err != nil {
		os.Exit(config.ExitCode(err))
	}

	agent.New(cfg).Run()
}
