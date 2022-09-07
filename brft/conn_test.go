package brft

import (
	"errors"
	"os"
	"path"
	"sync"
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
)

func setupTest(t *testing.T,
	brftOpts []log.Option,
	btpOpts []log.Option,
) (*zap.Logger, *Conn) {
	ld, _ := log.NewLogger(brftOpts...)
	lp, err := log.NewLogger(btpOpts...)

	opt := &ServerOptions{NewDefaultOptions(lp)}
	s, laddr, err := NewServer(ld, "127.0.0.1:1337", serverDir, opt)
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
	optD := NewDefaultOptions(lp)
	optD.chunkSizeFactor = 63 // 4MB
	c, err := Dial(ld, laddr.String(), clientDir, &optD)
	if err != nil {
		t.Error(err)
	}
	return ld, c
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

func logProgress(l *zap.Logger, filename string, ch <-chan float32) {
	logMultipleProgresses(l, map[string]<-chan float32{filename: ch})
}

func logMultipleProgresses(l *zap.Logger, chs map[string]<-chan float32) {
	wg := sync.WaitGroup{}
	for filename, ch := range chs {
		filename, ch := filename, ch // https://golang.org/doc/faq#closures_and_goroutines
		wg.Add(1)
		go func() {
			defer wg.Done()
			var prevProgress float32 = 0.0
			for {
				if prog, ok := <-ch; ok {
					//if prog > prevProgress {
					if prog > prevProgress+minProgressDelta {
						l.Info("current progress", zap.String("file_name", filename), zap.Float32("progress", prog))
						prevProgress = prog
					}
				} else {
					return
				}
			}
		}()
	}

	wg.Wait()
}

func consumeProgress(chs ...<-chan float32) {
	wg := sync.WaitGroup{}
	for _, ch := range chs {
		wg.Add(1)
		ch := ch // https://golang.org/doc/faq#closures_and_goroutines
		go func() {
			defer wg.Done()
			for {
				if _, ok := <-ch; !ok {
					return
				}
			}
		}()
	}

	wg.Wait()
}

func TestTransfer(t *testing.T) {
	testFile := "test-1.jpg"

	l, c := setupTest(t,
		[]log.Option{log.WithProd(true)}, // TODO: Re-vert
		[]log.Option{log.WithProd(true)},
	)
	removeTestFiles(t, testFile)
	prog, err := c.DownloadFile(testFile)
	if err != nil {
		t.Error(err)
	}

	logProgress(l, testFile, prog)
}

// TODO: Create separate tests for different chunk sizes
func TestBigTransfer(t *testing.T) {
	testFile := "video-1.mkv"

	l, c := setupTest(t,
		[]log.Option{log.WithProd(true)},
		[]log.Option{log.WithProd(true)},
	)
	removeTestFiles(t, testFile)
	prog, err := c.DownloadFile(testFile)
	if err != nil {
		t.Error(err)
	}

	logProgress(l, testFile, prog)
}

func TestMultiTransfer(t *testing.T) {
	testFiles := []string{"test-1.jpg", "test-2.jpg", "test-3.jpg", "test-4.jpg"}

	l, c := setupTest(t,
		[]log.Option{log.WithProd(false)},
		[]log.Option{log.WithProd(true)},
	)
	removeTestFiles(t, testFiles...)
	progs, err := c.DownloadFiles(testFiles)
	if err != nil {
		t.Error(err)
	}

	logMultipleProgresses(l, progs)
}

func TestConcurrentDownloadAndMetaDataRequest(t *testing.T) {
	testFiles := []string{"test-1.jpg", "test-2.jpg", "test-3.jpg", "test-4.jpg"}

	l, c := setupTest(t,
		[]log.Option{log.WithProd(false)},
		[]log.Option{log.WithProd(true)},
	)
	removeTestFiles(t, testFiles...)

	// Download the files
	progs, err := c.DownloadFiles(testFiles)
	if err != nil {
		t.Error(err)
	}

	// get the metadata 5 times to "ensure" concurrency
	numRepeate := 5
	metaFiles := make([]string, 0, numRepeate*len(testFiles))
	for i := 0; i < numRepeate; i++ {
		metaFiles = append(metaFiles, testFiles...)
	}
	g := new(errgroup.Group)
	for _, f := range metaFiles {
		f := f // https://golang.org/doc/faq#closures_and_goroutines

		g.Go(func() error {
			return c.ListFileMetaData(f)
		})
	}

	err = g.Wait()
	if err != nil {
		t.Fatal(err)
	}

	logMultipleProgresses(l, progs)
}

func TestNonExistingFile(t *testing.T) {
	testFile := "not.existing"

	l, c := setupTest(t,
		[]log.Option{log.WithProd(false)},
		[]log.Option{log.WithProd(true)},
	)
	removeTestFiles(t, testFile)
	prog, err := c.DownloadFile(testFile)
	if err != nil {
		t.Error(err)
	}

	logProgress(l, testFile, prog)
}

func TestMetaData(t *testing.T) {
	testFile := "test-1.jpg"

	_, c := setupTest(t,
		[]log.Option{log.WithProd(false)},
		[]log.Option{log.WithProd(true)},
	)
	if err := c.ListFileMetaData(""); err != nil {
		t.Error(err)
	}

	if err := c.ListFileMetaData(testFile); err != nil {
		t.Error(err)
	}

	time.Sleep(10 * time.Second)
}
