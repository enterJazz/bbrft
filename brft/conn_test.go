package brft

import (
	"net"
	"testing"
	"time"

	"gitlab.lrz.de/bbrft/btp"
	"gitlab.lrz.de/bbrft/log"
)

func TestTransfer(t *testing.T) {
	laddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:1337")
	if err != nil {
		t.Errorf("server ResolveUDPAddr error = %v", err)
		return
	}

	ld, _ := log.NewLogger()
	lp, _ := log.NewLogger(log.WithProd(true))

	l, err := btp.Listen(*btp.NewDefaultOptions(lp), laddr, lp)
	if err != nil {
		t.Error(err)
	}
	s := NewServer(ld, l, "../test/server")

	go func() {
		err := s.ListenAndServe()
		if err != nil {
			panic(err)
		}

	}()

	t.Log(laddr.String())
	c, err := Dial(ld, laddr.String(), "../test/downloads")
	if err != nil {
		t.Error(err)
	}
	err = c.DownloadFile("test.jpg", true)
	if err != nil {
		t.Error(err)
	}

	for {
		time.Sleep(100)
	}
}
