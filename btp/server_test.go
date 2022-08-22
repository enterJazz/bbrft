package btp

import (
	"net"
	"testing"

	"go.uber.org/zap"
)

func TestServer_Listen(t *testing.T) {
	type fields struct {
		l       *zap.Logger
		options *ServerOptions
		conn    *net.UDPConn
		conns   map[string]Connection
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Server{
				l:       tt.fields.l,
				options: tt.fields.options,
				conn:    tt.fields.conn,
				conns:   tt.fields.conns,
			}
			if err := s.Listen(); (err != nil) != tt.wantErr {
				t.Errorf("Server.Listen() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
