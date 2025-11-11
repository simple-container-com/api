package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/simple-container-com/api/pkg/api/logger"
)

// MCPMode represents the MCP server mode for logging behavior
type MCPMode string

const (
	MCPModeHTTP  MCPMode = "http"
	MCPModeStdio MCPMode = "stdio"
)

// MCPLogEntry represents a structured log entry in JSON format
type MCPLogEntry struct {
	Timestamp  string                 `json:"timestamp"`
	Level      string                 `json:"level"`
	Component  string                 `json:"component"`
	Message    string                 `json:"message"`
	Method     string                 `json:"method,omitempty"`
	Error      string                 `json:"error,omitempty"`
	ErrorType  string                 `json:"error_type,omitempty"`
	Duration   string                 `json:"duration,omitempty"`
	RequestID  string                 `json:"request_id,omitempty"`
	ClientID   string                 `json:"client_id,omitempty"`
	UserAgent  string                 `json:"user_agent,omitempty"`
	RemoteAddr string                 `json:"remote_addr,omitempty"`
	Mode       string                 `json:"mode"`
	ProcessID  int                    `json:"process_id"`
	ThreadID   string                 `json:"thread_id,omitempty"`
	Context    map[string]interface{} `json:"context,omitempty"`
	SessionID  string                 `json:"session_id"`
	StackTrace string                 `json:"stack_trace,omitempty"`
}

// MCPLogger implements the Simple Container Logger interface with multiple sinks
type MCPLogger struct {
	baseLogger    logger.Logger
	fileWriter    io.Writer
	filePath      string
	sessionID     string
	mode          MCPMode
	enableConsole bool
	verboseMode   bool
	mutex         sync.Mutex
	component     string
	processID     int
}

// NewMCPLogger creates a new MCP logger with mode-aware multiple sinks
func NewMCPLogger(component string, mode MCPMode, verboseMode bool) (*MCPLogger, error) {
	// Create session ID based on timestamp
	sessionID := fmt.Sprintf("mcp-%s", time.Now().Format("20060102-150405"))

	// Create ~/.sc/logs directory if it doesn't exist
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	logsDir := filepath.Join(homeDir, ".sc", "logs")
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create logs directory: %w", err)
	}

	// Create log file with mode suffix for clarity
	logFileName := fmt.Sprintf("%s-%s.log", sessionID, string(mode))
	logFilePath := filepath.Join(logsDir, logFileName)

	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("failed to create log file: %w", err)
	}

	// Console logging behavior based on mode
	enableConsole := false
	if mode == MCPModeHTTP && verboseMode {
		enableConsole = true
	}

	mcpLogger := &MCPLogger{
		baseLogger:    logger.New(),
		fileWriter:    logFile,
		filePath:      logFilePath,
		sessionID:     sessionID,
		mode:          mode,
		enableConsole: enableConsole,
		verboseMode:   verboseMode,
		component:     component,
		processID:     os.Getpid(),
	}

	// Log initialization
	initCtx := context.Background()
	mcpLogger.Info(initCtx, "MCP Logger initialized - session: %s, mode: %s, verbose: %v, file: %s",
		sessionID, string(mode), verboseMode, logFilePath)

	return mcpLogger, nil
}

// writeJSONLog writes a structured log entry to the JSON file with rich context
func (m *MCPLogger) writeJSONLog(level, message string, context map[string]interface{}) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	entry := MCPLogEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Level:     level,
		Component: m.component,
		Message:   message,
		SessionID: m.sessionID,
		Mode:      string(m.mode),
		ProcessID: m.processID,
		Context:   context,
	}

	// Add additional context if available
	if context != nil {
		if method, ok := context["method"].(string); ok {
			entry.Method = method
		}
		if requestID, ok := context["request_id"].(string); ok {
			entry.RequestID = requestID
		}
		if clientID, ok := context["client_id"].(string); ok {
			entry.ClientID = clientID
		}
		if userAgent, ok := context["user_agent"].(string); ok {
			entry.UserAgent = userAgent
		}
		if remoteAddr, ok := context["remote_addr"].(string); ok {
			entry.RemoteAddr = remoteAddr
		}
		if duration, ok := context["duration"].(string); ok {
			entry.Duration = duration
		}
		if errorStr, ok := context["error"].(string); ok {
			entry.Error = errorStr
		}
		if errorType, ok := context["error_type"].(string); ok {
			entry.ErrorType = errorType
		}
		if stackTrace, ok := context["stack_trace"].(string); ok {
			entry.StackTrace = stackTrace
		}
	}

	if jsonData, err := json.Marshal(entry); err == nil {
		fmt.Fprintf(m.fileWriter, "%s\n", jsonData)
	}
}

// Error logs an error message with mode-aware multiple sinks
func (m *MCPLogger) Error(ctx context.Context, format string, a ...any) {
	message := fmt.Sprintf(format, a...)

	// Console output only in HTTP mode with verbose enabled
	if m.enableConsole {
		m.baseLogger.Error(ctx, format, a...)
	}

	// Extract rich error context
	logContext := make(map[string]interface{})

	// Extract error information from arguments
	for i, arg := range a {
		if err, ok := arg.(error); ok {
			logContext["error"] = err.Error()
			logContext["error_type"] = fmt.Sprintf("%T", err)
			// Add error position in arguments for debugging
			logContext["error_arg_index"] = i
		}
	}

	// Add runtime context from context.Context if available
	m.addContextFromCtx(ctx, logContext)

	m.writeJSONLog("ERROR", message, logContext)
}

// addContextFromCtx extracts additional context from context.Context
func (m *MCPLogger) addContextFromCtx(ctx context.Context, logContext map[string]interface{}) {
	// Extract request ID if available (from HTTP requests or MCP request ID)
	if requestID := ctx.Value("request_id"); requestID != nil {
		if rid, ok := requestID.(string); ok {
			logContext["request_id"] = rid
		}
	}

	// Extract client information from HTTP context
	if clientID := ctx.Value("client_id"); clientID != nil {
		if cid, ok := clientID.(string); ok {
			logContext["client_id"] = cid
		}
	}

	// Extract user agent from HTTP context
	if userAgent := ctx.Value("user_agent"); userAgent != nil {
		if ua, ok := userAgent.(string); ok {
			logContext["user_agent"] = ua
		}
	}

	// Extract remote address from HTTP context
	if remoteAddr := ctx.Value("remote_addr"); remoteAddr != nil {
		if ra, ok := remoteAddr.(string); ok {
			logContext["remote_addr"] = ra
		}
	}

	// Extract method name from MCP context
	if method := ctx.Value("mcp_method"); method != nil {
		if m, ok := method.(string); ok {
			logContext["method"] = m
		}
	}
}

// Warn logs a warning message with mode-aware multiple sinks
func (m *MCPLogger) Warn(ctx context.Context, format string, a ...any) {
	message := fmt.Sprintf(format, a...)

	// Console output only in HTTP mode with verbose enabled
	if m.enableConsole {
		m.baseLogger.Warn(ctx, format, a...)
	}

	// Extract context information
	logContext := make(map[string]interface{})
	m.addContextFromCtx(ctx, logContext)

	m.writeJSONLog("WARN", message, logContext)
}

// Info logs an info message with mode-aware multiple sinks
func (m *MCPLogger) Info(ctx context.Context, format string, a ...any) {
	message := fmt.Sprintf(format, a...)

	// Console output only in HTTP mode with verbose enabled
	if m.enableConsole {
		m.baseLogger.Info(ctx, format, a...)
	}

	// Extract context information
	logContext := make(map[string]interface{})
	m.addContextFromCtx(ctx, logContext)

	m.writeJSONLog("INFO", message, logContext)
}

// Debug logs a debug message with mode-aware multiple sinks
func (m *MCPLogger) Debug(ctx context.Context, format string, a ...any) {
	message := fmt.Sprintf(format, a...)

	// Console output only in HTTP mode with verbose enabled
	if m.enableConsole {
		m.baseLogger.Debug(ctx, format, a...)
	}

	// Extract context information
	logContext := make(map[string]interface{})
	m.addContextFromCtx(ctx, logContext)

	m.writeJSONLog("DEBUG", message, logContext)
}

// SetLogLevel sets the log level for the underlying logger
func (m *MCPLogger) SetLogLevel(ctx context.Context, logLevel int) context.Context {
	return m.baseLogger.SetLogLevel(ctx, logLevel)
}

// Silent sets the logger to silent mode for the context
func (m *MCPLogger) Silent(ctx context.Context) context.Context {
	return m.baseLogger.Silent(ctx)
}

// LogMCPRequest logs an MCP request with rich structured data
func (m *MCPLogger) LogMCPRequest(method string, params interface{}, duration time.Duration, requestID string) {
	logContext := map[string]interface{}{
		"method":     method,
		"duration":   duration.String(),
		"request_id": requestID,
	}

	// Add parameters if provided (truncate large params for readability)
	if params != nil {
		paramStr := fmt.Sprintf("%v", params)
		if len(paramStr) > 1000 {
			paramStr = paramStr[:1000] + "... (truncated)"
		}
		logContext["params"] = paramStr
		logContext["params_type"] = fmt.Sprintf("%T", params)
	}

	// Add performance classification
	if duration < time.Millisecond*10 {
		logContext["performance"] = "fast"
	} else if duration < time.Millisecond*100 {
		logContext["performance"] = "normal"
	} else if duration < time.Second {
		logContext["performance"] = "slow"
	} else {
		logContext["performance"] = "very_slow"
	}

	message := fmt.Sprintf("MCP request: %s (duration: %s)", method, duration)
	m.writeJSONLog("INFO", message, logContext)
}

// LogMCPError logs an MCP error with enhanced structured data
func (m *MCPLogger) LogMCPError(method string, err error, additionalContext map[string]interface{}) {
	logContext := make(map[string]interface{})

	// Copy additional context if provided
	for k, v := range additionalContext {
		logContext[k] = v
	}

	// Add core error information
	logContext["method"] = method
	logContext["error"] = err.Error()
	logContext["error_type"] = fmt.Sprintf("%T", err)

	// Extract stack trace if it's a special error type with stack info
	if stackTracer, ok := err.(interface{ StackTrace() string }); ok {
		logContext["stack_trace"] = stackTracer.StackTrace()
	}

	message := fmt.Sprintf("MCP error in %s: %s", method, err.Error())
	m.writeJSONLog("ERROR", message, logContext)
}

// LogMCPPanic logs an MCP panic recovery with comprehensive context
func (m *MCPLogger) LogMCPPanic(method string, recovered interface{}, additionalContext map[string]interface{}) {
	logContext := make(map[string]interface{})

	// Copy additional context if provided
	for k, v := range additionalContext {
		logContext[k] = v
	}

	// Add panic information
	logContext["method"] = method
	logContext["panic"] = fmt.Sprintf("%v", recovered)
	logContext["panic_type"] = fmt.Sprintf("%T", recovered)
	logContext["error_type"] = "panic"

	// Try to capture stack trace (simplified version)
	logContext["stack_trace"] = "panic recovery - full stack trace in runtime logs"

	message := fmt.Sprintf("MCP panic recovered in %s: %v", method, recovered)
	m.writeJSONLog("ERROR", message, logContext)
}

// GetLogFilePath returns the path to the current log file
func (m *MCPLogger) GetLogFilePath() string {
	return m.filePath
}

// GetSessionID returns the current session ID
func (m *MCPLogger) GetSessionID() string {
	return m.sessionID
}

// Close closes the log file writer
func (m *MCPLogger) Close() error {
	if closer, ok := m.fileWriter.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}
