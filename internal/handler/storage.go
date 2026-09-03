package handler

type Storage interface {
	UpdateGauge(name string, value float64)
	UpdateCounter(name string, delta int64)
	Gauge(name string) (float64, bool)
	Counter(name string) (int64, bool)
	Gauges() map[string]float64
	Counters() map[string]int64
}

type Handler struct {
	store Storage
}

func New(store Storage) *Handler {
	return &Handler{store: store}
}
