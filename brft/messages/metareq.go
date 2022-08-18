package messages

import (
	"errors"

	"golang.org/x/crypto/cryptobyte"
)

type MetaReq struct {
	// Filename of the requested file, can be at most 255 characters long
	FileName string
}

// TODO: comment, include no filename -> also valid
func NewMetaReq(fileName string) (*MetaReq, error) {
	// TODO: maybe enforce filename length and regex

	return &MetaReq{
		FileName: fileName,
	}, nil
}

func (m *MetaReq) Marshal() ([]byte, error) {
	fileName := []byte(m.FileName)
	if len(fileName) > 255 {
		return nil, errors.New("filename too long")
	} else if len(fileName) == 0 {
		return nil, errors.New("filename empty")
	}

	len := 1 + len(m.FileName)
	b := cryptobyte.NewFixedBuilder(make([]byte, 0, len))

	// write the filename
	b.AddUint8LengthPrefixed(func(b *cryptobyte.Builder) {
		b.AddBytes(fileName)
	})

	return b.Bytes()
}

func (m *MetaReq) Unmarshal(data []byte) error {
	s := cryptobyte.String(data)

	// read the filename
	var fileName cryptobyte.String
	if !s.ReadUint8LengthPrefixed(&fileName) {
		return ErrReadFailed
	}
	m.FileName = string(fileName)

	return nil
}
