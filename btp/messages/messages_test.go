package messages

import (
	"bytes"
	"math"
	"reflect"
	"testing"
)

func Test_Coding(t *testing.T) {

	tests := []struct {
		name            string
		message         Codable
		wantEncodeError bool
		wantDecodeError bool
	}{
		{
			name: "conn",
			message: &Conn{
				PacketHeader: PacketHeader{
					ProtocolType: ProtocolVersionBTPv1,
					MessageType:  MessageTypeConn,
					SeqNr:        3,
				},
				MaxPacketSize: 1024,
			},
		},
		{
			name: "data",
			message: &Data{
				PacketHeader: PacketHeader{
					ProtocolType: ProtocolVersionBTPv1,
					MessageType:  MessageTypeData,
					SeqNr:        561,
				},
				Payload: []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10},
			},
		},
		{
			name: "data_max_size",
			message: &Data{
				PacketHeader: PacketHeader{
					ProtocolType: ProtocolVersionBTPv1,
					MessageType:  MessageTypeData,
					SeqNr:        12312,
				},
				Payload: make([]byte, math.MaxUint16),
			},
		},
		{
			name:            "data_exceeds_max_size",
			wantEncodeError: true,
			message: &Data{
				PacketHeader: PacketHeader{
					ProtocolType: ProtocolVersionBTPv1,
					MessageType:  MessageTypeData,
					SeqNr:        12312,
				},
				Payload: make([]byte, math.MaxUint16+1),
			},
		},
		{
			name: "data_no_payload",
			message: &Data{
				PacketHeader: PacketHeader{
					ProtocolType: ProtocolVersionBTPv1,
					MessageType:  MessageTypeData,
					SeqNr:        12312,
				},
				Payload: nil,
			},
		},
		{
			name: "ack",
			message: &Ack{
				PacketHeader: PacketHeader{
					ProtocolType: ProtocolVersionBTPv1,
					MessageType:  MessageTypeAck,
					SeqNr:        6,
				},
			},
		},
		{
			// ACK type but connection type is incorrect
			name:            "ack_type_missmatch",
			wantDecodeError: true,
			message: &Ack{
				PacketHeader: PacketHeader{
					ProtocolType: ProtocolVersionBTPv1,
					MessageType:  MessageTypeConn,
					SeqNr:        6,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			// TODO: split into two tests
			buf, err := tt.message.Marshal()

			// crahs if encoding error unexpected otherwise continue
			if (err != nil) != tt.wantEncodeError {
				t.Fatal(err)
			} else if tt.wantEncodeError && err != nil {
				return
			}

			r := bytes.NewReader(buf)
			h, err := ReadHeader(r)
			if err != nil {
				t.Fatal(err)
			}

			var decoded Codable

			switch h.MessageType {
			case MessageTypeConn:
				decoded = &Conn{}
			case MessageTypeAck:
				decoded = &Ack{}
			case MessageTypeClose:
				decoded = &Close{}
			case MessageTypeData:
				decoded = &Data{}
			case MessageTypeConnAck:
				decoded = &ConnAck{}
			default:
				t.Fatalf("unknown message type: %d", h.MessageType)
			}

			if err := decoded.Unmarshal(h, r); (err != nil) != tt.wantDecodeError {
				t.Errorf("Conn.Unmarshal() error = %v, wantErr %v", err, tt.wantDecodeError)
			}

			// remove buffer since it must not be equal

			if equal := reflect.DeepEqual(tt.message, decoded); (!equal) != tt.wantDecodeError {
				t.Errorf("messages are not equal want=%x, have=%x", tt.message, decoded)
			}
		})
	}
}
