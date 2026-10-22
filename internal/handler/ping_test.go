package handler_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Kusaykin/go-telemetry/internal/handler"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

type fakePinger struct {
	err error
}

func (f fakePinger) PingContext(context.Context) error {
	return f.err
}

func TestPing(t *testing.T) {
	tests := []struct {
		name    string
		db      handler.Pinger
		method  string
		want    int
		wantLog string
	}{
		{"БД доступна", fakePinger{}, http.MethodGet, http.StatusOK, ""},
		{"БД недоступна", fakePinger{err: errors.New("connection refused")}, http.MethodGet, http.StatusInternalServerError, "db ping failed"},
		{"БД не настроена", nil, http.MethodGet, http.StatusInternalServerError, "db ping: database is not configured"},
		{"метод POST вместо GET", fakePinger{}, http.MethodPost, http.StatusMethodNotAllowed, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core, logs := observer.New(zapcore.WarnLevel)
			req := httptest.NewRequest(tt.method, "/ping", nil)
			rec := httptest.NewRecorder()

			handler.NewRouter(newFakeStorage(), tt.db, zap.New(core)).ServeHTTP(rec, req)

			assert.Equal(t, tt.want, rec.Code)
			if tt.wantLog == "" {
				assert.Zero(t, logs.Len())
			} else {
				assert.Equal(t, 1, logs.FilterMessage(tt.wantLog).Len())
			}
		})
	}
}
