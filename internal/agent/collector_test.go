package agent

import (
	"strconv"
	"testing"

	"github.com/shirou/gopsutil/v4/cpu"

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

	metrics, _ := c.TakeSnapshot()
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

func snapshot(c *Collector) []models.Metrics {
	metrics, _ := c.TakeSnapshot()
	return metrics
}

func TestPollCount(t *testing.T) {
	c := NewCollector()
	c.Poll()
	c.Poll()
	c.Poll()
	assert.Equal(t, int64(3), c.PollCount())

	metrics, taken := c.TakeSnapshot()
	m := findMetric(metrics, PollCountName)
	require.NotNil(t, m.Delta)
	assert.Equal(t, int64(3), *m.Delta)
	assert.Equal(t, int64(3), taken)
}

func TestTakeSnapshotResetsPollCount(t *testing.T) {
	c := NewCollector()
	c.Poll()
	c.Poll()

	_, taken := c.TakeSnapshot()
	assert.Equal(t, int64(2), taken)
	assert.Zero(t, c.PollCount())

	c.Poll()

	m := findMetric(snapshot(c), PollCountName)
	require.NotNil(t, m.Delta)
	assert.Equal(t, int64(1), *m.Delta)
}

func TestRestorePollCountKeepsNewPolls(t *testing.T) {
	c := NewCollector()
	c.Poll()
	c.Poll()

	_, taken := c.TakeSnapshot()
	c.Poll()
	c.RestorePollCount(taken)

	assert.Equal(t, int64(3), c.PollCount())
}

func TestPollSystem(t *testing.T) {
	c := NewCollector()
	require.NoError(t, c.PollSystem(t.Context()))

	cpus, err := cpu.Counts(true)
	require.NoError(t, err)

	metrics := snapshot(c)
	require.Len(t, metrics, systemGaugesWithoutCPU+cpus+1)

	total := findMetric(metrics, TotalMemoryName)
	require.NotNil(t, total.Value)
	assert.Positive(t, *total.Value)

	free := findMetric(metrics, FreeMemoryName)
	require.NotNil(t, free.Value)
	assert.LessOrEqual(t, *free.Value, *total.Value)

	for i := 1; i <= cpus; i++ {
		name := CPUUtilizationPrefix + strconv.Itoa(i)
		m := findMetric(metrics, name)
		require.Equal(t, name, m.ID, "нет метрики "+name)
		assert.Equal(t, models.Gauge, m.MType)
		require.NotNil(t, m.Value)
		assert.GreaterOrEqual(t, *m.Value, float64(0))
		assert.LessOrEqual(t, *m.Value, float64(100))
	}

	assert.Zero(t, c.PollCount(), "PollSystem не увеличивает PollCount")
}

func TestSnapshotMergesRuntimeAndSystem(t *testing.T) {
	c := NewCollector()
	c.Poll()
	require.NoError(t, c.PollSystem(t.Context()))

	cpus, err := cpu.Counts(true)
	require.NoError(t, err)

	assert.Len(t, snapshot(c), metricsCount+systemGaugesWithoutCPU+cpus)
}

func TestRandomValueChanges(t *testing.T) {
	c := NewCollector()

	c.Poll()
	first := findMetric(snapshot(c), RandomValueName)
	require.NotNil(t, first.Value)

	c.Poll()
	second := findMetric(snapshot(c), RandomValueName)
	require.NotNil(t, second.Value)

	assert.NotEqual(t, *first.Value, *second.Value)
}

func TestMemStatsAreRead(t *testing.T) {
	c := NewCollector()
	c.Poll()

	m := findMetric(snapshot(c), "Sys")
	require.NotNil(t, m.Value)
	assert.Greater(t, *m.Value, float64(0))
}
