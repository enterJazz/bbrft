package messages

import (
	"errors"

	"gitlab.lrz.de/bbrft/cyberbyte"
	"go.uber.org/zap"
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

func (m *MetaReq) baseSize() int {
	return len([]byte(m.FileName))
}

func (m *MetaReq) Name() string {
	return "MetaItemReq"
}

func (m *MetaReq) Encode(l *zap.Logger) ([]byte, error) {
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

func (m *MetaReq) Decode(l *zap.Logger, s *cyberbyte.String) error {
	// read the filename
	var fileName []byte
	if err := s.ReadUint8LengthPrefixedBytes(&fileName); err != nil {
		return ErrReadFailed
	}
	m.FileName = string(fileName)

	return nil
}
