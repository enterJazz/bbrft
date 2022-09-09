package brft

import (
	"errors"
	"os"
	"path"
	"testing"
	"time"

	"gitlab.lrz.de/bbrft/log"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

const (
	serverDir        string  = "../test/server"
	clientDir        string  = "../test/downloads"
	minProgressDelta float32 = 0.05
	serverAddr       string  = "127.0.0.1:1337"
)

func setupTest(t *testing.T,
	brftOpts []log.Option,
	btpOpts []log.Option,
) (*zap.Logger, *Conn, func()) {
	l, _ := log.NewLogger(brftOpts...)
	lBtp, err := log.NewLogger(btpOpts...)

	opt := &ServerOptions{NewDefaultOptions(lBtp)}
	s, laddr, err := NewServer(l, serverAddr, serverDir, opt)
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		err := s.ListenAndServe()
		if err != nil {
			t.Fatal(err)
		}

	}()

	t.Log(laddr.String())
	optD := NewDefaultOptions(lBtp)
	optD.chunkSizeFactor = 63 // 4MB
	c, err := Dial(l, laddr.String(), clientDir, &optD)
	if err != nil {
		errS := s.Close()
		if errS != nil {
			l.Error("unable to close server", zap.Error(errS))
		}
		t.Error(err)
	}

	return l, c, func() {
		err := s.Close()
		if err != nil {
			l.Error("unable to close server", zap.Error(err))
		}
		err = c.Close()
		if err != nil {
			l.Error("unable to close client", zap.Error(err))
		}
	}
}

func checkMultipleClientFilesExist(files []string) error {
	for _, file := range files {
		if err := checkClientFileExists(file); err != nil {
			return err
		}
	}
	return nil
}

func checkClientFileExists(file string) error {
	p := path.Join(clientDir, file)
	if _, err := os.Stat(p); errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func removeTestFiles(t *testing.T, testFiles ...string) {
	// See if the results are already present in the download directory. If so remove them again
	for _, file := range testFiles {
		p := path.Join(clientDir, file)
		if stat, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
			if stat.IsDir() {
				t.Fatalf("expected file not directory: %s", p)
			} else {
				os.Remove(p)
			}
		}
	}
}

func TestTransfer(t *testing.T) {
	testFile := "test-1.jpg"

	l, c, close := setupTest(t,
		[]log.Option{log.WithProd(true)}, // TODO: Re-vert
		[]log.Option{log.WithProd(true)},
	)
	defer close()
	removeTestFiles(t, testFile)
	prog, err := c.DownloadFile(testFile)
	if err != nil {
		t.Error(err)
	}

	log.LogProgress(l, testFile, prog)
}

// TODO: Create separate tests for different chunk sizes
func TestBigTransfer(t *testing.T) {
	testFile := "video-1.mkv"

	l, c, close := setupTest(t,
		[]log.Option{log.WithProd(true)},
		[]log.Option{log.WithProd(true)},
	)
	defer close()
	removeTestFiles(t, testFile)
	prog, err := c.DownloadFile(testFile)
	if err != nil {
		t.Error(err)
	}

	log.LogProgress(l, testFile, prog)
}

func TestMultiTransfer(t *testing.T) {
	testFiles := []string{"test-1.jpg", "test-2.jpg", "test-3.jpg", "test-4.jpg"}

	l, c, close := setupTest(t,
		[]log.Option{log.WithProd(true)},
		[]log.Option{log.WithProd(true)},
	)
	defer close()
	removeTestFiles(t, testFiles...)
	progs, err := c.DownloadFiles(testFiles)
	if err != nil {
		t.Error(err)
	}

	log.LogMultipleProgresses(l, progs)
}

func TestConcurrentDownloadAndMetaDataRequest(t *testing.T) {
	testFiles := []string{"test-1.jpg", "test-2.jpg", "test-3.jpg", "test-4.jpg"}

	l, c, close := setupTest(t,
		[]log.Option{log.WithProd(false)},
		[]log.Option{log.WithProd(true)},
	)
	defer close()
	removeTestFiles(t, testFiles...)

	// Download the files
	progs, err := c.DownloadFiles(testFiles)
	if err != nil {
		t.Error(err)
	}

	// get the metadata 5 times to "ensure" concurrency
	numRepeat := 5
	metaFiles := make([]string, 0, numRepeat*len(testFiles))
	for i := 0; i < numRepeat; i++ {
		metaFiles = append(metaFiles, testFiles...)
	}
	g := new(errgroup.Group)
	for _, f := range metaFiles {
		f := f // https://golang.org/doc/faq#closures_and_goroutines

		g.Go(func() error {
			_, err = c.ListFileMetaData(f)
			return err
		})
	}

	err = g.Wait()
	if err != nil {
		t.Fatal(err)
	}

	log.LogMultipleProgresses(l, progs)
}

func TestNonExistingFile(t *testing.T) {
	testFile := "not.existing"

	l, c, close := setupTest(t,
		[]log.Option{log.WithProd(false)},
		[]log.Option{log.WithProd(true)},
	)
	defer close()
	removeTestFiles(t, testFile)
	prog, err := c.DownloadFile(testFile)
	if err != nil {
		t.Error(err)
	}

	log.LogProgress(l, testFile, prog)
}

// func TestDownloadResumption(t *testing.T) {
// 	filename := "tearsofsteel_4k.mov"

// 	if _, err := os.Stat(path.Join(serverDir, filename)); errors.Is(err, os.ErrNotExist) {
// 		println("please download test file first")
// 		println("wget -o ./test/server/tearsofsteel_4k.mov http://ftp.nluug.nl/pub/graphics/blender/demo/movies/ToS/tearsofsteel_4k.mov")
// 		panic("test file does not exist please run")
// 	}

// 	l, c, close := setupTest(t,
// 		[]log.Option{log.WithProd(false)},
// 		[]log.Option{log.WithProd(true)},
// 	)

// 	go func() {
// 		info, err := c.DownloadFile(filename)
// 		if err != nil {
// 			panic(err)
// 		}
// 		log.LogProgress(l, filename, info)
// 	}()
// 	c.Close()

// 	// try resuming download after close
// 	t.Log("attempting redownload")
// 	optD := NewDefaultOptions(l)
// 	optD.chunkSizeFactor = 63 // 4MB
// 	c, err := Dial(l, serverAddr, clientDir, &optD)
// 	if err != nil {
// 		close()
// 		return
// 	}

// 	info, err := c.DownloadFile(filename)
// 	if err != nil {
// 		panic(err)
// 	}
// 	log.LogProgress(l, filename, info)

// 	close()
// }

func TestMetaData(t *testing.T) {
	testFile := "test-1.jpg"

	_, c, close := setupTest(t,
		[]log.Option{log.WithProd(false)},
		[]log.Option{log.WithProd(true)},
	)
	defer close()
	if _, err := c.ListFileMetaData(""); err != nil {
		t.Error(err)
	}

	if _, err := c.ListFileMetaData(testFile); err != nil {
		t.Error(err)
	}

	time.Sleep(10 * time.Second)
}
