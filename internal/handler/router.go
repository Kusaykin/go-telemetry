package handler

import (
	"net/http"

	"github.com/Kusaykin/go-telemetry/internal/middleware"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

func NewRouter(s Storage, db Pinger, log *zap.Logger, key string) http.Handler {
	h := New(s, db, log)

	signResponse, verifyRequest := passThrough, passThrough
	if key != "" {
		k := []byte(key)
		signResponse, verifyRequest = middleware.SignResponse(k), middleware.VerifyRequest(k)
	}

	r := chi.NewRouter()

	r.Use(middleware.Logging(log))
	r.Use(middleware.Gzip)
	r.Use(signResponse)

	r.Get("/", h.Index)
	r.Get("/ping", h.Ping)

	r.Group(func(r chi.Router) {
		r.Use(verifyRequest)

		r.Post("/update", h.UpdateJSON)
		r.Post("/update/", h.UpdateJSON)
		r.Post("/update/{type}/{name}/{value}", h.Update)
		r.Post("/update/{type}/{name}/", h.Update)
		r.Post("/updates/", h.Updates)
	})

	r.Route("/value", func(r chi.Router) {
		r.Post("/", h.ValueJSON)
		r.Get("/{type}/{name}", h.Value)
	})

	return r
}

func passThrough(next http.Handler) http.Handler {
	return next
}
