package brft

import (
	"gitlab.lrz.de/bbrft/brft/compression"
	"gitlab.lrz.de/bbrft/brft/messages"
	"go.uber.org/zap"
)

// create a new connection object
type stream struct {
	l *zap.Logger

	id messages.StreamID

	// TODO: all of the below needs to be initialized in the server handling
	// file
	f                 *File
	fileName          string // only needed during neogtiation
	requestedChecksum []byte // only needed during neogtiation

	// compression
	comp      compression.Compressor
	chunkSize int

	// resumption
	isResumption bool
	offset       uint64

	handshakeDone bool

	// FIXME: actually use

	// handle incomming and outgoing messages
	in chan messages.BRFTMessage
}

func (c *Conn) getStream(ID messages.StreamID) *stream {
	c.streamsMu.RLock()
	if s, ok := c.streams[ID]; ok {
		c.streamsMu.RUnlock()
		return s
	}
	c.streamsMu.RUnlock()
	return nil
}
