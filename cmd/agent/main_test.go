package main

import (
	"flag"
	"io"
	"testing"
	"time"

	"github.com/Kusaykin/go-telemetry/internal/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFlagsDefaults(t *testing.T) {
	cfg, err := parseFlags(nil, io.Discard)

	require.NoError(t, err)
	assert.Equal(t, "localhost:8080", cfg.Address)
	assert.Equal(t, 2*time.Second, cfg.PollInterval)
	assert.Equal(t, 10*time.Second, cfg.ReportInterval)
	assert.Equal(t, agent.DefaultConfig(), cfg)
}

func TestParseFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want agent.Config
	}{
		{
			"адрес через знак равенства",
			[]string{"-a=example:9090"},
			agent.Config{Address: "example:9090", PollInterval: 2 * time.Second, ReportInterval: 10 * time.Second},
		},
		{
			"адрес через пробел",
			[]string{"-a", "example:9090"},
			agent.Config{Address: "example:9090", PollInterval: 2 * time.Second, ReportInterval: 10 * time.Second},
		},
		{
			"секунды переводятся в Duration",
			[]string{"-r=30", "-p=5"},
			agent.Config{Address: "localhost:8080", PollInterval: 5 * time.Second, ReportInterval: 30 * time.Second},
		},
		{
			"все флаги вместе",
			[]string{"-a=127.0.0.1:9000", "-r=1", "-p=1"},
			agent.Config{Address: "127.0.0.1:9000", PollInterval: time.Second, ReportInterval: time.Second},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := parseFlags(tt.args, io.Discard)

			require.NoError(t, err)
			assert.Equal(t, tt.want, cfg)
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
		{"нечисловой -r", []string{"-r=abc"}},
		{"нечисловой -p", []string{"-p=1s"}},
		{"нулевой -p", []string{"-p=0"}},
		{"отрицательный -r", []string{"-r=-1"}},
		{"лишний позиционный аргумент", []string{"-p=2", "мусор"}},
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
