package brft

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"

	"gitlab.lrz.de/bbrft/brft/common"
	shared "gitlab.lrz.de/bbrft/brft/common"
	"go.uber.org/zap"
)

var fileSignature = []byte{42, 52, 46, 54, 31} // "BRFT1" i.e. BRFT file version 1

var (
	ErrNoClientFile = errors.New("file is no client file")

	fileRegex, _ = regexp.Compile(`^([\w]|-|_)+([\.]([\w])+)?$`)
)

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
	l = l.With(FComponent("file"))
	var (
		f   *os.File
		err error
	)

	// ensure that the download directory exists
	err = os.MkdirAll(basePath, os.ModePerm)
	if err != nil {
		return nil, fmt.Errorf("unable to create base directory: %w", err)
	}

	filePath := filepath.Join(basePath, name)
	// see if the file already exists. If so, load it otherwise create a new file
	if _, err = os.Stat(filePath); errors.Is(err, os.ErrNotExist) {
		if checksum == nil {
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
		l.Debug("opening existing file", zap.String("file_path", filePath))

		// get a file desciptor
		f, err = os.OpenFile(filePath, os.O_RDWR, 0755)
		if err != nil {
			return nil, err
		}

		// read the checksum
		checksum, err = readChecksum(f)
		if err != nil {
			l.Debug("failed to read checksum", zap.Error(err))
			return nil, ErrInvalidChecksum
		}

		// seek end of file
		f.Seek(0, io.SeekEnd)
	}

	fs, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("cannot get file stats: %w", err)
	}

	return &File{
		l:            l,
		f:            f,
		stat:         fs,
		isClientFile: true,
		name:         name,
		basePath:     basePath,
		checksum:     checksum,
	}, nil
}

// OpenFile opens the requested file. It should be used by the server
func OpenFile(
	name string,
	basePath string,
) (*File, error) {
	ok := fileRegex.Match([]byte(name))
	if !ok {
		return nil, fmt.Errorf("filename %s fails BRFT regex check", name)
	}

	if _, err := os.Stat(filepath.Join(basePath, name)); errors.Is(err, os.ErrNotExist) {
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
	if f != nil && f.f != nil {
		return f.f.Close()
	}
	return nil
}

func (f *File) Remove() error {
	if f != nil && f.f != nil {
		if err := f.f.Close(); err != nil {
			return fmt.Errorf("unable to close fd: %s", err)
		}
		return os.Remove(filepath.Join(f.basePath, f.name))
	}
	return nil
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
	// Try to update the stat
	if f.isClientFile {
		fs, err := f.f.Stat()
		if err != nil {
			f.l.Error("unable to update client file stat", zap.Error(err))
		} else {
			f.stat = fs
		}
	}

	s := uint64(f.stat.Size())
	if f.isClientFile {
		s -= uint64(len(fileSignature) + common.ChecksumSize)
	}
	return s
}

// SeekOffset moves the file reader to the byte defined by offset
// depending in isClientFile this will include brft file header
func (f *File) SeekOffset(offset uint64) error {

	internalOffset := offset
	if f.isClientFile {
		internalOffset += uint64(len(fileSignature) + common.ChecksumSize)
	}

	if offset > f.Size() {
		return errors.New("invalid offset: offset is larger then payload")
	}

	_, err := f.f.Seek(int64(offset), io.SeekStart)
	return err
}

func (f *File) Checksum() []byte {
	return f.checksum
}

// CheckChecksum computes the checksum over the actual file content (i.e.
// without signature and checksum). It then compares the computed checksum with
// the previously advertised checksum set in f.checksum. If possible, the
// function will return the computed checksum as second return parameter for
// logging purposes.
// This is only possible on client files.
func (f *File) CheckChecksum() (bool, []byte, error) {
	if !f.isClientFile {
		return false, nil, ErrNoClientFile
	}

	prevOffset, err := f.f.Seek(0, io.SeekCurrent)
	if err != nil {
		return false, nil, err
	}
	// reset the offset at the end
	defer f.f.Seek(prevOffset, io.SeekStart)

	// set the fd to the beginning of the actual content
	offset := int64(len(fileSignature) + common.ChecksumSize)
	_, err = f.f.Seek(offset, io.SeekStart)
	if err != nil {
		return false, nil, err
	}

	if checksum, err := common.ComputeChecksum(f.f); err != nil {
		return false, checksum, err
	} else if !bytes.Equal(checksum, f.checksum) {
		return false, checksum, nil
	} else {
		return true, checksum, nil
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

	// write temporary file next to the current file
	tmpFile, err := os.CreateTemp(f.basePath, "*.brft")
	if err != nil {
		return err
	}
	defer tmpFile.Close()

	_, err = io.Copy(tmpFile, f.f)
	if err != nil {
		return err
	}

	err = os.Rename(tmpFile.Name(), f.f.Name())
	if err != nil {
		return err
	}

	return nil
}

func readChecksum(f *os.File) ([]byte, error) {
	// make sure the file is actually a pending BRFT file
	sig := make([]byte, len(fileSignature))
	if n, err := f.Read(sig); err != nil {
		return nil, err
	} else if n != len(fileSignature) {
		return nil, ErrInsufficientRead
	} else if !bytes.Equal(sig, fileSignature) {
		return nil, ErrNotABRFTFile
	}

	checksum := make([]byte, common.ChecksumSize)
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
