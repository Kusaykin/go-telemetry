package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/Kusaykin/go-telemetry/internal/handler"
	"github.com/Kusaykin/go-telemetry/internal/repository"
)

const defaultAddress = "localhost:8080"

type config struct {
	Address string
}

func parseFlags(args []string, errOut io.Writer) (config, error) {
	cfg := config{Address: defaultAddress}

	fs := flag.NewFlagSet("server", flag.ContinueOnError)
	fs.SetOutput(errOut)
	fs.StringVar(&cfg.Address, "a", cfg.Address, "адрес эндпоинта HTTP-сервера")

	if err := fs.Parse(args); err != nil {
		return config{}, err
	}

	if fs.NArg() > 0 {
		err := fmt.Errorf("неизвестный аргумент: %q", fs.Arg(0))
		fmt.Fprintln(errOut, err)
		fs.Usage()

		return config{}, err
	}

	return cfg, nil
}

func main() {
	cfg, err := parseFlags(os.Args[1:], os.Stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}

		os.Exit(2)
	}

	if err := run(cfg); err != nil {
		log.Fatal(err)
	}
}

func run(cfg config) error {
	store := repository.NewMemStorage()

	return http.ListenAndServe(cfg.Address, handler.NewRouter(store))
}
