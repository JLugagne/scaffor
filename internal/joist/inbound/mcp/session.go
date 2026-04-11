package mcp

// Session manages a per-MCP-session JSONL log stored in .joist/<uuid>.jsonl.
// Every tool call is appended as a single JSON line. The status tool reads
// all lines back as plain text.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ToolCall is a single logged entry written to the session JSONL file.
type ToolCall struct {
	Timestamp  time.Time         `json:"timestamp"`
	Tool       string            `json:"tool"`
	Parameters map[string]any    `json:"parameters,omitempty"`
	FileEvents []FileEvent       `json:"file_events,omitempty"`
}

// FileEvent records what happened to a single file during execution.
type FileEvent struct {
	Path   string `json:"path"`
	Action string `json:"action"` // "created", "overwritten", "skipped"
}

// Session holds the path to the JSONL log and a mutex to serialise writes.
type Session struct {
	path string
	mu   sync.Mutex
}

// NewSession creates the .joist/ directory (if needed) and returns a Session
// whose log file is .joist/<uuid>.jsonl.
func NewSession() (*Session, error) {
	dir := ".joist"
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("creating session directory: %w", err)
	}
	id := uuid.New().String()
	return &Session{path: filepath.Join(dir, id+".jsonl")}, nil
}

// Log appends a tool call entry to the JSONL file.
func (s *Session) Log(tool string, params map[string]any, events []FileEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry := ToolCall{
		Timestamp:  time.Now().UTC(),
		Tool:       tool,
		Parameters: params,
		FileEvents: events,
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	f, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(data)
	return err
}

// Status reads the entire JSONL file and returns it as plain text.
func (s *Session) Status() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return "No tool calls recorded yet.", nil
		}
		return "", err
	}
	if len(data) == 0 {
		return "No tool calls recorded yet.", nil
	}
	return string(data), nil
}
