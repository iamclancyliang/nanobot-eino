package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- helpers ---

func createSkillFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	skillDir := filepath.Join(dir, name)
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

const memorySkillMD = `---
name: memory
description: Two-layer memory system with grep-based recall.
always: true
---

# Memory

Use memory tools to store and retrieve facts.
`

const weatherSkillMD = `---
name: weather
description: Get current weather and forecasts for a location.
homepage: https://wttr.in/
metadata: {"nanobot":{"emoji":"🌤️"}}
---

# Weather Skill

Use the weather tool with location parameter.
`

const githubSkillMD = `---
name: github
description: Interact with GitHub using the gh CLI.
metadata: {"nanobot":{"emoji":"🐙","requires":{"bins":["gh"]}}}
---

# GitHub Skill

Use gh CLI commands.
`

const needsEnvSkillMD = `---
name: needs-env
description: Requires a specific env var.
metadata: {"nanobot":{"requires":{"env":["NANOBOT_TEST_SKILL_SECRET_12345"]}}}
---

# Needs Env Skill

Requires NANOBOT_TEST_SKILL_SECRET_12345.
`

const alwaysInMetaSkillMD = `---
name: always-meta
description: Always-on via nanobot metadata.
metadata: {"nanobot":{"always":true,"emoji":"🔁"}}
---

# Always Meta Skill
`

// --- Test: Load from single directory ---

func TestLoadSkillsFromSingleDir(t *testing.T) {
	dir := t.TempDir()
	builtinDir := filepath.Join(dir, "builtin")
	wsDir := filepath.Join(dir, "workspace")
	os.MkdirAll(filepath.Join(wsDir, "skills"), 0755)

	createSkillFile(t, builtinDir, "weather", weatherSkillMD)
	createSkillFile(t, builtinDir, "memory", memorySkillMD)

	m := NewManager(wsDir, builtinDir)
	if err := m.LoadSkills(); err != nil {
		t.Fatalf("LoadSkills failed: %v", err)
	}

	skills := m.ListSkills()
	if len(skills) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(skills))
	}

	w := m.GetSkill("weather")
	if w == nil {
		t.Fatal("weather skill not found")
	}
	if w.Meta.Description != "Get current weather and forecasts for a location." {
		t.Errorf("unexpected description: %s", w.Meta.Description)
	}
	if w.Source != "builtin" {
		t.Errorf("expected source=builtin, got %s", w.Source)
	}
}

// --- Test: Two-tier loading (workspace > builtin priority) ---

func TestWorkspaceSkillOverridesBuiltin(t *testing.T) {
	dir := t.TempDir()
	builtinDir := filepath.Join(dir, "builtin")
	wsDir := filepath.Join(dir, "workspace")
	wsSkillsDir := filepath.Join(wsDir, "skills")

	createSkillFile(t, builtinDir, "weather", weatherSkillMD)

	customWeather := `---
name: weather
description: Custom weather skill from workspace.
---

# Custom Weather
`
	createSkillFile(t, wsSkillsDir, "weather", customWeather)

	m := NewManager(wsDir, builtinDir)
	if err := m.LoadSkills(); err != nil {
		t.Fatalf("LoadSkills failed: %v", err)
	}

	w := m.GetSkill("weather")
	if w == nil {
		t.Fatal("weather skill not found")
	}
	if w.Source != "workspace" {
		t.Errorf("expected workspace to override builtin, got source=%s", w.Source)
	}
	if w.Meta.Description != "Custom weather skill from workspace." {
		t.Errorf("unexpected description: %s", w.Meta.Description)
	}
}

// --- Test: Nonexistent directories are safe ---

func TestLoadSkillsNonexistentDirs(t *testing.T) {
	m := NewManager("/nonexistent/workspace", "/nonexistent/builtin")
	if err := m.LoadSkills(); err != nil {
		t.Fatalf("LoadSkills should not fail for nonexistent dirs: %v", err)
	}
	if len(m.ListSkills()) != 0 {
		t.Errorf("expected 0 skills, got %d", len(m.ListSkills()))
	}
}

// --- Test: Frontmatter parsing ---

func TestParseFrontmatter(t *testing.T) {
	meta := parseFrontmatter(`
name: weather
description: Get current weather and forecasts for a location.
homepage: https://wttr.in/
always: true
license: MIT
metadata: {"nanobot":{"emoji":"🌤️"}}
`)

	if meta.Name != "weather" {
		t.Errorf("expected name=weather, got %s", meta.Name)
	}
	if meta.Description != "Get current weather and forecasts for a location." {
		t.Errorf("unexpected description: %s", meta.Description)
	}
	if meta.Homepage != "https://wttr.in/" {
		t.Errorf("unexpected homepage: %s", meta.Homepage)
	}
	if !meta.Always {
		t.Error("expected always=true")
	}
	if meta.License != "MIT" {
		t.Errorf("unexpected license: %s", meta.License)
	}
	if !strings.Contains(meta.Metadata, "nanobot") {
		t.Errorf("metadata not parsed: %s", meta.Metadata)
	}
}

// --- Test: GetAlwaysSkills (frontmatter always: true) ---

func TestGetAlwaysSkillsFrontmatter(t *testing.T) {
	dir := t.TempDir()
	builtinDir := filepath.Join(dir, "builtin")
	wsDir := filepath.Join(dir, "workspace")
	os.MkdirAll(filepath.Join(wsDir, "skills"), 0755)

	createSkillFile(t, builtinDir, "memory", memorySkillMD)
	createSkillFile(t, builtinDir, "weather", weatherSkillMD)

	m := NewManager(wsDir, builtinDir)
	if err := m.LoadSkills(); err != nil {
		t.Fatal(err)
	}

	always := m.GetAlwaysSkills()
	if len(always) != 1 {
		t.Fatalf("expected 1 always skill, got %d: %v", len(always), always)
	}
	if always[0] != "memory" {
		t.Errorf("expected memory in always skills, got %s", always[0])
	}
}

// --- Test: GetAlwaysSkills (nanobot metadata always: true) ---

func TestGetAlwaysSkillsNanobotMetadata(t *testing.T) {
	dir := t.TempDir()
	builtinDir := filepath.Join(dir, "builtin")
	wsDir := filepath.Join(dir, "workspace")
	os.MkdirAll(filepath.Join(wsDir, "skills"), 0755)

	createSkillFile(t, builtinDir, "always-meta", alwaysInMetaSkillMD)
	createSkillFile(t, builtinDir, "weather", weatherSkillMD)

	m := NewManager(wsDir, builtinDir)
	if err := m.LoadSkills(); err != nil {
		t.Fatal(err)
	}

	always := m.GetAlwaysSkills()
	if len(always) != 1 {
		t.Fatalf("expected 1 always skill, got %d: %v", len(always), always)
	}
	if always[0] != "always-meta" {
		t.Errorf("expected always-meta, got %s", always[0])
	}
}

// --- Test: Dependency checking (bins) ---

func TestIsAvailableWithBins(t *testing.T) {
	dir := t.TempDir()
	builtinDir := filepath.Join(dir, "builtin")
	wsDir := filepath.Join(dir, "workspace")
	os.MkdirAll(filepath.Join(wsDir, "skills"), 0755)

	// "ls" should be available on any POSIX system.
	availableSkill := `---
name: posix-tool
description: Uses ls.
metadata: {"nanobot":{"requires":{"bins":["ls"]}}}
---
# POSIX Tool
`
	createSkillFile(t, builtinDir, "posix-tool", availableSkill)

	// A non-existent binary.
	unavailableSkill := `---
name: missing-tool
description: Uses nonexistent binary.
metadata: {"nanobot":{"requires":{"bins":["__nanobot_test_nonexistent_bin_xyz__"]}}}
---
# Missing Tool
`
	createSkillFile(t, builtinDir, "missing-tool", unavailableSkill)

	m := NewManager(wsDir, builtinDir)
	if err := m.LoadSkills(); err != nil {
		t.Fatal(err)
	}

	posix := m.GetSkill("posix-tool")
	if !m.isAvailable(posix) {
		t.Error("posix-tool should be available (ls exists)")
	}

	missing := m.GetSkill("missing-tool")
	if m.isAvailable(missing) {
		t.Error("missing-tool should NOT be available")
	}
}

// --- Test: Dependency checking (env) ---

func TestIsAvailableWithEnv(t *testing.T) {
	dir := t.TempDir()
	builtinDir := filepath.Join(dir, "builtin")
	wsDir := filepath.Join(dir, "workspace")
	os.MkdirAll(filepath.Join(wsDir, "skills"), 0755)

	createSkillFile(t, builtinDir, "needs-env", needsEnvSkillMD)

	m := NewManager(wsDir, builtinDir)
	if err := m.LoadSkills(); err != nil {
		t.Fatal(err)
	}

	sk := m.GetSkill("needs-env")
	if m.isAvailable(sk) {
		t.Error("needs-env should NOT be available (env var not set)")
	}

	// Set the env var and re-check.
	t.Setenv("NANOBOT_TEST_SKILL_SECRET_12345", "test-value")
	if !m.isAvailable(sk) {
		t.Error("needs-env should be available after setting env var")
	}
}

// --- Test: getMissingRequirements ---

func TestGetMissingRequirements(t *testing.T) {
	dir := t.TempDir()
	builtinDir := filepath.Join(dir, "builtin")
	wsDir := filepath.Join(dir, "workspace")
	os.MkdirAll(filepath.Join(wsDir, "skills"), 0755)

	skill := `---
name: multi-req
description: Needs multiple things.
metadata: {"nanobot":{"requires":{"bins":["__nanobot_test_missing_bin__"],"env":["__NANOBOT_TEST_MISSING_ENV__"]}}}
---
# Multi Req
`
	createSkillFile(t, builtinDir, "multi-req", skill)

	m := NewManager(wsDir, builtinDir)
	if err := m.LoadSkills(); err != nil {
		t.Fatal(err)
	}

	sk := m.GetSkill("multi-req")
	msg := m.getMissingRequirements(sk)
	if !strings.Contains(msg, "CLI: __nanobot_test_missing_bin__") {
		t.Errorf("expected missing bin in message: %s", msg)
	}
	if !strings.Contains(msg, "ENV: __NANOBOT_TEST_MISSING_ENV__") {
		t.Errorf("expected missing env in message: %s", msg)
	}
}

// --- Test: BuildSkillsSummary (XML format) ---

func TestBuildSkillsSummaryXML(t *testing.T) {
	dir := t.TempDir()
	builtinDir := filepath.Join(dir, "builtin")
	wsDir := filepath.Join(dir, "workspace")
	os.MkdirAll(filepath.Join(wsDir, "skills"), 0755)

	createSkillFile(t, builtinDir, "weather", weatherSkillMD)
	createSkillFile(t, builtinDir, "memory", memorySkillMD)

	m := NewManager(wsDir, builtinDir)
	if err := m.LoadSkills(); err != nil {
		t.Fatal(err)
	}

	summary := m.BuildSkillsSummary()
	if !strings.Contains(summary, "<skills>") {
		t.Error("summary should start with <skills>")
	}
	if !strings.Contains(summary, "</skills>") {
		t.Error("summary should end with </skills>")
	}
	if !strings.Contains(summary, "<name>weather</name>") {
		t.Error("summary should contain weather skill")
	}
	if !strings.Contains(summary, "<name>memory</name>") {
		t.Error("summary should contain memory skill")
	}
	if !strings.Contains(summary, `available="true"`) {
		t.Error("summary should mark skills as available")
	}
	if !strings.Contains(summary, "<location>") {
		t.Error("summary should contain location tags")
	}
}

// --- Test: BuildSkillsSummary marks unavailable skills ---

func TestBuildSkillsSummaryUnavailable(t *testing.T) {
	dir := t.TempDir()
	builtinDir := filepath.Join(dir, "builtin")
	wsDir := filepath.Join(dir, "workspace")
	os.MkdirAll(filepath.Join(wsDir, "skills"), 0755)

	unavailable := `---
name: unavailable-skill
description: Requires a missing binary.
metadata: {"nanobot":{"requires":{"bins":["__nanobot_test_fake_bin__"]}}}
---
# Unavailable
`
	createSkillFile(t, builtinDir, "unavailable-skill", unavailable)

	m := NewManager(wsDir, builtinDir)
	if err := m.LoadSkills(); err != nil {
		t.Fatal(err)
	}

	summary := m.BuildSkillsSummary()
	if !strings.Contains(summary, `available="false"`) {
		t.Error("unavailable skill should be marked available=false")
	}
	if !strings.Contains(summary, "<requires>") {
		t.Error("unavailable skill should show missing requirements")
	}
}

// --- Test: BuildSkillsSummary includes install instructions ---

func TestBuildSkillsSummaryInstallInstructions(t *testing.T) {
	dir := t.TempDir()
	builtinDir := filepath.Join(dir, "builtin")
	wsDir := filepath.Join(dir, "workspace")
	os.MkdirAll(filepath.Join(wsDir, "skills"), 0755)

	withInstall := `---
name: needs-install
description: Requires a missing binary with install instructions.
metadata: {"nanobot":{"requires":{"bins":["__nanobot_test_fake_bin__"]},"install":[{"id":"brew","kind":"brew","formula":"steipete/tap/fake-tool","bins":["__nanobot_test_fake_bin__"],"label":"Install fake-tool (brew)"},{"id":"apt","kind":"apt","package":"fake-tool","label":"Install fake-tool (apt)"}]}}
---
# Needs Install
`
	createSkillFile(t, builtinDir, "needs-install", withInstall)

	m := NewManager(wsDir, builtinDir)
	if err := m.LoadSkills(); err != nil {
		t.Fatal(err)
	}

	summary := m.BuildSkillsSummary()
	if !strings.Contains(summary, "<install>") {
		t.Error("unavailable skill with install meta should include <install> tag")
	}
	if !strings.Contains(summary, "brew install steipete/tap/fake-tool") {
		t.Errorf("summary should contain brew install command, got: %s", summary)
	}
	if !strings.Contains(summary, "apt install fake-tool") {
		t.Errorf("summary should contain apt install command, got: %s", summary)
	}
}

// --- Test: BuildSkillsSummary empty ---

func TestBuildSkillsSummaryEmpty(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(filepath.Join(dir, "ws"), filepath.Join(dir, "builtin"))
	if err := m.LoadSkills(); err != nil {
		t.Fatal(err)
	}

	if summary := m.BuildSkillsSummary(); summary != "" {
		t.Errorf("expected empty summary, got: %s", summary)
	}
}

// --- Test: LoadSkillsForContext strips frontmatter ---

func TestLoadSkillsForContextStripsFrontmatter(t *testing.T) {
	dir := t.TempDir()
	builtinDir := filepath.Join(dir, "builtin")
	wsDir := filepath.Join(dir, "workspace")
	os.MkdirAll(filepath.Join(wsDir, "skills"), 0755)

	createSkillFile(t, builtinDir, "memory", memorySkillMD)

	m := NewManager(wsDir, builtinDir)
	if err := m.LoadSkills(); err != nil {
		t.Fatal(err)
	}

	ctx := m.LoadSkillsForContext([]string{"memory"})
	if strings.Contains(ctx, "always: true") {
		t.Error("LoadSkillsForContext should strip frontmatter content")
	}
	if !strings.Contains(ctx, "### Skill: memory") {
		t.Error("LoadSkillsForContext should include skill header")
	}
	if !strings.Contains(ctx, "# Memory") {
		t.Error("LoadSkillsForContext should include skill body")
	}
}

// --- Test: LoadSkillsForContext with multiple skills ---

func TestLoadSkillsForContextMultiple(t *testing.T) {
	dir := t.TempDir()
	builtinDir := filepath.Join(dir, "builtin")
	wsDir := filepath.Join(dir, "workspace")
	os.MkdirAll(filepath.Join(wsDir, "skills"), 0755)

	createSkillFile(t, builtinDir, "memory", memorySkillMD)
	createSkillFile(t, builtinDir, "weather", weatherSkillMD)

	m := NewManager(wsDir, builtinDir)
	if err := m.LoadSkills(); err != nil {
		t.Fatal(err)
	}

	ctx := m.LoadSkillsForContext([]string{"memory", "weather"})
	if !strings.Contains(ctx, "### Skill: memory") {
		t.Error("should contain memory skill")
	}
	if !strings.Contains(ctx, "### Skill: weather") {
		t.Error("should contain weather skill")
	}
	if !strings.Contains(ctx, "---") {
		t.Error("multiple skills should be separated by ---")
	}
}

// --- Test: LoadSkillsForContext with nonexistent skill ---

func TestLoadSkillsForContextNonexistent(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(filepath.Join(dir, "ws"), filepath.Join(dir, "builtin"))
	if err := m.LoadSkills(); err != nil {
		t.Fatal(err)
	}

	ctx := m.LoadSkillsForContext([]string{"nonexistent"})
	if ctx != "" {
		t.Errorf("expected empty for nonexistent skill, got: %s", ctx)
	}
}

// --- Test: getNanobotMeta parsing ---

func TestGetNanobotMeta(t *testing.T) {
	dir := t.TempDir()
	builtinDir := filepath.Join(dir, "builtin")
	wsDir := filepath.Join(dir, "workspace")
	os.MkdirAll(filepath.Join(wsDir, "skills"), 0755)

	createSkillFile(t, builtinDir, "github", githubSkillMD)
	createSkillFile(t, builtinDir, "weather", weatherSkillMD)

	m := NewManager(wsDir, builtinDir)
	if err := m.LoadSkills(); err != nil {
		t.Fatal(err)
	}

	gh := m.GetSkill("github")
	meta := m.getNanobotMeta(gh)
	if meta.Emoji != "🐙" {
		t.Errorf("expected emoji=🐙, got %s", meta.Emoji)
	}
	if meta.Requires == nil {
		t.Fatal("github skill should have requires")
	}
	if len(meta.Requires.Bins) != 1 || meta.Requires.Bins[0] != "gh" {
		t.Errorf("expected requires.bins=[gh], got %v", meta.Requires.Bins)
	}

	w := m.GetSkill("weather")
	wMeta := m.getNanobotMeta(w)
	if wMeta.Emoji != "🌤️" {
		t.Errorf("expected emoji=🌤️, got %s", wMeta.Emoji)
	}
	if wMeta.Requires != nil {
		t.Error("weather skill should not have requires")
	}
}

// --- Test: getNanobotMeta with empty metadata ---

func TestGetNanobotMetaEmpty(t *testing.T) {
	sk := &Skill{Meta: SkillMetadata{Metadata: ""}}
	m := &Manager{skills: map[string]*Skill{}}
	meta := m.getNanobotMeta(sk)
	if meta.Emoji != "" || meta.Always || meta.Requires != nil {
		t.Error("empty metadata should return zero NanobotMeta")
	}
}

// --- Test: getNanobotMeta with openclaw key ---

func TestGetNanobotMetaOpenclawKey(t *testing.T) {
	dir := t.TempDir()
	builtinDir := filepath.Join(dir, "builtin")
	wsDir := filepath.Join(dir, "workspace")
	os.MkdirAll(filepath.Join(wsDir, "skills"), 0755)

	ocSkill := `---
name: oc-skill
description: Uses openclaw key.
metadata: {"openclaw":{"emoji":"🦞","always":true}}
---
# OpenClaw Skill
`
	createSkillFile(t, builtinDir, "oc-skill", ocSkill)

	m := NewManager(wsDir, builtinDir)
	if err := m.LoadSkills(); err != nil {
		t.Fatal(err)
	}

	sk := m.GetSkill("oc-skill")
	meta := m.getNanobotMeta(sk)
	if meta.Emoji != "🦞" {
		t.Errorf("expected emoji=🦞 from openclaw key, got %s", meta.Emoji)
	}
	if !meta.Always {
		t.Error("expected always=true from openclaw key")
	}
}

// --- Test: Invalid skill format ---

func TestInvalidSkillFormat(t *testing.T) {
	dir := t.TempDir()
	builtinDir := filepath.Join(dir, "builtin")
	wsDir := filepath.Join(dir, "workspace")
	os.MkdirAll(filepath.Join(wsDir, "skills"), 0755)

	invalidDir := filepath.Join(builtinDir, "bad-skill")
	os.MkdirAll(invalidDir, 0755)
	os.WriteFile(filepath.Join(invalidDir, "SKILL.md"), []byte("No frontmatter here"), 0644)

	m := NewManager(wsDir, builtinDir)
	err := m.LoadSkills()
	if err == nil {
		t.Error("expected error for invalid skill format")
	}
	if !strings.Contains(err.Error(), "invalid skill format") {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- Test: Directory without SKILL.md is ignored ---

func TestDirWithoutSkillMDIgnored(t *testing.T) {
	dir := t.TempDir()
	builtinDir := filepath.Join(dir, "builtin")
	wsDir := filepath.Join(dir, "workspace")
	os.MkdirAll(filepath.Join(wsDir, "skills"), 0755)

	os.MkdirAll(filepath.Join(builtinDir, "not-a-skill"), 0755)
	os.WriteFile(filepath.Join(builtinDir, "not-a-skill", "README.md"), []byte("not a skill"), 0644)

	createSkillFile(t, builtinDir, "weather", weatherSkillMD)

	m := NewManager(wsDir, builtinDir)
	if err := m.LoadSkills(); err != nil {
		t.Fatal(err)
	}
	if len(m.ListSkills()) != 1 {
		t.Errorf("expected 1 skill (not-a-skill dir should be ignored), got %d", len(m.ListSkills()))
	}
}

// --- Test: escapeXML ---

func TestEscapeXML(t *testing.T) {
	tests := []struct {
		input, expected string
	}{
		{"hello", "hello"},
		{"a & b", "a &amp; b"},
		{"<tag>", "&lt;tag&gt;"},
		{"a & <b>", "a &amp; &lt;b&gt;"},
	}
	for _, tc := range tests {
		got := escapeXML(tc.input)
		if got != tc.expected {
			t.Errorf("escapeXML(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

// --- Test: BuiltinSkillsDir accessor ---

func TestBuiltinSkillsDir(t *testing.T) {
	m := NewManager("/ws", "/builtin/skills")
	if got := m.BuiltinSkillsDir(); got != "/builtin/skills" {
		t.Errorf("expected /builtin/skills, got %s", got)
	}
}

// --- Test: GetSkill returns nil for unknown ---

func TestGetSkillUnknown(t *testing.T) {
	m := NewManager("/ws", "/builtin")
	if err := m.LoadSkills(); err != nil {
		t.Fatal(err)
	}
	if sk := m.GetSkill("nonexistent"); sk != nil {
		t.Error("expected nil for unknown skill")
	}
}

// --- Test: LoadSkill returns empty string for unknown ---

func TestLoadSkillUnknown(t *testing.T) {
	m := NewManager("/ws", "/builtin")
	if err := m.LoadSkills(); err != nil {
		t.Fatal(err)
	}
	if content := m.LoadSkill("nonexistent"); content != "" {
		t.Errorf("expected empty for unknown skill, got: %s", content)
	}
}

// --- Test: ListAvailableSkills filters unavailable ---

func TestListAvailableSkills(t *testing.T) {
	dir := t.TempDir()
	builtinDir := filepath.Join(dir, "builtin")
	wsDir := filepath.Join(dir, "workspace")
	os.MkdirAll(filepath.Join(wsDir, "skills"), 0755)

	createSkillFile(t, builtinDir, "weather", weatherSkillMD)
	createSkillFile(t, builtinDir, "github", githubSkillMD)

	m := NewManager(wsDir, builtinDir)
	if err := m.LoadSkills(); err != nil {
		t.Fatal(err)
	}

	all := m.ListSkills()
	available := m.ListAvailableSkills()

	if len(all) != 2 {
		t.Fatalf("expected 2 total skills, got %d", len(all))
	}

	// weather has no requires → available; github requires "gh" → may or may not be available
	hasWeather := false
	for _, s := range available {
		if s.Meta.Name == "weather" {
			hasWeather = true
		}
	}
	if !hasWeather {
		t.Error("weather should be in available skills (no requires)")
	}

	if len(available) > len(all) {
		t.Error("available skills should be <= total skills")
	}
}

// --- Test: GetSkillMetadata ---

func TestGetSkillMetadata(t *testing.T) {
	dir := t.TempDir()
	builtinDir := filepath.Join(dir, "builtin")
	wsDir := filepath.Join(dir, "workspace")
	os.MkdirAll(filepath.Join(wsDir, "skills"), 0755)

	createSkillFile(t, builtinDir, "memory", memorySkillMD)
	createSkillFile(t, builtinDir, "weather", weatherSkillMD)

	m := NewManager(wsDir, builtinDir)
	if err := m.LoadSkills(); err != nil {
		t.Fatal(err)
	}

	meta := m.GetSkillMetadata("memory")
	if meta == nil {
		t.Fatal("expected metadata for memory skill")
	}
	if meta["name"] != "memory" {
		t.Errorf("expected name=memory, got %s", meta["name"])
	}
	if meta["always"] != "true" {
		t.Error("expected always=true for memory skill")
	}

	wMeta := m.GetSkillMetadata("weather")
	if wMeta == nil {
		t.Fatal("expected metadata for weather skill")
	}
	if wMeta["homepage"] != "https://wttr.in/" {
		t.Errorf("expected homepage, got %s", wMeta["homepage"])
	}
	if _, hasAlways := wMeta["always"]; hasAlways {
		t.Error("weather should not have always key in metadata")
	}

	nilMeta := m.GetSkillMetadata("nonexistent")
	if nilMeta != nil {
		t.Error("expected nil for nonexistent skill metadata")
	}
}

// --- Test: LoadSkill returns raw content with frontmatter ---

func TestLoadSkillReturnsRawContent(t *testing.T) {
	dir := t.TempDir()
	builtinDir := filepath.Join(dir, "builtin")
	wsDir := filepath.Join(dir, "workspace")
	os.MkdirAll(filepath.Join(wsDir, "skills"), 0755)

	createSkillFile(t, builtinDir, "memory", memorySkillMD)

	m := NewManager(wsDir, builtinDir)
	if err := m.LoadSkills(); err != nil {
		t.Fatal(err)
	}

	content := m.LoadSkill("memory")
	if !strings.HasPrefix(content, "---") {
		t.Error("LoadSkill should return raw content including frontmatter")
	}
	if !strings.Contains(content, "always: true") {
		t.Error("LoadSkill should include frontmatter fields")
	}
	if !strings.Contains(content, "# Memory") {
		t.Error("LoadSkill should include body")
	}
}

// --- Test: stripFrontmatter ---

func TestStripFrontmatter(t *testing.T) {
	input := "---\nname: test\ndescription: A test.\n---\n\n# Test Skill\n\nBody content."
	result := stripFrontmatter(input)
	if strings.Contains(result, "---") {
		t.Error("stripFrontmatter should remove frontmatter markers")
	}
	if strings.Contains(result, "name: test") {
		t.Error("stripFrontmatter should remove frontmatter content")
	}
	if !strings.Contains(result, "# Test Skill") {
		t.Error("stripFrontmatter should keep body")
	}

	noFM := "# No Frontmatter\n\nJust content."
	if stripFrontmatter(noFM) != noFM {
		t.Error("stripFrontmatter should return content unchanged if no frontmatter")
	}
}

// --- Test: Real configs/skills directory loads correctly ---

func TestLoadRealConfigSkills(t *testing.T) {
	builtinDir := "../../configs/skills"
	if _, err := os.Stat(builtinDir); os.IsNotExist(err) {
		t.Skip("configs/skills not found, skipping integration test")
	}

	wsDir := t.TempDir()
	os.MkdirAll(filepath.Join(wsDir, "skills"), 0755)

	m := NewManager(wsDir, builtinDir)
	if err := m.LoadSkills(); err != nil {
		t.Fatalf("Failed to load real config skills: %v", err)
	}

	expectedSkills := []string{"memory", "weather", "summarize", "skill-creator", "github", "cron", "tmux", "clawhub"}
	for _, name := range expectedSkills {
		if sk := m.GetSkill(name); sk == nil {
			t.Errorf("%s skill should exist in configs/skills", name)
		}
	}

	if mem := m.GetSkill("memory"); mem != nil {
		if !mem.Meta.Always {
			t.Error("memory skill should have always=true")
		}
	}

	if w := m.GetSkill("weather"); w != nil {
		wMeta := m.getNanobotMeta(w)
		if wMeta.Emoji != "🌤️" {
			t.Errorf("expected weather emoji=🌤️, got %s", wMeta.Emoji)
		}
		if wMeta.Requires == nil || len(wMeta.Requires.Bins) != 1 || wMeta.Requires.Bins[0] != "curl" {
			t.Errorf("weather should require curl, got %+v", wMeta.Requires)
		}
	}

	if s := m.GetSkill("summarize"); s != nil {
		if s.Meta.Homepage != "https://summarize.sh" {
			t.Errorf("summarize homepage mismatch: %s", s.Meta.Homepage)
		}
		sMeta := m.getNanobotMeta(s)
		if sMeta.Emoji != "🧾" {
			t.Errorf("expected summarize emoji=🧾, got %s", sMeta.Emoji)
		}
		if sMeta.Requires == nil {
			t.Error("summarize should declare requires.bins")
		} else if len(sMeta.Requires.Bins) != 1 || sMeta.Requires.Bins[0] != "summarize" {
			t.Errorf("expected summarize requires.bins=[summarize], got %v", sMeta.Requires.Bins)
		}
	}

	if gh := m.GetSkill("github"); gh != nil {
		ghMeta := m.getNanobotMeta(gh)
		if ghMeta.Requires == nil || len(ghMeta.Requires.Bins) != 1 || ghMeta.Requires.Bins[0] != "gh" {
			t.Errorf("github should require gh CLI, got %+v", ghMeta.Requires)
		}
	}

	if tmuxSk := m.GetSkill("tmux"); tmuxSk != nil {
		tmuxMeta := m.getNanobotMeta(tmuxSk)
		if tmuxMeta.Requires == nil || len(tmuxMeta.Requires.Bins) != 1 || tmuxMeta.Requires.Bins[0] != "tmux" {
			t.Errorf("tmux should require tmux binary, got %+v", tmuxMeta.Requires)
		}
	}

	always := m.GetAlwaysSkills()
	found := false
	for _, name := range always {
		if name == "memory" {
			found = true
		}
	}
	if !found {
		t.Errorf("memory should be in always skills, got: %v", always)
	}

	summary := m.BuildSkillsSummary()
	if summary == "" {
		t.Error("summary should not be empty for real skills")
	}
	if !strings.Contains(summary, "<skills>") {
		t.Error("summary should be in XML format")
	}

	allSkills := m.ListSkills()
	if len(allSkills) != 8 {
		t.Errorf("expected 8 builtin skills, got %d", len(allSkills))
	}
}

// TestLoadSkillsForContext_UsesInMemoryContent verifies that LoadSkillsForContext
// uses the skill content already loaded in memory rather than re-reading from disk
// on every call. After deleting the skill file, the method must still return the
// correct content (proving it no longer hits the disk each time).
func TestLoadSkillsForContext_UsesInMemoryContent(t *testing.T) {
	dir := t.TempDir()
	builtinDir := filepath.Join(dir, "builtin")
	wsDir := filepath.Join(dir, "workspace")
	os.MkdirAll(filepath.Join(wsDir, "skills"), 0755)

	createSkillFile(t, builtinDir, "memory", memorySkillMD)

	m := NewManager(wsDir, builtinDir)
	if err := m.LoadSkills(); err != nil {
		t.Fatalf("LoadSkills failed: %v", err)
	}

	// Delete the file from disk AFTER loading.
	os.RemoveAll(filepath.Join(builtinDir, "memory"))

	// LoadSkillsForContext must still return the content (from in-memory cache).
	result := m.LoadSkillsForContext([]string{"memory"})
	if result == "" {
		t.Error("LoadSkillsForContext returned empty after file deletion — it re-reads from disk instead of using in-memory content")
	}
	if !strings.Contains(result, "Use memory tools") {
		t.Errorf("expected skill content in result, got: %q", result)
	}
}
