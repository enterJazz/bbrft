package btp

import (
	"net"
	"time"

	"gitlab.lrz.de/brft/btp/congestioncontrol"
)

// Probably a client for each connection
type Client struct {
	conn *net.UDPConn
	cc   congestioncontrol.CongestionControlAlgorithm
	// TODO: add state that the client needs
}

func NewClient(
	conn *net.UDPConn,
	cc congestioncontrol.CongestionControlAlgorithm,
) net.Conn {
	return &Client{
		conn: conn,
		cc:   cc,
	}
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
