package workspace

import (
	"embed"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

var logWorkspace = slog.With("module", "workspace")

//go:embed templates/*.md
var embeddedTemplates embed.FS

// TemplateFiles lists the prompt template files that must exist for the agent to function.
var TemplateFiles = []string{
	"SOUL.md",
	"USER.md",
	"TOOLS.md",
	"AGENTS.md",
	"HEARTBEAT.md",
}

// SyncTemplates ensures all required template files exist in targetDir.
// Missing files are created from the embedded templates (pkg/workspace/templates/).
// Existing files are never overwritten — this mirrors nanobot's
// sync_workspace_templates() skip-if-exists behavior.
func SyncTemplates(targetDir string) error {
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("create target directory %s: %w", targetDir, err)
	}

	for _, name := range TemplateFiles {
		dst := filepath.Join(targetDir, name)

		if _, err := os.Stat(dst); err == nil {
			continue
		}

		content, err := embeddedTemplates.ReadFile("templates/" + name)
		if err != nil {
			logWorkspace.Warn("Embedded template not found", "template", name, "error", err)
			continue
		}
		if err := os.WriteFile(dst, content, 0644); err != nil {
			return fmt.Errorf("write template %s: %w", dst, err)
		}
		logWorkspace.Info("Created template", "path", dst)
	}

	return nil
}
