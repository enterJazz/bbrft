package btp

import (
	"bytes"
	"context"
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
	initCwndSize := 1
	maxCwndSize := 10

	return &ConnOptions{
		Network: "udp",

		Version:       messages.ProtocolVersionBTPv1,
		MaxPacketSize: 1024,

		// will be reset when establishing a connection
		InitCwndSize: 1,
		MaxCwndSize:  10,
		CC:           congestioncontrol.NewElasticTcpAlgorithm(l, initCwndSize, maxCwndSize),

		ReadBufferCap: 2048,
	}
}

type packet struct {
	transmissionTime time.Time

	seqNr   PacketNumber
	payload []byte

	rtoDuration time.Duration
	retransmits uint
}

type Conn struct {
	Options *ConnOptions

	conn *net.UDPConn
	// packet read buffer for incoming messages
	buf []byte
	// byte read buffer for ordered messages
	readBuf bytes.Buffer
	// TODO check Kernel who reorders what? (Read vs Conn)

	// conn open is set if the connection has completed the handshake process
	isConnOpen bool

	txMu  sync.Mutex
	rxMu  sync.Mutex
	ackMu sync.Mutex

	// closeChan is used to notify the run loop that it should terminate
	closeChan chan error
	// open channel will be written to after handshake was completed
	// nil = success
	// err = failure
	openChan chan error

	// flag connection as a server side connection
	// required to select correct handshake side
	isServerConnection bool
	isRunLoopRunning   bool
	ctx                context.Context
	ctxCancel          context.CancelFunc
	rttMeasurement     *RTTMeasurement

	txChan chan messages.Codable

	sequentialDataReader  *sequentialDataReader
	packetNumberGenerator packetNumberGenerator
	inflightPackets       map[PacketNumber]*packet // packets currently awaiting acknowledgements

	logger *zap.Logger
}

func Dial(options ConnOptions, laddr *net.UDPAddr, raddr *net.UDPAddr, l *zap.Logger) (c *Conn, err error) {
	conn, err := net.DialUDP(options.Network, laddr, raddr)
	if err != nil {
		return
	}

	c = newConn(conn, options, l)

	err = c.initiateClientHandshake()
	if err != nil {
		c.close(messages.CloseReasonBadRequest)
		return
	}

	c.start()

	openErr := <-c.openChan
	if openErr != nil {
		return nil, openErr
	}

	return
}

func (c *Conn) start() error {
	if c.isRunLoopRunning {
		return errors.New("connection run loop already running")
	}

	c.isRunLoopRunning = true
	go func() {
		err := c.run()
		if err != nil {
			c.logger.Error("run failed", zap.Error(err))
		}
	}()

	return nil
}

func (c *Conn) Write(b []byte) (n int, err error) {
	payload := b
	maxSize := int(c.Options.MaxPacketSize)

	if !c.isConnOpen {
		return 0, ErrConnectionNotReady
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

// Read() returns io.Err if channel closed and read buf cannot be filled completely
// returns num bytes read
func (c *Conn) Read(b []byte) (n int, err error) {
	// assumption: dataChan ordered beforehand by flow ctrl
	for c.readBuf.Len() < len(b) {
		inPacket, err := c.sequentialDataReader.Next()
		if err != nil {
			return 0, err
		}
		c.readBuf.Write(inPacket.Payload)
	}

	n, err = c.readBuf.Read(b)
	if err != nil {
		c.logger.Warn("Read() failed", zap.Error(err))
	}

	return n, err
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

func newConn(conn *net.UDPConn, options ConnOptions, l *zap.Logger) *Conn {
	c := &Conn{
		conn:    conn,
		Options: &options,
		buf:     make([]byte, options.ReadBufferCap),
		logger: l.Named("conn").With(zap.String("ip",
			conn.LocalAddr().String())),
		inflightPackets: make(map[PacketNumber]*packet),
		openChan:        make(chan error),
		closeChan:       make(chan error),
		txChan:          make(chan messages.Codable, options.MaxCwndSize),
		// TODO: use some better buffer defaults
		sequentialDataReader: NewDataReader(100, 50),
	}

	gen, err := NewRandomNumberGenerator()
	if err != nil {
		l.Panic("could not create initial sequence number")
	}
	c.packetNumberGenerator = gen

	c.ctx, c.ctxCancel = context.WithCancel(context.Background())
	c.rttMeasurement = NewRttMeasurement()

	return c
}

func (c *Conn) send(msg messages.Codable) (n int, err error) {
	c.txChan <- msg
	// TODO: update return parameters
	return int(msg.Size()), nil
}

func (c *Conn) transmit(msg messages.Codable) (n int, err error) {
	requiresAck := isAckElicitingPacket(msg)
	l := c.logger
	var p *packet
	c.txMu.Lock()
	defer c.txMu.Unlock()

	if requiresAck {
		p = &packet{
			transmissionTime: time.Now(),
			seqNr:            c.packetNumberGenerator.Peek(),
			retransmits:      0,
			rtoDuration:      c.rttMeasurement.RTO(),
		}
		msg.SetSeqNr(uint16(p.seqNr))
	}

	buf, err := msg.Marshal()
	if err != nil {
		return
	}

	// only create packet for state tracking if not a retransmission
	// only Ack Eliciting packets should be monitored
	if requiresAck {
		p.payload = buf

		// TODO: maybe add sanity check for collisions and number of packets in map
		// dangerous place to leak memory
		c.inflightPackets[p.seqNr] = p
		c.Options.CC.SentMessages(1)
	}

	n, err = c.conn.Write(buf)
	l.Debug("sending", FHeaderMessageTypeString(msg.GetHeader().MessageType), zap.String("raddr", c.conn.RemoteAddr().String()), zap.Int("len", n))

	if err != nil {
		return
	}

	if requiresAck {
		// only increment sequence number if transmission was actually completed
		// otherwise there might be a gap in the sequence numbers
		c.packetNumberGenerator.Next()
	}

	if n != len(buf) {
		return n, io.ErrShortWrite
	}

	return
}

func (c *Conn) retransmit(p *packet) (n int, err error) {
	c.Options.CC.HandleEvent(congestioncontrol.Loss)
	l := c.logger

	c.txMu.Lock()
	defer c.txMu.Unlock()
	p.transmissionTime = time.Now()

	n, err = c.conn.Write(p.payload)
	l.Debug("send lost packet", zap.Int("len", n), zap.Uint16("SeqNr", uint16(p.seqNr)))

	p.retransmits++

	if err != nil {
		return
	}

	return
}

func isAckElicitingPacket(msg messages.Codable) bool {
	// instantiate object depending on header type
	switch msg.GetHeader().MessageType {
	case messages.MessageTypeAck:
		return false
	case messages.MessageTypeConn,
		messages.MessageTypeConnAck,
		messages.MessageTypeData,
		messages.MessageTypeClose:
		return true
	}

	return false
}

// recvMsg reads the next incomming message
// - if message does not match expected type an error is returned
// - message type is asserted via Codable MessageType and MessageVersion headers
func (c *Conn) recv() (msg messages.Codable, err error) {
	c.rxMu.Lock()
	defer c.rxMu.Unlock()

	// l := c.logger
	buf := make([]byte, c.Options.ReadBufferCap)

	n, err := c.conn.Read(buf)
	if err != nil {
		return
	}

	// l.Debug("read packet", zap.Int("len", n))

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
			MessageType:  messages.MessageTypeClose,
		},
		Reason: reason,
	}

	// transmit directly as close must be sent before closing connection
	_, err := c.transmit(msg)
	if err != nil {
		return err
	}

	c.closeChan <- nil
	return nil
}

func (c *Conn) createConnReq() (req *messages.Conn, err error) {

	req = &messages.Conn{
		PacketHeader: messages.PacketHeader{
			ProtocolType: c.Options.Version,
			MessageType:  messages.MessageTypeConn,
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

func (c *Conn) onMessage(msg messages.Codable) error {
	l := c.logger

	l.Debug("incoming msg", FHeaderMessageTypeString(msg.GetHeader().MessageType))

	h := msg.GetHeader()
	// instantiate object depending on header type
	switch h.MessageType {
	case messages.MessageTypeAck:

		if c.isServerConnection && !c.isConnOpen {
			l.Debug("got client response", FSequenceNumber(PacketNumber(msg.GetHeader().SeqNr)))

			ack := msg.(*messages.Ack)
			// ack sequence number must match the current seqNr since no data can be send before connection is open
			if ack.SeqNr != uint16(c.packetNumberGenerator.Peek()) {
				c.openChan <- ErrInvalidClientResp
				return ErrInvalidSeqNr
			}
			l.Debug("got final handshake msg")

			c.isConnOpen = true
			// success
			c.openChan <- nil
			return nil
		}

		err := c.processAck(msg.GetHeader())
		if err != nil {
			c.logger.Error("could not process ack", zap.Error(err))
		}

	case messages.MessageTypeConnAck:
		if c.isServerConnection {
			l.Warn("got invalid packet on server: connAck")
			return nil
		}
		// ignore connacks after connection is open
		// if old connection is invalid we wait for the timeout to close it
		if c.isConnOpen {
			l.Warn("got connAck for open connection")
			return nil
		}

		// treat connack as ack
		err := c.processAck(msg.GetHeader())
		if err != nil {
			c.logger.Error("could not process ack", zap.Error(err))
			c.openChan <- ErrInvalidServerResp
			return err
		}

		resp := msg.(*messages.ConnAck)
		err = c.completeClientHandshake(resp)
		if err != nil {
			c.openChan <- err
			return err
		}

		// store next packet number for server messages
		c.sequentialDataReader.nextSeqNr = PacketNumber(resp.ServerSeqNr)

		c.isConnOpen = true
		// success
		c.openChan <- nil
	case messages.MessageTypeConn:
		// conn packets should only be received by server loop
		l.Warn("got invalid packet in run loop: conn")
		return nil
	case messages.MessageTypeData:
		if !c.isConnOpen {
			return ErrConnectionNotReady
		}
		// TODO: @wlad should we first send ack or process message?
		c.sendAck(msg.GetHeader().SeqNr)
		c.sequentialDataReader.Push(msg.(*messages.Data))
	case messages.MessageTypeClose:
		if !c.isConnOpen {
			return ErrConnectionNotReady
		}
		c.sendAck(msg.GetHeader().SeqNr)
		c.closeChan <- nil
	}

	return nil
}

func (c *Conn) run() error {
	defer c.ctxCancel()

	l := c.logger
	var (
		closeErr error
	)

	go func() {
		for {
			select {
			case <-c.ctx.Done():
				l.Debug("stopping run loop")
				c.isRunLoopRunning = false
				return
			default:
				msg, err := c.recv()
				if err != nil {
					// TODO: ignore close connection during port migration process
					// c.logger.Error("could not read msg", zap.Error(err))
					continue
				}
				err = c.onMessage(msg)
				if err != nil {
					l.Error("message handling error", zap.Error(err))
				}
			}
		}
	}()

runLoop:
	for {
		// Close immediately if requested
		select {
		case closeErr = <-c.closeChan:
			l.Error("connection loop terminated", zap.Error(closeErr))
			c.teardown()
			break runLoop

		default:
		}

		if c.Options.CC.NumFreeSend() > 0 {
			select {
			case msg := <-c.txChan:
				_, err := c.transmit(msg)
				// TODO: maybe take care of failed transmissions due to connection problems
				// IDEA: if transmission fails place packet back on queue
				if err != nil {
					l.Error("message handling error", zap.Error(err))
				}
			default:
				// l.Debug("no outgoing messages")
			}
		}

		err := c.proccessRetransmission()
		if err != nil {
			return err
		}
	}

	return closeErr
}

func (c *Conn) proccessRetransmission() error {
	l := c.logger
	c.ackMu.Lock()
	defer c.ackMu.Unlock()

	// check if packets need retransmission
	for _, p := range c.inflightPackets {
		if time.Since(p.transmissionTime) <= p.rtoDuration {
			continue
		}
		_, err := c.retransmit(p)
		if err != nil {
			l.Warn("failed to retransmit", FSequenceNumber(p.seqNr), zap.Error(err))
		}

		if p.retransmits > 2 {
			return errors.New("connection broken: too many retransmitts")
		}
	}
	return nil
}

func (c *Conn) processAck(h messages.PacketHeader) error {
	l := c.logger
	c.ackMu.Lock()
	defer c.ackMu.Unlock()

	if packet, ok := c.inflightPackets[PacketNumber(h.SeqNr)]; ok {
		delete(c.inflightPackets, PacketNumber(h.SeqNr))

		c.rttMeasurement.Update(packet.transmissionTime, time.Now())
		c.Options.CC.ReceivedAcks(1)

		// only update RTT if no retransmission was performed
		// otherwise since Acks cannot be distinguished
		// measuremnts might be wrong
		if packet.retransmits == 0 {
			c.Options.CC.UpdateRTT(int(c.rttMeasurement.srtt))
		}
	} else {
		l.Warn("got Ack for unknown packet", zap.Uint16("ack_seq_nr", h.SeqNr), FSequenceNumber(c.packetNumberGenerator.Peek()))
	}

	return nil
}

// tears down connections resources
func (c *Conn) teardown() {
	if err := c.conn.Close(); err != nil {
		c.logger.Error("failed to close UDP connection", zap.Error(err))
	}
	c.sequentialDataReader.Close()
	close(c.openChan)
	close(c.closeChan)
	close(c.txChan)
}
