package config

import (
	"io"
	"time"
)

const (
	DefaultPollInterval   = 2 * time.Second
	DefaultReportInterval = 10 * time.Second
)

type Agent struct {
	Address        string
	PollInterval   time.Duration
	ReportInterval time.Duration
}

func DefaultAgent() Agent {
	return Agent{
		Address:        DefaultAddress,
		PollInterval:   DefaultPollInterval,
		ReportInterval: DefaultReportInterval,
	}
}

func LoadAgent(args []string, errOut io.Writer) (Agent, error) {
	cfg := DefaultAgent()

	fs := newFlagSet("agent", errOut)
	fs.StringVar(&cfg.Address, "a", cfg.Address, addressUsage)
	secondsVar(fs, &cfg.ReportInterval, "r", "частота отправки метрик на сервер, `секунды`")
	secondsVar(fs, &cfg.PollInterval, "p", "частота опроса метрик из runtime, `секунды`")

	if err := parse(fs, args, errOut); err != nil {
		return Agent{}, err
	}

	return cfg, nil
}
