package server

import (
	"crypto/sha1"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gitlab.lrz.de/brft/brft/shared"
)

var fileSignature = []byte{42, 52, 46, 54, 31} // "BRFT1" i.e. BRFT file version 1

type File struct {
	f        *os.File
	id       []byte // TODO: probably SHA-1 of the filename?
	name     string
	basePath string
	// checksum is the supposed checksum of the complete file
	checksum []byte // TODO: probably SHA-256 in order to avoid collisions and attacks (?)
}

func NewFile(
	name string,
	basePath string,
) (*File, error) {
	// TODO: decide whether the file should be stored under it's human-readable or hashed name
	// see if the file already exists. If so, load it otherwise create a new file
	if _, err := os.Stat(filepath.Join(basePath, name)); errors.Is(err, os.ErrNotExist) {
		// TODO: make the server check for the error!
		return nil, err
	} else if err != nil {
		return nil, err
	}

	// get a file desciptor
	f, err := os.OpenFile(filepath.Join(basePath, name), os.O_RDONLY, 0755)
	if err != nil {
		return nil, err
	}

	// compute the checksum
	checksum, err := shared.ComputeChecksum(f)
	if err != nil {
		return nil, fmt.Errorf("unable to compute checksum: %w", err)
	}

	// reset the file descriptor (only possible since we do not open the file with O_APPEND)
	_, err = f.Seek(0, io.SeekStart)
	if err != nil {
		return nil, fmt.Errorf("cannot reset file descriptor: %w", err)
	}

	id := sha1.Sum([]byte(name))

	return &File{
		f:        f,
		id:       id[:],
		name:     name,
		basePath: basePath,
		checksum: checksum,
	}, nil
}

func (f *File) Close() (err error) {
	return f.f.Close()
}

func (f *File) Read(b []byte) (n int, err error) {
	return f.f.Read(b)
}

func (f *File) GetChecksum() []byte {
	return f.checksum[:]
}
