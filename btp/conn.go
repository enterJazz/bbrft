package btp

import (
	"bytes"
	"context"
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

	seqNr   uint16
	payload []byte

	rtoDuration time.Duration
	retransmits uint
}

type Conn struct {
	Options *ConnOptions

	conn *net.UDPConn
	// packet read buffer for incoming messages
	buf      []byte
	dataChan chan *messages.Data
	// byte read buffer for ordered messages
	readBuf bytes.Buffer
	// TODO check Kernel who reorders what? (Read vs Conn)

	// conn open is set if the connection has completed the handshake process
	isConnOpen bool

	txMu sync.Mutex
	rxMu sync.Mutex

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

	rxChan chan messages.Codable
	txChan chan messages.Codable

	currentSeqNr    uint16
	inflightPackets map[uint16]*packet // packets currently awaiting acknowledgements

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

// / Read() returns io.Err if channel closed and read buf cannot be filled completely
// / returns num bytes read
func (c *Conn) Read(b []byte) (n int, err error) {
	// TODO @robert
	// reorder buffer / reordering
	// read + order dataChan
	// returns Byte Stream!
	// Issue: which component does what?
	err = nil

	//	if !c.connOpen {
	//		return 0, ErrConnectionNotRead
	//	}

	// assumption: dataChan ordered beforehand by flow ctrl
	for c.readBuf.Len() < len(b) {
		in_packet, ok := <-c.dataChan
		if !ok {
			err = io.EOF
			break
		}
		c.readBuf.Write(in_packet.Payload)
	}

	n, err2 := c.readBuf.Read(b)
	if err2 != nil {
		c.logger.Warn("Read() failed", zap.Error(err2))
		err = err2
	}

	return n, err
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
		inflightPackets: make(map[uint16]*packet),
		openChan:        make(chan error),
		rxChan:          make(chan messages.Codable, options.MaxCwndSize),
		txChan:          make(chan messages.Codable, options.MaxCwndSize),
	}

	c.ctx, c.ctxCancel = context.WithCancel(context.Background())
	c.rttMeasurement = NewRttMeasurement()

	return c
}

func (c *Conn) nextSeqNumber() uint16 {
	return c.currentSeqNr
}

func (c *Conn) incrementSequenceNumber() {
	c.currentSeqNr = c.currentSeqNr + 1
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
			seqNr:            c.nextSeqNumber(),
			retransmits:      0,
			rtoDuration:      c.rttMeasurement.RTO(),
		}
		msg.SetSeqNr(p.seqNr)
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
		c.incrementSequenceNumber()
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
	l.Debug("send lost packet", zap.Int("len", n), zap.Uint16("SeqNr", p.seqNr))

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
			MessageType:  messages.MessageTypeConn,
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
			l.Debug("got client response", zap.Uint16("seq_nr", msg.GetHeader().SeqNr))

			ack := msg.(*messages.Ack)
			// ack sequence number must match the current seqNr since no data can be send before connection is open
			if ack.SeqNr != c.currentSeqNr {
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
		c.sendAck(msg.GetHeader().SeqNr)
	case messages.MessageTypeClose:
		if !c.isConnOpen {
			return ErrConnectionNotReady
		}
		c.sendAck(msg.GetHeader().SeqNr)

		// TODO: handle close
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
				return
			default:
				msg, err := c.recv()
				if err != nil {
					// TODO: ignore close connection during port migration process
					// c.logger.Error("could not read msg", zap.Error(err))
					continue
				}
				c.rxChan <- msg
			}
		}
	}()

runLoop:
	for {
		// Close immediately if requested
		select {
		case closeErr = <-c.closeChan:
			l.Error("connection loop terminated", zap.Error(closeErr))
			break runLoop

		case msg := <-c.rxChan:
			err := c.onMessage(msg)
			if err != nil {
				l.Error("message handling error", zap.Error(err))
			}
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

		// check if packets need retransmission
		for _, p := range c.inflightPackets {
			if time.Since(p.transmissionTime) <= p.rtoDuration {
				continue
			}
			_, err := c.retransmit(p)
			if err != nil {
				l.Warn("failed to retransmit", zap.Uint16("seq_nr", p.seqNr), zap.Error(err))
			}

			if p.retransmits > 2 {
				return errors.New("connection broken: too many retransmitts")
			}
		}
	}

	return closeErr
}

func (c *Conn) processAck(h messages.PacketHeader) error {
	l := c.logger

	if packet, ok := c.inflightPackets[h.SeqNr]; ok {
		delete(c.inflightPackets, h.SeqNr)

		c.rttMeasurement.Update(packet.transmissionTime, time.Now())
		c.Options.CC.ReceivedAcks(1)

		// only update RTT if no retransmission was performed
		// otherwise since Acks cannot be distinguished
		// measuremnts might be wrong
		if packet.retransmits == 0 {
			c.Options.CC.UpdateRTT(int(c.rttMeasurement.srtt))
		}
	} else {
		l.Warn("got Ack for uknown packet", zap.Uint16("ack_seq_nr", h.SeqNr), zap.Uint16("cur_seq_nr", c.currentSeqNr))
	}

	return nil
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
