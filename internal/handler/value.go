package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	models "github.com/Kusaykin/go-telemetry/internal/model"
)

func (h *Handler) Value(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	var value string

	switch chi.URLParam(r, "type") {
	case models.Gauge:
		gauge, ok := h.store.Gauge(name)
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		value = models.FormatGauge(gauge)
	case models.Counter:
		counter, ok := h.store.Counter(name)
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		value = models.FormatCounter(counter)
	default:
		w.WriteHeader(http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	if _, err := w.Write([]byte(value)); err != nil {
		return
	}
}
