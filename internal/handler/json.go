package handler

import (
	"bytes"
	"errors"
	"io"
	"net/http"

	"github.com/mailru/easyjson"

	models "github.com/Kusaykin/go-telemetry/internal/model"
)

const maxBodySize = 4 << 10

var errNullBody = errors.New("body is null")

func decodeMetric(w http.ResponseWriter, r *http.Request) (models.Metrics, error) {
	var m models.Metrics

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodySize))
	if err != nil {
		return m, err
	}

	if bytes.Equal(bytes.TrimSpace(body), []byte("null")) {
		return m, errNullBody
	}

	err = easyjson.Unmarshal(body, &m)

	return m, err
}

// decodeStatus возвращает код ответа для ошибки decodeMetric.
func decodeStatus(err error) int {
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		return http.StatusRequestEntityTooLarge
	}

	return http.StatusBadRequest
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
