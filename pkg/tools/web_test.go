package tools

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateURL_Valid(t *testing.T) {
	tests := []string{
		"https://example.com",
		"https://api.example.com/v1/data?q=test",
	}
	for _, u := range tests {
		ok, errMsg := validateURL(u)
		if !ok {
			t.Errorf("validateURL(%q) = false: %s", u, errMsg)
		}
	}
}

func TestValidateURL_BlocksInternalHost(t *testing.T) {
	tests := []string{
		"http://localhost:8080",
		"http://127.0.0.1:3000",
		"http://10.0.0.1",
		"http://169.254.169.254/latest/meta-data/",
	}
	for _, u := range tests {
		ok, errMsg := validateURL(u)
		if ok {
			t.Fatalf("validateURL(%q) should block internal host", u)
		}
		if !strings.Contains(strings.ToLower(errMsg), "internal") &&
			!strings.Contains(strings.ToLower(errMsg), "private") &&
			!strings.Contains(strings.ToLower(errMsg), "localhost") {
			t.Fatalf("validateURL(%q) expected internal/private hint, got: %q", u, errMsg)
		}
	}
}

func TestValidateURL_Invalid(t *testing.T) {
	tests := []struct {
		url     string
		wantMsg string
	}{
		{"ftp://files.example.com", "Only http/https"},
		{"file:///etc/passwd", "Only http/https"},
		{"://missing-scheme", ""},
	}
	for _, tc := range tests {
		ok, errMsg := validateURL(tc.url)
		if ok {
			t.Errorf("validateURL(%q) should be invalid", tc.url)
		}
		if tc.wantMsg != "" && !strings.Contains(errMsg, tc.wantMsg) {
			t.Errorf("validateURL(%q) error = %q, want containing %q", tc.url, errMsg, tc.wantMsg)
		}
	}
}

func TestValidateURL_NoScheme(t *testing.T) {
	ok, errMsg := validateURL("example.com")
	if ok {
		t.Error("URL without scheme should be invalid")
	}
	if !strings.Contains(errMsg, "none") {
		t.Errorf("error message should mention 'none', got: %s", errMsg)
	}
}

func TestStripHTMLTags(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"<p>Hello</p>", "Hello"},
		{"<script>alert('xss')</script>text", "text"},
		{"<style>.red{color:red}</style>content", "content"},
		{"<b>bold</b> &amp; <i>italic</i>", "bold & italic"},
		{"no tags here", "no tags here"},
	}
	for _, tc := range tests {
		got := stripHTMLTags(tc.input)
		if got != tc.expected {
			t.Errorf("stripHTMLTags(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestNormalizeWhitespace(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"  hello   world  ", "hello world"},
		{"a\n\n\n\n\nb", "a\n\nb"},
		{"  spaced  out  ", "spaced out"},
	}
	for _, tc := range tests {
		got := normalizeWhitespace(tc.input)
		if got != tc.expected {
			t.Errorf("normalizeWhitespace(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestHTMLToMarkdown(t *testing.T) {
	html := `<h1>Title</h1><p>Some text with <a href="https://example.com">a link</a>.</p><ul><li>Item 1</li><li>Item 2</li></ul>`

	result := htmlToMarkdown(html)

	if !strings.Contains(result, "# Title") {
		t.Errorf("should convert h1, got: %s", result)
	}
	if !strings.Contains(result, "[a link](https://example.com)") {
		t.Errorf("should convert links, got: %s", result)
	}
	if !strings.Contains(result, "- Item 1") {
		t.Errorf("should convert list items, got: %s", result)
	}
}

func TestHTMLToMarkdown_HeadingLevels(t *testing.T) {
	html := `<h2>Sub</h2><h3>SubSub</h3>`
	result := htmlToMarkdown(html)

	if !strings.Contains(result, "## Sub") {
		t.Errorf("should handle h2, got: %s", result)
	}
	if !strings.Contains(result, "### SubSub") {
		t.Errorf("should handle h3, got: %s", result)
	}
}

func TestFormatSearchResults_Empty(t *testing.T) {
	result := formatSearchResults("test query", nil, 5)
	if !strings.Contains(result, "No results") {
		t.Errorf("empty results should say 'No results', got: %s", result)
	}
}

func TestFormatSearchResults_WithItems(t *testing.T) {
	items := []searchResult{
		{Title: "Result 1", URL: "https://a.com", Content: "snippet 1"},
		{Title: "Result 2", URL: "https://b.com", Content: "snippet 2"},
	}

	result := formatSearchResults("my query", items, 5)

	if !strings.Contains(result, "Results for: my query") {
		t.Error("should contain query")
	}
	if !strings.Contains(result, "1. Result 1") {
		t.Error("should contain first result")
	}
	if !strings.Contains(result, "https://a.com") {
		t.Error("should contain URL")
	}
	if !strings.Contains(result, "snippet 1") {
		t.Error("should contain snippet")
	}
}

func TestFormatSearchResults_RespectsLimit(t *testing.T) {
	items := []searchResult{
		{Title: "A", URL: "https://a.com"},
		{Title: "B", URL: "https://b.com"},
		{Title: "C", URL: "https://c.com"},
	}

	result := formatSearchResults("q", items, 2)
	if strings.Contains(result, "3.") {
		t.Error("should respect n limit of 2")
	}
}

func TestExtractDuckDuckGoTopics_FlattensNestedGroups(t *testing.T) {
	raw := []any{
		map[string]any{
			"Name": "Group",
			"Topics": []any{
				map[string]any{
					"Text":     "CloudWeGo Eino - framework",
					"FirstURL": "https://example.com/eino",
				},
			},
		},
		map[string]any{
			"Text":     "Standalone result",
			"FirstURL": "https://example.com/standalone",
		},
	}

	items := extractDuckDuckGoTopics(raw)
	if len(items) != 2 {
		t.Fatalf("expected 2 extracted topics, got %d", len(items))
	}
	if items[0].URL != "https://example.com/eino" {
		t.Fatalf("unexpected first URL: %q", items[0].URL)
	}
	if items[1].Title != "Standalone result" {
		t.Fatalf("unexpected second title: %q", items[1].Title)
	}
}

func TestIsHTML(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"<!doctype html><html>", true},
		{"<html><body>hello</body></html>", true},
		{"plain text content", false},
		{"{\"json\": true}", false},
	}
	for _, tc := range tests {
		got := isHTML([]byte(tc.input))
		if got != tc.want {
			t.Errorf("isHTML(%q) = %v, want %v", tc.input[:min(30, len(tc.input))], got, tc.want)
		}
	}
}

func TestWebFetchJinaAddsUntrustedBanner(t *testing.T) {
	text := "example external content"
	resp, _ := json.Marshal(fetchResponse{
		URL:       "https://example.com",
		FinalURL:  "https://example.com",
		Status:    200,
		Extractor: "jina",
		Length:    len(text),
		Text:      text,
	})
	bannered := withUntrustedBanner(string(resp))
	if !strings.Contains(bannered, "External content") {
		t.Fatalf("expected untrusted banner in output, got: %s", bannered)
	}
}

func TestAddUntrustedBanner_Idempotent(t *testing.T) {
	once := addUntrustedBanner("hello")
	twice := addUntrustedBanner(once)
	if strings.Count(twice, "External content") != 1 {
		t.Fatalf("banner should be injected only once, got: %s", twice)
	}
}
