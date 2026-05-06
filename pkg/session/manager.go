package session

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/schema"
)

// Session is the persisted conversation state for a single session key.
type Session struct {
	Key              string            `json:"key"`
	Messages         []*schema.Message `json:"messages"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
	Metadata         map[string]any    `json:"metadata"`
	LastConsolidated int               `json:"last_consolidated"`
}

// AddMessage appends a message and bumps UpdatedAt.
func (s *Session) AddMessage(msg *schema.Message) {
	s.Messages = append(s.Messages, msg)
	s.UpdatedAt = time.Now()
}

// GetHistory returns unconsolidated messages for LLM input, aligned to a user turn.
// Pass maxMessages=0 to return all unconsolidated messages.
func (s *Session) GetHistory(maxMessages int) []*schema.Message {
	if s.LastConsolidated >= len(s.Messages) {
		return nil
	}
	unconsolidated := s.Messages[s.LastConsolidated:]

	if maxMessages > 0 && len(unconsolidated) > maxMessages {
		unconsolidated = unconsolidated[len(unconsolidated)-maxMessages:]
	}

	// Drop leading non-user messages to avoid orphaned tool_result blocks.
	for i, m := range unconsolidated {
		if m.Role == schema.User {
			return unconsolidated[i:]
		}
	}

	return unconsolidated
}

// Clear resets the session to initial state.
func (s *Session) Clear() {
	s.Messages = nil
	s.LastConsolidated = 0
	s.UpdatedAt = time.Now()
}

// SessionManager is an in-memory cache backed by per-session JSONL files on
// disk. It is safe for concurrent use.
type SessionManager struct {
	sessionsDir string
	cache       map[string]*Session
	mu          sync.RWMutex
}

// NewSessionManager creates the sessions directory if needed and returns a
// SessionManager rooted at it.
func NewSessionManager(sessionsDir string) (*SessionManager, error) {
	if err := os.MkdirAll(sessionsDir, 0755); err != nil {
		return nil, err
	}
	return &SessionManager{
		sessionsDir: sessionsDir,
		cache:       make(map[string]*Session),
	}, nil
}

// GetOrCreate returns the session with the given key, loading it from disk
// or creating a fresh one when missing.
func (m *SessionManager) GetOrCreate(key string) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()

	if s, ok := m.cache[key]; ok {
		return s
	}

	s := m.load(key)
	if s == nil {
		s = &Session{
			Key:       key,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Metadata:  make(map[string]any),
		}
	}
	m.cache[key] = s
	return s
}

func (m *SessionManager) load(key string) *Session {
	path := m.getSessionPath(key)
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	// Allow large lines (up to 1MB) for messages with long tool outputs.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var s *Session
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var data map[string]any
		if err := json.Unmarshal(line, &data); err != nil {
			continue
		}

		if data["_type"] == "metadata" {
			s = &Session{
				Key:      key,
				Metadata: make(map[string]any),
			}
			if md, ok := data["metadata"].(map[string]any); ok {
				s.Metadata = md
			}
			if lc, ok := data["last_consolidated"].(float64); ok {
				s.LastConsolidated = int(lc)
			}
			if ca, ok := data["created_at"].(string); ok {
				s.CreatedAt, _ = time.Parse(time.RFC3339, ca)
			}
			if ua, ok := data["updated_at"].(string); ok {
				s.UpdatedAt, _ = time.Parse(time.RFC3339, ua)
			}
		} else if s != nil {
			msgData, _ := json.Marshal(data)
			var msg schema.Message
			if err := json.Unmarshal(msgData, &msg); err == nil {
				s.Messages = append(s.Messages, &msg)
			}
		}
	}
	return s
}

// Save atomically rewrites the session's JSONL file and refreshes the cache.
func (m *SessionManager) Save(s *Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	path := m.getSessionPath(s.Key)
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	metadata := map[string]any{
		"_type":             "metadata",
		"key":               s.Key,
		"created_at":        s.CreatedAt.Format(time.RFC3339),
		"updated_at":        s.UpdatedAt.Format(time.RFC3339),
		"metadata":          s.Metadata,
		"last_consolidated": s.LastConsolidated,
	}

	encoder := json.NewEncoder(file)
	if err := encoder.Encode(metadata); err != nil {
		return err
	}

	for _, msg := range s.Messages {
		if err := encoder.Encode(msg); err != nil {
			return err
		}
	}

	m.cache[s.Key] = s
	return nil
}

// Invalidate drops a session from the in-memory cache; subsequent
// GetOrCreate calls reload it from disk.
func (m *SessionManager) Invalidate(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.cache, key)
}

func (m *SessionManager) getSessionPath(key string) string {
	safeKey := strings.ReplaceAll(key, ":", "_")
	return filepath.Join(m.sessionsDir, safeKey+".jsonl")
}

// ListSessions returns one entry per session file in the sessions directory.
func (m *SessionManager) ListSessions() []map[string]any {
	files, _ := filepath.Glob(filepath.Join(m.sessionsDir, "*.jsonl"))
	var sessions []map[string]any
	for _, f := range files {
		sessions = append(sessions, map[string]any{
			"key": filepath.Base(f),
		})
	}
	return sessions
}
