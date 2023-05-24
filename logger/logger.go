package logger

import (
	"context"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type ctxKey struct{}

func FromContext(ctx context.Context) *zap.Logger {
	c, _ := ctx.Value(ctxKey{}).(*zap.Logger)
	return c
}

func NewContext(parent context.Context, z *zap.Logger) context.Context {
	return context.WithValue(parent, ctxKey{}, z)
}

func New(level zapcore.Level, structured bool, options ...zap.Option) (*zap.Logger, error) {
	var cfg zap.Config
	if structured {
		cfg = zap.Config{
			Level:       zap.NewAtomicLevelAt(level),
			Development: false,
			Sampling: &zap.SamplingConfig{
				Initial:    100,
				Thereafter: 100,
			},
			Encoding:         "json",
			EncoderConfig:    zap.NewProductionEncoderConfig(),
			OutputPaths:      []string{"stderr"},
			ErrorOutputPaths: []string{"stderr"},
		}
	} else {
		cfg = zap.Config{
			Level:            zap.NewAtomicLevelAt(level),
			Development:      true,
			Encoding:         "console",
			EncoderConfig:    zap.NewDevelopmentEncoderConfig(),
			OutputPaths:      []string{"stderr"},
			ErrorOutputPaths: []string{"stderr"},
		}
	}

	return cfg.Build(options...)
}
