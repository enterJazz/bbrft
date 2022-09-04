package cli

type ServerArgs struct {
	serveDir string // path to directory of files served by server
	port     int    // port which the server binds to on localhost
}

func (*ServerArgs) GetOperationMode() OperationMode {
	return Server
}
