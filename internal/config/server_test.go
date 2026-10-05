package config

import (
	"bytes"
	"flag"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultServer(t *testing.T) {
	assert.Equal(t, Server{
		Address:         "localhost:8080",
		StoreInterval:   300 * time.Second,
		FileStoragePath: "metrics-db.json",
		Restore:         false,
	}, DefaultServer())
}

func TestLoadServerDefaults(t *testing.T) {
	cfg, err := LoadServer(nil, nil, io.Discard)

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
			cfg, err := LoadServer(tt.args, nil, io.Discard)

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
			_, err := LoadServer(tt.args, nil, io.Discard)

			assert.Error(t, err)
		})
	}
}

func TestLoadServerHelp(t *testing.T) {
	_, err := LoadServer([]string{"-h"}, nil, io.Discard)

	assert.ErrorIs(t, err, flag.ErrHelp)
}

func TestLoadServerEnv(t *testing.T) {
	tests := []struct {
		name string
		args []string
		env  map[string]string
		want string
	}{
		{"только env", nil, map[string]string{"ADDRESS": "env:1"}, "env:1"},
		{"env приоритетнее флага", []string{"-a=flag:2"}, map[string]string{"ADDRESS": "env:1"}, "env:1"},
		{"флаг, если env не задан", []string{"-a=flag:2"}, map[string]string{}, "flag:2"},
		{"пустой ADDRESS игнорируется", []string{"-a=flag:2"}, map[string]string{"ADDRESS": ""}, "flag:2"},
		{"по умолчанию без env и флага", nil, map[string]string{}, "localhost:8080"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := LoadServer(tt.args, envMap(tt.env), io.Discard)

			require.NoError(t, err)
			assert.Equal(t, tt.want, cfg.Address)
		})
	}
}

func TestLoadServerHelpListsEnv(t *testing.T) {
	var out bytes.Buffer

	_, err := LoadServer([]string{"-h"}, nil, &out)

	require.ErrorIs(t, err, flag.ErrHelp)
	assert.Contains(t, out.String(), "Usage of server:")
	assert.Contains(t, out.String(), "-a string")
	assert.Contains(t, out.String(), "ADDRESS")
	assert.NotContains(t, out.String(), "REPORT_INTERVAL")

	for _, want := range []string{"-i секунды", "-f путь", "-r\t", "STORE_INTERVAL", "FILE_STORAGE_PATH", "RESTORE"} {
		assert.Contains(t, out.String(), want)
	}
}

func TestLoadServerStorageFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		env  map[string]string
		want Server
	}{
		{
			name: "флаги",
			args: []string{"-i=10", "-f=/tmp/x.json", "-r"},
			want: Server{Address: DefaultAddress, StoreInterval: 10 * time.Second, FileStoragePath: "/tmp/x.json", Restore: true},
		},
		{
			name: "флаги через пробел",
			args: []string{"-i", "0", "-f", "x.json", "-r=false"},
			want: Server{Address: DefaultAddress, StoreInterval: 0, FileStoragePath: "x.json", Restore: false},
		},
		{
			name: "пустой путь флагом отключает файл",
			args: []string{"-f="},
			want: Server{Address: DefaultAddress, StoreInterval: DefaultStoreInterval, FileStoragePath: "", Restore: false},
		},
		{
			name: "только env",
			env:  map[string]string{"STORE_INTERVAL": "0", "FILE_STORAGE_PATH": "/env.json", "RESTORE": "true"},
			want: Server{Address: DefaultAddress, StoreInterval: 0, FileStoragePath: "/env.json", Restore: true},
		},
		{
			name: "env приоритетнее флагов",
			args: []string{"-i=10", "-f=/flag.json", "-r=true"},
			env:  map[string]string{"STORE_INTERVAL": "2", "FILE_STORAGE_PATH": "/env.json", "RESTORE": "false"},
			want: Server{Address: DefaultAddress, StoreInterval: 2 * time.Second, FileStoragePath: "/env.json", Restore: false},
		},
		{
			name: "пустые env игнорируются",
			args: []string{"-i=10", "-f=/flag.json", "-r"},
			env:  map[string]string{"STORE_INTERVAL": "", "FILE_STORAGE_PATH": "", "RESTORE": ""},
			want: Server{Address: DefaultAddress, StoreInterval: 10 * time.Second, FileStoragePath: "/flag.json", Restore: true},
		},
		{
			name: "по умолчанию",
			env:  map[string]string{},
			want: DefaultServer(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := LoadServer(tt.args, envMap(tt.env), io.Discard)

			require.NoError(t, err)
			assert.Equal(t, tt.want, cfg)
		})
	}
}

func TestLoadServerStorageErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		env  map[string]string
	}{
		{"отрицательный интервал", []string{"-i=-1"}, nil},
		{"интервал не число", []string{"-i=abc"}, nil},
		{"интервал с единицами", []string{"-i=10s"}, nil},
		{"env интервал не число", nil, map[string]string{"STORE_INTERVAL": "abc"}},
		{"флаг restore не bool", []string{"-r=abc"}, nil},
		{"env restore не bool", nil, map[string]string{"RESTORE": "abc"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadServer(tt.args, envMap(tt.env), io.Discard)

			assert.Error(t, err)
		})
	}
}
