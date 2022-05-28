package btp

import "go.uber.org/zap"

const (
	FBTPHeaderProtocolVersion = "version"
	FBTPHeaderMessageType     = "message_type"
)

func FHeaderProtocolVersion(v ProtocolVersion) zap.Field {
	return zap.Uint8(FBTPHeaderProtocolVersion, uint8(v))
}

func FHeaderMessageType(t MessageType) zap.Field {
	return zap.Uint8(FBTPHeaderMessageType, uint8(t))
}
