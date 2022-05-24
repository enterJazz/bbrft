package congestioncontrol

import (
	"testing"

	"go.uber.org/zap"
)

func TestLockStepAlgorithm_SentMessages(t *testing.T) {
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
			cc := NewLockStepAlgorithm(l)
			cc.SentMessages(tt.args.i)

			if cc.pipe != 1 {
				t.Errorf("SentMessages() got: %d, want: %d", cc.pipe, tt.args.i)
			}
		})
	}
}
