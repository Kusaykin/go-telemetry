package repository

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// metricStore — методы, общие для всех хранилищ. Хелперы ниже проверяют
// ошибку, чтобы тесты читались так же, как до появления context и error.
type metricStore interface {
	UpdateGauge(ctx context.Context, name string, value float64) error
	UpdateCounter(ctx context.Context, name string, delta int64) (int64, error)
	Gauge(ctx context.Context, name string) (float64, bool, error)
	Counter(ctx context.Context, name string) (int64, bool, error)
	Gauges(ctx context.Context) (map[string]float64, error)
	Counters(ctx context.Context) (map[string]int64, error)
}

func mustUpdateGauge(t *testing.T, s metricStore, name string, value float64) {
	t.Helper()

	require.NoError(t, s.UpdateGauge(t.Context(), name, value))
}

func mustUpdateCounter(t *testing.T, s metricStore, name string, delta int64) int64 {
	t.Helper()

	value, err := s.UpdateCounter(t.Context(), name, delta)
	require.NoError(t, err)

	return value
}

func mustGauge(t *testing.T, s metricStore, name string) (float64, bool) {
	t.Helper()

	value, ok, err := s.Gauge(t.Context(), name)
	require.NoError(t, err)

	return value, ok
}

func mustCounter(t *testing.T, s metricStore, name string) (int64, bool) {
	t.Helper()

	delta, ok, err := s.Counter(t.Context(), name)
	require.NoError(t, err)

	return delta, ok
}

func mustGauges(t *testing.T, s metricStore) map[string]float64 {
	t.Helper()

	gauges, err := s.Gauges(t.Context())
	require.NoError(t, err)

	return gauges
}

func mustCounters(t *testing.T, s metricStore) map[string]int64 {
	t.Helper()

	counters, err := s.Counters(t.Context())
	require.NoError(t, err)

	return counters
}
