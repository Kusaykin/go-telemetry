package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"

	"github.com/Kusaykin/go-telemetry/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewServer(t *testing.T) {
	srv := newServer(config.Server{Address: "127.0.0.1:9090"})

	assert.Equal(t, "127.0.0.1:9090", srv.Addr)
	assert.NotNil(t, srv.Handler)
}

func startServer(t *testing.T) string {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	srv := newServer(config.DefaultServer())
	go srv.Serve(l)

	t.Cleanup(func() {
		assert.NoError(t, srv.Shutdown(context.Background()))
	})

	return "http://" + l.Addr().String()
}

func post(t *testing.T, url string) int {
	t.Helper()

	resp, err := http.Post(url, "text/plain", nil)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	return resp.StatusCode
}

func get(t *testing.T, url string) (int, string) {
	t.Helper()

	resp, err := http.Get(url)
	require.NoError(t, err)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	return resp.StatusCode, string(body)
}

func TestServerStoresMetrics(t *testing.T) {
	base := startServer(t)

	require.Equal(t, http.StatusOK, post(t, base+"/update/counter/PollCount/5"))
	require.Equal(t, http.StatusOK, post(t, base+"/update/counter/PollCount/10"))
	require.Equal(t, http.StatusOK, post(t, base+"/update/gauge/Alloc/12.5"))

	status, body := get(t, base+"/value/counter/PollCount")
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, "15", body)

	status, body = get(t, base+"/value/gauge/Alloc")
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, "12.5", body)
}

func TestServerServesIndex(t *testing.T) {
	base := startServer(t)

	require.Equal(t, http.StatusOK, post(t, base+"/update/gauge/Alloc/12.5"))

	status, body := get(t, base+"/")
	assert.Equal(t, http.StatusOK, status)
	assert.Contains(t, body, "Alloc")
}

func TestServerRejectsUnknownMetric(t *testing.T) {
	base := startServer(t)

	assert.Equal(t, http.StatusBadRequest, post(t, base+"/update/unknown/Alloc/12.5"))

	status, _ := get(t, base+"/value/gauge/Missing")
	assert.Equal(t, http.StatusNotFound, status)
}
