package congestioncontrol

import "go.uber.org/zap"

type CongestionControlAlgorithm interface {
	Init(*zap.Logger, int) CongestionControlAlgorithm

	SentMessages(int)
	ReceivedAcks(int)
	NumFreeSend() int

	CongAvoid()
	UpdateRTT(int)
	HandleEvent(CaEvent)

	Name() string
}

type CaEvent int

const (
	Loss CaEvent = iota
	Duplicate
)
