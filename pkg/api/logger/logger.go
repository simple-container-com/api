package logger

import (
	"context"
	"fmt"
	"time"
)

type logConfigKey struct{}

var logLevel logConfigKey = struct{}{}

const (
	LogLevelDebug  = iota
	LogLevelInfo   = iota
	LogLevelWarn   = iota
	LogLevelError  = iota
	LogLevelSilent = 99

	defaultLogLevel = LogLevelInfo
)

type Logger interface {
	Error(ctx context.Context, format string, a ...any)
	Warn(ctx context.Context, format string, a ...any)
	Info(ctx context.Context, format string, a ...any)
	Debug(ctx context.Context, format string, a ...any)
	SetLogLevel(ctx context.Context, logLevel int) context.Context
	Silent(ctx context.Context) context.Context
}

type logger struct{}

func New() Logger {
	return &logger{}
}

func (l *logger) Error(ctx context.Context, format string, a ...any) {
	if getLogLevel(ctx) <= LogLevelError {
		l.println(ctx, "ERROR", format, a...)
	}
}

func (l logger) Warn(ctx context.Context, format string, a ...any) {
	if getLogLevel(ctx) <= LogLevelWarn {
		l.println(ctx, "WARN", format, a...)
	}
}

func (l logger) Info(ctx context.Context, format string, a ...any) {
	if getLogLevel(ctx) <= LogLevelInfo {
		l.println(ctx, "INFO", format, a...)
	}
}

func (l logger) Debug(ctx context.Context, format string, a ...any) {
	if getLogLevel(ctx) <= LogLevelDebug {
		l.println(ctx, "DEBUG", format, a...)
	}
}

func (l logger) println(ctx context.Context, levelString, format string, a ...any) {
	datePrefix := fmt.Sprintf("[%s] ", time.Now().Format("2006-01-02T15:04:05"))
	fmt.Println(fmt.Sprintf("%s%s: ", datePrefix, levelString) + fmt.Sprintf(format, a...))
}

func (l logger) Silent(ctx context.Context) context.Context {
	return context.WithValue(ctx, logLevel, LogLevelSilent)
}

func (l logger) SetLogLevel(ctx context.Context, level int) context.Context {
	return context.WithValue(ctx, logLevel, level)
}

func getLogLevel(ctx context.Context) int {
	if level, ok := ctx.Value(logLevel).(int); ok {
		return level
	}
	return defaultLogLevel
}
