package logger

import (
	"context"
	"fmt"
)

type logConfigKey int

const (
	logLevel logConfigKey = iota
)

const (
	logLevelDebug = iota
	logLevelInfo  = iota
	logLevelWarn  = iota
	logLevelError = iota

	defaultLogLevel = logLevelInfo
)

type Logger interface {
	Error(ctx context.Context, format string, a ...any)
	Warn(ctx context.Context, format string, a ...any)
	Info(ctx context.Context, format string, a ...any)
	Debug(ctx context.Context, format string, a ...any)
}

type logger struct{}

func New() Logger {
	return &logger{}
}

func (l *logger) Error(ctx context.Context, format string, a ...any) {
	if getLogLevel(ctx) <= logLevelError {
		fmt.Println("ERROR: " + fmt.Sprintf(format, a...))
	}
}

func (l logger) Warn(ctx context.Context, format string, a ...any) {
	if getLogLevel(ctx) <= logLevelWarn {
		fmt.Println("WARN: " + fmt.Sprintf(format, a...))
	}
}

func (l logger) Info(ctx context.Context, format string, a ...any) {
	if getLogLevel(ctx) <= logLevelInfo {
		fmt.Println("INFO: " + fmt.Sprintf(format, a...))
	}
}

func (l logger) Debug(ctx context.Context, format string, a ...any) {
	if getLogLevel(ctx) <= logLevelDebug {
		fmt.Println("DEBUG: " + fmt.Sprintf(format, a...))
	}
}

func getLogLevel(ctx context.Context) int {
	if level, ok := ctx.Value(logLevel).(int); ok {
		return level
	}
	return defaultLogLevel
}
