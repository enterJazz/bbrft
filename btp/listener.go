package btp

import (
	"bytes"
	"io"
	"net"

	"gitlab.lrz.de/bbrft/btp/messages"
	"go.uber.org/zap"
)

type Listener struct {
	// udp conn used to listen for incomming connection requests
	conn    *net.UDPConn
	options *ConnOptions
	laddr   *net.UDPAddr

	logger *zap.Logger
}

func (ls *Listener) Accept() (*Conn, error) {
	l := ls.logger
	c, err := ls.doServerHandshake()
	if err != nil {
		return nil, err
	}

	l.Debug("connection accepted", zap.String("local_addr", c.conn.LocalAddr().String()), zap.String("remote_addr", c.conn.RemoteAddr().String()))
	return c, nil
}

func Listen(options ConnOptions, laddr *net.UDPAddr, logger *zap.Logger) (l *Listener, err error) {
	conn, err := net.ListenUDP(options.Network, laddr)
	if err != nil {
		return
	}

	return &Listener{
		conn:    conn,
		options: &options,
		laddr:   laddr,
		logger:  logger.Named("listener").With(zap.String("ip", conn.LocalAddr().String())),
	}, nil
}

func (l *Listener) recvConnFrom() (msg *messages.Conn, addr *net.UDPAddr, err error) {
	msg = &messages.Conn{}

	buf := make([]byte, msg.Size())
	n, addr, err := l.conn.ReadFromUDP(buf)
	if err != nil {
		return
	}

	if uint(n) != msg.Size() {
		err = io.ErrUnexpectedEOF
		return
	}

	h, err := messages.ParseHeader(buf)
	if err != nil {
		return nil, addr, err
	}

	r := bytes.NewReader(buf[messages.HeaderSize:])
	err = msg.Unmarshal(h, r)

	return
}

func (ls *Listener) createConnResp(conn *net.UDPConn, req *messages.Conn) (resp *messages.ConnAck, err error) {
	resp = &messages.ConnAck{
		PacketHeader: messages.PacketHeader{
			ProtocolType: ls.options.Version,
			MessageType:  messages.MessageTypeConnAck,
			SeqNr:        req.SeqNr,
			Flags:        2,
		},
		MigrationPort:      uint16(conn.LocalAddr().(*net.UDPAddr).Port),
		ActualInitCwndSize: req.InitCwndSize,
		ActualMaxCwndSize:  req.MaxCwndSize,
		ActualPacketSize:   req.MaxPacketSize,
	}

	if req.InitCwndSize > ls.options.InitCwndSize {
		resp.ActualInitCwndSize = ls.options.InitCwndSize
	}

	if req.MaxCwndSize > ls.options.MaxCwndSize {
		resp.ActualMaxCwndSize = ls.options.MaxCwndSize
	}

	if req.MaxPacketSize > ls.options.MaxPacketSize {
		resp.ActualPacketSize = ls.options.MaxPacketSize
	}

	return
}

// dialMigrateConn creates a new UDP connection with the next free port on the host machine
// remote client address is maintained in the new connection
func (ls *Listener) dialMigratedConn(addr *net.UDPAddr) (c *Conn, err error) {
	laddr, err := net.ResolveUDPAddr(ls.options.Network, "0.0.0.0:0")
	if err != nil {
		return nil, err
	}

	conn, err := net.DialUDP(ls.options.Network, laddr, addr)
	if err != nil {
		return nil, err
	}

	c = newConn(conn, *ls.options, ls.logger)
	c.isServerConnection = true

	return
}

func (ls *Listener) doServerHandshake() (c *Conn, err error) {
	// TODO: only read messages from new parties here
	req, raddr, err := ls.recvConnFrom()
	if err != nil {
		return
	}

	l := ls.logger.Named("handshake").With(zap.String("raddr", raddr.String()))
	l.Debug("new connection req", zap.Uint("len", req.Size()))

	c, err = ls.dialMigratedConn(raddr)
	if err != nil {
		return
	}
	l.Debug("migrating local port", zap.Uint("len", req.Size()), zap.String("new_local_addr", c.conn.LocalAddr().String()))

	// try negotiating a valid option
	resp, err := ls.createConnResp(c.conn, req)
	if err != nil {
		return
	}
	resp.ServerSeqNr = uint16(c.packetNumberGenerator.Peek())

	// TODO: @wlad cleanup sequence number handling
	c.sequentialDataReader.nextSeqNr = PacketNumber(req.SeqNr) + 1

	buf, err := resp.Marshal()
	if err != nil {
		return nil, err
	}

	// transmit ConnAck on incomming conennection
	// client must respond to the new port
	_, err = ls.conn.WriteToUDP(buf, raddr)
	if err != nil {
		return nil, err
	}

	// start run loop
	err = c.start()
	if err != nil {
		return nil, err
	}

	l.Debug("waiting for client response")
	openErr := <-c.openChan
	if openErr != nil {
		return nil, openErr
	}

	l.Debug("handshake completed")
	return
}
