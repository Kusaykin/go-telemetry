package agent

import (
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/Kusaykin/go-telemetry/internal/config"
	models "github.com/Kusaykin/go-telemetry/internal/model"
	"github.com/mailru/easyjson"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func sent(t *testing.T, ts *testServer) []string {
	requests := ts.Requests()
	result := make([]string, 0, len(requests))
	for _, r := range requests {
		result = append(result, metricKey(t, []byte(r.body)))
	}

	return result
}

func metricKey(t *testing.T, body []byte) string {
	var m models.Metrics
	require.NoError(t, easyjson.Unmarshal(body, &m))

	value, err := m.ValueString()
	require.NoError(t, err)

	return m.MType + "/" + m.ID + "/" + value
}

func countSent(t *testing.T, ts *testServer, want string) int {
	count := 0

	for _, p := range sent(t, ts) {
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
	require.Empty(t, ts.Requests())

	// пятый tick — уходит первый отчёт
	a.tick()
	require.Len(t, ts.Requests(), metricsCount)
	assert.Contains(t, sent(t, ts), "counter/PollCount/5")

	for i := 0; i < 5; i++ {
		a.tick()
	}
	require.Len(t, ts.Requests(), 2*metricsCount)
	assert.Equal(t, 2, countSent(t, ts, "counter/PollCount/5"))
	assert.NotContains(t, sent(t, ts), "counter/PollCount/10")
}

func TestPollCountAccumulatesToPollsOnServer(t *testing.T) {
	var total atomic.Int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		zr, err := gzip.NewReader(r.Body)
		if !assert.NoError(t, err) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		defer zr.Close()

		var m models.Metrics
		if !assert.NoError(t, easyjson.UnmarshalFromReader(zr, &m)) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if m.MType == models.Counter && m.ID == PollCountName {
			if !assert.NotNil(t, m.Delta) {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			total.Add(*m.Delta)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	a := New(config.DefaultAgent(), zap.NewNop())
	a.client = NewClient(srv.Listener.Addr().String())

	const polls = 20
	for range polls {
		a.tick()
	}

	assert.Equal(t, int64(polls), total.Load())
}

func TestPollCountSurvivesFailedReport(t *testing.T) {
	ts, client := newTestServer(t, http.StatusInternalServerError)

	a := New(config.DefaultAgent(), zap.NewNop())
	a.client = client

	for i := 0; i < 10; i++ {
		a.tick()
	}

	assert.NotContains(t, sent(t, ts), "counter/PollCount/5")
	assert.Equal(t, int64(10), a.collector.PollCountDelta())

	ts.SetStatus(http.StatusOK)
	for i := 0; i < 5; i++ {
		a.tick()
	}

	assert.Contains(t, sent(t, ts), "counter/PollCount/15")
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

	assert.Len(t, ts.Requests(), 2)
	assert.Equal(t, 2, logs.FilterMessage("report").Len())

	failures := logs.FilterMessage("report failed").All()
	require.Len(t, failures, 2)
	assert.Equal(t, zapcore.ErrorLevel, failures[0].Level)
}
