package btp

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sync"
	"time"

	"gitlab.lrz.de/bbrft/btp/congestioncontrol"
	"gitlab.lrz.de/bbrft/btp/messages"
	"go.uber.org/zap"
)

type ConnOptions struct {
	Logger *zap.Logger
	// The network must be a UDP network name; see func Dial for details.
	Network string

	// The maximum number of bytes to send in a single packet.
	Version       messages.ProtocolVersion
	MaxPacketSize uint16
	// TODO: move to CC
	MaxCwndSize  uint8
	InitCwndSize uint8
	CC           congestioncontrol.CongestionControlAlgorithm

	ReadBufferCap uint
}

func NewDefaultOptions(l *zap.Logger) *ConnOptions {
	return &ConnOptions{
		Network: "udp",
		Logger:  l,

		Version:       messages.ProtocolVersionBTPv1,
		MaxPacketSize: 1024,

		// will be reset when establishing a connection
		InitCwndSize: 1,
		MaxCwndSize:  10,
		CC:           congestioncontrol.NewLockStepAlgorithm(l),

		ReadBufferCap: 2048,
	}
}

type Conn struct {
	Options *ConnOptions

	conn *net.UDPConn
	// read buffer for incomming messages
	buf []byte

	// conn open is set if the connection has completed the handshake process
	connOpen bool

	rwMu sync.RWMutex
}

func NewConn(conn *net.UDPConn, options ConnOptions) *Conn {
	c := &Conn{
		conn:    conn,
		Options: &options,
		buf:     make([]byte, options.ReadBufferCap),
	}

	// TODO: move logger from options
	options.Logger = options.Logger.Named("conn").With(zap.String("ip",
		conn.LocalAddr().String()))

	return c
}

func Dial(options ConnOptions, laddr *net.UDPAddr, raddr *net.UDPAddr) (c *Conn, err error) {
	conn, err := net.DialUDP(options.Network, laddr, raddr)
	if err != nil {
		return
	}

	c = NewConn(conn, options)

	err = c.doClientHandshake()
	if err != nil {
		c.close(messages.CloseReasonBadRequest)
		return
	}

	return
}

func (c *Conn) Write(b []byte) (n int, err error) {
	payload := b
	maxSize := int(c.Options.MaxPacketSize)

	if !c.connOpen {
		return 0, ErrConnectionNotRead
	}
	if len(b) == 0 {
		return
	}

	for {
		if len(payload) < maxSize {
			return c.send(messages.NewData(payload))
		}

		lenSend, err := c.send(messages.NewData(payload[0:maxSize]))

		n += lenSend
		if err != nil {
			return n, err
		}

		payload = payload[maxSize:]
	}

}

func (c *Conn) Read(b []byte) (n int, err error) {
	if !c.connOpen {
		return 0, ErrConnectionNotRead
	}

	return 0, nil
}

// RemoteAddr returns the remote network address. The Addr returned is shared
// by all invocations of RemoteAddr, so do not modify it.
func (c *Conn) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}

// LocalAddr returns the local network address. The Addr returned is shared by
// all invocations of LocalAddr, so do not modify it.
func (c *Conn) LocalAddr() net.Addr {
	return c.conn.LocalAddr()
}

// TODO: accoring to our rfc the BRFT layer is not concerned with timeouts. maybe remove timeouts again
// SetDeadline implements the Conn SetDeadline method.
//
// SetDeadline sets the read and write deadlines associated
// with the connection. It is equivalent to calling both
// SetReadDeadline and SetWriteDeadline.
//
// A deadline is an absolute time after which I/O operations
// fail instead of blocking. The deadline applies to all future
// and pending I/O, not just the immediately following call to
// Read or Write. After a deadline has been exceeded, the
// connection can be refreshed by setting a deadline in the future.
//
// If the deadline is exceeded a call to Read or Write or to other
// I/O methods will return an error that wraps os.ErrDeadlineExceeded.
// This can be tested using errors.Is(err, os.ErrDeadlineExceeded).
// The error's Timeout method will return true, but note that there
// are other possible errors for which the Timeout method will
// return true even if the deadline has not been exceeded.
//
// An idle timeout can be implemented by repeatedly extending
// the deadline after successful Read or Write calls.
//
// A zero value for t means I/O operations will not time out.
func (c *Conn) SetDeadline(t time.Time) error {
	if !c.connOpen {
		return ErrConnectionNotRead
	}

	return c.conn.SetDeadline(t)
}

// SetReadDeadline implements the Conn SetReadDeadline method.
//
// SetReadDeadline sets the deadline for future Read calls
// and any currently-blocked Read call.
// A zero value for t means Read will not time out.
func (c *Conn) SetReadDeadline(t time.Time) error {
	if !c.connOpen {
		return ErrConnectionNotRead
	}

	return c.conn.SetReadDeadline(t)
}

// SetWriteDeadline implements the Conn SetWriteDeadline method.
//
// SetWriteDeadline sets the deadline for future Write calls
// and any currently-blocked Write call.
// Even if write times out, it may return n > 0, indicating that
// some of the data was successfully written.
// A zero value for t means Write will not time out.
func (c *Conn) SetWriteDeadline(t time.Time) error {
	if !c.connOpen {
		return ErrConnectionNotRead
	}

	return c.conn.SetWriteDeadline(t)
}

// Close will close a connection
// NOTE: it should be only used by higher layers tearing down the connection
func (c *Conn) Close() error {
	return c.close(messages.CloseResonsDisconnect)
}

func (c *Conn) withLogger() *zap.Logger {
	return c.Options.Logger
}

func (c *Conn) send(msg messages.Codable) (n int, err error) {
	c.rwMu.Lock()
	defer c.rwMu.Unlock()

	l := c.withLogger()
	buf, err := msg.Marshal()
	if err != nil {
		return
	}

	n, err = c.conn.Write(buf)

	l.Debug("written bytes", zap.Int("len", n))

	if err != nil {
		return
	}

	if n != len(buf) {
		return n, io.ErrShortWrite
	}

	return
}

// recvMsg reads the next incomming message
// - if message does not match expected type an error is returned
// - message type is asserted via Codable MessageType and MessageVersion headers
func (c *Conn) recv() (msg messages.Codable, err error) {
	c.rwMu.RLock()
	defer c.rwMu.RUnlock()

	l := c.Options.Logger
	buf := make([]byte, c.Options.ReadBufferCap)

	n, err := c.conn.Read(buf)
	if err != nil {
		return
	}

	l.Debug("read packet", zap.String("IP", c.conn.LocalAddr().String()), zap.Int("len", n))

	if n < messages.HeaderSize {
		err = io.ErrUnexpectedEOF
		return
	}

	h, err := messages.ParseHeader(buf)
	if err != nil {
		return nil, err
	}

	// instantiate object depending on header type
	switch h.MessageType {
	case messages.MessageTypeAck:
		msg = &messages.Ack{}
	case messages.MessageTypeConn:
		msg = &messages.Conn{}
	case messages.MessageTypeConnAck:
		msg = &messages.ConnAck{}
	case messages.MessageTypeData:
		msg = &messages.Data{}
	case messages.MessageTypeClose:
		msg = &messages.Close{}
	default:
		// TODO: wlad unify errors
		return nil, errors.New("unexpected message type")
	}

	r := bytes.NewReader(buf[messages.HeaderSize:])
	err = msg.Unmarshal(h, r)

	return
}

// close is an package internal close method that allows to forward extended close reasons to the other party
func (c *Conn) close(reason messages.CloseResons) error {
	msg := &messages.Close{
		PacketHeader: messages.PacketHeader{
			ProtocolType: c.Options.Version,
			MessageType:  messages.MessageTypeConn,
			// TODO: add SeqNr handling
		},
		Reason: reason,
	}

	_, err := c.send(msg)
	if err != nil {
		return err
	}

	return c.conn.Close()
}

func (c *Conn) createConnReq() (req *messages.Conn, err error) {
	initSeqNr, err := randSeqNr()
	if err != nil {
		return
	}

	req = &messages.Conn{
		PacketHeader: messages.PacketHeader{
			ProtocolType: c.Options.Version,
			MessageType:  messages.MessageTypeConn,
			SeqNr:        initSeqNr,
		},

		// connection configutation
		MaxPacketSize: c.Options.MaxPacketSize,

		// CC options
		InitCwndSize: c.Options.InitCwndSize,
		MaxCwndSize:  c.Options.MaxCwndSize,
	}

	return
}

func (c *Conn) sendAck(seqNr uint16) error {
	_, err := c.send(&messages.Ack{
		PacketHeader: messages.PacketHeader{
			ProtocolType: c.Options.Version,
			MessageType:  messages.MessageTypeAck,
			SeqNr:        seqNr,
		},
	})
	return err
}

// execute connection handshake
func (c *Conn) doClientHandshake() (err error) {
	l := c.Options.Logger.Named("handshake").With(zap.String("remote_addr", c.conn.RemoteAddr().String()))

	l.Debug("connecting")

	req, err := c.createConnReq()
	if err != nil {
		return err
	}

	_, err = c.send(req)
	if err != nil {
		return err
	}

	msg, err := c.recv()
	if err != nil {
		return err
	}

	if msg.GetHeader().MessageType != messages.MessageTypeConnAck {
		return ErrInvalidServerResp
	}
	resp := msg.(*messages.ConnAck)

	// TODO: handle server and client sequence numbers here

	if resp.ActualInitCwndSize > c.Options.InitCwndSize {
		return ErrInvalidHandshakeOption("ActualInitCwndSize")
	}

	if resp.ActualMaxCwndSize > c.Options.MaxCwndSize {
		return ErrInvalidHandshakeOption("ActualMaxCwndSize")
	}

	l.Debug("found valid handshake options", zap.Uint16("max_cwnd", uint16(resp.ActualMaxCwndSize)), zap.Uint16("init_cwnd", uint16(resp.ActualInitCwndSize)), zap.Uint16("max_packet_size", uint16(resp.ActualMaxCwndSize)))

	// migrate remote port to new location
	c.conn.Close()

	// construct new remote addr
	raddr := c.conn.RemoteAddr().(*net.UDPAddr)
	raddr.Port = int(resp.MigrationPort)

	l.Debug("migrating remote addr", zap.String("new_remote_addr", raddr.String()))

	// create new connection with migration port
	newConn, err := net.DialUDP(c.Options.Network, c.conn.LocalAddr().(*net.UDPAddr), raddr)
	if err != nil {
		return
	}
	c.conn = newConn
	c.sendAck(resp.SeqNr)

	c.connOpen = true
	return err
}

// generate a cryptographically secure initial sequence number
// TODO: move somewhere else
func randSeqNr() (uint16, error) {
	var bytes [2]byte

	_, err := rand.Read(bytes[:])
	if err != nil {
		return 0, err
	}

	return binary.LittleEndian.Uint16(bytes[:]), nil
}
