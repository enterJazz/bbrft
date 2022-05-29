package messages

import "errors"

func NewDecodeError(msg string) error {
	return errors.New("decode error: " + msg)
}

func NewEncodeError(msg string) error {
	return errors.New("encode error: " + msg)
}
