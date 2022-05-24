package congestioncontrol

import (
	"go.uber.org/zap"
)

type SimpleAIMDAlgorithm struct {
	// TODO: Add lock step state variables
	// cwnd
	// pipe
	//
	l *zap.Logger
}

func NewSimpleAIMDAlgorithm(l *zap.Logger) *SimpleAIMDAlgorithm {
	return &SimpleAIMDAlgorithm{
		l: l,
	}
}
