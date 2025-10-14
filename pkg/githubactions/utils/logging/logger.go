package logging

import (
	"fmt"
	"os"
	"time"

	"github.com/simple-container-com/api/pkg/util"
)

// Logger interface for structured logging - maintains compatibility with existing githubactions code
type Logger interface {
	Info(msg string, keysAndValues ...interface{})
	Warn(msg string, keysAndValues ...interface{})
	Error(msg string, keysAndValues ...interface{})
	Debug(msg string, keysAndValues ...interface{})
}

// LoggerWrapper wraps SC's existing util.Logger to provide structured logging interface
type LoggerWrapper struct {
	component  string
	utilLogger util.Logger
}

// NewLogger creates a new logger wrapper around SC's util logger
func NewLogger(component string) Logger {
	// Create a StdoutLogger using SC's existing logger
	stdoutLogger := util.NewStdoutLogger(nil, nil) // Uses os.Stdout, os.Stderr

	return &LoggerWrapper{
		component:  component,
		utilLogger: stdoutLogger,
	}
}

// NewLoggerWithUtilLogger creates a logger wrapper around an existing util.Logger
func NewLoggerWithUtilLogger(component string, utilLogger util.Logger) Logger {
	return &LoggerWrapper{
		component:  component,
		utilLogger: utilLogger,
	}
}

// Info logs an info message with structured key-value pairs
func (l *LoggerWrapper) Info(msg string, keysAndValues ...interface{}) {
	formatted := l.formatMessage("INFO", msg, keysAndValues...)
	l.utilLogger.Log(formatted)
}

// Warn logs a warning message with structured key-value pairs
func (l *LoggerWrapper) Warn(msg string, keysAndValues ...interface{}) {
	formatted := l.formatMessage("WARN", msg, keysAndValues...)
	l.utilLogger.Log(formatted)
}

// Error logs an error message with structured key-value pairs
func (l *LoggerWrapper) Error(msg string, keysAndValues ...interface{}) {
	formatted := l.formatMessage("ERROR", msg, keysAndValues...)
	l.utilLogger.Err(formatted)
}

// Debug logs a debug message with structured key-value pairs
func (l *LoggerWrapper) Debug(msg string, keysAndValues ...interface{}) {
	// Only show debug logs if DEBUG environment variable is set
	if os.Getenv("DEBUG") == "" {
		return
	}
	formatted := l.formatMessage("DEBUG", msg, keysAndValues...)
	l.utilLogger.Debugf("%s", formatted)
}

// formatMessage formats a log message with timestamp, level, component, and key-value pairs
func (l *LoggerWrapper) formatMessage(level, msg string, keysAndValues ...interface{}) string {
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
func (l *LoggerWrapper) formatKeyValues(keysAndValues ...interface{}) string {
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

// NoOpLoggerWrapper wraps util.NoopLogger for compatibility
type NoOpLoggerWrapper struct{}

// NewNoOpLogger creates a logger that does nothing (useful for testing)
func NewNoOpLogger() Logger {
	return &NoOpLoggerWrapper{}
}

func (n *NoOpLoggerWrapper) Info(msg string, keysAndValues ...interface{})  {}
func (n *NoOpLoggerWrapper) Warn(msg string, keysAndValues ...interface{})  {}
func (n *NoOpLoggerWrapper) Error(msg string, keysAndValues ...interface{}) {}
func (n *NoOpLoggerWrapper) Debug(msg string, keysAndValues ...interface{}) {}
