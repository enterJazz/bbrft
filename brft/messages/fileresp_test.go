package messages

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"gitlab.lrz.de/bbrft/brft/common"
	"gitlab.lrz.de/bbrft/cyberbyte"
	"go.uber.org/zap"
)

// TODO: Write test for marshal and unmarshal individually

func TestFileRespMarshalUnmarshal(t *testing.T) {
	l, err := zap.NewDevelopment()
	if err != nil {
		t.Fatalf("unable to initialize logger")
	}

	tests := []struct {
		name string
		m    FileResp
		//want    []byte
		wantErrMarshal   bool
		wantErrUnmarshal bool
	}{
		// TODO: Add more test cases.
		{"valid-1", FileResp{
			Status:     FileRespStatusOk,
			OptHeaders: OptionalHeaders{},
			StreamID:   StreamID(12345),
			FileSize:   uint64(1234567890),
			Checksum:   make([]byte, common.ChecksumSize),
		}, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.m.Marshal(l)
			if (err != nil) != tt.wantErrMarshal {
				t.Errorf("FileResp.Marshal() error = %v, wantErr %v", err, tt.wantErrMarshal)
				return
			}

			fmt.Printf("message: %s\n", spew.Sdump(got))

			// read again
			m := new(FileResp)
			r := bytes.NewReader(got)
			s := cyberbyte.NewString(r, cyberbyte.DefaultTimeout)

			err = m.Read(l, s)
			if (err != nil) != tt.wantErrMarshal {
				t.Fatalf("FileResp.Marshal() error = %v, wantErr %v", err, tt.wantErrMarshal)
			}

			// compare the input and output
			if !reflect.DeepEqual(*m, tt.m) {
				t.Errorf("Marshal-Unmarshal = %v, want %v\n%s", *m, tt.m, spew.Sdump(*m, tt.m))

			}
		})
	}
}
