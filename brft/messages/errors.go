package messages

import "errors"

var (
	ErrReadFailed  = errors.New("read failed")
	ErrInvalidFlag = errors.New("invalid flag")
)
