package middleware_test

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/Kusaykin/go-telemetry/internal/compress"
	"github.com/Kusaykin/go-telemetry/internal/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func gunzip(t *testing.T, data []byte) string {
	t.Helper()

	zr, err := gzip.NewReader(bytes.NewReader(data))
	require.NoError(t, err)
	body, err := io.ReadAll(zr)
	require.NoError(t, err)

	return string(body)
}

func TestGzipRequest(t *testing.T) {
	const payload = `{"id":"Alloc","type":"gauge","value":1}`

	gz, err := compress.Compress([]byte(payload))
	require.NoError(t, err)

	tests := []struct {
		name       string
		body       []byte
		encoding   string
		wantStatus int
		wantBody   string
	}{
		{"сжатое тело", gz, "gzip", http.StatusOK, payload},
		{"несжатое тело", []byte(payload), "", http.StatusOK, payload},
		{"битый gzip", []byte("not gzip"), "gzip", http.StatusBadRequest, ""},
		// распаковывать нечего: запрос уходит в хендлер как есть
		{"пустое тело с Content-Encoding", nil, "gzip", http.StatusOK, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got string
			h := middleware.Gzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if len(tt.body) > 0 {
					assert.Empty(t, r.Header.Get("Content-Encoding"))
				}
				body, err := io.ReadAll(r.Body)
				assert.NoError(t, err)
				got = string(body)
			}))

			var body io.Reader = http.NoBody
			if tt.body != nil {
				body = bytes.NewReader(tt.body)
			}
			req := httptest.NewRequest(http.MethodPost, "/update", body)
			if tt.encoding != "" {
				req.Header.Set("Content-Encoding", tt.encoding)
			}
			rec := httptest.NewRecorder()

			h.ServeHTTP(rec, req)

			assert.Equal(t, tt.wantStatus, rec.Code)
			assert.Equal(t, tt.wantBody, got)
		})
	}
}

func TestGzipResponse(t *testing.T) {
	const body = "hello"

	writeTyped := func(contentType string, status int, body string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if contentType != "" {
				w.Header().Set("Content-Type", contentType)
			}
			w.WriteHeader(status)
			if body != "" {
				_, _ = w.Write([]byte(body))
			}
		}
	}

	tests := []struct {
		name           string
		handler        http.HandlerFunc
		method         string
		acceptEncoding string
		wantStatus     int
		wantGzip       bool
		wantVary       bool
		wantBody       string
	}{
		{
			name:           "json сжимается",
			handler:        writeTyped("application/json", http.StatusOK, body),
			acceptEncoding: "gzip",
			wantStatus:     http.StatusOK,
			wantGzip:       true,
			wantVary:       true,
			wantBody:       body,
		},
		{
			name:           "html с charset сжимается",
			handler:        writeTyped("text/html; charset=utf-8", http.StatusOK, body),
			acceptEncoding: "deflate, gzip;q=0.8",
			wantStatus:     http.StatusOK,
			wantGzip:       true,
			wantVary:       true,
			wantBody:       body,
		},
		{
			name: "json без явного WriteHeader сжимается",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(body))
			},
			acceptEncoding: "gzip",
			wantStatus:     http.StatusOK,
			wantGzip:       true,
			wantVary:       true,
			wantBody:       body,
		},
		{
			name:           "json с пустым телом — валидный gzip",
			handler:        writeTyped("application/json", http.StatusOK, ""),
			acceptEncoding: "gzip",
			wantStatus:     http.StatusOK,
			wantGzip:       true,
			wantVary:       true,
			wantBody:       "",
		},
		{
			name:           "text/plain не сжимается",
			handler:        writeTyped("text/plain; charset=utf-8", http.StatusOK, body),
			acceptEncoding: "gzip",
			wantStatus:     http.StatusOK,
			wantBody:       body,
		},
		{
			name:           "gzip;q=0 — клиент отказался от сжатия",
			handler:        writeTyped("application/json", http.StatusOK, body),
			acceptEncoding: "gzip;q=0, identity",
			wantStatus:     http.StatusOK,
			wantBody:       body,
		},
		{
			name:           "gzip; q=0.000 с пробелом — тоже отказ",
			handler:        writeTyped("application/json", http.StatusOK, body),
			acceptEncoding: "gzip; q=0.000",
			wantStatus:     http.StatusOK,
			wantBody:       body,
		},
		{
			name:       "клиент без Accept-Encoding",
			handler:    writeTyped("application/json", http.StatusOK, body),
			wantStatus: http.StatusOK,
			wantBody:   body,
		},
		{
			name:           "204 не сжимается",
			handler:        writeTyped("application/json", http.StatusNoContent, ""),
			acceptEncoding: "gzip",
			wantStatus:     http.StatusNoContent,
			wantVary:       true,
		},
		{
			name:           "404 без тела и типа",
			handler:        writeTyped("", http.StatusNotFound, ""),
			acceptEncoding: "gzip",
			wantStatus:     http.StatusNotFound,
		},
		{
			name:           "хендлер ничего не пишет",
			handler:        func(w http.ResponseWriter, r *http.Request) {},
			acceptEncoding: "gzip",
			wantStatus:     http.StatusOK,
		},
		{
			name:           "HEAD не сжимается",
			handler:        writeTyped("application/json", http.StatusOK, ""),
			method:         http.MethodHead,
			acceptEncoding: "gzip",
			wantStatus:     http.StatusOK,
			wantVary:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			method := tt.method
			if method == "" {
				method = http.MethodGet
			}
			req := httptest.NewRequest(method, "/", nil)
			if tt.acceptEncoding != "" {
				req.Header.Set("Accept-Encoding", tt.acceptEncoding)
			}
			rec := httptest.NewRecorder()

			middleware.Gzip(tt.handler).ServeHTTP(rec, req)

			assert.Equal(t, tt.wantStatus, rec.Code)
			assert.Empty(t, rec.Header().Get("Content-Length"))
			if tt.wantVary {
				assert.Equal(t, "Accept-Encoding", rec.Header().Get("Vary"))
			} else {
				assert.Empty(t, rec.Header().Get("Vary"))
			}

			if tt.wantGzip {
				assert.Equal(t, "gzip", rec.Header().Get("Content-Encoding"))
				assert.Equal(t, tt.wantBody, gunzip(t, rec.Body.Bytes()))
			} else {
				assert.Empty(t, rec.Header().Get("Content-Encoding"))
				assert.Equal(t, tt.wantBody, rec.Body.String())
			}
		})
	}
}

func TestGzipConcurrent(t *testing.T) {
	h := middleware.Gzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))

	var wg sync.WaitGroup
	for i := range 50 {
		wg.Go(func() {
			payload := strings.Repeat("x", i*100)
			gz, err := compress.Compress([]byte(payload))
			if !assert.NoError(t, err) {
				return
			}

			req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(gz))
			req.Header.Set("Content-Encoding", "gzip")
			req.Header.Set("Accept-Encoding", "gzip")
			rec := httptest.NewRecorder()

			h.ServeHTTP(rec, req)

			zr, err := gzip.NewReader(rec.Body)
			if !assert.NoError(t, err) {
				return
			}
			got, err := io.ReadAll(zr)
			assert.NoError(t, err)
			assert.Equal(t, payload, string(got))
		})
	}
	wg.Wait()
}
