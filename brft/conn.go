package brft

import (
	"fmt"
	"sync"
	"time"

	"github.com/davecgh/go-spew/spew"
	"gitlab.lrz.de/bbrft/brft/messages"
	"gitlab.lrz.de/bbrft/btp"
	"gitlab.lrz.de/bbrft/cyberbyte"
	"go.uber.org/zap"
)

type Conn struct {
	l *zap.Logger

	conn *btp.Conn

	isClient bool

	// basePath is either the base file directory for the server or the
	// directory where the client downloads to
	basePath string

	streams   map[messages.StreamID]*stream
	streamsMu sync.RWMutex
	wg        *sync.WaitGroup
	close     chan struct{}

	// buffers for sending data
	outCtrl chan []byte
	outData chan []byte

	// TODO: make sure the options are used
	options ConnOptions
}

// CloseStream will send a close message to the other peer indicating that the
// stream should be closed. It also tries to remove the stream from conn.streams.
// HOWEVER, it does not remove any streams from conn.reqStreams
func (c *Conn) CloseStream(
	sid *messages.StreamID, // TODO: make non pointer!
	r messages.CloseReason,
) { // no need to return an error since we want to close it either way
	// TODO: Send the close message

	if sid != nil {
		c.streamsMu.Lock()
		if _, ok := c.streams[*sid]; !ok {
			c.l.Warn("stream not found in streams",
				zap.String("streams", spew.Sdump("\n", c.streams)),
				zap.Uint16("stream_id", uint16(*sid)),
			)
		} else {
			delete(c.streams, *sid)
		}
		c.streamsMu.Unlock()
	}

	// TODO: Should the whole connection be closed if this is the only stream?!
}

// Close sends close messages to all the (remaining) streams and closes the
// btp.Conn.
func (c *Conn) Close() error {
	// TODO: Close all the (remaining) streams

	// wait for the streams
	c.wg.Wait()

	// close the btp.Conn
	return c.conn.Close()
}

func (c *Conn) sendMessages(
	outCtrl chan []byte,
	outData chan []byte,
) {
	c.wg.Add(1)
	defer c.wg.Done()

loop:
	for {
		time.Sleep(time.Nanosecond * 100) // TODO: Adjust
		select {
		case msg := <-outCtrl:
			_, err := c.conn.Write(msg)
			if err != nil {
				// actually close the whole connection
				c.l.Error("unable to write data", zap.Error(err))
				close(c.close)
				return
			}
			goto loop
		case <-c.close:
			c.l.Info("stop sending messages")
			return
		default:
		}

		select {
		case msg := <-outData:
			_, err := c.conn.Write(msg)
			if err != nil {
				// actually close the whole connection
				c.l.Error("unable to write data", zap.Error(err))
				close(c.close)
				return
			}
		case <-c.close:
			c.l.Info("stop sending messages")
			return
		default:
		}
	}
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
		c.l.Error("unsupported header protocol version",
			zap.String("header_raw", fmt.Sprintf("%08b", uint8(b[0]))),
			zap.Uint8("version", uint8(h.Version)),
			zap.Uint8("type", uint8(h.MessageType)),
		)
		errClose := c.Close()
		if errClose != nil {
			c.l.Error("unable to close connection", zap.Error(errClose))
			return messages.PacketHeader{}, errClose
		}
	}

	return h, nil
}

func (c *Conn) readMsg() (msg messages.BRFTMessage, h messages.PacketHeader, err error) {

	h, err = c.readHeader()
	if err != nil {
		return
	}

	switch h.MessageType {
	case messages.MessageTypeFileReq:
		msg = new(messages.FileReq)
	case messages.MessageTypeFileResp:
		msg = new(messages.FileResp)
	case messages.MessageTypeData:
		msg = new(messages.Data)
	case messages.MessageTypeStartTransmission:
		// TODO: probably remove the timeout - I think we can't because otherwise the connection will forever be idle waiting for remaining bytes
		// decode the packet
		msg = new(messages.StartTransmission)
	case messages.MessageTypeClose:
		msg = new(messages.Close)
	case messages.MessageTypeMetaDataReq:
		msg = new(messages.MetaReq)
	case messages.MessageTypeMetaDataResp:
		msg = new(messages.MetaResp)
	default:
		err = fmt.Errorf("uknown packet header received MessageType=%d", h.MessageType)
		return
	}

	// TODO: probably remove the timeout - I think we can't because otherwise the connection will forever be idle waiting for remaining bytes
	err = msg.Decode(c.l, cyberbyte.NewString(c.conn, cyberbyte.DefaultTimeout))
	if err != nil {
		err = fmt.Errorf("unable to decode %s: %w", msg.Name(), err)
		return
	}

	return
}
