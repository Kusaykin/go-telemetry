package middleware

import (
	"compress/gzip"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/Kusaykin/go-telemetry/internal/compress"
)

const encodingGzip = "gzip"

var compressibleTypes = map[string]bool{
	"application/json": true,
	"text/html":        true,
}

type gzipResponseWriter struct {
	http.ResponseWriter
	zw          *gzip.Writer
	head        bool
	wroteHeader bool
}

func (w *gzipResponseWriter) WriteHeader(code int) {
	if w.wroteHeader {
		w.ResponseWriter.WriteHeader(code)
		return
	}
	w.wroteHeader = true

	h := w.Header()
	mediaType, _, _ := mime.ParseMediaType(h.Get("Content-Type"))
	if compressibleTypes[mediaType] {
		h.Add("Vary", "Accept-Encoding")

		if bodyAllowed(code) && !w.head && h.Get("Content-Encoding") == "" {
			h.Set("Content-Encoding", encodingGzip)
			h.Del("Content-Length")
			w.zw = compress.GetWriter(w.ResponseWriter)
		}
	}

	w.ResponseWriter.WriteHeader(code)
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	if w.zw == nil {
		return w.ResponseWriter.Write(b)
	}

	return w.zw.Write(b)
}

func (w *gzipResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *gzipResponseWriter) Close() error {
	if w.zw == nil {
		return nil
	}

	err := w.zw.Close()
	compress.PutWriter(w.zw)
	w.zw = nil

	return err
}

func Gzip(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hasToken(r.Header.Get("Content-Encoding"), encodingGzip) &&
			r.Body != nil && r.Body != http.NoBody && r.ContentLength != 0 {
			zr, err := gzip.NewReader(r.Body)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			defer zr.Close()

			r.Body = zr
			r.ContentLength = -1
			r.Header.Del("Content-Encoding")
			r.Header.Del("Content-Length")
		}

		if acceptsEncoding(r.Header.Get("Accept-Encoding"), encodingGzip) {
			gw := &gzipResponseWriter{ResponseWriter: w, head: r.Method == http.MethodHead}
			defer gw.Close()
			w = gw
		}

		next.ServeHTTP(w, r)
	})
}

func hasToken(header, token string) bool {
	for part := range strings.SplitSeq(header, ",") {
		value, _, _ := strings.Cut(part, ";")
		if strings.EqualFold(strings.TrimSpace(value), token) {
			return true
		}
	}

	return false
}

func acceptsEncoding(header, token string) bool {
	for part := range strings.SplitSeq(header, ",") {
		value, params, _ := strings.Cut(part, ";")
		if strings.EqualFold(strings.TrimSpace(value), token) {
			return qValue(params) > 0
		}
	}

	return false
}

func qValue(params string) float64 {
	for param := range strings.SplitSeq(params, ";") {
		name, value, _ := strings.Cut(param, "=")
		if !strings.EqualFold(strings.TrimSpace(name), "q") {
			continue
		}
		q, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil {
			return 0
		}
		return q
	}

	return 1
}

func bodyAllowed(code int) bool {
	return code >= http.StatusOK && code != http.StatusNoContent && code != http.StatusNotModified
}
