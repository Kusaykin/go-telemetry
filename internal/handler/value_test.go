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

func TestValueJSON(t *testing.T) {
	store := newFakeStorage()
	store.UpdateGauge("LastGC", 1744184459)
	store.UpdateCounter("PollCount", 15)

	tests := []struct {
		name string
		body string
		want string
	}{
		{"gauge", `{"id":"LastGC","type":"gauge"}`, `{"id":"LastGC","type":"gauge","value":1744184459}`},
		{"counter", `{"id":"PollCount","type":"counter"}`, `{"id":"PollCount","type":"counter","delta":15}`},
		{"лишнее значение в запросе игнорируется", `{"id":"PollCount","type":"counter","value":1}`, `{"id":"PollCount","type":"counter","delta":15}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := doBody(store, http.MethodPost, "/value", tt.body)

			require.Equal(t, http.StatusOK, rec.Code)
			assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
			assert.JSONEq(t, tt.want, rec.Body.String())
		})
	}
}

func TestValueJSONErrors(t *testing.T) {
	store := newFakeStorage()
	store.UpdateGauge("Alloc", 1)

	tests := []struct {
		name string
		body string
		want int
	}{
		{"битый JSON", `{"id":`, http.StatusBadRequest},
		{"пустое тело", ``, http.StatusBadRequest},
		{"тело null", `null`, http.StatusBadRequest},
		{"нет такой метрики", `{"id":"Unknown","type":"gauge"}`, http.StatusNotFound},
		{"метрика другого типа", `{"id":"Alloc","type":"counter"}`, http.StatusNotFound},
		{"неизвестный тип", `{"id":"Alloc","type":"histogram"}`, http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := doBody(store, http.MethodPost, "/value", tt.body)

			assert.Equal(t, tt.want, rec.Code)
		})
	}
}
