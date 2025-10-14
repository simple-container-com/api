package chat

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// SessionHistory represents a saved chat session
type SessionHistory struct {
	ID                  string         `json:"id"`
	StartedAt           time.Time      `json:"started_at"`
	LastUsedAt          time.Time      `json:"last_used_at"`
	CommandHistory      []string       `json:"command_history"`      // Command history for input
	ConversationHistory []SavedMessage `json:"conversation_history"` // Full conversation context
	ProjectPath         string         `json:"project_path"`
	Mode                string         `json:"mode"`
	Title               string         `json:"title"` // Optional user-defined title
}

// SavedMessage represents a message in conversation history
type SavedMessage struct {
	Role      string                 `json:"role"`
	Content   string                 `json:"content"`
	Timestamp time.Time              `json:"timestamp"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// SessionManager manages session history
type SessionManager struct {
	historyDir     string
	maxSessions    int
	currentSession *SessionHistory
}

// NewSessionManager creates a new session manager
func NewSessionManager() (*SessionManager, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	historyDir := filepath.Join(homeDir, ".sc", "history")
	if err := os.MkdirAll(historyDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create history directory: %w", err)
	}

	return &SessionManager{
		historyDir:  historyDir,
		maxSessions: 5, // Default: keep 5 sessions
	}, nil
}

// SetMaxSessions sets the maximum number of sessions to keep
func (sm *SessionManager) SetMaxSessions(max int) {
	if max > 0 {
		sm.maxSessions = max
	}
}

// GetMaxSessions returns the maximum number of sessions
func (sm *SessionManager) GetMaxSessions() int {
	return sm.maxSessions
}

// CreateNewSession creates a new session
func (sm *SessionManager) CreateNewSession(projectPath, mode string) *SessionHistory {
	session := &SessionHistory{
		ID:                  fmt.Sprintf("session-%d", time.Now().Unix()),
		StartedAt:           time.Now(),
		LastUsedAt:          time.Now(),
		CommandHistory:      make([]string, 0),
		ConversationHistory: make([]SavedMessage, 0),
		ProjectPath:         projectPath,
		Mode:                mode,
		Title:               fmt.Sprintf("Session %s", time.Now().Format("2006-01-02 15:04")),
	}
	sm.currentSession = session
	return session
}

// LoadSession loads a session by ID
func (sm *SessionManager) LoadSession(sessionID string) (*SessionHistory, error) {
	sessionFile := filepath.Join(sm.historyDir, sessionID+".json")
	data, err := os.ReadFile(sessionFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read session file: %w", err)
	}

	var session SessionHistory
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("failed to parse session file: %w", err)
	}

	session.LastUsedAt = time.Now()
	sm.currentSession = &session
	return &session, nil
}

// SaveSession saves the current session to disk
func (sm *SessionManager) SaveSession(session *SessionHistory) error {
	if session == nil {
		return fmt.Errorf("no session to save")
	}

	session.LastUsedAt = time.Now()
	sessionFile := filepath.Join(sm.historyDir, session.ID+".json")

	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	if err := os.WriteFile(sessionFile, data, 0o644); err != nil {
		return fmt.Errorf("failed to write session file: %w", err)
	}

	return nil
}

// ListSessions lists all saved sessions sorted by last used time
func (sm *SessionManager) ListSessions() ([]*SessionHistory, error) {
	files, err := os.ReadDir(sm.historyDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read history directory: %w", err)
	}

	sessions := make([]*SessionHistory, 0)
	for _, file := range files {
		if file.IsDir() || filepath.Ext(file.Name()) != ".json" {
			continue
		}

		sessionID := file.Name()[:len(file.Name())-5] // Remove .json extension
		session, err := sm.LoadSessionWithoutUpdate(sessionID)
		if err != nil {
			// Skip corrupted session files
			continue
		}

		sessions = append(sessions, session)
	}

	// Sort by last used time (most recent first)
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].LastUsedAt.After(sessions[j].LastUsedAt)
	})

	return sessions, nil
}

// LoadSessionWithoutUpdate loads a session without updating LastUsedAt
func (sm *SessionManager) LoadSessionWithoutUpdate(sessionID string) (*SessionHistory, error) {
	sessionFile := filepath.Join(sm.historyDir, sessionID+".json")
	data, err := os.ReadFile(sessionFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read session file: %w", err)
	}

	var session SessionHistory
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("failed to parse session file: %w", err)
	}

	return &session, nil
}

// CleanupOldSessions removes old sessions beyond maxSessions limit
func (sm *SessionManager) CleanupOldSessions() error {
	sessions, err := sm.ListSessions()
	if err != nil {
		return err
	}

	if len(sessions) <= sm.maxSessions {
		return nil
	}

	// Remove sessions beyond the limit
	for i := sm.maxSessions; i < len(sessions); i++ {
		sessionFile := filepath.Join(sm.historyDir, sessions[i].ID+".json")
		if err := os.Remove(sessionFile); err != nil {
			// Log error but continue cleanup
			fmt.Printf("Warning: failed to remove old session %s: %v\n", sessions[i].ID, err)
		}
	}

	return nil
}

// AddMessage adds a message to command history
func (sm *SessionManager) AddMessage(message string) error {
	if sm.currentSession == nil {
		return fmt.Errorf("no active session")
	}

	sm.currentSession.CommandHistory = append(sm.currentSession.CommandHistory, message)
	sm.currentSession.LastUsedAt = time.Now()

	return sm.SaveSession(sm.currentSession)
}

// AddConversationMessage adds a message to conversation history
func (sm *SessionManager) AddConversationMessage(role, content string, metadata map[string]interface{}) error {
	if sm.currentSession == nil {
		return fmt.Errorf("no active session")
	}

	savedMsg := SavedMessage{
		Role:      role,
		Content:   content,
		Timestamp: time.Now(),
		Metadata:  metadata,
	}

	sm.currentSession.ConversationHistory = append(sm.currentSession.ConversationHistory, savedMsg)
	sm.currentSession.LastUsedAt = time.Now()

	return sm.SaveSession(sm.currentSession)
}

// GetCurrentSession returns the current session
func (sm *SessionManager) GetCurrentSession() *SessionHistory {
	return sm.currentSession
}

// DeleteSession deletes a session by ID
func (sm *SessionManager) DeleteSession(sessionID string) error {
	sessionFile := filepath.Join(sm.historyDir, sessionID+".json")
	if err := os.Remove(sessionFile); err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}
	return nil
}

// GetHistoryDir returns the history directory path
func (sm *SessionManager) GetHistoryDir() string {
	return sm.historyDir
}
