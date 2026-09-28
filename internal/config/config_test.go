package config

import (
	"errors"
	"flag"
	"io"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExitCode(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"успех", nil, 0},
		{"запрос справки", flag.ErrHelp, 0},
		{"обёрнутый запрос справки", errors.Join(flag.ErrHelp), 0},
		{"ошибка разбора", errors.New("неизвестный флаг"), 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ExitCode(tt.err))
		})
	}
}

func envMap(m map[string]string) LookupEnv {
	return func(key string) (string, bool) {
		v, ok := m[key]
		return v, ok
	}
}

func TestBindEnvUnknownFlagPanics(t *testing.T) {
	fs := newFlagSet("test", io.Discard)

	assert.Panics(t, func() { fs.bindEnv("ADDRESS", "a") })
}

func TestApplyEnvMarksFlagAsSet(t *testing.T) {
	var addr string

	fs := newFlagSet("test", io.Discard)
	fs.stringVar(&addr, "a", "ADDRESS", addressUsage)

	require.NoError(t, fs.parse(nil, envMap(map[string]string{"ADDRESS": "env:1"})))

	var visited []string
	fs.Visit(func(f *flag.Flag) { visited = append(visited, f.Name) })

	assert.Equal(t, "env:1", addr)
	assert.Equal(t, []string{"a"}, visited)
}

func TestSecondsValueSet(t *testing.T) {
	maxStr := strconv.FormatInt(maxSeconds, 10)
	overMax := strconv.FormatInt(maxSeconds+1, 10)

	tests := []struct {
		name    string
		in      string
		want    time.Duration
		wantErr error
	}{
		{"одна секунда", "1", time.Second, nil},
		{"максимум без переполнения", maxStr, time.Duration(maxSeconds) * time.Second, nil},
		{"не число", "abc", 0, errNotSeconds},
		{"с единицами измерения", "10s", 0, errNotSeconds},
		{"дробное", "1.5", 0, errNotSeconds},
		{"пустое", "", 0, errNotSeconds},
		{"ноль", "0", 0, errNotPositive},
		{"отрицательное", "-1", 0, errNotPositive},
		{"переполнение Duration", overMax, 0, errTooManySeconds},
		{"переполнение Duration при 9999999999", "9999999999", 0, errTooManySeconds},
		{"переполнение int64", "99999999999999999999", 0, errTooManySeconds},
		{"отрицательное переполнение int64", "-99999999999999999999", 0, errNotPositive},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var v secondsValue

			err := v.Set(tt.in)

			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				assert.Zero(t, v, "значение не должно меняться при ошибке")
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, time.Duration(v))
		})
	}
}
