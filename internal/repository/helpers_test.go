package repository

import (
	"context"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	models "github.com/Kusaykin/go-telemetry/internal/model"
)

func gaugeMetric(id string, value float64) models.Metrics {
	return models.Metrics{ID: id, MType: models.Gauge, Value: &value}
}

func counterMetric(id string, delta int64) models.Metrics {
	return models.Metrics{ID: id, MType: models.Counter, Delta: &delta}
}

func checkUpdateBatch(t *testing.T, s metricStore) {
	t.Helper()

	mustUpdateCounter(t, s, "PollCount", 1)

	require.NoError(t, s.UpdateBatch(t.Context(), []models.Metrics{
		gaugeMetric("Alloc", 1.5),
		counterMetric("PollCount", 2),
		gaugeMetric("Alloc", 2.5),
		gaugeMetric("Sys", 0),
		counterMetric("PollCount", 3),
	}))

	assert.Equal(t, map[string]float64{"Alloc": 2.5, "Sys": 0}, mustGauges(t, s))
	assert.Equal(t, map[string]int64{"PollCount": 6}, mustCounters(t, s))

	require.NoError(t, s.UpdateBatch(t.Context(), nil))
	assert.Equal(t, map[string]int64{"PollCount": 6}, mustCounters(t, s))
}

func checkUpdateBatchOverflow(t *testing.T, s metricStore) {
	t.Helper()

	mustUpdateGauge(t, s, "Alloc", 1)
	mustUpdateCounter(t, s, "PollCount", 1)

	err := s.UpdateBatch(t.Context(), []models.Metrics{
		gaugeMetric("Alloc", 2),
		counterMetric("Big", math.MaxInt64),
		counterMetric("Big", 1),
	})
	require.ErrorIs(t, err, models.ErrCounterOverflow)

	err = s.UpdateBatch(t.Context(), []models.Metrics{
		gaugeMetric("Alloc", 2),
		counterMetric("PollCount", math.MaxInt64),
	})
	require.ErrorIs(t, err, models.ErrCounterOverflow)

	assert.Equal(t, map[string]float64{"Alloc": 1}, mustGauges(t, s))
	assert.Equal(t, map[string]int64{"PollCount": 1}, mustCounters(t, s))
}

type metricStore interface {
	UpdateGauge(ctx context.Context, name string, value float64) error
	UpdateCounter(ctx context.Context, name string, delta int64) (int64, error)
	UpdateBatch(ctx context.Context, metrics []models.Metrics) error
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
