package main

import (
	"net/http"

	"github.com/Kusaykin/go-telemetry/internal/handler"
	"github.com/Kusaykin/go-telemetry/internal/repository"
)

func main() {
	if err := run(); err != nil {
		panic(err)
	}
}

func run() error {
	store := repository.NewMemStorage()
	update := handler.NewUpdateHandler(store)

	mux := http.NewServeMux()
	mux.Handle("POST /update/{type}/{name}/{value}", update)
	mux.Handle("POST /update/{type}/{name}/{$}", update)
	mux.HandleFunc("POST /update/{type}/{name}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	return http.ListenAndServe(`:8080`, mux)
}
