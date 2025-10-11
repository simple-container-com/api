package analysis

import (
	"os"
	"regexp"
	"strings"
)

// ComplexityAnalyzer analyzes code complexity metrics
type ComplexityAnalyzer struct{}

// NewComplexityAnalyzer creates a new complexity analyzer
func NewComplexityAnalyzer() *ComplexityAnalyzer {
	return &ComplexityAnalyzer{}
}

// AnalyzeFile analyzes complexity metrics for a single file
func (ca *ComplexityAnalyzer) AnalyzeFile(filePath, language string) (*CodeComplexity, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	text := string(content)
	lines := strings.Split(text, "\n")

	complexity := &CodeComplexity{
		LinesOfCode: ca.countLinesOfCode(lines, language),
	}

	// Language-specific analysis
	switch language {
	case "javascript", "typescript":
		ca.analyzeJavaScript(text, complexity)
	case "python":
		ca.analyzePython(text, complexity)
	case "go":
		ca.analyzeGo(text, complexity)
	case "java":
		ca.analyzeJava(text, complexity)
	default:
		ca.analyzeGeneric(text, complexity)
	}

	// Calculate overall complexity level
	complexity.ComplexityLevel = ca.calculateComplexityLevel(complexity)

	return complexity, nil
}

// countLinesOfCode counts non-empty, non-comment lines
func (ca *ComplexityAnalyzer) countLinesOfCode(lines []string, language string) int {
	loc := 0
	inBlockComment := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// Handle block comments based on language
		switch language {
		case "javascript", "typescript", "java", "go", "c", "cpp", "cs":
			if strings.Contains(trimmed, "/*") {
				inBlockComment = true
			}
			if inBlockComment {
				if strings.Contains(trimmed, "*/") {
					inBlockComment = false
				}
				continue
			}
			if strings.HasPrefix(trimmed, "//") {
				continue
			}
		case "python":
			if strings.HasPrefix(trimmed, "#") {
				continue
			}
			// Python docstrings
			if strings.HasPrefix(trimmed, `"""`) || strings.HasPrefix(trimmed, `'''`) {
				continue
			}
		}

		loc++
	}

	return loc
}

// analyzeJavaScript analyzes JavaScript/TypeScript specific metrics
func (ca *ComplexityAnalyzer) analyzeJavaScript(text string, complexity *CodeComplexity) {
	// Function patterns
	functionPatterns := []*regexp.Regexp{
		regexp.MustCompile(`function\s+\w+\s*\(`),
		regexp.MustCompile(`\w+\s*:\s*function\s*\(`),
		regexp.MustCompile(`\w+\s*=>\s*`),
		regexp.MustCompile(`\w+\s*=\s*function\s*\(`),
		regexp.MustCompile(`async\s+function\s+\w+\s*\(`),
	}

	for _, pattern := range functionPatterns {
		complexity.FunctionCount += len(pattern.FindAllString(text, -1))
	}

	// Class pattern
	classPattern := regexp.MustCompile(`class\s+\w+`)
	complexity.ClassCount = len(classPattern.FindAllString(text, -1))

	// Import patterns
	importPatterns := []*regexp.Regexp{
		regexp.MustCompile(`import\s+.*from\s+['"]`),
		regexp.MustCompile(`require\s*\(\s*['"]`),
		regexp.MustCompile(`import\s*\(\s*['"]`),
	}

	for _, pattern := range importPatterns {
		complexity.ImportCount += len(pattern.FindAllString(text, -1))
	}

	// Cyclomatic complexity indicators
	cyclomaticPatterns := []*regexp.Regexp{
		regexp.MustCompile(`\bif\s*\(`),
		regexp.MustCompile(`\belse\s+if\s*\(`),
		regexp.MustCompile(`\bwhile\s*\(`),
		regexp.MustCompile(`\bfor\s*\(`),
		regexp.MustCompile(`\bswitch\s*\(`),
		regexp.MustCompile(`\bcase\s+`),
		regexp.MustCompile(`\bcatch\s*\(`),
		regexp.MustCompile(`\?\s*.*\s*:`), // ternary operator
	}

	complexity.CyclomaticScore = 1 // Base complexity
	for _, pattern := range cyclomaticPatterns {
		complexity.CyclomaticScore += len(pattern.FindAllString(text, -1))
	}

	// Comment ratio
	commentLines := len(regexp.MustCompile(`//.*`).FindAllString(text, -1))
	blockComments := len(regexp.MustCompile(`/\*[\s\S]*?\*/`).FindAllString(text, -1))
	totalLines := len(strings.Split(text, "\n"))
	if totalLines > 0 {
		complexity.CommentRatio = float32(commentLines+blockComments) / float32(totalLines)
	}
}

// analyzePython analyzes Python specific metrics
func (ca *ComplexityAnalyzer) analyzePython(text string, complexity *CodeComplexity) {
	// Function patterns
	functionPattern := regexp.MustCompile(`def\s+\w+\s*\(`)
	complexity.FunctionCount = len(functionPattern.FindAllString(text, -1))

	// Class pattern
	classPattern := regexp.MustCompile(`class\s+\w+`)
	complexity.ClassCount = len(classPattern.FindAllString(text, -1))

	// Import patterns
	importPatterns := []*regexp.Regexp{
		regexp.MustCompile(`^import\s+\w+`),
		regexp.MustCompile(`^from\s+\w+\s+import`),
	}

	for _, pattern := range importPatterns {
		complexity.ImportCount += len(pattern.FindAllString(text, -1))
	}

	// Cyclomatic complexity indicators
	cyclomaticPatterns := []*regexp.Regexp{
		regexp.MustCompile(`\bif\s+`),
		regexp.MustCompile(`\belif\s+`),
		regexp.MustCompile(`\bwhile\s+`),
		regexp.MustCompile(`\bfor\s+`),
		regexp.MustCompile(`\bexcept\s*:`),
		regexp.MustCompile(`\band\s+`),
		regexp.MustCompile(`\bor\s+`),
	}

	complexity.CyclomaticScore = 1 // Base complexity
	for _, pattern := range cyclomaticPatterns {
		complexity.CyclomaticScore += len(pattern.FindAllString(text, -1))
	}

	// Comment ratio
	commentLines := len(regexp.MustCompile(`#.*`).FindAllString(text, -1))
	totalLines := len(strings.Split(text, "\n"))
	if totalLines > 0 {
		complexity.CommentRatio = float32(commentLines) / float32(totalLines)
	}
}

// analyzeGo analyzes Go specific metrics
func (ca *ComplexityAnalyzer) analyzeGo(text string, complexity *CodeComplexity) {
	// Function patterns
	functionPattern := regexp.MustCompile(`func\s+(\w+\s*)?\w+\s*\(`)
	complexity.FunctionCount = len(functionPattern.FindAllString(text, -1))

	// Struct pattern (Go's equivalent to classes)
	structPattern := regexp.MustCompile(`type\s+\w+\s+struct`)
	complexity.ClassCount = len(structPattern.FindAllString(text, -1))

	// Import patterns
	importPattern := regexp.MustCompile(`import\s+["']`)
	importBlockPattern := regexp.MustCompile(`import\s+\([\s\S]*?\)`)
	complexity.ImportCount = len(importPattern.FindAllString(text, -1))

	// Count imports in import blocks
	for _, block := range importBlockPattern.FindAllString(text, -1) {
		lines := strings.Split(block, "\n")
		for _, line := range lines {
			if strings.Contains(line, `"`) {
				complexity.ImportCount++
			}
		}
	}

	// Cyclomatic complexity indicators
	cyclomaticPatterns := []*regexp.Regexp{
		regexp.MustCompile(`\bif\s+`),
		regexp.MustCompile(`\belse\s+if\s+`),
		regexp.MustCompile(`\bfor\s+`),
		regexp.MustCompile(`\bswitch\s+`),
		regexp.MustCompile(`\bcase\s+`),
		regexp.MustCompile(`\bselect\s*{`),
		regexp.MustCompile(`&&`),
		regexp.MustCompile(`\|\|`),
	}

	complexity.CyclomaticScore = 1 // Base complexity
	for _, pattern := range cyclomaticPatterns {
		complexity.CyclomaticScore += len(pattern.FindAllString(text, -1))
	}

	// Comment ratio
	commentLines := len(regexp.MustCompile(`//.*`).FindAllString(text, -1))
	blockComments := len(regexp.MustCompile(`/\*[\s\S]*?\*/`).FindAllString(text, -1))
	totalLines := len(strings.Split(text, "\n"))
	if totalLines > 0 {
		complexity.CommentRatio = float32(commentLines+blockComments) / float32(totalLines)
	}
}

// analyzeJava analyzes Java specific metrics
func (ca *ComplexityAnalyzer) analyzeJava(text string, complexity *CodeComplexity) {
	// Function/method patterns
	methodPattern := regexp.MustCompile(`(public|private|protected|static).*\w+\s*\(`)
	complexity.FunctionCount = len(methodPattern.FindAllString(text, -1))

	// Class pattern
	classPattern := regexp.MustCompile(`(public|private)?\s*class\s+\w+`)
	complexity.ClassCount = len(classPattern.FindAllString(text, -1))

	// Import pattern
	importPattern := regexp.MustCompile(`import\s+[\w.]+;`)
	complexity.ImportCount = len(importPattern.FindAllString(text, -1))

	// Cyclomatic complexity indicators
	cyclomaticPatterns := []*regexp.Regexp{
		regexp.MustCompile(`\bif\s*\(`),
		regexp.MustCompile(`\belse\s+if\s*\(`),
		regexp.MustCompile(`\bwhile\s*\(`),
		regexp.MustCompile(`\bfor\s*\(`),
		regexp.MustCompile(`\bswitch\s*\(`),
		regexp.MustCompile(`\bcase\s+`),
		regexp.MustCompile(`\bcatch\s*\(`),
		regexp.MustCompile(`&&`),
		regexp.MustCompile(`\|\|`),
	}

	complexity.CyclomaticScore = 1 // Base complexity
	for _, pattern := range cyclomaticPatterns {
		complexity.CyclomaticScore += len(pattern.FindAllString(text, -1))
	}

	// Comment ratio
	commentLines := len(regexp.MustCompile(`//.*`).FindAllString(text, -1))
	blockComments := len(regexp.MustCompile(`/\*[\s\S]*?\*/`).FindAllString(text, -1))
	totalLines := len(strings.Split(text, "\n"))
	if totalLines > 0 {
		complexity.CommentRatio = float32(commentLines+blockComments) / float32(totalLines)
	}
}

// analyzeGeneric provides basic analysis for unsupported languages
func (ca *ComplexityAnalyzer) analyzeGeneric(text string, complexity *CodeComplexity) {
	// Basic patterns that might work across languages
	lines := strings.Split(text, "\n")

	// Estimate functions by looking for common patterns
	functionIndicators := []string{"function", "def ", "func ", "method", "sub ", "procedure"}
	for _, line := range lines {
		lower := strings.ToLower(strings.TrimSpace(line))
		for _, indicator := range functionIndicators {
			if strings.Contains(lower, indicator) {
				complexity.FunctionCount++
				break
			}
		}
	}

	// Basic cyclomatic complexity
	controlStructures := []string{"if", "else", "while", "for", "switch", "case", "catch"}
	complexity.CyclomaticScore = 1
	for _, line := range lines {
		lower := strings.ToLower(strings.TrimSpace(line))
		for _, structure := range controlStructures {
			if strings.Contains(lower, structure) {
				complexity.CyclomaticScore++
			}
		}
	}
}

// calculateComplexityLevel determines the overall complexity level
func (ca *ComplexityAnalyzer) calculateComplexityLevel(complexity *CodeComplexity) string {
	score := 0

	// Lines of code scoring
	if complexity.LinesOfCode > 1000 {
		score += 3
	} else if complexity.LinesOfCode > 500 {
		score += 2
	} else if complexity.LinesOfCode > 200 {
		score += 1
	}

	// Cyclomatic complexity scoring
	if complexity.CyclomaticScore > 50 {
		score += 3
	} else if complexity.CyclomaticScore > 20 {
		score += 2
	} else if complexity.CyclomaticScore > 10 {
		score += 1
	}

	// Function count scoring
	if complexity.FunctionCount > 50 {
		score += 2
	} else if complexity.FunctionCount > 20 {
		score += 1
	}

	// Comment ratio scoring (low comments increase complexity)
	if complexity.CommentRatio < 0.1 {
		score += 2
	} else if complexity.CommentRatio < 0.2 {
		score += 1
	}

	// Determine level based on total score
	switch {
	case score >= 8:
		return "very_high"
	case score >= 5:
		return "high"
	case score >= 3:
		return "medium"
	default:
		return "low"
	}
}
