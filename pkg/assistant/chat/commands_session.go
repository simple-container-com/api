package chat

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/simple-container-com/api/pkg/api/logger/color"
	"github.com/simple-container-com/api/pkg/assistant/config"
	"github.com/simple-container-com/api/pkg/assistant/llm/prompts"
)

// registerSessionCommands registers session and history management commands
func (c *ChatInterface) registerSessionCommands() {
	c.commands["history"] = &ChatCommand{
		Name:        "history",
		Description: "Show command history",
		Usage:       "/history [clear]",
		Handler:     c.handleHistory,
		Args: []CommandArg{
			{Name: "action", Type: "string", Required: false, Description: "Action: clear to clear history"},
		},
	}

	c.commands["sessions"] = &ChatCommand{
		Name:        "sessions",
		Description: "Manage chat sessions",
		Usage:       "/sessions [list|new|delete [<id>|all]|config <max>]",
		Handler:     c.handleSessions,
		Args: []CommandArg{
			{Name: "action", Type: "string", Required: false, Description: "Action: list, new, delete, or config"},
			{Name: "value", Type: "string", Required: false, Description: "Session ID, 'all' (for delete all), or max sessions count"},
		},
	}
}

// handleHistory shows or clears command history
func (c *ChatInterface) handleHistory(ctx context.Context, args []string, context *ConversationContext) (*CommandResult, error) {
	if len(args) > 0 && strings.ToLower(args[0]) == "clear" {
		c.inputHandler.ClearHistory()
		return &CommandResult{
			Success: true,
			Message: "✅ Command history cleared",
		}, nil
	}

	history := c.inputHandler.GetHistory()
	if len(history) == 0 {
		return &CommandResult{
			Success: true,
			Message: "No command history yet",
		}, nil
	}

	message := fmt.Sprintf("📜 Command History (%d commands):\n", len(history))

	// Show last 20 commands
	start := 0
	if len(history) > 20 {
		start = len(history) - 20
		message += fmt.Sprintf("\n(Showing last 20 of %d commands)\n", len(history))
	}

	for i := start; i < len(history); i++ {
		message += fmt.Sprintf("\n  %d. %s", i+1, history[i])
	}

	message += "\n\n💡 Tip: Use ↑/↓ arrow keys to navigate history, Tab for autocomplete"

	return &CommandResult{
		Success: true,
		Message: message,
	}, nil
}

// handleSessions handles session management commands
func (c *ChatInterface) handleSessions(ctx context.Context, args []string, context *ConversationContext) (*CommandResult, error) {
	if len(args) == 0 {
		// Show current session info
		currentSession := c.sessionManager.GetCurrentSession()
		if currentSession == nil {
			return &CommandResult{
				Success: false,
				Message: "No active session",
			}, nil
		}

		message := fmt.Sprintf("📋 Current Session: %s\n", currentSession.Title)
		message += fmt.Sprintf("   Session ID: %s\n", currentSession.ID)
		message += fmt.Sprintf("   Mode: %s\n", currentSession.Mode)
		message += fmt.Sprintf("   Project: %s\n", currentSession.ProjectPath)
		message += fmt.Sprintf("   Messages: %d\n", len(currentSession.ConversationHistory))
		message += fmt.Sprintf("   Started: %s\n", currentSession.StartedAt.Format("2006-01-02 15:04"))
		message += "\nUse '/sessions list' to see all sessions"

		return &CommandResult{
			Success: true,
			Message: message,
		}, nil
	}

	action := strings.ToLower(args[0])

	switch action {
	case "new":
		// Save current session before creating new one
		if currentSession := c.sessionManager.GetCurrentSession(); currentSession != nil {
			if err := c.sessionManager.SaveSession(currentSession); err != nil {
				fmt.Printf("%s Failed to save current session: %v\n", color.YellowString("⚠️"), err)
			}
		}

		// Create new session
		newSession := c.sessionManager.CreateNewSession(c.config.ProjectPath, c.config.Mode)

		// Save the new session first
		if err := c.sessionManager.SaveSession(newSession); err != nil {
			fmt.Printf("%s Failed to save new session: %v\n", color.YellowString("⚠️"), err)
		}

		// Cleanup old sessions if limit exceeded
		if err := c.sessionManager.CleanupOldSessions(); err != nil {
			fmt.Printf("%s Failed to cleanup old sessions: %v\n", color.YellowString("⚠️"), err)
		}

		// Clear conversation history for new session
		c.context.History = make([]Message, 0)
		c.context.SessionID = newSession.ID
		c.context.CreatedAt = time.Now()
		c.context.UpdatedAt = time.Now()

		// Clear input history
		c.inputHandler.ClearHistory()
		c.inputHandler.history = newSession.CommandHistory

		// Re-add system prompt with project context
		if c.context.ProjectInfo != nil {
			systemPrompt := prompts.GenerateContextualPrompt(c.config.Mode, c.context.ProjectInfo, c.context.Resources)
			c.addMessage("system", systemPrompt)
		}

		return &CommandResult{
			Success: true,
			Message: fmt.Sprintf("✨ Started new session: %s", newSession.Title),
		}, nil

	case "list":
		sessions, err := c.sessionManager.ListSessions()
		if err != nil {
			return &CommandResult{
				Success: false,
				Message: fmt.Sprintf("Failed to list sessions: %v", err),
			}, nil
		}

		if len(sessions) == 0 {
			return &CommandResult{
				Success: true,
				Message: "No saved sessions found",
			}, nil
		}

		message := fmt.Sprintf("📚 Saved Sessions (%d):\n\n", len(sessions))
		currentSession := c.sessionManager.GetCurrentSession()

		for i, session := range sessions {
			age := time.Since(session.LastUsedAt)
			ageStr := formatDuration(age)

			projectName := filepath.Base(session.ProjectPath)
			if projectName == "" || projectName == "." {
				projectName = "no project"
			}

			mark := ""
			if currentSession != nil && session.ID == currentSession.ID {
				mark = " ⭐ (current)"
			}

			message += fmt.Sprintf("%d. %s%s\n", i+1, session.Title, mark)
			message += fmt.Sprintf("   ID: %s | Mode: %s | Project: %s\n", session.ID, session.Mode, projectName)
			message += fmt.Sprintf("   %d messages | Last used: %s ago\n\n", len(session.ConversationHistory), ageStr)
		}

		maxSessions := c.sessionManager.GetMaxSessions()
		message += fmt.Sprintf("Max saved sessions: %d\n", maxSessions)
		message += "\nUse '/sessions delete <id>' to delete a session"
		message += "\nUse '/sessions config <max>' to change max sessions"

		return &CommandResult{
			Success: true,
			Message: message,
		}, nil

	case "delete":
		// If no session ID provided, show interactive selection menu
		if len(args) < 2 {
			return c.handleSessionDeleteInteractive()
		}

		// Check for "all" keyword
		if strings.ToLower(args[1]) == "all" {
			return c.handleSessionDeleteAll()
		}

		// Direct deletion with provided ID
		sessionID := args[1]
		if err := c.sessionManager.DeleteSession(sessionID); err != nil {
			return &CommandResult{
				Success: false,
				Message: fmt.Sprintf("Failed to delete session: %v", err),
			}, nil
		}

		return &CommandResult{
			Success: true,
			Message: fmt.Sprintf("✅ Session %s deleted successfully", sessionID),
		}, nil

	case "config":
		if len(args) < 2 {
			maxSessions := c.sessionManager.GetMaxSessions()
			return &CommandResult{
				Success: true,
				Message: fmt.Sprintf("Current max sessions: %d\nUse '/sessions config <number>' to change", maxSessions),
			}, nil
		}

		maxSessions, err := strconv.Atoi(args[1])
		if err != nil || maxSessions < 1 {
			return &CommandResult{
				Success: false,
				Message: "Invalid number. Please specify a positive integer.",
			}, nil
		}

		c.sessionManager.SetMaxSessions(maxSessions)

		// Save to config
		cfg, err := config.Load()
		if err != nil {
			return &CommandResult{
				Success: false,
				Message: fmt.Sprintf("Failed to load config: %v", err),
			}, nil
		}

		cfg.MaxSavedSessions = maxSessions
		if err := cfg.Save(); err != nil {
			return &CommandResult{
				Success: false,
				Message: fmt.Sprintf("Failed to save config: %v", err),
			}, nil
		}

		return &CommandResult{
			Success: true,
			Message: fmt.Sprintf("✅ Max saved sessions set to %d", maxSessions),
		}, nil

	default:
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("Unknown action '%s'. Use 'list', 'new', 'delete', or 'config'", action),
		}, nil
	}
}

// handleSessionDeleteInteractive shows interactive session deletion menu
func (c *ChatInterface) handleSessionDeleteInteractive() (*CommandResult, error) {
	sessions, err := c.sessionManager.ListSessions()
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("Failed to list sessions: %v", err),
		}, nil
	}

	if len(sessions) == 0 {
		return &CommandResult{
			Success: false,
			Message: "No sessions to delete",
		}, nil
	}

	currentSession := c.sessionManager.GetCurrentSession()

	fmt.Println()
	fmt.Println(color.YellowBold("🗑️  Select session to delete"))
	fmt.Println()

	// Show available sessions
	for i, session := range sessions {
		age := time.Since(session.LastUsedAt)
		ageStr := formatDuration(age)

		projectName := filepath.Base(session.ProjectPath)
		if projectName == "" || projectName == "." {
			projectName = "no project"
		}

		mark := ""
		if currentSession != nil && session.ID == currentSession.ID {
			mark = " ⭐ (current)"
		}

		fmt.Printf("  %d. %s%s\n", i+1, color.GreenString(session.Title), mark)
		fmt.Printf("     %s | %s | %d messages | %s ago\n",
			color.YellowString(session.Mode),
			color.CyanString(projectName),
			len(session.ConversationHistory),
			ageStr)
	}
	fmt.Println()
	fmt.Printf("  %d. %s\n", len(sessions)+1, color.GrayString("Cancel"))
	fmt.Println()

	// Read user choice
	input, err := c.inputHandler.ReadSimple(color.CyanString(fmt.Sprintf("Select session to delete [1-%d]: ", len(sessions)+1)))
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: "Failed to read input",
		}, nil
	}

	choice, err := strconv.Atoi(strings.TrimSpace(input))
	if err != nil || choice < 1 || choice > len(sessions)+1 {
		return &CommandResult{
			Success: false,
			Message: "Invalid choice",
		}, nil
	}

	// Cancel
	if choice == len(sessions)+1 {
		return &CommandResult{
			Success: true,
			Message: "Cancelled",
		}, nil
	}

	selectedSession := sessions[choice-1]

	// Confirm deletion
	fmt.Printf("\n%s Are you sure you want to delete '%s'? [y/N]: ",
		color.YellowString("⚠️"), selectedSession.Title)

	confirmation, err := c.inputHandler.ReadSimple("")
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: "Failed to read confirmation",
		}, nil
	}

	confirmation = strings.ToLower(strings.TrimSpace(confirmation))
	if confirmation != "y" && confirmation != "yes" {
		return &CommandResult{
			Success: true,
			Message: "Cancelled",
		}, nil
	}

	// Delete the session
	if err := c.sessionManager.DeleteSession(selectedSession.ID); err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("Failed to delete session: %v", err),
		}, nil
	}

	return &CommandResult{
		Success: true,
		Message: fmt.Sprintf("✅ Session '%s' deleted successfully", selectedSession.Title),
	}, nil
}

// handleSessionDeleteAll deletes all sessions with confirmation
func (c *ChatInterface) handleSessionDeleteAll() (*CommandResult, error) {
	sessions, err := c.sessionManager.ListSessions()
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("Failed to list sessions: %v", err),
		}, nil
	}

	if len(sessions) == 0 {
		return &CommandResult{
			Success: false,
			Message: "No sessions to delete",
		}, nil
	}

	currentSession := c.sessionManager.GetCurrentSession()

	fmt.Println()
	fmt.Printf("%s You are about to delete %s session(s)\n",
		color.YellowBold("⚠️  WARNING"),
		color.RedString(fmt.Sprintf("%d", len(sessions))))
	fmt.Println()

	// Show sessions that will be deleted
	for i, session := range sessions {
		projectName := filepath.Base(session.ProjectPath)
		if projectName == "" || projectName == "." {
			projectName = "no project"
		}

		mark := ""
		if currentSession != nil && session.ID == currentSession.ID {
			mark = " ⭐ (current)"
		}

		fmt.Printf("  %d. %s (%s)%s - %d messages\n", i+1, session.Title, projectName, mark, len(session.ConversationHistory))
	}
	fmt.Println()

	// Confirm deletion
	fmt.Printf("%s This action cannot be undone. Are you sure? Type 'DELETE ALL' to confirm: ",
		color.RedString("⚠️"))

	confirmation, err := c.inputHandler.ReadSimple("")
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: "Failed to read confirmation",
		}, nil
	}

	confirmation = strings.TrimSpace(confirmation)
	if confirmation != "DELETE ALL" {
		return &CommandResult{
			Success: true,
			Message: "Cancelled",
		}, nil
	}

	// Delete all sessions
	deletedCount := 0
	for _, session := range sessions {
		if err := c.sessionManager.DeleteSession(session.ID); err != nil {
			fmt.Printf("%s Failed to delete session '%s': %v\n",
				color.YellowString("⚠️"), session.Title, err)
		} else {
			deletedCount++
		}
	}

	// Create a new session after deleting all
	newSession := c.sessionManager.CreateNewSession(c.config.ProjectPath, c.config.Mode)

	// Save the new session
	if err := c.sessionManager.SaveSession(newSession); err != nil {
		fmt.Printf("%s Failed to save new session: %v\n", color.YellowString("⚠️"), err)
	}

	// Clear conversation history and start fresh
	c.context.History = make([]Message, 0)
	c.context.SessionID = newSession.ID
	c.context.CreatedAt = time.Now()
	c.context.UpdatedAt = time.Now()

	// Clear input history
	c.inputHandler.ClearHistory()
	c.inputHandler.history = newSession.CommandHistory

	// Re-add system prompt
	if c.context.ProjectInfo != nil {
		systemPrompt := prompts.GenerateContextualPrompt(c.config.Mode, c.context.ProjectInfo, c.context.Resources)
		c.addMessage("system", systemPrompt)
	}

	return &CommandResult{
		Success: true,
		Message: fmt.Sprintf("✅ Deleted %d sessions and created new session: %s", deletedCount, newSession.Title),
	}, nil
}
