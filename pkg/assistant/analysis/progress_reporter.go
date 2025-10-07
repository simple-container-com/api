package analysis

import (
	"fmt"
	"io"
	"strings"
	"time"
)

// ConsoleProgressReporter provides console-based progress reporting
type ConsoleProgressReporter struct {
	writer    io.Writer
	startTime time.Time
	lastPhase string
}

// NewConsoleProgressReporter creates a new console progress reporter
func NewConsoleProgressReporter(writer io.Writer) *ConsoleProgressReporter {
	return &ConsoleProgressReporter{
		writer:    writer,
		startTime: time.Now(),
	}
}

// ReportProgress reports progress to the console
func (c *ConsoleProgressReporter) ReportProgress(phase string, message string, percentage int) {
	elapsed := time.Since(c.startTime)

	// Create progress bar
	progressWidth := 30
	filled := int(float64(progressWidth) * float64(percentage) / 100.0)
	bar := strings.Repeat("█", filled) + strings.Repeat("░", progressWidth-filled)

	// Format time
	timeStr := fmt.Sprintf("%.1fs", elapsed.Seconds())

	// Print progress line
	fmt.Fprintf(c.writer, "\r🔍 [%s] %d%% %s (%s)", bar, percentage, message, timeStr)

	// Add newline for completion or phase changes
	if percentage == 100 || (c.lastPhase != "" && c.lastPhase != phase) {
		fmt.Fprintf(c.writer, "\n")
	}

	c.lastPhase = phase
}

// StreamingProgressReporter provides streaming progress updates for MCP compatibility
type StreamingProgressReporter struct {
	writer      io.Writer
	startTime   time.Time
	lastPhase   string
	updateCount int
}

// NewStreamingProgressReporter creates a new streaming progress reporter
func NewStreamingProgressReporter(writer io.Writer) *StreamingProgressReporter {
	return &StreamingProgressReporter{
		writer:    writer,
		startTime: time.Now(),
	}
}

// ReportProgress reports streaming progress updates
func (s *StreamingProgressReporter) ReportProgress(phase string, message string, percentage int) {
	elapsed := time.Since(s.startTime)
	s.updateCount++

	// Format timestamp
	timestamp := time.Now().Format("15:04:05")

	// Create different output for different phases to show progress
	phaseIndicators := map[string]string{
		"initialization":           "🚀",
		"tech_stack_detection":     "💻",
		"tech_stack_analysis":      "🔧",
		"architecture_detection":   "🏗️",
		"initial_recommendations":  "💡",
		"file_analysis":            "📁",
		"resource_detection":       "🔍",
		"git_analysis":             "📊",
		"enhanced_recommendations": "✨",
		"llm_enhancement":          "🤖",
		"completion":               "✅",
	}

	indicator := phaseIndicators[phase]
	if indicator == "" {
		indicator = "⚙️"
	}

	// Print streaming update
	fmt.Fprintf(s.writer, "[%s] %s %s (%d%% - %.1fs)\n", timestamp, indicator, message, percentage, elapsed.Seconds())

	// Add separator every few updates to improve readability
	if s.updateCount%3 == 0 && percentage < 100 {
		fmt.Fprintf(s.writer, "   ┊\n")
	}

	s.lastPhase = phase
}

// JSONProgressReporter provides JSON-formatted progress updates for programmatic consumption
type JSONProgressReporter struct {
	writer    io.Writer
	startTime time.Time
}

// NewJSONProgressReporter creates a new JSON progress reporter
func NewJSONProgressReporter(writer io.Writer) *JSONProgressReporter {
	return &JSONProgressReporter{
		writer:    writer,
		startTime: time.Now(),
	}
}

// ReportProgress reports progress in JSON format
func (j *JSONProgressReporter) ReportProgress(phase string, message string, percentage int) {
	elapsed := time.Since(j.startTime)

	progressUpdate := fmt.Sprintf(`{"type":"progress","phase":"%s","message":"%s","percentage":%d,"elapsed_seconds":%.2f,"timestamp":"%s"}`,
		phase, message, percentage, elapsed.Seconds(), time.Now().Format(time.RFC3339))

	fmt.Fprintf(j.writer, "%s\n", progressUpdate)
}
