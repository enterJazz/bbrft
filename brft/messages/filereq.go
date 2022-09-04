package messages

import (
	"errors"
	"fmt"

	"gitlab.lrz.de/bbrft/brft/common"
	"gitlab.lrz.de/bbrft/cyberbyte"
	"go.uber.org/zap"
	"golang.org/x/crypto/cryptobyte"
)

type FileReqFlag uint8

const (
	FileReqFlagResumption FileReqFlag = 1 << iota
	// ...
)

var AllFileReqFlag = []FileReqFlag{
	FileReqFlagResumption,
}

type FileReqFlags []FileReqFlag

func (flags FileReqFlags) IsSet(flag FileReqFlag) bool {
	for _, f := range flags {
		if f == flag {
			return true
		}
	}
	return false
}

type FileReq struct {
	// NOTE: The whole message has to be length delimeted in order to know how many bytes the receiver is supposed to read

	StreamID StreamID

	Flags FileReqFlags

	// OptionalHeaders for the upcomming file transfer
	OptHeaders OptionalHeaders
	// FileName of the requested file, can be at most 255 characters long
	FileName string
	// Checksum is the checksum of a previous partial download or if a specific
	// file version shall be requested. Might be unitilized or zeroed.
	Checksum []byte
}

func (m *FileReq) String() string {
	return "FileReq"
}

func (m *FileReq) baseSize() int {
	// streamID + flags + file name length
	return 2 + 1 + 1 + len([]byte(m.FileName)) + common.ChecksumSize
}

func (m *FileReq) Encode(l *zap.Logger) ([]byte, error) {
	if len(m.Checksum) == 0 {
		l.Warn("unset checksum, initializing with zeros<")
		m.Checksum = make([]byte, common.ChecksumSize)
	}

	if len(m.Checksum) != common.ChecksumSize {
		return nil, errors.New("invalid checksum length")
	}

	fileName := []byte(m.FileName)
	if len(fileName) > 255 {
		return nil, errors.New("invalid filename length")
	}

	optHeaderBytes, err := marshalOptionalHeaders(m.OptHeaders)
	if err != nil {
		return nil, fmt.Errorf("unable to marshal optional headers: %w", err)
	}

	// determine the length of the output
	b := NewFixedBRFTMessageBuilderWithExtra(m, len(optHeaderBytes))

	// write the streamID
	b.AddUint16(uint16(m.StreamID))

	// combine the flags
	var flags FileReqFlag
	for _, f := range m.Flags {
		flags = flags | f
	}
	l.Debug("file request flags",
		zap.String("flags", fmt.Sprintf("%X", m.Flags)),
		zap.String("combined_flags", fmt.Sprintf("%b", flags)),
	)
	b.AddUint8(uint8(flags))

	// add the number of optional header together with the actual optional headers
	b.AddBytes(optHeaderBytes)

	// write the filename
	b.AddUint8LengthPrefixed(func(b *cryptobyte.Builder) {
		b.AddBytes(fileName)
	})

	// write the checksum
	b.AddBytes(m.Checksum)

	return b.Bytes()
}

func (m *FileReq) Decode(l *zap.Logger, s *cyberbyte.String) error {
	// TODO: hand over the cyberbyte.String instead. This way we can easily
	//		adapt the timeout when the RTT changes (i.e. reuse the same
	// 		cyberbyte.String and maybe also set a timeout field on it)

	// read the streamID
	var streamID uint16
	if err := s.ReadUint16(&streamID); err != nil {
		return fmt.Errorf("unable to read streamID: %w", err)
	}
	m.StreamID = StreamID(streamID)

	// read the flags
	var joinedFlags uint8
	if err := s.ReadUint8(&joinedFlags); err != nil {
		return fmt.Errorf("unable to read flags: %w", err)
	}

	// convert the joined flags into a slice
	for _, f := range AllFileReqFlag {
		if joinedFlags&uint8(f) > 0 {
			m.Flags = append(m.Flags, f)
			// unset the bit
			joinedFlags &= ^uint8(f)
		}
	}

	// make sure all flags have been recognized
	if joinedFlags > 0 {
		// TODO: ideally this would not break the communication
		return fmt.Errorf("%w: %X", ErrInvalidFlag, byte(joinedFlags))
	}

	// read the optional headers
	headers, err := readOptionalHeaders(l, s)
	if err != nil {
		return fmt.Errorf("unable to read optional headers: %w", err)
	}
	m.OptHeaders = headers

	// read the filename
	var fileName []byte
	if s.ReadUint8LengthPrefixedBytes(&fileName) != nil {
		return fmt.Errorf("unable to read file name: %w", err)
	}
	m.FileName = string(fileName)

	// read the checksum
	m.Checksum = make([]byte, common.ChecksumSize)
	if s.ReadBytes(&m.Checksum, common.ChecksumSize) != nil {
		return fmt.Errorf("unable to read checksum: %w", err)
	}

	return nil
}
