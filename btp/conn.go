package btp

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sync"

	"gitlab.lrz.de/bbrft/btp/congestioncontrol"
	"gitlab.lrz.de/bbrft/btp/messages"
	"go.uber.org/zap"
)

type ConnOptions struct {
	Logger *zap.Logger
	// The network must be a UDP network name; see func Dial for details.
	Network string

	// The maximum number of bytes to send in a single packet.
	MaxPacketSize uint16

	// TODO: move to CC
	MaxCwndSize  uint8
	InitCwndSize uint8

	CC congestioncontrol.CongestionControlAlgorithm

	Version messages.ProtocolVersion

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

	// TODO: handle invalid versions currently only skips reading
	if h.ProtocolType != c.Options.Version {
		// TODO: wlad unify errors
		return nil, errors.New("received invalid packet version")
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
