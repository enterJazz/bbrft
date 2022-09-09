package cli

type ClientCommand int

const (
	FileRequest ClientCommand = iota
	MetaDataRequest
)

type ClientArgs struct {
	Command ClientCommand
	// map of target file name(s) with corresponding checksum (nil if none given) of operation; nil if none specified
	DownloadFiles map[string][]byte
	// dir in which requested files are stored
	DownloadDir string
	// target server address of operation
	ServerAddr string
	// disable / enable compression
	UseCompression bool
}

func (c *ClientArgs) GetOperationMode() OperationMode {
	return Client
}
