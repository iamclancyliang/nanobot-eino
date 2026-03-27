package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

// ---------------------------------------------------------------------------
// Path resolution — mirrors nanobot's _resolve_path / _is_under
// ---------------------------------------------------------------------------

func resolvePath(path string, workspace, allowedDir string, extraAllowedDirs []string) (string, error) {
	p := path
	if strings.HasPrefix(p, "~") {
		home, _ := os.UserHomeDir()
		p = filepath.Join(home, p[1:])
	}
	if !filepath.IsAbs(p) && workspace != "" {
		p = filepath.Join(workspace, p)
	}
	resolved, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	resolved = filepath.Clean(resolved)

	if allowedDir != "" {
		allDirs := append([]string{allowedDir}, extraAllowedDirs...)
		allowed := false
		for _, d := range allDirs {
			ad, _ := filepath.Abs(d)
			if isUnderDir(resolved, ad) {
				allowed = true
				break
			}
		}
		if !allowed {
			return "", fmt.Errorf("path %s is outside allowed directory %s", path, allowedDir)
		}
	}

	return resolved, nil
}

func isUnderDir(path, dir string) bool {
	dir = filepath.Clean(dir)
	path = filepath.Clean(path)
	if path == dir {
		return true
	}
	return strings.HasPrefix(path, dir+string(filepath.Separator))
}

// ---------------------------------------------------------------------------
// read_file
// ---------------------------------------------------------------------------

type ReadFileArgs struct {
	Path   string `json:"path" jsonschema:"description=The file path to read"`
	Offset int    `json:"offset,omitempty" jsonschema:"description=Line number to start reading from (1-indexed, default 1)"`
	Limit  int    `json:"limit,omitempty" jsonschema:"description=Maximum number of lines to read (default 2000)"`
}

func NewReadFileTool(workspace, allowedDir string, extraAllowedDirs ...string) tool.InvokableTool {
	const maxChars = 128_000
	const defaultLimit = 2000

	t, _ := utils.InferTool("read_file",
		"Read the contents of a file (including SKILL.md files). Returns numbered lines. Use offset and limit to paginate through large files.",
		func(ctx context.Context, args *ReadFileArgs) (string, error) {
			fp, err := resolvePath(args.Path, workspace, allowedDir, extraAllowedDirs)
			if err != nil {
				return fmt.Sprintf("Error: %v", err), nil
			}

			info, err := os.Stat(fp)
			if os.IsNotExist(err) {
				return fmt.Sprintf("Error: File not found: %s", args.Path), nil
			}
			if err != nil {
				return fmt.Sprintf("Error: %v", err), nil
			}
			if info.IsDir() {
				return fmt.Sprintf("Error: Not a file: %s", args.Path), nil
			}

			content, err := os.ReadFile(fp)
			if err != nil {
				return fmt.Sprintf("Error: %v", err), nil
			}

			lines := strings.Split(string(content), "\n")
			total := len(lines)

			if total == 0 {
				return fmt.Sprintf("(Empty file: %s)", args.Path), nil
			}

			offset := args.Offset
			if offset < 1 {
				offset = 1
			}
			if offset > total {
				return fmt.Sprintf("Error: offset %d is beyond end of file (%d lines)", offset, total), nil
			}

			limit := args.Limit
			if limit <= 0 {
				limit = defaultLimit
			}

			start := offset - 1
			end := start + limit
			if end > total {
				end = total
			}

			var sb strings.Builder
			currentChars := 0
			actualEnd := start

			for i := start; i < end; i++ {
				line := fmt.Sprintf("%d| %s\n", i+1, lines[i])
				if currentChars+len(line) > maxChars {
					break
				}
				sb.WriteString(line)
				currentChars += len(line)
				actualEnd = i + 1
			}

			result := sb.String()
			if actualEnd < total {
				result += fmt.Sprintf("\n(Showing lines %d-%d of %d. Use offset=%d to continue.)", offset, actualEnd, total, actualEnd+1)
			} else {
				result += fmt.Sprintf("\n(End of file — %d lines total)", total)
			}
			return result, nil
		})
	return t
}

// ---------------------------------------------------------------------------
// write_file
// ---------------------------------------------------------------------------

type WriteFileArgs struct {
	Path    string `json:"path" jsonschema:"description=The file path to write to"`
	Content string `json:"content" jsonschema:"description=The content to write"`
}

func NewWriteFileTool(workspace, allowedDir string) tool.InvokableTool {
	t, _ := utils.InferTool("write_file",
		"Write content to a file at the given path. Creates parent directories if needed.",
		func(ctx context.Context, args *WriteFileArgs) (string, error) {
			fp, err := resolvePath(args.Path, workspace, allowedDir, nil)
			if err != nil {
				return fmt.Sprintf("Error: %v", err), nil
			}

			if err := os.MkdirAll(filepath.Dir(fp), 0755); err != nil {
				return fmt.Sprintf("Error creating directories: %v", err), nil
			}

			if err := os.WriteFile(fp, []byte(args.Content), 0644); err != nil {
				return fmt.Sprintf("Error writing file: %v", err), nil
			}

			return fmt.Sprintf("Successfully wrote %d bytes to %s", len(args.Content), fp), nil
		})
	return t
}

// ---------------------------------------------------------------------------
// edit_file — with fallback matching and similarity diagnostics
// ---------------------------------------------------------------------------

type EditFileArgs struct {
	Path       string `json:"path" jsonschema:"description=The file path to edit"`
	OldText    string `json:"old_text" jsonschema:"description=The text to find and replace"`
	NewText    string `json:"new_text" jsonschema:"description=The text to replace with"`
	ReplaceAll bool   `json:"replace_all,omitempty" jsonschema:"description=Replace all occurrences (default false)"`
}

func NewEditFileTool(workspace, allowedDir string) tool.InvokableTool {
	t, _ := utils.InferTool("edit_file",
		"Edit a file by replacing old_text with new_text. Supports minor whitespace/line-ending differences. Set replace_all=true to replace every occurrence.",
		func(ctx context.Context, args *EditFileArgs) (string, error) {
			fp, err := resolvePath(args.Path, workspace, allowedDir, nil)
			if err != nil {
				return fmt.Sprintf("Error: %v", err), nil
			}

			data, err := os.ReadFile(fp)
			if os.IsNotExist(err) {
				return fmt.Sprintf("Error: File not found: %s", args.Path), nil
			}
			if err != nil {
				return fmt.Sprintf("Error: %v", err), nil
			}

			content := string(data)
			usesCRLF := strings.Contains(content, "\r\n")
			normContent := strings.ReplaceAll(content, "\r\n", "\n")
			oldText := strings.ReplaceAll(args.OldText, "\r\n", "\n")
			newText := strings.ReplaceAll(args.NewText, "\r\n", "\n")

			match, count := findMatch(normContent, oldText)
			if match == "" {
				return notFoundMessage(oldText, normContent, args.Path), nil
			}

			if count > 1 && !args.ReplaceAll {
				return fmt.Sprintf(
					"Warning: old_text appears %d times. Provide more context to make it unique, or set replace_all=true.",
					count,
				), nil
			}

			var updated string
			if args.ReplaceAll {
				updated = strings.ReplaceAll(normContent, match, newText)
			} else {
				updated = strings.Replace(normContent, match, newText, 1)
			}

			if usesCRLF {
				updated = strings.ReplaceAll(updated, "\n", "\r\n")
			}

			if err := os.WriteFile(fp, []byte(updated), 0644); err != nil {
				return fmt.Sprintf("Error writing file: %v", err), nil
			}

			return fmt.Sprintf("Successfully edited %s", fp), nil
		})
	return t
}

func findMatch(content, oldText string) (string, int) {
	if strings.Contains(content, oldText) {
		return oldText, strings.Count(content, oldText)
	}

	oldLines := strings.Split(oldText, "\n")
	if len(oldLines) == 0 {
		return "", 0
	}

	strippedOld := make([]string, len(oldLines))
	for i, l := range oldLines {
		strippedOld[i] = strings.TrimSpace(l)
	}

	contentLines := strings.Split(content, "\n")
	var candidates []string

	for i := 0; i <= len(contentLines)-len(strippedOld); i++ {
		window := contentLines[i : i+len(strippedOld)]
		match := true
		for j, line := range window {
			if strings.TrimSpace(line) != strippedOld[j] {
				match = false
				break
			}
		}
		if match {
			candidates = append(candidates, strings.Join(window, "\n"))
		}
	}

	if len(candidates) > 0 {
		return candidates[0], len(candidates)
	}
	return "", 0
}

func notFoundMessage(oldText, content, path string) string {
	contentLines := strings.Split(content, "\n")
	oldLines := strings.Split(oldText, "\n")
	window := len(oldLines)

	bestRatio := 0.0
	bestStart := 0

	maxStart := len(contentLines) - window + 1
	if maxStart < 1 {
		maxStart = 1
	}

	for i := 0; i < maxStart; i++ {
		end := i + window
		if end > len(contentLines) {
			end = len(contentLines)
		}
		candidate := contentLines[i:end]
		ratio := similarityRatio(oldLines, candidate)
		if ratio > bestRatio {
			bestRatio = ratio
			bestStart = i
		}
	}

	if bestRatio > 0.5 {
		end := bestStart + window
		if end > len(contentLines) {
			end = len(contentLines)
		}
		actual := contentLines[bestStart:end]

		var diffLines []string
		diffLines = append(diffLines, fmt.Sprintf("--- old_text (provided)"))
		diffLines = append(diffLines, fmt.Sprintf("+++ %s (actual, line %d)", path, bestStart+1))
		for i := 0; i < len(oldLines) || i < len(actual); i++ {
			if i < len(oldLines) && i < len(actual) {
				if oldLines[i] != actual[i] {
					diffLines = append(diffLines, "- "+oldLines[i])
					diffLines = append(diffLines, "+ "+actual[i])
				} else {
					diffLines = append(diffLines, "  "+oldLines[i])
				}
			} else if i < len(oldLines) {
				diffLines = append(diffLines, "- "+oldLines[i])
			} else {
				diffLines = append(diffLines, "+ "+actual[i])
			}
		}

		return fmt.Sprintf(
			"Error: old_text not found in %s.\nBest match (%.0f%% similar) at line %d:\n%s",
			path, bestRatio*100, bestStart+1, strings.Join(diffLines, "\n"),
		)
	}

	return fmt.Sprintf("Error: old_text not found in %s. No similar text found. Verify the file content.", path)
}

func similarityRatio(a, b []string) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1.0
	}
	matches := 0
	total := max(len(a), len(b))
	for i := 0; i < min(len(a), len(b)); i++ {
		if a[i] == b[i] {
			matches++
		} else {
			lineRatio := lineSimilarity(a[i], b[i])
			if lineRatio > 0.6 {
				matches++
			}
		}
	}
	return float64(matches) / float64(total)
}

func lineSimilarity(a, b string) float64 {
	if a == b {
		return 1.0
	}
	if len(a) == 0 || len(b) == 0 {
		return 0.0
	}
	lcs := longestCommonSubsequence(a, b)
	return 2.0 * float64(lcs) / float64(len(a)+len(b))
}

func longestCommonSubsequence(a, b string) int {
	m, n := len(a), len(b)
	prev := make([]int, n+1)
	curr := make([]int, n+1)
	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if a[i-1] == b[j-1] {
				curr[j] = prev[j-1] + 1
			} else {
				curr[j] = max(prev[j], curr[j-1])
			}
		}
		prev, curr = curr, make([]int, n+1)
	}
	return prev[n]
}

// ---------------------------------------------------------------------------
// list_dir
// ---------------------------------------------------------------------------

var defaultIgnoreDirs = map[string]bool{
	".git": true, "node_modules": true, "__pycache__": true, ".venv": true, "venv": true,
	"dist": true, "build": true, ".tox": true, ".mypy_cache": true, ".pytest_cache": true,
	".ruff_cache": true, ".coverage": true, "htmlcov": true,
}

type ListDirArgs struct {
	Path       string `json:"path" jsonschema:"description=The directory path to list"`
	Recursive  bool   `json:"recursive,omitempty" jsonschema:"description=Recursively list all files (default false)"`
	MaxEntries int    `json:"max_entries,omitempty" jsonschema:"description=Maximum entries to return (default 200)"`
}

func NewListDirTool(workspace, allowedDir string) tool.InvokableTool {
	t, _ := utils.InferTool("list_dir",
		"List the contents of a directory. Set recursive=true to explore nested structure. Common noise directories (.git, node_modules, __pycache__, etc.) are auto-ignored.",
		func(ctx context.Context, args *ListDirArgs) (string, error) {
			dp, err := resolvePath(args.Path, workspace, allowedDir, nil)
			if err != nil {
				return fmt.Sprintf("Error: %v", err), nil
			}

			info, statErr := os.Stat(dp)
			if os.IsNotExist(statErr) {
				return fmt.Sprintf("Error: Directory not found: %s", args.Path), nil
			}
			if statErr != nil {
				return fmt.Sprintf("Error: %v", statErr), nil
			}
			if !info.IsDir() {
				return fmt.Sprintf("Error: Not a directory: %s", args.Path), nil
			}

			maxEntries := args.MaxEntries
			if maxEntries <= 0 {
				maxEntries = 200
			}

			var items []string
			total := 0

			if args.Recursive {
				err = filepath.Walk(dp, func(p string, fi os.FileInfo, walkErr error) error {
					if walkErr != nil {
						return walkErr
					}
					if fi.IsDir() && defaultIgnoreDirs[fi.Name()] {
						return filepath.SkipDir
					}
					rel, _ := filepath.Rel(dp, p)
					if rel == "." {
						return nil
					}
					total++
					if len(items) < maxEntries {
						if fi.IsDir() {
							items = append(items, rel+"/")
						} else {
							items = append(items, rel)
						}
					}
					return nil
				})
				if err != nil {
					return fmt.Sprintf("Error walking directory: %v", err), nil
				}
			} else {
				entries, readErr := os.ReadDir(dp)
				if readErr != nil {
					return fmt.Sprintf("Error reading directory: %v", readErr), nil
				}
				for _, entry := range entries {
					if defaultIgnoreDirs[entry.Name()] {
						continue
					}
					total++
					if len(items) < maxEntries {
						pfx := "📄 "
						if entry.IsDir() {
							pfx = "📁 "
						}
						items = append(items, pfx+entry.Name())
					}
				}
			}

			if len(items) == 0 && total == 0 {
				return fmt.Sprintf("Directory %s is empty", args.Path), nil
			}

			sort.Strings(items)
			result := strings.Join(items, "\n")
			if total > maxEntries {
				result += fmt.Sprintf("\n\n(truncated, showing first %d of %d entries)", maxEntries, total)
			}
			return result, nil
		})
	return t
}
