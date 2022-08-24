package congestioncontrol

import (
	"go.uber.org/zap"
	"math"
	"sync"
)

type ElasticTcpAlgorithm struct {
	l *zap.Logger

	cwnd     int
	pipe     int
	ssthresh int
	rtt_max  int
	rtt_curr int
	m        sync.RWMutex
}

const MultiplicativeDecreaseFactor = 0.7

func Init(l *zap.Logger, initialCwndSize int) *ElasticTcpAlgorithm {

	return &ElasticTcpAlgorithm{
		l:    l,
		cwnd: initialCwndSize,
		// as in https://elixir.bootlin.com/linux/latest/source/net/ipv4/tcp_cong.c#L465
		ssthresh: initialCwndSize / 2,
	}
}

func (e *ElasticTcpAlgorithm) Name() string {
	return "elastic_tcp"
}

func (e *ElasticTcpAlgorithm) SentMessages(i int) {
	e.m.Lock()
	defer e.m.Unlock()
	e.pipe += i

	if e.pipe > e.cwnd {
		e.l.Warn("client send misbehaviour - more messages sent than allowed",
			zap.Int("num_sent", i),
			zap.Int("num_allowed", e.cwnd),
		)
	}
}

func (e *ElasticTcpAlgorithm) ReceivedAcks(i int) {
	e.m.Lock()
	defer e.m.Unlock()
	e.pipe -= i

	if e.pipe < 0 {
		e.l.Warn("client received misbehaviour - more messages received than expected",
			zap.Int("num_received", i),
			zap.Int("num_expected", i+e.pipe),
		)
	}

	if e.pipe < 0 {
		e.pipe = 0
	}
}

func (e *ElasticTcpAlgorithm) NumFreeSend() int {
	e.m.RLock()
	defer e.m.RUnlock()
	return e.cwnd - e.pipe
}

// implementation of methods as in Kernel TCP: https://elixir.bootlin.com/linux/latest/source/include/net/tcp.h#L1053

func (e *ElasticTcpAlgorithm) ElasticCongAvoid() {
	e.m.Lock()
	defer e.m.Unlock()

	// NOTE a check if tcp is cwnd limited could be inserted here

	// from V. Jacobson, “Congestion Avoidance and Control,” p. 21.
	if e.cwnd < e.ssthresh {
		e.cwnd += 1
	} else {
		// from M. A. Alrshah, M. A. Al-Maqri, and M. Othman, “Elastic-TCP: Flexible Congestion Control Algorithm to Adapt for High-BDP Networks,” IEEE Systems Journal, vol. 13, no. 2, pp. 1336–1346, Jun. 2019, doi: 10.1109/JSYST.2019.2896195.
		wwf64 := math.Sqrt(float64((e.rtt_max / e.rtt_curr) * e.cwnd))
		e.cwnd += int(math.Round(wwf64)) / e.cwnd
	}
}

// ElasticUpdateRtt rtt: RTT of last ACKed packet
func (e *ElasticTcpAlgorithm) UpdateRtt(rtt int) {
	e.m.Lock()
	defer e.m.Unlock()
	// rtt increment from https://github.com/cxxtao/tcp-elastic/blob/57d4f0cd2afb261654bfa9cc1339fc2e1f3e7c9e/tcp_elastic.c#L36
	rtt += 1
	if rtt > e.rtt_max || e.rtt_max == 0 {
		e.rtt_max = 0
	}
	e.rtt_curr = rtt
}

func (e *ElasticTcpAlgorithm) HandleEvent(event CaEvent) {
	switch event {
	case Loss, Duplicate:
		e.m.Lock()
		defer e.m.Unlock()
		e.rtt_max = 0
		e.cwnd = int(math.Round(float64(e.cwnd) * MultiplicativeDecreaseFactor))
		// from “RFC5681” Apr. 02, 2013. https://www.rfc-editor.org/rfc/pdfrfc/rfc5681.txt.pdf (accessed Aug. 06, 2022).
		e.ssthresh = int(math.Round(math.Max(float64(e.pipe/2), 2)))
	}
}
