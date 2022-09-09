package brft

import (
	"errors"
	"fmt"
	"os"
	"path"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"gitlab.lrz.de/bbrft/brft/messages"
	"gitlab.lrz.de/bbrft/log"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

const (
	serverDir  string = "../test/server"
	clientDir  string = "../test/downloads"
	serverAddr string = "127.0.0.1:1337"
)

func setupTest(t *testing.T,
	brftOpts []log.Option,
	btpOpts []log.Option,
	compression bool,
) (*zap.Logger, *Conn, func()) {
	l, _ := log.NewLogger(brftOpts...)
	lBtp, err := log.NewLogger(btpOpts...)

	opt := &ServerOptions{NewDefaultOptions(lBtp, compression)}
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
	optD := NewDefaultOptions(lBtp, compression)
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

func removeTestFilesServer(t *testing.T, testFiles ...string) {
	// See if the results are already present in the download directory. If so remove them again
	for _, file := range testFiles {
		p := path.Join(serverDir, file)
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
		true,
	)
	defer close()
	removeTestFiles(t, testFile)
	prog, err := c.DownloadFile(testFile, nil)
	if err != nil {
		t.Error(err)
	}

	LogProgress(l, testFile, prog)
}

func TestTransferNoCompression(t *testing.T) {
	testFile := "test-1.jpg"

	l, c, close := setupTest(t,
		[]log.Option{log.WithProd(true)}, // TODO: Re-vert
		[]log.Option{log.WithProd(true)},
		false,
	)
	defer close()
	removeTestFiles(t, testFile)
	prog, err := c.DownloadFile(testFile, nil)
	if err != nil {
		t.Error(err)
	}

	LogProgress(l, testFile, prog)
}

// TODO: Create separate tests for different chunk sizes
func TestBigTransfer(t *testing.T) {
	testFile := "video-1.mkv"

	l, c, close := setupTest(t,
		[]log.Option{log.WithProd(true)},
		[]log.Option{log.WithProd(true)},
		true,
	)
	defer close()
	removeTestFiles(t, testFile)
	prog, err := c.DownloadFile(testFile, nil)
	if err != nil {
		t.Error(err)
	}

	LogProgress(l, testFile, prog)
}

func TestMultiTransfer(t *testing.T) {
	testFiles := map[string][]byte{
		"test-1.jpg": nil,
		"test-2.jpg": nil,
		"test-3.jpg": nil,
		"test-4.jpg": nil,
	}
	keys := make([]string, 0, len(testFiles))
	for k := range testFiles {
		keys = append(keys, k)
	}

	l, c, close := setupTest(t,
		[]log.Option{log.WithProd(true)},
		[]log.Option{log.WithProd(true)},
		true,
	)
	defer close()
	removeTestFiles(t, keys...)
	progs, err := c.DownloadFiles(testFiles)
	if err != nil {
		t.Error(err)
	}

	LogMultipleProgresses(l, progs)
}

func TestConcurrentDownloadAndMetaDataRequest(t *testing.T) {
	testFiles := map[string][]byte{
		"test-1.jpg": nil,
		"test-2.jpg": nil,
		"test-3.jpg": nil,
		"test-4.jpg": nil,
	}
	keys := make([]string, 0, len(testFiles))
	for k := range testFiles {
		keys = append(keys, k)
	}

	l, c, close := setupTest(t,
		[]log.Option{log.WithProd(false)},
		[]log.Option{log.WithProd(true)},
		true,
	)
	defer close()
	removeTestFiles(t, keys...)

	// Download the files
	progs, err := c.DownloadFiles(testFiles)
	if err != nil {
		t.Error(err)
	}

	// get the metadata 5 times to "ensure" concurrency
	numRepeat := 5
	metaFiles := make([]string, 0, numRepeat*len(testFiles))
	for i := 0; i < numRepeat; i++ {
		metaFiles = append(metaFiles, keys...)
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

	LogMultipleProgresses(l, progs)
}

func TestNonExistingFile(t *testing.T) {
	testFile := "not.existing"

	l, c, close := setupTest(t,
		[]log.Option{log.WithProd(false)},
		[]log.Option{log.WithProd(true)},
		true,
	)
	defer close()
	removeTestFiles(t, testFile)
	prog, err := c.DownloadFile(testFile, nil)
	if err != nil {
		t.Error(err)
	}

	LogProgress(l, testFile, prog)
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
		true,
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

func TestMaxItemsMetadata(t *testing.T) {
	testFileNames := make([]string, 0)
	for i := 0; i < messages.MaxMetaItemsNum*2; i++ {
		testFileName := fmt.Sprintf("test-%v.txt", i)
		testFileNames = append(testFileNames, testFileName)
		testFile := path.Join(serverDir, testFileName)
		if _, err := os.Create(testFile); err != nil {
			t.Errorf("failed to create file: %v", err)
		}
	}

	_, c, close := setupTest(t,
		[]log.Option{log.WithProd(false)},
		[]log.Option{log.WithProd(true)},
		true,
	)
	defer func() {
		removeTestFilesServer(t, testFileNames...)
		close()
	}()
	resps, err := c.ListFileMetaData("")
	if err != nil {
		t.Errorf("ListFileMetaData failed = %v", err)
	}

	time.Sleep(10 * time.Second)

	itemFileNames := make([]string, 0)
	for _, resp := range resps {
		for _, item := range resp.Items {
			itemFileNames = append(itemFileNames, item.FileName)
		}
	}

	// also included from other tests
	cmpTestFileNames := append(testFileNames, "test-1.jpg", "test-2.jpg", "test-3.jpg", "test-4.jpg")

	sort.Strings(cmpTestFileNames)
	sort.Strings(itemFileNames)
	if !reflect.DeepEqual(cmpTestFileNames, itemFileNames) {
		t.Errorf("file names not equal = %v, want = %v", spew.Sdump("\n", itemFileNames), spew.Sdump("\n", cmpTestFileNames))
	}
}
