package brft

import (
	"bytes"
	"fmt"
	"net"
	"sync"

	"github.com/davecgh/go-spew/spew"
	"gitlab.lrz.de/bbrft/brft/common"
	"gitlab.lrz.de/bbrft/brft/compression"
	"gitlab.lrz.de/bbrft/brft/messages"
	"gitlab.lrz.de/bbrft/btp"
	"gitlab.lrz.de/bbrft/log"
	"go.uber.org/zap"
)

// TODO: move
const (
	serverAddrStr = "127.0.0.1:1337"
)

func Dial(
	l *zap.Logger,
	addr string, // server address
	downloadDir string,
	options *ConnOptions,
) (*Conn, error) {
	c := &Conn{
		l:        l.With(log.FPeer("brft_client")),
		basePath: downloadDir, // TODO: Make sure that it actually exists / create it
		isClient: true,
		streams:  make(map[messages.StreamID]*stream, 100),

		wg:      new(sync.WaitGroup),
		close:   make(chan struct{}),
		outCtrl: make(chan []byte, 100),
		outData: make(chan []byte, 100),
	}

	if options == nil {
		c.options = NewDefaultOptions(l)
	} else {
		c.options = *options
	}

	raddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("unable to resolve server address: %w", err)
	}

	c.conn, err = btp.Dial(c.options.btpOptions, nil, raddr, l)
	if err != nil {
		return nil, fmt.Errorf("unable to create connection: %w", err)
	}

	go c.handleClientConnection()

	return c, nil
}

func (c *Conn) DownloadFile(
	fileName string,
) error {
	if !c.isClient {
		return ErrExpectedClientConnection
	}

	c.l.Info("initiating new dowload")

	// TODO: See if this is a resumption (maybe make some kind of dialoge)
	// resumption := false
	// if resumption {
	// 	req.Flags = append(req.Flags, messages.FileReqFlagResumption)
	// 	req.Checksum = make([]byte, common.ChecksumSize)

	// 	var offset uint64 = 0
	// 	s.offset = offset
	// 	s.isResumption = true
	// }

	// create a new request
	sid := c.newStreamID()
	req := &messages.FileReq{
		StreamID: sid,
		// OptionalHeaders for the upcomming file transfer
		OptHeaders: make(messages.OptionalHeaders, 0, 1),
		// FileName of the requested file, can be at most 255 characters long
		FileName: fileName,
		// Checksum is the checksum of a previous partial download or if a specific
		// file version shall be requested. Might be unitilized or zeroed.
		Checksum: make([]byte, common.ChecksumSize),
	}

	s := &stream{
		l: c.l.With(
			zap.String("file_name", fileName),
			zap.Uint16("stream_id", uint16(sid)),
		),
		id:                sid,
		fileName:          fileName,
		requestedChecksum: req.Checksum,
		//isResumption: , TODO: set
		//offset: , TODO: set
		in: make(chan messages.BRFTMessage, 50),
	}

	// handle compression
	if c.options.GetPreferredCompression() != messages.CompressionReqHeaderAlgorithmReserved {
		h := messages.NewCompressionReqOptionalHeader(
			c.options.GetPreferredCompression(),
			uint8(c.options.chunkSizeFactor),
		)
		req.OptHeaders = append(req.OptHeaders, h)
		s.chunkSize = h.ChunkSize()
		// NOTE: compressor must be set once the server confirms compression
	}

	data, err := req.Encode(c.l)
	if err != nil {
		return fmt.Errorf("unable to encode FileRequest: %w", err)
	}

	s.l.Debug("sending FileRequest",
		zap.String("packet", spew.Sdump("\n", req)),
		zap.String("packet_encoded", spew.Sdump("\n", data)),
	)

	// write the encoded message
	_, err = c.conn.Write(data)
	if err != nil {
		return fmt.Errorf("unable to encode FileRequest: %w", err)
	}

	// add the stream to map of streams
	c.streamsMu.Lock()
	c.streams[s.id] = s
	c.streamsMu.Unlock()

	// start handling of the stream
	c.handleClientStream(s)

	// clean up the stream corresponding to the requested file after a set
	// amount of time
	// TODO: adjust to new structure
	// go func() {
	// 	time.Sleep(time.Minute)
	// 	c.reqStreamsMu.Lock()
	// 	defer c.reqStreamsMu.Unlock()
	// 	if s, ok := c.reqStreams[fileName]; ok {
	// 		s.l.Warn("cleaning up unanswered FileRequest")
	// 		delete(c.reqStreams, fileName)
	// 	}
	// }()

	return nil
}

// NOTE: This function can only run in a single thread, since we always need to
// read the whole message from the btp.Conn
func (c *Conn) handleClientConnection() {
	closeConn := func(messageType string, err error) {
		close(c.close)
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

	for {
		inMsg, h, err := c.readMsg()
		if err != nil {
			// errors already handled by function
			// TODO: maybe only return if we cannot read, not if the header is
			// unknown - however the question is how we know how long the
			// message is in order to advance over it
			if inMsg != nil {
				closeConn(inMsg.Name(), err)
			} else {
				closeConn("could not read message_type", err)
			}
			return
		}

		// handle the message here to make sure that the whole read happens in
		// on go (multiple concurrent reads could lead to nasty mixups)
		switch msg := inMsg.(type) {
		case *messages.FileResp:
			s := c.getStream(msg.StreamID)
			if s != nil {
				s.in <- msg
			} else {
				// TODO: We cannot remove the existing stream - we have to only send the close message!
				c.CloseStream(nil, messages.CloseReasonUndefined)
			}

			// handle the file transfer negotiation
			// err = c.handleClientTransferNegotiation(resp)
			// if err != nil {
			// 	closeConn("FileResponse", err)
			// 	return
			// }

		case *messages.Data:
			s := c.getStream(msg.StreamID)
			if s != nil {
				s.in <- msg
			} else {
				// TODO: We cannot remove the existing stream - we have to only send the close message!
				c.CloseStream(nil, messages.CloseReasonUndefined)
			}

		case *messages.Close:
			// TODO: handle gracefully
			s := c.getStream(msg.StreamID)
			if s != nil {
				s.in <- msg
			} else {
				// TODO: We cannot remove the existing stream - we have to only send the close message!
				c.CloseStream(nil, messages.CloseReasonUndefined)
			}

		case *messages.MetaResp:
			// TODO: handle the packet and display it somehow

		case *messages.FileReq,
			*messages.StartTransmission,
			*messages.MetaReq:
			c.l.Error("unexpected message type",
				zap.Uint8("type_encoding", uint8(h.MessageType)),
				zap.String("type", h.MessageType.Name()),
			)
			// TODO: maybe close

		default:
			c.l.Error("unknown message type",
				zap.Uint8("type_encoding", uint8(h.MessageType)),
			)
			// TODO: Probably close
		}

	}
}

// TODO: update comment
// handleClientTransferNegotiation handles an incomming FileResp packet and
// sends a StartTransmission packet if possible. It uses the stream saved in
// c.reqStreams[0]. Because of this is has to be made sure that the streams are
// added to c.reqStreams in the same order that they are sent to the server and
// that the server processes them sequentially as well!
// The function might close the stream and remove any information about it
// remaining on the Conn if a non-critical error occurs. As such, only critical
// errors that should lead to closing the whole btp.Conn are returned.
func (c *Conn) handleClientStream(s *stream) {

	c.wg.Add(1)
	defer c.wg.Done()

	// update the stream
	s.l = s.l.With(
		zap.String("remote_addr", c.conn.LocalAddr().String()),
		zap.String("local_addr", c.conn.RemoteAddr().String()),
		zap.Bool("client_conn", true),
	)

	// wait for incomming messages
	var msg messages.BRFTMessage
	select {
	case msg = <-s.in:
	case <-c.close:
		s.l.Info("stopping stream")
		return
	}

	// make sure its a FileResponse
	var resp *messages.FileResp
	switch v := msg.(type) {
	case *messages.FileResp:
		resp = v
	default:
		s.l.Error("unexpected message type",
			zap.String("expected_message_type", "FileResponse"),
			zap.String("actual_message", spew.Sdump("\n", v)),
		)
		c.CloseStream(&s.id, messages.CloseReasonUndefined)
		return
	}

	// set & check the checksum
	if !bytes.Equal(s.requestedChecksum, make([]byte, common.ChecksumSize)) &&
		!bytes.Equal(s.requestedChecksum, resp.Checksum) {
		// TODO: potentially add a dialogue to allow the resumption of the download
		s.l.Error("checksums do not match",
			zap.String("requestedChecksum", spew.Sdump("\n", s.requestedChecksum)),
			zap.String("checksum", spew.Sdump("\n", s.f.Checksum())),
		)

		// close the stream, but keep the connection open
		c.CloseStream(&s.id, messages.CloseReasonChecksumInvalid)
		return
	}

	// check the optional headers
	for _, opt := range resp.OptHeaders {
		// handl ethe optional header according to its type
		switch v := opt.(type) {
		case *messages.CompressionRespOptionalHeader:
			// if a compression has actually been requested we potentially want to enable the compression
			switch v.Status {
			case messages.CompressionRespHeaderStatusOk:
				s.l.Debug("enabling compression")
				s.comp = compression.NewGzipCompressor(s.l)
			case messages.CompressionRespHeaderStatusFileTooSmall:
				s.l.Info("disabling compression - file too small")
			case messages.CompressionRespHeaderStatusNoCompression:
				s.l.Info("disabling compression - server denied")
			case messages.CompressionRespHeaderStatusUnknownAlgorithm:
				s.l.Info("disabling compression - server unknown alogrithm")
			}
		case *messages.CompressionReqOptionalHeader:
			c.l.Error("got an unexpected optional header type",
				zap.String("dump", spew.Sdump("\n", opt)),
			)

			// close the stream, but keep the connection open
			c.CloseStream(&s.id, messages.CloseReasonUnexpectedOptionalHeader)
			return

		case *messages.UnknownOptionalHeader:
			c.l.Error("got an unkown optional header type",
				zap.String("dump", spew.Sdump("\n", opt)),
			)

			// close the stream, but keep the connection open
			c.CloseStream(&s.id, messages.CloseReasonUnsupportedOptionalHeader)
			return

		default:
			c.l.Error("unexpected optional header type [implementation error]",
				zap.String("dump", spew.Sdump("\n", opt)),
			)

			// close the stream, but keep the connection open
			c.CloseStream(&s.id, messages.CloseReasonUnsupportedOptionalHeader)
			return
		}
	}

	// TODO: Create the file - is there a way to allocate the memory?
	var err error
	s.f, err = NewFile(s.l, s.fileName, c.basePath, resp.Checksum)
	if err != nil {
		c.l.Error("unable to initialize file",
			zap.String("base_path", c.basePath),
			zap.String("s.checksum", spew.Sdump("\n", resp.Checksum)),
			zap.Error(err),
		)
		// close the stream, but keep the connection open´
		c.CloseStream(&s.id, messages.CloseReasonUndefined) // there's only a reason for files that are too big
		return
	}

	st := messages.StartTransmission{
		StreamID: resp.StreamID,
		Checksum: resp.Checksum,
	}

	// if this is a resumption, then set the offset on the packet
	if s.isResumption {
		if s.offset == 0 {
			s.l.Warn("resumption set, but offset is zero")
		}

		st.Offset = s.offset
	}

	data, err := st.Encode(s.l)
	if err != nil {
		c.l.Error("unable to encode StartTransmission",
			zap.String("packet", spew.Sdump("\n", st)),
			zap.Error(err),
		)
		// close the stream, but keep the connection open
		c.CloseStream(&s.id, messages.CloseReasonUndefined)
		return
	}

	s.l.Debug("sending StartTransmission",
		zap.String("file_response", spew.Sdump("\n", resp)),
		zap.String("packet", spew.Sdump("\n", st)),
		zap.String("packet_encoded", spew.Sdump("\n", data)),
	)

	// TODO: maybe introduce a high timeout (~ 10s)
	// send the data to the sender routing
	select {
	case c.outCtrl <- data:
	case <-c.close:
		s.l.Warn("closed before stream started")
		return
	}

	// FIXME: Add decompression
	// TODO: Start waiting for Data packets
	for {
		var msg messages.BRFTMessage

		select {
		case msg = <-s.in:
		case <-c.close:
			s.l.Warn("closed before stream started")
			return
		}

		switch m := msg.(type) {
		case *messages.Data:
			s.f.Write(m.Data)
		case *messages.Close:
			if m.Reason == messages.CloseReasonTransferComplete {
				// TODO: implement
				// err := s.f.CheckChecksum()
				// if err != nil {
				// 	s.l.Error("unable to clean up file", zap.Error(err))
				// 	return
				// }

				// clean up the file
				s.l.Info("finished receiving file")
				err := s.f.StripChecksum()
				if err != nil {
					s.l.Error("unable to clean up file", zap.Error(err))
					return
				}
			}

			return
		default:
			s.l.Warn("unexpected msg type received in stream", zap.String("packet", spew.Sdump("\n", st)))
		}
	}
}
