package btp

import (
	"errors"
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

	// packet sizes without header
	AckSize  = 2
	ConnSize = 2

	// masks for decoding fields
	HeaderProtocolVersionMask = 0xE0
	HeaderMessageTypeMask     = 0x1F
)

type PacketHeader struct {
	// ProtocolVersion(3bit) + MessageType(5bit)
	// will be encoded as
	// -----------------------
	// |  3bit  |    5bit    |
	// ----------------------
	ProtocolType ProtocolVersion
	MessageType  MessageType

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

func marshalHeader(b *cryptobyte.Builder, h PacketHeader) {
	t := uint8(h.ProtocolType) | uint8(h.MessageType)
	b.AddUint8(t)
	b.AddUint16(h.SeqNr)
}

func ReadHeader(r io.Reader) (h PacketHeader, err error) {
	buf := make([]byte, HeaderSize)
	n, err := r.Read(buf)

	if err != nil {
		return
	}

	if n != HeaderSize {
		err = io.ErrUnexpectedEOF
		return
	}

	return ParseHeader(buf)
}

func ParseHeader(buf []byte) (h PacketHeader, err error) {

	s := cryptobyte.String(buf)

	var t uint8
	ok := s.ReadUint8(&t)
	if !ok {
		err = errors.New("failed to read protocol type")
		return
	}

	h.ProtocolType = ProtocolVersion(t & HeaderProtocolVersionMask) // top 3 bits
	h.MessageType = MessageType(t & HeaderMessageTypeMask)          // bottom 5 bits

	ok = s.ReadUint16(&h.SeqNr)
	if !ok {
		err = errors.New("failed to read sequence number")
		return
	}

	return
}

func (p *Conn) Marshal() ([]byte, error) {
	buf := make([]byte, 0, HeaderSize+AckSize)
	println("buf:", buf)
	b := cryptobyte.NewFixedBuilder(buf)
	marshalHeader(b, p.PacketHeader)
	b.AddUint16(p.MaxPacketSize)
	return b.Bytes()
}

func (p *Conn) Unmarshal(h PacketHeader, r io.Reader) error {
	// read packet sans header
	buf := make([]byte, ConnSize)
	n, err := r.Read(buf)
	if err != nil {
		return err
	}
	if n != ConnSize {
		return io.ErrUnexpectedEOF
	}

	p.PacketHeader = h
	b := cryptobyte.String(buf)

	ok := b.ReadUint16(&p.MaxPacketSize)
	if !ok {
		return errors.New("failed to read max packet size")
	}

	return nil
}
