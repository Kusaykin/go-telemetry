package models

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddDelta(t *testing.T) {
	tests := []struct {
		name         string
		value, delta int64
		want         int64
		overflow     bool
	}{
		{"обычная сумма", 5, 10, 15, false},
		{"отрицательная дельта", 5, -10, -5, false},
		{"ровно до максимума", math.MaxInt64 - 1, 1, math.MaxInt64, false},
		{"переполнение вверх", math.MaxInt64, 1, 0, true},
		{"переполнение вниз", math.MinInt64, -1, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := AddDelta(tt.value, tt.delta)

			if tt.overflow {
				require.ErrorIs(t, err, ErrCounterOverflow)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCollapse(t *testing.T) {
	g := func(id string, v float64) Metrics { return Metrics{ID: id, MType: Gauge, Value: &v} }
	c := func(id string, d int64) Metrics { return Metrics{ID: id, MType: Counter, Delta: &d} }

	gauges, counters, err := Collapse([]Metrics{
		g("Alloc", 1), c("PollCount", 2), g("Alloc", 3), c("PollCount", 5),
	})

	require.NoError(t, err)
	assert.Equal(t, map[string]float64{"Alloc": 3}, gauges)
	assert.Equal(t, map[string]int64{"PollCount": 7}, counters)

	_, _, err = Collapse([]Metrics{c("PollCount", math.MaxInt64), c("PollCount", 1)})
	require.ErrorIs(t, err, ErrCounterOverflow)
	assert.Contains(t, err.Error(), "PollCount")
}

func TestCollapseInvalid(t *testing.T) {
	g := func(id string, v float64) Metrics { return Metrics{ID: id, MType: Gauge, Value: &v} }

	tests := []struct {
		name   string
		metric Metrics
	}{
		{"gauge без значения", Metrics{ID: "Alloc", MType: Gauge}},
		{"counter без delta", Metrics{ID: "PollCount", MType: Counter}},
		{"gauge NaN", g("Alloc", math.NaN())},
		{"неизвестный тип", Metrics{ID: "Hist", MType: "histogram"}},
		{"пустой id", g("", 1)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				gauges   map[string]float64
				counters map[string]int64
				err      error
			)
			require.NotPanics(t, func() {
				gauges, counters, err = Collapse([]Metrics{g("Sys", 1), tt.metric})
			})

			var invalid *InvalidMetricError
			require.ErrorAs(t, err, &invalid)
			assert.Equal(t, 1, invalid.Index)
			assert.Equal(t, tt.metric.ID, invalid.Metric.ID)
			assert.Contains(t, err.Error(), "metric 1")
			assert.Nil(t, gauges)
			assert.Nil(t, counters)
		})
	}
}
