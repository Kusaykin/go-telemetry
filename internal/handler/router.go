package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func NewRouter(s Storage) http.Handler {
	h := New(s)

	r := chi.NewRouter()

	r.Get("/", h.Index)

	r.Route("/update", func(r chi.Router) {
		r.Post("/{type}/{name}/{value}", h.Update)
		r.Post("/{type}/{name}/", h.Update)
	})

	r.Get("/value/{type}/{name}", h.Value)

	return r
}
