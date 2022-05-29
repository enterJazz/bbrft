package btp

import (
	"errors"
	"net"

	"gitlab.lrz.de/brft/btp/congestioncontrol"
	"gitlab.lrz.de/brft/btp/messages"
	"gitlab.lrz.de/brft/log"
	"go.uber.org/zap"
)

type ServerOptions struct {
	// The maximum number of bytes to send in a single packet.
	MaxPacketSize uint16

	CC congestioncontrol.CongestionControlAlgorithm

	Version ProtocolVersion
}

type ServerConnectionOptions struct {
	// The maximum number of bytes to send in a single packet.
	MaxPacketSize uint16

	CC congestioncontrol.CongestionControlAlgorithm

	Version ProtocolVersion
}

type Connection struct {
	conn   net.Conn
	Config *ServerConnectionOptions
}

// Probably a client for each connection
type Server struct {
	l *zap.Logger
	// TODO: add state that the client needs
	options *ServerOptions
	conn    *net.UDPConn

	// map of clients to udp connections
	conns map[string]Connection
}

func NewDefaultServerOptions(l *zap.Logger) *ServerOptions {
	return &ServerOptions{
		MaxPacketSize: 1024,
		// will be reset when establishing a connection
		CC: congestioncontrol.NewLockStepAlgorithm(l),
	}
}

func NewServer(
	l *zap.Logger,
	conn net.Conn,
	options *ServerOptions,
) *Server {
	if options == nil {
		options = NewDefaultServerOptions(l)
	}

	return &Server{
		conns: map[string]Connection{},
		l: l.With(log.FPeer("server")).
			With(log.FAddr(conn.LocalAddr())).
			With(zap.Uint8("version", uint8(options.Version))).
			With(zap.Uint16("max packet size", options.MaxPacketSize)).
			With(zap.String("congestion-control", options.CC.Name())),
		options: options,
	}
}

func (s *Server) GetOptions() *ServerOptions {
	return s.options
}

func (s *Server) GetConnection(addr net.Addr) *Connection {
	if val, ok := s.conns[addr.String()]; ok {
		return &val
	}

	return nil
}

func (s *Server) setConnection(addr net.Addr, conn *Connection) {
	s.conns[addr.String()] = *conn
}

func (s *Server) removeConnection(addr net.Addr) {
	// TODO: also close connection
	delete(s.conns, addr.String())
}

func (c *Connection) Write(b []byte) (int, error) {
	return c.conn.Write(b)
}

func (s *Server) Listen() error {
	if s.conn == nil {
		s.l.Error("no connection to listen on")
		return errors.New("no connection provided")
	}

	for {
		h, addr, err := messages.ReadHeaderFrom(s.conn)
		if err != nil {
			s.l.Error("error parsing header", log.FAddr(addr), zap.Error(err))
			continue
		}

		l := s.l.With(log.FAddr(addr), FHeaderProtocolVersion(h.ProtocolType), FHeaderMessageType(h.MessageType))
		l.Debug("received udp frame")

		// TODO: define message type fields
		s.l.Debug("received header")

		switch h.MessageType {
		case messages.MessageTypeConn:
			var c messages.Conn
			err = c.Unmarshal(h, s.conn)

			if err != nil {
				s.l.Error("error unmarshalling connection message", zap.Error(err))
				continue
			}

			// find maximal possible packet size
			maxPacketSize := s.options.MaxPacketSize
			if c.MaxPacketSize < maxPacketSize {
				maxPacketSize = c.MaxPacketSize
			}

			// create new connection
			conn := &Connection{
				Config: &ServerConnectionOptions{
					MaxPacketSize: maxPacketSize,
				},
			}

			s.setConnection(addr, conn)

			s.l.Debug("received connection request")

		default:
			// TODO: handle other messages
			s.l.Error("received unknown message type", FHeaderMessageType(h.MessageType))
		}
	}
}
