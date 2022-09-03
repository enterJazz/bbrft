package messages

import (
	"gitlab.lrz.de/bbrft/cyberbyte"
	"go.uber.org/zap"
	"golang.org/x/crypto/cryptobyte"
)

const (
	BRFTHeaderSize = 1
)

type BRFTMessage interface {
	Encode(l *zap.Logger) ([]byte, error)
	// Decode actually reads the bytes from the *cyberbyte.String and decodes them
	Decode(l *zap.Logger, s *cyberbyte.String) error

	// base size of the message without header but with all non variable length or optional fields
	baseSize() int
}

type StreamID uint16

// TODO: Maybe also create a cyberbyte.Builder
func AddUint64(b *cryptobyte.Builder, v uint64) {
	b.AddBytes([]byte{
		byte(v >> 56), byte(v >> 48), byte(v >> 40), byte(v >> 32),
		byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v),
	})
}

// TODO: Remove once everything has been ported to cyberbyte.String
func ReadUint64(s *cryptobyte.String, out *uint64) bool {
	v := make([]byte, 0, 8)
	if !s.ReadBytes(&v, 8) {
		return false
	}
	*out = uint64(v[0])<<56 | uint64(v[1])<<48 | uint64(v[2])<<40 | uint64(v[3])<<32 |
		uint64(v[4])<<24 | uint64(v[5])<<16 | uint64(v[6])<<8 | uint64(v[7])
	return true
}

func NewFixedBRFTMessageBuilder(m BRFTMessage) *cryptobyte.Builder {
	b := cryptobyte.NewFixedBuilder(make([]byte, 0, BRFTHeaderSize+m.baseSize()))
	writePacketHeader(m, b)
	return b
}

func NewFixedBRFTMessageBuilderWithExtra(m BRFTMessage, extra int) *cryptobyte.Builder {
	b := cryptobyte.NewFixedBuilder(make([]byte, 0, BRFTHeaderSize+m.baseSize()+extra))
	writePacketHeader(m, b)
	return b
}
