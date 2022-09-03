package server

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"gitlab.lrz.de/bbrft/brft/common"
)

var fileSignature = []byte{42, 52, 46, 54, 31} // "BRFT1" i.e. BRFT file version 1

type File struct {
	f        *os.File
	stat     fs.FileInfo
	name     string
	basePath string
	// checksum is the supposed checksum of the complete file
	checksum []byte // TODO: probably SHA-256 in order to avoid collisions and attacks (?)
}

// FIXME: Make sure the name matches the regex
func NewFile(
	name string,
	basePath string,
) (*File, error) {
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

	// TODO: would be probably nicer to store the checksum somewhere
	// compute the checksum
	checksum, err := common.ComputeChecksum(f)
	if err != nil {
		return nil, fmt.Errorf("unable to compute checksum: %w", err)
	}

	// reset the file descriptor (only possible since we do not open the file with O_APPEND)
	_, err = f.Seek(0, io.SeekStart)
	if err != nil {
		return nil, fmt.Errorf("cannot reset file descriptor: %w", err)
	}

	fs, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("cannot get file stats: %w", err)
	}

	return &File{
		f:        f,
		stat:     fs,
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

func (f *File) Checksum() []byte {
	return f.checksum
}

func (f *File) Size() uint64 {
	return uint64(f.stat.Size())
}
