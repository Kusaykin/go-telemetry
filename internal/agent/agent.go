package agent

import (
	"context"
	"slices"
	"time"

	"github.com/Kusaykin/go-telemetry/internal/config"
	models "github.com/Kusaykin/go-telemetry/internal/model"
	"go.uber.org/zap"
)

type Agent struct {
	cfg       config.Agent
	collector *Collector
	client    *Client
	log       *zap.Logger
	elapsed   time.Duration // время, прошедшее с последнего отчёта
}

func New(cfg config.Agent, log *zap.Logger) *Agent {
	return &Agent{
		cfg:       cfg,
		collector: NewCollector(),
		client:    NewClient(cfg.Address),
		log:       log,
	}
}

func (a *Agent) Run(ctx context.Context) {
	a.log.Info("starting agent",
		zap.String("address", a.cfg.Address),
		zap.Duration("poll_interval", a.cfg.PollInterval),
		zap.Duration("report_interval", a.cfg.ReportInterval),
	)

	ticker := time.NewTicker(a.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			a.log.Info("stopping agent")
			return
		case <-ticker.C:
			a.tick()
		}
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

	metrics := a.filterReport(snapshot)

	if err := a.client.SendBatch(metrics); err != nil {
		a.log.Error("report failed", zap.Error(err))
		return
	}

	if slices.ContainsFunc(metrics, isPollCount) {
		a.collector.AckPollCount(pollCount)
	}
}

func isPollCount(m models.Metrics) bool {
	return m.MType == models.Counter && m.ID == PollCountName
}

func (a *Agent) filterReport(snapshot []models.Metrics) []models.Metrics {
	a.log.Info("report", zap.Int("metrics", len(snapshot)))

	valid := make([]models.Metrics, 0, len(snapshot))
	for _, m := range snapshot {
		value, err := m.ValueString()
		if err != nil {
			a.log.Error("metric skipped",
				zap.String("type", m.MType),
				zap.String("id", m.ID),
				zap.Error(err),
			)
			continue
		}
		a.log.Debug("metric",
			zap.String("type", m.MType),
			zap.String("id", m.ID),
			zap.String("value", value),
		)
		valid = append(valid, m)
	}

	return valid
}
