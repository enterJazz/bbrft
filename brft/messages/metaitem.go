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

// TODO: Move checks into the New* function in order to later be able to use
// this function to test the server with invalid input
func (m *MetaItem) Marshal() ([]byte, error) {
	fileName := []byte(m.FileName)
	if len(fileName) > 255 {
		return nil, errors.New("filename too long")
	} else if len(fileName) == 0 {
		return nil, errors.New("filename empty")
	}

	fileLen := 1 + len(m.FileName)
	b := cryptobyte.NewFixedBuilder(make([]byte, 0, fileLen))

	// write the filename
	b.AddUint8LengthPrefixed(func(b *cryptobyte.Builder) {
		b.AddBytes(fileName)
	})

	// if the file size and checksum are not set we can return early
	if m.FileSize == nil && len(m.Checksum) != 0 {
		return b.Bytes()
	}

	// optionally write the filesize and checksum
	if m.FileSize == nil || len(m.Checksum) != 0 {
		return nil, errors.New("file size OR checksum empty, but not both")
	}

	// Make sure the checksum length is valid
	AddUint64(b, *m.FileSize)

	if len(m.Checksum) != common.ChecksumSize {
		return nil, errors.New("invalid checksum length")
	}
	b.AddBytes(m.Checksum)

	return b.Bytes()
}

func (m *MetaItem) Unmarshal(data []byte) error {
	s := cryptobyte.String(data)

	// read the filename
	var fileName cryptobyte.String
	if !s.ReadUint8LengthPrefixed(&fileName) {
		return ErrReadFailed
	}
	m.FileName = string(fileName)

	// TODO: It's not really ideal, but we do ot really have an indicator whether
	// 		the filesize and checksum will be present
	// try to read the file size, if it does not work we
	var size uint64
	if !ReadUint64(&s, &size) {
		return nil
	}
	m.FileSize = &size

	checksum := make([]byte, 0, common.ChecksumSize)
	if !s.ReadBytes(&checksum, common.ChecksumSize) {
		return ErrReadFailed
	}
	m.Checksum = checksum

	return nil
}
