package brft

import (
	"testing"
	"time"

	"gitlab.lrz.de/bbrft/log"
)

func setupTest(t *testing.T) *Conn {
	ld, _ := log.NewLogger()
	lp, err := log.NewLogger(log.WithProd(false))
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
	optD := NewDefaultOptions(lp)
	c, err := Dial(ld, laddr.String(), "../test/server", &optD)
	if err != nil {
		t.Error(err)
	}
	return c
}

func TestTransfer(t *testing.T) {

	c := setupTest(t)
	err := c.DownloadFile("test.jpg")
	if err != nil {
		t.Error(err)
	}

	for {
		time.Sleep(time.Nanosecond * 100)
	}
}

func TestMetaData(t *testing.T) {
	c := setupTest(t)

	if err := c.ListFileMetaData(""); err != nil {
		t.Error(err)
	}

	if err := c.ListFileMetaData("test.jpg"); err != nil {
		t.Error(err)
	}

	for {
		time.Sleep(time.Nanosecond * 100)
	}
}
