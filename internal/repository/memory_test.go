package repository

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUpdateGauge(t *testing.T) {
	m := NewMemStorage()

	m.UpdateGauge("Alloc", 1.5)
	assert.Equal(t, 1.5, m.gauges["Alloc"])

	m.UpdateGauge("Alloc", -2.25)
	assert.Equal(t, -2.25, m.gauges["Alloc"])
}

func TestUpdateCounter(t *testing.T) {
	m := NewMemStorage()

	m.UpdateCounter("PollCount", 5)
	assert.Equal(t, int64(5), m.counters["PollCount"])

	m.UpdateCounter("PollCount", 10)
	assert.Equal(t, int64(15), m.counters["PollCount"])
}

func TestGauge(t *testing.T) {
	m := NewMemStorage()
	m.UpdateGauge("Alloc", 12.5)

	value, ok := m.Gauge("Alloc")
	assert.True(t, ok)
	assert.Equal(t, 12.5, value)

	_, ok = m.Gauge("Unknown")
	assert.False(t, ok)
}

func TestCounter(t *testing.T) {
	m := NewMemStorage()
	m.UpdateCounter("PollCount", 5)
	m.UpdateCounter("PollCount", 10)

	delta, ok := m.Counter("PollCount")
	assert.True(t, ok)
	assert.Equal(t, int64(15), delta)

	_, ok = m.Counter("Unknown")
	assert.False(t, ok)
}

func TestZeroValueIsFound(t *testing.T) {
	m := NewMemStorage()
	m.UpdateGauge("Zero", 0)
	m.UpdateCounter("ZeroCount", 0)

	_, ok := m.Gauge("Zero")
	assert.True(t, ok)

	_, ok = m.Counter("ZeroCount")
	assert.True(t, ok)
}

func TestSnapshotsAreCopies(t *testing.T) {
	m := NewMemStorage()
	m.UpdateGauge("Alloc", 1)
	m.UpdateCounter("PollCount", 1)

	gauges := m.Gauges()
	assert.Equal(t, map[string]float64{"Alloc": 1}, gauges)
	gauges["Alloc"] = 100
	delete(gauges, "Alloc")

	counters := m.Counters()
	assert.Equal(t, map[string]int64{"PollCount": 1}, counters)
	counters["PollCount"] = 100

	value, ok := m.Gauge("Alloc")
	assert.True(t, ok)
	assert.Equal(t, float64(1), value)

	delta, ok := m.Counter("PollCount")
	assert.True(t, ok)
	assert.Equal(t, int64(1), delta)
}

func TestConcurrentAccess(t *testing.T) {
	m := NewMemStorage()

	var wg sync.WaitGroup

	for i := range 50 {
		wg.Add(3)

		go func() { defer wg.Done(); m.UpdateGauge("Alloc", float64(i)) }()
		go func() { defer wg.Done(); m.UpdateCounter("PollCount", 1) }()
		go func() {
			defer wg.Done()
			m.Gauge("Alloc")
			m.Counter("PollCount")
			m.Gauges()
			m.Counters()
		}()
	}

	wg.Wait()

	delta, ok := m.Counter("PollCount")
	assert.True(t, ok)
	assert.Equal(t, int64(50), delta)
}
