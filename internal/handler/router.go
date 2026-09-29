package handler

import (
	"net/http"

	"github.com/Kusaykin/go-telemetry/internal/middleware"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

func NewRouter(s Storage, log *zap.Logger) http.Handler {
	h := New(s)

	r := chi.NewRouter()

	r.Use(middleware.Logging(log))

	r.Get("/", h.Index)

	r.Route("/update", func(r chi.Router) {
		r.Post("/{type}/{name}/{value}", h.Update)
		r.Post("/{type}/{name}/", h.Update)
	})

	r.Get("/value/{type}/{name}", h.Value)

	return r
}
