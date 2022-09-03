package brft

import "errors"

var (
	ErrExpectedClientConnection = errors.New("expected client connection")
	ErrExpectedServerConnection = errors.New("expected server connection")

	// TODO: potentially clean up
	ErrReadLen           = errors.New("unexpected read length")
	ErrInvalidChecksum   = errors.New("invalid checksum")
	ErrInsufficientRead  = errors.New("insufficient read")
	ErrInsufficientWrite = errors.New("insufficient write")
	ErrNotABRFTFile      = errors.New("file is not a BRFT file")
)
