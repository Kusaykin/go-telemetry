package agent

import (
	"compress/gzip"
	"context"
	"io"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	models "github.com/Kusaykin/go-telemetry/internal/model"
	"github.com/Kusaykin/go-telemetry/internal/retry"
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

	err := client.SendBatch(t.Context(), metrics)
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

	require.NoError(t, client.SendBatch(t.Context(), nil))
	require.NoError(t, client.SendBatch(t.Context(), []models.Metrics{}))
	assert.Empty(t, ts.Requests())
}

func TestSendBatchServerError(t *testing.T) {
	ts, client := newTestServer(t, http.StatusInternalServerError)

	err := client.SendBatch(t.Context(), []models.Metrics{gauge("Alloc", 1)})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
	assert.Len(t, ts.Requests(), 1)
}

func refuseDials(c *Client, n int32) *atomic.Int32 {
	var (
		dials  atomic.Int32
		dialer net.Dialer
	)

	c.rest.SetTransport(&http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			if dials.Add(1) <= n {
				return nil, &net.OpError{Op: "dial", Net: network, Err: syscall.ECONNREFUSED}
			}
			return dialer.DialContext(ctx, network, addr)
		},
	})

	return &dials
}

func TestSendBatchGivesUpAfterRetries(t *testing.T) {
	client := NewClient("127.0.0.1:1")
	client.delays = []time.Duration{20 * time.Millisecond, 20 * time.Millisecond, 20 * time.Millisecond}
	refuseDials(client, math.MaxInt32)

	start := time.Now()
	err := client.SendBatch(t.Context(), []models.Metrics{gauge("Alloc", 1)})

	require.Error(t, err)
	assert.True(t, isConnError(err))
	assert.GreaterOrEqual(t, time.Since(start), 60*time.Millisecond)
}

func TestSendBatchRetriesUntilServerIsUp(t *testing.T) {
	received := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- struct{}{}
	}))
	t.Cleanup(srv.Close)

	client := NewClient(srv.Listener.Addr().String())
	client.delays = []time.Duration{20 * time.Millisecond, 20 * time.Millisecond, 20 * time.Millisecond}
	dials := refuseDials(client, 2)

	err := client.SendBatch(t.Context(), []models.Metrics{gauge("Alloc", 1)})

	require.NoError(t, err)
	assert.Equal(t, int32(3), dials.Load(), "две неудачные попытки и одна успешная")
	assert.Len(t, received, 1)
}

func TestSendBatchStopsRetryingOnCancel(t *testing.T) {
	client := NewClient("127.0.0.1:1")
	refuseDials(client, math.MaxInt32)

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := client.SendBatch(ctx, []models.Metrics{gauge("Alloc", 1)})

	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, time.Since(start), retry.Delays()[0])
}
