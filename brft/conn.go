package brft

import (
	"sync"

	"gitlab.lrz.de/bbrft/brft/compression"
	"gitlab.lrz.de/bbrft/brft/messages"
	"gitlab.lrz.de/bbrft/btp"
	"go.uber.org/zap"
)

// create a new connection object
type stream struct {
	l *zap.Logger

	id messages.StreamID
	f  *File

	// compression
	comp      compression.Compressor
	chunkSize int
}

type Conn struct {
	l *zap.Logger

	conn *btp.Conn

	isClient bool

	// base path to the directory where the files are located
	baseFilePath string

	streams      map[messages.StreamID]stream
	reqStreams   map[string]stream
	streamsMu    sync.Mutex
	reqStreamsMu sync.Mutex
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
