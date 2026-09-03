package config

import "io"

type Server struct {
	Address string
}

func DefaultServer() Server {
	return Server{
		Address: DefaultAddress,
	}
}

func LoadServer(args []string, errOut io.Writer) (Server, error) {
	cfg := DefaultServer()

	fs := newFlagSet("server", errOut)
	fs.StringVar(&cfg.Address, "a", cfg.Address, addressUsage)

	if err := parse(fs, args, errOut); err != nil {
		return Server{}, err
	}

	return cfg, nil
}
