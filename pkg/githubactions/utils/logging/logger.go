package logging

import (
	"fmt"
	"io"
	"log"
	"os"
	"time"
)

// Logger interface for structured logging
type Logger interface {
	Info(msg string, keysAndValues ...interface{})
	Warn(msg string, keysAndValues ...interface{})
	Error(msg string, keysAndValues ...interface{})
	Debug(msg string, keysAndValues ...interface{})
}

// StandardLogger implements Logger interface with structured logging
type StandardLogger struct {
	component string
	infoLog   *log.Logger
	warnLog   *log.Logger
	errorLog  *log.Logger
	debugLog  *log.Logger
}

// NewLogger creates a new structured logger
func NewLogger(component string) Logger {
	return &StandardLogger{
		component: component,
		infoLog:   log.New(os.Stdout, "", 0),
		warnLog:   log.New(os.Stdout, "", 0),
		errorLog:  log.New(os.Stderr, "", 0),
		debugLog:  log.New(os.Stdout, "", 0),
	}
}

// NewLoggerWithOutput creates a logger with custom output
func NewLoggerWithOutput(component string, out io.Writer, errOut io.Writer) Logger {
	return &StandardLogger{
		component: component,
		infoLog:   log.New(out, "", 0),
		warnLog:   log.New(out, "", 0),
		errorLog:  log.New(errOut, "", 0),
		debugLog:  log.New(out, "", 0),
	}
}

// Info logs an info message with structured key-value pairs
func (l *StandardLogger) Info(msg string, keysAndValues ...interface{}) {
	formatted := l.formatMessage("INFO", msg, keysAndValues...)
	l.infoLog.Print(formatted)
}

// Warn logs a warning message with structured key-value pairs
func (l *StandardLogger) Warn(msg string, keysAndValues ...interface{}) {
	formatted := l.formatMessage("WARN", msg, keysAndValues...)
	l.warnLog.Print(formatted)
}

// Error logs an error message with structured key-value pairs
func (l *StandardLogger) Error(msg string, keysAndValues ...interface{}) {
	formatted := l.formatMessage("ERROR", msg, keysAndValues...)
	l.errorLog.Print(formatted)
}

// Debug logs a debug message with structured key-value pairs
func (l *StandardLogger) Debug(msg string, keysAndValues ...interface{}) {
	// Only show debug logs if DEBUG environment variable is set
	if os.Getenv("DEBUG") == "" {
		return
	}
	formatted := l.formatMessage("DEBUG", msg, keysAndValues...)
	l.debugLog.Print(formatted)
}

// formatMessage formats a log message with timestamp, level, component, and key-value pairs
func (l *StandardLogger) formatMessage(level, msg string, keysAndValues ...interface{}) string {
	timestamp := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")

	// Build the base message
	formatted := fmt.Sprintf("[%s] %s [%s] %s", timestamp, level, l.component, msg)

	// Add key-value pairs if provided
	if len(keysAndValues) > 0 {
		formatted += " "
		formatted += l.formatKeyValues(keysAndValues...)
	}

	return formatted
}

// formatKeyValues formats key-value pairs into a readable string
func (l *StandardLogger) formatKeyValues(keysAndValues ...interface{}) string {
	if len(keysAndValues) == 0 {
		return ""
	}

	var parts []string

	// Process pairs of key-value arguments
	for i := 0; i < len(keysAndValues); i += 2 {
		if i+1 < len(keysAndValues) {
			key := fmt.Sprintf("%v", keysAndValues[i])
			value := fmt.Sprintf("%v", keysAndValues[i+1])
			parts = append(parts, fmt.Sprintf("%s=%s", key, value))
		} else {
			// Handle odd number of arguments
			key := fmt.Sprintf("%v", keysAndValues[i])
			parts = append(parts, fmt.Sprintf("%s=<missing_value>", key))
		}
	}

	result := ""
	for i, part := range parts {
		if i > 0 {
			result += " "
		}
		result += part
	}

	return result
}

// NoOpLogger is a logger that does nothing (useful for testing)
type NoOpLogger struct{}

// NewNoOpLogger creates a logger that does nothing
func NewNoOpLogger() Logger {
	return &NoOpLogger{}
}

func (n *NoOpLogger) Info(msg string, keysAndValues ...interface{})  {}
func (n *NoOpLogger) Warn(msg string, keysAndValues ...interface{})  {}
func (n *NoOpLogger) Error(msg string, keysAndValues ...interface{}) {}
func (n *NoOpLogger) Debug(msg string, keysAndValues ...interface{}) {}
