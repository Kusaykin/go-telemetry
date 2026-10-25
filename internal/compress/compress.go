package compress

import (
	"bytes"
	"compress/gzip"
	"io"
	"sync"
)

var writerPool = sync.Pool{
	New: func() any {
		zw, _ := gzip.NewWriterLevel(io.Discard, gzip.BestSpeed)
		return zw
	},
}

func GetWriter(w io.Writer) *gzip.Writer {
	zw := writerPool.Get().(*gzip.Writer)
	zw.Reset(w)
	return zw
}

func PutWriter(zw *gzip.Writer) {
	zw.Reset(io.Discard)
	writerPool.Put(zw)
}

func Compress(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	zw := GetWriter(&buf)
	defer PutWriter(zw)

	if _, err := zw.Write(data); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
