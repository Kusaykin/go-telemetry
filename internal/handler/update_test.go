package handler_test

import (
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Kusaykin/go-telemetry/internal/handler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
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

func (f *fakeStorage) UpdateCounter(name string, delta int64) int64 {
	f.counters[name] += delta

	return f.counters[name]
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
	return doBody(store, method, path, "")
}

func doBody(store handler.Storage, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	handler.NewRouter(store, zap.NewNop()).ServeHTTP(rec, req)

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
		{"gauge NaN", "/update/gauge/Alloc/NaN"},
		{"gauge +Inf", "/update/gauge/Alloc/+Inf"},
		{"gauge -Inf", "/update/gauge/Alloc/-Inf"},
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

func TestUpdateJSONGauge(t *testing.T) {
	store := newFakeStorage()

	rec := doBody(store, http.MethodPost, "/update", `{"id":"LastGC","type":"gauge","value":1744184459}`)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	assert.JSONEq(t, `{"id":"LastGC","type":"gauge","value":1744184459}`, rec.Body.String())
	assert.Equal(t, float64(1744184459), store.gauges["LastGC"])
}

func TestUpdateJSONCounter(t *testing.T) {
	store := newFakeStorage()

	rec := doBody(store, http.MethodPost, "/update", `{"id":"PollCount","type":"counter","delta":5}`)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"id":"PollCount","type":"counter","delta":5}`, rec.Body.String())

	rec = doBody(store, http.MethodPost, "/update", `{"id":"PollCount","type":"counter","delta":10}`)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	assert.JSONEq(t, `{"id":"PollCount","type":"counter","delta":15}`, rec.Body.String())

	assert.Equal(t, int64(15), store.counters["PollCount"])
}

func TestUpdateJSONErrors(t *testing.T) {
	tests := []struct {
		name string
		body string
		want int
	}{
		{"битый JSON", `{"id":`, http.StatusBadRequest},
		{"пустое тело", ``, http.StatusBadRequest},
		{"тело null", `null`, http.StatusBadRequest},
		{"слишком большое тело", `{"id":"` + strings.Repeat("a", 5<<10) + `","type":"gauge","value":1}`, http.StatusRequestEntityTooLarge},
		{"неизвестный тип", `{"id":"Alloc","type":"histogram","value":1}`, http.StatusBadRequest},
		{"gauge без value", `{"id":"Alloc","type":"gauge","delta":1}`, http.StatusBadRequest},
		{"counter без delta", `{"id":"PollCount","type":"counter","value":1}`, http.StatusBadRequest},
		{"пустой id", `{"id":"","type":"gauge","value":1}`, http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newFakeStorage()

			rec := doBody(store, http.MethodPost, "/update", tt.body)

			assert.Equal(t, tt.want, rec.Code)
			assert.Empty(t, store.gauges)
			assert.Empty(t, store.counters)
		})
	}
}
