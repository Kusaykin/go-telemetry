package handler_test

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	models "github.com/Kusaykin/go-telemetry/internal/model"
	"github.com/stretchr/testify/assert"
)

func TestStorageErrorReturns500(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"обновление gauge", http.MethodPost, "/update/gauge/Alloc/1.5", ""},
		{"обновление counter", http.MethodPost, "/update/counter/PollCount/1", ""},
		{"обновление gauge JSON", http.MethodPost, "/update/", `{"id":"Alloc","type":"gauge","value":1.5}`},
		{"обновление counter JSON", http.MethodPost, "/update/", `{"id":"PollCount","type":"counter","delta":1}`},
		{"значение gauge", http.MethodGet, "/value/gauge/Alloc", ""},
		{"значение counter", http.MethodGet, "/value/counter/PollCount", ""},
		{"значение gauge JSON", http.MethodPost, "/value/", `{"id":"Alloc","type":"gauge"}`},
		{"значение counter JSON", http.MethodPost, "/value/", `{"id":"PollCount","type":"counter"}`},
		{"список метрик", http.MethodGet, "/", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newFakeStorage()
			store.err = errors.New("db is down")

			rec := doBody(store, tt.method, tt.path, tt.body)

			assert.Equal(t, http.StatusInternalServerError, rec.Code)
		})
	}
}

func TestCounterOverflowReturns400(t *testing.T) {
	tests := []struct {
		name string
		path string
		body string
	}{
		{"обновление counter", "/update/counter/PollCount/1", ""},
		{"обновление counter JSON", "/update/", `{"id":"PollCount","type":"counter","delta":1}`},
		{"батч", "/updates/", `[{"id":"PollCount","type":"counter","delta":1}]`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newFakeStorage()
			store.err = fmt.Errorf("update counter PollCount: %w", models.ErrCounterOverflow)

			rec := doBody(store, http.MethodPost, tt.path, tt.body)

			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})
	}
}
