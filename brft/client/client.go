package client

import (
	"gitlab.lrz.de/bbrft/brft/compression"
	"gitlab.lrz.de/bbrft/btp"
	"gitlab.lrz.de/bbrft/log"
	"go.uber.org/zap"
)

// Probably a client for each connection
type Client struct {
	l *zap.Logger
	c *btp.Client
	// TODO: add state that the client needs

	// initialized during the handshake
	compressor compression.Compressor

	// initialized during the handshake
	chunkSize int
}

func NewClient(
	l *zap.Logger,
	client *btp.Client,
) *Client {
	return &Client{
		l: l.With(log.FPeer("client")),
		c: client,
	}
}

func (c *Client) Do() {
	// TODO: implement
}
