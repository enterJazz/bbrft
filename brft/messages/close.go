package messages

import (
	"gitlab.lrz.de/bbrft/cyberbyte"
	"go.uber.org/zap"
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
	// TODO: Update to current RFC specs
	StreamID uint16
	Reason   CloseReason

	Raw []byte
}

func (m *Close) Encode(l *zap.Logger) []byte {
	// TODO: Implement
	return nil
}

func (m *Close) Decode(l *zap.Logger, s *cyberbyte.String) error {
	// TODO: Implement
	return nil
}
