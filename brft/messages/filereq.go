package messages

import "io"

// TODO: Maybe uint8?
type FileReqFlags uint16

const (
	FileReqFlagsResumption FileReqFlags = 1 << iota
	// ...
)

type FileReq struct {
	// NOTE: The whole message has to be length delimeted in order to know how many bytes the receiver is supposed to read

	// Filename of the requested file, can be at most 255 characters long
	Filename string
	Flags    FileReqFlags
	// Checksum is the checksum of a previous partial download. If the
	// FileReqFlagsResumption is set, the checksum also has to be set
	Checksum uint64 // TODO: How long?!

	// TODO: Maybe add a mechanism for the compression algorithms

	Raw []byte
}

func (m *FileReq) Marshal() ([]byte, error) {

	return nil, nil
}

func (m *FileReq) Unmarshal([]byte) error {
	return nil
}

func (m *FileReq) GetLength(io.Reader) int64 {
	// TODO: Read the length from the reader
	return 0
}
