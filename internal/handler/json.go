package handler

import (
	"bytes"
	"errors"
	"io"
	"net/http"

	"github.com/mailru/easyjson"

	models "github.com/Kusaykin/go-telemetry/internal/model"
)

const (
	maxBodySize      = 4 << 10
	maxBatchBodySize = 1 << 20
)

var errNullBody = errors.New("body is null")

func decodeMetric(w http.ResponseWriter, r *http.Request) (models.Metrics, error) {
	var m models.Metrics

	err := decodeBody(w, r, maxBodySize, &m)

	return m, err
}

func decodeMetrics(w http.ResponseWriter, r *http.Request) (models.MetricsList, error) {
	var batch models.MetricsList

	err := decodeBody(w, r, maxBatchBodySize, &batch)

	return batch, err
}

func decodeBody(w http.ResponseWriter, r *http.Request, limit int64, v easyjson.Unmarshaler) error {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, limit))
	if err != nil {
		return err
	}

	if bytes.Equal(bytes.TrimSpace(body), []byte("null")) {
		return errNullBody
	}

	return easyjson.Unmarshal(body, v)
}

func decodeStatus(err error) int {
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		return http.StatusRequestEntityTooLarge
	}

	return http.StatusBadRequest
}

func validateMetric(m models.Metrics) int {
	err := m.Validate()
	switch {
	case err == nil:
		return http.StatusOK
	case errors.Is(err, models.ErrEmptyID):
		return http.StatusNotFound
	default:
		return http.StatusBadRequest
	}
}

func writeJSON(w http.ResponseWriter, m models.Metrics) {
	body, err := easyjson.Marshal(m)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}
