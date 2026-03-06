package log

import (
	"context"
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"
)

func New(level zerolog.Level) zerolog.Logger {
	var output io.Writer
	if os.Getenv("LOCAL_DEV") == "true" {
		output = zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
		}
	} else {
		output = os.Stdout
	}

	return zerolog.New(output).
		Level(level).
		With().
		Timestamp().
		Logger()
}
func Setup() zerolog.Level {
	level := zerolog.InfoLevel
	switch os.Getenv("LOG_LEVEL") {
	case "debug":
		level = zerolog.DebugLevel
	case "error":
		level = zerolog.ErrorLevel
	case "disabled":
		level = zerolog.Disabled
	}
	return level
}

type ctxKey struct{}

func WithLogger(ctx context.Context, l zerolog.Logger) context.Context {
	return context.WithValue(ctx, ctxKey{}, l)
}

func FromContext(ctx context.Context) zerolog.Logger {
	if l, ok := ctx.Value(ctxKey{}).(zerolog.Logger); ok {
		return l
	}
	return zerolog.Nop()
}
