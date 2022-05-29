package messages

import (
	"errors"
	"fmt"
	"io"

	"crypto/sha256"

	"gitlab.lrz.de/bbrft/brft/common"
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

	// Filename of the requested file, can be at most 255 characters long
	Filename string
	Flags    FileReqFlags
	// Checksum is the checksum of a previous partial download. If the
	// FileReqFlagResumption is set, the checksum also has to be set. In that case the checksum must be exactly of the
	// size of our checksum Algorithm (currently SHA256 -> 32 Byte)
	Checksum []byte

	// TODO: Maybe add a mechanism for the compression algorithms

	// TODO: Currently raw is missing the length of the message when unmarshaled, maybe find a way around this..
	Raw []byte
}

const (
	baseHeaderLen = 4
)

func (m *FileReq) Marshal() ([]byte, error) {
	if len(m.Checksum) != 0 && len(m.Checksum) != common.ChecksumSize {
		return nil, errors.New("invalid checksum length")
	}

	filename := []byte(m.Filename)
	if len(filename) > 255 {
		return nil, errors.New("invalid filename")
	}

	len := baseHeaderLen + len([]byte(filename))
	if m.Flags.IsSet(FileReqFlagResumption) {
		len += sha256.Size
	}
	b := cryptobyte.NewFixedBuilder(make([]byte, 0, len))

	// write the whole header length
	b.AddUint16LengthPrefixed(func(b *cryptobyte.Builder) {
		// write the filename
		b.AddUint8LengthPrefixed(func(b *cryptobyte.Builder) {
			b.AddBytes(filename)
		})

		// combine the flags
		var flags FileReqFlag
		fmt.Printf("m.Flags %X\n", m.Flags)

		for _, f := range m.Flags {
			flags = flags | f
		}
		fmt.Printf("flags %X\n", flags)
		b.AddUint8(uint8(flags))

		// write the checksum
		b.AddBytes(m.Checksum)
	})

	var err error
	m.Raw, err = b.Bytes()
	return m.Raw, err
}

func (m *FileReq) Unmarshal(data []byte) error {
	*m = FileReq{Raw: data}
	s := cryptobyte.String(data)

	// read the filename
	var fileName cryptobyte.String
	if !s.ReadUint8LengthPrefixed(&fileName) {
		return ErrReadFailed
	}
	m.Filename = string(fileName)

	// read the flags
	var joinedFlags uint8
	if !s.ReadUint8(&joinedFlags) {
		return ErrReadFailed
	}

	for _, f := range AllFileReqFlag {
		if joinedFlags&uint8(f) > 0 {
			m.Flags = append(m.Flags, f)
			// unset the bit
			joinedFlags &= ^uint8(f)
		}
	}

	if joinedFlags > 0 {
		return fmt.Errorf("%w: %X", ErrInvalidFlag, byte(joinedFlags))
	}

	// potentially read the checksum
	if m.Flags.IsSet(FileReqFlagResumption) {
		m.Checksum = make([]byte, common.ChecksumSize)
		if !s.ReadBytes(&m.Checksum, common.ChecksumSize) {
			return ErrReadFailed
		}
	}

	if !s.Empty() {
		// TODO: Better errror message
		return errors.New("bytes remaining")
	}

	return nil
}

func (m *FileReq) GetLength(r io.Reader) (int, error) {
	// read the length from the reader
	lenB := make([]byte, 2)
	n, err := r.Read(lenB)
	if err != nil {
		return 0, err
	} else if n != 2 {
		return 0, errors.New("insufficient Read")
	}

	s := cryptobyte.String(lenB)
	var len uint16
	if !s.ReadUint16(&len) {
		return 0, ErrReadFailed
	}
	return int(len), nil
}
