package brft

import (
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestTransfer(t *testing.T) {

	lpConf := zap.NewProductionConfig()
	lpConf.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	lp, _ := lpConf.Build()

	s, laddr, err := NewServer(lp, "127.0.0.1:1337", "../test/server", nil)
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
	c, err := Dial(lp, laddr.String(), "../test/downloads", nil)
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
