package agent

import (
	"io"
	"math"
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
	method      string
	path        string
	contentType string
	body        string
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
		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)

		ts.mu.Lock()
		ts.requests = append(ts.requests, request{
			method:      r.Method,
			path:        r.URL.Path,
			contentType: r.Header.Get("Content-Type"),
			body:        string(body),
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

func TestSendRequestFormat(t *testing.T) {
	tests := []struct {
		name   string
		metric models.Metrics
		want   string
	}{
		{"gauge", gauge("Alloc", 12.5), `{"id":"Alloc","type":"gauge","value":12.5}`},
		{"counter", counter("PollCount", 527), `{"id":"PollCount","type":"counter","delta":527}`},
		{"большое целое gauge", gauge("Sys", 1234567890), `{"id":"Sys","type":"gauge","value":1234567890}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts, client := newTestServer(t, http.StatusOK)

			err := client.Send(tt.metric)
			require.NoError(t, err)

			requests := ts.Requests()
			require.Len(t, requests, 1)
			assert.Equal(t, http.MethodPost, requests[0].method)
			assert.Equal(t, "/update", requests[0].path)
			assert.Equal(t, "application/json", requests[0].contentType)
			assert.JSONEq(t, tt.want, requests[0].body)
		})
	}
}

func TestSendServerError(t *testing.T) {
	ts, client := newTestServer(t, http.StatusInternalServerError)

	err := client.Send(gauge("Alloc", 1))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
	assert.Len(t, ts.Requests(), 1)
}

func TestSendInvalidMetric(t *testing.T) {
	tests := []struct {
		name   string
		metric models.Metrics
	}{
		{"gauge без значения", models.Metrics{ID: "Alloc", MType: models.Gauge}},
		{"неизвестный тип", models.Metrics{ID: "Alloc", MType: "histogram"}},
		{"gauge NaN", gauge("Alloc", math.NaN())},
		{"gauge +Inf", gauge("Alloc", math.Inf(1))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts, client := newTestServer(t, http.StatusOK)

			err := client.Send(tt.metric)

			assert.Error(t, err)
			assert.Empty(t, ts.Requests())
		})
	}
}

func TestSendAll(t *testing.T) {
	ts, client := newTestServer(t, http.StatusOK)

	metrics := []models.Metrics{
		gauge("Alloc", 1),
		gauge("Sys", 2),
		counter("PollCount", 3),
	}

	err := client.SendAll(metrics)
	require.NoError(t, err)

	requests := ts.Requests()
	require.Len(t, requests, 3)
	assert.JSONEq(t, `{"id":"Alloc","type":"gauge","value":1}`, requests[0].body)
	assert.JSONEq(t, `{"id":"Sys","type":"gauge","value":2}`, requests[1].body)
	assert.JSONEq(t, `{"id":"PollCount","type":"counter","delta":3}`, requests[2].body)
}

func TestSendAllStopsOnFirstError(t *testing.T) {
	ts, client := newTestServer(t, http.StatusInternalServerError)

	metrics := []models.Metrics{
		gauge("Alloc", 1),
		gauge("Sys", 2),
		counter("PollCount", 3),
	}

	err := client.SendAll(metrics)
	require.Error(t, err)

	requests := ts.Requests()
	require.Len(t, requests, 1)
	assert.JSONEq(t, `{"id":"Alloc","type":"gauge","value":1}`, requests[0].body)
}
