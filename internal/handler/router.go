package handler

import "net/http"

func NewRouter(s Storage) http.Handler {
	update := NewUpdateHandler(s)

	mux := http.NewServeMux()
	mux.Handle("POST /update/{type}/{name}/{value}", update)
	mux.Handle("POST /update/{type}/{name}/{$}", update)
	mux.HandleFunc("POST /update/{type}/{name}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	return mux
}
