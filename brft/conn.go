package brft

import (
	"fmt"
	"sync"

	"github.com/davecgh/go-spew/spew"
	"gitlab.lrz.de/bbrft/brft/messages"
	"gitlab.lrz.de/bbrft/btp"
	"gitlab.lrz.de/bbrft/cyberbyte"
	"go.uber.org/zap"
)

type metaDataReqRespMsg struct {
	resp messages.MetaResp
	err  error
}

type metaDataRequestRef struct {
	resp_chan chan metaDataReqRespMsg
}

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

	// metadata request storage, used to correctly correlate incoming metadata requests with
	// incoming responses
	metaDataRequestMu sync.Mutex
	metaDataRequests  []metaDataRequestRef

	// buffers for sending data
	outCtrl chan []byte
	outData chan []byte

	options ConnOptions
}

// Close sends close messages to all the (remaining) streams and closes the
// btp.Conn.
func (c *Conn) Close() error {
	defer c.l.Info("closed connection")

	c.l.Debug("closing connection")
	// close the close channel without closing it multiple times. This signals
	// to all streams of this connection to shutdown
	select {
	case <-c.close:
		// channel is closed, this is executed
	default:
		// channel is still open, the previous case is not executed
		close(c.close)
	}

	// wait for the streams
	c.l.Debug("waiting for streams...")
	c.wg.Wait()
	c.l.Debug("all streams shut down")

	// clean up data of the Conn struct
	close(c.outCtrl)
	close(c.outData)

	c.l.Debug("closing btp connection")
	// TODO: It could be that we try to close the connection after it has
	// 		already been closed (and returned an error on Read/Write)
	// close the btp.Conn
	return c.conn.Close()
}

func (c *Conn) sendMessages(
	outCtrl chan []byte,
	outData chan []byte,
) {
	c.wg.Add(1)
	defer c.wg.Done()
	defer c.l.Info("stop sending messages")

loop:
	for {
		select {
		case msg := <-outCtrl:
			_, err := c.conn.Write(msg)
			if err != nil {
				// actually close the whole connection
				c.l.Error("unable to write data - closing connection",
					zap.Error(err),
				)
				c.Close()
				return
			}
			goto loop
		case <-c.close:
			return
		default:
		}

		select {
		case msg := <-outData:
			_, err := c.conn.Write(msg)
			if err != nil {
				// actually close the whole connection
				c.l.Error("unable to write data - closing connection",
					zap.Error(err),
				)
				c.Close()
				return
			}
		case <-c.close:
			return
		default:
		}
	}
}

func (c *Conn) sendClosePacket(s messages.StreamID, r messages.CloseReason) {
	cl := &messages.Close{
		StreamID: s,
		Reason:   r,
	}

	data, err := cl.Encode(c.l)
	if err != nil {
		c.l.Error("unable to encode FileResponse",
			zap.String("packet", spew.Sdump("\n", cl)),
			zap.Error(err),
		)

		// we're already trying to close the stream, nothing else to do here
		c.l.Error("unable to send Close packet",
			zap.String("stream_id", s.String()),
			zap.Error(err),
		)
		return
	}

	// send the data to the sender routing
	// NOTE: Since the Close packet is also used to signalize that a download
	// has been completed, it cannot be sent on the Ctrl channel. Otherwise, the
	// close packet would arrive before the remaining Data packets
	select {
	case c.outData <- data:
	case <-c.close:
		c.l.Info("closing connection - canceling sending close packet")
		return
	}
}

func (c *Conn) readHeader() (messages.PacketHeader, error) {
	// read the message header
	b := make([]byte, 1)
	_, err := c.conn.Read(b)
	if err != nil {
		c.l.Error("unable to read message header - closing", zap.Error(err))

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

	// actually decode the packet
	err = msg.Decode(c.l, cyberbyte.NewString(c.conn, cyberbyte.DefaultTimeout))
	if err != nil {
		err = fmt.Errorf("unable to decode %s: %w", msg.Name(), err)
		return
	}

	return
}
