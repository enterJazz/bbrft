package messages

import (
	"fmt"

	"gitlab.lrz.de/bbrft/cyberbyte"
	"go.uber.org/zap"
)

type CloseReason uint8

const (
	// TODO: Update to current RFC specs
	CloseReasonUndefined CloseReason = iota
	CloseReasonTransferComplete
	CloseReasonChecksumInvalid
	CloseReasonInvalidOffset
	CloseReasonNotEnoughSpace
	CloseReasonInvalidFlags
	CloseReasonResumeNoChecksum
	CloseReasonFileNotFound
	CloseReasonUnsupportedOptionalHeader
	CloseReasonUnexpectedOptionalHeader
	CloseReasonStreamIDTaken
	// ...
)

var closeReasonToString = map[CloseReason]string{
	CloseReasonUndefined:                 "Undefined",
	CloseReasonTransferComplete:          "Transfer Complete",
	CloseReasonChecksumInvalid:           "Checksum Invalid",
	CloseReasonInvalidOffset:             "Invalid Offset",
	CloseReasonNotEnoughSpace:            "Not Enough Space",
	CloseReasonInvalidFlags:              "Invalid Flags",
	CloseReasonResumeNoChecksum:          "Resume No Checksum",
	CloseReasonFileNotFound:              "File Not Found",
	CloseReasonUnsupportedOptionalHeader: "Unsupported Optional Header",
	CloseReasonUnexpectedOptionalHeader:  "Unexpected Optional Header",
	CloseReasonStreamIDTaken:             "Stream ID Taken",
}

func (r CloseReason) String() string {
	if s, ok := closeReasonToString[r]; !ok {
		return "unknown"
	} else {
		return s
	}
}

type Close struct {
	StreamID StreamID
	Reason   CloseReason
}

func (m *Close) Name() string {
	return fmt.Sprintf("Close Reason=%d, StreamID=%d", m.Reason, m.StreamID)
}

func (m *Close) baseSize() int {
	return 2 + 1
}

func (m *Close) Encode(l *zap.Logger) ([]byte, error) {
	b := NewFixedBRFTMessageBuilder(m)
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
