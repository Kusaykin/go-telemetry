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
		{"пустое имя метрики", http.MethodPost, "/update/gauge//12.5", http.StatusNotFound},
		{"пустой тип метрики", http.MethodPost, "/update//Alloc/12.5", http.StatusBadRequest},
		{"пустое имя в /value", http.MethodGet, "/value/gauge/", http.StatusNotFound},
		{"неизвестный путь", http.MethodPost, "/unknown", http.StatusNotFound},
		{"метод GET вместо POST", http.MethodGet, "/update/gauge/Alloc/12.5", http.StatusMethodNotAllowed},
		{"нет имени метрики в /value", http.MethodGet, "/value/gauge", http.StatusNotFound},
		{"метод POST вместо GET в /value", http.MethodPost, "/value/gauge/Alloc", http.StatusMethodNotAllowed},
		{"JSON /update без тела", http.MethodPost, "/update", http.StatusBadRequest},
		{"JSON /update со слешем", http.MethodPost, "/update/", http.StatusBadRequest},
		{"метод GET вместо POST в JSON /update", http.MethodGet, "/update", http.StatusMethodNotAllowed},
		{"JSON /value без тела", http.MethodPost, "/value", http.StatusBadRequest},
		{"JSON /value со слешем", http.MethodPost, "/value/", http.StatusBadRequest},
		{"метод GET вместо POST в JSON /value", http.MethodGet, "/value", http.StatusMethodNotAllowed},
		{"список метрик", http.MethodGet, "/", http.StatusOK},
		{"метод POST вместо GET в корне", http.MethodPost, "/", http.StatusMethodNotAllowed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(newFakeStorage(), tt.method, tt.path)

			assert.Equal(t, tt.want, rec.Code)
		})
	}
}
