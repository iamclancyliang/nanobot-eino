package channels

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestIsSenderAllowed_DefaultDeny(t *testing.T) {
	cfg := FeishuConfig{}
	if IsSenderAllowed("feishu", "ou_xxx", cfg.AllowFrom) {
		t.Fatal("expected default deny when allowFrom is empty")
	}
}

func TestIsSenderAllowed_WildcardAllow(t *testing.T) {
	if !IsSenderAllowed("feishu", "ou_xxx", []string{"*"}) {
		t.Fatal("expected wildcard allow")
	}
}

func TestIsSenderAllowed_ExactAllow(t *testing.T) {
	if !IsSenderAllowed("feishu", "ou_owner", []string{"ou_owner"}) {
		t.Fatal("expected exact sender allow")
	}
	if IsSenderAllowed("feishu", "ou_other", []string{"ou_owner"}) {
		t.Fatal("expected non-owner denied")
	}
}

func TestShouldProcessGroupMessage(t *testing.T) {
	if shouldProcessGroupMessage("open", "hello everyone") != true {
		t.Fatal("open policy should pass all group messages")
	}
	if shouldProcessGroupMessage("mention", "hello everyone") {
		t.Fatal("mention policy should ignore non-mention message")
	}
	if !shouldProcessGroupMessage("mention", "@_user_123 hello") {
		t.Fatal("mention policy should accept mention message")
	}
	if !shouldProcessGroupMessage("mention", `<at user_id="ou_xxx">bot</at> hello`) {
		t.Fatal("mention policy should accept feishu <at> mention message")
	}
}

func TestNormalizeFeishuText(t *testing.T) {
	if got := normalizeFeishuText("  @_user_123 hello bot  "); got != "hello bot" {
		t.Fatalf("normalize mention prefix failed, got=%q", got)
	}
	if got := normalizeFeishuText("no mention content"); got != "no mention content" {
		t.Fatalf("normalize plain text failed, got=%q", got)
	}
	if got := normalizeFeishuText("@_user_123"); got != "" {
		t.Fatalf("normalize mention-only text should be empty, got=%q", got)
	}
}

func TestFormatToolHintMarkdown(t *testing.T) {
	got := formatToolHintMarkdown(`web_search("query"), read_file("/path/to/file")`)
	want := "**Tool Calls**\n\n```text\nweb_search(\"query\"),\nread_file(\"/path/to/file\")\n```"
	if got != want {
		t.Fatalf("tool hint markdown mismatch\nwant:\n%s\n\ngot:\n%s", want, got)
	}
}

func TestFormatToolHintMarkdown_KeepCommaInArguments(t *testing.T) {
	got := formatToolHintMarkdown(`web_search("foo, bar"), read_file("/path/to/file")`)
	want := "**Tool Calls**\n\n```text\nweb_search(\"foo, bar\"),\nread_file(\"/path/to/file\")\n```"
	if got != want {
		t.Fatalf("tool hint markdown mismatch\nwant:\n%s\n\ngot:\n%s", want, got)
	}
}

func TestBuildFeishuCardContent_EmptyToolHintSkipped(t *testing.T) {
	_, ok := buildFeishuCardContent("   ", map[string]any{"_tool_hint": true})
	if ok {
		t.Fatal("expected empty tool hint content to be skipped")
	}
}

func TestBuildFeishuCardContent_ToolHintRendersCard(t *testing.T) {
	cardJSON, ok := buildFeishuCardContent(`web_search("query")`, map[string]any{"_tool_hint": true})
	if !ok {
		t.Fatal("expected tool hint card to be produced")
	}
	var card struct {
		Elements []struct {
			Tag     string `json:"tag"`
			Content string `json:"content"`
		} `json:"elements"`
	}
	if err := json.Unmarshal([]byte(cardJSON), &card); err != nil {
		t.Fatalf("invalid card json: %v", err)
	}
	if len(card.Elements) != 1 || card.Elements[0].Tag != "markdown" {
		t.Fatalf("invalid card elements: %+v", card.Elements)
	}
	if card.Elements[0].Content != "**Tool Calls**\n\n```text\nweb_search(\"query\")\n```" {
		t.Fatalf("unexpected markdown content: %s", card.Elements[0].Content)
	}
}

func TestPickFeishuReplyTarget_PriorityReplyTo(t *testing.T) {
	got := pickFeishuReplyTarget("om_reply_to", map[string]any{"message_id": "om_meta"})
	if got != "om_reply_to" {
		t.Fatalf("expected ReplyTo to take priority, got %q", got)
	}
}

func TestPickFeishuReplyTarget_UseMetadataMessageID(t *testing.T) {
	got := pickFeishuReplyTarget("", map[string]any{"message_id": "om_meta"})
	if got != "om_meta" {
		t.Fatalf("expected metadata message_id, got %q", got)
	}
}

func TestPickFeishuReplyTarget_SkipProgressMessage(t *testing.T) {
	got := pickFeishuReplyTarget("", map[string]any{
		"message_id": "om_meta",
		"_progress":  true,
	})
	if got != "" {
		t.Fatalf("expected progress message to skip reply target, got %q", got)
	}
}

func TestIsFeishuProgressMessage(t *testing.T) {
	if !isFeishuProgressMessage(map[string]any{"_progress": true}) {
		t.Fatal("expected progress metadata to be recognized")
	}
	if isFeishuProgressMessage(map[string]any{"_progress": false}) {
		t.Fatal("expected non-progress metadata to be ignored")
	}
	if isFeishuProgressMessage(nil) {
		t.Fatal("expected nil metadata to be non-progress")
	}
}

func TestSplitFeishuMarkdownContent_LongText(t *testing.T) {
	long := strings.Repeat("a", feishuCardMarkdownMaxRunes+10)
	parts := splitFeishuMarkdownContent(long, feishuCardMarkdownMaxRunes)
	if len(parts) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(parts))
	}
	if parts[0] != strings.Repeat("a", feishuCardMarkdownMaxRunes) {
		t.Fatal("first chunk length/content mismatch")
	}
	if parts[1] != strings.Repeat("a", 10) {
		t.Fatal("second chunk length/content mismatch")
	}
}

func TestBuildFeishuCardContents_LongContentSplit(t *testing.T) {
	long := strings.Repeat("b", feishuCardMarkdownMaxRunes+1)
	cards, ok := buildFeishuCardContents(long, nil)
	if !ok {
		t.Fatal("expected card contents")
	}
	if len(cards) != 2 {
		t.Fatalf("expected 2 cards for long content, got %d", len(cards))
	}
}

// ---------------------------------------------------------------------------
// Message content extraction tests (P2 feature)
// ---------------------------------------------------------------------------

func TestExtractShareCardContent_ShareChat(t *testing.T) {
	content := map[string]any{"chat_id": "oc_abc123"}
	got := extractShareCardContent(content, "share_chat")
	if got != "[shared chat: oc_abc123]" {
		t.Fatalf("unexpected: %q", got)
	}
}

func TestExtractShareCardContent_ShareUser(t *testing.T) {
	content := map[string]any{"user_id": "ou_xyz"}
	got := extractShareCardContent(content, "share_user")
	if got != "[shared user: ou_xyz]" {
		t.Fatalf("unexpected: %q", got)
	}
}

func TestExtractShareCardContent_System(t *testing.T) {
	got := extractShareCardContent(map[string]any{}, "system")
	if got != "[system message]" {
		t.Fatalf("unexpected: %q", got)
	}
}

func TestExtractShareCardContent_MergeForward(t *testing.T) {
	got := extractShareCardContent(map[string]any{}, "merge_forward")
	if got != "[merged forward messages]" {
		t.Fatalf("unexpected: %q", got)
	}
}

func TestExtractShareCardContent_NilContent(t *testing.T) {
	got := extractShareCardContent(nil, "system")
	if got != "[system]" {
		t.Fatalf("nil content should fall back to [type], got: %q", got)
	}
}

func TestExtractInteractiveContent_Title(t *testing.T) {
	content := map[string]any{
		"title": "My Card Title",
	}
	parts := extractInteractiveContent(content)
	if len(parts) == 0 || parts[0] != "title: My Card Title" {
		t.Fatalf("expected title, got: %v", parts)
	}
}

func TestExtractInteractiveContent_Header(t *testing.T) {
	content := map[string]any{
		"header": map[string]any{
			"title": map[string]any{"content": "Header Title"},
		},
	}
	parts := extractInteractiveContent(content)
	if len(parts) == 0 || parts[0] != "title: Header Title" {
		t.Fatalf("expected header title, got: %v", parts)
	}
}

func TestExtractElementContent_Markdown(t *testing.T) {
	elem := map[string]any{"tag": "markdown", "content": "**hello**"}
	parts := extractElementContent(elem)
	if len(parts) != 1 || parts[0] != "**hello**" {
		t.Fatalf("unexpected: %v", parts)
	}
}

func TestExtractElementContent_Button(t *testing.T) {
	elem := map[string]any{
		"tag":  "button",
		"text": map[string]any{"content": "Click me"},
		"url":  "https://example.com",
	}
	parts := extractElementContent(elem)
	if len(parts) != 2 || parts[0] != "Click me" || parts[1] != "link: https://example.com" {
		t.Fatalf("unexpected: %v", parts)
	}
}

func TestExtractElementContent_Img(t *testing.T) {
	elem := map[string]any{
		"tag": "img",
		"alt": map[string]any{"content": "photo"},
	}
	parts := extractElementContent(elem)
	if len(parts) != 1 || parts[0] != "photo" {
		t.Fatalf("unexpected: %v", parts)
	}
}

func TestExtractElementContent_ColumnSet(t *testing.T) {
	elem := map[string]any{
		"tag": "column_set",
		"columns": []any{
			map[string]any{
				"elements": []any{
					map[string]any{"tag": "markdown", "content": "col1"},
				},
			},
			map[string]any{
				"elements": []any{
					map[string]any{"tag": "plain_text", "content": "col2"},
				},
			},
		},
	}
	parts := extractElementContent(elem)
	if len(parts) != 2 || parts[0] != "col1" || parts[1] != "col2" {
		t.Fatalf("unexpected: %v", parts)
	}
}

func TestExtractPostText_Direct(t *testing.T) {
	content := map[string]any{
		"title": "My Post",
		"content": []any{
			[]any{
				map[string]any{"tag": "text", "text": "Hello world"},
			},
		},
	}
	got := extractPostText(content)
	if !strings.Contains(got, "Hello world") {
		t.Fatalf("expected 'Hello world' in output, got: %q", got)
	}
}

func TestExtractPostText_Localized(t *testing.T) {
	content := map[string]any{
		"zh_cn": map[string]any{
			"title": "中文标题",
			"content": []any{
				[]any{
					map[string]any{"tag": "text", "text": "你好"},
				},
			},
		},
	}
	got := extractPostText(content)
	if !strings.Contains(got, "你好") {
		t.Fatalf("expected '你好' in output, got: %q", got)
	}
}

func TestExtractPostText_Wrapped(t *testing.T) {
	content := map[string]any{
		"post": map[string]any{
			"zh_cn": map[string]any{
				"title": "Wrapped",
				"content": []any{
					[]any{
						map[string]any{"tag": "text", "text": "wrapped text"},
					},
				},
			},
		},
	}
	got := extractPostText(content)
	if !strings.Contains(got, "wrapped text") {
		t.Fatalf("expected 'wrapped text' in output, got: %q", got)
	}
}

func TestExtractInteractiveContent_FullCard(t *testing.T) {
	// Simulate a realistic interactive card JSON
	raw := `{
		"header": {"title": {"content": "Card Header"}},
		"elements": [
			[
				{"tag": "markdown", "content": "**Status:** Done"},
				{"tag": "button", "text": {"content": "View"}, "url": "https://example.com"}
			]
		]
	}`
	var content map[string]any
	if err := json.Unmarshal([]byte(raw), &content); err != nil {
		t.Fatalf("json parse error: %v", err)
	}
	parts := extractInteractiveContent(content)
	joined := strings.Join(parts, "\n")
	if !strings.Contains(joined, "Card Header") {
		t.Fatalf("missing header in: %q", joined)
	}
	if !strings.Contains(joined, "**Status:** Done") {
		t.Fatalf("missing markdown in: %q", joined)
	}
	if !strings.Contains(joined, "https://example.com") {
		t.Fatalf("missing button url in: %q", joined)
	}
}
