// Package skills provides skill discovery and loading aligned to pi's skills.ts.
// Skills are discovered via SKILL.md files with YAML frontmatter per the Agent Skills spec.
// See: https://agentskills.io/integrate-skills
package skills

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"miniclaudecode-go/pkg/core/diagnostics"
	"miniclaudecode-go/pkg/core/sourceinfo"
)

// Max name/description lengths per Agent Skills spec.
const (
	MaxNameLength        = 64
	MaxDescriptionLength = 1024
)

// Ignore file names for skill discovery.
var ignoreFileNames = []string{".gitignore", ".ignore", ".fdignore"}

// SkillFrontmatter represents parsed YAML frontmatter from SKILL.md.
type SkillFrontmatter struct {
	Name                 string `yaml:"name"`
	Description          string `yaml:"description"`
	DisableModelInvocation bool   `yaml:"disable-model-invocation"`
}

// Skill represents a discovered skill.
type Skill struct {
	Name                 string                       `json:"name"`
	Description          string                       `json:"description"`
	FilePath             string                       `json:"filePath"`
	BaseDir              string                       `json:"baseDir"`
	SourceInfo           *sourceinfo.ResourceSourceInfo `json:"sourceInfo"`
	DisableModelInvocation bool                        `json:"disableModelInvocation"`
}

// LoadSkillsResult contains loaded skills and any diagnostics.
type LoadSkillsResult struct {
	Skills      []Skill                        `json:"skills"`
	Diagnostics []diagnostics.ResourceDiagnostic `json:"diagnostics"`
}

// LoadSkillsOptions configures skill loading.
type LoadSkillsOptions struct {
	// Cwd is the working directory for project-local skills.
	Cwd string
	// AgentDir is the global agent config directory.
	AgentDir string
	// SkillPaths are explicit skill paths (files or directories).
	SkillPaths []string
	// IncludeDefaults includes default skill directories.
	IncludeDefaults bool
}

// ValidateName validates a skill name per Agent Skills spec.
// Returns validation error messages (empty if valid).
func ValidateName(name string) []string {
	var errors []string

	if len(name) > MaxNameLength {
		errors = append(errors, fmt.Sprintf("name exceeds %d characters (%d)", MaxNameLength, len(name)))
	}

	// Must be lowercase a-z, 0-9, hyphens only
	matched, _ := regexp.MatchString("^[a-z0-9-]+$", name)
	if !matched {
		errors = append(errors, "name contains invalid characters (must be lowercase a-z, 0-9, hyphens only)")
	}

	if strings.HasPrefix(name, "-") || strings.HasSuffix(name, "-") {
		errors = append(errors, "name must not start or end with a hyphen")
	}

	if strings.Contains(name, "--") {
		errors = append(errors, "name must not contain consecutive hyphens")
	}

	return errors
}

// ValidateDescription validates a skill description per Agent Skills spec.
func ValidateDescription(description string) []string {
	var errors []string

	if description == "" || strings.TrimSpace(description) == "" {
		errors = append(errors, "description is required")
	} else if len(description) > MaxDescriptionLength {
		errors = append(errors, fmt.Sprintf("description exceeds %d characters (%d)", MaxDescriptionLength, len(description)))
	}

	return errors
}

// LoadSkills loads skills from all configured locations.
// Returns skills and any validation diagnostics.
func LoadSkills(opts LoadSkillsOptions) LoadSkillsResult {
	cwd := opts.Cwd
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	agentDir := opts.AgentDir
	if agentDir == "" {
		home, _ := os.UserHomeDir()
		agentDir = filepath.Join(home, ".miniclaude")
	}

	skillMap := make(map[string]Skill)
	realPathSet := make(map[string]struct{})
	var allDiagnostics []diagnostics.ResourceDiagnostic
	var collisionDiagnostics []diagnostics.ResourceDiagnostic

	addSkills := func(result LoadSkillsResult) {
		allDiagnostics = append(allDiagnostics, result.Diagnostics...)
		for _, skill := range result.Skills {
			// Resolve symlinks to detect duplicate files
			realPath := canonicalizePath(skill.FilePath)

			// Skip silently if we've already loaded this exact file (via symlink)
			if _, exists := realPathSet[realPath]; exists {
				continue
			}

			if existing, exists := skillMap[skill.Name]; exists {
				collisionDiagnostics = append(collisionDiagnostics, diagnostics.ResourceDiagnostic{
					Type:    diagnostics.DiagCollision,
					Message: fmt.Sprintf("name %q collision", skill.Name),
					Path:    skill.FilePath,
					Collision: &diagnostics.ResourceCollision{
						ResourceType: diagnostics.ResourceSkill,
						Name:         skill.Name,
						WinnerPath:   existing.FilePath,
						LoserPath:    skill.FilePath,
					},
				})
			} else {
				skillMap[skill.Name] = skill
				realPathSet[realPath] = struct{}{}
			}
		}
	}

	if opts.IncludeDefaults {
		// User skills: ~/.miniclaude/skills/
		addSkills(loadSkillsFromDir(filepath.Join(agentDir, "skills"), "user"))
		// Project skills: <cwd>/.miniclaude/skills/
		addSkills(loadSkillsFromDir(filepath.Join(cwd, ".miniclaude", "skills"), "project"))
	}

	// Load from explicit paths
	for _, rawPath := range opts.SkillPaths {
		resolvedPath := rawPath
		if !filepath.IsAbs(rawPath) {
			resolvedPath = filepath.Join(cwd, rawPath)
		}

		if _, err := os.Stat(resolvedPath); os.IsNotExist(err) {
			allDiagnostics = append(allDiagnostics, diagnostics.ResourceDiagnostic{
				Type:    diagnostics.DiagWarning,
				Message: "skill path does not exist",
				Path:    resolvedPath,
			})
			continue
		}

		info, err := os.Stat(resolvedPath)
		if err != nil {
			allDiagnostics = append(allDiagnostics, diagnostics.ResourceDiagnostic{
				Type:    diagnostics.DiagWarning,
				Message: err.Error(),
				Path:    resolvedPath,
			})
			continue
		}

		source := "path"
		if info.IsDir() {
			addSkills(loadSkillsFromDir(resolvedPath, source))
		} else if strings.HasSuffix(strings.ToLower(resolvedPath), ".md") {
			result := loadSkillFromFile(resolvedPath, source)
			if result.Skill != nil {
				addSkills(LoadSkillsResult{Skills: []Skill{*result.Skill}, Diagnostics: result.Diagnostics})
			} else {
				allDiagnostics = append(allDiagnostics, result.Diagnostics...)
			}
		} else {
			allDiagnostics = append(allDiagnostics, diagnostics.ResourceDiagnostic{
				Type:    diagnostics.DiagWarning,
				Message: "skill path is not a markdown file",
				Path:    resolvedPath,
			})
		}
	}

	// Combine diagnostics
	allDiagnostics = append(allDiagnostics, collisionDiagnostics...)

	// Convert map to sorted slice
	skills := make([]Skill, 0, len(skillMap))
	for _, s := range skillMap {
		skills = append(skills, s)
	}
	sort.Slice(skills, func(i, j int) bool {
		return skills[i].Name < skills[j].Name
	})

	return LoadSkillsResult{Skills: skills, Diagnostics: allDiagnostics}
}

// loadSkillsFromDir loads skills from a directory.
// Discovery rules (per TS):
// - if a directory contains SKILL.md, treat it as a skill root and do not recurse further
// - otherwise, load direct .md children in the root
// - recurse into subdirectories to find SKILL.md
func loadSkillsFromDir(dir string, source string) LoadSkillsResult {
	return loadSkillsFromDirInternal(dir, source, true, nil, "")
}

func loadSkillsFromDirInternal(dir string, source string, includeRootFiles bool, ignoreMatcher *ignoreMatcher, rootDir string) LoadSkillsResult {
	var skills []Skill
	var diags []diagnostics.ResourceDiagnostic

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return LoadSkillsResult{Skills: skills, Diagnostics: diags}
	}

	root := rootDir
	if root == "" {
		root = dir
	}

	ig := ignoreMatcher
	if ig == nil {
		ig = newIgnoreMatcher()
	}
	addIgnoreRules(ig, dir, root)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return LoadSkillsResult{Skills: skills, Diagnostics: diags}
	}

	// First pass: check for SKILL.md in this directory
	for _, entry := range entries {
		if entry.Name() != "SKILL.md" {
			continue
		}

		fullPath := filepath.Join(dir, entry.Name())

		isFile := entry.Type().IsRegular()
		if entry.Type()&os.ModeSymlink != 0 {
			if fi, err := os.Stat(fullPath); err == nil {
				isFile = fi.Mode().IsRegular()
			} else {
				continue // broken symlink
			}
		}

		relPath := toPosixPath(relPath(root, fullPath))
		if !isFile || ig.ignores(relPath) {
			continue
		}

		result := loadSkillFromFile(fullPath, source)
		if result.Skill != nil {
			skills = append(skills, *result.Skill)
		}
		diags = append(diags, result.Diagnostics...)
		return LoadSkillsResult{Skills: skills, Diagnostics: diags} // SKILL.md found, don't recurse
	}

	// Second pass: recurse into subdirectories and check root .md files
	for _, entry := range entries {
		name := entry.Name()

		// Skip hidden files and node_modules
		if strings.HasPrefix(name, ".") || name == "node_modules" {
			continue
		}

		fullPath := filepath.Join(dir, name)

		isDir := entry.IsDir()
		isFile := entry.Type().IsRegular()
		if entry.Type()&os.ModeSymlink != 0 {
			if fi, err := os.Stat(fullPath); err == nil {
				isDir = fi.IsDir()
				isFile = fi.Mode().IsRegular()
			} else {
				continue // broken symlink
			}
		}

		relPath := toPosixPath(relPath(root, fullPath))
		ignorePath := relPath
		if isDir {
			ignorePath = relPath + "/"
		}
		if ig.ignores(ignorePath) {
			continue
		}

		if isDir {
			subResult := loadSkillsFromDirInternal(fullPath, source, false, ig, root)
			skills = append(skills, subResult.Skills...)
			diags = append(diags, subResult.Diagnostics...)
			continue
		}

		if !isFile || !includeRootFiles || !strings.HasSuffix(strings.ToLower(name), ".md") {
			continue
		}

		result := loadSkillFromFile(fullPath, source)
		if result.Skill != nil {
			skills = append(skills, *result.Skill)
		}
		diags = append(diags, result.Diagnostics...)
	}

	return LoadSkillsResult{Skills: skills, Diagnostics: diags}
}

// loadSkillFromFile loads a single skill from a SKILL.md file.
func loadSkillFromFile(filePath string, source string) struct {
	Skill       *Skill
	Diagnostics []diagnostics.ResourceDiagnostic
} {
	var diags []diagnostics.ResourceDiagnostic

	data, err := os.ReadFile(filePath)
	if err != nil {
		diags = append(diags, diagnostics.ResourceDiagnostic{
			Type:    diagnostics.DiagWarning,
			Message: err.Error(),
			Path:    filePath,
		})
		return struct {
			Skill       *Skill
			Diagnostics []diagnostics.ResourceDiagnostic
		}{nil, diags}
	}

	content := string(data)
	frontmatter, _ := parseFrontmatter(content)
	skillDir := filepath.Dir(filePath)
	parentDirName := filepath.Base(skillDir)

	// Validate description
	for _, err := range ValidateDescription(frontmatter.Description) {
		diags = append(diags, diagnostics.ResourceDiagnostic{
			Type:    diagnostics.DiagWarning,
			Message: err,
			Path:    filePath,
		})
	}

	// Use name from frontmatter, or fall back to parent directory name
	name := frontmatter.Name
	if name == "" {
		name = parentDirName
	}

	// Validate name
	for _, err := range ValidateName(name) {
		diags = append(diags, diagnostics.ResourceDiagnostic{
			Type:    diagnostics.DiagWarning,
			Message: err,
			Path:    filePath,
		})
	}

	// Don't load skill if description is completely missing
	if frontmatter.Description == "" || strings.TrimSpace(frontmatter.Description) == "" {
		return struct {
			Skill       *Skill
			Diagnostics []diagnostics.ResourceDiagnostic
		}{nil, diags}
	}

	return struct {
		Skill       *Skill
		Diagnostics []diagnostics.ResourceDiagnostic
	}{
		Skill: &Skill{
			Name:                 name,
			Description:          frontmatter.Description,
			FilePath:             filePath,
			BaseDir:              skillDir,
			SourceInfo:           createSkillSourceInfo(filePath, skillDir, source),
			DisableModelInvocation: frontmatter.DisableModelInvocation,
		},
		Diagnostics: diags,
	}
}

// FormatSkillsForPrompt formats skills for inclusion in a system prompt.
// Uses XML format per Agent Skills standard.
// Skills with DisableModelInvocation=true are excluded from the prompt
// (they can only be invoked explicitly via /skill:name commands).
func FormatSkillsForPrompt(skills []Skill) string {
	visibleSkills := make([]Skill, 0, len(skills))
	for _, s := range skills {
		if !s.DisableModelInvocation {
			visibleSkills = append(visibleSkills, s)
		}
	}

	if len(visibleSkills) == 0 {
		return ""
	}

	var lines []string
	lines = append(lines, "\n\nThe following skills provide specialized instructions for specific tasks.")
	lines = append(lines, "Use the read tool to load a skill's file when the task matches its description.")
	lines = append(lines, "When a skill file references a relative path, resolve it against the skill directory (parent of SKILL.md / dirname of the path) and use that absolute path in tool commands.")
	lines = append(lines, "")
	lines = append(lines, "<available_skills>")

	for _, skill := range visibleSkills {
		lines = append(lines, "  <skill>")
		lines = append(lines, fmt.Sprintf("    <name>%s</name>", escapeXML(skill.Name)))
		lines = append(lines, fmt.Sprintf("    <description>%s</description>", escapeXML(skill.Description)))
		lines = append(lines, fmt.Sprintf("    <location>%s</location>", escapeXML(skill.FilePath)))
		lines = append(lines, "  </skill>")
	}

	lines = append(lines, "</available_skills>")

	return strings.Join(lines, "\n")
}

// --- Helpers ---

func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
}

func toPosixPath(p string) string {
	return strings.ReplaceAll(p, string(filepath.Separator), "/")
}

func relPath(root, target string) string {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return target
	}
	return rel
}

func canonicalizePath(p string) string {
	// Resolve symlinks
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return resolved
	}
	return p
}

func createSkillSourceInfo(filePath, baseDir, source string) *sourceinfo.ResourceSourceInfo {
	var scope sourceinfo.SourceScope
	switch source {
	case "user":
		scope = sourceinfo.ScopeUser
	case "project":
		scope = sourceinfo.ScopeProject
	default:
		scope = sourceinfo.ScopeTemporary
	}
	si := sourceinfo.CreateSyntheticSourceInfo(filePath, "local", scope, sourceinfo.OriginTopLevel, baseDir)
	return &si
}

// parseFrontmatter parses YAML frontmatter from content.
// Returns frontmatter and body.
func parseFrontmatter(content string) (SkillFrontmatter, string) {
	var fm SkillFrontmatter
	body := content

	// Check for --- delimiter at start
	if !strings.HasPrefix(content, "---") {
		return fm, body
	}

	// Find closing ---
	rest := content[3:]
	endIdx := strings.Index(rest, "\n---")
	if endIdx == -1 {
		return fm, body
	}

	fmContent := strings.TrimSpace(rest[:endIdx])
	body = strings.TrimSpace(rest[endIdx+4:])

	// Parse simple YAML key: value
	lines := strings.Split(fmContent, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		colonIdx := strings.Index(line, ":")
		if colonIdx == -1 {
			continue
		}

		key := strings.TrimSpace(line[:colonIdx])
		val := strings.TrimSpace(line[colonIdx+1:])

		// Remove quotes
		if len(val) >= 2 {
			if (val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'') {
				val = val[1 : len(val)-1]
			}
		}

		switch key {
		case "name":
			fm.Name = val
		case "description":
			fm.Description = val
		case "disable-model-invocation":
			fm.DisableModelInvocation = val == "true"
		}
	}

	return fm, body
}

// --- Ignore matcher (simplified) ---

type ignoreMatcher struct {
	patterns []string
}

func newIgnoreMatcher() *ignoreMatcher {
	return &ignoreMatcher{}
}

func (ig *ignoreMatcher) add(pattern string) {
	ig.patterns = append(ig.patterns, pattern)
}

func (ig *ignoreMatcher) ignores(path string) bool {
	for _, p := range ig.patterns {
		// Simple glob matching
		matched, err := filepath.Match(p, path)
		if err == nil && matched {
			return true
		}
		// Also try as prefix for directory patterns
		if strings.HasSuffix(p, "/") && strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

func addIgnoreRules(ig *ignoreMatcher, dir string, rootDir string) {
	relDir := ""
	if dir != rootDir {
		var err error
		relDir, err = filepath.Rel(rootDir, dir)
		if err != nil {
			relDir = ""
		}
	}
	prefix := toPosixPath(relDir)
	if prefix != "" {
		prefix = prefix + "/"
	}

	for _, filename := range ignoreFileNames {
		ignorePath := filepath.Join(dir, filename)
		data, err := os.ReadFile(ignorePath)
		if err != nil {
			continue
		}

		lines := bytes.Split(data, []byte("\n"))
		for _, line := range lines {
			lineStr := strings.TrimSpace(string(line))
			if lineStr == "" || strings.HasPrefix(lineStr, "#") {
				continue
			}

			// Handle negation
			negated := false
			if strings.HasPrefix(lineStr, "!") {
				negated = true
				lineStr = lineStr[1:]
			} else if strings.HasPrefix(lineStr, "\\!") {
				lineStr = lineStr[1:]
			}

			// Remove leading slash (relative to gitignore root)
			if strings.HasPrefix(lineStr, "/") {
				lineStr = lineStr[1:]
			}

			// Prefix with relative directory
			if prefix != "" {
				lineStr = prefix + lineStr
			}

			if negated {
				lineStr = "!" + lineStr
			}

			ig.add(lineStr)
		}
	}
}
