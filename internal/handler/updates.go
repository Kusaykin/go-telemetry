package handler

import (
	"errors"
	"net/http"

	models "github.com/Kusaykin/go-telemetry/internal/model"
	"go.uber.org/zap"
)

func (h *Handler) Updates(w http.ResponseWriter, r *http.Request) {
	metrics, err := decodeMetrics(w, r)
	if err != nil {
		status := decodeStatus(err)
		h.log.Warn("updates: batch rejected", zap.Int("status", status), zap.Error(err))
		w.WriteHeader(status)
		return
	}

	if err := h.store.UpdateBatch(r.Context(), metrics); err != nil {
		var invalid *models.InvalidMetricError
		if errors.As(err, &invalid) {
			h.log.Warn("updates: batch rejected",
				zap.Int("status", http.StatusBadRequest),
				zap.Error(err),
				zap.Int("index", invalid.Index),
				zap.String("id", invalid.Metric.ID),
				zap.String("type", invalid.Metric.MType),
			)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		h.storageError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("{}"))
}
