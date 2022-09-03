package btp

import (
	"io"
	"testing"
	"time"

	"gitlab.lrz.de/bbrft/btp/messages"
)

func TestDataReaderSequentialInput(t *testing.T) {
	r := NewDataReader(20, 5)
	r.nextSeqNr = 0
	numPackets := 3

	for i := 0; i < numPackets; i++ {
		r.dataChan <- &messages.Data{
			PacketHeader: messages.PacketHeader{SeqNr: uint16(i)},
		}
	}

	for i := 0; i < numPackets; i++ {
		p, err := r.Next()
		if err != nil {
			t.Errorf("Next() error = %d", err)
		}
		if p.SeqNr != uint16(i) {
			t.Errorf("out of order packet received want = %d, have = %d", i, p.SeqNr)
		}
	}
}

func TestDataReaderOutOfOrderInput(t *testing.T) {
	r := NewDataReader(20, 5)
	r.nextSeqNr = 0
	numPackets := 3
	done := false

	go func() {
		for i := 0; i < numPackets; i++ {
			p, err := r.Next()
			if err != nil {
				t.Errorf("Next() error = %d", err)
			}
			if p.SeqNr != uint16(i) {
				t.Errorf("out of order packet received want = %d, have = %d", i, p.SeqNr)
			}
		}
		done = true
	}()

	for i := numPackets; i >= 0; i-- {
		r.dataChan <- &messages.Data{
			PacketHeader: messages.PacketHeader{SeqNr: uint16(i)},
		}
	}

	for !done {
		time.Sleep(time.Millisecond * 100)
	}
}

func TestDataReaderChannelClose(t *testing.T) {
	r := NewDataReader(20, 5)
	close(r.dataChan)
	_, err := r.Next()
	if err != io.EOF {
		t.Errorf("Expected: %v, Got: %v", io.EOF, err)
	}
}
