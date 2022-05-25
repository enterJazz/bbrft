package shared

import (
	"crypto/sha256"
	"io"
	"os"
)

const ChecksumSize = sha256.Size

func ComputeChecksum(f *os.File) ([]byte, error) {
	// compute the hash in a streaming fashion
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}
