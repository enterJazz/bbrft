package messages

import (
	"errors"

	"gitlab.lrz.de/bbrft/brft/common"
	"golang.org/x/crypto/cryptobyte"
)

type MetaItem struct {
	// Filename of the requested file, can be at most 255 characters long
	FileName string
	// (optional) file fize of the respective file
	FileSize *uint64
	// (optional) checksum of the respective file
	Checksum []byte
}

// TODO: comment, include no filename -> also valid
func NewMetaItem(
	fileName string,
	fileSize *uint64, // might be nil
	checksum []byte, // might be empty
) (*MetaItem, error) {
	// TODO: maybe enforce filename length and regex
	// TODO: probably enforce checksum length

	return &MetaItem{
		FileName: fileName,
		FileSize: fileSize,
		Checksum: checksum,
	}, nil
}

func (m *MetaItem) Marshal() ([]byte, error) {
	fileName := []byte(m.FileName)
	if len(fileName) > 255 {
		return nil, errors.New("filename too long")
	} else if len(fileName) == 0 {
		return nil, errors.New("filename empty")
	}
	outLen := 1 + len(m.FileName)

	// check whether we should optionally add the filesize and checksum
	extended := false
	if m.FileSize != nil && len(m.Checksum) != 0 {
		extended = true
		outLen += 8 + common.ChecksumSize
	}

	// make sure either both or none of the optional parameters are set (!XOR)
	if (m.FileSize == nil) != (len(m.Checksum) == 0) {
		return nil, errors.New("file size OR checksum empty, but not both")
	}

	// write the filename
	b := cryptobyte.NewFixedBuilder(make([]byte, 0, outLen))
	b.AddUint8LengthPrefixed(func(b *cryptobyte.Builder) {
		b.AddBytes(fileName)
	})

	// if no optional fields are set, we can return early
	if !extended {
		return b.Bytes()
	}

	// Make sure the checksum length is valid
	AddUint64(b, *m.FileSize)

	if len(m.Checksum) != common.ChecksumSize {
		return nil, errors.New("invalid checksum length")
	}
	b.AddBytes(m.Checksum)

	return b.Bytes()
}

func (m *MetaItem) Unmarshal(data []byte, extended bool) error {
	s := cryptobyte.String(data)

	return m.UnmarshalWithString(&s, extended)
}

func (m *MetaItem) UnmarshalWithString(s *cryptobyte.String, extended bool) error {
	// read the filename
	var fileName cryptobyte.String
	if !s.ReadUint8LengthPrefixed(&fileName) {
		return ErrReadFailed
	}
	m.FileName = string(fileName)

	// TODO: It's not really ideal, but we do not really have an indicator whether
	// 		the filesize and checksum will be present
	if !extended {
		return nil
	}

	// try to read the file size
	var size uint64
	if !ReadUint64(s, &size) {
		return errors.New("extended item, but unable to read file size")
	}
	m.FileSize = &size

	checksum := make([]byte, 0, common.ChecksumSize)
	if !s.ReadBytes(&checksum, common.ChecksumSize) {
		return errors.New("extended item, but unable to read checksum")
	}
	m.Checksum = checksum

	return nil
}
