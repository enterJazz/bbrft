package main

import (
	"fmt"
	"os"
	"strconv"
	"sync"

	"gitlab.lrz.de/bbrft/brft"
	"gitlab.lrz.de/bbrft/cli"
	"go.uber.org/zap"
)

func main() {
	args := cli.ParseArgs()
	// FIXME: ParseArgs should ret err
	// FIXME: calling binary without correct input shows goroutine and SIGSEGV
	if args == nil {
		os.Exit(0)
	}

	args.L, _ = zap.NewProduction()

	switch args.OperationArgs.GetOperationMode() {
	case cli.Client:
		runClient(args)
	case cli.Server:
		runServer(args)
	}
}

// FIXME: @michi or wlad robert, wlad was tired and lazy and just diplicated progress bar from test, we should make this pretty
const (
	minProgressDelta float32 = 0.05
)

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
						fmt.Println("current progress", filename, prog)
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

func runClient(args *cli.Args) {
	cArgs := args.OperationArgs.(*cli.ClientArgs)
	// connect to server
	optD := brft.NewDefaultOptions(args.L)
	c, err := brft.Dial(args.L, cArgs.ServerAddr, cArgs.DownloadDir, &optD)
	if err != nil {
		args.L.Fatal("failed to connect to server", zap.Error(err))
	}

	switch cArgs.Command {
	case cli.FileRequest:
		progs, err := c.DownloadFiles([]string{cArgs.FileName})
		if err != nil {
			args.L.Fatal("failed to download file", zap.Error(err))
		}
		fmt.Println("download starting")
		logMultipleProgresses(args.L, progs)
	case cli.MetaDataRequest:
		resp, err := c.ListFileMetaData(cArgs.FileName)
		if err != nil {
			args.L.Fatal("failed to fetch file metadata", zap.Error(err))
		}

		fmt.Println("files available on server:")
		fmt.Println("--------------------------")
		for _, item := range resp.Items {
			if item.FileSize != nil {
				fmt.Printf("%s %dB %x \n", item.FileName, *item.FileSize, item.Checksum)
			} else {
				fmt.Printf("%s\n", item.FileName)
			}
		}
	}
}

func runServer(args *cli.Args) {
	sArgs := args.OperationArgs.(*cli.ServerArgs)
	// TODO: should this be parameterized? or set by env var?
	lAddr := "0.0.0.0:" + strconv.Itoa(sArgs.Port)

	opt := &brft.ServerOptions{brft.NewDefaultOptions(args.L)}
	s, _, err := brft.NewServer(args.L, lAddr, sArgs.ServeDir, opt)
	if err != nil {
		args.L.Fatal("failed to create server", zap.Error(err))
	}
	if err := s.ListenAndServe(); err != nil {
		args.L.Fatal("server failed during execution", zap.Error(err))
	}
}
