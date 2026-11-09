package config

import (
	"io"
	"time"
)

const (
	DefaultPollInterval   = 2 * time.Second
	DefaultReportInterval = 10 * time.Second
	DefaultRateLimit      = 1
	MaxRateLimit          = 1024
)

type Agent struct {
	Address        string
	PollInterval   time.Duration
	ReportInterval time.Duration
	Key            string
	RateLimit      int
}

func DefaultAgent() Agent {
	return Agent{
		Address:        DefaultAddress,
		PollInterval:   DefaultPollInterval,
		ReportInterval: DefaultReportInterval,
		RateLimit:      DefaultRateLimit,
	}
}

func LoadAgent(args []string, lookup LookupEnv, errOut io.Writer) (Agent, error) {
	cfg := DefaultAgent()

	fs := newFlagSet("agent", errOut)
	fs.stringVar(&cfg.Address, "a", envAddress, addressUsage)
	fs.secondsVar(&cfg.ReportInterval, "r", envReportInterval, "частота отправки метрик на сервер, `секунды`")
	fs.secondsVar(&cfg.PollInterval, "p", envPollInterval, "частота опроса метрик из runtime, `секунды`")
	fs.stringVar(&cfg.Key, "k", envKey, keyUsage)
	fs.boundedIntVar(&cfg.RateLimit, "l", envRateLimit, "максимальное `число` одновременных запросов к серверу", MaxRateLimit)

	if err := fs.parse(args, lookup); err != nil {
		return Agent{}, err
	}

	return cfg, nil
}
