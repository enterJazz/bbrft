package client

import (
	"gitlab.lrz.de/brft/btp"
	"gitlab.lrz.de/brft/log"
	"go.uber.org/zap"
)

// Probably a client for each connection
type Client struct {
	l *zap.Logger
	c *btp.Client

	// TODO: add state that the client needs
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
