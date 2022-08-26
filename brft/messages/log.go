package messages

import "go.uber.org/zap"

// TODO: Move and solve circular dependency
func FOptionalHeaderType(optType OptionalHeaderType) zap.Field {
	return zap.Uint8("header_type", uint8(optType))

}
