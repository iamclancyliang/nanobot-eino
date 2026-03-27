package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/wall/nanobot-eino/pkg/config"
	"github.com/wall/nanobot-eino/pkg/workspace"
)

func newOnboardCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "onboard",
		Short: "Initialize configuration and workspace",
		RunE:  runOnboard,
	}
}

func runOnboard(cmd *cobra.Command, args []string) error {
	configDir := config.DefaultConfigDir()
	configPath := config.DefaultConfigPath()

	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	if _, err := os.Stat(configPath); err == nil {
		fmt.Printf("Config already exists: %s\n", configPath)
	} else {
		cfg := config.DefaultConfig()
		if err := config.Save(configPath, cfg); err != nil {
			return fmt.Errorf("save default config: %w", err)
		}
		fmt.Printf("Created config: %s\n", configPath)
	}

	dirs := []string{
		config.GetPromptsDir(),
		config.GetSkillsDir(),
		config.GetWorkspacePath(""),
		config.GetSessionsDir(),
		config.GetMemoryDir(),
		config.GetCronDir(),
		config.GetLogsDir(),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("create %s: %w", d, err)
		}
	}
	fmt.Println("Directories ready")

	if err := workspace.SyncTemplates(config.GetPromptsDir()); err != nil {
		return fmt.Errorf("sync prompt templates: %w", err)
	}

	fmt.Printf("All files stored under: %s\n", configDir)
	fmt.Println("Onboarding complete! Next steps:")
	fmt.Printf("  1. Edit %s to configure your model and API key\n", configPath)
	fmt.Println("  2. Run 'go run ./cmd/nanobot agent' to start chatting")
	fmt.Println("  3. Run 'go run ./cmd/nanobot gateway' to start the full server")

	return nil
}
