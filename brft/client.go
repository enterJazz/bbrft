package brft

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"strconv"
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

// should be performed single threaded as waits for metaDataResp on connection in lock-step fashion
func (c *Conn) ListFileMetaData(
	fileName string,
) error {
	if !c.isClient {
		return ErrExpectedClientConnection
	}

	l := c.l.With(zap.String("filename", fileName))

	l.Info("fetching metadata")

	// create the request
	metaReq, err := messages.NewMetaReq(fileName)
	if err != nil {
		return fmt.Errorf("failed to create MetaDataReq: %w", err)
	}

	data, err := metaReq.Encode(c.l)
	if err != nil {
		return fmt.Errorf("unable to encode MetaDataReq: %w", err)
	}

	// TODO: maybe introduce a high timeout (~ 10s)
	// send the data to the sender routing
	select {
	case c.outCtrl <- data:
	case <-c.close:
		// we're only receiving the channel message, so no need to get proactive
		c.l.Warn("channel got closed during active MetaDataRequest")
		return nil
	}

	l.Debug("sent MetaDataRequest")

	return nil
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

	return nil
}

// NOTE: This function can only run in a single thread, since we always need to
// read the whole message from the btp.Conn
func (c *Conn) handleClientConnection() {
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
		case *messages.FileResp:
			s := c.getStream(msg.StreamID)
			if s != nil {
				s.in <- msg
			} else {
				c.l.Warn("received FileResp packet for unknown stream",
					zap.String("stream_id", msg.StreamID.String()),
				)

				// cannot remove the stream since it does not exist - only can
				// send a close message
				c.sendClosePacket(msg.StreamID, messages.CloseReasonUndefined)
			}

		case *messages.Data:
			s := c.getStream(msg.StreamID)
			if s != nil {
				s.in <- msg
			} else {
				c.l.Warn("received Data packet for unknown stream",
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

		case *messages.MetaResp:
			// TODO: handle the packet and display it somehow
			c.printMetaResponse(msg)

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

			// close whole connection since we don't know how much bytes to advance
			closeConn(fmt.Sprintf("[%d]", h.MessageType), errors.New("unknown message type"))
			return
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
	defer s.l.Info("closed stream")

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
		c.CloseStream(s, false, 0)
		return
	}

	// make sure its a FileResponse
	var resp *messages.FileResp
	switch v := msg.(type) {
	case *messages.FileResp:
		resp = v
	case *messages.Close:
		s.l.Warn("received Close packet",
			zap.String("reason", v.Reason.String()),
		)
		c.CloseStream(s, false, 0)
		return
	default:
		s.l.Error("unexpected message type",
			zap.String("expected_message_type", "FileResponse"),
			zap.String("actual_message", spew.Sdump("\n", v)),
		)
		c.CloseStream(s, true, messages.CloseReasonUndefined)
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
		c.CloseStream(s, true, messages.CloseReasonChecksumInvalid)
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
			c.CloseStream(s, true, messages.CloseReasonUnexpectedOptionalHeader)
			return

		case *messages.UnknownOptionalHeader:
			c.l.Error("got an unkown optional header type",
				zap.String("dump", spew.Sdump("\n", opt)),
			)

			// close the stream, but keep the connection open
			c.CloseStream(s, true, messages.CloseReasonUnsupportedOptionalHeader)
			return

		default:
			c.l.Error("unexpected optional header type [implementation error]",
				zap.String("dump", spew.Sdump("\n", opt)),
			)

			// close the stream, but keep the connection open
			c.CloseStream(s, true, messages.CloseReasonUnsupportedOptionalHeader)
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
		c.CloseStream(s, true, messages.CloseReasonUndefined) // there's only a reason for files that are too big
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
		c.CloseStream(s, true, messages.CloseReasonUndefined)
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
		c.CloseStream(s, false, 0)
		return
	}

	// FIXME: Add decompression
	// TODO: Start waiting for Data packets
	for {
		var msg messages.BRFTMessage

		select {
		case msg = <-s.in:
		case <-c.close:
			c.CloseStream(s, false, 0)
			return
		}

		switch m := msg.(type) {
		case *messages.Data:
			// (optionally) decompress the data
			data := m.Data
			if s.comp != nil {
				data, err = s.comp.Compress(data)
				if err != nil {
					s.l.Error("unable to decompress chunk",
						zap.String("data", spew.Sdump("\n", m.Data)),
						zap.Error(err),
					)

					c.CloseStream(s, true, messages.CloseReasonUndefined)
					return
				}
			}

			n, err := s.f.Write(data)
			if err != nil {
				s.l.Error("unable to write to file",
					zap.String("data", spew.Sdump("\n", m.Data)),
					zap.Error(err),
				)

				c.CloseStream(s, true, messages.CloseReasonUndefined)
				return
			} else if n != len(data) {
				s.l.Error("partial file write",
					zap.Int("expected", len(data)),
					zap.Int("actual", n),
				)

				c.CloseStream(s, true, messages.CloseReasonUndefined)
				return
			}

		case *messages.Close:
			// clean up the stream after finishing the file
			// no need to send a close packet since the peer already sent us one
			defer c.CloseStream(s, false, 0)

			if m.Reason == messages.CloseReasonTransferComplete {
				s.l.Debug("finished receiving file")

				// make sure the file matches the initially advertised checksum
				correct, computedChecksum, err := s.f.CheckChecksum()
				if err != nil {
					s.l.Error("unable to validate the checksum of complete file",
						zap.Error(err),
					)
					return
				} else if !correct {
					s.l.Error("invalid checksum of complete file",
						zap.String("expected_checksum", spew.Sdump("\n", s.f.Checksum())),
						zap.String("actual_checksum", spew.Sdump("\n", computedChecksum)),
					)

					// TODO: maybe remove the file
					return
				} else {
					s.l.Debug("downloaded file checksum matches",
						zap.String("checksum", spew.Sdump("\n", computedChecksum)),
					)
				}

				// clean up the file - this will also close the file handle
				err = s.f.StripChecksum()
				if err != nil {
					s.l.Error("unable to clean up file", zap.Error(err))
					return
				}

				s.l.Info("finished file download")
			} else {
				s.l.Warn("received Close packet",
					zap.String("reason", m.Reason.String()),
				)
			}

			return
		default:
			s.l.Error("unexpected message type",
				zap.String("expected_message_type", "FileRequest"),
				zap.String("actual_message", spew.Sdump("\n", m)),
			)

			c.CloseStream(s, true, messages.CloseReasonUndefined)
		}
	}
}

func (c *Conn) printMetaResponse(resp *messages.MetaResp) error {
	// print output of MetaDataResp
	output := "\n____________________________________\n~ MetaDataResponse:\n"
	if len(resp.Items) == 0 {
		output += "~ NO ITEMS FOUND"
	}
	for i, item := range resp.Items {
		output += "~   MetaDataItem " + strconv.Itoa(i) + ":\n"
		output += "~     FILE NAME: " + item.FileName + "\n"
		if item.FileSize != nil {
			output += "~     FILE SIZE: " + strconv.Itoa(int(*item.FileSize)) + "\n"
		}
		if item.Checksum != nil {
			output += "~     CHECKSUM: " + fmt.Sprintf("%x", item.Checksum) + "\n"
		}
	}
	output += "~\n____________________________________\n"

	if c.l.Core().Enabled(zap.InfoLevel) {
		c.l.Info("got metadata", zap.String("metadata", output))
	} else {
		fmt.Println(output)
	}

	return nil
}
