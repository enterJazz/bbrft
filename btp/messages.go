package btp

import (
	"io"

	"golang.org/x/crypto/cryptobyte"
)

// TODO: Define message contents - make sure to
//		- not include the length field length when we use it anywhere
//		- use the Builder from golang.org/x/crypto/cryptobyte
type ProtocolVersion uint8

const (
	ProtocolVersionBTPv0 ProtocolVersion = (iota + 1) << 5
	ProtocolVersionBTPv1 ProtocolVersion = (iota + 1) << 5
)

type MessageType uint8

const (
	MessageTypeConn    MessageType = iota + 1
	MessageTypeConnAck MessageType = iota + 1
	MessageTypeAck     MessageType = iota + 1
	MessageTypeData    MessageType = iota + 1
	MessageTypeClose   MessageType = iota + 1
)

type CloseResons uint8

const (
	CloseResonsConn    CloseResons = iota + 1
	CloseResonsConnAck CloseResons = iota + 1
	CloseResonsAck     CloseResons = iota + 1
	CloseResonsData    CloseResons = iota + 1
	CloseResonsClose   CloseResons = iota + 1
)

const (
	HeaderSize = 3
	AckSize    = 2
)

type PacketHeader struct {
	// ProtocolVersion(3bit) + MessageType(5bit)
	// will be encoded as
	// -----------------------
	// |  3bit  |    5bit    |
	// ----------------------
	ProtoclType ProtocolVersion
	MessageType MessageType

	// acked packet sequence number
	SeqNr uint16
}

type Conn struct {
	PacketHeader
	// maximal packet size acceptable by initiator/client
	MaxPacketSize uint16

	Raw []byte
}

type ConnAck struct {
	PacketHeader

	// packet size selected by server
	ActualPacketSize uint16

	Raw []byte
}

type Ack struct {
	PacketHeader

	Raw []byte
}

type Data struct {
	PacketHeader
	Flags uint8

	// should not be larger 1472 to not cause MTU
	Length uint16

	Raw []byte
}

type Close struct {
	PacketHeader

	// reason why the connection was closed
	Reason CloseResons

	Raw []byte
}

func (p *Conn) Marshal() ([]byte, error) {
	buf := make([]byte, HeaderSize+AckSize)
	b := cryptobyte.NewFixedBuilder(buf)
	marshalHeader(b, p.PacketHeader)
	b.AddUint16(p.MaxPacketSize)
	return b.Bytes()
}

func marshalHeader(b *cryptobyte.Builder, h PacketHeader) {
	t := uint8(h.ProtoclType) | uint8(h.MessageType)
	b.AddUint8(t)
	b.AddUint16(h.SeqNr)
}

func readHeader(r io.Reader) (h PacketHeader, err error) {
	buf := make([]byte, HeaderSize)
	n, err := r.Read(buf[:HeaderSize])

	if err != nil {
		return
	}

	if n != HeaderSize {
		err = io.ErrUnexpectedEOF
		return
	}

	// TODO: wip implement reader
	// var b cryptobyte.String
	// t := b.ReadUint8(buf[0])
	return
}
