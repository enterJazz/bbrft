package messages

import (
	"io"

	"golang.org/x/crypto/cryptobyte"
)

type Conn struct {
	PacketHeader
	// maximal packet size acceptable by initiator/client
	MaxPacketSize uint16

	InitCwndSize uint8
	MaxCwndSize  uint8

	// TODO: figure out testing
	// Raw []byte
}

func (p *Conn) Size() uint {
	return HeaderSize + ConnSize
}

func (p *Conn) GetHeader() PacketHeader {
	return p.PacketHeader
}

// Marshal encodes a given Conn message into transport format
func (p *Conn) Marshal() ([]byte, error) {
	b := createPacketBuilder(p)

	b.AddUint16(p.MaxPacketSize)
	return b.Bytes()
}

// Unmarshal decodes a given Conn message from transport format given header was already read and reader cursor is past it
func (p *Conn) Unmarshal(h PacketHeader, r io.Reader) error {
	// read packet sans header
	buf, err := readPacketBuffer(r, p)
	if err != nil {
		return err
	}

	// p.Raw = buf

	p.PacketHeader = h
	b := cryptobyte.String(buf)

	ok := b.ReadUint16(&p.MaxPacketSize)
	if !ok {
		return NewDecodeError("MaxPacketSize")
	}

	ok = b.ReadUint8(&p.InitCwndSize)
	if !ok {
		return NewDecodeError("InitCwndSize")
	}

	ok = b.ReadUint8(&p.MaxCwndSize)
	if !ok {
		return NewDecodeError("MaxCwndSize")
	}

	return nil
}
