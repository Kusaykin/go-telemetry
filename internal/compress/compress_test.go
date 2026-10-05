package compress_test

import (
	"bytes"
	"compress/gzip"
	"io"
	"testing"

	"github.com/Kusaykin/go-telemetry/internal/compress"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompress(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{"пустые данные", []byte{}},
		{"json", []byte(`{"id":"Alloc","type":"gauge","value":12.5}`)},
		{"большой объём", bytes.Repeat([]byte("metric "), 10000)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gz, err := compress.Compress(tt.data)
			require.NoError(t, err)

			zr, err := gzip.NewReader(bytes.NewReader(gz))
			require.NoError(t, err)
			got, err := io.ReadAll(zr)
			require.NoError(t, err)

			assert.Equal(t, tt.data, got)
		})
	}
}
