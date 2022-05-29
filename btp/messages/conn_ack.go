package messages

import (
	"io"

	"golang.org/x/crypto/cryptobyte"
)

type ConnAck struct {
	PacketHeader

	// packet size selected by server
	ActualPacketSize uint16

	Raw []byte
}

func (p *ConnAck) Size() uint {
	return HeaderSize + ConnAckSize
}

func (p *ConnAck) GetHeader() PacketHeader {
	return p.PacketHeader
}

// Marshal encodes a given ConnAck message into transport format
func (p *ConnAck) Marshal() ([]byte, error) {
	b := createPacketBuilder(p)

	// packet encoding after header
	b.AddUint16(p.ActualPacketSize)
	return b.Bytes()
}

// Unmarshal decodes a given ConnAck message from transport format given header was already read and reader cursor is past it
func (p *ConnAck) Unmarshal(h PacketHeader, r io.Reader) error {
	buf, err := readPacketBuffer(r, p)
	if err != nil {
		return err
	}

	// as debug save butes on package
	p.Raw = buf

	p.PacketHeader = h
	b := cryptobyte.String(buf)

	ok := b.ReadUint16(&p.ActualPacketSize)
	if !ok {
		return NewDecodeError("ActualPacketSize")
	}

	return nil
}