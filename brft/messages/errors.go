package messages

import "errors"

var (
	ErrReadFailed  = errors.New("read failed")
	ErrInvalidFlag = errors.New("invalid flag")

	ErrOptionalHeaderNil              = errors.New("optional header nil")
	ErrReceivedReservedOptionalHeader = errors.New("received reserved optional header")
)
