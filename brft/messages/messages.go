package messages

import (
	"io"

	"golang.org/x/crypto/cryptobyte"
)

// TODO: Define message contents - make sure to
//		- not include the length field length when we use it anywhere
//		- use the Builder from golang.org/x/crypto/cryptobyte

type BRFTMessage interface {
	Marshal() ([]byte, error)
	Unmarshal([]byte) error
	GetLength(io.Reader) int
}

// ProcolType defines the protocol type of the packet
type ProtocolType uint8

const (
	ProtocolTypeBRFTv0 ProtocolType = (iota + 1) << 5 // 0010 0000
	ProtocolTypeBRFTv1                                // 0100 0000
	// 0110 0000 ...

	ProtocolTypeMask ProtocolType = 0b11100000
)

// MessageType defines the type of message within the given protocol
type MessageType uint8

const (
	MessageTypeFileReq MessageType = iota + 1
	MessageTypeFileResp
	MessageTypeData
	MessageTypeStartTransmission
	MessageTypeStopTransmission
	MessageTypeMetaDataReq
	MessageTypeMetaDataResp

	MessageTypeMask MessageType = 0b00011111
)

type PacketHeader struct {
	ProtocolType ProtocolType
	MessageType  MessageType
}

func AddUint64(b *cryptobyte.Builder, v uint64) {
	b.AddBytes([]byte{
		byte(v >> 56), byte(v >> 48), byte(v >> 40), byte(v >> 32),
		byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v),
	})
}

func ReadUint64(s *cryptobyte.String, out *uint64) bool {
	v := make([]byte, 0, 8)
	if !s.ReadBytes(&v, 8) {
		return false
	}
	*out = uint64(v[0])<<56 | uint64(v[1])<<48 | uint64(v[2])<<40 | uint64(v[3])<<32 |
		uint64(v[4])<<24 | uint64(v[5])<<16 | uint64(v[6])<<8 | uint64(v[7])
	return true
}
