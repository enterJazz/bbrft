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

	// progress channel transmitting the percentage of the overall progress.
	// Will beclose when the transfer is complete / canceled
	totalSize uint64
	// total data bytes transmiited over stream (raw bytes received including compression)
	totalTransmitted uint64
	// total number of decoded (if compression is enabled) bytes
	// if no compression enabled equals totalTransmitted
	totalPayloadTransmitted uint64
	progress                chan uint64

	// compression
	comp      compression.Compressor
	chunkSize int

	// resumption
	isResumption bool
	offset       uint64

	// handle incomming and outgoing messages
	in chan messages.BRFTMessage

	// if set method will be called when file resp is received from server
	onFileResp func(resp *messages.FileResp, err error)
}

func (s *stream) updateProgress(lenTransmitted int, lenDecoded int) {
	s.totalTransmitted += uint64(lenTransmitted)
	s.totalPayloadTransmitted += uint64(lenDecoded)

	if s.totalTransmitted > s.totalSize {
		s.l.Warn("progress is beyound file size",
			zap.Uint64("len_advertised", s.totalSize),
			zap.Uint64("len_received", s.totalTransmitted),
		)
		// TODO: should we close the stream?
	}

	// try to send the current progress
	select {
	case s.progress <- s.totalPayloadTransmitted:
	default:
		// try to read the first value and append the new one
		select {
		case <-s.progress:
			s.l.Warn("dropped progress entry")
			select {
			case s.progress <- s.totalPayloadTransmitted:
			default:
			}
		default:
		}
	}

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

	allFinished := len(c.streams) == 0

	c.streamsMu.Unlock()

	// close the file
	// TODO: if we want to keep all file checksums in memory on the server this
	//		would have to be copied to Conn.Close() as well (add a if case on
	//		isClient)
	err := s.f.Close()
	if err != nil {
		s.l.Error("unable to close file associated with stream", zap.Error(err))
	}

	// close own channels
	close(s.in)
	close(s.progress)

	// (optionally) send the close message
	if sendClosePacket {
		c.l.Debug("sending close packet to peer", zap.String("reason", r.String()))
		c.sendClosePacket(s.id, r)
	}

	// reset BTP read timeout to normal after all streams are closed
	// close BRFT client after all streams finished
	if allFinished {
		c.conn.ResetReadTimeout()
		// IDEA: close connections when no more open streams
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
