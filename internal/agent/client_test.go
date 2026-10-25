package agent

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"

	models "github.com/Kusaykin/go-telemetry/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type request struct {
	method          string
	path            string
	contentType     string
	contentEncoding string
	body            string
}

// testServer пишется из горутины HTTP-сервера, а читается из теста,
// поэтому доступ к полям идёт только под mu.
type testServer struct {
	mu       sync.Mutex
	requests []request
	status   int
}

func newTestServer(t *testing.T, status int) (*testServer, *Client) {
	ts := &testServer{status: status}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reader io.Reader = r.Body
		if r.Header.Get("Content-Encoding") == "gzip" {
			zr, err := gzip.NewReader(r.Body)
			if !assert.NoError(t, err) {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			defer zr.Close()
			reader = zr
		}

		body, err := io.ReadAll(reader)
		assert.NoError(t, err)

		ts.mu.Lock()
		ts.requests = append(ts.requests, request{
			method:          r.Method,
			path:            r.URL.Path,
			contentType:     r.Header.Get("Content-Type"),
			contentEncoding: r.Header.Get("Content-Encoding"),
			body:            string(body),
		})
		status := ts.status
		ts.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	return ts, NewClient(srv.Listener.Addr().String())
}

// Requests возвращает копию полученных сервером запросов.
func (ts *testServer) Requests() []request {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	return slices.Clone(ts.requests)
}

func (ts *testServer) SetStatus(status int) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	ts.status = status
}

func gauge(id string, value float64) models.Metrics {
	return models.Metrics{ID: id, MType: models.Gauge, Value: &value}
}

func counter(id string, delta int64) models.Metrics {
	return models.Metrics{ID: id, MType: models.Counter, Delta: &delta}
}

func TestSendBatch(t *testing.T) {
	ts, client := newTestServer(t, http.StatusOK)

	metrics := []models.Metrics{
		gauge("Alloc", 1),
		gauge("Sys", 2),
		gauge("TotalAlloc", 1234567890),
		counter("PollCount", 3),
	}

	err := client.SendBatch(metrics)
	require.NoError(t, err)

	requests := ts.Requests()
	require.Len(t, requests, 1)
	assert.Equal(t, http.MethodPost, requests[0].method)
	assert.Equal(t, "/updates/", requests[0].path)
	assert.Equal(t, "application/json", requests[0].contentType)
	assert.Equal(t, "gzip", requests[0].contentEncoding)
	assert.JSONEq(t, `[
		{"id":"Alloc","type":"gauge","value":1},
		{"id":"Sys","type":"gauge","value":2},
		{"id":"TotalAlloc","type":"gauge","value":1234567890},
		{"id":"PollCount","type":"counter","delta":3}
	]`, requests[0].body)
}

func TestSendBatchEmpty(t *testing.T) {
	ts, client := newTestServer(t, http.StatusOK)

	require.NoError(t, client.SendBatch(nil))
	require.NoError(t, client.SendBatch([]models.Metrics{}))
	assert.Empty(t, ts.Requests())
}

func TestSendBatchServerError(t *testing.T) {
	ts, client := newTestServer(t, http.StatusInternalServerError)

	err := client.SendBatch([]models.Metrics{gauge("Alloc", 1)})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
	assert.Len(t, ts.Requests(), 1)
}
