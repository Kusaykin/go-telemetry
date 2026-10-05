package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Kusaykin/go-telemetry/internal/config"
	"github.com/Kusaykin/go-telemetry/internal/handler"
	"github.com/Kusaykin/go-telemetry/internal/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewServer(t *testing.T) {
	srv := newServer(config.Server{Address: "127.0.0.1:9090"}, repository.NewMemStorage(), zap.NewNop())

	assert.Equal(t, "127.0.0.1:9090", srv.Addr)
	assert.NotNil(t, srv.Handler)
}

func startServer(t *testing.T, store handler.Storage) string {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	srv := newServer(config.DefaultServer(), store, zap.NewNop())
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
	base := startServer(t, repository.NewMemStorage())

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
	base := startServer(t, repository.NewMemStorage())

	require.Equal(t, http.StatusOK, post(t, base+"/update/gauge/Alloc/12.5"))

	status, body := get(t, base+"/")
	assert.Equal(t, http.StatusOK, status)
	assert.Contains(t, body, "Alloc")
}

func TestServerRejectsUnknownMetric(t *testing.T) {
	base := startServer(t, repository.NewMemStorage())

	assert.Equal(t, http.StatusBadRequest, post(t, base+"/update/unknown/Alloc/12.5"))

	status, _ := get(t, base+"/value/gauge/Missing")
	assert.Equal(t, http.StatusNotFound, status)
}

func TestServerRestoresSavedMetrics(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metrics.json")

	saving, err := repository.NewFileStorage(path, 0, false, zap.NewNop())
	require.NoError(t, err)

	base := startServer(t, saving)
	require.Equal(t, http.StatusOK, post(t, base+"/update/counter/PollCount/5"))
	require.Equal(t, http.StatusOK, post(t, base+"/update/gauge/Alloc/12.5"))

	restored, err := repository.NewFileStorage(path, time.Hour, true, zap.NewNop())
	require.NoError(t, err)

	base = startServer(t, restored)

	status, body := get(t, base+"/value/counter/PollCount")
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, "5", body)

	status, body = get(t, base+"/value/gauge/Alloc")
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, "12.5", body)

	require.Equal(t, http.StatusOK, post(t, base+"/update/counter/PollCount/10"))
	_, body = get(t, base+"/value/counter/PollCount")
	assert.Equal(t, "15", body, "восстановленный counter продолжает накапливаться")
}

func TestRunFailsOnCorruptedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metrics.json")
	require.NoError(t, os.WriteFile(path, []byte(`[{"id":`), 0o644))

	err := run(context.Background(), config.Server{Address: "127.0.0.1:0", FileStoragePath: path, Restore: true}, zap.NewNop())
	assert.Error(t, err)
}

func TestRunFailsOnBusyAddress(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { l.Close() })

	cfg := config.Server{Address: l.Addr().String(), StoreInterval: time.Hour}

	err = run(context.Background(), cfg, zap.NewNop())
	assert.Error(t, err)
}

func TestRunSavesOnShutdown(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metrics.json")
	cfg := config.Server{Address: "127.0.0.1:0", StoreInterval: time.Hour, FileStoragePath: path}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- run(ctx, cfg, zap.NewNop()) }()

	cancel()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("run не завершился после отмены контекста")
	}

	data, err := os.ReadFile(path)
	require.NoError(t, err, "при завершении метрики должны сохраняться в файл")
	assert.JSONEq(t, "[]", string(data))
}
