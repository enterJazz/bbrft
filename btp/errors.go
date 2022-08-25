package btp

import "errors"

var (
	ErrInvalidClientResp      = errors.New("invalid client response")
	ErrInvalidServerResp      = errors.New("invalid client response")
	ErrInvalidSeqNr           = errors.New("invalid sequence number")
	ErrInvalidProtocolVersion = errors.New("invalid protocol version")
)

func ErrInvalidHandshakeOption(option string) error {
	return errors.New("invalid handshake option:" + option)
}
