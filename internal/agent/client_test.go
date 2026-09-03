package agent

import (
	"net/http"
	"net/http/httptest"
	"testing"

	models "github.com/Kusaykin/go-telemetry/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type request struct {
	method      string
	path        string
	contentType string
}

type testServer struct {
	requests []request
	status   int
}

func newTestServer(t *testing.T, status int) (*testServer, *Client) {
	ts := &testServer{status: status}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ts.requests = append(ts.requests, request{
			method:      r.Method,
			path:        r.URL.Path,
			contentType: r.Header.Get("Content-Type"),
		})
		w.WriteHeader(ts.status)
	}))
	t.Cleanup(srv.Close)

	return ts, NewClient(srv.Listener.Addr().String())
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
		{"gauge", gauge("Alloc", 12.5), "/update/gauge/Alloc/12.5"},
		{"counter", counter("PollCount", 527), "/update/counter/PollCount/527"},
		{"большое целое gauge", gauge("Sys", 1234567890), "/update/gauge/Sys/1234567890"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts, client := newTestServer(t, http.StatusOK)

			err := client.Send(tt.metric)
			require.NoError(t, err)

			require.Len(t, ts.requests, 1)
			assert.Equal(t, http.MethodPost, ts.requests[0].method)
			assert.Equal(t, tt.want, ts.requests[0].path)
			assert.Equal(t, "text/plain", ts.requests[0].contentType)
		})
	}
}

func TestSendServerError(t *testing.T) {
	ts, client := newTestServer(t, http.StatusInternalServerError)

	err := client.Send(gauge("Alloc", 1))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
	assert.Len(t, ts.requests, 1)
}

func TestSendInvalidMetric(t *testing.T) {
	tests := []struct {
		name   string
		metric models.Metrics
	}{
		{"gauge без значения", models.Metrics{ID: "Alloc", MType: models.Gauge}},
		{"неизвестный тип", models.Metrics{ID: "Alloc", MType: "histogram"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts, client := newTestServer(t, http.StatusOK)

			err := client.Send(tt.metric)

			assert.Error(t, err)
			assert.Empty(t, ts.requests)
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

	require.Len(t, ts.requests, 3)
	assert.Equal(t, "/update/gauge/Alloc/1", ts.requests[0].path)
	assert.Equal(t, "/update/gauge/Sys/2", ts.requests[1].path)
	assert.Equal(t, "/update/counter/PollCount/3", ts.requests[2].path)
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

	require.Len(t, ts.requests, 1)
	assert.Equal(t, "/update/gauge/Alloc/1", ts.requests[0].path)
}
