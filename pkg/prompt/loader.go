package prompt

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cloudwego/eino/schema"
)

type Loader struct {
	baseDir string
}

func NewLoader(baseDir string) *Loader {
	return &Loader{baseDir: baseDir}
}

func (l *Loader) BuildSystemMessages(ctx context.Context) ([]*schema.Message, error) {
	soul, err := l.readFile("SOUL.md")
	if err != nil {
		return nil, err
	}
	user, err := l.readFile("USER.md")
	if err != nil {
		return nil, err
	}
	tools, err := l.readFile("TOOLS.md")
	if err != nil {
		return nil, err
	}
	agents, err := l.readFile("AGENTS.md")
	if err != nil {
		return nil, err
	}

	content := fmt.Sprintf("# SOUL\n%s\n\n# USER PROFILE\n%s\n\n# TOOL USAGE\n%s\n\n# AGENT INSTRUCTIONS\n%s", soul, user, tools, agents)

	// Try to load HEARTBEAT.md if it exists
	if hb, err := l.readFile("HEARTBEAT.md"); err == nil {
		content = fmt.Sprintf("%s\n\n# HEARTBEAT TASKS\n%s", content, hb)
	}

	return []*schema.Message{
		schema.SystemMessage(content),
	}, nil
}

func (l *Loader) readFile(name string) (string, error) {
	path := filepath.Join(l.baseDir, name)
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read prompt file %s: %w", name, err)
	}
	return string(content), nil
}
