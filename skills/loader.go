package skills

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

// SkillMeta represents the parsed YAML frontmatter from a SKILL.md file.
type SkillMeta struct {
	Name        string
	Description string
	Always      bool
	Available   bool
	Commands    []string
	Tags        []string
	Version     string
	Requires    []string // simple dependency list (files, tools, env vars)
	ExtBins     []string // extended_requires.bins
	ExtEnv      []string // extended_requires.env
	WhenToUse   string   // when_to_use field describing ideal usage scenarios
}

// SkillInfo represents metadata about a skill.
type SkillInfo struct {
	Name        string   `json:"name"`
	Path        string   `json:"path"`
	Source      string   `json:"source"` // "builtin" or "workspace"
	Available   bool     `json:"available"`
	Always      bool     `json:"always"`
	Description string   `json:"description"`
	Commands    []string `json:"commands"`
	Tags        []string `json:"tags"`
	Version     string   `json:"version"`
	MissingDeps []string `json:"missing_deps,omitempty"`
	WhenToUse   string   `json:"when_to_use,omitempty"`
}

// Loader loads and parses skill definitions.
type Loader struct {
	workspace  string
	builtinDir string
	cache      map[string]string    // name -> full file content
	skillIndex map[string]SkillInfo // name -> SkillInfo
	mu         sync.RWMutex
}

// NewLoader creates a new skill loader.
func NewLoader(workspace string) *Loader {
	return &Loader{
		workspace:  filepath.Join(workspace, "skills"),
		builtinDir: "",
		cache:      make(map[string]string),
		skillIndex: make(map[string]SkillInfo),
	}
}

// SetBuiltinDir sets the builtin skills directory.
func (l *Loader) SetBuiltinDir(dir string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.builtinDir = dir
}

// Refresh re-scans skill directories and rebuilds the index.
func (l *Loader) Refresh() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.cache = make(map[string]string)
	l.skillIndex = make(map[string]SkillInfo)

	if l.builtinDir != "" {
		l.scanDirLocked(l.builtinDir, "builtin")
	}
	l.scanDirLocked(l.workspace, "workspace")

	return nil
}

// LoadSkill returns the full SKILL.md content for a named skill.
func (l *Loader) LoadSkill(name string) string {
	l.mu.RLock()
	defer l.mu.RUnlock()

	wsPath := filepath.Join(l.workspace, name, "SKILL.md")
	if data, err := os.ReadFile(wsPath); err == nil {
		return string(data)
	}

	if l.builtinDir != "" {
		bsPath := filepath.Join(l.builtinDir, name, "SKILL.md")
		if data, err := os.ReadFile(bsPath); err == nil {
			return string(data)
		}
	}

	return ""
}

// LoadSkillsForContext loads multiple skills and returns formatted content.
func (l *Loader) LoadSkillsForContext(names []string) string {
	if len(names) == 0 {
		return ""
	}

	l.mu.RLock()
	defer l.mu.RUnlock()

	var parts []string
	for _, name := range names {
		if content, ok := l.cache[name]; ok {
			stripped := stripFrontmatter(content)
			parts = append(parts, fmt.Sprintf("### Skill: %s\n\n%s", name, stripped))
		}
	}

	return strings.Join(parts, "\n\n---\n\n")
}

// GetAlwaysSkills returns skills with always=true that meet requirements.
func (l *Loader) GetAlwaysSkills() []SkillInfo {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var result []SkillInfo
	for _, s := range l.skillIndex {
		if s.Always && s.Available {
			result = append(result, s)
		}
	}
	return result
}

// ListSkills returns all available skills, optionally filtering by availability.
func (l *Loader) ListSkills(filterUnavailable bool) []SkillInfo {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var result []SkillInfo
	for _, s := range l.skillIndex {
		if filterUnavailable && !s.Available {
			continue
		}
		result = append(result, s)
	}
	return result
}

// GetSkillInfo returns skill metadata by name.
func (l *Loader) GetSkillInfo(name string) (SkillInfo, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	info, ok := l.skillIndex[name]
	return info, ok
}

// CheckAvailability checks if a skill's dependencies are met.
func (l *Loader) CheckAvailability(skill SkillInfo) bool {
	return len(skill.MissingDeps) == 0
}

// BuildSystemPrompt builds the system prompt section for named skills.
func (l *Loader) BuildSystemPrompt(skillNames []string) string {
	if len(skillNames) == 0 {
		return ""
	}

	l.mu.RLock()
	defer l.mu.RUnlock()

	var sb strings.Builder
	sb.WriteString("\n# Active Skills\n\n")

	for _, name := range skillNames {
		if info, ok := l.skillIndex[name]; ok && info.Available {
			if content, ok := l.cache[name]; ok {
				sb.WriteString(fmt.Sprintf("### Skill: %s\n\n", info.Name))
				sb.WriteString(content)
				sb.WriteString("\n\n")
			}
		}
	}

	return sb.String()
}

// BuildSkillsSummary builds an XML-formatted summary of all skills.
func (l *Loader) BuildSkillsSummary() string {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if len(l.skillIndex) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("<skills>\n")
	for _, s := range l.skillIndex {
		avail := "true"
		if !s.Available {
			avail = "false"
		}
		always := "false"
		if s.Always {
			always = "true"
		}
		sb.WriteString(fmt.Sprintf("  <skill available=%q always=%q>\n", avail, always))
		sb.WriteString(fmt.Sprintf("    <name>%s</name>\n", escapeXML(s.Name)))
		sb.WriteString(fmt.Sprintf("    <description>%s</description>\n", escapeXML(s.Description)))
		loc := s.Source + ":" + s.Name
		sb.WriteString(fmt.Sprintf("    <location>%s</location>\n", loc))
		if !s.Available && len(s.MissingDeps) > 0 {
			sb.WriteString(fmt.Sprintf("    <requires>%s</requires>\n", escapeXML(strings.Join(s.MissingDeps, ", "))))
		}
		sb.WriteString("  </skill>\n")
	}
	sb.WriteString("</skills>")
	return sb.String()
}

// --- Internal helpers ---

func (l *Loader) scanDirLocked(dir string, source string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		skillName := entry.Name()
		skillPath := filepath.Join(dir, skillName)
		skillFile := filepath.Join(skillPath, "SKILL.md")

		if _, err := os.Stat(skillFile); err != nil {
			continue
		}

		if _, exists := l.skillIndex[skillName]; exists {
			continue
		}

		info := l.parseSkillFileLocked(skillName, skillFile, source)
		if info != nil {
			l.skillIndex[skillName] = *info
		}
	}
}

func (l *Loader) parseSkillFileLocked(name string, path string, source string) *SkillInfo {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	content := string(data)
	l.cache[name] = content

	meta := parseFrontmatter(content)

	available := true
	var missingDeps []string
	if !meta.Available {
		available = false
	}

	for _, req := range meta.Requires {
		if !l.checkDependencyLocked(req) {
			available = false
			missingDeps = append(missingDeps, "Missing: "+req)
		}
	}

	for _, bin := range meta.ExtBins {
		if !existsInPath(bin) {
			available = false
			missingDeps = append(missingDeps, "CLI: "+bin)
		}
	}
	for _, env := range meta.ExtEnv {
		if os.Getenv(env) == "" {
			available = false
			missingDeps = append(missingDeps, "ENV: "+env)
		}
	}

	return &SkillInfo{
		Name:        name,
		Path:        path,
		Source:      source,
		Available:   available,
		Always:      meta.Always,
		Description: meta.Description,
		Commands:    meta.Commands,
		Tags:        meta.Tags,
		Version:     meta.Version,
		MissingDeps: missingDeps,
		WhenToUse:   meta.WhenToUse,
	}
}

// parseFrontmatter parses simple YAML frontmatter from SKILL.md.
// Pure string-based - no yaml library needed.
func parseFrontmatter(content string) SkillMeta {
	if !strings.HasPrefix(content, "---") {
		return SkillMeta{}
	}

	rest := content[3:]
	endIdx := strings.Index(rest, "\n---")
	if endIdx == -1 {
		return SkillMeta{}
	}

	frontmatter := rest[:endIdx]
	var meta SkillMeta

	// Track which multi-key block we're in
	var currentKey string
	var extRequires map[string][]string

	lines := strings.Split(frontmatter, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		indent := len(line) - len(strings.TrimLeft(line, " \t"))

		// Multi-line list item (e.g. "  - GIT")
		if indent > 0 && strings.HasPrefix(trimmed, "- ") {
			val := strings.TrimSpace(trimmed[2:])
			switch currentKey {
			case "requires":
				meta.Requires = append(meta.Requires, val)
			case "commands":
				meta.Commands = append(meta.Commands, val)
			case "tags":
				meta.Tags = append(meta.Tags, val)
			case "bins":
				meta.ExtBins = append(meta.ExtBins, val)
			case "env":
				meta.ExtEnv = append(meta.ExtEnv, val)
			}
			continue
		}

		// Key: value pair
		colonIdx := strings.Index(trimmed, ":")
		if colonIdx == -1 {
			continue
		}

		key := strings.TrimSpace(trimmed[:colonIdx])
		val := strings.TrimSpace(trimmed[colonIdx+1:])

		// Check if it's a multi-line block start
		if val == "" {
			currentKey = key
			// Initialize sub-map for extended_requires
			if key == "extended_requires" {
				extRequires = make(map[string][]string)
			}
			continue
		}

		currentKey = "" // Reset multi-line context

		// Remove inline comments
		if idx := strings.Index(val, " #"); idx != -1 {
			val = strings.TrimSpace(val[:idx])
		}

		switch key {
		case "name":
			meta.Name = unquote(val)
		case "description":
			meta.Description = unquote(val)
		case "always":
			meta.Always = parseBool(val)
		case "available":
			meta.Available = parseBool(val)
		case "version":
			meta.Version = unquote(val)
		case "commands":
			meta.Commands = parseInlineList(val)
		case "tags":
			meta.Tags = parseInlineList(val)
		case "requires":
			// Could be inline: requires: [a, b] or multi-line
			if strings.HasPrefix(val, "[") {
				meta.Requires = parseInlineList(val)
			} else {
				currentKey = "requires"
			}
		case "when_to_use":
			meta.WhenToUse = unquote(val)
		}

		// Handle extended_requires sub-keys
		if currentKey == "" && extRequires != nil {
			// already reset above
		}
		_ = extRequires
	}

	return meta
}

// parseInlineList parses "[a, b, c]" or '["a", "b"]' style inline YAML list.
func parseInlineList(s string) []string {
	s = strings.TrimSpace(s)
	if len(s) < 2 || s[0] != '[' {
		return nil
	}
	s = strings.TrimSuffix(s, "]")
	s = strings.TrimSpace(s[1:])
	if s == "" {
		return nil
	}
	parts := splitList(s)
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		result = append(result, unquote(strings.TrimSpace(p)))
	}
	return result
}

// splitList splits "a, b, c" by commas, respecting quotes.
func splitList(s string) []string {
	var result []string
	var sb strings.Builder
	inQuote := byte(0)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inQuote == 0 && (c == '"' || c == '\'') {
			inQuote = c
			continue
		}
		if inQuote != 0 && c == inQuote {
			inQuote = 0
			continue
		}
		if inQuote == 0 && c == ',' {
			result = append(result, sb.String())
			sb.Reset()
			continue
		}
		sb.WriteByte(c)
	}
	if sb.Len() > 0 {
		result = append(result, sb.String())
	}
	return result
}

func parseBool(s string) bool {
	return s == "true" || s == "yes" || s == "True" || s == "Yes"
}

func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

func stripFrontmatter(content string) string {
	if !strings.HasPrefix(content, "---") {
		return strings.TrimSpace(content)
	}

	rest := content[3:]
	endIdx := strings.Index(rest, "\n---")
	if endIdx == -1 {
		return strings.TrimSpace(content)
	}

	return strings.TrimSpace(rest[endIdx+4:])
}

// ParseFrontmatter parses YAML frontmatter and returns the SkillMeta.
func ParseFrontmatter(content string) (*SkillMeta, error) {
	if !strings.HasPrefix(content, "---") {
		return nil, fmt.Errorf("content does not start with frontmatter delimiter")
	}

	rest := content[3:]
	endIdx := strings.Index(rest, "\n---")
	if endIdx == -1 {
		return nil, fmt.Errorf("frontmatter end delimiter not found")
	}

	// Use the same parser as internal
	meta := parseFrontmatter(content)
	if meta.Name == "" && meta.Description == "" {
		return nil, fmt.Errorf("frontmatter could not be parsed")
	}
	return &meta, nil
}

// CheckDependencies checks if all requirements are met.
func (l *Loader) CheckDependencies(requires []string) bool {
	for _, req := range requires {
		if !l.checkDependency(req) {
			return false
		}
	}
	return true
}

func (l *Loader) checkDependency(req string) bool {
	if isEnvVarName(req) && os.Getenv(req) != "" {
		return true
	}
	if existsInPath(req) {
		return true
	}

	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.checkDependencyLocked(req)
}

// checkDependencyLocked checks a single requirement. Caller must hold l.mu (read or write).
func (l *Loader) checkDependencyLocked(req string) bool {
	if isEnvVarName(req) && os.Getenv(req) != "" {
		return true
	}
	if existsInPath(req) {
		return true
	}

	wsPath := filepath.Join(l.workspace, req)
	if _, err := os.Stat(wsPath); err == nil {
		return true
	}
	if l.builtinDir != "" {
		bsPath := filepath.Join(l.builtinDir, req)
		if _, err := os.Stat(bsPath); err == nil {
			return true
		}
	}
	return false
}

func isEnvVarName(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, c := range s {
		if !((c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
			return false
		}
	}
	return s[0] >= 'A' && s[0] <= 'Z'
}

func existsInPath(bin string) bool {
	// Use exec.LookPath which checks the PATH environment variable
	if _, err := exec.LookPath(bin); err == nil {
		return true
	}
	// On Windows, try common extensions if not already present
	if runtime.GOOS == "windows" {
		base := bin
		hasExt := false
		for _, ext := range []string{".exe", ".cmd", ".bat", ".com"} {
			if strings.HasSuffix(strings.ToLower(base), ext) {
				hasExt = true
				break
			}
		}
		if !hasExt {
			for _, ext := range []string{".exe", ".cmd", ".bat"} {
				if _, err := exec.LookPath(bin + ext); err == nil {
					return true
				}
			}
		}
	}
	return false
}

func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
}

// Ensure strconv is used (for potential future use, but currently not needed)
var _ = strconv.Itoa
