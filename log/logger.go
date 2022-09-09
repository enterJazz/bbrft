package log

import (
	"bytes"

	"go.uber.org/zap"
	"go.uber.org/zap/buffer"
	"go.uber.org/zap/zapcore"
)

type Options struct {
	color         bool
	prettyNewLine bool
	prod          bool
	client        bool
}

func NewOptions() *Options {
	return &Options{
		color:         true,
		prettyNewLine: true,
		prod:          false,
		client:        false,
	}
}

type Option func(o *Options)

func WithColor(v bool) Option {
	return func(o *Options) {
		o.color = v
	}
}

func WithProd(v bool) Option {
	return func(o *Options) {
		o.prod = v
	}
}

func WithClient(v bool) Option {
	return func(o *Options) {
		o.client = v
	}
}

// TODO: Add option for log level

func NewLogger(opts ...Option) (*zap.Logger, error) {
	o := NewOptions()
	for _, opt := range opts {
		opt(o)
	}

	conf := zap.NewDevelopmentConfig()
	if o.prod {
		conf = zap.NewProductionConfig()
	}

	// if production client make logs human readable
	if o.client {
		conf.Encoding = "console"
		conf.EncoderConfig.EncodeTime = zapcore.TimeEncoderOfLayout("15:04:05.000000")
		conf.EncoderConfig.EncodeDuration = zapcore.StringDurationEncoder
	}

	if o.color {
		conf.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	if o.prettyNewLine {
		zap.RegisterEncoder("pretty-console", NewEscapeSeqJSONEncoder)
		conf.Encoding = "pretty-console"
	}

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
