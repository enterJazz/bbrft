package compression

type CompressionAlgorithm interface {
	GetChunkSize() int
	Compress(chunk []byte) []byte
	Decompress(chunk []byte) []byte
}
