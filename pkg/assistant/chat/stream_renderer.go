package chat

import (
	"strings"
)

// StreamRenderer handles real-time markdown rendering for streaming text
type StreamRenderer struct {
	inCodeBlock   bool
	codeBlockLang string
	theme         *Theme
	buffer        string // Small buffer for ``` detection across chunks
}

// NewStreamRenderer creates a new stream renderer
func NewStreamRenderer() *StreamRenderer {
	return &StreamRenderer{
		theme: GetCurrentTheme(),
	}
}

// ProcessChunk processes a chunk of streaming text and returns colored output
func (sr *StreamRenderer) ProcessChunk(chunk string) string {
	// Add chunk to small buffer for ``` detection across chunks
	sr.buffer += chunk

	// Keep only last 10 characters to detect split ``` markers
	if len(sr.buffer) > 10 {
		sr.buffer = sr.buffer[len(sr.buffer)-10:]
	}

	// Check for ``` in buffer (handles split markers)
	if strings.Contains(sr.buffer, "```") {
		// Toggle state when ``` completion is detected
		sr.inCodeBlock = !sr.inCodeBlock

		if sr.inCodeBlock {
			// Try to extract language from buffer
			lines := strings.Split(sr.buffer, "\n")
			for _, line := range lines {
				if strings.Contains(line, "```") {
					parts := strings.Split(line, "```")
					if len(parts) > 1 {
						lang := strings.TrimSpace(parts[1])
						if lang != "" {
							sr.codeBlockLang = lang
						}
					}
					break
				}
			}
		} else {
			// Exiting code block
			sr.codeBlockLang = ""
		}

		// Clear buffer after processing ``` to avoid re-triggering
		sr.buffer = ""
	}

	// Apply styling based on current state
	if sr.inCodeBlock {
		return sr.theme.ApplyCode(chunk)
	} else {
		return sr.theme.ApplyText(chunk)
	}
}

// Flush returns any remaining buffered content
func (sr *StreamRenderer) Flush() string {
	// For real-time streaming, everything has already been output immediately
	// in ProcessChunk, so there's nothing to flush to avoid duplication
	sr.inCodeBlock = false // Reset state for next response
	sr.codeBlockLang = ""
	sr.buffer = "" // Clear detection buffer
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
