package congestioncontrol

type CongestionControlAlgorithm interface {
	SentMessages(int)
	ReceivedAcks(int)
	NumFreeSend() int
}
