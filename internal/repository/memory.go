package repository

import (
	"maps"
	"sync"
)

type MemStorage struct {
	mu       sync.RWMutex
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

func (m *MemStorage) UpdateCounter(name string, delta int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.counters[name] += delta
}

func (m *MemStorage) Gauge(name string) (float64, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	value, ok := m.gauges[name]

	return value, ok
}

func (m *MemStorage) Counter(name string) (int64, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	delta, ok := m.counters[name]

	return delta, ok
}

func (m *MemStorage) Gauges() map[string]float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return maps.Clone(m.gauges)
}

func (m *MemStorage) Counters() map[string]int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return maps.Clone(m.counters)
}
