package btp

import (
	"bytes"
	"fmt"
	"math/rand"
	"net"
	"testing"
	"time"

	"gitlab.lrz.de/bbrft/btp/messages"
	"go.uber.org/zap"
)

const testNetwork = "udp"

func setupConn(t *testing.T) (cl_c, s_c *Conn) {
	l, err := zap.NewProduction()
	if err != nil {
		t.Fatal("unable to initialize logger")
	}
	lAddr, err := net.ResolveUDPAddr(testNetwork, "127.0.0.1:1337")
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

	clAddr, err := net.ResolveUDPAddr(testNetwork, "127.0.0.1:9999")
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
	if cl_c == nil {
		t.Errorf("test failed, client conn: %v", cl_c)
	}
	if s_c == nil {
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

// tests simple read / write between connections
func TestLargeComm(t *testing.T) {
	client, server := setupConn(t)
	testPayload := make([]byte, 2*1024*1024)
	_, err := rand.Read(testPayload)
	if err != nil {
		t.Errorf("rand.Read() failed: %v", err)
	}

	readBuf := make([]byte, len(testPayload))
	startTime := time.Now()

	go func() {
		if _, err := server.Write(testPayload); err != nil {
			t.Errorf("Write() failed: %v", err)
		}
	}()

	if _, err := client.Read(readBuf); err != nil {
		t.Errorf("Read() failed: %v", err)
	}
	fmt.Printf("Speed %f Mbit/Sec \n", float64(len(testPayload)/1024/1024)/(float64(time.Since(startTime))/float64(time.Second))*8)

	if !bytes.Equal(testPayload, readBuf) {
		t.Errorf("Buffers do not match")
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
	sdr := NewDataReader(2, 4)
	conn := Conn{sequentialDataReader: sdr, logger: l}
	payload := []byte{1, 2, 3, 4}

	testBuf1 := make([]byte, 2)
	testBuf2 := make([]byte, 2)

	testDataPacket := messages.NewData(payload)
	sdr.Push(testDataPacket)

	_, err1 := conn.Read(testBuf1)
	if err1 != nil {
		t.Errorf("Read() error: %v", err1)
	}

	_, err2 := conn.Read(testBuf2)
	if err2 != nil {
		t.Errorf("Read() error: %v", err2)
	}

	for i := 0; i < 4; i++ {
		j := i % len(testBuf1)
		var compare byte
		if i < 2 {
			compare = testBuf1[j]
		} else {
			compare = testBuf2[j]
		}
		if payload[i] != compare {
			t.Errorf("Expected: %v, Got: %v", payload[i], compare)
		}
	}

	testBuf3 := make([]byte, 2)
	in := []byte{5, 6}
	sdr.nextSeqNr = 0
	go func() {
		_, err3 := conn.Read(testBuf3)
		if err3 != nil {
			t.Errorf("Read() error: %v", err3)
		}
	}()
	sdr.Push(messages.NewData(in))
	time.Sleep(time.Millisecond * 100)

	for i := 0; i < 2; i++ {
		if testBuf3[i] != in[i] {
			t.Errorf("Expected: %v, Got: %v", in[i], testBuf3[i])
		}
	}

	// close(conn.dataChan)
	// test_buf_4 := make([]byte, 2)
	// _, err4 := conn.Read(test_buf_4)
	// if err4 != io.EOF {
	// 	t.Errorf("Expected: %v, Got: %v", io.EOF, err4)
	// }
}
