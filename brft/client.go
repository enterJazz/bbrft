package brft

import (
	"fmt"
	"net"
	"time"

	"gitlab.lrz.de/bbrft/brft/common"
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
		l:            l.With(log.FPeer("brft_client")), // extend logger
		baseFilePath: downloadDir,                      // TODO: Make sure that it actually exists / create it
		isClient:     true,
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

	// create a new request
	req := &messages.FileReq{
		// OptionalHeaders for the upcomming file transfer
		OptHeaders: messages.OptionalHeaders{
			messages.NewCompressionReqOptionalHeader(
				messages.CompressionReqHeaderAlgorithmGzip,
				0, // -> 64kB (chunk size = (chunk size multiplier + 1) ∗ 64kB)
			),
		},
		// FileName of the requested file, can be at most 255 characters long
		FileName: fileName,
		// Checksum is the checksum of a previous partial download or if a specific
		// file version shall be requested. Might be unitilized or zeroed.
		Checksum: make([]byte, common.ChecksumSize),
	}

	data, err := req.Encode(c.l)
	if err != nil {
		return fmt.Errorf("unable to encode FileRequest: %w", err)
	}

	_, err = c.conn.Write(data)
	if err != nil {
		return fmt.Errorf("unable to encode FileRequest: %w", err)
	}

	// add a new stream to the preliminary streams waiting for a server response
	c.reqStreamsMu.Lock()
	c.reqStreams[fileName] = stream{
		l: c.l.With(zap.String("file_name", fileName)),
	}
	c.reqStreamsMu.Unlock()

	// clean up the stream corresponding to the requested file after a set
	// amount of time
	go func() {
		time.Sleep(time.Minute)
		c.reqStreamsMu.Lock()
		defer c.reqStreamsMu.Unlock()
		if s, ok := c.reqStreams[fileName]; ok {
			s.l.Warn("cleaning up unanswered FileRequest")
			delete(c.reqStreams, fileName)
		}
	}()

	return nil
}

func (c *Conn) handleClientConnection() {
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
		case messages.MessageTypeFileResp:
			// handle the file transfer negotiation
			err := c.handleClientTransferNegotiation()
			// TODO: handle - probably differentiate between different errors
			if err != nil {
			}
		case messages.MessageTypeStartTransmission:
		case messages.MessageTypeClose:
		case messages.MessageTypeMetaDataReq:
		case messages.MessageTypeFileReq,
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

func (c *Conn) handleClientTransferNegotiation() error {
	// read the file request
	resp := new(messages.FileReq)

	// TODO: probably remove the timeout
	//			- I think we can't because otherwise the connection will forever be idle waiting for remaining bytes
	// TODO: probably create one cyberbyte string for the whole connection
	err := resp.Decode(c.l, cyberbyte.NewString(c.conn, cyberbyte.DefaultTimeout))
	if err != nil {
		return fmt.Errorf("unanable to decode FileRequest: %w", err)
	}

	// FIXME: Implement me
	return nil
}
