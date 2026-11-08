package agent

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/Kusaykin/go-telemetry/internal/config"
	models "github.com/Kusaykin/go-telemetry/internal/model"
	"go.uber.org/zap"
)

type Agent struct {
	cfg          config.Agent
	collector    *Collector
	client       *Client
	log          *zap.Logger
	elapsed      time.Duration
	flushTimeout time.Duration
}

const flushTimeout = 5 * time.Second

func New(cfg config.Agent, log *zap.Logger) *Agent {
	return &Agent{
		cfg:       cfg,
		collector: NewCollector(),
		client:    NewClient(cfg.Address),
		log:       log,

		flushTimeout: flushTimeout,
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

	sendCtx, cancelSend := context.WithCancel(context.WithoutCancel(ctx))
	defer cancelSend()

	sending := make(chan struct{}, 1)
	var wg sync.WaitGroup

	for {
		select {
		case <-ctx.Done():
			a.log.Info("stopping agent")
			a.flush(ctx, cancelSend, &wg)
			return
		case <-ticker.C:
			a.step(sendCtx, sending, &wg)
		}
	}
}

func (a *Agent) step(ctx context.Context, sending chan struct{}, wg *sync.WaitGroup) {
	if !a.poll() {
		return
	}

	select {
	case sending <- struct{}{}:
		a.elapsed = 0
		r := a.prepare()
		wg.Go(func() {
			defer func() { <-sending }()
			a.send(ctx, r)
		})
	default:
		a.log.Warn("report postponed: previous report is still in progress")
	}
}

func (a *Agent) flush(ctx context.Context, cancelInflight context.CancelFunc, wg *sync.WaitGroup) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), a.flushTimeout)
	defer cancel()

	timer := time.AfterFunc(a.flushTimeout/2, cancelInflight)
	wg.Wait()
	timer.Stop()

	a.send(ctx, a.prepare())
}

type report struct {
	metrics   []models.Metrics
	pollCount int64
}

func (a *Agent) poll() bool {
	a.collector.Poll()

	a.elapsed += a.cfg.PollInterval

	return a.elapsed >= a.cfg.ReportInterval
}

func (a *Agent) prepare() report {
	snapshot := a.collector.Snapshot()
	pollCount := a.collector.PollCountDelta()

	return report{metrics: a.filterReport(snapshot), pollCount: pollCount}
}

func (a *Agent) send(ctx context.Context, r report) {
	if err := a.client.SendBatch(ctx, r.metrics); err != nil {
		a.log.Error("report failed", zap.Error(err))
		return
	}

	if slices.ContainsFunc(r.metrics, isPollCount) {
		a.collector.AckPollCount(r.pollCount)
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
