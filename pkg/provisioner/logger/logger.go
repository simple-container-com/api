package logger

import (
	"context"
	"fmt"
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
	fmt.Println("ERROR: " + fmt.Sprintf(format, a...))
}

func (l logger) Warn(ctx context.Context, format string, a ...any) {
	fmt.Println("WARN: " + fmt.Sprintf(format, a...))
}

func (l logger) Info(ctx context.Context, format string, a ...any) {
	fmt.Println("INFO: " + fmt.Sprintf(format, a...))
}

func (l logger) Debug(ctx context.Context, format string, a ...any) {
	fmt.Println("DEBUG: " + fmt.Sprintf(format, a...))
}
