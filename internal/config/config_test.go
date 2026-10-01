package config

import (
	"errors"
	"flag"
	"testing"

	"github.com/stretchr/testify/assert"
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
