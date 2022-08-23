package compression

import (
	"bytes"
	"go.uber.org/zap"
	"testing"
)

func TestDeCompress(t *testing.T) {
	compressor := GzipCompressor{l: zap.L()}
	raw := []byte("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")

	compressed, compressErr := compressor.Compress(raw)
	if compressErr != nil {
		t.Errorf("Expected compressed, received %v", compressErr)
	}
	if len(compressed) >= len(raw) {
		t.Errorf("Expected len compressed < len raw, got: %v !< %v", len(compressed), len(raw))
	}

	decompressed, decompressErr := compressor.Decompress(compressed)
	if decompressErr != nil {
		t.Errorf("Expected decompressed, received %v", decompressErr)
	}

	if !bytes.Equal(decompressed, raw) {
		t.Errorf("Expected decompressed equal to raw, got decompressed: %v, raw: %v", decompressed, raw)
	}
}
