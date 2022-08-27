package messages

import "errors"

var (
	ErrProtocolVersionMissmatch = errors.New("protocol version missmatch")
)

func NewDecodeError(msg string) error {
	return errors.New("decode error: " + msg)
}

func NewEncodeError(msg string) error {
	return errors.New("encode error: " + msg)
}
