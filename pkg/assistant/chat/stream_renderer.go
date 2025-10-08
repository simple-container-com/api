package chat

import (
	"strings"
)

// StreamRenderer handles real-time markdown rendering for streaming text
type StreamRenderer struct {
	inCodeBlock   bool
	codeBlockLang string
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
	// Handle code block state transitions when ``` markers are found
	if strings.Contains(chunk, "```") {
		// Process each line to handle ``` markers properly
		lines := strings.Split(chunk, "\n")
		for _, line := range lines {
			if strings.Contains(line, "```") {
				// Count ``` occurrences in this line
				codeBlockMarkers := strings.Count(line, "```")

				// Toggle state for each ``` marker found
				for i := 0; i < codeBlockMarkers; i++ {
					if !sr.inCodeBlock {
						// Entering code block
						sr.inCodeBlock = true
						// Extract language from the opening ``` line
						parts := strings.Split(line, "```")
						if len(parts) > 1 {
							lang := strings.TrimSpace(parts[1])
							if lang != "" {
								sr.codeBlockLang = lang
							}
						}
					} else {
						// Exiting code block
						sr.inCodeBlock = false
						sr.codeBlockLang = ""
					}
				}
			}
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
	sr.inCodeBlock = false // Reset state for next response
	sr.codeBlockLang = ""
	return ""
}

// Reset resets the renderer state
func (sr *StreamRenderer) Reset() {
	sr.inCodeBlock = false
	sr.codeBlockLang = ""
	sr.theme = GetCurrentTheme()
}

// SetTheme updates the theme
func (sr *StreamRenderer) SetTheme(theme *Theme) {
	sr.theme = theme
}
