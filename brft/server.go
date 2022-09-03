package brft

import (
	"errors"
	"fmt"
	"math/rand"
	"os"

	"github.com/davecgh/go-spew/spew"
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
	baseFilePath string

	// compressor used for possible compression of the transferred data
	compressor compression.Compressor

	// initialized during the handshake
	chunkSize int

	numConns int
}

func NewServer(
	l *zap.Logger,
	listener *btp.Listener,
	baseFilePath string,
) *Server {
	return &Server{
		l:            l.With(log.FPeer("brft_server")),
		listener:     listener,
		baseFilePath: baseFilePath,
		chunkSize:    -1,
	}
}

func (s *Server) Close() error {
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
			l:            l,
			conn:         conn,
			baseFilePath: s.baseFilePath,
			isClient:     false,
		}

		// handle the connection
		go c.handleServerConnection()
	}
}

// FIXME: Figure out how to implement a gracefull shutdown when the BTP layer
// handles timeouts
func (c *Conn) handleServerConnection() {

	// wait for incomming messages
	for {
		h, err := c.readHeader()
		if err != nil {
			// errors already handled by function
			// TODO: maybe only return if we cannot read, not if the header is
			// unknown - however the question is how we know how long the
			// message is in order to advance over it
			return
		}

		switch h.MessageType {
		case messages.MessageTypeFileReq:
			// handle the file transfer negotiation
			err := c.handleServerTransferNegotiation()
			// TODO: handle - probably differentiate between different errors
			if err != nil {
			}
		case messages.MessageTypeStartTransmission:
		case messages.MessageTypeClose:
		case messages.MessageTypeMetaDataReq:
		case messages.MessageTypeFileResp,
			messages.MessageTypeData,
			messages.MessageTypeMetaDataResp:
			c.l.Error("unexpected message type",
				zap.Uint8("type_encoding", uint8(h.MessageType)),
				zap.String("type", h.MessageType.String()),
			)
			// TODO: Probably close

		default:
			c.l.Error("unknown message type",
				zap.Uint8("type_encoding", uint8(h.MessageType)),
			)
			// TODO: Probably close
		}
	}
}

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
		l:  l,
		id: id,
	}

	// create a FileResponse
	resp, err := func() (*messages.FileResp, error) {
		resp := &messages.FileResp{
			Status:   messages.FileRespStatusOk,
			StreamID: stream.id,
			Checksum: make([]byte, 32), // empty checksum
		}

		// find the requested file
		stream.f, err = OpenFile(req.FileName, c.baseFilePath)
		if errors.Is(err, os.ErrNotExist) {
			// TODO: Need to send a Close message with the appropraite reason set - maybe directly from here?!
			return nil, errors.New("file not found")
		} else if err != nil {
			// TODO: How should we handle this? internal error?!
			return nil, errors.New("unable to get file")
		}
		resp.FileSize = stream.f.Size()
		resp.Checksum = stream.f.Checksum()

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
				// adjust the status
				if messages.FileRespStatusUnexpectedOptionalHeader.HasPrecedence(resp.Status) {
					resp.Status = messages.FileRespStatusUnexpectedOptionalHeader
				}
			case *messages.UnknownOptionalHeader:
				// adjust the status
				if messages.FileRespStatusUnsupportedOptionalHeader.HasPrecedence(resp.Status) {
					resp.Status = messages.FileRespStatusUnsupportedOptionalHeader
				}
			default:
				c.l.Error("unexpected optional header type [implementation error]",
					zap.String("dump", spew.Sdump(opt)),
				)
				return nil, errors.New("unexpected optional header type [implementation error]")
			}

			if respOpt != nil {
				optHeaders = append(optHeaders, respOpt)
			}
		}

		return resp, nil
	}()

	if err != nil {
		return err
	}

	// send the response
	data, err := resp.Encode(l)
	if err != nil {
		return fmt.Errorf("unanable to encode FileResponse: %w", err)
	}

	_, err = c.conn.Write(data)
	if err != nil {
		return fmt.Errorf("unanable to write FileResponse: %w", err)
	}

	// add the stream to the connection
	c.streams[stream.id] = stream

	return nil
}

// TODO: Add reasons why the connection is closed
func (c *Conn) Close() error {

	// TODO: Send a close message
	// close the btp.Conn // FIXME: Probably don't want to close the btp conn all the time - we might have multiple transfers after all
	return c.conn.Close()
}

// newStreamID generates a new unique streamID for the connection
func (c *Conn) newStreamID() messages.StreamID {
	exists := false
	for {
		newId := messages.StreamID(rand.Uint32())
		for id, _ := range c.streams {
			if newId == id {
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
