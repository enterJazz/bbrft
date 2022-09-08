package cli

type ClientCommand int

const (
	FileRequest ClientCommand = iota
	MetaDataRequest
)

type ClientArgs struct {
	Command ClientCommand
	// checksum of to-be-fetched file; nil if none specified
	Checksum string
	// target file name of operation; nil if none specified
	FileName string
	// dir in which requested files are stored
	DownloadDir string
	// target server address of operation
	ServerAddr string
}

func (c *ClientArgs) GetOperationMode() OperationMode {
	return Client
}
