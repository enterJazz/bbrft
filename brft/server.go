package brft

import (
	"bytes"
	"errors"
	"fmt"
	"math/rand"
	"os"

	"github.com/davecgh/go-spew/spew"
	"gitlab.lrz.de/bbrft/brft/common"
	"gitlab.lrz.de/bbrft/brft/compression"
	"gitlab.lrz.de/bbrft/brft/messages"
	"gitlab.lrz.de/bbrft/btp"
	"gitlab.lrz.de/bbrft/cyberbyte"
	"gitlab.lrz.de/bbrft/log"
	"go.uber.org/zap"
)

type Server struct {
	l *zap.Logger

	listener *btp.Listener

	// base path to the directory where the files are located
	basePath string

	numConns int
}

func NewServer(
	l *zap.Logger,
	listener *btp.Listener,
	basePath string,
) *Server {
	return &Server{
		l:        l.With(log.FPeer("brft_server")),
		listener: listener,
		basePath: basePath,
	}
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

		switch h.MessageType {
		case messages.MessageTypeFileReq:
			// handle the file transfer negotiation
			err := c.handleServerTransferNegotiation()
			if err != nil {
				c.l.Error("unable to handle FileRequest - closing connection",
					zap.Error(err),
				)

				errClose := c.Close()
				if errClose != nil {
					c.l.Error("error while closing connection", zap.Error(errClose))
				}
				return
			}
		case messages.MessageTypeStartTransmission:
			// TODO: handle the packet and start transmitting data packets (concurrently?)

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
			// TODO: probably close
		}
	}
}

// handleServerTransferNegotiation handles an incomming FileReq packet and
// sends a FileResponse packet if possible. For this, a new stream is created
// and added to the Conn. Since the client cannot create an association between
// the FileRequest and FileResponse, it relies on the server to send back
// FileResponses in the same order the FileRequests have been received.
// Therefore, the server MUST processes them sequentially!
// The function might close the stream and remove any information about it
// remaining on the Conn if a non-critical error occurs. As such, only critical
// errors that should lead to closing the whole btp.Conn are returned.
func (c *Conn) handleServerTransferNegotiation() error {
	// read the file request
	req := new(messages.FileReq)

	// TODO: probably remove the timeout
	//			- I think we can't because otherwise the connection will forever be idle waiting for remaining bytes
	// TODO: probably create one cyberbyte string for the whole connection
	err := req.Decode(c.l, cyberbyte.NewString(c.conn, cyberbyte.DefaultTimeout))
	if err != nil {
		return fmt.Errorf("unanable to decode FileRequest: %w", err)
	}

	// create a new stream
	id := c.newStreamID()
	l := c.l.With(zap.Uint16("stream_id", uint16(id)))
	stream := stream{
		l:                 l,
		id:                id,
		fileName:          req.FileName,
		requestedChecksum: req.Checksum,
	}

	// create a FileResponse
	resp := &messages.FileResp{
		Status:   messages.FileRespStatusOk,
		StreamID: stream.id,
		Checksum: make([]byte, 32), // empty checksum
	}

	// find the requested file
	stream.f, err = OpenFile(req.FileName, c.basePath)
	if errors.Is(err, os.ErrNotExist) {
		// close the stream, but keep the connection open
		// NOTE: The stream object has not yet been added to c.streams
		// (i.e. no cleanup needed)
		c.CloseStream(nil, messages.CloseReasonFileNotFound)
		return nil
	} else if err != nil {
		// close the stream, but keep the connection open
		// NOTE: The stream object has not yet been added to c.streams
		// (i.e. no cleanup needed)
		c.CloseStream(nil, messages.CloseReasonUndefined)
		return nil
	}
	resp.FileSize = stream.f.Size()
	resp.Checksum = stream.f.Checksum()

	// handle resumption
	if req.Flags.IsSet(messages.FileReqFlagResumption) {
		// we need a checksum for resumption
		if bytes.Compare(req.Checksum, make([]byte, common.ChecksumSize)) == 0 {
			// close the stream, but keep the connection open
			// NOTE: The stream object has not yet been added to c.streams
			// (i.e. no cleanup needed)
			c.CloseStream(nil, messages.CloseReasonResumeNoChecksum)
			return nil
		}

		stream.isResumption = true
		// NOTE offset can only be set after the StartTransmission packet
		// has been received
	}
	// if the checksum is not zero we expect it to be identical to the one
	// of the requested file.
	if bytes.Compare(req.Checksum, make([]byte, common.ChecksumSize)) != 0 &&
		bytes.Compare(req.Checksum, stream.f.Checksum()) != 0 {
		// close the stream, but keep the connection open
		// NOTE: The stream object has not yet been added to c.streams
		// (i.e. no cleanup needed)
		c.CloseStream(nil, messages.CloseReasonChecksumInvalid)
		return nil
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
			stream.comp = compression.NewGzipCompressor(stream.l)
			stream.chunkSize = v.ChunkSize()

			if resp.FileSize < stream.comp.MinFileSize() {
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
				zap.String("dump", spew.Sdump(opt)),
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
				zap.String("dump", spew.Sdump(opt)),
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
				zap.String("dump", spew.Sdump(opt)),
			)
			// actually close the stream since something went wrong on our
			// side, but keep the connection open
			// NOTE: The stream object has not yet been added to c.streams
			// (i.e. no cleanup needed)
			c.CloseStream(nil, messages.CloseReasonUndefined)
			return nil
		}

		if respOpt != nil {
			optHeaders = append(optHeaders, respOpt)
		}
	}

	resp.OptHeaders = optHeaders

	// send the response
	data, err := resp.Encode(l)
	if err != nil {
		c.l.Error("unanable to encode FileResponse",
			zap.String("packet", spew.Sdump(resp)),
			zap.Error(err),
		)
		// close the stream, but keep the connection open
		// NOTE: The stream object has not yet been added to c.streams
		// (i.e. no cleanup needed)
		c.CloseStream(nil, messages.CloseReasonUndefined)
		return nil
	}

	_, err = c.conn.Write(data)
	if err != nil {
		// actually close the whole connection TODO: is that correct?
		return fmt.Errorf("unanable to write FileResponse: %w", err)
	}

	// add the stream to the connection
	c.streamsMu.Lock()
	c.streams[stream.id] = stream
	c.streamsMu.Unlock()

	return nil
}

// newStreamID generates a new unique streamID for the connection
func (c *Conn) newStreamID() messages.StreamID {
	if c.isClient {
		c.l.Warn("only servers should generate new StreamIDs")
	}

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
