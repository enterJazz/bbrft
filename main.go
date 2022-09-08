package main

import (
	"os"
	"strconv"

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
		if prog, err := c.DownloadFile(cArgs.FileName); err != nil {
			args.L.Fatal("failed to download file", zap.Error(err))
		} else {
			brft.LogProgress(args.L, cArgs.FileName, prog)
		}
	case cli.MetaDataRequest:
		if err := c.ListFileMetaData(cArgs.FileName); err != nil {
			args.L.Fatal("failed to fetch file metadata", zap.Error(err))
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
