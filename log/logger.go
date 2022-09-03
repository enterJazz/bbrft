package log

import (
	"bytes"

	"go.uber.org/zap"
	"go.uber.org/zap/buffer"
	"go.uber.org/zap/zapcore"
)

func NewDevelopment() (*zap.Logger, error) {
	zap.RegisterEncoder("pretty-console", NewEscapeSeqJSONEncoder)
	conf := zap.NewDevelopmentConfig()
	conf.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	conf.Encoding = "pretty-console"
	return conf.Build()
}

func NewProduction() (*zap.Logger, error) {
	conf := zap.NewProductionConfig()
	conf.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	return conf.Build()
}

type EscapeSeqJSONEncoder struct {
	zapcore.Encoder
}

// constructor needed for RegisterEncoder
func NewEscapeSeqJSONEncoder(encoderConfig zapcore.EncoderConfig) (zapcore.Encoder, error) {
	return &EscapeSeqJSONEncoder{
		Encoder: zapcore.NewConsoleEncoder(encoderConfig),
	}, nil
}

func (enc *EscapeSeqJSONEncoder) Clone() zapcore.Encoder {
	return &EscapeSeqJSONEncoder{
		Encoder: enc.Encoder.Clone(),
	}
}

func (enc *EscapeSeqJSONEncoder) EncodeEntry(entry zapcore.Entry, fields []zapcore.Field) (*buffer.Buffer, error) {
	// call EncodeEntry on the embedded interface to get the
	// original output
	b, err := enc.Encoder.EncodeEntry(entry, fields)
	if err != nil {
		return nil, err
	}
	newb := buffer.NewPool().Get()

	// then manipulate that output into what you need it to be
	newb.Write(bytes.Replace(b.Bytes(), []byte("\\n"), []byte("\n"), -1))
	return newb, nil
}
