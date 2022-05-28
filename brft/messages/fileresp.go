package messages

import "io"

type FileRespStatusCode uint8

const (
	FileRespStatusCodeUndefined FileRespStatusCode = iota
	FileRespStatusCodeOk
	FileRespStatusCodeFileChanged
	// ...
)

type FileResp struct {
	PacketHeader
	// Checksum of the file the server offers for download. TODO: Maybe mutiple?
	Checksum   uint64 // TODO: How long?!
	StreamID   uint16
	FileSize   uint64
	StatusCode FileRespStatusCode

	// TODO: Maybe add a mechanism for the compression algorithms - would have to include a header length in that case as well

	Raw []byte
}

func (m *FileResp) Marshal() ([]byte, error) {

	return nil, nil
}

func (m *FileResp) Unmarshal([]byte) error {
	return nil
}

func (m *FileResp) GetLength(io.Reader) int {
	return 11
}
