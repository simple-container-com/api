package chat

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
	"golang.org/x/term"
)

// InputHandler handles enhanced input with autocomplete and history
type InputHandler struct {
	history           []string
	historyIndex      int
	commands          map[string]*ChatCommand
	maxHistory        int
	currentInput      string
	cursorPos         int
	menuVisible       bool
	menuLines         int
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
	var selectedSuggestionIndex int = -1

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
						// Clear menu first
						h.hideMenu()
						suggestions = nil
						selectedSuggestionIndex = -1
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
						// Clear menu first
						h.hideMenu()
						suggestions = nil
						selectedSuggestionIndex = -1
						// Clear current line
						h.clearLine(&input)
						// Set input to history item
						input.Reset()
						input.WriteString(h.history[historyPos])
						fmt.Print("\r" + prompt + input.String())
					} else if historyPos == len(h.history)-1 {
						historyPos = len(h.history)
						// Clear menu first
						h.hideMenu()
						suggestions = nil
						selectedSuggestionIndex = -1
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
			h.hideMenu()
			fmt.Println()
			return "", fmt.Errorf("interrupted")
		case 4: // Ctrl+D (EOF)
			if input.Len() == 0 {
				h.hideMenu()
				fmt.Println()
				return "exit", nil
			}
		case 9: // Tab - autocomplete/cycle
			currentInput := input.String()
			if strings.HasPrefix(currentInput, "/") {
				newSuggestions := h.getCommandSuggestions(currentInput)

				if len(newSuggestions) == 0 {
					// No suggestions - hide menu
					h.hideMenu()
					suggestions = nil
					selectedSuggestionIndex = -1
					continue
				}

				if len(newSuggestions) == 1 {
					// Single match - autocomplete and add space
					if newSuggestions[0] == currentInput {
						// Already complete - do nothing
						continue
					}
					// Clear menu if visible
					h.hideMenu()
					// Autocomplete with space at the end
					completed := newSuggestions[0] + " "
					fmt.Print("\r\033[K" + prompt + completed)
					input.Reset()
					input.WriteString(completed)
					suggestions = nil
					selectedSuggestionIndex = -1
					continue
				}

				// Multiple suggestions
				if !equalSlices(suggestions, newSuggestions) {
					// New suggestions - reset selection
					suggestions = newSuggestions
					selectedSuggestionIndex = 0

					// Update menu
					h.updateMenu(suggestions, selectedSuggestionIndex, prompt, currentInput)
				} else {
					// Same suggestions - cycle selection
					selectedSuggestionIndex++
					if selectedSuggestionIndex >= len(suggestions) {
						selectedSuggestionIndex = 0
					}

					// Update menu with new selection
					h.updateMenu(suggestions, selectedSuggestionIndex, prompt, currentInput)
				}
				continue
			}
			// For non-command input, Tab does nothing (ignore it)
			continue
		case 13, 10: // Enter
			// If menu is shown, accept selected suggestion
			if h.menuVisible && selectedSuggestionIndex >= 0 && len(suggestions) > 0 {
				selected := suggestions[selectedSuggestionIndex]
				// Clear menu
				h.hideMenu()
				// Add space after selection
				completed := selected + " "
				fmt.Print("\r\033[K" + prompt + completed)
				input.Reset()
				input.WriteString(completed)
				suggestions = nil
				selectedSuggestionIndex = -1
				continue
			}

			// Clear menu if visible
			h.hideMenu()

			// Normal enter - execute command
			fmt.Println()
			result := input.String()
			if result != "" {
				h.addToHistory(result)
			}
			return result, nil
		case 127, 8: // Backspace
			if input.Len() > 0 {
				// Clear menu if shown
				h.hideMenu()

				str := input.String()
				input.Reset()
				input.WriteString(str[:len(str)-1])
				fmt.Print("\b \b")
				// Reset suggestions when user types
				suggestions = nil
				selectedSuggestionIndex = -1
			}
			continue
		default:
			if buf[0] >= 32 && buf[0] < 127 { // Printable characters
				// Clear menu if shown
				h.hideMenu()

				input.WriteByte(buf[0])
				fmt.Printf("%c", buf[0])
				// Reset suggestions when user types
				suggestions = nil
				selectedSuggestionIndex = -1
			}
		}
	}
}

// equalSlices compares two string slices
func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// getCommandSuggestions returns command suggestions based on input
func (h *InputHandler) getCommandSuggestions(input string) []string {
	input = strings.TrimPrefix(input, "/")

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
}

// printSuggestionsCompact prints suggestions in compact format (like bash)
func (h *InputHandler) printSuggestionsCompact(suggestions []string) {
	// Extract just the last part (subcommand) for compact display
	var parts []string
	for _, s := range suggestions {
		// Get the part after last space
		idx := strings.LastIndex(s, " ")
		if idx != -1 {
			parts = append(parts, s[idx+1:])
		} else {
			parts = append(parts, strings.TrimPrefix(s, "/"))
		}
	}
	
	// Print in one line, separated by spaces
	os.Stdout.WriteString(color.YellowString("Options: "))
	for i, part := range parts {
		if i > 0 {
			os.Stdout.WriteString("  ")
		}
		os.Stdout.WriteString(color.CyanString(part))
	}
	os.Stdout.WriteString("\r\n")
}

// updateMenu updates the menu display - erases old one if needed and draws new one
func (h *InputHandler) updateMenu(suggestions []string, selectedIndex int, prompt, currentInput string) {
	var output strings.Builder

	// If menu exists, we update in place
	// If menu doesn't exist, we need to create space first
	if !h.menuVisible {
		// First time showing menu
		// Step 1: Clear current line and print input
		output.WriteString("\r\033[K")
		output.WriteString(prompt + currentInput)

		// Step 2: Create space for menu by printing newlines
		for i := 0; i < len(suggestions); i++ {
			output.WriteString("\n")
		}

		// Step 3: Go back up to input line
		for i := 0; i < len(suggestions); i++ {
			output.WriteString("\033[A")
		}

		// Mark that we now have menu space
		h.menuLines = len(suggestions)
	}

	// Now update the menu in-place without creating new lines
	// Save cursor position (at end of input)
	output.WriteString("\033[s")

	// Draw each menu item by moving down from saved position
	for i, suggestion := range suggestions {
		// Extract display text
		idx := strings.LastIndex(suggestion, " ")
		var display string
		if idx != -1 {
			display = suggestion[idx+1:]
		} else {
			display = strings.TrimPrefix(suggestion, "/")
		}

		// Move down i+1 lines from saved position
		output.WriteString("\033[u")           // Restore to input line
		output.WriteString("\033[")
		output.WriteString(fmt.Sprintf("%d", i+1))
		output.WriteString("B")                // Move down
		output.WriteString("\r\033[K")         // Clear line

		// Print with or without highlight
		if i == selectedIndex {
			output.WriteString("\033[7m" + display + "\033[0m")
		} else {
			output.WriteString(display)
		}
	}

	// Restore cursor to end of input
	output.WriteString("\033[u")

	// Write everything at once
	fmt.Print(output.String())

	// Update state
	h.menuVisible = true
}

// hideMenu hides the menu if visible
func (h *InputHandler) hideMenu() {
	if !h.menuVisible || h.menuLines == 0 {
		return
	}

	var output strings.Builder

	// Save cursor position
	output.WriteString("\033[s")

	// Clear each menu line by moving down and clearing
	for i := 0; i < h.menuLines; i++ {
		output.WriteString("\033[u")  // Restore position
		output.WriteString("\033[")
		output.WriteString(fmt.Sprintf("%d", i+1))
		output.WriteString("B")       // Move down
		output.WriteString("\r\033[K") // Clear line
	}

	// Restore cursor
	output.WriteString("\033[u")

	// Write everything at once
	fmt.Print(output.String())

	h.menuVisible = false
	h.menuLines = 0
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
