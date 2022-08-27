package btp

import (
	"gitlab.lrz.de/bbrft/btp/messages"
	"go.uber.org/zap"
)

const (
	FBTPHeaderProtocolVersion   = "version"
	FBTPHeaderMessageType       = "message_type"
	FBTPHeaderMessageTypeString = "message_type_name"
)

func FHeaderProtocolVersion(v messages.ProtocolVersion) zap.Field {
	return zap.Uint8(FBTPHeaderProtocolVersion, uint8(v))
}

func FHeaderMessageType(t messages.MessageType) zap.Field {
	return zap.Uint8(FBTPHeaderMessageType, uint8(t))
}

func FHeaderMessageTypeString(t messages.MessageType) zap.Field {
	return zap.String(FBTPHeaderMessageTypeString, t.String())
}
