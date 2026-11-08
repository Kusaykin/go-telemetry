package agent

import (
	"context"
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
	flushTimeout time.Duration
}

const flushTimeout = 5 * time.Second

func New(cfg config.Agent, log *zap.Logger) *Agent {
	return &Agent{
		cfg:       cfg,
		collector: NewCollector(),
		client:    NewClient(cfg.Address, cfg.Key),
		log:       log,

		flushTimeout: flushTimeout,
	}
}

type report struct {
	metrics   []models.Metrics
	pollCount int64
}

func (a *Agent) Run(ctx context.Context) {
	a.log.Info("starting agent",
		zap.String("address", a.cfg.Address),
		zap.Duration("poll_interval", a.cfg.PollInterval),
		zap.Duration("report_interval", a.cfg.ReportInterval),
		zap.Int("rate_limit", a.cfg.RateLimit),
		zap.Bool("signing", a.cfg.Key != ""),
	)

	sendCtx, cancelSend := context.WithCancel(context.WithoutCancel(ctx))
	defer cancelSend()

	jobs := make(chan report, 1)

	var workers sync.WaitGroup
	for range a.cfg.RateLimit {
		workers.Go(func() { a.worker(sendCtx, jobs) })
	}

	var producers sync.WaitGroup
	producers.Go(func() { every(ctx, a.cfg.PollInterval, a.collector.Poll) })
	producers.Go(func() { every(ctx, a.cfg.PollInterval, func() { a.pollSystem(ctx) }) })
	producers.Go(func() { every(ctx, a.cfg.ReportInterval, func() { a.enqueue(jobs) }) })

	<-ctx.Done()
	a.log.Info("stopping agent")

	producers.Wait()
	close(jobs)
	a.flush(ctx, cancelSend, &workers)
}

func every(ctx context.Context, interval time.Duration, fn func()) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			fn()
		}
	}
}

func (a *Agent) pollSystem(ctx context.Context) {
	if err := a.collector.PollSystem(ctx); err != nil && ctx.Err() == nil {
		a.log.Warn("system metrics poll failed", zap.Error(err))
	}
}

func (a *Agent) enqueue(jobs chan report) {
	select {
	case old := <-jobs:
		a.collector.RestorePollCount(old.pollCount)
		a.log.Info("pending report replaced: all workers are busy", zap.Int("rate_limit", a.cfg.RateLimit))
	default:
	}

	jobs <- a.prepare()
}

func (a *Agent) worker(ctx context.Context, jobs <-chan report) {
	for r := range jobs {
		a.send(ctx, r)
	}
}

func (a *Agent) flush(ctx context.Context, cancelInflight context.CancelFunc, workers *sync.WaitGroup) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), a.flushTimeout)
	defer cancel()

	timer := time.AfterFunc(a.flushTimeout/2, cancelInflight)
	workers.Wait()
	timer.Stop()

	a.send(ctx, a.prepare())
}

func (a *Agent) prepare() report {
	metrics, pollCount := a.collector.TakeSnapshot()

	return report{metrics: a.filterReport(metrics), pollCount: pollCount}
}

func (a *Agent) send(ctx context.Context, r report) {
	if err := a.client.SendBatch(ctx, r.metrics); err != nil {
		a.collector.RestorePollCount(r.pollCount)
		a.log.Error("report failed", zap.Error(err))
	}
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
