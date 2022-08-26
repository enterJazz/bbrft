package compression

type Compressor interface {
	Compress(chunk []byte) ([]byte, error)
	Decompress(chunk []byte) ([]byte, error)
}
