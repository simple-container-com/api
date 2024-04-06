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
	LogLevelDebug = iota
	LogLevelInfo  = iota
	LogLevelWarn  = iota
	LogLevelError = iota

	defaultLogLevel = LogLevelInfo
)

type Logger interface {
	Error(ctx context.Context, format string, a ...any)
	Warn(ctx context.Context, format string, a ...any)
	Info(ctx context.Context, format string, a ...any)
	Debug(ctx context.Context, format string, a ...any)
	SetLogLevel(ctx context.Context, logLevel int) context.Context
}

type logger struct{}

func New() Logger {
	return &logger{}
}

func (l *logger) Error(ctx context.Context, format string, a ...any) {
	if getLogLevel(ctx) <= LogLevelError {
		fmt.Println("ERROR: " + fmt.Sprintf(format, a...))
	}
}

func (l logger) Warn(ctx context.Context, format string, a ...any) {
	if getLogLevel(ctx) <= LogLevelWarn {
		fmt.Println("WARN: " + fmt.Sprintf(format, a...))
	}
}

func (l logger) Info(ctx context.Context, format string, a ...any) {
	if getLogLevel(ctx) <= LogLevelInfo {
		fmt.Println("INFO: " + fmt.Sprintf(format, a...))
	}
}

func (l logger) Debug(ctx context.Context, format string, a ...any) {
	if getLogLevel(ctx) <= LogLevelDebug {
		fmt.Println("DEBUG: " + fmt.Sprintf(format, a...))
	}
}

func (l *logger) SetLogLevel(ctx context.Context, logLevel int) context.Context {
	return context.WithValue(ctx, logLevel, logLevel)
}

func getLogLevel(ctx context.Context) int {
	if level, ok := ctx.Value(logLevel).(int); ok {
		return level
	}
	return defaultLogLevel
}
