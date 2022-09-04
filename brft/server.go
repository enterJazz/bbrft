package brft

import (
	"bytes"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"os"
	"sync"

	"github.com/davecgh/go-spew/spew"
	"gitlab.lrz.de/bbrft/brft/common"
	"gitlab.lrz.de/bbrft/brft/compression"
	"gitlab.lrz.de/bbrft/brft/messages"
	"gitlab.lrz.de/bbrft/btp"
	"gitlab.lrz.de/bbrft/cyberbyte"
	"gitlab.lrz.de/bbrft/log"
	"go.uber.org/zap"
)

type ServerOptions struct {
	ConnOptions
}

type Server struct {
	l *zap.Logger

	listener *btp.Listener
	numConns int

	// base path to the directory where the files are located
	basePath string

	options ServerOptions
}

func NewServer(
	l *zap.Logger,
	listen_addr string,
	basePath string,
	options *ServerOptions,
) (*Server, *net.UDPAddr, error) {
	opt := options
	if opt == nil {
		opt = &ServerOptions{NewDefaultOptions(l)}
	}

	laddr, err := net.ResolveUDPAddr(opt.btpOptions.Network, listen_addr)
	if err != nil {
		return nil, nil, err
	}

	listener, err := btp.Listen(opt.btpOptions, laddr, l)
	if err != nil {
		return nil, nil, err
	}

	return &Server{
		l:        l.With(log.FPeer("brft_server")),
		listener: listener,
		basePath: basePath,
		options:  *opt,
	}, laddr, nil
}

func (s *Server) Close() error {
	// TODO: stop the listener
	return nil
}

func (s *Server) ListenAndServe() error {
	// listen for incomming connections
	s.l.Debug("listening for incomming connections")

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			// we still want to accept future connections
			s.l.Error("failed to accept new connection", zap.Error(err))
			continue
		}

		// get a connection id here since we otherwise have to lock it
		s.numConns += 1
		id := s.numConns

		// extend logging
		l := s.l.With(
			zap.Int("id", id),
			zap.String("remote_addr", conn.LocalAddr().String()),
			zap.String("local_addr", conn.RemoteAddr().String()),
			zap.Bool("client_conn", false),
		)

		l.Info("accepted new connection")

		c := &Conn{
			l:        l,
			conn:     conn,
			basePath: s.basePath,
			isClient: false,
			streams:  make(map[messages.StreamID]*stream, 100),

			wg:      new(sync.WaitGroup),
			close:   make(chan struct{}),
			outCtrl: make(chan []byte, 100),
			outData: make(chan []byte, 100),

			options: s.options.ConnOptions,
		}

		// handle the connection
		go c.handleServerConnection()
	}
}

// FIXME: Figure out how to implement a gracefull shutdown when the BTP layer
// handles timeouts
// NOTE: This function can only run in a single thread, since we always need to
// read the whole message from the btp.Conn
func (c *Conn) handleServerConnection() {
	closeConn := func(messageType string, err error) {
		c.l.Error("unable to handle packet - closing connection",
			zap.String("message_type", messageType),
			zap.Error(err),
		)

		errClose := c.Close()
		if errClose != nil {
			c.l.Error("error while closing connection", zap.Error(errClose))
		}
	}

	// start routine for coordinating the sending messages
	go c.sendMessages(c.outCtrl, c.outData)

	// wait for incomming messages
	for {
		h, err := c.readHeader()
		if err != nil {
			// errors already handled by function
			// TODO: maybe only return if we cannot read, not if the header is
			// unknown - however the question is how we know how long the
			// message is in order to advance over it
			c.Close()
			return
		}

		// handle the message here to make sure that the whole read happens in
		// on go (multiple concurrent reads could lead to nasty mixups)
		switch h.MessageType {
		case messages.MessageTypeFileReq:
			// TODO: probably remove the timeout - I think we can't because otherwise the connection will forever be idle waiting for remaining bytes
			// decode the packet
			req := new(messages.FileReq)
			err := req.Decode(c.l, cyberbyte.NewString(c.conn, cyberbyte.DefaultTimeout))
			if err != nil {
				closeConn("FileRequest", fmt.Errorf("unable to decode FileRequest: %w", err))
				return
			}

			// create a new stream
			s := &stream{
				l: c.l.With(
					zap.Uint16("stream_id", uint16(req.StreamID)),
					zap.String("remote_addr", c.conn.LocalAddr().String()),
					zap.String("local_addr", c.conn.RemoteAddr().String()),
					zap.Bool("client_conn", false),
				),
				in: make(chan messages.BRFTMessage, 50),
			}

			// make sure the streamID is not already taken
			if c.isDuplicateStreamID(req.StreamID) {
				c.l.Error("streamID already exists - closing stream")
				// TODO: We cannot remove the existing stream - we have to only send the close message!
				// close the stream, but keep the connection open
				c.CloseStream(nil, messages.CloseReasonStreamIDTaken)
				break
			}

			c.streamsMu.RLock()
			c.streams[req.StreamID] = s
			c.streamsMu.RUnlock()

			s.in <- req

			// handle the file transfer negotiation
			c.handleServerStream(s)
			if err != nil {
				closeConn("FileRequest", err)
				return
			}

		case messages.MessageTypeStartTransmission:
			// TODO: probably remove the timeout - I think we can't because otherwise the connection will forever be idle waiting for remaining bytes
			// decode the packet
			st := new(messages.StartTransmission)
			err := st.Decode(c.l, cyberbyte.NewString(c.conn, cyberbyte.DefaultTimeout))
			if err != nil {
				closeConn("StartTransmission", fmt.Errorf("unable to decode StartTransmission: %w", err))
				return
			}

			c.streamsMu.RLock()
			if s, ok := c.streams[st.StreamID]; !ok {
				c.CloseStream(&st.StreamID, messages.CloseReasonUndefined)
			} else {
				s.in <- st
			}
			c.streamsMu.RUnlock()

			// TODO: Remove
			return
		case messages.MessageTypeClose:
			// TODO: Close the stream, but not the connection

		case messages.MessageTypeMetaDataReq:
			// TODO: handle the request (concurrently?)

		case messages.MessageTypeFileResp,
			messages.MessageTypeData,
			messages.MessageTypeMetaDataResp:
			c.l.Error("unexpected message type",
				zap.Uint8("type_encoding", uint8(h.MessageType)),
				zap.String("type", h.MessageType.String()),
			)
			// TODO: maybe close

		default:
			c.l.Error("unknown message type",
				zap.Uint8("type_encoding", uint8(h.MessageType)),
			)
			// TODO: need to close since we don't know how many bytes to skip
		}
	}
}

// TODO: Update comment
// FIXME: actually use the channels
// handleServerTransferNegotiation handles an incomming FileReq packet and
// sends a FileResponse packet if possible. For this, a new stream is created
// and added to the Conn. Since the client cannot create an association between
// the FileRequest and FileResponse, it relies on the server to send back
// FileResponses in the same order the FileRequests have been received.
// Therefore, the server MUST processes them sequentially!
// The function might close the stream and remove any information about it
// remaining on the Conn if a non-critical error occurs. As such, only critical
// errors that should lead to closing the whole btp.Conn are returned.
func (c *Conn) handleServerStream(s *stream) {

	c.wg.Add(1)
	defer c.wg.Done()

	// wait for the FileReq packet
	var msg messages.BRFTMessage
	select {
	case msg = <-s.in:
	case <-c.close:
		s.l.Info("stopping stream")
		return
	}

	// make sure its a FileResponse
	var req *messages.FileReq
	switch v := msg.(type) {
	case *messages.FileReq:
		req = v
	default:
		s.l.Error("unexpected message type",
			zap.String("expected_message_type", "FileRequest"),
			zap.String("actual_message", spew.Sdump("\n", v)),
		)
		c.CloseStream(&s.id, messages.CloseReasonUndefined)
		return
	}

	// create a FileResponse
	resp := &messages.FileResp{
		Status:   messages.FileRespStatusOk,
		StreamID: s.id,
		Checksum: make([]byte, 32), // empty checksum
	}

	// find the requested file
	var err error
	s.f, err = OpenFile(req.FileName, c.basePath)
	if errors.Is(err, os.ErrNotExist) {
		s.l.Error("file does not exists - closing stream")
		// close the stream, but keep the connection open
		c.CloseStream(&s.id, messages.CloseReasonFileNotFound)
		return
	} else if err != nil {
		s.l.Error("unable to open file - closing stream", zap.Error(err))
		// close the stream, but keep the connection open
		c.CloseStream(&s.id, messages.CloseReasonUndefined)
		return
	}
	s.fileName = req.FileName
	s.requestedChecksum = req.Checksum
	resp.FileSize = s.f.Size()
	resp.Checksum = s.f.Checksum()

	// handle resumption
	if req.Flags.IsSet(messages.FileReqFlagResumption) {
		// we need a checksum for resumption
		if bytes.Equal(req.Checksum, make([]byte, common.ChecksumSize)) {
			// close the stream, but keep the connection open
			c.CloseStream(&s.id, messages.CloseReasonResumeNoChecksum)
			return
		}

		s.isResumption = true
		// NOTE offset can only be set after the StartTransmission packet
		// has been received
	}
	// if the checksum is not zero we expect it to be identical to the one
	// of the requested file.
	if !bytes.Equal(req.Checksum, make([]byte, common.ChecksumSize)) &&
		!bytes.Equal(req.Checksum, s.f.Checksum()) {
		s.l.Error("invalid checksum",
			zap.String("actual_checksum", spew.Sdump("\n", s.f.Checksum())),
			zap.String("received_checksum", spew.Sdump("\n", req.Checksum)),
		)
		// close the stream, but keep the connection open
		c.CloseStream(&s.id, messages.CloseReasonChecksumInvalid)
		return
	}

	// handle the optional headers
	optHeaders := make(messages.OptionalHeaders, 0, 2)
	for _, opt := range req.OptHeaders {
		var respOpt messages.OptionalHeader
		// handl ethe optional header according to its type
		switch v := opt.(type) {
		case *messages.CompressionReqOptionalHeader:
			// create a optional header response
			if v.Algorithm != messages.CompressionReqHeaderAlgorithmGzip {
				respOpt = messages.NewCompressionRespOptionalHeader(
					messages.CompressionRespHeaderStatusUnknownAlgorithm,
				)
				break
			}

			// TODO: it would be nicer to have one compressor per Server
			s.comp = compression.NewGzipCompressor(s.l)
			s.chunkSize = v.ChunkSize()

			if resp.FileSize < s.comp.MinFileSize() {
				respOpt = messages.NewCompressionRespOptionalHeader(
					messages.CompressionRespHeaderStatusFileTooSmall,
				)
				break
			}

			respOpt = messages.NewCompressionRespOptionalHeader(
				messages.CompressionRespHeaderStatusOk,
			)

		case *messages.CompressionRespOptionalHeader:
			c.l.Error("got an unexpected optional header type",
				zap.String("dump", spew.Sdump("\n", opt)),
			)
			// adjust the status
			if messages.FileRespStatusUnexpectedOptionalHeader.HasPrecedence(resp.Status) {
				resp.Status = messages.FileRespStatusUnexpectedOptionalHeader
			}
			// TODO: maybe this should not be tollerated, since it indicates
			// 		that something went wrong
			// NOTE: according to our specs we could also close the
			// connection, but let's give the client a chance to continue
			// without the option
		case *messages.UnknownOptionalHeader:
			c.l.Error("got an unkown optional header type",
				zap.String("dump", spew.Sdump("\n", opt)),
			)
			// adjust the status, but keep the stream open
			if messages.FileRespStatusUnsupportedOptionalHeader.HasPrecedence(resp.Status) {
				resp.Status = messages.FileRespStatusUnsupportedOptionalHeader
			}
			// NOTE: according to our specs we could also close the
			// connection, but let's give the client a chance to continue
			// without the option
		default:
			c.l.Error("unexpected optional header type [implementation error]",
				zap.String("dump", spew.Sdump("\n", opt)),
			)
			// actually close the stream since something went wrong on our
			// side, but keep the connection open
			c.CloseStream(&s.id, messages.CloseReasonUndefined)
			return
		}

		if respOpt != nil {
			optHeaders = append(optHeaders, respOpt)
		}
	}

	resp.OptHeaders = optHeaders

	// send the response
	data, err := resp.Encode(s.l)
	if err != nil {
		c.l.Error("unable to encode FileResponse",
			zap.String("packet", spew.Sdump("\n", resp)),
			zap.Error(err),
		)
		// close the stream, but keep the connection open
		// NOTE: The stream object has not yet been added to c.streams
		// (i.e. no cleanup needed)
		c.CloseStream(&s.id, messages.CloseReasonUndefined)
		return
	}

	s.l.Debug("sending FileResponse",
		zap.String("file_request", spew.Sdump("\n", req)),
		zap.String("packet", spew.Sdump("\n", resp)),
		zap.String("packet_encoded", spew.Sdump("\n", data)),
	)

	// TODO: maybe introduce a high timeout (~ 10s)
	// send the data to the sender routing
	select {
	case c.outCtrl <- data:
	case <-c.close:
		s.l.Info("stopping stream")
		return
	}

	// wait for the subsequent StartTransmission packet
	select {
	case msg = <-s.in:
	case <-c.close:
		s.l.Info("stopping stream")
		return
	}

	// make sure its a FileResponse
	var st *messages.StartTransmission
	switch v := msg.(type) {
	case *messages.StartTransmission:
		st = v
	default:
		s.l.Error("unexpected message type",
			zap.String("expected_message_type", "StartTransmission"),
			zap.String("actual_message", spew.Sdump("\n", v)),
		)
		c.CloseStream(&s.id, messages.CloseReasonUndefined)
		return
	}

	// once again check the checksum
	if !bytes.Equal(st.Checksum, s.f.Checksum()) {
		s.l.Error("invalid checksum",
			zap.String("actual_checksum", spew.Sdump("\n", s.f.Checksum())),
			zap.String("received_checksum", spew.Sdump("\n", st.Checksum)),
		)
		c.CloseStream(&s.id, messages.CloseReasonChecksumInvalid)
		return
	}

	// make sure the offset (if set) is not too big for the file
	if st.Offset >= s.f.Size() {
		s.l.Error("invalid offset",
			zap.Uint64("offset", st.Offset),
			zap.Uint64("file_size", s.f.Size()),
		)
		c.CloseStream(&s.id, messages.CloseReasonInvalidOffset)
		return
	}

	s.l.Debug("start sending data packets",
		zap.String("start_transmission", spew.Sdump("\n", st)),
		//TODO:  zap.String("packet", spew.Sdump("\n",resp)),
		// TODO: zap.String("packet_encoded", spew.Sdump("\n",data)),
	)

	// start sending data from the file
	for {

	}
}

// TODO: somehow we should block other routines from sending as well or even modify the stream
// 		Maybe:
//			- sendData returns channels: cancel (to close from the Conn)
//			- remove the stream from c.streams

func (c *Conn) sendData(s *stream) {

	// TODO: Implement
	// if s.isSending {
	// 	return ErrStreamIsSending
	// }

	// for {
	// 	// TODO: read a chunk size from the file

	// 	// TODO: optionally compress the data

	// 	// TODO: assemble the packet

	// 	// TODO: send the packet
	// }
}

// newStreamID generates a new unique streamID for the connection
func (c *Conn) newStreamID() messages.StreamID {
	if !c.isClient {
		c.l.Warn("only clients should generate new StreamIDs")
	}

	c.streamsMu.RLock()
	c.streamsMu.RUnlock()

	exists := false
	for {
		newId := messages.StreamID(rand.Uint32())
		for id := range c.streams {
			// apart from existing ids also disallow 0, since all sessionIDs
			// will be that on by default
			if newId == id || newId == 0 {
				exists = true
			}
		}
		if !exists {
			return newId
		}
	}
}

// newStreamID generates a new unique streamID for the connection
func (c *Conn) isDuplicateStreamID(newId messages.StreamID) bool {
	if c.isClient {
		c.l.Warn("only servers should have to check StreamIDs")
	}

	c.streamsMu.RLock()
	c.streamsMu.RUnlock()

	for id := range c.streams {
		// apart from existing ids also disallow 0, since all sessionIDs
		// will be that on by default
		if newId == id || newId == 0 {
			return true
		}
	}
	return false
}

// TODO: Probably not needed
// func (s *Server)Serve(l net.Listener) error {
// 	return nil
// }

// func (s *Server)SetKeepAlivesEnabled() error {
// 	return nil
// }

// More graceful shutdown as Close. Might be useful - see: https://pkg.go.dev/net/http#Server.Shutdown
// func (s *Server) Shutdown() error {
// 	return nil
// }
