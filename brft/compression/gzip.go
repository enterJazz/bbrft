package compression

type GzipCompressor struct {
}

func NewGzipCompressor() *GzipCompressor {
	return &GzipCompressor{}
}

func (c *GzipCompressor) Compress(chunk []byte) ([]byte, error) {
	// TODO: @robert implement me
	return nil, nil
}
func (c *GzipCompressor) Decompress(chunk []byte) ([]byte, error) {
	// TODO: @robert implement me
	return nil, nil
}
