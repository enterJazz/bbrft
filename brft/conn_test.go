package brft

import (
	"net"
	"testing"

	"gitlab.lrz.de/bbrft/btp"
	"go.uber.org/zap"
)

func TestTransfer(t *testing.T) {
	laddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:1337")
	if err != nil {
		t.Errorf("server ResolveUDPAddr error = %v", err)
		return
	}

	log, _ := zap.NewDevelopment()
	lp, _ := zap.NewProduction()
	l, err := btp.Listen(*btp.NewDefaultOptions(lp), laddr, lp)
	if err != nil {
		t.Error(err)
	}
	s := NewServer(log, l, "./test")

	go func() {
		err := s.ListenAndServe()
		if err != nil {
			panic(err)
		}

	}()

	t.Log(laddr.String())
	c, err := Dial(log, laddr.String(), "./downloads")
	if err != nil {
		t.Error(err)
	}
	err = c.DownloadFile("test2.png", true)
	if err != nil {
		t.Error(err)
	}

	// for {
	// 	time.Sleep(100)
	// }
}
