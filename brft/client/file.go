package client

import (
	"bytes"
	"crypto/sha1"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"gitlab.lrz.de/brft/brft/common"
	shared "gitlab.lrz.de/brft/brft/common"
	"gitlab.lrz.de/brft/log"
	"go.uber.org/zap"
)

var fileSignature = []byte{42, 52, 46, 54, 31} // "BRFT1" i.e. BRFT file version 1

type File struct {
	l        *zap.Logger
	f        *os.File
	id       []byte // TODO: probably SHA-1 of the filename?
	name     string
	basePath string
	// checksum is the supposed checksum of the complete file
	checksum []byte // TODO: probably SHA-256 in order to avoid collisions and attacks (?)
}

func NewFile(
	l *zap.Logger,
	name string,
	basePath string,
	// checksum (optional)
	checksum []byte,
) (*File, error) {
	l = l.With(log.FComponent("file"))
	var (
		f   *os.File
		err error
	)

	filePath := filepath.Join(basePath, name)

	// see if the file already exists. If so, load it otherwise create a new file
	if _, err = os.Stat(filePath); errors.Is(err, os.ErrNotExist) {
		if checksum == nil {
			// TODO: make the client checks for the error it should then send the server a request without checksum and
			//		later retry creating the file with a checksum given
			l.Debug("file does not exist and no checksum is provided", zap.String("file_path", filePath))
			return nil, err
		}

		l.Info("creating new file", zap.String("file_path", filePath))

		// get a file desciptor
		f, err = os.OpenFile(filePath, os.O_RDWR|os.O_CREATE, 0755)
		if err != nil {
			return nil, err
		}

		// write the file signature and checksum
		err = writeChecksum(f, checksum)
		if err != nil {
			return nil, fmt.Errorf("unable to write checksum: %w", err)
		}

	} else if err != nil {
		return nil, err
	} else {
		l.Info("opening existing file", zap.String("file_path", filePath))

		// get a file desciptor
		f, err = os.OpenFile(filePath, os.O_RDWR, 0755)
		if err != nil {
			return nil, err
		}

		// read the checksum
		checksum, err = readChecksum(f)
		if err != nil {
			return nil, fmt.Errorf("unable to read checksum: %w", err)
		}
	}

	// create the file id
	id := sha1.Sum([]byte(name))

	return &File{
		l:        l,
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

func (f *File) Write(b []byte) (n int, err error) {
	return f.f.Write(b)
}

func (f *File) GetChecksum() []byte {
	return f.checksum
}

func (f *File) CheckChecksum([]byte) (bool, error) {
	prevOffset, err := f.f.Seek(0, io.SeekCurrent)
	if err != nil {
		return false, err
	}
	// reset the offset at the end
	defer f.f.Seek(prevOffset, io.SeekStart)

	offset := int64(len(fileSignature) + common.ChecksumSize - 1)
	f.f.Seek(offset, io.SeekStart)

	if checksum, err := common.ComputeChecksum(f.f); err != nil {
		return false, err
	} else if bytes.Compare(checksum, f.checksum) != 0 {
		return false, nil
	} else {
		return true, nil
	}
}

// Finish will remove any temporary information from the file. After completion
// the file descriptor will be closed.
func (f *File) Finish() error {
	// remove the magic file signature and the checksum
	return f.stripChecksum()
}

// stripChecksum removes the magic file signature and the checksum. After completion the file descriptor will be closed
func (f *File) stripChecksum() error {
	// go to the start of the file
	_, err := f.f.Seek(0, io.SeekStart)
	if err != nil {
		return err
	}

	// read in the file signature and checksum
	_, err = readChecksum(f.f)
	if err != nil {
		return err
	}

	// write the remainder of the file to a temporary file
	tmpFile, err := ioutil.TempFile(os.TempDir(), "*.brft")
	if err != nil {
		return err
	}

	_, err = io.Copy(tmpFile, f.f)
	if err != nil {
		return err
	}

	tmpFile.Close()
	f.f.Close()

	return nil
}

func readChecksum(f *os.File) ([]byte, error) {
	// make sure the file is actually a pending BRFT file
	sig := make([]byte, len(fileSignature))
	if n, err := f.Read(sig); err != nil {
		return nil, err
	} else if n != len(fileSignature) {
		return nil, ErrInsufficientRead
	} else if bytes.Compare(sig, fileSignature) != 0 {
		return nil, ErrNotABRFTFile
	}

	// TODO: Maybe make lenght delimeted
	checksum := make([]byte, shared.ChecksumSize)
	if n, err := f.Read(checksum); err != nil {
		return nil, err
	} else if n != len(checksum) {
		return nil, ErrInsufficientRead
	}

	return checksum, nil
}

func writeChecksum(f *os.File, checksum []byte) error {
	// TODO: maybe make variable length
	if len(checksum) != shared.ChecksumSize {
		return ErrInvalidChecksum
	}

	n, err := f.Write(fileSignature)
	if err != nil {
		return err
	} else if n != len(checksum) {
		return ErrInsufficientWrite
	}

	n, err = f.Write(checksum)
	if err != nil {
		return err
	} else if n != len(checksum) {
		return ErrInsufficientWrite
	}

	return nil
}
