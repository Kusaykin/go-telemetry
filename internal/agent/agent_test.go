package agent

import (
	"bytes"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.Equal(t, "localhost:8080", cfg.Address)
	assert.Equal(t, 2*time.Second, cfg.PollInterval)
	assert.Equal(t, 10*time.Second, cfg.ReportInterval)
}

func paths(ts *testServer) []string {
	result := make([]string, 0, len(ts.requests))
	for _, r := range ts.requests {
		result = append(result, r.path)
	}

	return result
}

func TestAgentSendsReportEveryFifthPoll(t *testing.T) {
	ts, client := newTestServer(t, http.StatusOK)
	out := &bytes.Buffer{}

	a := New(DefaultConfig())
	a.client = client
	a.out = out

	for i := 0; i < 4; i++ {
		a.tick()
	}
	require.Empty(t, ts.requests)

	// пятый tick — уходит первый отчёт
	a.tick()
	require.Len(t, ts.requests, metricsCount)
	assert.Contains(t, paths(ts), "/update/counter/PollCount/5")

	// ещё пять tick — второй отчёт, PollCount накопительный
	for i := 0; i < 5; i++ {
		a.tick()
	}
	require.Len(t, ts.requests, 2*metricsCount)
	assert.Contains(t, paths(ts), "/update/counter/PollCount/10")
}

func TestAgentLogsReport(t *testing.T) {
	_, client := newTestServer(t, http.StatusOK)
	out := &bytes.Buffer{}

	a := New(DefaultConfig())
	a.client = client
	a.out = out

	for i := 0; i < 5; i++ {
		a.tick()
	}

	assert.Contains(t, out.String(), "--- отчёт, метрик: 29 ---")
	assert.Contains(t, out.String(), PollCountName)
	assert.Contains(t, out.String(), RandomValueName)
}

func TestAgentSurvivesServerErrors(t *testing.T) {
	ts, client := newTestServer(t, http.StatusInternalServerError)
	out := &bytes.Buffer{}

	a := New(DefaultConfig())
	a.client = client
	a.out = out

	for i := 0; i < 10; i++ {
		a.tick()
	}

	assert.Len(t, ts.requests, 2*metricsCount)
	assert.Equal(t, 2, strings.Count(out.String(), "--- отчёт"))
}
