package btp

import (
	"fmt"
	"net"
	"testing"
	"time"

	"go.uber.org/zap"
)

// TODO: Write test for marshal and unmarshal individually

func TestConn(t *testing.T) {
	l, err := zap.NewDevelopment()
	if err != nil {
		t.Fatal("unable to initialize logger")
	}
	lAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:1337")
	if err != nil {
		t.Errorf("server ResolveUDPAddr error = %v", err)
		return
	}
	ls, err := Listen(*NewDefaultOptions(l), lAddr)
	if err != nil {
		t.Errorf("server Listen error = %v", err)
		return
	}

	// use okay to wait for a connection
	ok := false

	go func() {
		t.Log("waiting for incoming messages")
		_, err := ls.Accept()
		if err != nil {
			t.Errorf("Accept error = %v", err)
			return
		}

		t.Log("connection accepted")
		ok = true
	}()

	clAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:9999")
	if err != nil {
		t.Errorf("client ResolveUDPAddr error = %v", err)
		return
	}
	_, err = Dial(*NewDefaultOptions(l), clAddr, lAddr)
	if err != nil {
		t.Errorf("Dial error = %v", err)
		return
	}

	for !ok {
		time.Sleep(time.Millisecond * 100)
	}

}

func TestConnMultiple(t *testing.T) {
	l, err := zap.NewDevelopment()
	if err != nil {
		t.Fatal("unable to initialize logger")
	}
	lAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:1337")
	if err != nil {
		t.Errorf("server ResolveUDPAddr error = %v", err)
		return
	}
	ls, err := Listen(*NewDefaultOptions(l), lAddr)
	if err != nil {
		t.Errorf("server Listen error = %v", err)
		return
	}

	// use okay to wait for a connection
	acceptedCoons := 0
	numConns := 100

	go func() {
		for {

			t.Log("waiting for incoming messages")
			_, err := ls.Accept()
			if err != nil {
				t.Errorf("Accept error = %v", err)
				return
			}

			t.Log("connection accepted", acceptedCoons)
			acceptedCoons++
		}
	}()

	for i := 0; i < numConns; i++ {
		clAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:"+fmt.Sprintf("%d", 9999+i))
		if err != nil {
			t.Errorf("client ResolveUDPAddr error = %v", err)
			return
		}
		_, err = Dial(*NewDefaultOptions(l), clAddr, lAddr)
		if err != nil {
			t.Errorf("Dial error = %v", err)
			return
		}
	}

	for acceptedCoons != numConns {
		time.Sleep(time.Millisecond * 100)
	}

}
