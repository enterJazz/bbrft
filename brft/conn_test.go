package brft

import (
	"testing"
	"time"

	"gitlab.lrz.de/bbrft/log"
)

func TestTransfer(t *testing.T) {

	ld, _ := log.NewLogger()
	lp, err := log.NewLogger(log.WithProd(true))
	opt := &ServerOptions{NewDefaultOptions(lp)}
	s, laddr, err := NewServer(ld, "127.0.0.1:1337", "../test/server", opt)
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		err := s.ListenAndServe()
		if err != nil {
			t.Fatal(err)
		}

	}()

	t.Log(laddr.String())
	c, err := Dial(ld, laddr.String(), "../test/downloads", nil)
	if err != nil {
		t.Error(err)
	}
	err = c.DownloadFile("test.jpg")
	if err != nil {
		t.Error(err)
	}

	for {
		time.Sleep(100)
	}
}
