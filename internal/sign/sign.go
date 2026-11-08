package sign

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

const Header = "HashSHA256"

func Sum(data, key []byte) string {
	return hex.EncodeToString(mac(data, key))
}

func Verify(data, key []byte, sum string) bool {
	got, err := hex.DecodeString(sum)
	if err != nil {
		return false
	}

	return hmac.Equal(got, mac(data, key))
}

func mac(data, key []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)

	return h.Sum(nil)
}
