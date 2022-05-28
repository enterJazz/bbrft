package messages

import "io"

type StartTransmission struct {
	StreamID uint16
	// Checksum of the file to be downloaded, since it might have changed since the FileResp.
	Checksum uint64 // TODO: How long?!
	// (optional) Offset of a previous download
	Offset uint64

	Raw []byte
}

func (m *StartTransmission) Marshal() ([]byte, error) {

	return nil, nil
}

func (m *StartTransmission) Unmarshal([]byte) error {
	return nil
}

func (m *StartTransmission) GetLength(io.Reader) int64 {
	return 10
}
