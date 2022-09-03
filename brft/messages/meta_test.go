package messages

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"gitlab.lrz.de/bbrft/cyberbyte"
	"go.uber.org/zap"
)

func TestMetaReqMarshalUnmarshal(t *testing.T) {
	l, err := zap.NewDevelopment()
	if err != nil {
		t.Fatalf("unable to initialize logger")
	}

	tests := []struct {
		name             string
		m                MetaReq
		wantErrMarshal   bool
		wantErrUnmarshal bool
	}{
		// TODO: Add test cases.
		{"valid-1", MetaReq{
			FileName: "some-filename",
		}, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.m.Encode(l)
			if (err != nil) != tt.wantErrMarshal {
				t.Errorf("MetaReq.Marshal() error = %v, wantErr %v", err, tt.wantErrMarshal)
				return
			}

			m := &MetaReq{}
			r := bytes.NewReader(got)
			s := cyberbyte.NewString(r, cyberbyte.DefaultTimeout)

			err = m.Decode(l, s)
			if (err != nil) != tt.wantErrMarshal {
				t.Errorf("MetaReq.Unmarshal() error = %v, wantErr %v", err, tt.wantErrMarshal)
				return
			}

			// compare the input and output
			if !reflect.DeepEqual(*m, tt.m) {
				t.Errorf("MetaReq-Unmarshal = %v, want %v", spew.Sdump(*m), spew.Sdump(tt.m))
			}
		})
	}
}

func TestMetaItemMarshalUnmarshal(t *testing.T) {
	size := uint64(1024)

	tests := []struct {
		name             string
		m                MetaItem
		extended         bool
		wantErrMarshal   bool
		wantErrUnmarshal bool
	}{
		// TODO: Add test cases.
		{"valid", MetaItem{
			FileName: "some-filename",
		}, false, false, false},
		{"valid-extended", MetaItem{
			FileName: "some-filename",
			FileSize: &size,
			Checksum: []byte("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"),
		}, true, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.m.Encode()
			if (err != nil) != tt.wantErrMarshal {
				t.Errorf("MetaItem.Marshal() error = %v, wantErr %v", err, tt.wantErrMarshal)
				return
			}

			m := &MetaItem{}
			r := bytes.NewReader(got)
			s := cyberbyte.NewString(r, cyberbyte.DefaultTimeout)

			err = m.Decode(s, tt.extended)
			if (err != nil) != tt.wantErrMarshal {
				t.Errorf("MetaItem.Unmarshal() error = %v, wantErr %v", err, tt.wantErrMarshal)
				return
			}

			// compare the input and output
			if !reflect.DeepEqual(*m, tt.m) {
				t.Errorf("MetaItem-Unmarshal = %v, want %v", spew.Sdump(*m), spew.Sdump(tt.m))
			}
		})
	}
}

func TestMetaRespMarshalUnmarshal(t *testing.T) {
	l, err := zap.NewDevelopment()
	if err != nil {
		t.Fatalf("unable to initialize logger")
	}
	size := uint64(1024)

	tests := []struct {
		name     string
		m        MetaResp
		extended bool

		wantErrMarshal   bool
		wantErrUnmarshal bool
	}{
		// TODO: Add test cases.
		{"valid",
			MetaResp{
				isExtended: false,
				Items: []MetaItem{
					{FileName: "filename-1"},
					{FileName: "filename-2"},
					{FileName: "filename-3"},
					{FileName: "filename-4"},
				},
			},
			false,
			false, false},
		{"valid-extended",
			MetaResp{
				isExtended: true,
				Items: []MetaItem{
					{
						FileName: "filename-1",
						FileSize: &size,
						Checksum: []byte("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"),
					},
					{
						FileName: "filename-2",
						FileSize: &size,
						Checksum: []byte("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"),
					},
					{
						FileName: "filename-3",
						FileSize: &size,
						Checksum: []byte("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"),
					},
					{
						FileName: "filename-4",
						FileSize: &size,
						Checksum: []byte("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"),
					},
				},
			},
			true,
			false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.m.Encode(l)
			if (err != nil) != tt.wantErrMarshal {
				t.Errorf("MetaResp.Marshal() error = %v, wantErr %v", err, tt.wantErrMarshal)
				return
			}

			m := &MetaResp{}
			m.SetExtended(tt.extended)
			r := bytes.NewReader(got)
			s := cyberbyte.NewString(r, cyberbyte.DefaultTimeout)

			err = m.Decode(l, s)
			if (err != nil) != tt.wantErrMarshal {
				t.Errorf("MetaResp.Unmarshal() error = %v, wantErr %v", err, tt.wantErrMarshal)
				return
			}

			// compare the input and output
			if !reflect.DeepEqual(*m, tt.m) {
				t.Errorf("MetaResp-Unmarshal = %v, want %v", spew.Sdump(*m), spew.Sdump(tt.m))
			}
		})
	}
}
