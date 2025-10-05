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
	currentInput string
	cursorPos    int
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
				if len(suggestions) == 0 {
					// No suggestions - do nothing
					continue
				}

				if len(suggestions) == 1 {
					// Single match - check if it's the same as current input
					if suggestions[0] == currentInput {
						// Already complete - do nothing
						continue
					}
					// Autocomplete - use ANSI escape to clear line
					// \r - go to start, \033[K - clear from cursor to end of line
					output := "\r\033[K" + prompt + suggestions[0]
					os.Stdout.WriteString(output)
					input.Reset()
					input.WriteString(suggestions[0])
					continue
				} else if len(suggestions) > 1 {
					// Multiple matches - show suggestions
					// Print directly in raw mode to avoid extra newlines
					os.Stdout.WriteString("\r\n")
					h.printSuggestionsRaw(suggestions)
					os.Stdout.WriteString("\r\n" + prompt + input.String())
					continue
				}
			}
			// For non-command input, Tab does nothing (ignore it)
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

	// Check if input contains a space (subcommand)
	spaceIndex := strings.Index(input, " ")

	if spaceIndex != -1 {
		// Has space - extract command and subcommand
		cmdName := input[:spaceIndex]
		subCmd := strings.TrimSpace(input[spaceIndex+1:])
		return h.getSubcommandSuggestions(cmdName, subCmd)
	}

	// Check if input is an exact command match - show subcommands
	inputLower := strings.ToLower(input)
	if _, exists := h.commands[inputLower]; exists {
		// Exact match - show subcommands with empty subcommand
		return h.getSubcommandSuggestions(inputLower, "")
	}

	// Command suggestions (partial match)
	var suggestions []string

	for cmdName, cmd := range h.commands {
		// Check command name
		if strings.HasPrefix(cmdName, inputLower) {
			suggestions = append(suggestions, "/"+cmdName)
		}

		// Check aliases
		for _, alias := range cmd.Aliases {
			if strings.HasPrefix(alias, inputLower) {
				suggestions = append(suggestions, "/"+alias)
			}
		}
	}

	return suggestions
}

// getSubcommandSuggestions returns subcommand suggestions for a command
func (h *InputHandler) getSubcommandSuggestions(cmdName, subCmd string) []string {
	cmdName = strings.ToLower(cmdName)
	subCmd = strings.ToLower(subCmd)

	var suggestions []string

	// Define subcommands for each command
	subcommands := map[string][]string{
		"apikey":   {"set", "delete", "status"},
		"provider": {"list", "switch", "info"},
		"history":  {"clear"},
		"search":   {}, // search takes a query
		"help":     {}, // help can take command names
		"switch":   {"dev", "devops", "general"},
	}

	// Get subcommands for this command
	subs, exists := subcommands[cmdName]
	if !exists {
		return suggestions
	}

	// If subCmd is empty, show all subcommands
	if subCmd == "" {
		for _, sub := range subs {
			suggestions = append(suggestions, "/"+cmdName+" "+sub)
		}
	} else {
		// Check if subCmd is already a complete match
		isComplete := false
		for _, sub := range subs {
			if sub == subCmd {
				isComplete = true
				break
			}
		}

		// If already complete, don't show suggestions
		if isComplete {
			return suggestions
		}

		// Filter subcommands by prefix
		for _, sub := range subs {
			if strings.HasPrefix(sub, subCmd) {
				suggestions = append(suggestions, "/"+cmdName+" "+sub)
			}
		}
	}

	// For help command, suggest other command names
	if cmdName == "help" {
		if subCmd == "" {
			// Show all commands
			for name := range h.commands {
				suggestions = append(suggestions, "/help "+name)
			}
		} else {
			// Filter by prefix
			for name := range h.commands {
				if strings.HasPrefix(name, subCmd) {
					suggestions = append(suggestions, "/help "+name)
				}
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

// printSuggestionsRaw prints command suggestions in raw mode without extra newlines
func (h *InputHandler) printSuggestionsRaw(suggestions []string) {
	os.Stdout.WriteString(color.YellowString("Suggestions:") + "\r\n")
	for _, suggestion := range suggestions {
		if cmd, exists := h.commands[strings.TrimPrefix(suggestion, "/")]; exists {
			os.Stdout.WriteString("  " + color.CyanString(suggestion) + " - " + cmd.Description + "\r\n")
		} else {
			os.Stdout.WriteString("  " + color.CyanString(suggestion) + "\r\n")
		}
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
