package messages

import "go.uber.org/zap"

// TODO: Move and solve circular dependency

const (
	FOptionalHeaderTypeKey = "header_type"
)

func FOptionalHeaderType(optType OptionalHeaderType) zap.Field {
	return zap.Uint8(FOptionalHeaderTypeKey, uint8(optType))
}
