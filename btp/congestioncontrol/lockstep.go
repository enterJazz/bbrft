package congestioncontrol

import (
	"sync"

	"go.uber.org/zap"
)

type LockStepAlgorithm struct {
	// TODO: Add lock step state variables
	// cwnd
	// pipe
	//
	l *zap.Logger

	m    sync.RWMutex
	pipe int
}

func NewLockStepAlgorithm(l *zap.Logger) *LockStepAlgorithm {
	return &LockStepAlgorithm{
		l: l,
	}
}

func (l *LockStepAlgorithm) SentMessages(i int) {
	if i > 1 {
		l.l.Warn("client send misbehaviour - more messages sent than allowed",
			zap.Int("num_sent", i),
			zap.Int("num_allowed", 1),
		)
	} else if i == 0 {
		return
	}
	l.m.Lock()
	defer l.m.Unlock()
	l.pipe += i
}

func (l *LockStepAlgorithm) ReceivedAcks(i int) {
	if i > 1 {
		l.l.Warn("client received misbehaviour - more messages received than expected",
			zap.Int("num_received", i),
			zap.Int("num_expected", 1),
		)
	} else if i == 0 {
		return
	}
	l.m.Lock()
	defer l.m.Unlock()
	l.pipe -= i
	if l.pipe < 0 {
		l.pipe = 0
	}
}

func (l *LockStepAlgorithm) NumFreeSend() int {
	l.m.RLock()
	defer l.m.RUnlock()
	if l.pipe == 0 {
		return 1
	}
	return 0
}
