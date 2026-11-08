package handler

import (
	"context"

	"go.uber.org/zap"

	models "github.com/Kusaykin/go-telemetry/internal/model"
)

type Storage interface {
	UpdateGauge(ctx context.Context, name string, value float64) error
	UpdateCounter(ctx context.Context, name string, delta int64) (int64, error)
	UpdateBatch(ctx context.Context, metrics []models.Metrics) error
	Gauge(ctx context.Context, name string) (float64, bool, error)
	Counter(ctx context.Context, name string) (int64, bool, error)
	Gauges(ctx context.Context) (map[string]float64, error)
	Counters(ctx context.Context) (map[string]int64, error)
}

type Handler struct {
	store Storage
	db    Pinger
	log   *zap.Logger
}

func New(store Storage, db Pinger, log *zap.Logger) *Handler {
	return &Handler{store: store, db: db, log: log}
}
