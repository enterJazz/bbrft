package cli

type ServerArgs struct {
	ServeDir string // path to directory of files served by server
	Port     int    // port which the server binds to on localhost
}

func (*ServerArgs) GetOperationMode() OperationMode {
	return Server
}
