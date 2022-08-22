package compression

import "go.uber.org/zap"

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
	// TODO: @robert implement me
	return nil, nil
}
func (c *GzipCompressor) Decompress(chunk []byte) ([]byte, error) {
	// TODO: @robert implement me
	return nil, nil
}
