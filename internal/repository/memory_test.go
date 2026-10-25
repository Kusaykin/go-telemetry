package repository

import (
	"math"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	models "github.com/Kusaykin/go-telemetry/internal/model"
)

func TestUpdateGauge(t *testing.T) {
	m := NewMemStorage()

	mustUpdateGauge(t, m, "Alloc", 1.5)
	assert.Equal(t, 1.5, m.gauges["Alloc"])

	mustUpdateGauge(t, m, "Alloc", -2.25)
	assert.Equal(t, -2.25, m.gauges["Alloc"])
}

func TestUpdateCounter(t *testing.T) {
	m := NewMemStorage()

	assert.Equal(t, int64(5), mustUpdateCounter(t, m, "PollCount", 5))
	assert.Equal(t, int64(5), m.counters["PollCount"])

	assert.Equal(t, int64(15), mustUpdateCounter(t, m, "PollCount", 10))
	assert.Equal(t, int64(15), m.counters["PollCount"])
}

func TestUpdateBatch(t *testing.T) {
	checkUpdateBatch(t, NewMemStorage())
}

func TestUpdateBatchOverflow(t *testing.T) {
	checkUpdateBatchOverflow(t, NewMemStorage())
}

func TestUpdateCounterOverflow(t *testing.T) {
	m := NewMemStorage()
	mustUpdateCounter(t, m, "PollCount", math.MaxInt64)

	_, err := m.UpdateCounter(t.Context(), "PollCount", 1)

	require.ErrorIs(t, err, models.ErrCounterOverflow)
	assert.Equal(t, int64(math.MaxInt64), m.counters["PollCount"])
}

func TestUpdateBatchConcurrent(t *testing.T) {
	m := NewMemStorage()

	const workers, batches = 8, 20

	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for range batches {
				assert.NoError(t, m.UpdateBatch(t.Context(), []models.Metrics{
					counterMetric("PollCount", 1),
					counterMetric("PollCount", 1),
				}))
			}
		})
	}
	wg.Wait()

	assert.Equal(t, int64(2*workers*batches), m.counters["PollCount"])
}

func TestGauge(t *testing.T) {
	m := NewMemStorage()
	mustUpdateGauge(t, m, "Alloc", 12.5)

	value, ok := mustGauge(t, m, "Alloc")
	assert.True(t, ok)
	assert.Equal(t, 12.5, value)

	_, ok = mustGauge(t, m, "Unknown")
	assert.False(t, ok)
}

func TestCounter(t *testing.T) {
	m := NewMemStorage()
	mustUpdateCounter(t, m, "PollCount", 5)
	mustUpdateCounter(t, m, "PollCount", 10)

	delta, ok := mustCounter(t, m, "PollCount")
	assert.True(t, ok)
	assert.Equal(t, int64(15), delta)

	_, ok = mustCounter(t, m, "Unknown")
	assert.False(t, ok)
}

func TestZeroValueIsFound(t *testing.T) {
	m := NewMemStorage()
	mustUpdateGauge(t, m, "Zero", 0)
	mustUpdateCounter(t, m, "ZeroCount", 0)

	_, ok := mustGauge(t, m, "Zero")
	assert.True(t, ok)

	_, ok = mustCounter(t, m, "ZeroCount")
	assert.True(t, ok)
}

func TestSnapshotsAreCopies(t *testing.T) {
	m := NewMemStorage()
	mustUpdateGauge(t, m, "Alloc", 1)
	mustUpdateCounter(t, m, "PollCount", 1)

	gauges := mustGauges(t, m)
	assert.Equal(t, map[string]float64{"Alloc": 1}, gauges)
	gauges["Alloc"] = 100
	delete(gauges, "Alloc")

	counters := mustCounters(t, m)
	assert.Equal(t, map[string]int64{"PollCount": 1}, counters)
	counters["PollCount"] = 100

	value, ok := mustGauge(t, m, "Alloc")
	assert.True(t, ok)
	assert.Equal(t, float64(1), value)

	delta, ok := mustCounter(t, m, "PollCount")
	assert.True(t, ok)
	assert.Equal(t, int64(1), delta)
}

func TestConcurrentAccess(t *testing.T) {
	ctx := t.Context()
	m := NewMemStorage()

	var wg sync.WaitGroup

	for i := range 50 {
		wg.Add(3)

		go func() { defer wg.Done(); assert.NoError(t, m.UpdateGauge(ctx, "Alloc", float64(i))) }()
		go func() {
			defer wg.Done()
			_, err := m.UpdateCounter(ctx, "PollCount", 1)
			assert.NoError(t, err)
		}()
		go func() {
			defer wg.Done()
			_, _, _ = m.Gauge(ctx, "Alloc")
			_, _, _ = m.Counter(ctx, "PollCount")
			_, _ = m.Gauges(ctx)
			_, _ = m.Counters(ctx)
		}()
	}

	wg.Wait()

	delta, ok := mustCounter(t, m, "PollCount")
	assert.True(t, ok)
	assert.Equal(t, int64(50), delta)
}

func TestSnapshotSortedAndIndependent(t *testing.T) {
	m := NewMemStorage()
	mustUpdateGauge(t, m, "b", 2)
	mustUpdateGauge(t, m, "a", 1)
	mustUpdateCounter(t, m, "c", 3)

	snapshot := m.Snapshot()

	var got []string
	for _, metric := range snapshot {
		got = append(got, metric.MType+"/"+metric.ID)
	}
	assert.Equal(t, []string{"counter/c", "gauge/a", "gauge/b"}, got)

	*snapshot[1].Value = 100
	value, _ := mustGauge(t, m, "a")
	assert.Equal(t, 1.0, value, "изменение снимка не должно менять хранилище")
}

func TestRestoreReplacesContent(t *testing.T) {
	m := NewMemStorage()
	mustUpdateGauge(t, m, "Old", 1)

	src := NewMemStorage()
	mustUpdateGauge(t, src, "Alloc", 12.5)
	mustUpdateCounter(t, src, "PollCount", 5)

	assert.NoError(t, m.restore(src.Snapshot()))
	assert.Equal(t, map[string]float64{"Alloc": 12.5}, mustGauges(t, m))
	assert.Equal(t, map[string]int64{"PollCount": 5}, mustCounters(t, m))
}

func TestRestoreInvalidKeepsContent(t *testing.T) {
	m := NewMemStorage()
	mustUpdateGauge(t, m, "Alloc", 1)

	src := NewMemStorage()
	mustUpdateGauge(t, src, "Good", 2)
	metrics := src.Snapshot()
	metrics = append(metrics, models.Metrics{ID: "Bad", MType: models.Gauge})

	assert.Error(t, m.restore(metrics))
	assert.Equal(t, map[string]float64{"Alloc": 1}, mustGauges(t, m))
}
