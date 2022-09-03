package server

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
	"go.uber.org/zap"
)

// create a new connection object
type stream struct {
	l *zap.Logger

	id   messages.StreamID
	f    *File
	comp compression.Compressor
}

type Conn struct {
	l *zap.Logger

	*btp.Conn

	// base path to the directory where the files are located
	baseFilePath string

	streams map[messages.StreamID]stream
}

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

var (
	ErrReadLen = errors.New("unexpected read length")
)

func NewServer(
	l *zap.Logger,
	listener *btp.Listener,
	baseFilePath string,
) *Server {
	return &Server{
		l:            l.With(zap.String("instance", "brft_server")),
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

		// handle the connection
		go s.handleConnection(conn, id)
	}
}

// FIXME: Figure out how to implement a gracefull shutdown when the BTP layer
// handles timeouts
func (s *Server) handleConnection(conn *btp.Conn, id int) {

	// extend logging
	l := s.l.With(
		zap.Int("id", id),
		zap.String("remote_addr", conn.LocalAddr().String()),
		zap.String("local_addr", conn.RemoteAddr().String()),
	)

	l.Info("accepted new connection")

	c := &Conn{
		l:            l,
		Conn:         conn,
		baseFilePath: s.baseFilePath,
	}

	// wait for incomming messages
	for {
		// read the message header
		b := make([]byte, 1)
		n, err := c.Read(b)
		if err != nil {
			c.l.Error("unable to read message header - closing", zap.Error(err))
			err := c.Close()
			if err != nil {
				c.l.Error("unable to close connection", zap.Error(err))
				return
			}
		} else if n != 1 {
			c.l.Error("unable to close connection", zap.Error(ErrReadLen))
			err := c.Close()
			if err != nil {
				c.l.Error("unable to close connection", zap.Error(err))
				return
			}
		}

		// determine the message version and type
		h := messages.NewPacketHeader(b[0])
		if !h.Version.Valid() {
			c.l.Error("unsupported header protocol version")
			err := c.Close()
			if err != nil {
				c.l.Error("unable to close connection", zap.Error(err))
				return
			}
		}

		switch h.MessageType {
		case messages.MessageTypeFileReq:
			// handle the file transfer negotiation
			err := c.handleTransferNegotiation()
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

func (c *Conn) handleTransferNegotiation() error {
	// read the file request
	req := new(messages.FileReq)

	// TODO: probably remove the timeout
	//			- I think we can't because otherwise the connection will forever be idle waiting for remaining bytes
	// TODO: probably create one cyberbyte string for the whole connection
	err := req.Read(c.l, cyberbyte.NewString(c.Conn, cyberbyte.DefaultTimeout))
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
		stream.f, err = NewFile(req.FileName, c.baseFilePath)
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
	data, err := resp.Marshal(l)
	if err != nil {
		return fmt.Errorf("unanable to encode FileResponse: %w", err)
	}

	n, err := c.Conn.Write(data)
	if err != nil {
		return fmt.Errorf("unanable to write FileResponse: %w", err)
	} else if n != len(data) {
		return fmt.Errorf("short write on FileResponse: %w", err)
	}

	// add the stream to the connection
	c.streams[stream.id] = stream

	return nil
}

// TODO: Add reasons why the connection is closed
func (c *Conn) Close() error {

	// TODO: Send a close message
	// close the btp.Conn // FIXME: Probably don't want to close the btp conn all the time - we might have multiple transfers after all
	return c.Conn.Close()
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
