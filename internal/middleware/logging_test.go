package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Kusaykin/go-telemetry/internal/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestLogging(t *testing.T) {
	tests := []struct {
		name       string
		handler    http.HandlerFunc
		wantStatus int
		wantSize   int64
	}{
		{
			name: "тело без WriteHeader",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte("hello"))
			},
			wantStatus: http.StatusOK,
			wantSize:   5,
		},
		{
			name: "явный статус",
			handler: func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "not found", http.StatusNotFound)
			},
			wantStatus: http.StatusNotFound,
			wantSize:   int64(len("not found\n")),
		},
		{
			name:       "пустой ответ",
			handler:    func(w http.ResponseWriter, r *http.Request) {},
			wantStatus: http.StatusOK,
			wantSize:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core, logs := observer.New(zapcore.DebugLevel)
			h := middleware.Logging(zap.New(core))(tt.handler)

			req := httptest.NewRequest(http.MethodPost, "/update/gauge/Alloc/1?x=1", nil)
			h.ServeHTTP(httptest.NewRecorder(), req)

			require.Equal(t, 1, logs.Len())
			entry := logs.All()[0]
			fields := entry.ContextMap()

			assert.Equal(t, zapcore.InfoLevel, entry.Level)
			assert.Equal(t, "/update/gauge/Alloc/1?x=1", fields["uri"])
			assert.Equal(t, http.MethodPost, fields["method"])
			assert.Contains(t, fields, "duration")
			assert.Equal(t, int64(tt.wantStatus), fields["status"])
			assert.Equal(t, tt.wantSize, fields["size"])
		})
	}
}
