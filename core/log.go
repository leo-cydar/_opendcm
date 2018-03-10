package core

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func normaliseWriters(writers ...zapcore.WriteSyncer) zapcore.WriteSyncer {
	var writer zapcore.WriteSyncer
	if len(writers) == 1 {
		writer = writers[0]
	} else {
		writer = zapcore.NewMultiWriteSyncer(writers...)
	}
	return writer
}

// NewJSONLogger creates a `zap.SugaredLogger` configured for JSON output to `writers`
func NewJSONLogger(writers ...zapcore.WriteSyncer) *zap.SugaredLogger {
	writer := normaliseWriters(writers...)
	encoderCfg := zapcore.EncoderConfig{
		MessageKey:     "msg",
		LevelKey:       "level",
		NameKey:        "logger",
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
	}
	core := zapcore.NewCore(zapcore.NewJSONEncoder(encoderCfg), writer, zapcore.DebugLevel)
	return zap.New(core).Sugar()
}

// NewConsoleLogger creates a `zap.SugaredLogger` configured for human-readable output to `writers`
func NewConsoleLogger(writers ...zapcore.WriteSyncer) *zap.SugaredLogger {
	writer := normaliseWriters(writers...)
	encoderCfg := zapcore.EncoderConfig{
		MessageKey:     "msg",
		LevelKey:       "level",
		NameKey:        "logger",
		EncodeLevel:    zapcore.LowercaseColorLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
	}
	core := zapcore.NewCore(zapcore.NewConsoleEncoder(encoderCfg), writer, zapcore.DebugLevel)
	return zap.New(core).Sugar()
}
