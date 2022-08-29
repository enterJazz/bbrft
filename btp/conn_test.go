package btp

import (
	"bytes"
	"fmt"
	"io"
	"math/rand"
	"net"
	"testing"
	"time"

	"gitlab.lrz.de/bbrft/btp/messages"
	"go.uber.org/zap"
)

const testNetwork = "udp"
const mitmReadBufferCap = 2048
const clientAddrStr = "127.0.0.1:9999"
const serverAddrStr = "127.0.0.1:1337"
const mitmClLAddrStr = "127.0.0.1:9000"
const mitmSLAddrStr = "127.0.0.1:9001"

// MitM relays messages between communicating parties, whereby it may drop and / or reorder packets
type MitM struct {
	t        *testing.T
	clLAddr  *net.UDPAddr
	sLAddr   *net.UDPAddr
	clAddr   *net.UDPAddr
	sAddr    *net.UDPAddr
	clConn   *net.UDPConn
	sConn    *net.UDPConn
	dropProb float64 // probability of dropping a packet
}

func NewMitM(t *testing.T, clLAddrStr, sLAddrStr, clAddrStr, sAddrStr *string, dropProb float64) *MitM {
	addrStrs := []*string{clLAddrStr, sLAddrStr, clAddrStr, sAddrStr}
	udpAddrs := make([]*net.UDPAddr, len(addrStrs))
	for i, addrStr := range addrStrs {
		addr, err := net.ResolveUDPAddr(testNetwork, *addrStr)
		if err != nil {
			t.Errorf("ResolveUDPAddr error of %v = %v", addrStr, err)
			return nil
		}
		udpAddrs[i] = addr
	}

	if !(0 <= dropProb && dropProb <= 1) {
		t.Errorf("dropProb must be between 0 and 1; got: %v", dropProb)
	}

	m := &MitM{
		t:        t,
		clLAddr:  udpAddrs[0],
		sLAddr:   udpAddrs[1],
		clAddr:   udpAddrs[2],
		sAddr:    udpAddrs[3],
		dropProb: dropProb,
	}
	go m.run()
	return m
}

func (m *MitM) run() {
	m.t.Logf("mitm connecting to %v from %v", m.sAddr, m.sLAddr)
	// connect to server
	sConn, err := net.DialUDP(testNetwork, m.sLAddr, m.sAddr)
	if err != nil {
		m.t.Errorf("Failed to connect to server: %v; retrying...", err)
	}
	m.sConn = sConn
	// expect conn from client
	m.t.Logf("mitm listening at %v", m.clLAddr)
	clConn, err := net.ListenUDP(testNetwork, m.clLAddr)
	if err != nil {
		return
	}
	m.clConn = clConn
	m.t.Logf("mitm connected to by %v", clConn)

	go func() {
		for {
			err := m.probRelay(m.sAddr, true)
			if err != nil {
				m.t.Errorf("Relay error = %v", err)
			}
		}
	}()
	go func() {
		for {
			err := m.probRelay(m.clAddr, false)
			if err != nil {
				m.t.Errorf("Relay error = %v", err)
			}
		}
	}()
}

func (m *MitM) probRelay(targetAddr *net.UDPAddr, connector bool) (err error) {
	// read source message
	buf := make([]byte, mitmReadBufferCap)
	srcSock := m.getSrcSock(connector)
	_, err = srcSock.Read(buf)
	if err != nil {
		return
	}
	m.t.Logf("Relaying packet from %v to %v", m.getSrcSock(connector).LocalAddr(), m.getTargetSock(connector).LocalAddr())

	// TODO: @robert if ConnAck: also migrate mitm connection
	h, err := messages.ParseHeader(buf)
	if err != nil {
		m.t.Errorf("error while parsing header: %v", err)
		return
	}

	var msg messages.Codable
	// instantiate object depending on header type
	switch h.MessageType {
	case messages.MessageTypeAck:
		msg = &messages.Ack{}
	case messages.MessageTypeConn:
		msg = &messages.Conn{}
	case messages.MessageTypeConnAck:
		msg = &messages.ConnAck{}
	case messages.MessageTypeData:
		msg = &messages.Data{}
	case messages.MessageTypeClose:
		msg = &messages.Close{}
	default:
		m.t.Error("unexpected message type")
	}

	r := bytes.NewReader(buf[messages.HeaderSize:])
	err = msg.Unmarshal(h, r)
	if err != nil {
		m.t.Errorf("Unmarshal() err: %v", err)
	}

	// migrate mitm connection from server connAck
	switch h.MessageType {
	case messages.MessageTypeConnAck:
		connAck := msg.(*messages.ConnAck)
		// close migrate mitm conn to server
		sourceSock := m.getSrcSock(connector)
		sourceSock.Close()
		raddr := sourceSock.RemoteAddr().(*net.UDPAddr)
		raddr.Port = int(connAck.MigrationPort)
		m.t.Logf("mitm remigrating server conn to %v from %v", raddr, m.sLAddr)
		newConn, err1 := net.DialUDP(testNetwork, m.sLAddr, raddr)
		if err1 != nil {
			m.t.Errorf("migration err: %v", err1)
		}
		m.sConn = newConn
		// change migration port to this port (client - mitm remains unchanged)
		connAck.MigrationPort = m.clLAddr.AddrPort().Port()
		buf, err = connAck.Marshal()
		if err != nil {
			m.t.Errorf("Marshal() error: %v", err)
		}
	default:
		// decide wether to drop
		// TODO: @robert replace with Markov chain (OR only in final tests)
		rand := rand.Float64()
		if !(m.dropProb < rand) {
			// drop packet
			m.t.Logf("dropping packet: %v >= %v", m.dropProb, rand)
			return
		}
	}

	// forward packet to target
	var n int
	targetSock := m.getTargetSock(connector)
	if connector {
		n, err = targetSock.Write(buf)
	} else {
		n, err = targetSock.WriteToUDP(buf, targetAddr)
	}
	if err != nil {
		return
	}
	if n != len(buf) {
		return io.ErrShortWrite
	}
	return
}

func setupConn(t *testing.T, l *zap.Logger, sourceAddrStr, targetAddrStr *string) (cl_c, s_c *Conn) {
	lAddr, err := net.ResolveUDPAddr(testNetwork, *targetAddrStr)
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

	clAddr, err := net.ResolveUDPAddr(testNetwork, *sourceAddrStr)
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

func (m *MitM) getSrcSock(connector bool) *net.UDPConn {
	if connector {
		return m.clConn
	} else {
		return m.sConn
	}
}

func (m *MitM) getTargetSock(connector bool) *net.UDPConn {
	return m.getSrcSock(!connector)
}

func setupDefaultConn(t *testing.T) (cl_c, s_c *Conn) {
	l, err := zap.NewDevelopment()
	if err != nil {
		t.Fatal("unable to initialize logger")
	}
	manualClientAddrStr := clientAddrStr
	manualServerAddrStr := serverAddrStr
	return setupConn(t, l, &manualClientAddrStr, &manualServerAddrStr)
}

func setupLossyConn(t *testing.T, dropProb float64) (cl_c, s_c *Conn) {
	l, err := zap.NewDevelopment()
	if err != nil {
		t.Fatal("unable to initialize logger")
	}
	manualMitmClLAddrStr := mitmClLAddrStr
	manualMitmSLAddrStr := mitmSLAddrStr
	manualClientAddrStr := clientAddrStr
	manualServerAddrStr := serverAddrStr

	// setup server
	lAddr, err := net.ResolveUDPAddr(testNetwork, manualServerAddrStr)
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

	// setup mitm
	_ = NewMitM(t, &manualMitmClLAddrStr, &manualMitmSLAddrStr, &manualClientAddrStr, &manualServerAddrStr, dropProb)
	mitmAddr, err := net.ResolveUDPAddr(testNetwork, manualMitmClLAddrStr)

	// connect client to mitm
	time.Sleep(time.Second * 1)
	clAddr, err := net.ResolveUDPAddr(testNetwork, manualClientAddrStr)
	if err != nil {
		t.Errorf("client ResolveUDPAddr error = %v", err)
		return
	}
	cl_c, err = Dial(*NewDefaultOptions(l), clAddr, mitmAddr, l)
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
	cl_c, s_c := setupDefaultConn(t)
	if cl_c == nil {
		t.Errorf("test failed, client conn: %v", cl_c)
	}
	if s_c == nil {
		t.Errorf("test failed, server conn: %v", s_c)
	}
}

func TestLossyConn(t *testing.T) {
	cl_c, s_c := setupLossyConn(t, 0)
	if cl_c == nil {
		t.Errorf("test failed, client conn: %v", cl_c)
	}
	if s_c == nil {
		t.Errorf("test failed, server conn: %v", s_c)
	}
}

// tests simple read / write between connections
func TestComm(t *testing.T) {
	cl_c, s_c := setupDefaultConn(t)
	testComm(t, cl_c, s_c)
}

// tests lossy read / write between connections
func TestLossyComm(t *testing.T) {
	dropProb := 0.1
	cl_c, s_c := setupLossyConn(t, dropProb)
	testComm(t, cl_c, s_c)
}

func testComm(t *testing.T, cl_c, s_c *Conn) {
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

func TestLargeComm(t *testing.T) {
	client, server := setupDefaultConn(t)
	testLargeComm(t, client, server)
}

func TestLossyLargeComm(t *testing.T) {
	dropProb := 0.01
	client, server := setupLossyConn(t, dropProb)
	testLargeComm(t, client, server)
}

// tests simple read / write between connections
func testLargeComm(t *testing.T, client, server *Conn) {
	testPayload := make([]byte, 2*1024*1024)
	_, err := rand.Read(testPayload)
	if err != nil {
		t.Errorf("rand.Read() failed: %v", err)
	}

	readBuf := make([]byte, len(testPayload))

	go func() {
		if _, err := server.Write(testPayload); err != nil {
			t.Errorf("Write() failed: %v", err)
		}
	}()

	startTime := time.Now()
	if _, err := client.Read(readBuf); err != nil {
		t.Errorf("Read() failed: %v", err)
	}
	fmt.Printf("Server -> Client transfer speed %.2f Mbit/Sec \n", float64(len(testPayload)/1024/1024)/(float64(time.Since(startTime))/float64(time.Second))*8)

	if !bytes.Equal(testPayload, readBuf) {
		t.Errorf("Buffers do not match")
	}

	go func() {
		if _, err := client.Write(testPayload); err != nil {
			t.Errorf("Write() failed: %v", err)
		}
	}()

	startTime = time.Now()
	if _, err := server.Read(readBuf); err != nil {
		t.Errorf("Read() failed: %v", err)
	}
	fmt.Printf("Client -> Server transfer speed %.2f Mbit/Sec \n", float64(len(testPayload)/1024/1024)/(float64(time.Since(startTime))/float64(time.Second))*8)
}

func TestParallelDataTransfer(t *testing.T) {
	client, server := setupDefaultConn(t)

	testPayload1 := make([]byte, 20*1024)
	testPayload2 := make([]byte, 20*1024)

	clientReadBuf := make([]byte, len(testPayload1))
	serverReadBuf := make([]byte, len(testPayload2))

	clientReady := false
	serverReady := false

	go func() {
		if _, err := server.Write(testPayload1); err != nil {
			t.Errorf("server.Write() failed: %v", err)
		}
	}()

	go func() {
		if _, err := client.Write(testPayload2); err != nil {
			t.Errorf("client.Write() failed: %v", err)
		}
	}()

	go func() {
		if _, err := server.Read(serverReadBuf); err != nil {
			t.Errorf("server.Read() failed: %v", err)
		}

		serverReady = true
	}()

	go func() {
		if _, err := server.Read(clientReadBuf); err != nil {
			t.Errorf("client.Read() failed: %v", err)
		}

		clientReady = true
	}()

	for !serverReady && !clientReady {
		time.Sleep(time.Millisecond * 100)
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
