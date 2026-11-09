package middleware

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Kusaykin/go-telemetry/internal/sign"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testKey      = "secret"
	testReqBody  = `{"id":"PollCount","type":"counter","delta":1}`
	testRespBody = `{"id":"PollCount","type":"counter","delta":5}`
)

func echoHandler(called *bool, got *string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*called = true
		b, _ := io.ReadAll(r.Body)
		*got = string(b)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, testRespBody)
	})
}

func signedRoute(key []byte, next http.Handler) http.Handler {
	return SignResponse(key)(VerifyRequest(key)(next))
}

func TestVerifyRequest(t *testing.T) {
	key := []byte(testKey)

	tests := []struct {
		name       string
		hash       string
		wantStatus int
		wantCalled bool
	}{
		{"верная подпись", sign.Sum([]byte(testReqBody), key), http.StatusOK, true},
		{"без подписи", "", http.StatusBadRequest, false},
		{"неверная подпись", sign.Sum([]byte(testReqBody), []byte("other")), http.StatusBadRequest, false},
		{"подпись не hex", "not-hex", http.StatusBadRequest, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				called bool
				got    string
			)

			req := httptest.NewRequest(http.MethodPost, "/update/", strings.NewReader(testReqBody))
			if tt.hash != "" {
				req.Header.Set(sign.Header, tt.hash)
			}
			rec := httptest.NewRecorder()

			signedRoute(key, echoHandler(&called, &got)).ServeHTTP(rec, req)

			assert.Equal(t, tt.wantStatus, rec.Code)
			assert.Equal(t, tt.wantCalled, called)
			assert.True(t, sign.Verify(rec.Body.Bytes(), key, rec.Header().Get(sign.Header)),
				"любой ответ, включая отказ, должен быть подписан")
			if !tt.wantCalled {
				return
			}

			assert.Equal(t, testReqBody, got, "обработчик должен получить исходное тело")
			assert.Equal(t, testRespBody, rec.Body.String())
			assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
		})
	}
}

func TestVerifyRequestWithoutBody(t *testing.T) {
	key := []byte(testKey)

	tests := []struct {
		name       string
		hash       string
		wantStatus int
		wantCalled bool
	}{
		{"без подписи", "", http.StatusBadRequest, false},
		{"подпись пустого тела", sign.Sum(nil, key), http.StatusOK, true},
		{"неверная подпись", sign.Sum([]byte(testReqBody), key), http.StatusBadRequest, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				called bool
				got    string
			)

			req := httptest.NewRequest(http.MethodPost, "/update/counter/PollCount/1", nil)
			if tt.hash != "" {
				req.Header.Set(sign.Header, tt.hash)
			}
			rec := httptest.NewRecorder()

			signedRoute(key, echoHandler(&called, &got)).ServeHTTP(rec, req)

			assert.Equal(t, tt.wantStatus, rec.Code)
			assert.Equal(t, tt.wantCalled, called)
			assert.True(t, sign.Verify(rec.Body.Bytes(), key, rec.Header().Get(sign.Header)))
		})
	}
}

func TestSignResponseWithoutRequestSignature(t *testing.T) {
	key := []byte(testKey)

	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodPost} {
		t.Run(method, func(t *testing.T) {
			var (
				called bool
				got    string
			)

			rec := httptest.NewRecorder()
			SignResponse(key)(echoHandler(&called, &got)).
				ServeHTTP(rec, httptest.NewRequest(method, "/value/counter/PollCount", nil))

			assert.Equal(t, http.StatusOK, rec.Code)
			assert.True(t, called)
			assert.True(t, sign.Verify(rec.Body.Bytes(), key, rec.Header().Get(sign.Header)))
		})
	}
}

func TestSignResponsePanicNotReportedAsSuccess(t *testing.T) {
	h := SignResponse([]byte(testKey))(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("partial"))
		panic("storage failure")
	}))

	req := httptest.NewRequest(http.MethodPost, "/updates/", strings.NewReader(testReqBody))
	rec := httptest.NewRecorder()

	assert.PanicsWithValue(t, "storage failure", func() { h.ServeHTTP(rec, req) })
	assert.Empty(t, rec.Body.String(), "после паники тело не должно отправляться")
	assert.Empty(t, rec.Header().Get(sign.Header), "после паники подпись не должна отправляться")
}

func TestSignResponseKeepsStatusCode(t *testing.T) {
	h := SignResponse([]byte(testKey))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/value/gauge/x", nil))

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.True(t, sign.Verify(rec.Body.Bytes(), []byte(testKey), rec.Header().Get(sign.Header)))
}

func TestSignResponseEmptyResponse(t *testing.T) {
	h := SignResponse([]byte(testKey))(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/update/gauge/x/1", nil))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, sign.Sum(nil, []byte(testKey)), rec.Header().Get(sign.Header))
}

// Gzip снаружи, подпись внутри: подпись запроса и ответа считается от несжатых данных.
func TestSignBehindGzip(t *testing.T) {
	key := []byte(testKey)

	var (
		called bool
		got    string
	)
	h := Gzip(signedRoute(key, echoHandler(&called, &got)))

	var compressed bytes.Buffer
	zw := gzip.NewWriter(&compressed)
	_, err := zw.Write([]byte(testReqBody))
	require.NoError(t, err)
	require.NoError(t, zw.Close())

	req := httptest.NewRequest(http.MethodPost, "/update/", &compressed)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set(sign.Header, sign.Sum([]byte(testReqBody), key))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, called)
	assert.Equal(t, testReqBody, got)
	assert.Equal(t, "gzip", rec.Header().Get("Content-Encoding"))

	zr, err := gzip.NewReader(rec.Body)
	require.NoError(t, err)
	plain, err := io.ReadAll(zr)
	require.NoError(t, err)

	assert.Equal(t, testRespBody, string(plain))
	assert.True(t, sign.Verify(plain, key, rec.Header().Get(sign.Header)),
		"подпись ответа должна считаться от несжатого тела")
}
