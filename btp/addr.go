package btp

import "net"

type Addr struct {
	net.UDPAddr
	// TODO: Add own address identifier(s) - maybe:
	// connectionID int
}
