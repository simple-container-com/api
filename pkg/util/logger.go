package util

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/pkg/errors"
)

type Logger interface {
	Debugf(format string, args ...interface{})

	Log(msg string)

	Logf(format string, args ...interface{})

	Err(msg string)

	Errf(format string, args ...interface{})

	SubLogger(name string) Logger
}

type NoopLogger struct{}

type StdoutLogger struct {
	stdout io.WriteCloser
	stderr io.WriteCloser
	debug  bool
}

type PrefixLogger struct {
	prefix     string
	debug      bool
	printTime  bool
	timeFormat string
}

func NewStdoutLogger(stdout io.WriteCloser, stderr io.WriteCloser) *StdoutLogger {
	if stdout == nil {
		stdout = os.Stdout
	}
	if stderr == nil {
		stderr = os.Stderr
	}
	return &StdoutLogger{
		stdout: stdout,
		stderr: stderr,
	}
}

func (l *StdoutLogger) Debug() *StdoutLogger {
	l.debug = true
	return l
}

func (l *NoopLogger) Err(msg string) {
}

func (l *NoopLogger) Errf(format string, msg ...interface{}) {
}

func (l *NoopLogger) Log(msg string) {
}

func (l *NoopLogger) Logf(format string, msg ...interface{}) {
}

func (l *NoopLogger) SubLogger(name string) Logger {
	return l
}

func (l *NoopLogger) Debugf(format string, msg ...interface{}) {
}

func (l *StdoutLogger) Log(msg string) {
	_, _ = l.stdout.Write([]byte(msg + "\n"))
}

func (l *StdoutLogger) Logf(format string, msg ...interface{}) {
	l.Log(fmt.Sprintf(format, msg...))
}

func (l *StdoutLogger) Err(msg string) {
	_, _ = color.New(color.FgRed).Fprint(l.stderr, []byte(msg+"\n"))
}

func (l *StdoutLogger) Errf(format string, msg ...interface{}) {
	l.Err(fmt.Sprintf(format, msg...))
}

func (l *StdoutLogger) SubLogger(name string) Logger {
	return l
}

func (l *StdoutLogger) Debugf(format string, msg ...interface{}) {
	if l.debug {
		l.Logf(format, msg...)
	}
}

func (l *PrefixLogger) WithTimeFormat(format string) *PrefixLogger {
	l.timeFormat = format
	return l
}

func (l *PrefixLogger) log(writer io.Writer, color *color.Color, msg string) {
	message := strings.Trim(msg, "\n")
	if l.printTime {
		_, _ = color.Fprintln(writer, time.Now().Format(l.timeFormat), l.prefix, message)
	} else {
		_, _ = color.Fprintln(writer, l.prefix, message)
	}
}

func (l *PrefixLogger) Log(msg string) {
	l.log(os.Stdout, color.New(color.Reset), msg)
}

func (l *PrefixLogger) Logf(format string, msg ...interface{}) {
	l.Log(fmt.Sprintf(format, msg...))
}

func (l *PrefixLogger) Err(msg string) {
	l.log(os.Stderr, color.New(color.FgRed), msg)
}

func (l *PrefixLogger) Errf(format string, msg ...interface{}) {
	l.Err(fmt.Sprintf(format, msg...))
}

func (l *PrefixLogger) SubLogger(name string) Logger {
	return &PrefixLogger{
		prefix:     l.prefix + " [" + name + "]",
		debug:      l.debug,
		timeFormat: l.timeFormat,
		printTime:  l.printTime,
	}
}

func (l *PrefixLogger) Debugf(format string, msg ...interface{}) {
	if l.debug {
		l.Logf(format, msg...)
	}
}

// ReaderToLogFunc returns function that is meant to be called from a separate goroutine
// function starts streaming from reader to logger and appends extra prefix to each line
func ReaderToLogFunc(reader io.Reader, logToErr bool, prefix string, logger Logger, subject string) func() error {
	scanner := NewLineOrReturnScanner(reader)
	return func() error {
		for {
			if !scanner.Scan() {
				if scanner.Err() != nil {
					return errors.Wrapf(scanner.Err(), "failed to read next log stream for %s", subject)
				}
				return nil
			}
			switch scanner.Err() {
			case nil:
				if logToErr {
					logger.Err(prefix + scanner.Text())
				} else {
					logger.Log(prefix + scanner.Text())
				}
			default:
				return errors.Wrapf(scanner.Err(), "failed to read next log stream for %s", subject)
			}
		}
	}
}
