package handler_test

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Kusaykin/go-telemetry/internal/compress"
	"github.com/Kusaykin/go-telemetry/internal/handler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestRouter(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		want   int
	}{
		{"метрика со значением", http.MethodPost, "/update/gauge/Alloc/12.5", http.StatusOK},
		{"пустое значение", http.MethodPost, "/update/gauge/Alloc/", http.StatusBadRequest},
		{"нет значения", http.MethodPost, "/update/gauge/Alloc", http.StatusNotFound},
		{"нет имени метрики", http.MethodPost, "/update/gauge", http.StatusNotFound},
		{"пустое имя метрики", http.MethodPost, "/update/gauge//12.5", http.StatusNotFound},
		{"пустой тип метрики", http.MethodPost, "/update//Alloc/12.5", http.StatusBadRequest},
		{"пустое имя в /value", http.MethodGet, "/value/gauge/", http.StatusNotFound},
		{"неизвестный путь", http.MethodPost, "/unknown", http.StatusNotFound},
		{"метод GET вместо POST", http.MethodGet, "/update/gauge/Alloc/12.5", http.StatusMethodNotAllowed},
		{"нет имени метрики в /value", http.MethodGet, "/value/gauge", http.StatusNotFound},
		{"метод POST вместо GET в /value", http.MethodPost, "/value/gauge/Alloc", http.StatusMethodNotAllowed},
		{"JSON /update без тела", http.MethodPost, "/update", http.StatusBadRequest},
		{"JSON /update со слешем", http.MethodPost, "/update/", http.StatusBadRequest},
		{"метод GET вместо POST в JSON /update", http.MethodGet, "/update", http.StatusMethodNotAllowed},
		{"JSON /value без тела", http.MethodPost, "/value", http.StatusBadRequest},
		{"JSON /value со слешем", http.MethodPost, "/value/", http.StatusBadRequest},
		{"метод GET вместо POST в JSON /value", http.MethodGet, "/value", http.StatusMethodNotAllowed},
		{"список метрик", http.MethodGet, "/", http.StatusOK},
		{"метод POST вместо GET в корне", http.MethodPost, "/", http.StatusMethodNotAllowed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(newFakeStorage(), tt.method, tt.path)

			assert.Equal(t, tt.want, rec.Code)
		})
	}
}

func gzipBody(t *testing.T, body string) []byte {
	t.Helper()

	gz, err := compress.Compress([]byte(body))
	require.NoError(t, err)

	return gz
}

func TestRouterGzip(t *testing.T) {
	t.Run("сжатый запрос и ответ JSON /update", func(t *testing.T) {
		store := newFakeStorage()
		req := httptest.NewRequest(http.MethodPost, "/update",
			bytes.NewReader(gzipBody(t, `{"id":"PollCount","type":"counter","delta":5}`)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Content-Encoding", "gzip")
		req.Header.Set("Accept-Encoding", "gzip")
		rec := httptest.NewRecorder()

		handler.NewRouter(store, zap.NewNop()).ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, int64(5), store.counters["PollCount"])
		assert.Equal(t, "gzip", rec.Header().Get("Content-Encoding"))

		zr, err := gzip.NewReader(rec.Body)
		require.NoError(t, err)
		body, err := io.ReadAll(zr)
		require.NoError(t, err)
		assert.JSONEq(t, `{"id":"PollCount","type":"counter","delta":5}`, string(body))
	})

	t.Run("сжатая страница /", func(t *testing.T) {
		store := newFakeStorage()
		store.gauges["Alloc"] = 12.5
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Accept-Encoding", "gzip")
		rec := httptest.NewRecorder()

		handler.NewRouter(store, zap.NewNop()).ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "gzip", rec.Header().Get("Content-Encoding"))

		zr, err := gzip.NewReader(rec.Body)
		require.NoError(t, err)
		body, err := io.ReadAll(zr)
		require.NoError(t, err)
		assert.Contains(t, string(body), "Alloc: 12.5")
	})

	t.Run("распакованное тело больше лимита — 413", func(t *testing.T) {
		payload := `{"id":"Alloc","type":"gauge","value":1,"pad":"` + strings.Repeat("x", 64<<10) + `"}`
		req := httptest.NewRequest(http.MethodPost, "/update", bytes.NewReader(gzipBody(t, payload)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Content-Encoding", "gzip")
		rec := httptest.NewRecorder()

		handler.NewRouter(newFakeStorage(), zap.NewNop()).ServeHTTP(rec, req)

		assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	})
}
