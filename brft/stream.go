package brft

import (
	"errors"

	"gitlab.lrz.de/bbrft/brft/compression"
	"gitlab.lrz.de/bbrft/brft/messages"
	"go.uber.org/zap"
)

// TODO: Add a close for the stream
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

	// handle incomming and outgoing messages
	in chan messages.BRFTMessage
}

// TODO: maybe simply make this the close on the stream
//   - that's not so easy, since we'd have to move the wg and more improtantly streams & streamsMu to the stream...
//
// CloseStream will send a close message to the other peer indicating that the
// stream should be closed. It also removes the stream from conn.streams.
// HOWEVER, it does not remove any streams from conn.reqStreams
func (c *Conn) CloseStream(
	s *stream,
	sendClosePacket bool,
	r messages.CloseReason,
) { // no need to return an error since we want to close the stream either way
	defer s.l.Info("closed stream")

	s.l.Debug("closing stream")

	c.streamsMu.Lock()
	if _, ok := c.streams[s.id]; !ok {
		s.l.Error("implementation error: cannot remove streamID",
			zap.Error(errors.New("streamID does not exist")),
		)
	} else {
		s.l.Debug("removing stream connection")
		delete(c.streams, s.id)
	}

	// TODO: Probably shutdown the client if this was the last active stream

	c.streamsMu.Unlock()

	// close the file
	// TODO: if we want to keep all file checksums in memory on the server this
	//		would have to be copied to Conn.Close() as well (add a if case on
	//		isClient)
	err := s.f.Close()
	if err != nil {
		s.l.Error("unable to close file associated with stream", zap.Error(err))
	}

	// close the channel
	close(s.in)

	// (optionally) send the close message
	if sendClosePacket {
		c.l.Debug("sending close packet to peer", zap.String("reason", r.String()))
		c.sendClosePacket(s.id, r)
	}
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
