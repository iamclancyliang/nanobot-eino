package memory

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	emodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"github.com/wall/nanobot-eino/pkg/session"
)

// ---------------------------------------------------------------------------
// tool_choice fallback tests
// ---------------------------------------------------------------------------

// mockChatModel is a ChatModel that fails on the first Generate call with a
// configurable error, then succeeds on subsequent calls.
type mockChatModel struct {
	calls      int
	firstErr   error
	successMsg *schema.Message
}

func (m *mockChatModel) Generate(_ context.Context, _ []*schema.Message, _ ...emodel.Option) (*schema.Message, error) {
	m.calls++
	if m.calls == 1 && m.firstErr != nil {
		return nil, m.firstErr
	}
	return m.successMsg, nil
}

func (m *mockChatModel) Stream(_ context.Context, _ []*schema.Message, _ ...emodel.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, errors.New("stream not implemented in mock")
}

func (m *mockChatModel) BindTools(_ []*schema.ToolInfo) error {
	return nil
}

// savememoryMsg constructs a valid save_memory tool-call response.
func savememoryMsg() *schema.Message {
	args, _ := json.Marshal(SaveMemoryArgs{
		MemoryUpdate: "updated memory",
		HistoryEntry: "[2026-03-22] test conversation",
	})
	return &schema.Message{
		Role: schema.Assistant,
		ToolCalls: []schema.ToolCall{
			{
				ID: "tc-1",
				Function: schema.FunctionCall{
					Name:      "save_memory",
					Arguments: string(args),
				},
			},
		},
	}
}

func TestIsToolChoiceUnsupportedError_Variants(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{errors.New("tool_choice is not supported"), true},
		{errors.New("model does not support tool_choice parameter"), true},
		{errors.New("should be one of auto or none"), true},
		{errors.New("TOOL_CHOICE REQUIRED"), true}, // case-insensitive
		{errors.New("rate limit exceeded"), false},
		{errors.New("context deadline exceeded"), false},
		{nil, false},
	}
	for _, c := range cases {
		got := isToolChoiceUnsupportedError(c.err)
		if got != c.want {
			t.Errorf("isToolChoiceUnsupportedError(%v) = %v, want %v", c.err, got, c.want)
		}
	}
}

func TestConsolidate_FallsBackWhenToolChoiceUnsupported(t *testing.T) {
	dir := t.TempDir()
	store, err := NewMemoryStore(dir)
	if err != nil {
		t.Fatalf("NewMemoryStore: %v", err)
	}

	mock := &mockChatModel{
		firstErr:   errors.New("does not support tool_choice parameter"),
		successMsg: savememoryMsg(),
	}

	messages := []*schema.Message{
		schema.UserMessage("hello"),
		{Role: schema.Assistant, Content: "hi there"},
	}

	ok := store.Consolidate(context.Background(), messages, mock)
	if !ok {
		t.Fatal("Consolidate should succeed after fallback")
	}
	if mock.calls != 2 {
		t.Errorf("expected 2 Generate calls (forced then allowed), got %d", mock.calls)
	}

	// Verify history was written
	history, err := os.ReadFile(filepath.Join(dir, "HISTORY.md"))
	if err != nil {
		t.Fatalf("read HISTORY.md: %v", err)
	}
	if !strings.Contains(string(history), "test conversation") {
		t.Errorf("HISTORY.md should contain consolidated entry, got: %s", history)
	}
}

func TestConsolidate_NoFallbackWhenOtherError(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewMemoryStore(dir)

	mock := &mockChatModel{
		firstErr:   errors.New("rate limit exceeded"),
		successMsg: savememoryMsg(),
	}

	messages := []*schema.Message{schema.UserMessage("hello")}

	ok := store.Consolidate(context.Background(), messages, mock)
	// rate limit error should NOT trigger fallback — consolidation fails
	if ok {
		t.Fatal("Consolidate should fail on non-tool_choice errors without fallback")
	}
	if mock.calls != 1 {
		t.Errorf("expected exactly 1 Generate call (no retry), got %d", mock.calls)
	}
}

func TestConsolidate_SucceedsFirstTryWhenNoError(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewMemoryStore(dir)

	mock := &mockChatModel{
		firstErr:   nil,
		successMsg: savememoryMsg(),
	}

	messages := []*schema.Message{schema.UserMessage("hello")}

	ok := store.Consolidate(context.Background(), messages, mock)
	if !ok {
		t.Fatal("Consolidate should succeed on first try")
	}
	if mock.calls != 1 {
		t.Errorf("expected 1 Generate call, got %d", mock.calls)
	}
}

func TestMemoryStore_NewCreatesDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "memory")
	_, err := NewMemoryStore(dir)
	if err != nil {
		t.Fatalf("NewMemoryStore error: %v", err)
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Error("memory directory should be created")
	}
}

func TestMemoryStore_ReadLongTerm_Empty(t *testing.T) {
	store, _ := NewMemoryStore(t.TempDir())
	if got := store.ReadLongTerm(); got != "" {
		t.Errorf("ReadLongTerm on empty = %q, want empty", got)
	}
}

func TestMemoryStore_WriteLongTerm_ReadLongTerm(t *testing.T) {
	store, _ := NewMemoryStore(t.TempDir())

	if err := store.WriteLongTerm("User likes cats"); err != nil {
		t.Fatalf("WriteLongTerm error: %v", err)
	}

	got := store.ReadLongTerm()
	if got != "User likes cats" {
		t.Errorf("ReadLongTerm = %q, want %q", got, "User likes cats")
	}
}

func TestMemoryStore_WriteLongTerm_Overwrites(t *testing.T) {
	store, _ := NewMemoryStore(t.TempDir())

	store.WriteLongTerm("first")
	store.WriteLongTerm("second")

	if got := store.ReadLongTerm(); got != "second" {
		t.Errorf("WriteLongTerm should overwrite, got %q", got)
	}
}

func TestMemoryStore_AppendHistory(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewMemoryStore(dir)

	store.AppendHistory("[2026-01-01] First entry")
	store.AppendHistory("[2026-01-02] Second entry")

	data, err := os.ReadFile(filepath.Join(dir, "HISTORY.md"))
	if err != nil {
		t.Fatalf("read HISTORY.md: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "First entry") {
		t.Error("HISTORY.md should contain first entry")
	}
	if !strings.Contains(content, "Second entry") {
		t.Error("HISTORY.md should contain second entry")
	}
	if strings.Count(content, "\n\n") < 2 {
		t.Error("entries should be separated by blank lines")
	}
}

func TestMemoryStore_GetMemoryContext_Empty(t *testing.T) {
	store, _ := NewMemoryStore(t.TempDir())
	if got := store.GetMemoryContext(); got != "" {
		t.Errorf("GetMemoryContext on empty = %q, want empty", got)
	}
}

func TestMemoryStore_GetMemoryContext_WithContent(t *testing.T) {
	store, _ := NewMemoryStore(t.TempDir())
	store.WriteLongTerm("User prefers Go")

	got := store.GetMemoryContext()
	if !strings.Contains(got, "Long-term Memory") {
		t.Errorf("GetMemoryContext should contain header, got %q", got)
	}
	if !strings.Contains(got, "User prefers Go") {
		t.Errorf("GetMemoryContext should contain memory content, got %q", got)
	}
}

func TestFormatMessages(t *testing.T) {
	msgs := []*schema.Message{
		{Role: schema.User, Content: "hello"},
		{Role: schema.Assistant, Content: "hi"},
		{Role: schema.User, Content: ""},
	}

	result := formatMessages(msgs)
	if !strings.Contains(result, "[USER]: hello") {
		t.Errorf("should contain user message, got %q", result)
	}
	if !strings.Contains(result, "[ASSISTANT]: hi") {
		t.Errorf("should contain assistant message, got %q", result)
	}
	lines := strings.Split(result, "\n")
	if len(lines) != 2 {
		t.Errorf("empty messages should be skipped, got %d lines", len(lines))
	}
}

func TestMemoryStore_FailOrRawArchive_UnderThreshold(t *testing.T) {
	store, _ := NewMemoryStore(t.TempDir())
	msgs := []*schema.Message{schema.UserMessage("test")}

	result := store.failOrRawArchive(msgs)
	if result {
		t.Error("should return false under threshold")
	}
	if store.consecutiveFailures != 1 {
		t.Errorf("consecutiveFailures = %d, want 1", store.consecutiveFailures)
	}
}

func TestMemoryStore_FailOrRawArchive_AtThreshold(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewMemoryStore(dir)
	store.consecutiveFailures = maxFailuresBeforeRawArchive - 1
	msgs := []*schema.Message{schema.UserMessage("test message")}

	result := store.failOrRawArchive(msgs)
	if !result {
		t.Error("should return true at threshold (raw archive)")
	}
	if store.consecutiveFailures != 0 {
		t.Errorf("consecutiveFailures should reset to 0, got %d", store.consecutiveFailures)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "HISTORY.md"))
	if !strings.Contains(string(data), "RAW ARCHIVE") {
		t.Error("HISTORY.md should contain RAW ARCHIVE entry")
	}
}

// --- MemoryConsolidator tests (non-LLM parts) ---

func TestEstimateMessageTokens(t *testing.T) {
	msg := &schema.Message{Content: "hello world"} // 11 chars → 3 tokens
	got := estimateMessageTokens(msg)
	if got != 3 {
		t.Errorf("estimateMessageTokens = %d, want 3", got)
	}
}

func TestEstimateMessageTokens_Nil(t *testing.T) {
	if got := estimateMessageTokens(nil); got != 0 {
		t.Errorf("estimateMessageTokens(nil) = %d, want 0", got)
	}
}

func TestEstimateStringTokens(t *testing.T) {
	if got := estimateStringTokens(""); got != 0 {
		t.Errorf("empty string = %d, want 0", got)
	}
	if got := estimateStringTokens("hello"); got != 1 {
		t.Errorf("5 chars = %d, want 1", got)
	}
	if got := estimateStringTokens("hello world!"); got != 4 {
		t.Errorf("12 chars = %d, want 4", got)
	}
}

func makeTestSession(lastConsolidated, msgCount int) *session.Session {
	msgs := make([]*schema.Message, msgCount)
	for i := range msgs {
		if i%2 == 0 {
			msgs[i] = schema.UserMessage(strings.Repeat("x", 30))
		} else {
			msgs[i] = &schema.Message{Role: schema.Assistant, Content: strings.Repeat("y", 30)}
		}
	}
	return &session.Session{
		Key:              "test",
		Messages:         msgs,
		LastConsolidated: lastConsolidated,
	}
}

func TestPickConsolidationBoundary_NoMessages(t *testing.T) {
	c := &MemoryConsolidator{}
	s := makeTestSession(0, 0)

	_, _, found := c.PickConsolidationBoundary(s, 100)
	if found {
		t.Error("should not find boundary with no messages")
	}
}

func TestPickConsolidationBoundary_FindsBoundary(t *testing.T) {
	c := &MemoryConsolidator{}
	s := makeTestSession(0, 6)

	endIdx, _, found := c.PickConsolidationBoundary(s, 1)
	if !found {
		t.Fatal("should find boundary")
	}
	if endIdx < 1 {
		t.Errorf("endIdx = %d, should be >= 1", endIdx)
	}
}

func TestPickConsolidationBoundary_RespectsUserBoundary(t *testing.T) {
	c := &MemoryConsolidator{}
	s := makeTestSession(0, 8)

	endIdx, _, found := c.PickConsolidationBoundary(s, 1)
	if !found {
		t.Fatal("should find boundary")
	}
	if s.Messages[endIdx].Role != schema.User {
		t.Errorf("boundary should be at a user message, got role=%s", s.Messages[endIdx].Role)
	}
}

func TestPickConsolidationBoundary_ZeroTokensToRemove(t *testing.T) {
	c := &MemoryConsolidator{}
	s := makeTestSession(0, 4)

	_, _, found := c.PickConsolidationBoundary(s, 0)
	if found {
		t.Error("should not find boundary with 0 tokensToRemove")
	}
}

func TestPickConsolidationBoundary_AllConsolidated(t *testing.T) {
	c := &MemoryConsolidator{}
	s := makeTestSession(4, 4)

	_, _, found := c.PickConsolidationBoundary(s, 100)
	if found {
		t.Error("should not find boundary when all messages already consolidated")
	}
}
