package chat

import (
	"fmt"
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

		// Render line based on context
		if sr.inCodeBlock {
			output.WriteString(sr.theme.ApplyCode("│ " + line))
		} else {
			output.WriteString(sr.renderInlineLine(line))
		}

		// Always add newline after each processed line
		output.WriteString("\n")
	}

	return output.String()
}

// renderInlineLine renders a line with inline markdown
func (sr *StreamRenderer) renderInlineLine(line string) string {
	// For streaming, we'll use a simple approach - just color the text
	// Full markdown parsing happens on complete responses

	// Check for inline code
	if strings.Contains(line, "`") {
		var result strings.Builder
		inCode := false
		lastIdx := 0

		for i, ch := range line {
			if ch == '`' {
				if inCode {
					// End of code
					result.WriteString(sr.theme.ApplyCode(line[lastIdx : i+1]))
					inCode = false
					lastIdx = i + 1
				} else {
					// Start of code - output text before
					if lastIdx < i {
						result.WriteString(sr.theme.ApplyText(line[lastIdx:i]))
					}
					inCode = true
					lastIdx = i
				}
			}
		}

		// Output remaining text
		if lastIdx < len(line) {
			if inCode {
				result.WriteString(sr.theme.ApplyCode(line[lastIdx:]))
			} else {
				result.WriteString(sr.theme.ApplyText(line[lastIdx:]))
			}
		}

		return result.String()
	}

	// Check for bold text
	if strings.Contains(line, "**") {
		parts := strings.Split(line, "**")
		var result strings.Builder
		for i, part := range parts {
			if i%2 == 1 {
				// Odd index = bold text
				result.WriteString(sr.theme.ApplyEmphasis(part))
			} else {
				result.WriteString(sr.theme.ApplyText(part))
			}
		}
		return result.String()
	}

	// Check for headers
	if strings.HasPrefix(line, "#") {
		return sr.theme.ApplyHeader(line)
	}

	// Check for list items
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
		indent := line[:len(line)-len(trimmed)]
		content := trimmed[2:]
		return indent + sr.theme.ApplyEmphasis("• ") + sr.theme.ApplyText(content)
	}

	// Default text color
	return sr.theme.ApplyText(line)
}

// Flush returns any remaining buffered content
func (sr *StreamRenderer) Flush() string {
	// For real-time streaming, everything has already been output immediately
	// in ProcessChunk, so there's nothing to flush to avoid duplication
	sr.inCodeBlock = false // Reset state for next response
	sr.codeBlockLang = ""
	return ""
}

	remaining := sr.buffer
	sr.buffer = ""

	if sr.inCodeBlock {
		return sr.theme.ApplyCode("│ " + remaining)
	}

	return sr.renderInlineLine(remaining)
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

// Example usage in streaming context:
func ExampleStreamUsage() {
	renderer := NewStreamRenderer()

	// Simulate streaming chunks
	chunks := []string{
		"Hello, this is ",
		"some text with `code` ",
		"and more text\n",
		"```go\n",
		"func main() {\n",
		"    fmt.Println",
		"(\"Hello\")\n",
		"}\n",
		"```\n",
		"Done!\n",
	}

	for _, chunk := range chunks {
		output := renderer.ProcessChunk(chunk)
		fmt.Print(output)
	}

	// Flush any remaining content
	fmt.Print(renderer.Flush())
}
