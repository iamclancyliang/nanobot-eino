package reactutil_test

import (
	"context"
	"errors"
	"io"
	"testing"

	emodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"github.com/wall/nanobot-eino/pkg/reactutil"
)

// --- StreamToolCallChecker tests ---

func makeStream(msgs []*schema.Message, finalErr error) *schema.StreamReader[*schema.Message] {
	sr, sw := schema.Pipe[*schema.Message](len(msgs) + 1)
	go func() {
		for _, m := range msgs {
			sw.Send(m, nil)
		}
		if finalErr != nil {
			sw.Send(nil, finalErr)
		}
		sw.Close()
	}()
	return sr
}

func TestStreamToolCallChecker_ReturnsTrueWhenToolCallPresent(t *testing.T) {
	msgs := []*schema.Message{
		{Role: schema.Assistant, Content: "thinking..."},
		{Role: schema.Assistant, ToolCalls: []schema.ToolCall{{ID: "1", Function: schema.FunctionCall{Name: "my_tool"}}}},
	}
	got, err := reactutil.StreamToolCallChecker(context.Background(), makeStream(msgs, nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got {
		t.Error("expected true (tool call present), got false")
	}
}

func TestStreamToolCallChecker_ReturnsFalseWhenNoToolCall(t *testing.T) {
	msgs := []*schema.Message{
		{Role: schema.Assistant, Content: "plain response"},
	}
	got, err := reactutil.StreamToolCallChecker(context.Background(), makeStream(msgs, nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got {
		t.Error("expected false (no tool call), got true")
	}
}

func TestStreamToolCallChecker_ReturnsFalseOnEmptyStream(t *testing.T) {
	got, err := reactutil.StreamToolCallChecker(context.Background(), makeStream(nil, nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got {
		t.Error("expected false on empty stream, got true")
	}
}

func TestStreamToolCallChecker_ReturnsErrorOnStreamError(t *testing.T) {
	boom := errors.New("stream error")
	sr, sw := schema.Pipe[*schema.Message](2)
	go func() {
		sw.Send(&schema.Message{Role: schema.Assistant, Content: "hi"}, nil)
		sw.Send(nil, boom)
		sw.Close()
	}()
	_, err := reactutil.StreamToolCallChecker(context.Background(), sr)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- Model type detection tests ---

// mockToolCallingModel implements emodel.ToolCallingChatModel.
type mockToolCallingModel struct{}

func (m *mockToolCallingModel) Generate(_ context.Context, _ []*schema.Message, _ ...emodel.Option) (*schema.Message, error) {
	return nil, nil
}
func (m *mockToolCallingModel) Stream(_ context.Context, _ []*schema.Message, _ ...emodel.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, nil
}
func (m *mockToolCallingModel) WithTools(_ []*schema.ToolInfo) (emodel.ToolCallingChatModel, error) {
	return m, nil
}
func (m *mockToolCallingModel) BindTools(_ []*schema.ToolInfo) error { return nil }

func TestModelTypeDetection_ToolCallingChatModel(t *testing.T) {
	m := &mockToolCallingModel{}
	if !reactutil.IsToolCallingChatModel(m) {
		t.Error("expected mockToolCallingModel to be detected as ToolCallingChatModel")
	}
}

// mockPlainModel implements emodel.ChatModel (not ToolCallingChatModel).
type mockPlainModel struct{}

func (m *mockPlainModel) Generate(_ context.Context, _ []*schema.Message, _ ...emodel.Option) (*schema.Message, error) {
	return nil, nil
}
func (m *mockPlainModel) Stream(_ context.Context, _ []*schema.Message, _ ...emodel.Option) (*schema.StreamReader[*schema.Message], error) {
	sr, sw := schema.Pipe[*schema.Message](1)
	go func() {
		sw.Send(&schema.Message{Role: schema.Assistant, Content: "hi"}, nil)
		sw.Close()
	}()
	return sr, nil
}
func (m *mockPlainModel) BindTools(_ []*schema.ToolInfo) error { return nil }

func TestModelTypeDetection_PlainModel(t *testing.T) {
	m := &mockPlainModel{}
	if reactutil.IsToolCallingChatModel(m) {
		t.Error("expected plain ChatModel NOT to be detected as ToolCallingChatModel")
	}
}

// Ensure io is used (for EOF constant reference in package under test).
var _ = io.EOF
