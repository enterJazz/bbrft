package btp

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"gitlab.lrz.de/bbrft/btp/messages"
	"go.uber.org/zap"
)

const testNetwork = "udp"

func setupConn(t *testing.T) (cl_c, s_c *Conn) {
	l, err := zap.NewDevelopment()
	if err != nil {
		t.Fatal("unable to initialize logger")
	}
	lAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:1337")
	if err != nil {
		t.Errorf("server ResolveUDPAddr error = %v", err)
		return
	}
	ls, err := Listen(*NewDefaultOptions(l), lAddr, l)
	if err != nil {
		t.Errorf("server Listen error = %v", err)
		return
	}

	// use okay to wait for a connection
	ok := false

	go func() {
		t.Log("waiting for incoming messages")
		s_c, err = ls.Accept()
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
	cl_c, err = Dial(*NewDefaultOptions(l), clAddr, lAddr, l)
	if err != nil {
		t.Errorf("Dial error = %v", err)
		return
	}

	for !ok {
		time.Sleep(time.Millisecond * 100)
	}
	return
}

func TestConn(t *testing.T) {
	cl_c, s_c := setupConn(t)
	if cl_c != nil {
		t.Errorf("test failed, client conn: %v", cl_c)
	}
	if s_c != nil {
		t.Errorf("test failed, server conn: %v", s_c)
	}
}

// tests simple read / write between connections
func TestComm(t *testing.T) {
	cl_c, s_c := setupConn(t)
	test_payload := []byte{1, 2, 3, 4, 5, 6}
	read_buf := make([]byte, len(test_payload))

	if _, err := s_c.Write(test_payload); err != nil {
		t.Errorf("Write() failed: %v", err)
	}
	if _, err := cl_c.Read(read_buf); err != nil {
		t.Errorf("Read() failed: %v", err)
	}

	if !bytes.Equal(test_payload, read_buf) {
		t.Errorf("Expected read_buf: %v, Got: %v", test_payload, read_buf)
	}
}

func TestConnMultiple(t *testing.T) {
	l, err := zap.NewDevelopment()
	if err != nil {
		t.Fatal("unable to initialize logger")
	}
	lAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:1338")
	if err != nil {
		t.Errorf("server ResolveUDPAddr error = %v", err)
		return
	}
	ls, err := Listen(*NewDefaultOptions(l), lAddr, l)
	if err != nil {
		t.Errorf("server Listen error = %v", err)
		return
	}

	// use okay to wait for a connection
	acceptedConns := 0
	numConns := 10

	go func() {
		for {

			t.Log("waiting for incoming messages")
			_, err := ls.Accept()
			if err != nil {
				t.Errorf("Accept error = %v", err)
				return
			}

			t.Log("connection accepted", acceptedConns)
			acceptedConns++
		}
	}()

	for i := 0; i < numConns; i++ {
		clAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:"+fmt.Sprintf("%d", 9999+i))
		if err != nil {
			t.Errorf("client ResolveUDPAddr error = %v", err)
			return
		}
		_, err = Dial(*NewDefaultOptions(l), clAddr, lAddr, l)
		if err != nil {
			t.Errorf("Dial error = %v", err)
			return
		}
	}

	for acceptedConns != numConns {
		time.Sleep(time.Millisecond * 100)
	}

}

func TestRead(t *testing.T) {
	l, err := zap.NewDevelopment()
	if err != nil {
		t.Fatal("unable to initialize logger")
	}
	conn := Conn{dataChan: make(chan *messages.Data, 2), logger: l}
	payload := []byte{1, 2, 3, 4}

	test_buf_1 := make([]byte, 2)
	test_buf_2 := make([]byte, 2)

	testDataPacket := messages.NewData(payload)
	conn.dataChan <- testDataPacket

	_, err1 := conn.Read(test_buf_1)
	if err1 != nil {
		t.Errorf("Read() error: %v", err1)
	}

	_, err2 := conn.Read(test_buf_2)
	if err2 != nil {
		t.Errorf("Read() error: %v", err2)
	}

	for i := 0; i < 4; i++ {
		j := i % len(test_buf_1)
		var compare byte
		if i < 2 {
			compare = test_buf_1[j]
		} else {
			compare = test_buf_2[j]
		}
		if payload[i] != compare {
			t.Errorf("Expected: %v, Got: %v", payload[i], compare)
		}
	}

	test_buf_3 := make([]byte, 2)
	in := []byte{5, 6}
	go func() {
		_, err3 := conn.Read(test_buf_3)
		if err3 != nil {
			t.Errorf("Read() error: %v", err3)
		}
	}()
	conn.dataChan <- messages.NewData(in)
	time.Sleep(time.Second * 1)

	for i := 0; i < 2; i++ {
		if test_buf_3[i] != in[i] {
			t.Errorf("Expected: %v, Got: %v", in[i], test_buf_3[i])
		}
	}

	close(conn.dataChan)
	test_buf_4 := make([]byte, 2)
	_, err4 := conn.Read(test_buf_4)
	if err4 != io.EOF {
		t.Errorf("Expected: %v, Got: %v", io.EOF, err4)
	}
}
