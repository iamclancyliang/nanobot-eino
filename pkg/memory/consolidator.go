package memory

import (
	"context"
	"fmt"
	"sync"

	emodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"github.com/wall/nanobot-eino/pkg/session"
	"github.com/wall/nanobot-eino/pkg/trace"
)

const maxConsolidationRounds = 5

// MemoryConsolidator owns consolidation policy, locking, and session offset updates.
// Mirrors Python nanobot's MemoryConsolidator.
type MemoryConsolidator struct {
	Store               *MemoryStore
	chatModel           emodel.ChatModel
	sessions            *session.SessionManager
	contextWindowTokens int
	basePromptTokens    int // estimated fixed overhead (system prompt + tools)

	locks sync.Map // map[string]*sync.Mutex — per-session consolidation locks
}

func NewMemoryConsolidator(
	store *MemoryStore,
	chatModel emodel.ChatModel,
	sessions *session.SessionManager,
	contextWindowTokens int,
	basePromptTokens int,
) *MemoryConsolidator {
	return &MemoryConsolidator{
		Store:               store,
		chatModel:           chatModel,
		sessions:            sessions,
		contextWindowTokens: contextWindowTokens,
		basePromptTokens:    basePromptTokens,
	}
}

func (c *MemoryConsolidator) getLock(sessionKey string) *sync.Mutex {
	lock, _ := c.locks.LoadOrStore(sessionKey, &sync.Mutex{})
	return lock.(*sync.Mutex)
}

// estimateMessageTokens approximates the token count for one message (chars/3).
func estimateMessageTokens(msg *schema.Message) int {
	if msg == nil {
		return 0
	}
	chars := len(msg.Content)
	for _, tc := range msg.ToolCalls {
		chars += len(tc.Function.Name) + len(tc.Function.Arguments)
	}
	return max(1, chars/3)
}

func estimateStringTokens(s string) int {
	if len(s) == 0 {
		return 0
	}
	return max(1, len(s)/3)
}

// estimatePromptTokens approximates the total prompt token count for a session.
func (c *MemoryConsolidator) estimatePromptTokens(s *session.Session) int {
	total := c.basePromptTokens
	total += estimateStringTokens(c.Store.GetMemoryContext())

	for _, msg := range s.Messages[s.LastConsolidated:] {
		total += estimateMessageTokens(msg)
	}
	return total
}

// PickConsolidationBoundary finds a user-turn boundary that removes enough old tokens.
// Returns (endIndex, removedTokens, found).
func (c *MemoryConsolidator) PickConsolidationBoundary(
	s *session.Session,
	tokensToRemove int,
) (endIdx int, removedTokens int, found bool) {
	start := s.LastConsolidated
	if start >= len(s.Messages) || tokensToRemove <= 0 {
		return 0, 0, false
	}

	removed := 0
	lastEndIdx := -1
	lastRemoved := 0

	for idx := start; idx < len(s.Messages); idx++ {
		msg := s.Messages[idx]
		if idx > start && msg.Role == schema.User {
			lastEndIdx = idx
			lastRemoved = removed
			if removed >= tokensToRemove {
				return idx, removed, true
			}
		}
		removed += estimateMessageTokens(msg)
	}

	if lastEndIdx >= 0 {
		return lastEndIdx, lastRemoved, true
	}
	return 0, 0, false
}

// ConsolidateMessages archives a selected message chunk into persistent memory.
func (c *MemoryConsolidator) ConsolidateMessages(ctx context.Context, messages []*schema.Message) bool {
	return c.Store.Consolidate(ctx, messages, c.chatModel)
}

// MaybeConsolidateByTokens loops: archive old messages until prompt fits
// within half the context window. Up to maxConsolidationRounds iterations.
func (c *MemoryConsolidator) MaybeConsolidateByTokens(ctx context.Context, s *session.Session) {
	if len(s.Messages) == 0 || c.contextWindowTokens <= 0 {
		return
	}

	lock := c.getLock(s.Key)
	lock.Lock()
	defer lock.Unlock()

	target := c.contextWindowTokens / 2
	estimated := c.estimatePromptTokens(s)

	if estimated <= 0 || estimated < target {
		logMemory.Debug("Token consolidation idle", "session", s.Key, "estimated", estimated, "window", c.contextWindowTokens, "target", target)
		return
	}

	logMemory.Info("Token consolidation triggered", "session", s.Key, "estimated", estimated, "window", c.contextWindowTokens, "target", target)

	for round := range maxConsolidationRounds {
		if estimated <= target {
			return
		}

		endIdx, _, found := c.PickConsolidationBoundary(s, max(1, estimated-target))
		if !found {
			logMemory.Warn("Token consolidation: no safe boundary", "session", s.Key, "round", round)
			return
		}

		chunk := s.Messages[s.LastConsolidated:endIdx]
		if len(chunk) == 0 {
			return
		}

		logMemory.Info("Token consolidation round", "round", round, "session", s.Key, "estimated", estimated, "window", c.contextWindowTokens, "chunk_msgs", len(chunk))

		ctx = trace.StartSpan(ctx, "Memory Consolidation", map[string]any{
			"session":    s.Key,
			"round":      round,
			"chunk_msgs": len(chunk),
			"estimated":  estimated,
			"target":     target,
		})
		consolidated := c.ConsolidateMessages(ctx, chunk)
		if !consolidated {
			trace.EndSpan(ctx, nil, fmt.Errorf("consolidation failed"))
			return
		}
		trace.EndSpan(ctx, map[string]any{"success": true}, nil)

		s.LastConsolidated = endIdx
		if err := c.sessions.Save(s); err != nil {
			logMemory.Warn("Token consolidation: failed to save session", "error", err)
		}

		estimated = c.estimatePromptTokens(s)
		if estimated <= 0 {
			return
		}
	}
}

// ArchiveUnconsolidated archives the full unconsolidated tail (for /new session rollover).
func (c *MemoryConsolidator) ArchiveUnconsolidated(ctx context.Context, s *session.Session) error {
	lock := c.getLock(s.Key)
	lock.Lock()
	defer lock.Unlock()

	snapshot := s.Messages[s.LastConsolidated:]
	if len(snapshot) == 0 {
		logMemory.Debug("ArchiveUnconsolidated: no messages to archive", "session", s.Key)
		return nil
	}

	logMemory.Info("ArchiveUnconsolidated: archiving", "messages", len(snapshot), "session", s.Key, "last_consolidated", s.LastConsolidated, "total", len(s.Messages))

	if !c.ConsolidateMessages(ctx, snapshot) {
		logMemory.Warn("ArchiveUnconsolidated: consolidation failed", "session", s.Key)
		return fmt.Errorf("consolidation failed")
	}

	logMemory.Info("ArchiveUnconsolidated: consolidation succeeded", "session", s.Key)
	return nil
}
