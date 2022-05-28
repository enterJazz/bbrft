package messages

import "io"

type Data struct {
	// NOTE: The whole message has to be length delimeted in order to know how many bytes the receiver is supposed to read

	StreamID uint16
	// Data is the actual payloag, on the wire this field is length delimeted
	Data []byte

	Raw []byte
}

func (m *Data) Marshal() ([]byte, error) {

	return nil, nil
}

func (m *Data) Unmarshal([]byte) error {
	return nil
}

func (m *Data) GetLength(io.Reader) int {
	// TODO: Read the length from the reader
	return 0
}
