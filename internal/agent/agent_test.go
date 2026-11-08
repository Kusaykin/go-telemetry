package agent

import (
	"compress/gzip"
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Kusaykin/go-telemetry/internal/config"
	models "github.com/Kusaykin/go-telemetry/internal/model"
	"github.com/mailru/easyjson"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// tick выполняет тот же шаг, что и Run, и дожидается фоновой отправки.
func (a *Agent) tick(ctx context.Context) {
	sending := make(chan struct{}, 1)
	var wg sync.WaitGroup

	a.step(ctx, sending, &wg)
	wg.Wait()
}

func sent(t *testing.T, ts *testServer) []string {
	var result []string
	for _, r := range ts.Requests() {
		var batch models.MetricsList
		require.NoError(t, easyjson.Unmarshal([]byte(r.body), &batch))

		for _, m := range batch {
			result = append(result, metricKey(t, m))
		}
	}

	return result
}

func metricKey(t *testing.T, m models.Metrics) string {
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
		a.tick(t.Context())
	}
	require.Empty(t, ts.Requests())

	// пятый tick — уходит первый отчёт одним батчем
	a.tick(t.Context())
	require.Len(t, ts.Requests(), 1)
	assert.Equal(t, "/updates/", ts.Requests()[0].path)
	require.Len(t, sent(t, ts), metricsCount)
	assert.Contains(t, sent(t, ts), "counter/PollCount/5")

	for i := 0; i < 5; i++ {
		a.tick(t.Context())
	}
	require.Len(t, ts.Requests(), 2)
	require.Len(t, sent(t, ts), 2*metricsCount)
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

		var batch models.MetricsList
		if !assert.NoError(t, easyjson.UnmarshalFromReader(zr, &batch)) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		for _, m := range batch {
			if m.MType == models.Counter && m.ID == PollCountName {
				if !assert.NotNil(t, m.Delta) {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				total.Add(*m.Delta)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	a := New(config.DefaultAgent(), zap.NewNop())
	a.client = NewClient(srv.Listener.Addr().String())

	const polls = 20
	for range polls {
		a.tick(t.Context())
	}

	assert.Equal(t, int64(polls), total.Load())
}

func TestPollCountSurvivesFailedReport(t *testing.T) {
	ts, client := newTestServer(t, http.StatusInternalServerError)

	a := New(config.DefaultAgent(), zap.NewNop())
	a.client = client

	for i := 0; i < 10; i++ {
		a.tick(t.Context())
	}

	assert.Contains(t, sent(t, ts), "counter/PollCount/5")
	assert.Contains(t, sent(t, ts), "counter/PollCount/10")
	assert.Equal(t, int64(10), a.collector.PollCountDelta())

	ts.SetStatus(http.StatusOK)
	for i := 0; i < 5; i++ {
		a.tick(t.Context())
	}

	assert.Contains(t, sent(t, ts), "counter/PollCount/15")
	assert.Zero(t, a.collector.PollCountDelta())
}

func TestAgentLogsReport(t *testing.T) {
	_, client := newTestServer(t, http.StatusOK)
	core, logs := observer.New(zapcore.DebugLevel)

	a := New(config.DefaultAgent(), zap.New(core))
	a.client = client

	for i := 0; i < 5; i++ {
		a.tick(t.Context())
	}

	reports := logs.FilterMessage("report").All()
	require.Len(t, reports, 1)
	assert.Equal(t, int64(metricsCount), reports[0].ContextMap()["metrics"])
	assert.Equal(t, zapcore.InfoLevel, reports[0].Level)

	metrics := logs.FilterMessage("metric")
	assert.Equal(t, metricsCount, metrics.Len())
	assert.Equal(t, 1, metrics.FilterField(zap.String("id", PollCountName)).Len())
	assert.Equal(t, 1, metrics.FilterField(zap.String("id", RandomValueName)).Len())
	for _, e := range metrics.All() {
		assert.Equal(t, zapcore.DebugLevel, e.Level)
	}
}

func TestAgentLogsSkippedMetrics(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)

	a := New(config.DefaultAgent(), zap.New(core))

	valid := a.filterReport([]models.Metrics{
		gauge("Alloc", 1),
		gauge("HeapAlloc", math.NaN()),
		gauge("", 2),
		{ID: "Sys", MType: models.Gauge},
		{ID: "Hist", MType: "histogram"},
		counter("PollCount", 3),
	})

	assert.Equal(t, []models.Metrics{gauge("Alloc", 1), counter("PollCount", 3)}, valid)

	skipped := logs.FilterMessage("metric skipped").All()
	require.Len(t, skipped, 4)
	for _, e := range skipped {
		assert.Equal(t, zapcore.ErrorLevel, e.Level)
	}
	assert.Equal(t, 2, logs.FilterMessage("metric").Len())
}

func TestAgentSurvivesServerErrors(t *testing.T) {
	ts, client := newTestServer(t, http.StatusInternalServerError)
	core, logs := observer.New(zapcore.InfoLevel)

	a := New(config.DefaultAgent(), zap.New(core))
	a.client = client

	for i := 0; i < 10; i++ {
		a.tick(t.Context())
	}

	assert.Len(t, ts.Requests(), 2)
	assert.Equal(t, 2, logs.FilterMessage("report").Len())

	failures := logs.FilterMessage("report failed").All()
	require.Len(t, failures, 2)
	assert.Equal(t, zapcore.ErrorLevel, failures[0].Level)
}

func TestAgentKeepsPollingWhileReportRetries(t *testing.T) {
	cfg := config.DefaultAgent()
	cfg.PollInterval = 10 * time.Millisecond
	cfg.ReportInterval = 10 * time.Millisecond

	core, logs := observer.New(zapcore.InfoLevel)
	a := New(cfg, zap.New(core))
	a.client = NewClient("127.0.0.1:1")
	a.client.delays = []time.Duration{time.Hour}
	refuseDials(a.client, math.MaxInt32)
	a.flushTimeout = 50 * time.Millisecond

	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	a.Run(ctx)

	assert.Less(t, time.Since(start), time.Second)
	assert.Greater(t, a.collector.PollCountDelta(), int64(5))
	assert.Positive(t, logs.FilterMessage("report postponed: previous report is still in progress").Len())
}
