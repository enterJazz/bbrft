package messages

import (
	"io"
	"math"

	"golang.org/x/crypto/cryptobyte"
)

type Data struct {
	PacketHeader

	// should not be larger 1472-HeaderSize+F64`lags to not cause MTU
	Payload []byte

	// Raw []byte
}

func (p *Data) Size() uint {
	return HeaderSize + DataSize + uint(len(p.Payload))
}

func (p *Data) GetHeader() PacketHeader {
	return p.PacketHeader
}

func (p *Data) SetSeqNr(seqNr uint16) {
	p.PacketHeader.SeqNr = seqNr
}

// Marshal encodes a given Data message into transport format
func (p *Data) Marshal() ([]byte, error) {
	b := createPacketBuilder(p)

	if len(p.Payload) > math.MaxUint16 {
		return nil, NewEncodeError("payload too large")
	}

	b.AddUint16(uint16(len(p.Payload)))
	b.AddBytes(p.Payload)
	return b.Bytes()
}

func NewData(b []byte) *Data {
	return &Data{
		PacketHeader: PacketHeader{
			ProtocolType: ProtocolVersionBTPv1,
			MessageType:  MessageTypeData,
		},
		Payload: b,
	}
}

// Unmarshal decodes a given Data message from transport format given header was already read and reader cursor is past it
func (p *Data) Unmarshal(h PacketHeader, r io.Reader) error {
	// buf does not contain packet length yet
	buf, err := readPacketBuffer(r, p)
	if err != nil {
		return err
	}

	// as debug save butes on package
	// p.Raw = buf

	p.PacketHeader = h
	b := cryptobyte.String(buf)

	var length uint16
	ok := b.ReadUint16(&length)
	if !ok {
		return NewDecodeError("length")
	}

	// if no payload provided do not decode
	if length == 0 {
		return nil
	}

	// read payload
	payloadBuf := make([]byte, length)
	n, err := r.Read(payloadBuf)
	if err != nil {
		return err
	}

	if n != int(length) {
		return io.ErrUnexpectedEOF
	}
	bp := cryptobyte.String(payloadBuf)
	ok = bp.ReadBytes(&p.Payload, int(length))
	if !ok {
		return NewDecodeError("payload")
	}

	return nil
}
