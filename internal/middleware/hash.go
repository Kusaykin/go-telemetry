package middleware

import (
	"bytes"
	"io"
	"net/http"

	"github.com/Kusaykin/go-telemetry/internal/sign"
)

type hashResponseWriter struct {
	http.ResponseWriter
	buf    bytes.Buffer
	status int
}

func (w *hashResponseWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
}

func (w *hashResponseWriter) Write(b []byte) (int, error) {
	return w.buf.Write(b)
}

func (w *hashResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *hashResponseWriter) flush(key []byte) error {
	if w.status == 0 {
		w.status = http.StatusOK
	}

	w.ResponseWriter.Header().Set(sign.Header, sign.Sum(w.buf.Bytes(), key))
	w.ResponseWriter.WriteHeader(w.status)
	_, err := w.ResponseWriter.Write(w.buf.Bytes())

	return err
}

func SignResponse(key []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hw := &hashResponseWriter{ResponseWriter: w}

			next.ServeHTTP(hw, r)
			_ = hw.flush(key)
		})
	}
}

func VerifyRequest(key []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sum := r.Header.Get(sign.Header)
			if sum == "" {
				http.Error(w, "missing "+sign.Header, http.StatusBadRequest)
				return
			}

			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "cannot read request body", http.StatusBadRequest)
				return
			}

			if !sign.Verify(body, key, sum) {
				http.Error(w, "invalid "+sign.Header, http.StatusBadRequest)
				return
			}

			r.Body = io.NopCloser(bytes.NewReader(body))

			next.ServeHTTP(w, r)
		})
	}
}
