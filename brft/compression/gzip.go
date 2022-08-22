package compression

import (
	"bytes"
	"compress/gzip"
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
	zw := gzip.NewWriter(&buf)
	_, err := zw.Write(chunk)
	if err != nil {
		c.l.Error(err.Error())
		return nil, err
	}
	err = zw.Close()
	if err != nil {
		c.l.Error(err.Error())
		return nil, err
	}
	return buf.Bytes(), err
}
func (c *GzipCompressor) Decompress(chunk []byte) ([]byte, error) {
	var buf bytes.Buffer
	zr, err := gzip.NewReader(&buf)
	if err != nil {
		c.l.Error(err.Error())
		return nil, err
	}
	_, err = zr.Read(chunk)
	if err != nil {
		c.l.Error(err.Error())
		return nil, err
	}
	return buf.Bytes(), err
}
