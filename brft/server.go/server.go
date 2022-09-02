package server

import (
	"sync"

	"gitlab.lrz.de/bbrft/brft/compression"
	"gitlab.lrz.de/bbrft/btp"
	"go.uber.org/zap"
)

type Server struct {
	l *zap.Logger

	listener *btp.Listener

	// initialized during the handshake
	compressor compression.Compressor

	// initialized during the handshake
	chunkSize int

	numConns int
	numConnsMu sync.Mutex
}

func NewServer(
	l *zap.Logger,
	listener *btp.Listener,
) *Server {
	return &Server{
		l:         l,
		listener:  listener,
		chunkSize: -1,
	}
}

func (s *Server) Close() error {
	return nil
}

func (s *Server) ListenAndServe() error {
	// listen for incomming connections
	s.l.Debug("listening for incomming connections")

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			// we still want to accept future connections
			s.l.Error("failed to accept new connection", zap.Error(err))
			continue
		}

		// handle the connection
		go s.handleConnection(conn)
	}

	return nil
}

func (s *Server) handleConnection(conn *btp.Conn) {

	// extend logging
s.numConnsMu.Lock()
s.numConns += 1
id := s.numConns
s.numConnsMu.Unlock()



	l := s.l.With(
		zap.String("id", id),
		zap.String(conn.)
	)
// create a new connection object
type conn struct {
	*btp.Conn

} 

c := &conn{
	btp.Conn: conn,
}

	// TODO: Wait for incomming messages
	for {
// read the first byte of the message in order to determine the version and type


		b := make([]byte , 1) 

// TODO: Try to read using a timeout
		n, err := c.Read(b)
		if err != nil {
s.l.
		} else if n != 1 {
			// TODO: Handle
		}
		
	}

	return nil
}

// TODO: Probably not needed
// func (s *Server)Serve(l net.Listener) error {
// 	return nil
// }

// func (s *Server)SetKeepAlivesEnabled() error {
// 	return nil
// }

// More graceful shutdown as Close. Might be useful - see: https://pkg.go.dev/net/http#Server.Shutdown
// func (s *Server) Shutdown() error {
// 	return nil
// }
