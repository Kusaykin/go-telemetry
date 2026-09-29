package agent

import (
	"net/http"
	"net/http/httptest"
	"path"
	"strconv"
	"strings"
	"testing"

	"github.com/Kusaykin/go-telemetry/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func paths(ts *testServer) []string {
	result := make([]string, 0, len(ts.requests))
	for _, r := range ts.requests {
		result = append(result, r.path)
	}

	return result
}

func countPath(ts *testServer, want string) int {
	count := 0

	for _, p := range paths(ts) {
		if p == want {
			count++
		}
	}

	return count
}

func TestAgentSendsReportEveryFifthPoll(t *testing.T) {
	ts, client := newTestServer(t, http.StatusOK)
	a := New(config.DefaultAgent(), zap.NewNop())
	a.client = client

	for i := 0; i < 4; i++ {
		a.tick()
	}
	require.Empty(t, ts.requests)

	// пятый tick — уходит первый отчёт
	a.tick()
	require.Len(t, ts.requests, metricsCount)
	assert.Contains(t, paths(ts), "/update/counter/PollCount/5")

	for i := 0; i < 5; i++ {
		a.tick()
	}
	require.Len(t, ts.requests, 2*metricsCount)
	assert.Equal(t, 2, countPath(ts, "/update/counter/PollCount/5"))
	assert.NotContains(t, paths(ts), "/update/counter/PollCount/10")
}

func TestPollCountAccumulatesToPollsOnServer(t *testing.T) {
	var total int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/update/counter/"+PollCountName+"/") {
			delta, err := strconv.ParseInt(path.Base(r.URL.Path), 10, 64)
			require.NoError(t, err)
			total += delta
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	a := New(config.DefaultAgent(), zap.NewNop())
	a.client = NewClient(srv.Listener.Addr().String())

	const polls = 20
	for range polls {
		a.tick()
	}

	assert.Equal(t, int64(polls), total)
}

func TestPollCountSurvivesFailedReport(t *testing.T) {
	ts, client := newTestServer(t, http.StatusInternalServerError)

	a := New(config.DefaultAgent(), zap.NewNop())
	a.client = client

	for i := 0; i < 10; i++ {
		a.tick()
	}

	assert.NotContains(t, paths(ts), "/update/counter/PollCount/5")
	assert.Equal(t, int64(10), a.collector.PollCountDelta())

	ts.status = http.StatusOK
	for i := 0; i < 5; i++ {
		a.tick()
	}

	assert.Contains(t, paths(ts), "/update/counter/PollCount/15")
	assert.Zero(t, a.collector.PollCountDelta())
}

func TestAgentLogsReport(t *testing.T) {
	_, client := newTestServer(t, http.StatusOK)
	core, logs := observer.New(zapcore.InfoLevel)

	a := New(config.DefaultAgent(), zap.New(core))
	a.client = client

	for i := 0; i < 5; i++ {
		a.tick()
	}

	reports := logs.FilterMessage("report").All()
	require.Len(t, reports, 1)
	assert.Equal(t, int64(metricsCount), reports[0].ContextMap()["metrics"])

	metrics := logs.FilterMessage("metric")
	assert.Equal(t, metricsCount, metrics.Len())
	assert.Equal(t, 1, metrics.FilterField(zap.String("id", PollCountName)).Len())
	assert.Equal(t, 1, metrics.FilterField(zap.String("id", RandomValueName)).Len())
	for _, e := range metrics.All() {
		assert.Equal(t, zapcore.InfoLevel, e.Level)
	}
}

func TestAgentSurvivesServerErrors(t *testing.T) {
	ts, client := newTestServer(t, http.StatusInternalServerError)
	core, logs := observer.New(zapcore.InfoLevel)

	a := New(config.DefaultAgent(), zap.New(core))
	a.client = client

	for i := 0; i < 10; i++ {
		a.tick()
	}

	assert.Len(t, ts.requests, 2)
	assert.Equal(t, 2, logs.FilterMessage("report").Len())

	failures := logs.FilterMessage("report failed").All()
	require.Len(t, failures, 2)
	assert.Equal(t, zapcore.ErrorLevel, failures[0].Level)
}
