package chat

import (
	"regexp"
	"strings"
)

// MarkdownRenderer renders markdown text with colors for terminal
type MarkdownRenderer struct {
	codeBlockRegex   *regexp.Regexp
	inlineCodeRegex  *regexp.Regexp
	boldRegex        *regexp.Regexp
	headerRegex      *regexp.Regexp
	listItemRegex    *regexp.Regexp
}

// NewMarkdownRenderer creates a new markdown renderer
func NewMarkdownRenderer() *MarkdownRenderer {
	return &MarkdownRenderer{
		codeBlockRegex:  regexp.MustCompile("(?s)```([a-z]*)\n(.*?)```"),
		inlineCodeRegex: regexp.MustCompile("`([^`]+)`"),
		boldRegex:       regexp.MustCompile(`\*\*([^*]+)\*\*`),
		headerRegex:     regexp.MustCompile(`^(#{1,6})\s+(.+)$`),
		listItemRegex:   regexp.MustCompile(`^(\s*)[-*]\s+(.+)$`),
	}
}

// Render renders markdown text with colors
func (mr *MarkdownRenderer) Render(text string) string {
	// Split into lines for processing
	lines := strings.Split(text, "\n")
	var result []string

	inCodeBlock := false
	var codeBlockContent []string
	var codeBlockLang string

	for _, line := range lines {
		// Check for code block start/end
		if strings.HasPrefix(line, "```") {
			if !inCodeBlock {
				// Start of code block
				inCodeBlock = true
				codeBlockLang = strings.TrimPrefix(line, "```")
				codeBlockContent = []string{}
				continue
			} else {
				// End of code block
				inCodeBlock = false
				// Render code block
				result = append(result, mr.renderCodeBlock(strings.Join(codeBlockContent, "\n"), codeBlockLang))
				codeBlockContent = []string{}
				codeBlockLang = ""
				continue
			}
		}

		if inCodeBlock {
			codeBlockContent = append(codeBlockContent, line)
			continue
		}

		// Process regular lines
		line = mr.renderLine(line)
		result = append(result, line)
	}

	return strings.Join(result, "\n")
}

// renderLine renders a single line with markdown formatting
func (mr *MarkdownRenderer) renderLine(line string) string {
	theme := GetCurrentTheme()

	// Headers
	if matches := mr.headerRegex.FindStringSubmatch(line); matches != nil {
		level := len(matches[1])
		text := matches[2]
		if level <= 2 {
			return theme.ApplyHeader(text)
		}
		return theme.ApplyEmphasis(text)
	}

	// List items
	if matches := mr.listItemRegex.FindStringSubmatch(line); matches != nil {
		indent := matches[1]
		content := matches[2]
		content = mr.renderInline(content)
		return indent + theme.ApplyEmphasis("• ") + content
	}

	// Process inline formatting
	return mr.renderInline(line)
}

// renderInline renders inline markdown formatting
func (mr *MarkdownRenderer) renderInline(text string) string {
	theme := GetCurrentTheme()
	var result strings.Builder
	lastIndex := 0

	// Track all special segments (code and bold)
	type segment struct {
		start int
		end   int
		isCode bool
		isBold bool
		content string
	}
	var segments []segment

	// Find inline code segments
	for _, match := range mr.inlineCodeRegex.FindAllStringSubmatchIndex(text, -1) {
		segments = append(segments, segment{
			start: match[0],
			end: match[1],
			isCode: true,
			content: text[match[2]:match[3]], // captured group
		})
	}

	// Find bold segments
	for _, match := range mr.boldRegex.FindAllStringSubmatchIndex(text, -1) {
		segments = append(segments, segment{
			start: match[0],
			end: match[1],
			isBold: true,
			content: text[match[2]:match[3]], // captured group
		})
	}

	// Sort segments by start position
	for i := 0; i < len(segments); i++ {
		for j := i + 1; j < len(segments); j++ {
			if segments[j].start < segments[i].start {
				segments[i], segments[j] = segments[j], segments[i]
			}
		}
	}

	// Build the result with colored segments
	for _, seg := range segments {
		// Add plain text before this segment
		if lastIndex < seg.start {
			plainText := text[lastIndex:seg.start]
			if plainText != "" {
				result.WriteString(theme.ApplyText(plainText))
			}
		}

		// Add formatted segment
		if seg.isCode {
			result.WriteString(theme.ApplyCode("`" + seg.content + "`"))
		} else if seg.isBold {
			result.WriteString(theme.ApplyEmphasis(seg.content))
		}

		lastIndex = seg.end
	}

	// Add remaining plain text
	if lastIndex < len(text) {
		plainText := text[lastIndex:]
		if plainText != "" {
			result.WriteString(theme.ApplyText(plainText))
		}
	}

	// If no segments were found, just color the whole text
	if len(segments) == 0 {
		return theme.ApplyText(text)
	}

	return result.String()
}

// renderCodeBlock renders a code block with syntax highlighting
func (mr *MarkdownRenderer) renderCodeBlock(code, lang string) string {
	theme := GetCurrentTheme()
	var result strings.Builder

	// Header with language
	if lang != "" {
		result.WriteString(theme.ApplyHeader("┌─ " + lang + " ─\n"))
	} else {
		result.WriteString(theme.ApplyHeader("┌─ code ─\n"))
	}

	// Code content
	lines := strings.Split(code, "\n")
	for _, line := range lines {
		result.WriteString(theme.ApplyCode("│ " + line + "\n"))
	}

	// Footer
	result.WriteString(theme.ApplyHeader("└─────────"))

	return result.String()
}
