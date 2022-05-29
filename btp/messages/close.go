package messages

import (
	"io"

	"golang.org/x/crypto/cryptobyte"
)

type CloseResons uint8

const (
	CloseResonsConn    CloseResons = iota + 1
	CloseResonsConnAck CloseResons = iota + 1
	CloseResonsAck     CloseResons = iota + 1
	CloseResonsData    CloseResons = iota + 1
	CloseResonsClose   CloseResons = iota + 1
)

type Close struct {
	PacketHeader

	// reason why the connection was closed
	Reason CloseResons

	Raw []byte
}

func (p *Close) Size() uint {
	return HeaderSize + CloseSize
}

func (p *Close) GetHeader() PacketHeader {
	return p.PacketHeader
}

// Marshal encodes a given Close message into transport format
func (p *Close) Marshal() ([]byte, error) {
	b := createPacketBuilder(p)

	// packet encoding after header
	b.AddUint8(uint8(p.Reason))

	return b.Bytes()
}

// Unmarshal decodes a given Close message from transport format given header was already read and reader cursor is past it
func (p *Close) Unmarshal(h PacketHeader, r io.Reader) error {
	buf, err := readPacketBuffer(r, p)
	if err != nil {
		return err
	}

	// as debug save butes on package
	p.Raw = buf

	p.PacketHeader = h

	b := cryptobyte.String(buf)
	ok := b.ReadUint8((*uint8)(&p.Reason))
	if !ok {
		return NewDecodeError("Reason")
	}

	return nil
}
