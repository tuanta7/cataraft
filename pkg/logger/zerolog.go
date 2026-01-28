package logger

import (
	"os"

	"github.com/rs/zerolog"
)

type Logger struct {
	*zerolog.Logger
}

func NewLogger(level string) *Logger {
	zl := zerolog.New(os.Stdout)

	zl = zl.With().Timestamp().Logger()
	zl.Level(zerolog.DebugLevel)

	return &Logger{
		Logger: &zl,
	}
}
