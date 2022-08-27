package btp

import "time"

const (
	rttAlpha          = 0.125
	oneMinusAlpha     = 1 - rttAlpha
	rttBeta           = 0.25
	oneMinusBeta      = 1 - rttBeta
	defaultInitialRTT = 100 * time.Millisecond
)

type RTTMeasurement struct {
	min    time.Duration
	latest time.Duration
	// Smoothed RTT
	srtt time.Duration
	// RTT varitance
	rttvar time.Duration

	hasSample bool
}

func NewRttMeasurement() *RTTMeasurement {
	return &RTTMeasurement{}
}

func (r *RTTMeasurement) Update(sendTimestamp, ackTimestamp time.Time) {
	delta := ackTimestamp.Sub(sendTimestamp)
	// make sure ack is in the future
	if delta < 0 {
		return
	}

	// first RTT measurement
	if !r.hasSample {
		r.hasSample = true
		// SRTT <- R
		r.srtt = delta
		// RTTVAR <- R/2
		r.rttvar = delta / 2
	} else {
		// RTTVAR <- (1 - beta) * RTTVAR + beta * |SRTT - R'|
		r.rttvar = time.Duration(oneMinusBeta*float32(r.rttvar) + rttBeta*float32(absDuration(r.srtt-delta)))
		// SRTT <- (1 - alpha) * SRTT + alpha * R'
		r.srtt = time.Duration(oneMinusAlpha*float32(r.srtt) + rttAlpha*float32(delta))
	}

	r.latest = delta
	if r.min > delta {
		r.min = delta
	}
}

func (r *RTTMeasurement) RTO() time.Duration {
	if !r.hasSample {
		// based on QUIC defaults
		return defaultInitialRTT * 2
	}

	// TODO: clock granularity (G) is ignored for now
	// RTO <- SRTT + max (G, K*RTTVAR)
	return r.srtt + 4*r.rttvar
}

func (r *RTTMeasurement) Min() time.Duration {
	return r.min
}
func (r *RTTMeasurement) Latest() time.Duration {
	return r.latest
}

func (r *RTTMeasurement) SRTT() time.Duration {
	return r.srtt
}

func (r *RTTMeasurement) RTTVar() time.Duration {
	return r.rttvar
}

func absDuration(a time.Duration) time.Duration {
	if a >= 0 {
		return a
	}
	return -a
}
