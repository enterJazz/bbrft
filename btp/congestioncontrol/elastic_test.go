package congestioncontrol

import (
	"testing"

	"go.uber.org/zap"
)

const TestInitCwndSize = 10
const MaxCwndSize = 20

func TestElasticAlgorithm(t *testing.T) {
	l, err := zap.NewDevelopment()
	if err != nil {
		t.Fatal("unable to initialize logger")
	}

	type args struct {
		i int
	}
	tests := []struct {
		name string
		args args
		want int
	}{
		{"sent-1", args{1}, 1},
		{"sent-2", args{1}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cc := NewElasticTcpAlgorithm(l, TestInitCwndSize, MaxCwndSize)
			cc.SentMessages(tt.args.i)

			if cc.pipe != tt.args.i {
				t.Errorf("SentMessages() got: %d, want: %d", cc.pipe, tt.args.i)
			}

			if cc.NumFreeSend() != (TestInitCwndSize - tt.args.i) {
				t.Errorf("NumFreeSend() got %d, want %d", cc.NumFreeSend(), TestInitCwndSize-tt.args.i)
			}

			cc.rtt_max = 10
			cc.rtt_curr = 1
			cc.CongAvoid()

			if cc.cwnd <= TestInitCwndSize {
				t.Errorf("Expected cc.cwnd > TestInitCwndSize; Got: %d !> %d", cc.cwnd, TestInitCwndSize)
			}

			cc.ReceivedAcks(tt.args.i)

			if cc.pipe != 0 {
				t.Errorf("Expected cc.pipe == 0; Got: %d != 0", cc.pipe)
			}

			// check for maxCwnd
			cc.rtt_max = 100
			cc.rtt_curr = 1
			for a := 0; a < 4; a++ {
				cc.CongAvoid()
			}

			if cc.cwnd != MaxCwndSize {
				t.Errorf("Expected cc.cwnd == MaxCwndSize; Got: %d != %d", cc.cwnd, MaxCwndSize)
			}

		})
	}
}
