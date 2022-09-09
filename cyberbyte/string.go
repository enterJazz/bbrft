// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package cryptobyte contains types that help with parsing and constructing
// length-prefixed, binary messages, including ASN.1 DER. (The asn1 subpackage
// contains useful ASN.1 constants.)
//
// The String type is for parsing. It wraps a []byte slice and provides helper
// functions for consuming structures, value by value.
//
// The Builder type is for constructing messages. It providers helper functions
// for appending values and also for appending length-prefixed submessages –
// without having to worry about calculating the length prefix ahead of time.
//
// See the documentation and examples for the Builder and String types to get
// started.
package cyberbyte // import "golang.org/x/crypto/cryptobyte"

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"time"
)

// TODO: Remove timeout- it's not used
// String represents a string of bytes. It provides methods for parsing
// fixed-length and length-prefixed values from it.
type String struct {
	io.Reader
	timeout time.Duration
}

const DefaultTimeout time.Duration = time.Second

func NewString(r io.Reader, t time.Duration) *String {
	return &String{
		Reader:  r,
		timeout: t,
	}
}

func (s *String) UpdateTimeout(t time.Duration) {
	s.timeout = t
}

// read advances a String by n bytes and returns them. If less than n bytes
// remain, it returns nil.
func (s *String) read(n int) ([]byte, error) {
	buf := make([]byte, n)
	readN, err := s.Read(buf)
	if err != nil {
		return nil, err
	} else if readN != n || n < 0 {
		return nil, fmt.Errorf("short read (%d != %d)", readN, n)
	}
	return buf[:n], nil
}

// Skip advances the String by n byte and reports whether it was successful.
func (s *String) Skip(n int) error {
	_, err := s.read(n)
	return err
}

// ReadUint8 decodes an 8-bit value into out and advances over it.
// It reports whether the read was successful.
func (s *String) ReadUint8(out *uint8) error {
	v, err := s.read(1)
	if err != nil {
		return err
	}
	*out = uint8(v[0])
	return nil
}

// ReadUint16 decodes a big-endian, 16-bit value into out and advances over it.
// It reports whether the read was successful.
func (s *String) ReadUint16(out *uint16) error {
	v, err := s.read(2)
	if err != nil {
		return err
	}
	*out = uint16(v[0])<<8 | uint16(v[1])
	return nil
}

// ReadUint24 decodes a big-endian, 24-bit value into out and advances over it.
// It reports whether the read was successful.
func (s *String) ReadUint24(out *uint32) error {
	v, err := s.read(3)
	if err != nil {
		return err
	}
	*out = uint32(v[0])<<16 | uint32(v[1])<<8 | uint32(v[2])
	return nil
}

// ReadUint32 decodes a big-endian, 32-bit value into out and advances over it.
// It reports whether the read was successful.
func (s *String) ReadUint32(out *uint32) error {
	v, err := s.read(4)
	if err != nil {
		return err
	}
	*out = uint32(v[0])<<24 | uint32(v[1])<<16 | uint32(v[2])<<8 | uint32(v[3])
	return nil
}

// ReadUint64 decodes a big-endian, 64-bit value into out and advances over it.
// It reports whether the read was successful.
func (s *String) ReadUint64(out *uint64) error {
	v, err := s.read(8)
	if err != nil {
		return err
	}
	*out = uint64(v[0])<<56 | uint64(v[1])<<48 | uint64(v[2])<<40 | uint64(v[3])<<32 |
		uint64(v[4])<<24 | uint64(v[5])<<16 | uint64(v[6])<<8 | uint64(v[7])
	return nil
}

// NOTE: not needed by us
// func (s *String) readUnsigned(out *uint32, length int) error {
// 	v := s.read(length)
// 	if v == nil {
// 		return false
// 	}
// 	var result uint32
// 	for i := 0; i < length; i++ {
// 		result <<= 8
// 		result |= uint32(v[i])
// 	}
// 	*out = result
// 	return true
// }

func (s *String) readLengthPrefixed(lenLen int, outChild *[]byte) error {
	lenBytes, err := s.read(lenLen)
	if err != nil {
		return err
	}
	var length uint32
	for _, b := range lenBytes {
		length = length << 8
		length = length | uint32(b)
	}
	v, err := s.read(int(length))
	if err != nil {
		return err
	}
	*outChild = v
	return nil
}

// ReadUint8LengthPrefixed reads the content of an 8-bit length-prefixed value
// into out and advances over it. It reports whether the read was successful.
func (s *String) ReadUint8LengthPrefixed(out *String) error {
	var b []byte
	err := s.readLengthPrefixed(1, &b)
	if err != nil {
		return err
	}
	*out = *NewString(bytes.NewReader(b), s.timeout)
	return nil
}

// ReadUint16LengthPrefixed reads the content of a big-endian, 16-bit
// length-prefixed value into out and advances over it. It reports whether the
// read was successful.
func (s *String) ReadUint16LengthPrefixed(out *String) error {
	var b []byte
	err := s.readLengthPrefixed(2, &b)
	if err != nil {
		return err
	}
	*out = *NewString(bytes.NewReader(b), s.timeout)
	return nil
}

// ReadUint24LengthPrefixed reads the content of a big-endian, 24-bit
// length-prefixed value into out and advances over it. It reports whether
// the read was successful.
func (s *String) ReadUint24LengthPrefixed(out *String) error {
	var b []byte
	err := s.readLengthPrefixed(3, &b)
	if err != nil {
		return err
	}
	*out = *NewString(bytes.NewReader(b), s.timeout)
	return nil
}

// ReadUint8LengthPrefixed reads the content of an 8-bit length-prefixed value
// into out and advances over it. It reports whether the read was successful.
func (s *String) ReadUint8LengthPrefixedBytes(out *[]byte) error {
	return s.readLengthPrefixed(1, out)
}

// ReadUint16LengthPrefixed reads the content of a big-endian, 16-bit
// length-prefixed value into out and advances over it. It reports whether the
// read was successful.
func (s *String) ReadUint16LengthPrefixedBytes(out *[]byte) error {
	return s.readLengthPrefixed(2, out)
}

// ReadUint24LengthPrefixed reads the content of a big-endian, 24-bit
// length-prefixed value into out and advances over it. It reports whether
// the read was successful.
func (s *String) ReadUint24LengthPrefixedBytes(out *[]byte) error {
	return s.readLengthPrefixed(3, out)
}

// ReadBytes reads n bytes into out and advances over them. It reports
// whether the read was successful.
func (s *String) ReadBytes(out *[]byte, n int) error {
	v, err := s.read(n)
	if err != nil {
		return err
	}
	*out = v
	return nil
}

// CopyBytes copies len(out) bytes into out and advances over them. It reports
// whether the copy operation was successful
func (s *String) CopyBytes(out []byte) error {
	n := len(out)
	v, err := s.read(n)
	if err != nil {
		return err
	}
	if copy(out, v) != n {
		return errors.New("short copy")
	}
	return nil
}

// NOTE: there is no way to do this using the reader
// Empty reports whether the string does not contain any bytes.
// func (s String) Empty() bool {
// 	return len(s) == 0
// }
