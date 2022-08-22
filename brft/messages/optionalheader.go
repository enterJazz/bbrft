package messages

import "io"

type OptionalHeader interface {
	Write(io.Writer) error // i.e. Marshal & Write
	Read(io.Reader) error  // i.e. Read & Unmarshal
	// TODO: Probably more functions needed
}

type OptionalHeaders []OptionalHeader

func (opts OptionalHeaders) Read(opt OptionalHeader) bool {
	for _, o := range opts {
		// FIXME: can't really do this (equal will ot work - actually want to compare the struct types)
		if opt == o {
			return true
		}
	}

	return false
}

type BaseOptionalHeader struct {
	Length uint8
	Type   uint8
}

type UnknownOptionalHeader struct {
	BaseOptionalHeader
}
