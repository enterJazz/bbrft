package main

import (
	"fmt"
	"os"
	"strconv"

	"gitlab.lrz.de/bbrft/brft"
	"gitlab.lrz.de/bbrft/cli"
	"gitlab.lrz.de/bbrft/log"
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
		fmt.Println("starting download")
		prog, err := c.DownloadFile(cArgs.FileName)
		if err != nil {
			args.L.Fatal("failed to download file", zap.Error(err))
		} else {
			brft.LogProgress(args.L, cArgs.FileName, prog)
		}
		log.LogProgress(args.L, cArgs.FileName, prog)
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
