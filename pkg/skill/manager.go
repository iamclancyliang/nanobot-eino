package skill

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// SkillMetadata is the YAML front-matter at the top of a skill file.
type SkillMetadata struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Homepage    string `yaml:"homepage,omitempty"`
	Always      bool   `yaml:"always,omitempty"`
	License     string `yaml:"license,omitempty"`
	Metadata    string `yaml:"metadata,omitempty"`
}

// NanobotMeta is the nanobot-specific extension block parsed from a skill's
// metadata JSON.
type NanobotMeta struct {
	Emoji    string            `json:"emoji,omitempty"`
	Always   bool              `json:"always,omitempty"`
	OS       []string          `json:"os,omitempty"`
	Requires *RequirementsMeta `json:"requires,omitempty"`
	Install  []InstallMeta     `json:"install,omitempty"`
}

// RequirementsMeta lists the binaries and environment variables a skill
// expects to be available before it runs.
type RequirementsMeta struct {
	Bins []string `json:"bins,omitempty"`
	Env  []string `json:"env,omitempty"`
}

// InstallMeta describes one installation step for a skill (Homebrew formula,
// pip package, ...).
type InstallMeta struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Label   string `json:"label,omitempty"`
	Formula string `json:"formula,omitempty"`
	Package string `json:"package,omitempty"`
}

// Skill is one loaded skill: parsed metadata, full markdown body, and the
// origin (workspace overrides builtin).
type Skill struct {
	Meta    SkillMetadata
	Content string
	Path    string
	Source  string // "workspace" or "builtin"
}

// Manager loads and serves skills from a workspace directory and a builtin
// directory.
type Manager struct {
	workspaceSkillsDir string
	builtinSkillsDir   string
	skills             map[string]*Skill
}

// NewManager returns a Manager that loads skills from
// workspaceDir/skills and builtinDir.
func NewManager(workspaceDir, builtinDir string) *Manager {
	return &Manager{
		workspaceSkillsDir: filepath.Join(workspaceDir, "skills"),
		builtinSkillsDir:   builtinDir,
		skills:             make(map[string]*Skill),
	}
}

// LoadSkills (re)scans the workspace and builtin directories. Workspace
// skills shadow builtin ones with the same name.
func (m *Manager) LoadSkills() error {
	m.skills = make(map[string]*Skill)

	if err := m.loadFromDir(m.workspaceSkillsDir, "workspace"); err != nil {
		return err
	}
	if err := m.loadFromDir(m.builtinSkillsDir, "builtin"); err != nil {
		return err
	}
	return nil
}

func (m *Manager) loadFromDir(dir, source string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if _, exists := m.skills[name]; exists {
			continue
		}

		skillPath := filepath.Join(dir, name, "SKILL.md")
		if _, err := os.Stat(skillPath); err != nil {
			continue
		}

		sk, err := m.parseSkill(skillPath, source)
		if err != nil {
			return fmt.Errorf("parse skill %s: %w", name, err)
		}
		m.skills[sk.Meta.Name] = sk
	}
	return nil
}

var frontmatterRe = regexp.MustCompile(`(?s)^---\n(.*?)\n---\n`)

func (m *Manager) parseSkill(path, source string) (*Skill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	raw := string(data)
	parts := strings.SplitN(raw, "---", 3)
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid skill format in %s", path)
	}

	meta := parseFrontmatter(parts[1])

	return &Skill{
		Meta:    meta,
		Content: strings.TrimSpace(parts[2]),
		Path:    path,
		Source:  source,
	}, nil
}

func parseFrontmatter(block string) SkillMetadata {
	var meta SkillMetadata
	for _, line := range strings.Split(strings.TrimSpace(block), "\n") {
		idx := strings.Index(line, ":")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		val = strings.Trim(val, "\"'")

		switch key {
		case "name":
			meta.Name = val
		case "description":
			meta.Description = val
		case "homepage":
			meta.Homepage = val
		case "always":
			meta.Always = val == "true"
		case "license":
			meta.License = val
		case "metadata":
			meta.Metadata = val
		}
	}
	return meta
}

// GetSkill returns the skill with the given name, or nil when not loaded.
func (m *Manager) GetSkill(name string) *Skill {
	return m.skills[name]
}

// ListSkills returns every loaded skill regardless of availability.
func (m *Manager) ListSkills() []*Skill {
	list := make([]*Skill, 0, len(m.skills))
	for _, s := range m.skills {
		list = append(list, s)
	}
	return list
}

// ListAvailableSkills returns only skills whose requirements are met.
// Mirrors Python's list_skills(filter_unavailable=True).
func (m *Manager) ListAvailableSkills() []*Skill {
	var list []*Skill
	for _, s := range m.skills {
		if m.isAvailable(s) {
			list = append(list, s)
		}
	}
	return list
}

// LoadSkill returns the raw SKILL.md content (including frontmatter) for a skill.
// Mirrors Python's load_skill(name) which returns the full file content.
func (m *Manager) LoadSkill(name string) string {
	sk := m.skills[name]
	if sk == nil {
		return ""
	}
	data, err := os.ReadFile(sk.Path)
	if err != nil {
		return ""
	}
	return string(data)
}

// GetAlwaysSkills returns the names of skills marked as always-on (whose
// requirements are also met).
func (m *Manager) GetAlwaysSkills() []string {
	var result []string
	for _, sk := range m.skills {
		if !m.isAvailable(sk) {
			continue
		}
		if sk.Meta.Always || m.getNanobotMeta(sk).Always {
			result = append(result, sk.Meta.Name)
		}
	}
	return result
}

// LoadSkillsForContext concatenates the bodies of the named skills into one
// markdown blob ready to be injected into the system prompt.
func (m *Manager) LoadSkillsForContext(names []string) string {
	var parts []string
	for _, name := range names {
		sk := m.skills[name]
		if sk == nil {
			continue
		}
		// sk.Content is already stripped of frontmatter by parseSkill.
		// Use it directly instead of re-reading from disk on every call.
		if sk.Content == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("### Skill: %s\n\n%s", name, sk.Content))
	}
	return strings.Join(parts, "\n\n---\n\n")
}

// stripFrontmatter removes YAML frontmatter from markdown content.
// Mirrors Python's _strip_frontmatter.
func stripFrontmatter(content string) string {
	if !strings.HasPrefix(content, "---") {
		return content
	}
	loc := frontmatterRe.FindStringIndex(content)
	if loc != nil {
		return strings.TrimSpace(content[loc[1]:])
	}
	return content
}

// BuildSkillsSummary renders an XML <skills> block describing every loaded
// skill, including availability and install hints.
func (m *Manager) BuildSkillsSummary() string {
	allSkills := m.ListSkills()
	if len(allSkills) == 0 {
		return ""
	}

	var lines []string
	lines = append(lines, "<skills>")
	for _, sk := range allSkills {
		name := escapeXML(sk.Meta.Name)
		desc := escapeXML(sk.Meta.Description)
		if desc == "" {
			desc = name
		}
		available := m.isAvailable(sk)

		lines = append(lines, fmt.Sprintf(`  <skill available="%v">`, available))
		lines = append(lines, fmt.Sprintf("    <name>%s</name>", name))
		lines = append(lines, fmt.Sprintf("    <description>%s</description>", desc))
		lines = append(lines, fmt.Sprintf("    <location>%s</location>", sk.Path))

		if !available {
			missing := m.getMissingRequirements(sk)
			if missing != "" {
				lines = append(lines, fmt.Sprintf("    <requires>%s</requires>", escapeXML(missing)))
			}
			if installs := m.getInstallInstructions(sk); installs != "" {
				lines = append(lines, fmt.Sprintf("    <install>%s</install>", escapeXML(installs)))
			}
		}
		lines = append(lines, "  </skill>")
	}
	lines = append(lines, "</skills>")
	return strings.Join(lines, "\n")
}

// GetSkillMetadata returns the parsed frontmatter of a skill as a map.
// Mirrors Python's get_skill_metadata(name).
func (m *Manager) GetSkillMetadata(name string) map[string]string {
	sk := m.skills[name]
	if sk == nil {
		return nil
	}
	result := map[string]string{
		"name":        sk.Meta.Name,
		"description": sk.Meta.Description,
	}
	if sk.Meta.Homepage != "" {
		result["homepage"] = sk.Meta.Homepage
	}
	if sk.Meta.Metadata != "" {
		result["metadata"] = sk.Meta.Metadata
	}
	if sk.Meta.Always {
		result["always"] = "true"
	}
	if sk.Meta.License != "" {
		result["license"] = sk.Meta.License
	}
	return result
}

// BuiltinSkillsDir returns the path of the builtin skills directory.
func (m *Manager) BuiltinSkillsDir() string {
	return m.builtinSkillsDir
}

func (m *Manager) getNanobotMeta(sk *Skill) NanobotMeta {
	if sk.Meta.Metadata == "" {
		return NanobotMeta{}
	}
	var wrapper map[string]json.RawMessage
	if err := json.Unmarshal([]byte(sk.Meta.Metadata), &wrapper); err != nil {
		return NanobotMeta{}
	}

	raw, ok := wrapper["nanobot"]
	if !ok {
		raw, ok = wrapper["openclaw"]
		if !ok {
			return NanobotMeta{}
		}
	}

	var meta NanobotMeta
	if err := json.Unmarshal(raw, &meta); err != nil {
		return NanobotMeta{}
	}
	return meta
}

func (m *Manager) isAvailable(sk *Skill) bool {
	nbm := m.getNanobotMeta(sk)
	if nbm.Requires == nil {
		return true
	}
	for _, bin := range nbm.Requires.Bins {
		if _, err := exec.LookPath(bin); err != nil {
			return false
		}
	}
	for _, env := range nbm.Requires.Env {
		if os.Getenv(env) == "" {
			return false
		}
	}
	return true
}

func (m *Manager) getMissingRequirements(sk *Skill) string {
	nbm := m.getNanobotMeta(sk)
	if nbm.Requires == nil {
		return ""
	}
	var missing []string
	for _, bin := range nbm.Requires.Bins {
		if _, err := exec.LookPath(bin); err != nil {
			missing = append(missing, "CLI: "+bin)
		}
	}
	for _, env := range nbm.Requires.Env {
		if os.Getenv(env) == "" {
			missing = append(missing, "ENV: "+env)
		}
	}
	return strings.Join(missing, ", ")
}

func (m *Manager) getInstallInstructions(sk *Skill) string {
	nbm := m.getNanobotMeta(sk)
	if len(nbm.Install) == 0 {
		return ""
	}
	var parts []string
	for _, inst := range nbm.Install {
		label := inst.Label
		if label == "" {
			label = inst.ID
		}
		switch inst.Kind {
		case "brew":
			if inst.Formula != "" {
				parts = append(parts, fmt.Sprintf("%s: brew install %s", label, inst.Formula))
			}
		case "apt":
			if inst.Package != "" {
				parts = append(parts, fmt.Sprintf("%s: apt install %s", label, inst.Package))
			}
		default:
			parts = append(parts, label)
		}
	}
	return strings.Join(parts, " | ")
}

func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}
