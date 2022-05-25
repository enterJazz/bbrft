package log

import (
	"go.uber.org/zap"
)

const (
	FPeerKey      = "peer"
	FComponentKey = "component"
)

func FPeer(peer string) zap.Field {
	return zap.String(FPeerKey, peer)
}

func FComponent(component string) zap.Field {
	return zap.String(FComponentKey, component)
}
