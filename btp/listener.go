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
}

func (ls *Listener) Accept() (*Conn, error) {
	l := ls.options.Logger
	c, err := ls.doServerHandshake()
	if err != nil {
		return nil, err
	}

	l.Debug("connection accepted", zap.String("local_addr", c.conn.LocalAddr().String()), zap.String("remote_addr", c.conn.RemoteAddr().String()))
	return c, nil
}

func Listen(options ConnOptions, laddr *net.UDPAddr) (l *Listener, err error) {
	conn, err := net.ListenUDP(options.Network, laddr)
	if err != nil {
		return
	}

	// TODO: move logger from options
	options.Logger = options.Logger.Named("listener").With(zap.String("ip", conn.LocalAddr().String()))

	return &Listener{
		conn:    conn,
		options: &options,
		laddr:   laddr,
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
	initSeqNr, err := randSeqNr()
	if err != nil {
		return
	}
	resp = &messages.ConnAck{
		PacketHeader: messages.PacketHeader{
			ProtocolType: ls.options.Version,
			MessageType:  messages.MessageTypeConnAck,
			SeqNr:        initSeqNr,
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
	conn, err := net.DialUDP(ls.options.Network, nil, addr)
	if err != nil {
		return nil, err
	}

	c = NewConn(conn, *ls.options)

	return
}

func (ls *Listener) doServerHandshake() (c *Conn, err error) {
	// TODO: only read messages from new parties here
	req, raddr, err := ls.recvConnFrom()
	if err != nil {
		return
	}

	l := ls.options.Logger.Named("handshake").With(zap.String("source_addr", raddr.String()))
	l.Debug("new incomming request", zap.Uint("len", req.Size()))

	c, err = ls.dialMigratedConn(raddr)
	if err != nil {
		return
	}
	l.Debug("migrating", zap.Uint("len", req.Size()), zap.String("new_local_addr", c.conn.LocalAddr().String()))

	// try negotiating a valid option
	resp, err := ls.createConnResp(c.conn, req)
	if err != nil {
		return
	}

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

	// receive client ack on new port
	msg, err := c.recv()

	l.Debug("got client response", zap.Uint8("msg_type", uint8(msg.GetHeader().MessageType)), zap.Uint16("seq_nr", msg.GetHeader().SeqNr))

	if msg.GetHeader().MessageType != messages.MessageTypeAck {
		return nil, ErrInvalidClientResp
	}

	ack := msg.(*messages.Ack)
	if ack.SeqNr != resp.SeqNr {
		return nil, ErrInvalidSeqNr
	}

	return
}
