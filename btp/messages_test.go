package btp

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func assertHeader(t *testing.T, want PacketHeader, have PacketHeader) {
	assert.Equal(t, want.MessageType, have.MessageType, "message type not equal")
	assert.Equal(t, want.ProtocolType, have.ProtocolType, "protocol version not equal")
	assert.Equal(t, want.SeqNr, have.SeqNr, "sequence number not equal")
}

func TestConn_Unmarshal(t *testing.T) {
	type fields struct {
		PacketHeader  PacketHeader
		MaxPacketSize uint16
	}

	tests := []struct {
		name    string
		fields  fields
		wantErr bool
	}{
		{
			name: "valid",
			fields: fields{
				PacketHeader: PacketHeader{
					ProtocolType: ProtocolVersionBTPv1,
					MessageType:  MessageTypeConn,
					SeqNr:        3,
				},
				MaxPacketSize: 1024,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Conn{
				PacketHeader:  tt.fields.PacketHeader,
				MaxPacketSize: tt.fields.MaxPacketSize,
			}

			// TODO: split into two tests
			buf, err := p.Marshal()
			if err != nil {
				t.Fatal(err)
			}

			r := bytes.NewReader(buf)
			h, err := ReadHeader(r)
			if err != nil {
				t.Fatal(err)
			}

			out := new(Conn)

			if err := out.Unmarshal(h, r); (err != nil) != tt.wantErr {
				t.Errorf("Conn.Unmarshal() error = %v, wantErr %v", err, tt.wantErr)
			}

			assertHeader(t, p.PacketHeader, out.PacketHeader)

			assert.Equal(t, p.PacketHeader, out.MaxPacketSize, "max packet size not equal")
		})
	}
}
