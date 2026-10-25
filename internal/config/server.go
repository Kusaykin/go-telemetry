package config

import (
	"io"
	"time"
)

const (
	DefaultStoreInterval   = 300 * time.Second
	DefaultFileStoragePath = "metrics-db.json"
	DefaultRestore         = false

	envStoreInterval   = "STORE_INTERVAL"
	envFileStoragePath = "FILE_STORAGE_PATH"
	envRestore         = "RESTORE"
)

type Server struct {
	Address         string
	StoreInterval   time.Duration
	FileStoragePath string
	Restore         bool
}

func DefaultServer() Server {
	return Server{
		Address:         DefaultAddress,
		StoreInterval:   DefaultStoreInterval,
		FileStoragePath: DefaultFileStoragePath,
		Restore:         DefaultRestore,
	}
}

func LoadServer(args []string, lookup LookupEnv, errOut io.Writer) (Server, error) {
	cfg := DefaultServer()

	fs := newFlagSet("server", errOut)
	fs.stringVar(&cfg.Address, "a", envAddress, addressUsage)
	fs.nonNegativeSecondsVar(&cfg.StoreInterval, "i", envStoreInterval,
		"интервал сохранения метрик на диск, `секунды` (0 — синхронная запись)")
	fs.stringVar(&cfg.FileStoragePath, "f", envFileStoragePath,
		"`путь` до файла с сохранёнными метриками (пусто — не сохранять)")
	fs.boolVar(&cfg.Restore, "r", envRestore, "загружать сохранённые метрики из файла при старте")

	if err := fs.parse(args, lookup); err != nil {
		return Server{}, err
	}

	return cfg, nil
}
