package btp

import (
	"crypto/rand"
	"encoding/binary"
)

type PacketNumber uint16

type packetNumberGenerator interface {
	Next() PacketNumber
	Peek() PacketNumber
}

type sequentialNumberGenerator struct {
	next PacketNumber
}

func (s *sequentialNumberGenerator) Next() PacketNumber {
	n := s.next
	s.next++
	return n
}

func (s *sequentialNumberGenerator) Peek() PacketNumber {
	return s.next
}

func NewRandomNumberGenerator() (gen *sequentialNumberGenerator, err error) {
	init, err := randSeqNr()
	if err != nil {
		return
	}

	return &sequentialNumberGenerator{
		next: init,
	}, nil
}

// generate a cryptographically secure initial sequence number
func randSeqNr() (PacketNumber, error) {
	var bytes [2]byte

	_, err := rand.Read(bytes[:])
	if err != nil {
		return 0, err
	}

	return PacketNumber(binary.LittleEndian.Uint16(bytes[:])), nil
}
