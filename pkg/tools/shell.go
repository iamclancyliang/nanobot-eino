package tools

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

var defaultDenyPatterns = []string{
	`\brm\s+-[rf]{1,2}\b`,
	`\bdel\s+/[fq]\b`,
	`\brmdir\s+/s\b`,
	`(?:^|[;&|]\s*)format\b`,
	`\b(mkfs|diskpart)\b`,
	`\bdd\s+if=`,
	`>\s*/dev/sd`,
	`\b(shutdown|reboot|poweroff)\b`,
	`:\(\)\s*\{.*\};\s*:`,
}

// ShellConfig is the safety policy and runtime configuration for the shell
// tool.
type ShellConfig struct {
	Timeout             time.Duration
	MaxOutput           int
	WorkingDir          string
	DenyPatterns        []string
	AllowPatterns       []string
	RestrictToWorkspace bool
	PathAppend          string
}

// ShellExecArgs are the arguments accepted by the shell_exec tool.
type ShellExecArgs struct {
	Command    string `json:"command" jsonschema:"description=The shell command to execute"`
	WorkingDir string `json:"working_dir,omitempty" jsonschema:"description=Optional working directory for the command"`
	Timeout    int    `json:"timeout,omitempty" jsonschema:"description=Timeout in seconds. Increase for long-running commands (default 60, max 600)"`
}

const maxTimeout = 600

// NewShellTool returns the "shell_exec" tool configured with cfg. Missing
// fields receive sensible defaults (60s timeout, 10k max output, built-in
// deny list).
func NewShellTool(cfg ShellConfig) (tool.InvokableTool, error) {
	if cfg.Timeout == 0 {
		cfg.Timeout = 60 * time.Second
	}
	if cfg.MaxOutput == 0 {
		cfg.MaxOutput = 10000
	}
	if cfg.DenyPatterns == nil {
		cfg.DenyPatterns = defaultDenyPatterns
	}

	compiledDeny := make([]*regexp.Regexp, 0, len(cfg.DenyPatterns))
	for _, p := range cfg.DenyPatterns {
		r, err := regexp.Compile(p)
		if err != nil {
			return nil, fmt.Errorf("invalid deny pattern %q: %w", p, err)
		}
		compiledDeny = append(compiledDeny, r)
	}

	compiledAllow := make([]*regexp.Regexp, 0, len(cfg.AllowPatterns))
	for _, p := range cfg.AllowPatterns {
		r, err := regexp.Compile(p)
		if err != nil {
			return nil, fmt.Errorf("invalid allow pattern %q: %w", p, err)
		}
		compiledAllow = append(compiledAllow, r)
	}

	return utils.InferTool("exec",
		"Execute a shell command and return its output. Use with caution.",
		func(ctx context.Context, args *ShellExecArgs) (string, error) {
			cwd := args.WorkingDir
			if cwd == "" {
				cwd = cfg.WorkingDir
			}
			if cwd == "" {
				cwd, _ = os.Getwd()
			}

			if errMsg := guardCommand(args.Command, cwd, compiledDeny, compiledAllow, cfg.RestrictToWorkspace); errMsg != "" {
				return errMsg, nil
			}

			effectiveTimeout := cfg.Timeout
			if args.Timeout > 0 {
				requested := time.Duration(args.Timeout) * time.Second
				if requested > time.Duration(maxTimeout)*time.Second {
					requested = time.Duration(maxTimeout) * time.Second
				}
				effectiveTimeout = requested
			}

			env := os.Environ()
			if cfg.PathAppend != "" {
				for i, e := range env {
					if strings.HasPrefix(e, "PATH=") {
						env[i] = e + string(os.PathListSeparator) + cfg.PathAppend
						break
					}
				}
			}

			tCtx, cancel := context.WithTimeout(ctx, effectiveTimeout)
			defer cancel()

			cmd := exec.CommandContext(tCtx, "sh", "-c", args.Command)
			cmd.Dir = cwd
			cmd.Env = env

			stdout, err := cmd.Output()
			var stderrText string
			if exitErr, ok := err.(*exec.ExitError); ok {
				stderrText = string(exitErr.Stderr)
			}

			if tCtx.Err() == context.DeadlineExceeded {
				return fmt.Sprintf("Error: Command timed out after %v", effectiveTimeout), nil
			}

			var parts []string
			if len(stdout) > 0 {
				parts = append(parts, string(stdout))
			}
			if strings.TrimSpace(stderrText) != "" {
				parts = append(parts, "STDERR:\n"+stderrText)
			}

			exitCode := 0
			if err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					exitCode = exitErr.ExitCode()
				} else {
					return fmt.Sprintf("Error executing command: %v", err), nil
				}
			}
			parts = append(parts, fmt.Sprintf("\nExit code: %d", exitCode))

			result := strings.Join(parts, "\n")
			if result == "" {
				result = "(no output)"
			}

			maxLen := cfg.MaxOutput
			if len(result) > maxLen {
				half := maxLen / 2
				result = result[:half] +
					fmt.Sprintf("\n\n... (%d chars truncated) ...\n\n", len(result)-maxLen) +
					result[len(result)-half:]
			}

			return result, nil
		})
}

func guardCommand(command, cwd string, deny, allow []*regexp.Regexp, restrictToWorkspace bool) string {
	cmd := strings.TrimSpace(command)
	lower := strings.ToLower(cmd)

	for _, r := range deny {
		if r.MatchString(lower) {
			return "Error: Command blocked by safety guard (dangerous pattern detected)"
		}
	}

	if len(allow) > 0 {
		matched := false
		for _, r := range allow {
			if r.MatchString(lower) {
				matched = true
				break
			}
		}
		if !matched {
			return "Error: Command blocked by safety guard (not in allowlist)"
		}
	}

	if containsInternalURLInCommand(cmd) {
		return "Error: Command blocked by safety guard (internal/private URL detected)"
	}

	if restrictToWorkspace {
		if strings.Contains(cmd, "..\\") || strings.Contains(cmd, "../") {
			return "Error: Command blocked by safety guard (path traversal detected)"
		}

		cwdAbs, _ := filepath.Abs(cwd)
		for _, raw := range extractAbsolutePaths(cmd) {
			expanded := os.ExpandEnv(strings.TrimSpace(raw))
			p, err := filepath.Abs(expanded)
			if err != nil {
				continue
			}
			if filepath.IsAbs(p) && !strings.HasPrefix(p, cwdAbs) {
				return "Error: Command blocked by safety guard (path outside working dir)"
			}
		}
	}

	return ""
}

var (
	posixPathRe = regexp.MustCompile(`(?:^|[\s|>'"])(/[^\s"'>;|<]+)`)
	homePathRe  = regexp.MustCompile(`(?:^|[\s|>'"]) (~[^\s"'>;|<]*)`)
	winPathRe   = regexp.MustCompile(`[A-Za-z]:\\[^\s"'|><;]+`)
	urlRe       = regexp.MustCompile(`https?://[^\s"'<>|;]+`)
)

func extractAbsolutePaths(command string) []string {
	var paths []string
	paths = append(paths, winPathRe.FindAllString(command, -1)...)
	for _, m := range posixPathRe.FindAllStringSubmatch(command, -1) {
		if len(m) > 1 {
			paths = append(paths, m[1])
		}
	}
	for _, m := range homePathRe.FindAllStringSubmatch(command, -1) {
		if len(m) > 1 {
			paths = append(paths, m[1])
		}
	}
	return paths
}

func containsInternalURLInCommand(command string) bool {
	matches := urlRe.FindAllString(command, -1)
	for _, m := range matches {
		u, err := url.Parse(m)
		if err != nil || u.Hostname() == "" {
			continue
		}
		if ok, _ := validateURLTarget(u.Hostname()); !ok {
			return true
		}
	}
	return false
}
