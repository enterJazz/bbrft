package messages

import (
	"errors"
	"fmt"

	"gitlab.lrz.de/bbrft/brft/common"
	"gitlab.lrz.de/bbrft/cyberbyte"
	"go.uber.org/zap"
)

type FileRespStatus uint8

const (
	FileRespStatusReserved FileRespStatus = iota
	FileRespStatusOk
	FileRespStatusFileChanged
	FileRespStatusUnsupportedOptionalHeader
	FileRespStatusUnexpectedOptionalHeader
	// ...
)

var fileRespStatusPrecedence = map[FileRespStatus]int{
	FileRespStatusReserved:                  0,
	FileRespStatusOk:                        1,
	FileRespStatusFileChanged:               4,
	FileRespStatusUnsupportedOptionalHeader: 2,
	FileRespStatusUnexpectedOptionalHeader:  3,
}

func (s FileRespStatus) HasPrecedence(other FileRespStatus) bool {
	return fileRespStatusPrecedence[s] > fileRespStatusPrecedence[other]
}

type FileResp struct {
	Status     FileRespStatus
	OptHeaders OptionalHeaders

	StreamID StreamID
	FileSize uint64

	// Checksum of the file the server offers for download. Must have a length
	// of common.ChecksumSize
	Checksum []byte
}

func (m *FileResp) baseSize() int {
	// status + streamID + file size + checksum size
	return 1 + 2 + 8 + common.ChecksumSize
}

func (m *FileResp) Name() string {
	return "FileResp"
}

func (m *FileResp) Encode(l *zap.Logger) ([]byte, error) {
	if len(m.Checksum) != common.ChecksumSize {
		return nil, errors.New("invalid checksum length")
	}

	optHeaderBytes, err := marshalOptionalHeaders(m.OptHeaders)
	if err != nil {
		return nil, fmt.Errorf("unable to marshal optional headers: %w", err)
	}

	// determine the length of the output
	b := NewFixedBRFTMessageBuilderWithExtra(m, len(optHeaderBytes))

	// add the status
	b.AddUint8(uint8(m.Status))

	// add the number of optional header together with the actual optional headers
	b.AddBytes(optHeaderBytes)

	// add the streamID, file size and checksum
	b.AddUint16(uint16(m.StreamID))
	AddUint64(b, m.FileSize)
	b.AddBytes(m.Checksum)

	return b.Bytes()
}

func (m *FileResp) Decode(l *zap.Logger, s *cyberbyte.String) error {
	// read the status
	var status uint8
	if err := s.ReadUint8(&status); err != nil {
		return fmt.Errorf("unable to read status: %w", err)
	}
	m.Status = FileRespStatus(status)

	// read the optional headers
	headers, err := readOptionalHeaders(l, s)
	if err != nil {
		return fmt.Errorf("unable to read optional headers: %w", err)
	}
	m.OptHeaders = headers

	// read the streamID
	var streamID uint16
	if err := s.ReadUint16(&streamID); err != nil {
		return fmt.Errorf("unable to read streamID: %w", err)
	}
	m.StreamID = StreamID(streamID)

	// read the streamID
	var fileSize uint64
	if err := s.ReadUint64(&fileSize); err != nil {
		return fmt.Errorf("unable to read file size: %w", err)
	}
	m.FileSize = fileSize

	// read the checksum
	m.Checksum = make([]byte, common.ChecksumSize)
	if s.ReadBytes(&m.Checksum, common.ChecksumSize) != nil {
		return fmt.Errorf("unable to read checksum: %w", err)
	}

	return nil
}
