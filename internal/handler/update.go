package handler

import (
	"errors"
	"math"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	models "github.com/Kusaykin/go-telemetry/internal/model"
)

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if err := models.ValidateID(name); err != nil {
		w.WriteHeader(invalidMetricStatus(err))
		return
	}

	switch chi.URLParam(r, "type") {
	case models.Gauge:
		value, err := strconv.ParseFloat(chi.URLParam(r, "value"), 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if err := h.store.UpdateGauge(r.Context(), name, value); err != nil {
			h.storageError(w, r, err)
			return
		}
	case models.Counter:
		delta, err := strconv.ParseInt(chi.URLParam(r, "value"), 10, 64)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if _, err := h.store.UpdateCounter(r.Context(), name, delta); err != nil {
			h.storageError(w, r, err)
			return
		}
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

	if err := m.Validate(); err != nil {
		w.WriteHeader(invalidMetricStatus(err))
		return
	}

	switch m.MType {
	case models.Gauge:
		if err := h.store.UpdateGauge(r.Context(), m.ID, *m.Value); err != nil {
			h.storageError(w, r, err)
			return
		}
		m.Delta = nil
	case models.Counter:
		delta, err := h.store.UpdateCounter(r.Context(), m.ID, *m.Delta)
		if err != nil {
			h.storageError(w, r, err)
			return
		}
		m.Delta = &delta
		m.Value = nil
	}

	writeJSON(w, m)
}

func (h *Handler) storageError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, models.ErrCounterOverflow) {
		h.log.Warn("storage: counter overflow", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if r.Context().Err() != nil {
		h.log.Debug("storage: request canceled", zap.Error(err))
	} else {
		h.log.Error("storage", zap.Error(err))
	}

	w.WriteHeader(http.StatusInternalServerError)
}
