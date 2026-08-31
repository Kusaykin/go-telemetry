package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/Kusaykin/go-telemetry/internal/agent"
)

func parseFlags(args []string, errOut io.Writer) (agent.Config, error) {
	cfg := agent.DefaultConfig()

	fs := flag.NewFlagSet("agent", flag.ContinueOnError)
	fs.SetOutput(errOut)

	fs.StringVar(&cfg.Address, "a", cfg.Address, "адрес эндпоинта HTTP-сервера")
	report := fs.Int("r", int(agent.DefaultReportInterval.Seconds()), "частота отправки метрик на сервер, секунды")
	poll := fs.Int("p", int(agent.DefaultPollInterval.Seconds()), "частота опроса метрик из runtime, секунды")

	if err := fs.Parse(args); err != nil {
		return agent.Config{}, err
	}

	fail := func(err error) (agent.Config, error) {
		fmt.Fprintln(errOut, err)
		fs.Usage()

		return agent.Config{}, err
	}

	if fs.NArg() > 0 {
		return fail(fmt.Errorf("неизвестный аргумент: %q", fs.Arg(0)))
	}

	if *report <= 0 {
		return fail(fmt.Errorf("-r должен быть положительным, получено %d", *report))
	}

	if *poll <= 0 {
		return fail(fmt.Errorf("-p должен быть положительным, получено %d", *poll))
	}

	cfg.ReportInterval = time.Duration(*report) * time.Second
	cfg.PollInterval = time.Duration(*poll) * time.Second

	return cfg, nil
}

func main() {
	cfg, err := parseFlags(os.Args[1:], os.Stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}

		os.Exit(2)
	}

	agent.New(cfg).Run()
}
