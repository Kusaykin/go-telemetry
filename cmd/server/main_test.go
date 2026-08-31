package main

import (
	"flag"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFlagsDefaults(t *testing.T) {
	cfg, err := parseFlags(nil, io.Discard)

	require.NoError(t, err)
	assert.Equal(t, "localhost:8080", cfg.Address)
}

func TestParseFlagsAddress(t *testing.T) {
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
			cfg, err := parseFlags(tt.args, io.Discard)

			require.NoError(t, err)
			assert.Equal(t, tt.want, cfg.Address)
		})
	}
}

func TestParseFlagsErrors(t *testing.T) {
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
			_, err := parseFlags(tt.args, io.Discard)

			assert.Error(t, err)
		})
	}
}

func TestParseFlagsHelp(t *testing.T) {
	_, err := parseFlags([]string{"-h"}, io.Discard)

	assert.ErrorIs(t, err, flag.ErrHelp)
}
