package agent

import (
	"fmt"
	"io"
	"log"
	"os"
	"time"

	models "github.com/Kusaykin/go-telemetry/internal/model"
)

const (
	DefaultAddress        = "localhost:8080"
	DefaultPollInterval   = 2 * time.Second
	DefaultReportInterval = 10 * time.Second
)

type Config struct {
	Address        string
	PollInterval   time.Duration
	ReportInterval time.Duration
}

func DefaultConfig() Config {
	return Config{
		Address:        DefaultAddress,
		PollInterval:   DefaultPollInterval,
		ReportInterval: DefaultReportInterval,
	}
}

type Agent struct {
	cfg       Config
	collector *Collector
	client    *Client
	out       io.Writer
	elapsed   time.Duration // время, прошедшее с последнего отчёта
}

func New(cfg Config) *Agent {
	return &Agent{
		cfg:       cfg,
		collector: NewCollector(),
		client:    NewClient(cfg.Address),
		out:       os.Stdout,
	}
}

func (a *Agent) Run() {
	for {
		time.Sleep(a.cfg.PollInterval)
		a.tick()
	}
}

func (a *Agent) tick() {
	a.collector.Poll()

	a.elapsed += a.cfg.PollInterval
	if a.elapsed < a.cfg.ReportInterval {
		return
	}
	a.elapsed = 0

	snapshot := a.collector.Snapshot()
	pollCount := a.collector.PollCountDelta()

	a.logReport(snapshot)

	if err := a.client.SendAll(snapshot); err != nil {
		log.Println("report failed:", err)
		return
	}

	a.collector.AckPollCount(pollCount)
}

func (a *Agent) logReport(snapshot []models.Metrics) {
	fmt.Fprintf(a.out, "--- отчёт, метрик: %d ---\n", len(snapshot))

	for _, m := range snapshot {
		value, err := metricValue(m)
		if err != nil {
			fmt.Fprintf(a.out, "%-7s %-14s ошибка: %v\n", m.MType, m.ID, err)
			continue
		}
		fmt.Fprintf(a.out, "%-7s %-14s %s\n", m.MType, m.ID, value)
	}
}
