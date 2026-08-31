package repository

import (
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
