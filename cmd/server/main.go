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

	return http.ListenAndServe(`:8080`, handler.NewRouter(store))
}
