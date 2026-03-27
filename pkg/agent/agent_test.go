package agent

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type testBaseTool struct {
	name string
}

func (t *testBaseTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{Name: t.name}, nil
}

func (t *testBaseTool) InvokableRun(_ context.Context, _ string, _ ...tool.Option) (string, error) {
	return "ok", nil
}

func TestStringToStream(t *testing.T) {
	sr := stringToStream("hello world")

	msg, err := sr.Recv()
	if err != nil {
		t.Fatalf("Recv error: %v", err)
	}
	if msg.Role != schema.Assistant {
		t.Errorf("Role = %s, want assistant", msg.Role)
	}
	if msg.Content != "hello world" {
		t.Errorf("Content = %q, want %q", msg.Content, "hello world")
	}

	_, err = sr.Recv()
	if err != io.EOF {
		t.Errorf("second Recv should EOF, got %v", err)
	}
}

func TestStringToStream_EmptyContent(t *testing.T) {
	sr := stringToStream("")

	msg, err := sr.Recv()
	if err != nil {
		t.Fatalf("Recv error: %v", err)
	}
	if msg.Content != "" {
		t.Errorf("Content = %q, want empty", msg.Content)
	}
}

func TestHelpTextIncludesRestart(t *testing.T) {
	help := commandHelpText()
	if !strings.Contains(help, "/restart") {
		t.Fatalf("help text should include /restart, got: %s", help)
	}
}

func TestRestartAckText(t *testing.T) {
	ack := restartAckText()
	if strings.TrimSpace(ack) == "" {
		t.Fatal("restart ack should not be empty")
	}
	if !strings.Contains(strings.ToLower(ack), "restart") {
		t.Fatalf("restart ack should mention restart, got: %s", ack)
	}
}

func TestNormalizeToolArguments_EmptyToObject(t *testing.T) {
	got := normalizeToolArguments("   ")
	if got != "{}" {
		t.Fatalf("expected empty args normalized to {}, got %q", got)
	}
}

func TestNormalizeToolArguments_StripsCodeFence(t *testing.T) {
	got := normalizeToolArguments("```json\n{\"query\":\"hello\"}\n```")
	if got != `{"query":"hello"}` {
		t.Fatalf("unexpected normalized args: %q", got)
	}
}

func TestNormalizeToolArguments_InvalidJSONFallback(t *testing.T) {
	got := normalizeToolArguments("{invalid")
	if !strings.Contains(got, `"_tool_argument_error":"invalid_json"`) {
		t.Fatalf("expected invalid_json marker, got %q", got)
	}
	if !strings.Contains(got, `"_raw_arguments":"{invalid"`) {
		t.Fatalf("expected raw arguments in fallback payload, got %q", got)
	}
}

func TestListToolNames_Sorted(t *testing.T) {
	tools := []tool.BaseTool{
		&testBaseTool{name: "web_fetch"},
		&testBaseTool{name: "read_file"},
		&testBaseTool{name: "exec"},
	}
	got := listToolNames(context.Background(), tools)
	want := []string{"exec", "read_file", "web_fetch"}
	if len(got) != len(want) {
		t.Fatalf("unexpected tool count: got=%d want=%d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected order: got=%v want=%v", got, want)
		}
	}
}
