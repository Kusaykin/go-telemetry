package handler

import (
	"net/http"

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

	for i, m := range metrics {
		if err := m.Validate(); err != nil {
			h.log.Warn("updates: batch rejected",
				zap.Int("status", http.StatusBadRequest),
				zap.Int("index", i),
				zap.String("id", m.ID),
				zap.String("type", m.MType),
				zap.Error(err),
			)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}

	if err := h.store.UpdateBatch(r.Context(), metrics); err != nil {
		h.storageError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("{}"))
}
