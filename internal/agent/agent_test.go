package agent

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path"
	"strconv"
	"strings"
	"testing"

	"github.com/Kusaykin/go-telemetry/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	out := &bytes.Buffer{}

	a := New(config.DefaultAgent())
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

	a := New(config.DefaultAgent())
	a.client = NewClient(srv.Listener.Addr().String())
	a.out = &bytes.Buffer{}

	const polls = 20
	for range polls {
		a.tick()
	}

	assert.Equal(t, int64(polls), total)
}

func TestPollCountSurvivesFailedReport(t *testing.T) {
	ts, client := newTestServer(t, http.StatusInternalServerError)

	a := New(config.DefaultAgent())
	a.client = client
	a.out = &bytes.Buffer{}

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
	out := &bytes.Buffer{}

	a := New(config.DefaultAgent())
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

	a := New(config.DefaultAgent())
	a.client = client
	a.out = out

	for i := 0; i < 10; i++ {
		a.tick()
	}

	assert.Len(t, ts.requests, 2)
	assert.Equal(t, 2, strings.Count(out.String(), "--- отчёт"))
}
