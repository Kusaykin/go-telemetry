package agent

import (
	"compress/gzip"
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
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

func (a *Agent) pollAndReport(ctx context.Context, n int) {
	for range n {
		a.collector.Poll()
	}

	a.send(ctx, a.prepare())
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

func TestAgentSendsReportAsBatch(t *testing.T) {
	ts, client := newTestServer(t, http.StatusOK)
	a := New(config.DefaultAgent(), zap.NewNop())
	a.client = client

	a.pollAndReport(t.Context(), 5)
	require.Len(t, ts.Requests(), 1)
	assert.Equal(t, "/updates/", ts.Requests()[0].path)
	require.Len(t, sent(t, ts), metricsCount)
	assert.Contains(t, sent(t, ts), "counter/PollCount/5")

	a.pollAndReport(t.Context(), 5)
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
	a.client = NewClient(srv.Listener.Addr().String(), "")

	for range 4 {
		a.pollAndReport(t.Context(), 5)
	}

	assert.Equal(t, int64(20), total.Load())
}

func TestPollCountSurvivesFailedReport(t *testing.T) {
	ts, client := newTestServer(t, http.StatusInternalServerError)

	a := New(config.DefaultAgent(), zap.NewNop())
	a.client = client

	a.pollAndReport(t.Context(), 5)
	a.pollAndReport(t.Context(), 5)

	assert.Contains(t, sent(t, ts), "counter/PollCount/5")
	assert.Contains(t, sent(t, ts), "counter/PollCount/10")
	assert.Equal(t, int64(10), a.collector.PollCount())

	ts.SetStatus(http.StatusOK)
	a.pollAndReport(t.Context(), 5)

	assert.Contains(t, sent(t, ts), "counter/PollCount/15")
	assert.Zero(t, a.collector.PollCount())
}

func TestAgentLogsReport(t *testing.T) {
	_, client := newTestServer(t, http.StatusOK)
	core, logs := observer.New(zapcore.DebugLevel)

	a := New(config.DefaultAgent(), zap.New(core))
	a.client = client

	a.pollAndReport(t.Context(), 5)

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

	a.pollAndReport(t.Context(), 5)
	a.pollAndReport(t.Context(), 5)

	assert.Len(t, ts.Requests(), 2)
	assert.Equal(t, 2, logs.FilterMessage("report").Len())

	failures := logs.FilterMessage("report failed").All()
	require.Len(t, failures, 2)
	assert.Equal(t, zapcore.ErrorLevel, failures[0].Level)
}

func TestEnqueueQueuesReport(t *testing.T) {
	a := New(config.DefaultAgent(), zap.NewNop())
	a.collector.Poll()

	jobs := make(chan report, 1)
	a.enqueue(jobs)

	require.Len(t, jobs, 1, "отчёт ждёт в очереди, даже если воркер ещё не вернулся за задачей")
	r := <-jobs
	assert.Equal(t, int64(1), r.pollCount)
	assert.Zero(t, a.collector.PollCount(), "PollCount забран в отчёт")
}

func TestEnqueueReplacesPendingReport(t *testing.T) {
	core, logs := observer.New(zapcore.InfoLevel)
	a := New(config.DefaultAgent(), zap.New(core))

	jobs := make(chan report, 1)

	a.collector.Poll()
	a.collector.Poll()
	a.enqueue(jobs)

	a.collector.Poll()
	a.enqueue(jobs)

	require.Len(t, jobs, 1, "в очереди остаётся только свежий отчёт")
	r := <-jobs
	assert.Equal(t, int64(3), r.pollCount, "PollCount вытесненного отчёта переходит в новый")
	assert.Contains(t, r.metrics, counter(PollCountName, 3))
	assert.Equal(t, 1, logs.FilterMessage("pending report replaced: all workers are busy").Len())
}

func TestAgentLimitsConcurrentRequests(t *testing.T) {
	const rateLimit = 3

	var inflight, maxInflight atomic.Int32
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := inflight.Add(1)
		defer inflight.Add(-1)

		for {
			m := maxInflight.Load()
			if n <= m || maxInflight.CompareAndSwap(m, n) {
				break
			}
		}

		select {
		case <-release:
		case <-r.Context().Done():
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	cfg := config.DefaultAgent()
	cfg.PollInterval = 5 * time.Millisecond
	cfg.ReportInterval = 5 * time.Millisecond
	cfg.RateLimit = rateLimit

	a := New(cfg, zap.NewNop())
	a.client = NewClient(srv.Listener.Addr().String(), "")

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		a.Run(ctx)
	}()

	require.Eventually(t, func() bool { return inflight.Load() == rateLimit },
		5*time.Second, time.Millisecond, "пул должен использоваться целиком")

	time.Sleep(10 * cfg.ReportInterval)
	assert.Equal(t, int32(rateLimit), maxInflight.Load())

	close(release)
	cancel()
	<-done

	assert.LessOrEqual(t, maxInflight.Load(), int32(rateLimit))
}

func TestAgentSendsSystemMetrics(t *testing.T) {
	ts, client := newTestServer(t, http.StatusOK)

	cfg := config.DefaultAgent()
	cfg.PollInterval = 10 * time.Millisecond
	cfg.ReportInterval = time.Hour

	a := New(cfg, zap.NewNop())
	a.client = client

	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	a.Run(ctx)

	var ids []string
	for _, key := range sent(t, ts) {
		ids = append(ids, strings.Split(key, "/")[1])
	}

	assert.Contains(t, ids, TotalMemoryName)
	assert.Contains(t, ids, FreeMemoryName)
	assert.Contains(t, ids, CPUUtilizationPrefix+"1")
	assert.Contains(t, ids, PollCountName)
}

func TestAgentKeepsPollingWhileReportRetries(t *testing.T) {
	cfg := config.DefaultAgent()
	cfg.PollInterval = 10 * time.Millisecond
	cfg.ReportInterval = 10 * time.Millisecond

	core, logs := observer.New(zapcore.InfoLevel)
	a := New(cfg, zap.New(core))
	a.client = NewClient("127.0.0.1:1", "")
	a.client.delays = []time.Duration{time.Hour}
	refuseDials(a.client, math.MaxInt32)
	a.flushTimeout = 50 * time.Millisecond

	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	a.Run(ctx)

	assert.Less(t, time.Since(start), time.Second)
	assert.Greater(t, a.collector.PollCount(), int64(5))
	assert.Positive(t, logs.FilterMessage("pending report replaced: all workers are busy").Len())
}
