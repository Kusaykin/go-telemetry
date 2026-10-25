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

func LoadAgent(args []string, lookup LookupEnv, errOut io.Writer) (Agent, error) {
	cfg := DefaultAgent()

	fs := newFlagSet("agent", errOut)
	fs.stringVar(&cfg.Address, "a", envAddress, addressUsage)
	fs.secondsVar(&cfg.ReportInterval, "r", envReportInterval, "частота отправки метрик на сервер, `секунды`")
	fs.secondsVar(&cfg.PollInterval, "p", envPollInterval, "частота опроса метрик из runtime, `секунды`")

	if err := fs.parse(args, lookup); err != nil {
		return Agent{}, err
	}

	return cfg, nil
}
