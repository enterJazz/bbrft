package server

import (
	"gitlab.lrz.de/bbrft/brft/compression"
	"gitlab.lrz.de/bbrft/btp"
)

type Server struct {
	conn btp.Conn

	// initialized during the handshake
	compressor compression.Compressor

	// initialized during the handshake
	chunkSize int
}

func NewServer(
	conn btp.Conn,
) *Server {
	return &Server{
		conn:      conn,
		chunkSize: -1,
	}
}
