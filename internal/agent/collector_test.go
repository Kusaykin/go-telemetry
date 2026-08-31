package agent

import (
	"testing"

	models "github.com/Kusaykin/go-telemetry/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const metricsCount = 29

var wantRuntimeGauges = []string{
	"Alloc",
	"BuckHashSys",
	"Frees",
	"GCCPUFraction",
	"GCSys",
	"HeapAlloc",
	"HeapIdle",
	"HeapInuse",
	"HeapObjects",
	"HeapReleased",
	"HeapSys",
	"LastGC",
	"Lookups",
	"MCacheInuse",
	"MCacheSys",
	"MSpanInuse",
	"MSpanSys",
	"Mallocs",
	"NextGC",
	"NumForcedGC",
	"NumGC",
	"OtherSys",
	"PauseTotalNs",
	"StackInuse",
	"StackSys",
	"Sys",
	"TotalAlloc",
}

func findMetric(metrics []models.Metrics, id string) models.Metrics {
	for _, m := range metrics {
		if m.ID == id {
			return m
		}
	}

	return models.Metrics{}
}

func TestSnapshotHasAllMetrics(t *testing.T) {
	c := NewCollector()
	c.Poll()

	metrics := c.Snapshot()
	require.Len(t, metrics, metricsCount)

	for _, name := range wantRuntimeGauges {
		m := findMetric(metrics, name)
		assert.Equal(t, name, m.ID, "нет метрики "+name)
		assert.Equal(t, models.Gauge, m.MType)
		assert.NotNil(t, m.Value)
	}

	random := findMetric(metrics, RandomValueName)
	assert.Equal(t, models.Gauge, random.MType)
	assert.NotNil(t, random.Value)

	poll := findMetric(metrics, PollCountName)
	assert.Equal(t, models.Counter, poll.MType)
	assert.NotNil(t, poll.Delta)
}

func TestPollCount(t *testing.T) {
	c := NewCollector()
	c.Poll()
	c.Poll()
	c.Poll()

	m := findMetric(c.Snapshot(), PollCountName)
	require.NotNil(t, m.Delta)
	assert.Equal(t, int64(3), *m.Delta)
}

func TestRandomValueChanges(t *testing.T) {
	c := NewCollector()

	c.Poll()
	first := findMetric(c.Snapshot(), RandomValueName)
	require.NotNil(t, first.Value)

	c.Poll()
	second := findMetric(c.Snapshot(), RandomValueName)
	require.NotNil(t, second.Value)

	assert.NotEqual(t, *first.Value, *second.Value)
}

func TestMemStatsAreRead(t *testing.T) {
	c := NewCollector()
	c.Poll()

	m := findMetric(c.Snapshot(), "Sys")
	require.NotNil(t, m.Value)
	assert.Greater(t, *m.Value, float64(0))
}
