package messages

import (
	"gitlab.lrz.de/bbrft/cyberbyte"
	"go.uber.org/zap"
)

type StartTransmission struct {
	// TODO: Update to current RFC specs

	StreamID uint16
	// Checksum of the file to be downloaded, since it might have changed since the FileResp.
	Checksum []byte
	// (optional) Offset of a previous download
	Offset uint64
}

func (m *StartTransmission) Encode(l *zap.Logger) ([]byte, error) {
	// TODO: Implement
	return nil, nil
}

func (m *StartTransmission) Decode(l *zap.Logger, s *cyberbyte.String) error {
	// TODO: Implement
	return nil
}
