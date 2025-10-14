package chat

import (
	"fmt"

	"github.com/fatih/color"
)

// Theme represents a color theme for the chat interface
type Theme struct {
	Name          string
	Description   string
	TextColor     *color.Color
	CodeColor     *color.Color
	HeaderColor   *color.Color
	EmphasisColor *color.Color
}

var (
	// Available themes
	themes = map[string]*Theme{
		"default": {
			Name:          "default",
			Description:   "Default theme - cyan text, yellow code, magenta headers",
			TextColor:     color.New(color.FgHiCyan),
			CodeColor:     color.New(color.FgHiYellow),
			HeaderColor:   color.New(color.FgHiMagenta, color.Bold),
			EmphasisColor: color.New(color.FgHiGreen),
		},
		"ocean": {
			Name:          "ocean",
			Description:   "Ocean theme - blue text, cyan code, deep blue headers",
			TextColor:     color.New(color.FgCyan),
			CodeColor:     color.New(color.FgHiYellow),
			HeaderColor:   color.New(color.FgHiBlue, color.Bold),
			EmphasisColor: color.New(color.FgHiCyan),
		},
		"sunset": {
			Name:          "sunset",
			Description:   "Sunset theme - yellow text, orange code, red headers",
			TextColor:     color.New(color.FgHiYellow),
			CodeColor:     color.New(color.FgYellow),
			HeaderColor:   color.New(color.FgHiRed, color.Bold),
			EmphasisColor: color.New(color.FgRed),
		},
		"forest": {
			Name:          "forest",
			Description:   "Forest theme - green text, yellow code, bright green headers",
			TextColor:     color.New(color.FgGreen),
			CodeColor:     color.New(color.FgHiYellow),
			HeaderColor:   color.New(color.FgHiGreen, color.Bold),
			EmphasisColor: color.New(color.FgHiCyan),
		},
		"purple": {
			Name:          "purple",
			Description:   "Purple theme - magenta text, yellow code, bright magenta headers",
			TextColor:     color.New(color.FgMagenta),
			CodeColor:     color.New(color.FgHiYellow),
			HeaderColor:   color.New(color.FgHiMagenta, color.Bold),
			EmphasisColor: color.New(color.FgHiMagenta),
		},
		"matrix": {
			Name:          "matrix",
			Description:   "Matrix theme - green text, bright yellow code, bright green headers",
			TextColor:     color.New(color.FgHiGreen),
			CodeColor:     color.New(color.FgHiYellow, color.Bold),
			HeaderColor:   color.New(color.FgHiGreen, color.Bold, color.Underline),
			EmphasisColor: color.New(color.FgGreen, color.Bold),
		},
		"fire": {
			Name:          "fire",
			Description:   "Fire theme - red text, bright yellow code, bright red headers",
			TextColor:     color.New(color.FgRed),
			CodeColor:     color.New(color.FgHiYellow, color.Bold),
			HeaderColor:   color.New(color.FgHiRed, color.Bold),
			EmphasisColor: color.New(color.FgHiYellow),
		},
		"monochrome": {
			Name:          "monochrome",
			Description:   "Monochrome theme - white text, bright white code",
			TextColor:     color.New(color.FgWhite),
			CodeColor:     color.New(color.FgHiWhite, color.Bold),
			HeaderColor:   color.New(color.FgHiWhite, color.Bold, color.Underline),
			EmphasisColor: color.New(color.FgWhite, color.Bold),
		},
	}

	// Current active theme
	currentTheme = themes["default"]
)

// GetTheme returns the theme by name
func GetTheme(name string) (*Theme, error) {
	theme, exists := themes[name]
	if !exists {
		return nil, fmt.Errorf("theme '%s' not found", name)
	}
	return theme, nil
}

// SetCurrentTheme sets the current active theme
func SetCurrentTheme(name string) error {
	theme, err := GetTheme(name)
	if err != nil {
		return err
	}
	currentTheme = theme
	return nil
}

// GetCurrentTheme returns the current active theme
func GetCurrentTheme() *Theme {
	return currentTheme
}

// ListThemes returns all available themes
func ListThemes() []*Theme {
	themeList := make([]*Theme, 0, len(themes))
	for _, theme := range themes {
		themeList = append(themeList, theme)
	}
	return themeList
}

// ApplyTheme applies colors from the theme
func (t *Theme) ApplyText(text string) string {
	return t.TextColor.Sprint(text)
}

func (t *Theme) ApplyCode(text string) string {
	return t.CodeColor.Sprint(text)
}

func (t *Theme) ApplyHeader(text string) string {
	return t.HeaderColor.Sprint(text)
}

func (t *Theme) ApplyEmphasis(text string) string {
	return t.EmphasisColor.Sprint(text)
}
