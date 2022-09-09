package cli

import (
	"errors"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"gitlab.lrz.de/bbrft/brft/common"
)

const serveCmd string = "serve"
const getCmd string = "get"
const getMetaDataSubCmd = "metadata"
const getFileSubCmd = "file"

func TestParseServerArgs(t *testing.T) {
	tests := []struct {
		name      string
		dir       string
		port      string
		expectErr bool
	}{
		{
			"valid-relative",
			"./my-dir",
			"1234",
			false,
		},
		{
			"valid-absoulte",
			"/path/to/my/dir",
			"1234",
			false,
		},
		{
			"invalid-empty-dir",
			"",
			"1234",
			true,
		},
		{
			"invalid-empty-port",
			"./my-dir",
			"",
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			binary := os.Args[0]
			os.Args = []string{binary, serveCmd, tt.dir, tt.port}
			got, err := ParseArgs()

			if tt.expectErr && err == nil {
				t.Errorf("ParseArgs() expected error, yet err is nil; args: %v", spew.Sdump("\n", got))
				return
			} else if !tt.expectErr && err != nil {
				t.Errorf("ParseArgs() failed; err = %v", spew.Sdump("\n", got))
			}

			if err != nil {
				return
			}

			serveArgs := got.OperationArgs.(*ServerArgs)
			if serveArgs.ServeDir != tt.dir {
				t.Errorf("unexpected arg parse = %v, want = %v", serveArgs.ServeDir, tt.dir)
			}

			if strconv.Itoa(serveArgs.Port) != tt.port {
				t.Errorf("unexpected arg parse = %s, want = %s", strconv.Itoa(serveArgs.Port), tt.port)
			}
		})
	}
}

func TestParseArgsGetMetaData(t *testing.T) {
	tests := []struct {
		name       string
		file       string
		serverAddr string
		expectErr  bool
	}{
		{
			"valid-specific",
			"./my-file",
			"1.2.3.4:1234",
			false,
		},
		{
			"valid-no-file",
			"",
			"1.2.3.4:1234",
			false,
		},
		{
			"invalid-empty-server-addr",
			"",
			"",
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			binary := os.Args[0]
			os.Args = []string{binary, getCmd, getMetaDataSubCmd}
			if tt.file != "" {
				os.Args = append(os.Args, tt.file)
			}
			os.Args = append(os.Args, tt.serverAddr)

			got, err := ParseArgs()

			if tt.expectErr && err == nil {
				t.Errorf("ParseArgs() expected error, yet err is nil; args: %v", spew.Sdump("\n", got))
				return
			} else if !tt.expectErr && err != nil {
				t.Errorf("ParseArgs() failed; err = %v", spew.Sdump("\n", got))
			}

			if err != nil {
				return
			}

			cArgs := got.OperationArgs.(*ClientArgs)
			if cArgs.Command != ClientCommand(MetaDataRequest) {
				t.Errorf("unexpected arg parse = %v, want = %v", cArgs.Command, MetaDataRequest)
			}

			if tt.file != "" {
				file, _, err := getSingleKeyValPair(cArgs.DownloadFiles)
				if err != nil {
					t.Errorf("err = %v", err)
				}

				if file != tt.file {
					t.Errorf("unexpected arg parse = %s, want %s", file, tt.file)
				}
			}

			if cArgs.ServerAddr != tt.serverAddr {
				t.Errorf("unexpected arg parse = %s, want = %s", cArgs.ServerAddr, tt.serverAddr)
			}
		})
	}
}

func TestParseArgsGetFile(t *testing.T) {
	tests := []struct {
		name        string
		fCs         []string
		downloadDir string
		serverAddr  string
		expectErr   bool
	}{
		{
			"valid-full-single",
			[]string{
				"foo.file:b958bc7b84c18196b0f548eca3a88a2ee7b1ce81231c22c231cc1af99fc36f6c",
			},
			"./my-download-dir",
			"1.2.3.4:1234",
			false,
		},
		{
			"valid-full-multiple",
			[]string{
				"foo.file:b958bc7b84c18196b0f548eca3a88a2ee7b1ce81231c22c231cc1af99fc36f6c",
				"bar.baz:556610cdc01708847cbb1f0a051b302b46dac41d20ef9c25d386877aaef784fa",
				"foobar.bar:dcdf5ce055a53f714a45fd9d69c4a494e5282f9d0d0849202b3cb63823515a6c",
			},
			"./my-download-dir",
			"1.2.3.4:1234",
			false,
		},
		{
			"valid-incomplete-single",
			[]string{
				"foo.file",
			},
			"./my-download-dir",
			"1.2.3.4:1234",
			false,
		},
		{
			"valid-incomplete-multiple",
			[]string{
				"foo.file",
				"bar.baz",
				"foobar.bar",
			},
			"./my-download-dir",
			"1.2.3.4:1234",
			false,
		},
		{
			"valid-mixed-multiple",
			[]string{
				"foo.file:b958bc7b84c18196b0f548eca3a88a2ee7b1ce81231c22c231cc1af99fc36f6c",
				"bar.baz",
				"foobar.bar",
			},
			"./my-download-dir",
			"1.2.3.4:1234",
			false,
		},
		{
			"invalid-no-filename-no-checksum",
			[]string{
				"",
			},
			"./my-download-dir",
			"1.2.3.4:1234",
			true,
		},
		{
			"invalid-no-filename-but-checksum",
			[]string{
				":b958bc7b84c18196b0f548eca3a88a2ee7b1ce81231c22c231cc1af99fc36f6c",
			},
			"./my-download-dir",
			"1.2.3.4:1234",
			true,
		},
		{
			"invalid-filename-invalid-checksum",
			[]string{
				"foo.file:iamachecksum",
			},
			"./my-download-dir",
			"1.2.3.4:1234",
			true,
		},
		{
			"invalid-no-args",
			[]string{},
			"",
			"",
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			binary := os.Args[0]
			os.Args = []string{
				binary,
				getCmd,
				getFileSubCmd,
			}
			os.Args = append(os.Args, tt.fCs...)
			os.Args = append(os.Args, tt.downloadDir, tt.serverAddr)

			got, err := ParseArgs()

			if tt.expectErr && err == nil {
				t.Errorf("ParseArgs() expected error, yet err is nil; args: %v", spew.Sdump("\n", got))
				return
			} else if !tt.expectErr && err != nil {
				t.Errorf("ParseArgs() failed; err = %v, %v", err, spew.Sdump("\n", got))
			}

			if err != nil {
				return
			}

			cArgs := got.OperationArgs.(*ClientArgs)
			if cArgs.Command != ClientCommand(FileRequest) {
				t.Errorf("unexpected arg parse = %v, want = %v", cArgs.Command, FileRequest)
			}

			if len(cArgs.DownloadFiles) != len(tt.fCs) {
				t.Errorf("len of download dir and file checksums don't match = %v, want = %v", spew.Sdump("\n", cArgs.DownloadFiles), spew.Sdump("\n", tt.fCs))
			}

			var valueContained bool
			for f, cs := range cArgs.DownloadFiles {
				valueContained = false
				for _, fCheck := range tt.fCs {
					fCP := strings.FieldsFunc(fCheck, func(r rune) bool {
						return r == ':'
					})
					if f == fCP[0] {
						if !reflect.DeepEqual(cs, make([]byte, common.ChecksumSize)) {
							if reflect.DeepEqual(cs, []byte(fCP[1])) {
								t.Errorf("got = %v, want = %v", cs, fCP[1])
							}
						}
						valueContained = true
						break
					}
				}
				if !valueContained {
					t.Errorf("value %v not found in given file names = %v, want = %v", f, spew.Sdump("\n", cArgs.DownloadFiles), spew.Sdump("\n", tt.fCs))
				}
			}

			if cArgs.DownloadDir != tt.downloadDir {
				t.Errorf("unexpected arg parse = %s, want = %s", cArgs.DownloadDir, tt.downloadDir)
			}

			if cArgs.ServerAddr != tt.serverAddr {
				t.Errorf("unexpected arg parse = %s, want = %s", cArgs.ServerAddr, tt.serverAddr)
			}
		})
	}
}

func getSingleKeyValPair(m map[string][]byte) (f string, cs []byte, err error) {
	if len(m) != 1 {
		return "", nil, errors.New("given map does not exactly contain a single k:v pair")
	}
	for k, v := range m {
		f = k
		cs = v
	}
	return
}
