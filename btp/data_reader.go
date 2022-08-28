package btp

import (
	"io"
	"sync"

	"gitlab.lrz.de/bbrft/btp/messages"
)

type sequentialDataReader struct {
	dataChan chan *messages.Data

	nextSeqNr PacketNumber
	mu        sync.Mutex

	buf []*messages.Data
}

func NewDataReader(chanCapacity uint, bufCapacity uint) *sequentialDataReader {
	return &sequentialDataReader{
		dataChan: make(chan *messages.Data, chanCapacity),
		buf:      make([]*messages.Data, bufCapacity),
	}
}

func (s *sequentialDataReader) Push(data *messages.Data) {
	s.dataChan <- data
}

// read data packets until we have the next sequential packet
func (s *sequentialDataReader) Next() (data *messages.Data, err error) {
	// lock during while reading or writing
	s.mu.Lock()
	defer s.mu.Unlock()
	// enable to visually debug state
	//defer func() { fmt.Println(s.String()); s.mu.Unlock() }()

	// check if we already have the next packet in the buffer
	nextIdx := uint(int(s.nextSeqNr) % len(s.buf))
	if item := s.buf[nextIdx]; item != nil {
		s.buf[nextIdx] = nil
		s.nextSeqNr++
		return item, nil
	}

	for {

		newData, ok := <-s.dataChan
		if !ok {
			return nil, io.EOF
		}

		// sanity check for nil in channel
		if newData == nil || !ok {
			continue
		}

		// if we got the directly next packet no need to reorder just pass it to consumer
		if s.nextSeqNr == PacketNumber(newData.SeqNr) {
			s.nextSeqNr++
			return newData, nil
		}
		idx := uint(int(newData.SeqNr) % len(s.buf))
		s.buf[idx] = newData

		// TODO: @wlad think about number circle rotations
		continue
	}
}

func (s *sequentialDataReader) String() string {
	str := "[ "
	for i, v := range s.buf {
		if v == nil {
			str += " "
		} else {
			str += "x"
		}

		if i < len(s.buf)-1 {
			str += " | "
		}
	}

	str += "]"
	return str
}
