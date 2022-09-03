package brft

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"io/ioutil"
	"os"
	"path/filepath"

	"gitlab.lrz.de/bbrft/brft/common"
	shared "gitlab.lrz.de/bbrft/brft/common"
	"gitlab.lrz.de/bbrft/log"
	"go.uber.org/zap"
)

var fileSignature = []byte{42, 52, 46, 54, 31} // "BRFT1" i.e. BRFT file version 1

var (
	ErrNoClientFile = errors.New("file is no client file")
)

// TODO: maybe we should include the expected filesize to the client file
type File struct {
	l    *zap.Logger
	f    *os.File
	stat fs.FileInfo
	// files on the recipient end of the connection will have the checksum
	// prepended. This is used for resumptions
	isClientFile bool
	name         string
	basePath     string
	// checksum is the supposed checksum of the complete file
	checksum []byte
}

// NewFile creates a new file using the given information. It should be used
// only by the client
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

	return &File{
		l: l,
		f: f,
		// FIXME: stat: fs,
		isClientFile: true,
		name:         name,
		basePath:     basePath,
		checksum:     checksum,
	}, nil
}

// FIXME: Make sure the name matches the regex
// OpenFile opens the requested file. It should be used by the server
func OpenFile(
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

func (f *File) Write(b []byte) (n int, err error) {
	return f.f.Write(b)
}

// Size will return the (current) size of the underlying file. In case the file
// is a client file, the actual size will be reduced by the size of the
// additional information (e.g. checksum). Therefore only the size of the actual
// file content will be returned
func (f *File) Size() uint64 {
	s := uint64(f.stat.Size())
	if f.isClientFile {
		s -= uint64(len(fileSignature) + common.ChecksumSize) // TODO: Make const/var
	}
	return s
}

func (f *File) Checksum() []byte {
	return f.checksum
}

// TODO: see if actually needed
// TODO: comment
// This is only possible on client files.
func (f *File) CheckChecksum([]byte) (bool, error) {
	if !f.isClientFile {
		return false, ErrNoClientFile
	}

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

// StripChecksum removes the magic file signature and the checksum. This is only
// possible on client files. After completion the file descriptor will be closed.
func (f *File) StripChecksum() error {
	if !f.isClientFile {
		return ErrNoClientFile
	}

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
	if len(checksum) != shared.ChecksumSize {
		return ErrInvalidChecksum
	}

	n, err := f.Write(fileSignature)
	if err != nil {
		return err
	} else if n != len(fileSignature) {
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
