package brft

import (
	"gitlab.lrz.de/brft/btp"
)

// Probably a client for each connection
type Client struct {
	c *btp.Client

	// TODO: add state that the client needs
}

func NewClient(
	client *btp.Client,
) *Client {
	return &Client{
		c: client,
	}
}

func (c *Client) Do() {
	// TODO: implement
}
