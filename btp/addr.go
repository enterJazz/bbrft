package btp

import "net"

type Addr struct {
	net.Addr
	// TODO: Add own address identifier(s) - maybe:
	// connectionID int
}
