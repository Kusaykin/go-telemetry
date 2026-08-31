package handler_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRouter(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		want   int
	}{
		{"метрика со значением", http.MethodPost, "/update/gauge/Alloc/12.5", http.StatusOK},
		{"пустое значение", http.MethodPost, "/update/gauge/Alloc/", http.StatusBadRequest},
		{"нет значения", http.MethodPost, "/update/gauge/Alloc", http.StatusNotFound},
		{"нет имени метрики", http.MethodPost, "/update/gauge", http.StatusNotFound},
		{"неизвестный путь", http.MethodPost, "/unknown", http.StatusNotFound},
		{"метод GET вместо POST", http.MethodGet, "/update/gauge/Alloc/12.5", http.StatusMethodNotAllowed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(newFakeStorage(), tt.method, tt.path)

			assert.Equal(t, tt.want, rec.Code)
		})
	}
}
