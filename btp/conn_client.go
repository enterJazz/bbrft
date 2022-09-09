package btp

import (
	"errors"
	"net"

	"gitlab.lrz.de/bbrft/btp/messages"
	"go.uber.org/zap"
)

func (c *Conn) initiateClientHandshake() (err error) {
	l := c.logger.Named("handshake").With(zap.String("remote_addr", c.conn.RemoteAddr().String()))
	if c.isConnOpen {
		return errors.New("connection already open")
	}
	req, err := c.createConnReq()
	if err != nil {
		return err
	}

	l.Debug("connecting")

	_, err = c.send(req)
	if err != nil {
		return err
	}

	return
}

func (c *Conn) completeClientHandshake(resp *messages.ConnAck) (err error) {
	l := c.logger.Named("handshake").With(zap.String("remote_addr", c.conn.RemoteAddr().String()))

	if resp.ActualInitCwndSize > c.Options.InitCwndSize {
		return ErrInvalidHandshakeOption("ActualInitCwndSize")
	}

	if resp.ActualMaxCwndSize > c.Options.MaxCwndSize {
		return ErrInvalidHandshakeOption("ActualMaxCwndSize")
	}

	l.Debug("found valid handshake options", zap.Uint16("max_cwnd", uint16(resp.ActualMaxCwndSize)), zap.Uint16("init_cwnd", uint16(resp.ActualInitCwndSize)), zap.Uint16("max_packet_size", uint16(resp.ActualMaxCwndSize)))

	c.Options.MaxCwndSize = resp.ActualMaxCwndSize
	c.Options.InitCwndSize = resp.ActualInitCwndSize
	c.Options.MaxPacketSize = resp.ActualPacketSize

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
	c.sendAck(resp.ServerSeqNr)

	return err
}
