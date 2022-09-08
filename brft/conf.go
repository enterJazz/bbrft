package brft

import (
	"gitlab.lrz.de/bbrft/brft/messages"
	"gitlab.lrz.de/bbrft/btp"
	"go.uber.org/zap"
)

type ConnOptions struct {
	ignoreChecksumMissmatch bool

	// supported compression options
	// ordered from favorite to least favorite
	compressionOptions []messages.CompressionReqHeaderAlgorithm

	chunkSizeFactor uint8

	btpOptions btp.ConnOptions
}

func NewDefaultOptions(l *zap.Logger) ConnOptions {

	return ConnOptions{
		ignoreChecksumMissmatch: false,
		compressionOptions:      []messages.CompressionReqHeaderAlgorithm{messages.CompressionReqHeaderAlgorithmGzip},

		chunkSizeFactor: 0,

		btpOptions: *btp.NewDefaultOptions(l),
	}
}

func (c ConnOptions) GetPreferredCompression() messages.CompressionReqHeaderAlgorithm {
	if c.compressionOptions == nil || len(c.compressionOptions) == 0 {
		// FIXME: we should have a different default here
		return messages.CompressionReqHeaderAlgorithmReserved
	}

	return c.compressionOptions[0]
}
