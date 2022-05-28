package messages

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"
)

// TODO: Write test for marshal and unmarshal individually

func TestMarshalUnmarshal(t *testing.T) {
	tests := []struct {
		name string
		m    FileReq
		//want    []byte
		wantErrMarshal   bool
		wantErrUnmarshal bool
	}{
		// TODO: Add test cases.
		{"valid-1", FileReq{
			Filename: "some-filename",
			Flags:    nil,
			Checksum: nil,
		}, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.m.Marshal()
			if (err != nil) != tt.wantErrMarshal {
				t.Errorf("FileReq.Marshal() error = %v, wantErr %v", err, tt.wantErrMarshal)
				return
			}

			fmt.Printf("message: %X\n", got)

			// receiver stuff
			m := new(FileReq)
			r := bytes.NewReader(got)
			len, err := m.GetLength(r)
			if err != nil {
				t.Errorf("FileReq.GetLength() error = %v", err)
				return
			}
			data := make([]byte, len)
			n, err := r.Read(data)
			if err != nil {
				t.Errorf("r.Read() error = %v, n = %d", err, n)
				return
			} else if n < len {
				t.Errorf("r.Read() short read: n = %d", n)
				return
			}

			fmt.Printf("message after GetLength: %X\n", data)

			err = m.Unmarshal(data)
			if (err != nil) != tt.wantErrMarshal {
				t.Errorf("FileReq.Marshal() error = %v, wantErr %v", err, tt.wantErrMarshal)
				return
			}

			// Strip the length
			tt.m.Raw = tt.m.Raw[2:]

			// compare the input and output
			if !reflect.DeepEqual(*m, tt.m) {
				t.Errorf("Marshal-Unmarshal = %v, want %v", *m, tt.m)
			}
		})
	}
}
