package config

import (
	"flag"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultServer(t *testing.T) {
	assert.Equal(t, "localhost:8080", DefaultServer().Address)
}

func TestLoadServerDefaults(t *testing.T) {
	cfg, err := LoadServer(nil, io.Discard)

	require.NoError(t, err)
	assert.Equal(t, "localhost:8080", cfg.Address)
	assert.Equal(t, DefaultServer(), cfg)
}

func TestLoadServerAddress(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"через знак равенства", []string{"-a=example:9090"}, "example:9090"},
		{"через пробел", []string{"-a", "example:9090"}, "example:9090"},
		{"только порт", []string{"-a=:9090"}, ":9090"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := LoadServer(tt.args, io.Discard)

			require.NoError(t, err)
			assert.Equal(t, tt.want, cfg.Address)
		})
	}
}

func TestLoadServerErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"неизвестный флаг", []string{"-x=1"}},
		{"неизвестный флаг без значения", []string{"-unknown"}},
		{"флаг агента серверу не подходит", []string{"-p=2"}},
		{"лишний позиционный аргумент", []string{"-a=example:9090", "мусор"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadServer(tt.args, io.Discard)

			assert.Error(t, err)
		})
	}
}

func TestLoadServerHelp(t *testing.T) {
	_, err := LoadServer([]string{"-h"}, io.Discard)

	assert.ErrorIs(t, err, flag.ErrHelp)
}
