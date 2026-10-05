package repository

import (
	"cmp"
	"errors"
	"maps"
	"slices"
	"sync"

	models "github.com/Kusaykin/go-telemetry/internal/model"
)

type MemStorage struct {
	mu       sync.Mutex
	gauges   map[string]float64
	counters map[string]int64
}

func NewMemStorage() *MemStorage {
	return &MemStorage{
		gauges:   make(map[string]float64),
		counters: make(map[string]int64),
	}
}

func (m *MemStorage) UpdateGauge(name string, value float64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.gauges[name] = value
}

func (m *MemStorage) UpdateCounter(name string, delta int64) int64 {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.counters[name] += delta

	return m.counters[name]
}

func (m *MemStorage) Gauge(name string) (float64, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	value, ok := m.gauges[name]

	return value, ok
}

func (m *MemStorage) Counter(name string) (int64, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delta, ok := m.counters[name]

	return delta, ok
}

func (m *MemStorage) Gauges() map[string]float64 {
	m.mu.Lock()
	defer m.mu.Unlock()

	return maps.Clone(m.gauges)
}

func (m *MemStorage) Counters() map[string]int64 {
	m.mu.Lock()
	defer m.mu.Unlock()

	return maps.Clone(m.counters)
}

func (m *MemStorage) Snapshot() []models.Metrics {
	m.mu.Lock()
	defer m.mu.Unlock()

	metrics := make([]models.Metrics, 0, len(m.gauges)+len(m.counters))

	for name, value := range m.gauges {
		metrics = append(metrics, models.Metrics{ID: name, MType: models.Gauge, Value: &value})
	}

	for name, delta := range m.counters {
		metrics = append(metrics, models.Metrics{ID: name, MType: models.Counter, Delta: &delta})
	}

	slices.SortFunc(metrics, func(a, b models.Metrics) int {
		return cmp.Or(cmp.Compare(a.MType, b.MType), cmp.Compare(a.ID, b.ID))
	})

	return metrics
}

func (m *MemStorage) restore(metrics []models.Metrics) error {
	gauges := make(map[string]float64)
	counters := make(map[string]int64)

	for _, metric := range metrics {
		if metric.ID == "" {
			return errors.New("metric id is empty")
		}

		if err := metric.Validate(); err != nil {
			return err
		}

		switch metric.MType {
		case models.Gauge:
			gauges[metric.ID] = *metric.Value
		case models.Counter:
			counters[metric.ID] = *metric.Delta
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.gauges = gauges
	m.counters = counters

	return nil
}
