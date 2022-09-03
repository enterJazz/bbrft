package messages

import (
	"fmt"

	"gitlab.lrz.de/bbrft/brft/common"
	"gitlab.lrz.de/bbrft/cyberbyte"
	"go.uber.org/zap"
)

type StartTransmission struct {
	StreamID StreamID
	// Checksum of the file to be downloaded, since it might have changed since the FileResp.
	Checksum []byte
	// (optional) Offset of a previous download
	Offset uint64
}

func (m *StartTransmission) baseSize() int {
	// flags + file name length
	return 2 + common.ChecksumSize + 8
}

func (m *StartTransmission) Encode(l *zap.Logger) ([]byte, error) {
	b := NewFixedBRFTMessageBuilder(m)

	b.AddUint16(uint16(m.StreamID))
	b.AddBytes(m.Checksum)
	AddUint64(b, m.Offset)
	return b.Bytes()
}

func (m *StartTransmission) Decode(l *zap.Logger, s *cyberbyte.String) error {
	if err := s.ReadUint16((*uint16)(&m.StreamID)); err != nil {
		return fmt.Errorf("unable to read StreamID: %w", err)
	}

	if err := s.ReadBytes(&m.Checksum, common.ChecksumSize); err != nil {
		return fmt.Errorf("unable to read checksum: %w", err)
	}

	if err := s.ReadUint64(&m.Offset); err != nil {
		return fmt.Errorf("unable to read offset: %w", err)
	}

	return nil
}
