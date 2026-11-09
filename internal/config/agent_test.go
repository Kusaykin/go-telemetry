package config

import (
	"bytes"
	"flag"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultAgent(t *testing.T) {
	cfg := DefaultAgent()

	assert.Equal(t, "localhost:8080", cfg.Address)
	assert.Equal(t, 2*time.Second, cfg.PollInterval)
	assert.Equal(t, 10*time.Second, cfg.ReportInterval)
	assert.Equal(t, 1, cfg.RateLimit)
}

func TestLoadAgentDefaults(t *testing.T) {
	cfg, err := LoadAgent(nil, nil, io.Discard)

	require.NoError(t, err)
	assert.Equal(t, DefaultAgent(), cfg)
}

func TestLoadAgent(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want Agent
	}{
		{
			"адрес через знак равенства",
			[]string{"-a=example:9090"},
			Agent{Address: "example:9090", PollInterval: 2 * time.Second, ReportInterval: 10 * time.Second, RateLimit: 1},
		},
		{
			"адрес через пробел",
			[]string{"-a", "example:9090"},
			Agent{Address: "example:9090", PollInterval: 2 * time.Second, ReportInterval: 10 * time.Second, RateLimit: 1},
		},
		{
			"секунды переводятся в Duration",
			[]string{"-r=30", "-p=5"},
			Agent{Address: "localhost:8080", PollInterval: 5 * time.Second, ReportInterval: 30 * time.Second, RateLimit: 1},
		},
		{
			"все флаги вместе",
			[]string{"-a=127.0.0.1:9000", "-r=1", "-p=1"},
			Agent{Address: "127.0.0.1:9000", PollInterval: time.Second, ReportInterval: time.Second, RateLimit: 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := LoadAgent(tt.args, nil, io.Discard)

			require.NoError(t, err)
			assert.Equal(t, tt.want, cfg)
		})
	}
}

func TestLoadAgentErrors(t *testing.T) {
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
		{"переполнение -r", []string{"-r=9999999999"}},
		{"переполнение -p", []string{"-p=9999999999"}},
		{"лишний позиционный аргумент", []string{"-p=2", "мусор"}},
		{"нечисловой -l", []string{"-l=abc"}},
		{"нулевой -l", []string{"-l=0"}},
		{"отрицательный -l", []string{"-l=-3"}},
		{"-l больше максимума", []string{"-l=1025"}},
		{"переполнение -l", []string{"-l=99999999999999999999"}},
		{"отрицательное переполнение -l", []string{"-l=-99999999999999999999"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadAgent(tt.args, nil, io.Discard)

			assert.Error(t, err)
		})
	}
}

func TestLoadAgentHelp(t *testing.T) {
	_, err := LoadAgent([]string{"-h"}, nil, io.Discard)

	assert.ErrorIs(t, err, flag.ErrHelp)
}

func TestLoadAgentEnv(t *testing.T) {
	tests := []struct {
		name string
		args []string
		env  map[string]string
		want Agent
	}{
		{
			"только env",
			nil,
			map[string]string{"ADDRESS": "env:1", "REPORT_INTERVAL": "20", "POLL_INTERVAL": "4"},
			Agent{Address: "env:1", PollInterval: 4 * time.Second, ReportInterval: 20 * time.Second, RateLimit: 1},
		},
		{
			"env приоритетнее флагов",
			[]string{"-a=flag:2", "-r=30", "-p=5"},
			map[string]string{"ADDRESS": "env:1", "REPORT_INTERVAL": "20", "POLL_INTERVAL": "4"},
			Agent{Address: "env:1", PollInterval: 4 * time.Second, ReportInterval: 20 * time.Second, RateLimit: 1},
		},
		{
			"флаги остаются, если env не задан",
			[]string{"-a=flag:2", "-r=30"},
			map[string]string{"POLL_INTERVAL": "4"},
			Agent{Address: "flag:2", PollInterval: 4 * time.Second, ReportInterval: 30 * time.Second, RateLimit: 1},
		},
		{
			"пустые переменные игнорируются",
			[]string{"-a=flag:2"},
			map[string]string{"ADDRESS": "", "REPORT_INTERVAL": "", "POLL_INTERVAL": ""},
			Agent{Address: "flag:2", PollInterval: 2 * time.Second, ReportInterval: 10 * time.Second, RateLimit: 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := LoadAgent(tt.args, envMap(tt.env), io.Discard)

			require.NoError(t, err)
			assert.Equal(t, tt.want, cfg)
		})
	}
}

func TestLoadAgentKey(t *testing.T) {
	tests := []struct {
		name string
		args []string
		env  map[string]string
		want string
	}{
		{"флаг", []string{"-k=flag-key"}, nil, "flag-key"},
		{"только env", nil, map[string]string{"KEY": "env-key"}, "env-key"},
		{"env приоритетнее флага", []string{"-k=flag-key"}, map[string]string{"KEY": "env-key"}, "env-key"},
		{"по умолчанию пусто", nil, map[string]string{}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := LoadAgent(tt.args, envMap(tt.env), io.Discard)

			require.NoError(t, err)
			assert.Equal(t, tt.want, cfg.Key)
		})
	}
}

func TestLoadAgentRateLimit(t *testing.T) {
	tests := []struct {
		name string
		args []string
		env  map[string]string
		want int
	}{
		{"по умолчанию", nil, nil, 1},
		{"флаг", []string{"-l=4"}, nil, 4},
		{"только env", nil, map[string]string{"RATE_LIMIT": "3"}, 3},
		{"env приоритетнее флага", []string{"-l=4"}, map[string]string{"RATE_LIMIT": "3"}, 3},
		{"пустая env игнорируется", []string{"-l=4"}, map[string]string{"RATE_LIMIT": ""}, 4},
		{"максимум", []string{"-l=1024"}, nil, MaxRateLimit},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := LoadAgent(tt.args, envMap(tt.env), io.Discard)

			require.NoError(t, err)
			assert.Equal(t, tt.want, cfg.RateLimit)
		})
	}
}

func TestLoadAgentRateLimitEnvError(t *testing.T) {
	var out bytes.Buffer

	_, err := LoadAgent(nil, envMap(map[string]string{"RATE_LIMIT": "0"}), &out)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "RATE_LIMIT")
	assert.Contains(t, err.Error(), "должно быть больше нуля")
}

func TestLoadAgentEnvErrorOutput(t *testing.T) {
	var out bytes.Buffer

	_, err := LoadAgent(nil, envMap(map[string]string{"REPORT_INTERVAL": "abc"}), &out)

	require.Error(t, err)
	assert.Equal(t, 2, ExitCode(err))
	assert.Contains(t, out.String(), "REPORT_INTERVAL")
	assert.NotContains(t, out.String(), "Usage of")
}

func TestLoadAgentHelpListsEnv(t *testing.T) {
	var out bytes.Buffer

	_, err := LoadAgent([]string{"-h"}, nil, &out)
	require.ErrorIs(t, err, flag.ErrHelp)
	assert.Contains(t, out.String(), "Usage of agent:")
	assert.Contains(t, out.String(), "-a string")

	_, envHelp, found := strings.Cut(out.String(), "Переменные окружения")
	require.True(t, found)

	assert.Regexp(t, `ADDRESS +адрес эндпоинта HTTP-сервера \(-a\)`, envHelp)
	assert.Regexp(t, `REPORT_INTERVAL +частота отправки метрик на сервер, секунды \(-r\)`, envHelp)
	assert.Regexp(t, `POLL_INTERVAL +частота опроса метрик из runtime, секунды \(-p\)`, envHelp)
	assert.Regexp(t, `RATE_LIMIT +максимальное число одновременных запросов к серверу \(-l\)`, envHelp)
	assert.NotContains(t, envHelp, "`", "обратные кавычки из usage должны убираться")
}

func TestLoadAgentFlagErrorShowsUsage(t *testing.T) {
	var out bytes.Buffer

	_, err := LoadAgent([]string{"-r=abc"}, nil, &out)

	require.Error(t, err)
	assert.Contains(t, out.String(), "Usage of agent:")
	assert.Contains(t, out.String(), "REPORT_INTERVAL")
}

func TestLoadAgentSecondsErrorMessage(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		env     map[string]string
		value   string
		context string
		reason  string
	}{
		{"флаг не число", []string{"-r=abc"}, nil, `"abc"`, "-r", "ожидается целое число секунд"},
		{"флаг ноль", []string{"-p=0"}, nil, `"0"`, "-p", "должно быть больше нуля"},
		{"флаг переполнение", []string{"-r=9999999999"}, nil, `"9999999999"`, "-r", "должно быть не больше"},
		{"env не число", nil, map[string]string{"REPORT_INTERVAL": "abc"}, `"abc"`, "REPORT_INTERVAL", "ожидается целое число секунд"},
		{"env ноль", nil, map[string]string{"POLL_INTERVAL": "0"}, `"0"`, "POLL_INTERVAL", "должно быть больше нуля"},
		{"env переполнение", nil, map[string]string{"POLL_INTERVAL": "9999999999"}, `"9999999999"`, "POLL_INTERVAL", "должно быть не больше"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer

			_, err := LoadAgent(tt.args, envMap(tt.env), &out)
			require.Error(t, err)

			msg := err.Error()
			assert.Contains(t, msg, tt.context)
			assert.Contains(t, msg, tt.reason)
			assert.Equal(t, 1, strings.Count(msg, tt.value), "значение должно встречаться в сообщении один раз: %s", msg)
			assert.NotContains(t, out.String(), "strconv")
		})
	}
}
