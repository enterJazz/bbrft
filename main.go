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

	switch args.OperationArgs.GetOperationMode() {
	case cli.Client:
		runClient(args)
	case cli.Server:
		runServer(args)
	}
}

func getLoggers(testMode bool, isClient bool) (cliLogger *zap.Logger, btpLogger *zap.Logger, brftLogger *zap.Logger, err error) {
	debug := testMode || os.Getenv("DEBUG") == "true"
	btpDebug := debug || os.Getenv("DEBUG_BTP") == "true"
	brftDebug := debug || os.Getenv("DEBUG_BRFT") == "true"

	cliLogger, err = log.NewLogger(
		log.WithClient(true),
		log.WithColor(true),
		log.WithProd(!debug),
	)
	if err != nil {
		return
	}

	btpLogger, err = log.NewLogger(
		log.WithClient(true),
		log.WithColor(true),
		log.WithProd(!btpDebug),
	)
	if err != nil {
		return
	}

	brftLogger, err = log.NewLogger(
		log.WithClient(true),
		log.WithColor(true),
		log.WithProd(!brftDebug),
	)
	if err != nil {
		return
	}

	return
}

func runClient(args *cli.Args) {
	cliLog, btpLog, brftLog, err := getLoggers(args.TestMode, true)
	if err != nil {
		panic(err)
	}

	cArgs := args.OperationArgs.(*cli.ClientArgs)
	// connect to server
	optD := brft.NewDefaultOptions(brftLog)
	optD.BtpLogger = btpLog

	c, err := brft.Dial(brftLog, cArgs.ServerAddr, cArgs.DownloadDir, &optD)
	if err != nil {
		cliLog.Fatal("failed to connect to server", zap.Error(err))
	}

	switch cArgs.Command {
	case cli.FileRequest:
		fmt.Println("starting download")
		prog, err := c.DownloadFile(cArgs.FileName)
		if err != nil {
			cliLog.Fatal("failed to download file", zap.Error(err))
		} else {
			log.LogProgress(cliLog, cArgs.FileName, prog)
		}
		log.LogProgress(cliLog, cArgs.FileName, prog)
	case cli.MetaDataRequest:
		resp, err := c.ListFileMetaData(cArgs.FileName)
		if err != nil {
			cliLog.Fatal("failed to fetch file metadata", zap.Error(err))
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
	cliLog, btpLog, brftLog, err := getLoggers(args.TestMode, false)
	if err != nil {
		panic(err)
	}

	sArgs := args.OperationArgs.(*cli.ServerArgs)
	// TODO: should this be parameterized? or set by env var?
	lAddr := "0.0.0.0:" + strconv.Itoa(sArgs.Port)

	opt := &brft.ServerOptions{brft.NewDefaultOptions(brftLog)}
	opt.BtpLogger = btpLog

	s, _, err := brft.NewServer(brftLog, lAddr, sArgs.ServeDir, opt)
	if err != nil {
		cliLog.Fatal("failed to create server", zap.Error(err))
	}
	if err := s.ListenAndServe(); err != nil {
		cliLog.Fatal("server failed during execution", zap.Error(err))
	}
}
