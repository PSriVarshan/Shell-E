package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Exchange represents one user-agent interaction
type Exchange struct {
	Timestamp time.Time `json:"timestamp"`
	UserInput string    `json:"user_input"`
	Command   string    `json:"command,omitempty"`
	Result    string    `json:"result,omitempty"`
	Response  string    `json:"response"`
}

// ContextInfo is injected into the LLM prompt
type ContextInfo struct {
	WorkingDirectory string
	LastAction       string
	LastCreated      string
	RecentExchanges  []Exchange
}

// FormatForPrompt converts context into a string for the LLM
func (c *ContextInfo) FormatForPrompt() string {
	var parts []string

	if c.LastAction != "" {
		parts = append(parts, fmt.Sprintf("- Last action: %s", c.LastAction))
	}
	if c.LastCreated != "" {
		parts = append(parts, fmt.Sprintf("- Last created/opened: %s", c.LastCreated))
	}

	if len(c.RecentExchanges) > 0 {
		parts = append(parts, "- Recent conversation:")
		for _, ex := range c.RecentExchanges {
			parts = append(parts, fmt.Sprintf("  User: %s", ex.UserInput))
			if ex.Command != "" {
				parts = append(parts, fmt.Sprintf("  Command: %s", ex.Command))
			}
			if ex.Response != "" {
				parts = append(parts, fmt.Sprintf("  Response: %s", ex.Response))
			}
		}
	}

	return strings.Join(parts, "\n")
}

// Memory manages persistent context
type Memory struct {
	mu           sync.Mutex
	dataDir      string
	WorkingDir   string     `json:"working_directory"`
	LastAction   string     `json:"last_action"`
	LastCreated  string     `json:"last_created"`
	Exchanges    []Exchange `json:"exchanges"`
	MaxExchanges int        `json:"-"` // How many to keep in active memory
	CompactAfter int        `json:"-"` // Compact after this many exchanges
}

func NewMemory(dataDir string) *Memory {
	cwd, _ := os.Getwd()
	return &Memory{
		dataDir:      dataDir,
		WorkingDir:   cwd,
		MaxExchanges: 5,
		CompactAfter: 20,
	}
}

// Load reads memory state from disk
func (m *Memory) Load() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	path := filepath.Join(m.dataDir, "memory.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Fresh start
		}
		return err
	}

	return json.Unmarshal(data, m)
}

// Save writes memory state to disk
func (m *Memory) Save() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	os.MkdirAll(m.dataDir, 0755)

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(m.dataDir, "memory.json"), data, 0644)
}

// RecordExchange adds a new interaction to memory
func (m *Memory) RecordExchange(userInput, command, result, response string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	ex := Exchange{
		Timestamp: time.Now(),
		UserInput: userInput,
		Command:   command,
		Result:    result,
		Response:  response,
	}

	m.Exchanges = append(m.Exchanges, ex)

	// Update context hints
	m.LastAction = response
	if command != "" {
		lower := strings.ToLower(command)
		// Try to detect what was created/opened for context resolution
		if strings.Contains(lower, "new-item") || strings.Contains(lower, "mkdir") {
			m.LastCreated = ExtractNameFromCommand(command)
		} else if strings.Contains(lower, "explorer") {
			m.LastCreated = ExtractPathFromCommand(command)
		}
	}

	// Compact if needed
	if len(m.Exchanges) > m.CompactAfter {
		m.compact()
	}
}

// GetContext returns the current context for the planner
func (m *Memory) GetContext() *ContextInfo {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Get the last N exchanges for the active context
	recent := m.Exchanges
	if len(recent) > m.MaxExchanges {
		recent = recent[len(recent)-m.MaxExchanges:]
	}

	return &ContextInfo{
		WorkingDirectory: m.WorkingDir,
		LastAction:       m.LastAction,
		LastCreated:      m.LastCreated,
		RecentExchanges:  recent,
	}
}

// GetHistory returns all exchanges
func (m *Memory) GetHistory() []Exchange {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]Exchange, len(m.Exchanges))
	copy(result, m.Exchanges)
	return result
}

// Clear resets the conversation memory
func (m *Memory) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Exchanges = nil
	m.LastAction = ""
	m.LastCreated = ""
}

// compact summarizes old exchanges and saves to daily note
func (m *Memory) compact() {
	if len(m.Exchanges) <= m.MaxExchanges {
		return
	}

	// Keep last MaxExchanges, summarize the rest into a daily note
	old := m.Exchanges[:len(m.Exchanges)-m.MaxExchanges]
	m.Exchanges = m.Exchanges[len(m.Exchanges)-m.MaxExchanges:]

	// Write compacted history to daily note
	today := time.Now().Format("2006-01-02")
	notePath := filepath.Join(m.dataDir, "memory", today+".md")

	var note strings.Builder
	note.WriteString(fmt.Sprintf("# Shell-E Session — %s\n\n", today))
	for _, ex := range old {
		note.WriteString(fmt.Sprintf("- [%s] User: %s\n", ex.Timestamp.Format("15:04"), ex.UserInput))
		if ex.Command != "" {
			note.WriteString(fmt.Sprintf("  Command: `%s`\n", ex.Command))
		}
		note.WriteString(fmt.Sprintf("  Response: %s\n\n", ex.Response))
	}

	// Append to existing note (or create new)
	f, err := os.OpenFile(notePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		f.WriteString(note.String())
		f.Close()
	}
}

// ExtractNameFromCommand tries to pull a folder/file name from a command
func ExtractNameFromCommand(cmd string) string {
	// Look for -Name 'xxx' or -Name xxx
	lower := strings.ToLower(cmd)
	idx := strings.Index(lower, "-name")
	if idx != -1 {
		rest := strings.TrimSpace(cmd[idx+5:])
		rest = strings.TrimLeft(rest, " ")
		// Remove quotes
		name := strings.Trim(rest, "'\"")
		// Take first word/token
		if sp := strings.IndexAny(name, " \t"); sp != -1 {
			name = name[:sp]
		}
		return name
	}

	// Look for mkdir xxx
	if strings.Contains(lower, "mkdir") {
		parts := strings.Fields(cmd)
		if len(parts) >= 2 {
			return strings.Trim(parts[len(parts)-1], "'\"")
		}
	}

	return ""
}

// ExtractPathFromCommand pulls a path from explorer or cd commands
func ExtractPathFromCommand(cmd string) string {
	parts := strings.Fields(cmd)
	if len(parts) >= 2 {
		return strings.Trim(parts[len(parts)-1], "'\"")
	}
	return ""
}
