package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"

	"gitlab.lrz.de/bbrft/brft"
	"gitlab.lrz.de/bbrft/brft/compression"
	"gitlab.lrz.de/bbrft/cli"
	"gitlab.lrz.de/bbrft/log"
	"go.uber.org/zap"
)

func main() {
	args, err := cli.ParseArgs()
	if err != nil {
		if err == cli.HelpError {
			os.Exit(2)
		}
		fmt.Printf("failed to parse args: %v\n", err)
		os.Exit(1)
	}
	// FIXME: calling binary without correct input shows goroutine and SIGSEGV

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
	optD := brft.NewDefaultOptions(brftLog, cArgs.UseCompression)
	optD.BtpLogger = btpLog

	c, err := brft.Dial(brftLog, cArgs.ServerAddr, cArgs.DownloadDir, &optD)
	if err != nil {
		cliLog.Fatal("failed to connect to server", zap.Error(err))
	}

	getSingleKeyValPair := func(m map[string][]byte) (f string, cs []byte, err error) {
		if len(m) != 1 {
			return "", nil, errors.New("given map does not exactly contain a single k:v pair")
		}
		for k, v := range m {
			f = k
			cs = v
		}
		return
	}

	switch cArgs.Command {
	case cli.FileRequest:
		fmt.Println("starting download")
		// case single download
		if len(cArgs.DownloadFiles) == 1 {
			f, cs, err := getSingleKeyValPair(cArgs.DownloadFiles)
			if err != nil {
				cliLog.Fatal("error parsing files", zap.Error(err))
			}
			prog, err := c.DownloadFile(f, cs)
			if err != nil {
				cliLog.Fatal("failed to download file", zap.Error(err))
			} else {
				brft.LogProgress(cliLog, f, prog)
			}
		} else {
			progs, err := c.DownloadFiles(cArgs.DownloadFiles)
			if err != nil {
				cliLog.Fatal("failed to download files", zap.Error(err))
			}

			brft.LogMultipleProgresses(cliLog, progs)
		}
	case cli.MetaDataRequest:
		f, _, err := getSingleKeyValPair(cArgs.DownloadFiles)
		if err != nil {
			cliLog.Fatal("error parsing files", zap.Error(err))
		}

		resps, err := c.ListFileMetaData(f)
		if err != nil {
			cliLog.Fatal("failed to fetch file metadata", zap.Error(err))
		}
		c.Close()

		// case MetaDataReq lists server dir
		if f != "" {
			fmt.Println("files available on server:")
			fmt.Println("--------------------------")
		} else {
			fmt.Printf("file details of %s\n", f)
			fmt.Println("--------------------------")
		}
		for _, resp := range resps {
			for _, item := range resp.Items {
				if item.FileSize != nil {
					fmt.Printf("%s %dB %x \n", item.FileName, *item.FileSize, item.Checksum)
				} else {
					fmt.Printf("%s\n", item.FileName)
				}
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

	opt := &brft.ServerOptions{brft.NewDefaultOptions(brftLog, compression.DefaultCompressionEnabled)}
	opt.BtpLogger = btpLog

	s, _, err := brft.NewServer(brftLog, lAddr, sArgs.ServeDir, opt)
	if err != nil {
		cliLog.Fatal("failed to create server", zap.Error(err))
	}
	if err := s.ListenAndServe(); err != nil {
		cliLog.Fatal("server failed during execution", zap.Error(err))
	}
}
