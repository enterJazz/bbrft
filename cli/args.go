package cli

import (
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/urfave/cli/v2"
	"go.uber.org/zap"
)

type OperationMode int

const (
	Server OperationMode = iota
	Client
)

type Args struct {
	TestMode      bool
	L             *zap.Logger
	OperationArgs OperationArgs
}

type OperationArgs interface {
	GetOperationMode() OperationMode
}

func ParseArgs() *Args {
	var (
		testMode   bool
		l          *zap.Logger
		optionArgs OperationArgs
	)

	app := &cli.App{
		Name:  "BRFTP",
		Usage: "serve or fetch remote files",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "test-mode",
				Aliases: []string{"t"},
				Usage:   "enables test mode",
			},
		},
		Action: func(cCtx *cli.Context) error {
			if cCtx.NArg() == 0 {
				return errors.New("no Command specified; use Command `help` to view available commands")
			}
			testMode = cCtx.Bool("test-mode")
			var logErr error
			if testMode {
				l, logErr = zap.NewDevelopment()
				l.Warn("Development Mode Active")

			} else {
				l, logErr = zap.NewProduction()
			}
			if logErr != nil {
				return logErr
			}
			return nil
		},
		Commands: []*cli.Command{
			{
				Name:      "serve",
				Aliases:   []string{"s"},
				Usage:     "serve the files in a directory for download",
				ArgsUsage: "[DIR] [PORT]",
				Action: func(cCtx *cli.Context) error {
					sArgs := ServerArgs{}
					if cCtx.NArg() == 2 {
						sArgs.ServeDir = cCtx.Args().First()
						port, err := strconv.Atoi(cCtx.Args().Get(1))
						if err != nil {
							return errors.New("parsing [PORT] failed: " + err.Error())
						}
						sArgs.Port = port
					} else {
						return errors.New("[DIR] [PORT] required")
					}
					optionArgs = &sArgs
					return nil
				},
			},
			{
				Name:    "get",
				Aliases: []string{"g"},
				Usage:   "get resources from a BRFTP server",
				Subcommands: []*cli.Command{
					{
						Name:      "metadata",
						Aliases:   []string{"m"},
						Usage:     "get metadata from a BRFTP server; specify no file name to list all available files, or specify a file name to get its size and checksum",
						ArgsUsage: "[FILE (optional)] [SERVER ADDRESS]",
						Action: func(cCtx *cli.Context) error {
							cArgs := ClientArgs{
								Command: MetaDataRequest,
							}
							if cCtx.NArg() == 1 {
								cArgs.ServerAddr = cCtx.Args().First()
							} else if cCtx.NArg() == 2 {
								cArgs.FileName = cCtx.Args().First()
								cArgs.ServerAddr = cCtx.Args().Get(1)
							} else {
								return errors.New("[SERVER ADDRESS] required")
							}
							optionArgs = &cArgs
							return nil
						},
					},
					{
						Name:      "file",
						Aliases:   []string{"f"},
						Usage:     "get a FILE from a BRFTP server and store it in DOWNLOAD DIR; set CHECKSUM to assert file content's SHA-256 hash matches CHECKSUM",
						ArgsUsage: "[FILE] [DOWNLOAD DIR] [CHECKSUM (optional)] [SERVER ADDRESS]",
						Action: func(cCtx *cli.Context) error {
							cArgs := ClientArgs{
								Command: FileRequest,
							}
							if cCtx.NArg() == 3 {
								cArgs.FileName = cCtx.Args().First()
								cArgs.DownloadDir = cCtx.Args().Get(1)
								cArgs.ServerAddr = cCtx.Args().Get(2)
							} else if cCtx.NArg() == 4 {
								cArgs.FileName = cCtx.Args().First()
								cArgs.DownloadDir = cCtx.Args().Get(1)
								cArgs.Checksum = cCtx.Args().Get(2)
								cArgs.ServerAddr = cCtx.Args().Get(3)
							} else {
								return errors.New("[FILE] [DOWNLOAD DIR] [CHECKSUM (optional)] [SERVER ADDRESS] required")
							}
							optionArgs = &cArgs
							return nil
						},
					},
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Printf("failed to parse args: %v\n", err)
		os.Exit(1)
	}

	args := Args{
		TestMode:      testMode,
		L:             l,
		OperationArgs: optionArgs,
	}

	return &args
}