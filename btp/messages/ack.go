package messages

import (
	"io"
)

type Ack struct {
	PacketHeader

	// Raw []byte
}

func (p *Ack) Size() uint {
	return HeaderSize + AckSize
}

func (p *Ack) GetHeader() PacketHeader {
	return p.PacketHeader
}

// Marshal encodes a given Ack message into transport format
func (p *Ack) Marshal() ([]byte, error) {
	b := createPacketBuilder(p)

	// packet encoding after header
	// TODO: encode fields here

	return b.Bytes()
}

// Unmarshal decodes a given Ack message from transport format given header was already read and reader cursor is past it
func (p *Ack) Unmarshal(h PacketHeader, r io.Reader) error {
	p.PacketHeader = h
	return nil

	// _, err := readPacketBuffer(r, p)
	// if err != nil {
	// 	return err
	// }

	// // as debug save butes on package
	// // p.Raw = buf

	// p.PacketHeader = h

	// // TODO decode fields here
	// // b := cryptobyte.String(buf)

	// return nil
}

func (p *Ack) SetSeqNr(seqNr uint16) {
	p.PacketHeader.SeqNr = seqNr
}
