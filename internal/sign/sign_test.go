package sign

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSum(t *testing.T) {
	got := Sum([]byte("what do ya want for nothing?"), []byte("Jefe"))
	assert.Equal(t, "5bdcc146bf60754e6a042426089575c75a003f089d2739839dec58b964ec3843", got)
}

func TestVerify(t *testing.T) {
	data := []byte(`{"id":"a","type":"counter","delta":1}`)
	key := []byte("secret")
	sum := Sum(data, key)

	tests := []struct {
		name string
		data []byte
		key  []byte
		sum  string
		want bool
	}{
		{name: "valid", data: data, key: key, sum: sum, want: true},
		{name: "wrong key", data: data, key: []byte("other"), sum: sum, want: false},
		{name: "tampered data", data: []byte(`{}`), key: key, sum: sum, want: false},
		{name: "invalid hex", data: data, key: key, sum: "zz", want: false},
		{name: "empty sum", data: data, key: key, sum: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, Verify(tt.data, tt.key, tt.sum))
		})
	}
}
