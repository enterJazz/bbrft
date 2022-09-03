package compression

import (
	"bytes"
	"compress/gzip"
	"io"

	"go.uber.org/zap"
)

type GzipCompressor struct {
	l *zap.Logger
}

func NewGzipCompressor(
	l *zap.Logger,
) *GzipCompressor {
	return &GzipCompressor{
		l: l,
	}
}

func (c *GzipCompressor) Compress(chunk []byte) ([]byte, error) {
	var buf bytes.Buffer
	reader := bytes.NewReader(chunk)
	zw := gzip.NewWriter(&buf)

	_, err := io.Copy(zw, reader)
	if err != nil {
		return nil, err
	}

	if err := zw.Close(); err != nil {
		c.l.Error(err.Error())
	}

	return buf.Bytes(), err
}
func (c *GzipCompressor) Decompress(chunk []byte) ([]byte, error) {
	var buf bytes.Buffer
	reader := bytes.NewReader(chunk)
	zr, err := gzip.NewReader(reader)
	if err != nil {
		c.l.Error(err.Error())
		return nil, err
	}

	if _, err = io.Copy(&buf, zr); err != nil {
		c.l.Error(err.Error())
		return nil, err
	}

	if err := zr.Close(); err != nil {
		c.l.Error(err.Error())
	}

	return buf.Bytes(), err
}
