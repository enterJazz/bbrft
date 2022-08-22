package btp

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"io"
	"net"

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
}

func NewDefaultOptions(l *zap.Logger) *ConnOptions {
	return &ConnOptions{
		Logger: l,

		MaxPacketSize: 1024,

		// will be reset when establishing a connection
		InitCwndSize: 1,
		MaxCwndSize:  10,
		CC:           congestioncontrol.NewLockStepAlgorithm(l),
	}
}

type Conn struct {
	conn    *net.UDPConn
	Options *ConnOptions
}

type Listener struct {
	options *ConnOptions
	laddr   *net.UDPAddr
}

func (c *Conn) send(msg messages.Codable) (n int, err error) {
	buf, err := msg.Marshal()
	if err != nil {
		return
	}

	n, err = c.conn.Write(buf)
	if err != nil {
		return
	}

	if n != len(buf) {
		return n, io.ErrShortWrite
	}

	return
}

func (c *Conn) recvHeader() (h messages.PacketHeader, err error) {
	buf := make([]byte, messages.HeaderSize)
	n, err := c.conn.ReadFromUDP(buf)

	if err != nil {
		return
	}

	if n != messages.HeaderSize {
		err = io.ErrUnexpectedEOF
		return
	}

	h, err = messages.ParseHeader(buf)
	return
}

// recvMsg reads the next incomming message
// - if message does not match expected type an error is returned
// - message type is asserted via Codable MessageType and MessageVersion headers
func (c *Conn) recv() (msg messages.Codable, err error) {
	h, err := c.recvHeader()
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

	err = msg.Unmarshal(h, c.conn)

	return
}

// Close will close a connection
// NOTE: it should be only used by higher layers tearing down the connection
func (c *Conn) Close() error {
	return c.close(messages.CloseResonsDisconnect)
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

func (c *Conn) doServerHandshake() (err error) {
	// TODO: only read messages from new parties hier
	msg, err := c.recv()
	if err != nil {
		return err
	}

	// TODO: improve message type check
	if msg.GetHeader().MessageType != messages.MessageTypeConn {
		return nil
	}

	connReq := msg.(*messages.Conn)

	initSeqNr, err := randSeqNr()
	if err != nil {
		return
	}
	connAck := &messages.ConnAck{
		PacketHeader: messages.PacketHeader{
			ProtocolType: c.Options.Version,
			MessageType:  messages.MessageTypeConnAck,
			// TODO: add SeqNr handling
			SeqNr: initSeqNr,
		},
	}

	if connReq.InitCwndSize > c.Options.InitCwndSize {
		connAck.ActualInitCwndSize = c.Options.InitCwndSize
	}

	if connReq.MaxCwndSize > c.Options.MaxCwndSize {
		connAck.ActualMaxCwndSize = c.Options.MaxCwndSize
	}

	if connReq.MaxPacketSize > c.Options.MaxPacketSize {
		connAck.ActualPacketSize = c.Options.MaxPacketSize
	}

	// receive client ack
	msg, err = c.recv()

	if msg.GetHeader().MessageType != messages.MessageTypeAck {
		// TODO: wlad unify errors
		return errors.New("invalid client response")
	}

	ack := msg.(*messages.Ack)
	if ack.SeqNr != initSeqNr {
		// TODO: wlad unify errors
		return errors.New("invalid sequence number response")
	}

	return
}

// execute connection handshake
func (c *Conn) doClientHandshake() (err error) {
	initSeqNr, err := randSeqNr()
	if err != nil {
		return
	}

	connMsg := &messages.Conn{
		PacketHeader: messages.PacketHeader{
			ProtocolType: c.Options.Version,
			MessageType:  messages.MessageTypeConn,
			// TODO: add SeqNr handling
			SeqNr: initSeqNr,
		},

		// connection configutation
		MaxPacketSize: c.Options.MaxPacketSize,

		// CC options
		InitCwndSize: c.Options.InitCwndSize,
		MaxCwndSize:  c.Options.MaxCwndSize,
	}

	_, err = c.send(connMsg)
	if err != nil {
		return err
	}

	msg, err := c.recv()
	if err != nil {
		return err
	}

	if msg.GetHeader().MessageType != messages.MessageTypeConnAck {
		// TODO: wlad unify errors
		return errors.New("invalid server response")
	}
	connAck := msg.(*messages.ConnAck)

	// TODO: handle server and client sequence numbers here

	// TODO: validate values according to PROTOCOL SPEC for now simply use defaults
	if connAck.ActualInitCwndSize > c.Options.InitCwndSize {
		// TODO: wlad unify errors
		return errors.New("invalid ActualInitCwndSize value from remote")
	}

	if connAck.ActualMaxCwndSize > c.Options.MaxCwndSize {
		// TODO: wlad unify errors
		return errors.New("invalid ActualMaxCwndSize value from remote")
	}

	_, err = c.send(&messages.Ack{
		PacketHeader: messages.PacketHeader{
			ProtocolType: c.Options.Version,
			MessageType:  messages.MessageTypeAck,
			SeqNr:        connAck.SeqNr,
		},
	})

	return nil
}

func (l *Listener) Accept() (*Conn, error) {
	udpConn, err := net.ListenUDP(l.options.Network, l.laddr)
	if err != nil {
		return nil, err
	}

	// initiallize empty btp connection
	c := &Conn{
		conn:    udpConn,
		Options: l.options,
	}

	err = c.doServerHandshake()
	if err != nil {
		return nil, err
	}

	return c, nil
}

func Listen(options ConnOptions, laddr *net.UDPAddr) (l *Listener, err error) {
	if err != nil {
		return
	}
	return &Listener{
		options: &options,
		laddr:   laddr,
	}, nil
}

func Dial(options ConnOptions, laddr *net.UDPAddr, raddr *net.UDPAddr) (conn *Conn, err error) {
	udpConn, err := net.DialUDP(options.Network, laddr, raddr)

	conn = &Conn{
		conn:    udpConn,
		Options: &options,
	}

	err = conn.doClientHandshake()

	if err != nil {
		conn.close(messages.CloseReasonBadRequest)
		return
	}

	return
}
