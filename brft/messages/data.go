package messages

import (
	"fmt"

	"gitlab.lrz.de/bbrft/cyberbyte"
	"go.uber.org/zap"
)

type Data struct {
	StreamID StreamID
	// Data is the actual payload, on the wire this field is length delimeted
	Data []byte
}

func (m *Data) baseSize() int {
	// streamID + length of payload
	return 2 + 3 + len(m.Data)
}

func (m *Data) Name() string {
	return fmt.Sprintf("Data len=%d", len(m.Data))
}

func (m *Data) Encode(l *zap.Logger) ([]byte, error) {
	b := NewFixedBRFTMessageBuilder(m)

	b.AddUint16(uint16(m.StreamID))
	b.AddUint24(uint32(len(m.Data)))
	b.AddBytes(m.Data)
	return b.Bytes()
}

func (m *Data) Decode(l *zap.Logger, s *cyberbyte.String) error {
	if err := s.ReadUint16((*uint16)(&m.StreamID)); err != nil {
		return fmt.Errorf("unable to read StreamID: %w", err)
	}

	var lenPayload uint32
	if err := s.ReadUint24(&lenPayload); err != nil {
		return fmt.Errorf("unable to read payload length: %w", err)
	}

	if err := s.ReadBytes(&m.Data, int(lenPayload)); err != nil {
		return fmt.Errorf("unable to read data: %w", err)
	}

	return nil
}
