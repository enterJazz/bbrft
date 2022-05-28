package messages

import "io"

// TODO: Define message contents - make sure to
//		- not include the length field length when we use it anywhere
//		- use the Builder from golang.org/x/crypto/cryptobyte

type BRFTMessage interface {
	Marshal() ([]byte, error)
	Unmarshal([]byte) error
	GetLength(io.Reader) int64
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

	MessageTypeMask MessageType = 0b00011111
)

type PacketHeader struct {
	ProtocolType ProtocolType
	MessageType  MessageType
}
