package chat

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/fatih/color"
)

// InputHandler handles enhanced input with autocomplete and history
type InputHandler struct {
	history      []string
	historyIndex int
	commands     map[string]*ChatCommand
	maxHistory   int
}

// NewInputHandler creates a new input handler
func NewInputHandler(commands map[string]*ChatCommand) *InputHandler {
	return &InputHandler{
		history:      make([]string, 0),
		historyIndex: -1,
		commands:     commands,
		maxHistory:   100,
	}
}

// ReadLine reads a line with autocomplete and history support
func (h *InputHandler) ReadLine(prompt string) (string, error) {
	// Check if we're in a terminal
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		// Fallback to simple input
		fmt.Print(prompt)
		reader := bufio.NewReader(os.Stdin)
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(line), nil
	}

	// Print prompt
	fmt.Print(prompt)

	var input strings.Builder
	var suggestions []string
	showingSuggestions := false

	// Set terminal to raw mode
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		// Fallback to simple input
		reader := bufio.NewReader(os.Stdin)
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(line), nil
	}
	defer func() { _ = term.Restore(int(os.Stdin.Fd()), oldState) }()

	buf := make([]byte, 3)
	historyPos := len(h.history)

	for {
		n, err := os.Stdin.Read(buf)
		if err != nil {
			return "", err
		}

		if n == 0 {
			continue
		}

		// Handle escape sequences (arrow keys, etc.)
		if buf[0] == 27 && n >= 3 {
			if buf[1] == 91 {
				switch buf[2] {
				case 65: // Up arrow
					if historyPos > 0 {
						historyPos--
						// Clear current line
						h.clearLine(&input)
						// Set input to history item
						input.Reset()
						input.WriteString(h.history[historyPos])
						fmt.Print("\r" + prompt + input.String())
					}
					continue
				case 66: // Down arrow
					if historyPos < len(h.history)-1 {
						historyPos++
						// Clear current line
						h.clearLine(&input)
						// Set input to history item
						input.Reset()
						input.WriteString(h.history[historyPos])
						fmt.Print("\r" + prompt + input.String())
					} else if historyPos == len(h.history)-1 {
						historyPos = len(h.history)
						// Clear current line
						h.clearLine(&input)
						input.Reset()
						fmt.Print("\r" + prompt)
					}
					continue
				}
			}
			continue
		}

		// Handle regular characters
		switch buf[0] {
		case 3: // Ctrl+C
			fmt.Println()
			return "", fmt.Errorf("interrupted")
		case 4: // Ctrl+D (EOF)
			if input.Len() == 0 {
				fmt.Println()
				return "exit", nil
			}
		case 9: // Tab - autocomplete
			currentInput := input.String()
			if strings.HasPrefix(currentInput, "/") {
				suggestions = h.getCommandSuggestions(currentInput)
				if len(suggestions) == 1 {
					// Single match - autocomplete
					h.clearLine(&input)
					input.Reset()
					input.WriteString(suggestions[0])
					fmt.Print("\r" + prompt + input.String())
					showingSuggestions = false
				} else if len(suggestions) > 1 {
					// Multiple matches - show suggestions
					if !showingSuggestions {
						fmt.Println()
						h.printSuggestions(suggestions)
						fmt.Print(prompt + input.String())
						showingSuggestions = true
					}
				}
			}
			continue
		case 13, 10: // Enter
			fmt.Println()
			result := input.String()
			if result != "" {
				h.addToHistory(result)
			}
			return result, nil
		case 127, 8: // Backspace
			if input.Len() > 0 {
				str := input.String()
				input.Reset()
				input.WriteString(str[:len(str)-1])
				fmt.Print("\b \b")
				showingSuggestions = false

				// Show suggestions if typing command
				if strings.HasPrefix(input.String(), "/") {
					suggestions = h.getCommandSuggestions(input.String())
					if len(suggestions) > 0 && len(suggestions) <= 5 {
						h.showInlineSuggestions(input.String(), suggestions)
					}
				}
			}
			continue
		default:
			if buf[0] >= 32 && buf[0] < 127 { // Printable characters
				input.WriteByte(buf[0])
				fmt.Printf("%c", buf[0])
				showingSuggestions = false

				// Show suggestions if typing command
				if strings.HasPrefix(input.String(), "/") {
					suggestions = h.getCommandSuggestions(input.String())
					if len(suggestions) > 0 && len(suggestions) <= 5 {
						h.showInlineSuggestions(input.String(), suggestions)
					}
				}
			}
		}
	}
}

// getCommandSuggestions returns command suggestions based on input
func (h *InputHandler) getCommandSuggestions(input string) []string {
	input = strings.TrimPrefix(input, "/")
	input = strings.ToLower(input)

	var suggestions []string

	for cmdName, cmd := range h.commands {
		// Check command name
		if strings.HasPrefix(cmdName, input) {
			suggestions = append(suggestions, "/"+cmdName)
		}

		// Check aliases
		for _, alias := range cmd.Aliases {
			if strings.HasPrefix(alias, input) {
				suggestions = append(suggestions, "/"+alias)
			}
		}
	}

	return suggestions
}

// printSuggestions prints command suggestions
func (h *InputHandler) printSuggestions(suggestions []string) {
	fmt.Println(color.YellowString("Suggestions:"))
	for _, suggestion := range suggestions {
		if cmd, exists := h.commands[strings.TrimPrefix(suggestion, "/")]; exists {
			fmt.Printf("  %s - %s\n", color.CyanString(suggestion), cmd.Description)
		} else {
			fmt.Printf("  %s\n", color.CyanString(suggestion))
		}
	}
}

// showInlineSuggestions shows suggestions inline (grayed out)
func (h *InputHandler) showInlineSuggestions(input string, suggestions []string) {
	if len(suggestions) == 0 {
		return
	}

	// Save cursor position
	fmt.Print("\033[s")

	// Print suggestion in gray
	suggestion := suggestions[0]
	remaining := strings.TrimPrefix(suggestion, input)
	fmt.Print(color.New(color.FgHiBlack).Sprint(remaining))

	// Restore cursor position
	fmt.Print("\033[u")
}

// clearLine clears the current input line
func (h *InputHandler) clearLine(input *strings.Builder) {
	// Move cursor to beginning and clear line
	fmt.Print("\r\033[K")
}

// addToHistory adds a command to history
func (h *InputHandler) addToHistory(cmd string) {
	// Don't add empty commands or duplicates of the last command
	if cmd == "" || (len(h.history) > 0 && h.history[len(h.history)-1] == cmd) {
		return
	}

	h.history = append(h.history, cmd)

	// Limit history size
	if len(h.history) > h.maxHistory {
		h.history = h.history[1:]
	}
}

// GetHistory returns the command history
func (h *InputHandler) GetHistory() []string {
	return h.history
}

// ClearHistory clears the command history
func (h *InputHandler) ClearHistory() {
	h.history = make([]string, 0)
}
