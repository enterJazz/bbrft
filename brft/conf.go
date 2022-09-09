package brft

import (
	"time"

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

	// read timeout for BTP connection while trying to read stream messages (client only)
	activeStreamTimeout time.Duration

	BtpOptions btp.ConnOptions
	BtpLogger  *zap.Logger
}

func (c ConnOptions) Clone(l *zap.Logger) ConnOptions {
	return ConnOptions{
		ignoreChecksumMissmatch: c.ignoreChecksumMissmatch,
		compressionOptions:      c.compressionOptions,
		chunkSizeFactor:         c.chunkSizeFactor,
		activeStreamTimeout:     c.activeStreamTimeout,
		BtpOptions:              c.BtpOptions.Clone(l),
	}
}

func NewDefaultOptions(l *zap.Logger, useCompression bool) ConnOptions {

	var comprOpts []messages.CompressionReqHeaderAlgorithm
	if useCompression {
		comprOpts = []messages.CompressionReqHeaderAlgorithm{messages.CompressionReqHeaderAlgorithmGzip}
	} else {
		comprOpts = nil
	}

	return ConnOptions{
		ignoreChecksumMissmatch: false,
		compressionOptions:      comprOpts,

		chunkSizeFactor: 0,

		activeStreamTimeout: time.Second * 20,

		BtpOptions: *btp.NewDefaultOptions(l),
	}
}

func (c ConnOptions) GetPreferredCompression() *messages.CompressionReqHeaderAlgorithm {
	if c.compressionOptions == nil || len(c.compressionOptions) == 0 {
		return nil
	}

	return &c.compressionOptions[0]
}
