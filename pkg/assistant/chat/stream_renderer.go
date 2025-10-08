package chat

import (
	"strings"
)

// StreamRenderer handles real-time markdown rendering for streaming text
type StreamRenderer struct {
	inCodeBlock   bool
	codeBlockLang string
	buffer        string
	theme         *Theme
}

// NewStreamRenderer creates a new stream renderer
func NewStreamRenderer() *StreamRenderer {
	return &StreamRenderer{
		theme: GetCurrentTheme(),
	}
}

// ProcessChunk processes a chunk of streaming text and returns colored output
func (sr *StreamRenderer) ProcessChunk(chunk string) string {
	// For real-time streaming, we need to track state properly
	// Add chunk to buffer for robust ``` detection
	sr.buffer += chunk

	// Manage buffer size to prevent memory issues (keep last 200 chars)
	const maxBufferSize = 200
	if len(sr.buffer) > maxBufferSize {
		sr.buffer = sr.buffer[len(sr.buffer)-maxBufferSize:]
	}

	// Check for ``` patterns in the buffer
	codeBlockCount := strings.Count(sr.buffer, "```")
	newCodeBlockState := (codeBlockCount%2 == 1) // Odd count = inside code block

	// Update state if it changed
	if newCodeBlockState != sr.inCodeBlock {
		sr.inCodeBlock = newCodeBlockState
		if sr.inCodeBlock {
			// Extract language if we just entered a code block
			if strings.Contains(chunk, "```") {
				parts := strings.Split(chunk, "```")
				if len(parts) > 1 {
					sr.codeBlockLang = strings.TrimSpace(parts[1])
				}
			}
		} else {
			// Clear language when exiting code block
			sr.codeBlockLang = ""
		}
	}

	// Apply styling based on current state
	if sr.inCodeBlock {
		return sr.theme.ApplyCode(chunk)
	}

	// For regular text, apply basic text styling
	return sr.theme.ApplyText(chunk)
}

// Flush returns any remaining buffered content
func (sr *StreamRenderer) Flush() string {
	// For real-time streaming, everything has already been output immediately
	// in ProcessChunk, so there's nothing to flush to avoid duplication
	sr.buffer = ""         // Clear state tracking buffer
	sr.inCodeBlock = false // Reset state
	sr.codeBlockLang = ""
	return ""
}

// Reset resets the renderer state
func (sr *StreamRenderer) Reset() {
	sr.inCodeBlock = false
	sr.codeBlockLang = ""
	sr.buffer = ""
	sr.theme = GetCurrentTheme()
}

// SetTheme updates the theme
func (sr *StreamRenderer) SetTheme(theme *Theme) {
	sr.theme = theme
}
