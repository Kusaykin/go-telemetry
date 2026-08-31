package handler

import (
	"net/http"
	"strconv"
)

const (
	typeGauge   = "gauge"
	typeCounter = "counter"
)

type Storage interface {
	UpdateGauge(name string, value float64)
	UpdateCounter(name string, delta int64)
}

func NewUpdateHandler(s Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")

		switch r.PathValue("type") {
		case typeGauge:
			value, err := strconv.ParseFloat(r.PathValue("value"), 64)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			s.UpdateGauge(name, value)
		case typeCounter:
			delta, err := strconv.ParseInt(r.PathValue("value"), 10, 64)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			s.UpdateCounter(name, delta)
		default:
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
	}
}
