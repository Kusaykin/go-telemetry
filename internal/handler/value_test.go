package handler_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValueGauge(t *testing.T) {
	tests := []struct {
		name  string
		value float64
		want  string
	}{
		{"целое", 527, "527"},
		{"дробное", 12.5, "12.5"},
		{"отрицательное", -0.25, "-0.25"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newFakeStorage()
			store.UpdateGauge("Alloc", tt.value)

			rec := do(store, http.MethodGet, "/value/gauge/Alloc")

			require.Equal(t, http.StatusOK, rec.Code)
			assert.Equal(t, "text/plain; charset=utf-8", rec.Header().Get("Content-Type"))
			assert.Equal(t, tt.want, rec.Body.String())
		})
	}
}

func TestValueCounterIsAccumulated(t *testing.T) {
	store := newFakeStorage()
	store.UpdateCounter("PollCount", 5)
	store.UpdateCounter("PollCount", 10)

	rec := do(store, http.MethodGet, "/value/counter/PollCount")

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "15", rec.Body.String())
}

func TestValueZeroIsFound(t *testing.T) {
	store := newFakeStorage()
	store.UpdateGauge("Zero", 0)
	store.UpdateCounter("ZeroCount", 0)

	rec := do(store, http.MethodGet, "/value/gauge/Zero")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "0", rec.Body.String())

	rec = do(store, http.MethodGet, "/value/counter/ZeroCount")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "0", rec.Body.String())
}

func TestValueNotFound(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{"неизвестное имя gauge", "/value/gauge/Unknown"},
		{"неизвестное имя counter", "/value/counter/Unknown"},
		{"неизвестный тип метрики", "/value/histogram/Alloc"},
		{"counter запрошен как gauge", "/value/gauge/PollCount"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newFakeStorage()
			store.UpdateGauge("Alloc", 1)
			store.UpdateCounter("PollCount", 1)

			rec := do(store, http.MethodGet, tt.path)

			assert.Equal(t, http.StatusNotFound, rec.Code)
		})
	}
}

func TestUpdateThenValue(t *testing.T) {
	store := newFakeStorage()

	rec := do(store, http.MethodPost, "/update/gauge/Alloc/12.5")
	require.Equal(t, http.StatusOK, rec.Code)

	rec = do(store, http.MethodGet, "/value/gauge/Alloc")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "12.5", rec.Body.String())
}
