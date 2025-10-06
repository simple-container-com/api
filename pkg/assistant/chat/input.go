package chat

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"unsafe"

	"github.com/peterh/liner"
)

// InputHandler handles enhanced input with autocomplete and history
type InputHandler struct {
	history    []string
	commands   map[string]*ChatCommand
	maxHistory int
	liner      *liner.State
}

// NewInputHandler creates a new input handler
func NewInputHandler(commands map[string]*ChatCommand) *InputHandler {
	return &InputHandler{
		history:    make([]string, 0),
		commands:   commands,
		maxHistory: 100,
	}
}

// ReadLine reads a line with autocomplete and history support
func (h *InputHandler) ReadLine(promptText string) (string, error) {
	// Initialize liner if not done
	if h.liner == nil {
		h.liner = liner.NewLiner()
		h.liner.SetCtrlCAborts(true)

		// Set tab completion to circular mode (cycle through options)
		h.liner.SetTabCompletionStyle(liner.TabCircular)

		// Set completer function
		h.liner.SetCompleter(func(line string) []string {
			suggestions := h.getCommandSuggestions(line)
			// Return nil if no suggestions to avoid showing empty menu
			if len(suggestions) == 0 {
				return nil
			}
			return suggestions
		})

		// Load history
		for _, item := range h.history {
			h.liner.AppendHistory(item)
		}
	}

	// Strip ANSI color codes from prompt for liner
	// liner doesn't handle ANSI codes well in prompts
	cleanPrompt := stripANSI(promptText)

	// liner also doesn't like newlines or emojis in prompt
	// Use a simple prompt instead
	cleanPrompt = strings.ReplaceAll(cleanPrompt, "\n", "")
	// If prompt contains non-ASCII (emoji), use a simple prompt
	simplePrompt := "> "
	for _, r := range cleanPrompt {
		if r > 127 {
			cleanPrompt = simplePrompt
			break
		}
	}

	line, err := h.liner.Prompt(cleanPrompt)
	if err != nil {
		if err == liner.ErrPromptAborted {
			return "", fmt.Errorf("interrupted")
		}
		if err == io.EOF {
			return "exit", nil
		}
		return "", err
	}

	line = strings.TrimSpace(line)
	if line != "" {
		h.addToHistory(line)
		h.liner.AppendHistory(line)
	}

	return line, nil
}

// stripANSI removes ANSI escape codes from a string
func stripANSI(str string) string {
	result := str
	for strings.Contains(result, "\x1b[") {
		start := strings.Index(result, "\x1b[")
		if start == -1 {
			break
		}
		end := strings.Index(result[start:], "m")
		if end == -1 {
			break
		}
		result = result[:start] + result[start+end+1:]
	}
	return result
}

// getCommandSuggestions returns command suggestions based on input
func (h *InputHandler) getCommandSuggestions(input string) []string {
	if !strings.HasPrefix(input, "/") {
		return nil
	}

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
		"model":    {"list", "switch", "info"},
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
	if h.liner != nil {
		// Clear liner history too
		h.liner.ClearHistory()
	}
}

// ReadSimple reads a simple line without autocomplete (for menus, prompts, etc)
func (h *InputHandler) ReadSimple(promptText string) (string, error) {
	// Temporarily close liner to release stdin
	if h.liner != nil {
		h.liner.Close()
		h.liner = nil
	}

	// Reset terminal to sane state using syscall
	// Get terminal fd
	fd := int(syscall.Stdin)

	// Get current terminal settings
	var termios syscall.Termios
	if _, _, err := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), syscall.TIOCGETA, uintptr(unsafe.Pointer(&termios))); err != 0 {
		// Fallback: try using stty command
		cmd := exec.Command("stty", "sane", "-F", "/dev/tty")
		_ = cmd.Run()
	} else {
		// Enable canonical mode (ICANON) and echo (ECHO)
		termios.Lflag |= syscall.ICANON | syscall.ECHO | syscall.ECHOE | syscall.ECHOK | syscall.ECHOCTL | syscall.ECHOKE
		// Enable ICRNL (translate CR to NL on input)
		termios.Iflag |= syscall.ICRNL
		// Enable ONLCR (translate NL to CR-NL on output)
		termios.Oflag |= syscall.ONLCR
		// Set the terminal attributes
		syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), syscall.TIOCSETA, uintptr(unsafe.Pointer(&termios)))
	}

	// Print prompt
	fmt.Print(promptText)

	// Now use normal buffered reading
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}

	// Clean the input
	return strings.TrimSpace(line), nil
}

// Close closes the liner instance
func (h *InputHandler) Close() error {
	if h.liner != nil {
		return h.liner.Close()
	}
	return nil
}
