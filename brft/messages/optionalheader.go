package messages

import (
	"errors"

	"gitlab.lrz.de/bbrft/cyberbyte"
	"go.uber.org/zap"
	"golang.org/x/crypto/cryptobyte"
)

// TODO: enrich error messages of Marshal and Read functions

type OptionalHeaderType uint8

const (
	OptionalHeaderTypeReserved OptionalHeaderType = iota
	OptionalHeaderTypeCompressionReq
	OptionalHeaderTypeCompressionResp
)

type OptionalHeader interface {
	// TODO: maybe rename Marshal function
	Marshal() ([]byte, error)                         // i.e. Marshal & Write
	Read(*cyberbyte.String, BaseOptionalHeader) error // i.e. Read & Unmarshal
}

type OptionalHeaders []OptionalHeader

// BaseOptionalHeader is a struct that holds the two fields that need to be
// present on all optional headers.
// NOTE: the BaseOptionalHeader does not actually implement the OptionalHeader
// interface
type BaseOptionalHeader struct {
	Length uint8
	Type   OptionalHeaderType
}

// TODO: Rename to InitBuilder
func (h *BaseOptionalHeader) Marshal() (*cryptobyte.Builder, error) {
	if h == nil {
		return nil, ErrOptionalHeaderNil
	}

	b := cryptobyte.NewFixedBuilder(make([]byte, 0, 1+h.Length)) // length field itself is not included in length

	b.AddUint8(h.Length)
	b.AddUint8(uint8(h.Type))

	return b, nil
}

func (h *BaseOptionalHeader) Read(s *cyberbyte.String) error {
	if h == nil {
		return ErrOptionalHeaderNil
	}

	// read the length & type
	var length, optType uint8
	if err := s.ReadUint8(&length); err != nil {
		return err
	}
	if err := s.ReadUint8(&optType); err != nil {
		return err
	}

	*h = BaseOptionalHeader{
		Length: length,
		Type:   OptionalHeaderType(optType),
	}

	return nil
}

// UnknownOptionalHeader is a dedicated catch all header for all headers unknown
// to this implementation.
type UnknownOptionalHeader struct {
	BaseOptionalHeader
	Data []byte
}

func (h *UnknownOptionalHeader) Marshal() ([]byte, error) {
	if h == nil {
		return nil, ErrOptionalHeaderNil
	}

	b, err := h.BaseOptionalHeader.Marshal()
	if err != nil {
		return nil, err
	}

	// TODO: Should I make sure that the base.length is respected?

	b.AddBytes(h.Data)

	return b.Bytes()
}

func (h *UnknownOptionalHeader) Read(s *cyberbyte.String, base BaseOptionalHeader) error {
	if h == nil {
		return ErrOptionalHeaderNil
	}

	data := make([]byte, base.Length)
	err := s.ReadBytes(&data, int(base.Length))
	if err != nil {
		return err
	}

	*h = UnknownOptionalHeader{
		BaseOptionalHeader: base,
		Data:               data,
	}

	return nil
}

type CompressionReqHeaderAlgorithm uint8

const (
	CompressionReqHeaderAlgorithmReserved CompressionReqHeaderAlgorithm = iota
	CompressionReqHeaderAlgorithmGzip
)

type CompressionReqOptionalHeader struct {
	BaseOptionalHeader
	Algorithm           CompressionReqHeaderAlgorithm
	ChunkSizeMultiplier uint8
}

func (h *CompressionReqOptionalHeader) Marshal() ([]byte, error) {
	if h == nil {
		return nil, ErrOptionalHeaderNil
	}

	b, err := h.BaseOptionalHeader.Marshal()
	if err != nil {
		return nil, err
	}

	// TODO: Should we validate that the algorithm is known?
	b.AddUint8(uint8(h.Algorithm))
	b.AddUint8(h.ChunkSizeMultiplier)

	return b.Bytes()
}

func (h *CompressionReqOptionalHeader) Read(
	s *cyberbyte.String,
	base BaseOptionalHeader,
) error {
	if h == nil {
		return ErrOptionalHeaderNil
	}

	// read the length & type
	var alg, mult uint8
	if err := s.ReadUint8(&alg); err != nil {
		return err
	}
	if err := s.ReadUint8(&mult); err != nil {
		return err
	}

	*h = CompressionReqOptionalHeader{
		BaseOptionalHeader:  base,
		Algorithm:           CompressionReqHeaderAlgorithm(alg), // TODO: unknown algorithms must be handled by caller
		ChunkSizeMultiplier: mult,
	}

	return nil
}

type CompressionRespHeaderStatus uint8

const (
	CompressionRespHeaderStatusReserved CompressionRespHeaderStatus = iota
	CompressionRespHeaderStatusOk
	CompressionRespHeaderStatusNoCompression
	CompressionRespHeaderStatusUnknownAlgorithm
	CompressionRespHeaderStatusFileTooSmall
)

type CompressionRespOptionalHeader struct {
	BaseOptionalHeader
	Status CompressionRespHeaderStatus
}

func (h *CompressionRespOptionalHeader) Marshal() ([]byte, error) {
	if h == nil {
		return nil, ErrOptionalHeaderNil
	}

	b, err := h.BaseOptionalHeader.Marshal()
	if err != nil {
		return nil, err
	}

	// TODO: Should we validate that the status is known?
	b.AddUint8(uint8(h.Status))

	return b.Bytes()
}

func (h *CompressionRespOptionalHeader) Read(
	s *cyberbyte.String,
	base BaseOptionalHeader,
) error {
	if h == nil {
		return ErrOptionalHeaderNil
	}

	// read the length & type
	var stat uint8
	if err := s.ReadUint8(&stat); err != nil {
		return err
	}

	*h = CompressionRespOptionalHeader{
		BaseOptionalHeader: base,
		Status:             CompressionRespHeaderStatus(stat), // TODO: unknown status must be handled by caller
	}

	return nil
}

func marshalOptionalHeaders(hs OptionalHeaders) ([]byte, error) {
	if len(hs) > 255 {
		return nil, errors.New("too many optional headers")
	}

	b := make([]byte, 0, len(hs)*3) // educated guess on length

	// add the number of optional headers
	b = append(b, uint8(len(hs)))

	// marshal the individual headers
	for _, h := range hs {
		hb, err := h.Marshal()
		if err != nil {
			return nil, err
		}

		b = append(b, hb...)
	}

	return b, nil
}

func readOptionalHeaders(l *zap.Logger, s *cyberbyte.String) (OptionalHeaders, error) {
	// get the number of the optional headers
	var numHeaders uint8
	if err := s.ReadUint8(&numHeaders); err != nil {
		return nil, err
	}

	headers := make(OptionalHeaders, 0, numHeaders)
	for n := 0; n < int(numHeaders); n++ {

		base := new(BaseOptionalHeader)
		base.Read(s)

		// check whether the type is known
		var h OptionalHeader
		switch base.Type {
		case OptionalHeaderTypeReserved:
			l.Warn(ErrReceivedReservedOptionalHeader.Error(),
				FOptionalHeaderType(base.Type),
			)
			return nil, ErrReceivedReservedOptionalHeader

		case OptionalHeaderTypeCompressionReq:
			// simply parse the header
			h = new(CompressionReqOptionalHeader)
			err := h.Read(s, *base)
			if err != nil {
				return nil, err
			}

		case OptionalHeaderTypeCompressionResp:

			l.Warn("received reserved optional header",
				FOptionalHeaderType(base.Type),
			)

			// parse the header nonetheless and let the caller handle the actual logic TODO: caller check
			h = new(CompressionRespOptionalHeader)
			err := h.Read(s, *base)
			if err != nil {
				return nil, err
			}

		default:
			// TODO: log somewhere
			l.Warn("received unknown optional header",
				FOptionalHeaderType(base.Type),
			)

			// add an unknown optional header
			h = new(UnknownOptionalHeader)
			err := h.Read(s, *base)
			if err != nil {
				return nil, err
			}
		}
		headers = append(headers, h)
	}

	return headers, nil
}
