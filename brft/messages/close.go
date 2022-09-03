package messages

import (
	"fmt"

	"gitlab.lrz.de/bbrft/cyberbyte"
	"go.uber.org/zap"
	"golang.org/x/crypto/cryptobyte"
)

type CloseReason uint8

const (
	// TODO: Update to current RFC specs
	CloseReasonUndefined CloseReason = iota
	CloseReasonChecksumInvalid
	CloseReasonInvalidOffset
	CloseReasonNotEnoughSpace
	CloseReasonTimeout
	CloseReasonInvalidFlags
	CloseReasonResumeNoChecksum
	CloseReasonInvalidChecksumAlgo
	CloseReasonFileNotFound
	// ...
)

type Close struct {
	StreamID StreamID
	Reason   CloseReason
}

func (m *Close) baseHeaderLen() int {
	return 2 + 1
}

func (m *Close) Encode(l *zap.Logger) ([]byte, error) {
	b := cryptobyte.NewFixedBuilder(make([]byte, 0, m.baseHeaderLen()))

	b.AddUint16(uint16(m.StreamID))
	b.AddUint8(uint8(m.Reason))
	return b.Bytes()
}

func (m *Close) Decode(l *zap.Logger, s *cyberbyte.String) error {
	if err := s.ReadUint16((*uint16)(&m.StreamID)); err != nil {
		return fmt.Errorf("unable to read StreamID: %w", err)
	}
	if err := s.ReadUint8((*uint8)(&m.Reason)); err != nil {
		return fmt.Errorf("unable to read Reason: %w", err)
	}

	return nil
}
