package handler_test

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Kusaykin/go-telemetry/internal/handler"
	models "github.com/Kusaykin/go-telemetry/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestUpdates(t *testing.T) {
	store := newFakeStorage()
	store.counters["PollCount"] = 1

	rec := doBody(store, http.MethodPost, "/updates/", `[
		{"id":"Alloc","type":"gauge","value":1.5},
		{"id":"Alloc","type":"gauge","value":2.5},
		{"id":"PollCount","type":"counter","delta":2},
		{"id":"PollCount","type":"counter","delta":3}
	]`)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	assert.JSONEq(t, `{}`, rec.Body.String())
	assert.Equal(t, 2.5, store.gauges["Alloc"])
	assert.Equal(t, int64(6), store.counters["PollCount"])
}

func TestUpdatesEmptyBatch(t *testing.T) {
	store := newFakeStorage()

	rec := doBody(store, http.MethodPost, "/updates/", `[]`)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, store.gauges)
	assert.Empty(t, store.counters)
}

func TestUpdatesErrors(t *testing.T) {
	tests := []struct {
		name string
		body string
		want int
	}{
		{"null", `null`, http.StatusBadRequest},
		{"битый JSON", `[{"id":`, http.StatusBadRequest},
		{"объект вместо массива", `{"id":"Alloc","type":"gauge","value":1}`, http.StatusBadRequest},
		{"gauge без значения", `[{"id":"Alloc","type":"gauge","value":1},{"id":"Sys","type":"gauge"}]`, http.StatusBadRequest},
		{"неизвестный тип", `[{"id":"Alloc","type":"histogram","value":1}]`, http.StatusBadRequest},
		{"пустой id", `[{"id":"Alloc","type":"gauge","value":1},{"id":"","type":"gauge","value":1}]`, http.StatusBadRequest},
		{"слишком длинный id", `[{"id":"Alloc","type":"gauge","value":1},{"id":"` + strings.Repeat("a", models.MaxIDLength+1) + `","type":"gauge","value":1}]`, http.StatusBadRequest},
		{"null-элемент", `[null]`, http.StatusBadRequest},
		{"null после валидной метрики", `[{"id":"Alloc","type":"gauge","value":1}, null]`, http.StatusBadRequest},
		{"мусор после массива", `[{"id":"Alloc","type":"gauge","value":1}] x`, http.StatusBadRequest},
		{"слишком большое тело", `[` + strings.Repeat(" ", 1<<20) + `]`, http.StatusRequestEntityTooLarge},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newFakeStorage()

			rec := doBody(store, http.MethodPost, "/updates/", tt.body)

			assert.Equal(t, tt.want, rec.Code)
			// батч невалиден — ничего не записано, даже валидные метрики
			assert.Empty(t, store.gauges)
			assert.Empty(t, store.counters)
		})
	}
}

func TestUpdatesLogsRejectReason(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		want   map[string]any
		reason string
	}{
		{
			name:   "невалидная метрика",
			body:   `[{"id":"Alloc","type":"gauge","value":1},{"id":"Sys","type":"gauge"}]`,
			want:   map[string]any{"status": int64(http.StatusBadRequest), "index": int64(1), "id": "Sys", "type": "gauge"},
			reason: "value is not set",
		},
		{
			name:   "пустой id",
			body:   `[{"id":"","type":"counter","delta":1}]`,
			want:   map[string]any{"status": int64(http.StatusBadRequest), "index": int64(0), "id": ""},
			reason: "metric id is empty",
		},
		{
			name:   "null-элемент",
			body:   `[{"id":"Alloc","type":"gauge","value":1}, null]`,
			want:   map[string]any{"status": int64(http.StatusBadRequest), "index": int64(1), "id": ""},
			reason: "metric id is empty",
		},
		{
			name:   "битый JSON",
			body:   `[{"id":`,
			want:   map[string]any{"status": int64(http.StatusBadRequest)},
			reason: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core, logs := observer.New(zapcore.WarnLevel)
			req := httptest.NewRequest(http.MethodPost, "/updates/", strings.NewReader(tt.body))
			rec := httptest.NewRecorder()

			handler.NewRouter(newFakeStorage(), nil, zap.New(core)).ServeHTTP(rec, req)

			entries := logs.FilterMessage("updates: batch rejected").All()
			require.Len(t, entries, 1)

			fields := entries[0].ContextMap()
			for k, v := range tt.want {
				assert.Equal(t, v, fields[k], k)
			}
			assert.Contains(t, fields["error"], tt.reason)
		})
	}
}

func TestUpdatesStorageError(t *testing.T) {
	store := newFakeStorage()
	store.err = errors.New("db is down")

	rec := doBody(store, http.MethodPost, "/updates/", `[{"id":"Alloc","type":"gauge","value":1}]`)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestUpdatesGzip(t *testing.T) {
	store := newFakeStorage()

	req := httptest.NewRequest(http.MethodPost, "/updates/",
		bytes.NewReader(gzipBody(t, `[{"id":"PollCount","type":"counter","delta":5}]`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")
	rec := httptest.NewRecorder()

	handler.NewRouter(store, nil, zap.NewNop()).ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, int64(5), store.counters["PollCount"])
}
