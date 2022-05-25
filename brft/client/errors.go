package client

import "errors"

var (
	ErrInvalidChecksum   = errors.New("invalid checksum")
	ErrInsufficientRead  = errors.New("insufficient read")
	ErrInsufficientWrite = errors.New("insufficient write")
	ErrNotABRFTFile      = errors.New("file is not a BRFT file")
)
