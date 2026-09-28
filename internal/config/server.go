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

func LoadServer(args []string, lookup LookupEnv, errOut io.Writer) (Server, error) {
	cfg := DefaultServer()

	fs := newFlagSet("server", errOut)
	fs.stringVar(&cfg.Address, "a", envAddress, addressUsage)

	if err := fs.parse(args, lookup); err != nil {
		return Server{}, err
	}

	return cfg, nil
}
