package messages

import (
	"gitlab.lrz.de/bbrft/cyberbyte"
	"go.uber.org/zap"
)

type Data struct {
	StreamID uint16
	// Data is the actual payloag, on the wire this field is length delimeted
	Data []byte
}

func (m *Data) Encode(l *zap.Logger) ([]byte, error) {
	// TODO: Implement
	return nil, nil
}

func (m *Data) Decode(l *zap.Logger, s *cyberbyte.String) error {
	// TODO: Implement
	return nil
}
