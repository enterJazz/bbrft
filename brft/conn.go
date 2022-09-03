package brft

import (
	"sync"

	"github.com/davecgh/go-spew/spew"
	"gitlab.lrz.de/bbrft/brft/compression"
	"gitlab.lrz.de/bbrft/brft/messages"
	"gitlab.lrz.de/bbrft/btp"
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
}

type Conn struct {
	l *zap.Logger

	conn *btp.Conn

	isClient bool

	// basePath is either the base file directory for the server or the
	// directory where the client downloads to
	basePath string

	streams      map[messages.StreamID]stream
	reqStreams   []stream // TODO: Implement a ring buffer for the requested streams
	streamsMu    sync.RWMutex
	reqStreamsMu sync.RWMutex
}

// CloseStream will send a close message to the other peer indicating that the
// stream should be closed. It also tries to remove the stream from conn.streams.
// HOWEVER, it does not remove any streams from conn.reqStreams
func (c *Conn) CloseStream(
	sid *messages.StreamID,
	r messages.CloseReason,
) error {
	// TODO: Send the close message

	if sid != nil {
		c.streamsMu.Lock()
		if _, ok := c.streams[*sid]; !ok {
			c.l.Warn("stream not found in streams",
				zap.String("streams", spew.Sdump(c.streams)),
				zap.String("requested_streams", spew.Sdump(c.reqStreams)),
				zap.Uint16("stream_id", uint16(*sid)),
			)
		} else {
			delete(c.streams, *sid)
			c.l.Warn("deleted stream not found in streams",
				zap.Uint16("stream_id", uint16(*sid)),
			)
		}
		c.streamsMu.Unlock()
	}

	// TODO: Should the whole connection be closed if this is the only stream?!

	return nil
}

// Close sends close messages to all the (remaining) streams and closes the
// btp.Conn.
func (c *Conn) Close() error {
	// TODO: Close all the (remaining) streams

	// close the btp.Conn
	return c.conn.Close()
}

func (c *Conn) readHeader() (messages.PacketHeader, error) {
	// read the message header
	b := make([]byte, 1)
	_, err := c.conn.Read(b)
	if err != nil {
		c.l.Error("unable to read message header - closing", zap.Error(err))
		errClose := c.Close()
		if errClose != nil {
			c.l.Error("unable to close connection", zap.Error(errClose))
			return messages.PacketHeader{}, errClose
		}

		return messages.PacketHeader{}, err
	}

	// determine the message version and type
	h := messages.NewPacketHeader(b[0])
	if !h.Version.Valid() {
		c.l.Error("unsupported header protocol version")
		errClose := c.Close()
		if errClose != nil {
			c.l.Error("unable to close connection", zap.Error(errClose))
			return messages.PacketHeader{}, errClose
		}
	}

	return h, nil
}
