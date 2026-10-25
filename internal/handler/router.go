package handler

import (
	"net/http"

	"github.com/Kusaykin/go-telemetry/internal/middleware"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

func NewRouter(s Storage, db Pinger, log *zap.Logger) http.Handler {
	h := New(s, db, log)

	r := chi.NewRouter()

	r.Use(middleware.Logging(log))
	r.Use(middleware.Gzip)

	r.Get("/", h.Index)
	r.Get("/ping", h.Ping)

	r.Route("/update", func(r chi.Router) {
		r.Post("/", h.UpdateJSON)
		r.Post("/{type}/{name}/{value}", h.Update)
		r.Post("/{type}/{name}/", h.Update)
	})

	r.Post("/updates/", h.Updates)

	r.Route("/value", func(r chi.Router) {
		r.Post("/", h.ValueJSON)
		r.Get("/{type}/{name}", h.Value)
	})

	return r
}
