package brft

import (
	"errors"
	"os"
	"path"
	"testing"
	"time"

	"gitlab.lrz.de/bbrft/log"
)

const (
	serverDir string = "../test/server"
	clientDir string = "../test/downloads"
)

func setupTest(t *testing.T) *Conn {
	ld, _ := log.NewLogger()
	lp, err := log.NewLogger(log.WithProd(false))
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
	c, err := Dial(ld, laddr.String(), clientDir, &optD)
	if err != nil {
		t.Error(err)
	}
	return c
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

	c := setupTest(t)
	removeTestFiles(t, testFile)
	err := c.DownloadFile(testFile)
	if err != nil {
		t.Error(err)
	}

	for {
		time.Sleep(time.Nanosecond * 100)
	}
}

func TestMultiTransfer(t *testing.T) {
	testFiles := []string{"test-1.jpg", "test-2.jpg", "test-3.jpg", "test-4.jpg"}

	c := setupTest(t)
	removeTestFiles(t, testFiles...)
	err := c.DownloadFiles(testFiles)
	if err != nil {
		t.Error(err)
	}

	for {
		time.Sleep(time.Nanosecond * 100)
	}
}

func TestNonExistingFile(t *testing.T) {
	testFiles := []string{"not.existing"}

	c := setupTest(t)
	removeTestFiles(t, testFiles...)
	err := c.DownloadFiles(testFiles)
	if err != nil {
		t.Error(err)
	}

	for {
		time.Sleep(time.Nanosecond * 100)
	}
}

func TestMetaData(t *testing.T) {
	testFile := "test-1.jpg"

	c := setupTest(t)
	if err := c.ListFileMetaData(""); err != nil {
		t.Error(err)
	}

	if err := c.ListFileMetaData(testFile); err != nil {
		t.Error(err)
	}

	for {
		time.Sleep(time.Nanosecond * 100)
	}
}
