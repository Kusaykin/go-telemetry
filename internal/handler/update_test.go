package handler_test

import (
	"maps"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Kusaykin/go-telemetry/internal/handler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeStorage struct {
	gauges   map[string]float64
	counters map[string]int64
}

func newFakeStorage() *fakeStorage {
	return &fakeStorage{
		gauges:   make(map[string]float64),
		counters: make(map[string]int64),
	}
}

func (f *fakeStorage) UpdateGauge(name string, value float64) {
	f.gauges[name] = value
}

func (f *fakeStorage) UpdateCounter(name string, delta int64) {
	f.counters[name] += delta
}

func (f *fakeStorage) Gauge(name string) (float64, bool) {
	value, ok := f.gauges[name]

	return value, ok
}

func (f *fakeStorage) Counter(name string) (int64, bool) {
	delta, ok := f.counters[name]

	return delta, ok
}

func (f *fakeStorage) Gauges() map[string]float64 {
	return maps.Clone(f.gauges)
}

func (f *fakeStorage) Counters() map[string]int64 {
	return maps.Clone(f.counters)
}

func do(store handler.Storage, method, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	rec := httptest.NewRecorder()
	handler.NewRouter(store).ServeHTTP(rec, req)

	return rec
}

func TestUpdateGauge(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  float64
	}{
		{"целое", "527", 527},
		{"дробное", "12.5", 12.5},
		{"отрицательное", "-0.25", -0.25},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newFakeStorage()

			rec := do(store, http.MethodPost, "/update/gauge/Alloc/"+tt.value)

			require.Equal(t, http.StatusOK, rec.Code)
			assert.Equal(t, "text/plain; charset=utf-8", rec.Header().Get("Content-Type"))
			assert.Equal(t, tt.want, store.gauges["Alloc"])
		})
	}
}

func TestUpdateCounter(t *testing.T) {
	store := newFakeStorage()

	rec := do(store, http.MethodPost, "/update/counter/PollCount/5")
	require.Equal(t, http.StatusOK, rec.Code)

	rec = do(store, http.MethodPost, "/update/counter/PollCount/10")
	require.Equal(t, http.StatusOK, rec.Code)

	assert.Equal(t, int64(15), store.counters["PollCount"])
}

func TestUpdateEmptyName(t *testing.T) {
	store := newFakeStorage()

	rec := do(store, http.MethodPost, "/update/gauge//12.5")

	require.Equal(t, http.StatusNotFound, rec.Code)
	assert.Empty(t, store.gauges)
	assert.Empty(t, store.counters)
}

func TestUpdateBadRequest(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{"нечисловое значение gauge", "/update/gauge/Alloc/none"},
		{"дробное значение counter", "/update/counter/PollCount/12.5"},
		{"неизвестный тип метрики", "/update/histogram/Alloc/1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newFakeStorage()

			rec := do(store, http.MethodPost, tt.path)

			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Empty(t, store.gauges)
			assert.Empty(t, store.counters)
		})
	}
}
