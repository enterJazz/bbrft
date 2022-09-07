package cli

type ClientCommand int

const (
	FileRequest ClientCommand = iota
	MetaDataRequest
)

type ClientArgs struct {
	command ClientCommand
	// checksum of to-be-fetched file; nil if none specified
	checksum string
	// target file name of operation; nil if none specified
	fileName string
	// target server address of operation
	serverAddr string
}

func (c *ClientArgs) GetOperationMode() OperationMode {
	return Client
}
