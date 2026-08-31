package main

import (
	"github.com/Kusaykin/go-telemetry/internal/agent"
)

func main() {
	agent.New(agent.DefaultConfig()).Run()
}
