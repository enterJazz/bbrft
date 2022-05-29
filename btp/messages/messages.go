package messages

import (
	"errors"
	"io"
	"net"

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

const (
	HeaderSize = 3

	// packet sizes without header
	AckSize     = 2
	ConnSize    = 2
	ConnAckSize = 2
	CloseSize   = 1
	DataSize    = 3 // without payload + full size is DataSize + Length

	// masks for decoding fields
	HeaderProtocolVersionMask = 0xE0
	HeaderMessageTypeMask     = 0x1F
)

type Codable interface {
	Marshal() ([]byte, error)
	Unmarshal(h PacketHeader, r io.Reader) error
	Size() uint
	GetHeader() PacketHeader
}

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

// reads in the entire message from a io.Reader
func readPacketBuffer(r io.Reader, p Codable) (buf []byte, err error) {
	buf = make([]byte, p.Size()-HeaderSize)
	n, err := r.Read(buf)

	if err != nil {
		return
	}

	if n != len(buf) {
		err = io.ErrUnexpectedEOF
		return
	}

	return
}

func createPacketBuilder(p Codable) (b *cryptobyte.Builder) {
	buf := make([]byte, 0, p.Size())
	b = cryptobyte.NewFixedBuilder(buf)
	marshalHeader(b, p.GetHeader())

	return
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

func ReadHeaderFrom(r net.PacketConn) (h PacketHeader, addr net.Addr, err error) {
	buf := make([]byte, HeaderSize)
	n, addr, err := r.ReadFrom(buf)

	if err != nil {
		return
	}

	if n != HeaderSize {
		err = io.ErrUnexpectedEOF
		return
	}

	h, err = ParseHeader(buf)

	return
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