package brft

import (
	"bytes"
	"fmt"
	"net"

	"github.com/davecgh/go-spew/spew"
	"gitlab.lrz.de/bbrft/brft/common"
	"gitlab.lrz.de/bbrft/brft/compression"
	"gitlab.lrz.de/bbrft/brft/messages"
	"gitlab.lrz.de/bbrft/btp"
	"gitlab.lrz.de/bbrft/cyberbyte"
	"gitlab.lrz.de/bbrft/log"
	"go.uber.org/zap"
)

// TODO: move
const (
	serverAddrStr = "127.0.0.1:1337"
	compressionMultiplier
)

func Dial(
	l *zap.Logger,
	addr string, // server address
	downloadDir string,
) (*Conn, error) {
	c := &Conn{
		l:        l.With(log.FPeer("brft_client")), // extend logger
		basePath: downloadDir,                      // TODO: Make sure that it actually exists / create it
		isClient: true,
	}

	raddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("unable to resolve server address: %w", err)
	}

	// TODO: Figure out options
	c.conn, err = btp.Dial(btp.ConnOptions{}, nil, raddr, l)
	if err != nil {
		return nil, fmt.Errorf("unable to create connection: %w", err)
	}

	// TODO: Add a loop that handles incomming files
	go c.handleClientConnection()

	return c, nil
}

func (c *Conn) DownloadFile(
	fileName string,
	withCompression bool, // TODO: Replace by some options on the client
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
	req := &messages.FileReq{
		// OptionalHeaders for the upcomming file transfer
		OptHeaders: make(messages.OptionalHeaders, 0, 1),
		// FileName of the requested file, can be at most 255 characters long
		FileName: fileName,
		// Checksum is the checksum of a previous partial download or if a specific
		// file version shall be requested. Might be unitilized or zeroed.
		Checksum: make([]byte, common.ChecksumSize),
	}

	s := stream{
		l:                 c.l.With(zap.String("file_name", fileName)),
		fileName:          fileName,
		requestedChecksum: req.Checksum,
		//isResumption: , TODO: set
		//offset: , TODO: set
	}

	// handle compression
	if withCompression {
		h := messages.NewCompressionReqOptionalHeader(
			messages.CompressionReqHeaderAlgorithmGzip,
			0, // -> 64kB
		)
		req.OptHeaders = append(req.OptHeaders, h)
		s.chunkSize = h.ChunkSize()
		// NOTE: compressor must be set once the server confirms compression
	}

	data, err := req.Encode(c.l)
	if err != nil {
		return fmt.Errorf("unable to encode FileRequest: %w", err)
	}

	// We need to ensure that all FileRequests are sent to the server in the
	// same order as they are added to the reqStreams slice in order to be able
	// to create an association between FileReq and FileResp
	c.reqStreamsMu.Lock()
	_, err = c.conn.Write(data)
	if err != nil {
		return fmt.Errorf("unable to encode FileRequest: %w", err)
	}

	// add the stream to the preliminary streams waiting for a server response
	c.reqStreams = append(c.reqStreams, s)
	c.reqStreamsMu.Unlock()

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

		switch h.MessageType {
		case messages.MessageTypeFileResp:
			// handle the file transfer negotiation
			err := c.handleClientTransferNegotiation()
			if err != nil {
				c.l.Error("unable to handle FileResponse - closing connection",
					zap.Error(err),
				)

				errClose := c.Close()
				if errClose != nil {
					c.l.Error("error while closing connection", zap.Error(errClose))
				}
				return
			}
		case messages.MessageTypeData:
			// TODO: handle the packet and save it to the correct file

		case messages.MessageTypeClose:
			// TODO: close the stream, but not the connection

		case messages.MessageTypeMetaDataResp:
			// TODO: handle the packet and display it somehow

		case messages.MessageTypeFileReq,
			messages.MessageTypeStartTransmission,
			messages.MessageTypeMetaDataReq:
			c.l.Error("unexpected message type",
				zap.Uint8("type_encoding", uint8(h.MessageType)),
				zap.String("type", h.MessageType.String()),
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

// handleClientTransferNegotiation handles an incomming FileResp packet and
// sends a StartTransmission packet if possible. It uses the stream saved in
// c.reqStreams[0]. Because of this is has to be made sure that the streams are
// added to c.reqStreams in the same order that they are sent to the server and
// that the server processes them sequentially as well!
// The function might close the stream and remove any information about it
// remaining on the Conn if a non-critical error occurs. As such, only critical
// errors that should lead to closing the whole btp.Conn are returned.
func (c *Conn) handleClientTransferNegotiation() error {

	// read the file request
	resp := new(messages.FileResp)

	// TODO: probably remove the timeout
	//			- I think we can't because otherwise the connection will forever be idle waiting for remaining bytes
	// TODO: probably create one cyberbyte string for the whole connection
	err := resp.Decode(c.l, cyberbyte.NewString(c.conn, cyberbyte.DefaultTimeout))
	if err != nil {
		// actually close the whole connection TODO: is that correct?
		return fmt.Errorf("unanable to decode FileRequest: %w", err)
	}

	// get the next stream without a response yet
	c.reqStreamsMu.Lock()
	if len(c.reqStreams) < 1 {
		c.l.Error("no file request found for FileResponse message",
			zap.String("requested_streams", spew.Sdump(c.reqStreams)),
		)

		// close the stream, but keep the connection open
		// NOTE: The stream object has been removed from c.reqStreams and
		// not yet added to c.streams (i.e. no cleanup needed)
		c.CloseStream(nil, messages.CloseReasonUndefined)
		return nil
	}
	s := c.reqStreams[0]
	// remove the stream from the requested ones
	c.reqStreams = c.reqStreams[1:]
	c.reqStreamsMu.Unlock()

	// update the stream
	s.id = resp.StreamID
	s.l = s.l.With(
		zap.Uint16("stream_id", uint16(s.id)),
		zap.String("remote_addr", c.conn.LocalAddr().String()),
		zap.String("local_addr", c.conn.RemoteAddr().String()),
		zap.Bool("client_conn", true),
	)

	// set & check the checksum
	if bytes.Compare(s.requestedChecksum, make([]byte, common.ChecksumSize)) != 0 &&
		bytes.Compare(s.requestedChecksum, s.f.Checksum()) != 0 {
		// TODO: potentially add a dialogue to allow the resumption of the download
		s.l.Error("checksums do not match",
			zap.String("requestedChecksum", spew.Sdump(s.requestedChecksum)),
			zap.String("checksum", spew.Sdump(s.f.Checksum())),
		)

		// close the stream, but keep the connection open
		// NOTE: The stream object has been removed from c.reqStreams and
		// not yet added to c.streams (i.e. no cleanup needed)
		c.CloseStream(nil, messages.CloseReasonChecksumInvalid)
		return nil
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
				zap.String("dump", spew.Sdump(opt)),
			)

			// close the stream, but keep the connection open
			// NOTE: The stream object has been removed from c.reqStreams and
			// not yet added to c.streams (i.e. no cleanup needed)
			c.CloseStream(nil, messages.CloseReasonUnexpectedOptionalHeader)
			return nil

		case *messages.UnknownOptionalHeader:
			c.l.Error("got an unkown optional header type",
				zap.String("dump", spew.Sdump(opt)),
			)

			// close the stream, but keep the connection open
			// NOTE: The stream object has been removed from c.reqStreams and
			// not yet added to c.streams (i.e. no cleanup needed)
			c.CloseStream(nil, messages.CloseReasonUnsupportedOptionalHeader)
			return nil

		default:
			c.l.Error("unexpected optional header type [implementation error]",
				zap.String("dump", spew.Sdump(opt)),
			)

			// close the stream, but keep the connection open
			// NOTE: The stream object has been removed from c.reqStreams and
			// not yet added to c.streams (i.e. no cleanup needed)
			c.CloseStream(nil, messages.CloseReasonUnsupportedOptionalHeader)
			return nil
		}
	}

	// TODO: Create the file - is there a way to allocate the memory?
	s.f, err = NewFile(s.l, s.fileName, c.basePath, resp.Checksum)
	if err != nil {
		c.l.Error("unable to initialize file",
			zap.String("base_path", c.basePath),
			zap.String("s.checksum", spew.Sdump(resp.Checksum)),
			zap.Error(err),
		)
		// close the stream, but keep the connection open
		// NOTE: The stream object has been removed from c.reqStreams and
		// not yet added to c.streams (i.e. no cleanup needed)
		c.CloseStream(nil, messages.CloseReasonUndefined) // there's only a reason for files that are too big
		return fmt.Errorf("unable to initialize file: %w", err)
	}

	st := messages.StartTransmission{
		StreamID: uint16(resp.StreamID),
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
		c.l.Error("unanable to encode StartTransmission",
			zap.String("packet", spew.Sdump(st)),
			zap.Error(err),
		)
		// close the stream, but keep the connection open
		// NOTE: The stream object has been removed from c.reqStreams and
		// not yet added to c.streams (i.e. no cleanup needed)
		c.CloseStream(nil, messages.CloseReasonUndefined)
		return nil
	}

	_, err = c.conn.Write(data)
	if err != nil {
		// actually close the whole connection TODO: is that correct?
		return fmt.Errorf("unanable to write StartTransmission: %w", err)
	}

	// add the updated stream to the connection
	c.streamsMu.Lock()
	c.streams[s.id] = s
	c.streamsMu.Unlock()

	return nil
}
