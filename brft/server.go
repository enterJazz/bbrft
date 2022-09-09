package brft

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net"
	"os"
	"sync"

	"github.com/davecgh/go-spew/spew"
	"gitlab.lrz.de/bbrft/brft/common"
	"gitlab.lrz.de/bbrft/brft/compression"
	"gitlab.lrz.de/bbrft/brft/messages"
	"gitlab.lrz.de/bbrft/btp"
	"go.uber.org/zap"
)

const (
	EMPTY_FILE_NAME string = ""

	// if enabled in combination with debug log level packet contents
	// will be logged (will drastically impact performance)
	DEBUG_PACKET_CONTENT = false
)

type ServerOptions struct {
	ConnOptions
}

type Server struct {
	l *zap.Logger

	listener *btp.Listener
	numConns int

	listen_addr *net.UDPAddr

	// base path to the directory where the files are located
	basePath string

	options ServerOptions
}

func NewServer(
	l *zap.Logger,
	listenAddr string,
	basePath string,
	options *ServerOptions,
) (*Server, *net.UDPAddr, error) {
	opt := options
	if opt == nil {
		opt = &ServerOptions{NewDefaultOptions(l, compression.DefaultCompressionEnabled)}
	}

	laddr, err := net.ResolveUDPAddr(opt.BtpOptions.Network, listenAddr)
	if err != nil {
		return nil, nil, err
	}

	btpLogger := l
	if options.BtpLogger != nil {
		btpLogger = opt.BtpLogger
	}

	listener, err := btp.Listen(opt.BtpOptions, laddr, btpLogger)
	if err != nil {
		return nil, nil, err
	}

	return &Server{
		l:           l.With(FPeer("brft_server")),
		listener:    listener,
		basePath:    basePath,
		listen_addr: laddr,
		options:     *opt,
	}, laddr, nil
}

func (s *Server) Close() error {
	// TODO: stop the listener
	return nil
}

func (s *Server) ListenAndServe() error {
	// listen for incomming connections
	s.l.Info("listening for incomming connections", zap.String("addr", s.listen_addr.String()))

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

			options: s.options.ConnOptions.Clone(s.options.BtpLogger),
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
		// FIXME: this is blocking frever - we need to either read or listen on c.close channel
		inMsg, h, err := c.readMsg()
		if err != nil {
			closeConn("could not read message_type", err)
			return
		}

		// handle the message here to make sure that the whole read happens in
		// on go (multiple concurrent reads could lead to nasty mixups)
		switch msg := inMsg.(type) {
		case *messages.FileReq:
			// create a new stream
			s := &stream{
				l: c.l.With(
					zap.Uint16("stream_id", uint16(msg.StreamID)),
					zap.String("file_name", msg.FileName),
				),
				id:        msg.StreamID,
				progress:  make(chan Progress),
				chunkSize: messages.ComputeChunkSize(c.options.chunkSizeFactor),
				in:        make(chan messages.BRFTMessage, 50),
			}

			// make sure the streamID is not already taken
			if c.isDuplicateStreamID(s.id) {
				c.l.Error("received FileReq packet for already existing stream",
					zap.String("stream_id", msg.StreamID.String()),
				)

				// TODO: Maybe we should remove the stream afterall. The peer
				// 		will close the stream and our packets would just trigger
				//		another Close packet
				// cannot remove the stream since it does not exist - only can
				// send a close message
				c.sendClosePacket(msg.StreamID, messages.CloseReasonUndefined)
				break
			}

			c.streamsMu.RLock()
			c.streams[msg.StreamID] = s
			c.streamsMu.RUnlock()

			s.in <- msg

			// handle the file transfer negotiation
			go c.handleServerStream(s)
			if err != nil {
				closeConn("FileRequest", err)
				return
			}

		case *messages.StartTransmission:
			s := c.getStream(msg.StreamID)

			// ensure startTransmission checksum matches stream checksum
			// if checksum invalid abort
			if !bytes.Equal(s.f.checksum, msg.Checksum) {
				c.sendClosePacket(msg.StreamID, messages.CloseReasonChecksumInvalid)
				continue
			}

			// validate incoming offset
			if msg.Offset != 0 && msg.Offset >= uint64(s.f.stat.Size()) {
				c.sendClosePacket(msg.StreamID, messages.CloseReasonInvalidOffset)
				continue
			} else {
				s.l.Info("resuming download", zap.Uint64("offset", msg.Offset))
			}
			s.offset = msg.Offset

			if s != nil {
				s.in <- msg
			} else {
				c.l.Warn("received StartTransmission packet for unknown stream",
					zap.String("stream_id", msg.StreamID.String()),
				)

				// cannot remove the stream since it does not exist - only can
				// send a close message
				c.sendClosePacket(msg.StreamID, messages.CloseReasonUndefined)
			}

		case *messages.Close:
			// send the message to the stream to handle it (i.e. close itself)
			s := c.getStream(msg.StreamID)
			if s != nil {
				s.in <- msg
			} else {
				c.l.Warn("received Close packet for unknown stream",
					zap.String("stream_id", msg.StreamID.String()),
				)

				// since we did get a close packet it's safe to assume the peer
				// closed the stream. Therefore, we don't need to send a close packet
			}

		case *messages.MetaReq:
			c.handleMetaDataReq(*msg)

		case *messages.FileResp,
			*messages.Data,
			*messages.MetaResp:
			c.l.Error("unexpected message type",
				zap.Uint8("type_encoding", uint8(h.MessageType)),
				zap.String("type", h.MessageType.Name()),
			)
			// TODO: maybe close

		default:
			c.l.Error("unknown message type",
				zap.Uint8("type_encoding", uint8(h.MessageType)),
			)

			// close whole connection since we don't know how much bytes to advance
			closeConn(fmt.Sprintf("[%d]", h.MessageType), errors.New("unknown message type"))
			return
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
	defer s.l.Info("closed stream")

	// wait for the FileReq packet
	var msg messages.BRFTMessage
	select {
	case msg = <-s.in:
	case <-c.close:
		c.CloseStream(s, false, 0)
		return
	}

	// make sure its a FileResponse
	var req *messages.FileReq
	switch v := msg.(type) {
	case *messages.FileReq:
		req = v
	case *messages.Close:
		s.l.Warn("received Close packet",
			zap.String("reason", v.Reason.String()),
		)
		c.CloseStream(s, false, 0)
		return
	default:
		s.l.Error("unexpected message type",
			zap.String("expected_message_type", "FileRequest"),
			zap.String("actual_message", spew.Sdump("\n", v)),
		)
		c.CloseStream(s, true, messages.CloseReasonUndefined)
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
		c.CloseStream(s, true, messages.CloseReasonFileNotFound)
		return
	} else if err != nil {
		s.l.Error("unable to open file - closing stream", zap.Error(err))
		// close the stream, but keep the connection open
		c.CloseStream(s, true, messages.CloseReasonUndefined)
		return
	}
	s.fileName = req.FileName
	s.requestedChecksum = req.Checksum
	s.totalSize = s.f.Size()
	resp.FileSize = s.totalSize
	resp.Checksum = s.f.Checksum()

	// handle resumption
	if req.Flags.IsSet(messages.FileReqFlagResumption) {
		// we need a checksum for resumption
		if bytes.Equal(req.Checksum, make([]byte, common.ChecksumSize)) {
			// close the stream, but keep the connection open
			c.CloseStream(s, true, messages.CloseReasonResumeNoChecksum)
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
		c.CloseStream(s, true, messages.CloseReasonChecksumInvalid)
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

			s.comp = compression.NewGzipCompressor(s.l)

			if resp.FileSize < s.comp.MinFileSize() {
				respOpt = messages.NewCompressionRespOptionalHeader(
					messages.CompressionRespHeaderStatusFileTooSmall,
				)
				s.comp = nil
				break
			}

			// TODO: it would be nicer to have one compressor per Server
			s.chunkSize = v.ChunkSize()

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
			c.CloseStream(s, true, messages.CloseReasonUndefined)
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
		c.CloseStream(s, true, messages.CloseReasonUndefined)
		return
	}

	if DEBUG_PACKET_CONTENT {
		s.l.Debug("sending FileResponse",
			zap.String("file_request", spew.Sdump("\n", req)),
			zap.String("packet", spew.Sdump("\n", resp)),
			zap.String("packet_encoded", spew.Sdump("\n", data)),
		)
	} else {
		s.l.Debug("sending FileResponse")
	}

	// TODO: maybe introduce a high timeout (~ 10s)
	// send the data to the sender routing
	select {
	case c.outCtrl <- data:
	case <-c.close:
		c.CloseStream(s, false, 0)
		return
	}

	// wait for the subsequent StartTransmission packet
	select {
	case msg = <-s.in:
	case <-c.close:
		c.CloseStream(s, false, 0)
		return
	}

	// make sure its a FileResponse
	var st *messages.StartTransmission
	switch v := msg.(type) {
	case *messages.StartTransmission:
		st = v
	case *messages.Close:
		s.l.Warn("received Close packet",
			zap.String("reason", v.Reason.String()),
		)
		c.CloseStream(s, false, 0)
		return
	default:
		s.l.Error("unexpected message type",
			zap.String("expected_message_type", "StartTransmission"),
			zap.String("actual_message", spew.Sdump("\n", v)),
		)
		c.CloseStream(s, true, messages.CloseReasonUndefined)
		return
	}

	// once again check the checksum
	if !bytes.Equal(st.Checksum, s.f.Checksum()) {
		s.l.Error("invalid checksum",
			zap.String("actual_checksum", spew.Sdump("\n", s.f.Checksum())),
			zap.String("received_checksum", spew.Sdump("\n", st.Checksum)),
		)
		c.CloseStream(s, true, messages.CloseReasonChecksumInvalid)
		return
	}

	// make sure the offset (if set) is not too big for the file
	if st.Offset >= s.f.Size() {
		s.l.Error("invalid offset",
			zap.Uint64("offset", st.Offset),
			zap.Uint64("file_size", s.f.Size()),
		)
		c.CloseStream(s, true, messages.CloseReasonInvalidOffset)
		return
	}

	s.f.SeekOffset(st.Offset)
	s.l.Debug("seeking offset in file", zap.Uint64("offset", st.Offset))

	if DEBUG_PACKET_CONTENT {
		s.l.Debug("validated StartTransmission packet",
			zap.String("start_transmission", spew.Sdump("\n", st)),
		)
	} else {
		s.l.Debug("validated StartTransmission packet")
	}

	// start sending data from the file
	for {
		// assemble the data packet
		d := messages.Data{
			StreamID: s.id,
			Data:     make([]byte, s.chunkSize),
		}

		// read a chunk size from the file
		lastPacket := false
		n, err := s.f.Read(d.Data)
		if err != nil {
			s.l.Error("unable to read from file", zap.Error(err))

			// close the stream, but keep the connection open
			c.CloseStream(s, true, messages.CloseReasonUndefined)
			return
		} else if n < len(d.Data) {
			lastPacket = true
			d.Data = d.Data[:n]
		}
		dLen := n

		// (optionally) compress the data
		if s.comp != nil {
			d.Data, err = s.comp.Compress(d.Data)
			if err != nil {
				s.l.Error("unable to compress chunk",
					zap.String("data", spew.Sdump("\n", d.Data)),
					zap.Error(err),
				)

				// close the stream, but keep the connection open
				c.CloseStream(s, true, messages.CloseReasonUndefined)
				return
			}
		}

		data, err := d.Encode(s.l)
		if err != nil {
			c.l.Error("unable to encode Data",
				zap.String("packet", spew.Sdump("\n", d)),
				zap.Error(err),
			)

			// close the stream, but keep the connection open
			c.CloseStream(s, true, messages.CloseReasonUndefined)
			return
		}

		if DEBUG_PACKET_CONTENT {
			s.l.Debug("sending Data packet",
				zap.String("packet", spew.Sdump("\n", d)),
				// TODO: re-enable zap.String("packet_encoded", spew.Sdump("\n", data)),
			)
		}

		// TODO: maybe introduce a high timeout (~ 10s)
		// send the data to the sender routing
		select {
		case c.outData <- data:
		case <-c.close:
			c.CloseStream(s, false, 0)
			return
		}

		// TODO: Somehow we send more data than we initially advertised
		s.updateProgress(len(d.Data), dLen)

		if lastPacket {
			// sent last packet
			s.l.Debug("sent last packet")

			// signalize the peer that the transfer is complete by sending a
			// close packet with the TransferComplete reason
			c.CloseStream(s, true, messages.CloseReasonTransferComplete)
			return
		}
	}
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

func (c *Conn) handleMetaDataReq(metaReq messages.MetaReq) {
	var metaItems []*messages.MetaItem

	// helper in case of an error
	sendEmptyResponse := func() {
		// send collected metaResps
		data, err := new(messages.MetaResp).Encode(c.l)
		if err != nil {
			c.l.Error("failed to encode metaDataResp - skipping", zap.Error(err))
		}
		c.outCtrl <- data
	}

	// empty file name: list all available files in dir
	if metaReq.FileName == EMPTY_FILE_NAME {
		dirContents, err := ioutil.ReadDir(c.basePath)
		if err != nil {
			c.l.Error("unable to open dir - sending empty response", zap.Error(err))
			sendEmptyResponse()
			return
		}
		for _, fInfo := range dirContents {
			if !fInfo.IsDir() {
				metaItem, err := messages.NewMetaItem(fInfo.Name(), nil, nil)
				if err != nil {
					c.l.Error("unable to create metaDataItem - skipping this item", zap.Error(err))
					continue
				}
				metaItems = append(metaItems, metaItem)
			}
		}
	} else {
		// specific filename specified; get extended metadata
		f, err := OpenFile(metaReq.FileName, c.basePath)
		if errors.Is(err, os.ErrNotExist) {
			c.l.Error("file does not exist - sending empty response", zap.String("file_name", metaReq.FileName))
			sendEmptyResponse()
			return
		} else if err != nil {
			c.l.Error("unable to open file - sending empty response", zap.Error(err))
			sendEmptyResponse()
			return
		}

		name := f.f.Name()
		size := f.Size()
		checksum := f.Checksum()
		metaItem, err := messages.NewMetaItem(
			name,
			&size,
			checksum,
		)
		if err != nil {
			c.l.Error("unable to create metaDataItem - returning empty response", zap.Error(err))
			sendEmptyResponse()
			return
		}
		metaItems = append(metaItems, metaItem)
	}

	// split metaItems into metaResps
	metaResps, err := messages.NewMetaResps(metaItems)
	if err != nil {
		c.l.Error("unable to create metaDataResp - returning empty response", zap.Error(err))
		panic("todo")
	}

	// send collected metaResps
	for _, metaResp := range metaResps {
		data, err := metaResp.Encode(c.l)
		if err != nil {
			c.l.Error("failed to encode metaDataResp - skipping", zap.Error(err))
			continue
		}

		if DEBUG_PACKET_CONTENT {
			c.l.Debug("sending MetaDataResponses",
				zap.String("metadata_responses", spew.Sdump("\n", metaResps)),
				zap.String("metadata_response", spew.Sdump("\n", metaResp)),
				zap.String("packet_encoded", spew.Sdump("\n", data)),
			)
		} else {
			c.l.Debug("sending MetaDataResponses")
		}

		c.l.Debug("waiting")
		// TODO: maybe introduce a high timeout (~ 10s)
		// send the data to the sender routing
		select {
		case c.outCtrl <- data:
		case <-c.close:
			// we're only receiving the channel message, so no need to get proactive
			return
		}
		c.l.Debug("done waiting")
	}
}
