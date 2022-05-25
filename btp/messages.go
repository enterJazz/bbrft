package btp

// TODO: Define message contents - make sure to
//		- not include the length field length when we use it anywhere
//		- use the Builder from golang.org/x/crypto/cryptobyte

type Conn struct {
}

type ConnAck struct {
}

type Ack struct {
}

type Data struct {
}

type Close struct {
}
