package handler

import "go.uber.org/zap"

type Storage interface {
	UpdateGauge(name string, value float64)
	UpdateCounter(name string, delta int64) int64
	Gauge(name string) (float64, bool)
	Counter(name string) (int64, bool)
	Gauges() map[string]float64
	Counters() map[string]int64
}

type Handler struct {
	store Storage
	db    Pinger
	log   *zap.Logger
}

// New создаёт обработчики. db может быть nil — тогда /ping отвечает 500.
func New(store Storage, db Pinger, log *zap.Logger) *Handler {
	return &Handler{store: store, db: db, log: log}
}
