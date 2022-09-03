package congestioncontrol

type CongestionControlAlgorithm interface {
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
