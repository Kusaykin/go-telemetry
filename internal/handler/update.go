package handler

import (
	"math"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	models "github.com/Kusaykin/go-telemetry/internal/model"
)

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	switch chi.URLParam(r, "type") {
	case models.Gauge:
		value, err := strconv.ParseFloat(chi.URLParam(r, "value"), 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		h.store.UpdateGauge(name, value)
	case models.Counter:
		delta, err := strconv.ParseInt(chi.URLParam(r, "value"), 10, 64)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		h.store.UpdateCounter(name, delta)
	default:
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) UpdateJSON(w http.ResponseWriter, r *http.Request) {
	m, err := decodeMetric(w, r)
	if err != nil {
		w.WriteHeader(decodeStatus(err))
		return
	}

	if m.ID == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	if err := m.Validate(); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	switch m.MType {
	case models.Gauge:
		h.store.UpdateGauge(m.ID, *m.Value)
		m.Delta = nil
	case models.Counter:
		delta := h.store.UpdateCounter(m.ID, *m.Delta)
		m.Delta = &delta
		m.Value = nil
	}

	writeJSON(w, m)
}
