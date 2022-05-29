package btp

import (
	"io"
	"net"
	"time"

	"gitlab.lrz.de/bbrft/btp/congestioncontrol"
	"gitlab.lrz.de/bbrft/btp/messages"
	"gitlab.lrz.de/bbrft/log"
	"go.uber.org/zap"
)

type ClientOptions struct {
	// The maximum number of bytes to send in a single packet.
	MaxPacketSize uint16

	CC congestioncontrol.CongestionControlAlgorithm

	Version messages.ProtocolVersion
}

// Probably a client for each connection
type Client struct {
	l    *zap.Logger
	conn *net.UDPConn
	// TODO: add state that the client needs
	options *ClientOptions
}

func NewDefaultClientOptions(l *zap.Logger) *ClientOptions {
	return &ClientOptions{
		MaxPacketSize: 1024,
		CC:            congestioncontrol.NewLockStepAlgorithm(l),
	}
}

func NewClient(
	l *zap.Logger,
	conn *net.UDPConn,
	options *ClientOptions,
) net.Conn {
	if options == nil {
		options = NewDefaultClientOptions(l)
	}

	return &Client{
		conn:    conn,
		l:       l.With(log.FPeer("client")),
		options: options,
	}
}

func (c *Client) GetOptions() *ClientOptions {
	return c.options
}

// connects to the server specified in the initial setup
func (c *Client) Connect() error {
	connMsg := &messages.Conn{
		PacketHeader: messages.PacketHeader{
			ProtocolType: c.options.Version,
			MessageType:  messages.MessageTypeConn,
		},
	}
	buf, err := connMsg.Marshal()
	if err != nil {
		return err
	}

	n, err := c.Write(buf)
	if err != nil {
		return err
	}
	if n != len(buf) {
		return io.ErrShortWrite
	}

	return nil
}

func (c *Client) Close() error {
	// TODO: implement
	return nil
}

func (c *Client) Read(b []byte) (int, error) {
	// TODO: implement
	return 0, nil
}

func (c *Client) Write(b []byte) (int, error) {
	// TODO: implement
	return 0, nil
}

func (c *Client) LocalAddr() net.Addr {
	// TODO: implement
	return Addr{}
}

func (c *Client) RemoteAddr() net.Addr {
	// TODO: implement
	return Addr{}
}

func (c *Client) SetDeadline(t time.Time) error {
	return c.conn.SetDeadline(t)
}

func (c *Client) SetReadDeadline(t time.Time) error {
	return c.conn.SetReadDeadline(t)
}

func (c *Client) SetWriteDeadline(t time.Time) error {
	return c.conn.SetWriteDeadline(t)
}

// TODO: Other potentially interesting methods
/*
func (c *UDPConn) ReadFrom(b []byte) (int, Addr, error)
func (c *UDPConn) ReadFromUDP(b []byte) (n int, addr *UDPAddr, err error)
func (c *UDPConn) ReadFromUDPAddrPort(b []byte) (n int, addr netip.AddrPort, err error)
func (c *UDPConn) ReadMsgUDP(b, oob []byte) (n, oobn, flags int, addr *UDPAddr, err error)
func (c *UDPConn) ReadMsgUDPAddrPort(b, oob []byte) (n, oobn, flags int, addr netip.AddrPort, err error)
func (c *UDPConn) SetDeadline(t time.Time) error
func (c *UDPConn) SetReadBuffer(bytes int) error
func (c *UDPConn) SetReadDeadline(t time.Time) error
func (c *UDPConn) SetWriteBuffer(bytes int) error
func (c *UDPConn) SetWriteDeadline(t time.Time) error
func (c *UDPConn) Write(b []byte) (int, error)
func (c *UDPConn) WriteMsgUDP(b, oob []byte, addr *UDPAddr) (n, oobn int, err error)
func (c *UDPConn) WriteMsgUDPAddrPort(b, oob []byte, addr netip.AddrPort) (n, oobn int, err error)
func (c *UDPConn) WriteTo(b []byte, addr Addr) (int, error)
func (c *UDPConn) WriteToUDP(b []byte, addr *UDPAddr) (int, error)
func (c *UDPConn) WriteToUDPAddrPort(b []byte, addr netip.AddrPort) (int, error)
*/
