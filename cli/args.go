package cli

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/urfave/cli/v2"
	"gitlab.lrz.de/bbrft/brft/common"
	"gitlab.lrz.de/bbrft/brft/compression"
)

type OperationMode int

const (
	Server OperationMode = iota
	Client
)

// len of checksum for base64 encoding
const inputChecksumLen int = 64

type Args struct {
	TestMode      bool
	OperationArgs OperationArgs
}

type OperationArgs interface {
	GetOperationMode() OperationMode
}

func ParseArgs() (*Args, error) {
	var (
		testMode   bool
		optionArgs OperationArgs
	)

	app := &cli.App{
		Name:  "BRFTP",
		Usage: "serve or fetch remote files",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "debug",
				Aliases: []string{"d"},
				Usage:   "enables debug/test mode",
			},
		},
		CommandNotFound: func(cCtx *cli.Context, command string) {
			fmt.Fprintf(cCtx.App.Writer, "command not found: %q\n", command)
		},
		OnUsageError: func(cCtx *cli.Context, err error, isSubcommand bool) error {
			if isSubcommand {
				return err
			}

			fmt.Fprintf(cCtx.App.Writer, "ERROR: %#v\n", err)
			return nil
		},
		Action: func(cCtx *cli.Context) error {
			if cCtx.NArg() == 0 {
				return errors.New("no Command specified; use Command `help` to view available commands")
			}
			testMode = cCtx.Bool("debug")
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
						if sArgs.ServeDir == "" {
							return errors.New("[DIR] may not be empty")
						}
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
								Command:        MetaDataRequest,
								DownloadFiles:  make(map[string][]byte),
								UseCompression: compression.DefaultCompressionEnabled,
							}
							if cCtx.NArg() == 1 {
								cArgs.DownloadFiles[""] = make([]byte, common.ChecksumSize)
								cArgs.ServerAddr = cCtx.Args().First()
							} else if cCtx.NArg() == 2 {
								cArgs.DownloadFiles[cCtx.Args().First()] = make([]byte, common.ChecksumSize)
								cArgs.ServerAddr = cCtx.Args().Get(1)
							} else {
								return errors.New("[SERVER ADDRESS] required")
							}
							// sanity check: no empty server addr
							if cArgs.ServerAddr == "" {
								return errors.New("[SERVER ADDRESS] required: must be non-empty")
							}
							optionArgs = &cArgs
							return nil
						},
					},
					{
						Name:      "file",
						Aliases:   []string{"f"},
						Usage:     "get FILE(s) from a BRFTP server at SERVER ADDRESS and store them in DOWNLOAD DIR; set CHECKSUM to assert file content's SHA-256 hash matches CHECKSUM",
						ArgsUsage: "[FILE1:CHECKSUM1(optional) | (optional) FILE2:CHECKSUM2(optional) | ...] [DOWNLOAD DIR] [SERVER ADDRESS]",
						Flags: []cli.Flag{
							&cli.BoolFlag{
								Name:    "disable-compression",
								Aliases: []string{"c"},
								Usage:   "disables brft layer compression for getting files",
							},
						},
						Action: func(cCtx *cli.Context) error {
							cArgs := ClientArgs{
								Command:        FileRequest,
								DownloadFiles:  make(map[string][]byte),
								UseCompression: !cCtx.Bool("disable-compression"),
							}
							if cCtx.NArg() < 3 {
								return errors.New("[FILE:CHECKSUM(optional) | ...] [DOWNLOAD DIR] [SERVER ADDRESS] required")
							}

							// parse [FILE:CHECKSUM | ...]
							for i := 0; i < cCtx.Args().Len()-2; i++ {
								var (
									downloadFile string
									checksum     []byte
								)
								// check if ':' contained; if yes, tail is checksum
								fileChecksumArg := cCtx.Args().Get(i)
								// check not empty
								if fileChecksumArg == "" {
									return errors.New("error parsing [FILE:CHECKSUM]: may not be set as empty")
								}
								// check that parsed arg does not begin with ':' (empty file name)
								if fileChecksumArg[0] == ':' {
									return errors.New("error parsing [FILE:CHECKSUM]: FILE may not be empty")
								}
								parsedArg := strings.FieldsFunc(fileChecksumArg, func(r rune) bool {
									return r == ':'
								})
								// in specs, we don't allow : to be part of file name; so if the result array has more than two els,
								// we error (as : was in file name)
								if len(parsedArg) > 2 || len(parsedArg) == 0 {
									// == 0 : sanity check if only ':' as input
									return errors.New("error parsing FILE: ':' is NOT allowed as part of the file name")
								}
								downloadFile = parsedArg[0]
								checksum = make([]byte, common.ChecksumSize)
								if len(parsedArg) == 2 {
									// parse to []byte
									checksum, err := hex.DecodeString(parsedArg[1])
									if err != nil {
										return err
									}

									// check if checksum is valid length
									if len(checksum) != common.ChecksumSize {
										return fmt.Errorf("error parsing CHECKSUM: given checksum is invalid (not of length %v) or not in base64", inputChecksumLen)
									}
									cArgs.DownloadFiles[downloadFile] = checksum
								} else {
									cArgs.DownloadFiles[downloadFile] = checksum
								}
							}

							cArgs.DownloadDir = cCtx.Args().Get(cCtx.Args().Len() - 2)
							cArgs.ServerAddr = cCtx.Args().Get(cCtx.Args().Len() - 1)
							optionArgs = &cArgs
							return nil
						},
					},
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		return nil, err
	}

	if optionArgs == nil {
		return nil, errors.New("error parsing args: given command unknown")
	}

	args := Args{
		TestMode:      testMode,
		OperationArgs: optionArgs,
	}

	return &args, nil
}
