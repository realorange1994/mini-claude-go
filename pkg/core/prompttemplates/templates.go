// Package prompttemplates loads and manages prompt template files.
// Aligned to pi's prompt-templates.ts.
package prompttemplates

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// PromptTemplate represents a loaded prompt template.
type PromptTemplate struct {
	Name         string      // filename without .md extension
	Description  string      // from frontmatter or first body line
	ArgumentHint string      // from frontmatter "argument-hint"
	Content      string      // markdown body after frontmatter
	SourceInfo   SourceInfo
	FilePath     string      // absolute path to the .md file
}

// SourceInfo tracks where a template was loaded from.
type SourceInfo struct {
	Source  string // "user", "project", "local"
	BaseDir string
}

// Registry manages prompt templates with thread-safe access.
type Registry struct {
	mu        sync.RWMutex
	templates map[string]PromptTemplate
}

// NewRegistry creates an empty prompt template registry.
func NewRegistry() *Registry {
	return &Registry{
		templates: make(map[string]PromptTemplate),
	}
}

// Register adds or replaces a template.
func (r *Registry) Register(t PromptTemplate) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.templates[t.Name] = t
}

// Unregister removes a template by name.
func (r *Registry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.templates, name)
}

// Get returns a template by name.
func (r *Registry) Get(name string) (PromptTemplate, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.templates[name]
	return t, ok
}

// All returns all templates sorted by name.
func (r *Registry) All() []PromptTemplate {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]PromptTemplate, 0, len(r.templates))
	for _, t := range r.templates {
		out = append(out, t)
	}
	// Simple sort by name
	for i := 0; i < len(out)-1; i++ {
		for j := i + 1; j < len(out); j++ {
			if out[i].Name > out[j].Name {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

// LoadFromDir loads .md templates from a directory into the registry.
func (r *Registry) LoadFromDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		tmpl := LoadTemplateFromFile(path, SourceInfo{Source: "local", BaseDir: dir})
		if tmpl != nil {
			r.Register(*tmpl)
		}
	}
	return nil
}

// Expand finds a template by name in the text (format: /name args...)
// and substitutes arguments, returning the expanded content.
func (r *Registry) Expand(text string) string {
	if !strings.HasPrefix(text, "/") {
		return text
	}

	spaceIdx := strings.Index(text, " ")
	var name, argsStr string
	if spaceIdx == -1 {
		name = strings.TrimPrefix(text, "/")
	} else {
		name = text[1:spaceIdx]
		argsStr = strings.TrimSpace(text[spaceIdx+1:])
	}

	t, ok := r.Get(name)
	if !ok {
		return text
	}

	args := ParseCommandArgs(argsStr)
	return SubstituteArgs(t.Content, args)
}

// LoadPromptTemplatesOptions configures template loading.
type LoadPromptTemplatesOptions struct {
	Cwd           string
	AgentDir      string
	PromptPaths   []string
	IncludeDefaults bool
}

// parseFrontmatter parses YAML-like frontmatter from markdown.
func parseFrontmatter(text string) (map[string]string, string) {
	metadata := make(map[string]string)
	body := text

	lines := strings.SplitN(text, "\n", 2)
	if len(lines) >= 2 && strings.TrimSpace(lines[0]) == "---" {
		rest := lines[1]
		closeIdx := strings.Index(rest, "---")
		if closeIdx != -1 {
			fmContent := strings.TrimSpace(rest[:closeIdx])
			body = strings.TrimSpace(rest[closeIdx+3:])

			for _, line := range strings.Split(fmContent, "\n") {
				line = strings.TrimSpace(line)
				if idx := strings.Index(line, ":"); idx > 0 {
					key := strings.TrimSpace(line[:idx])
					val := strings.TrimSpace(line[idx+1:])
					// Strip quotes
					if len(val) >= 2 && val[0] == '"' && val[len(val)-1] == '"' {
						val = val[1 : len(val)-1]
					}
					metadata[key] = val
				}
			}
		}
	}

	return metadata, body
}

// LoadTemplateFromFile reads a .md file and constructs a PromptTemplate.
func LoadTemplateFromFile(filePath string, sourceInfo SourceInfo) *PromptTemplate {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil
	}

	content := string(data)
	metadata, body := parseFrontmatter(content)

	name := strings.TrimSuffix(filepath.Base(filePath), ".md")
	description := metadata["description"]
	if description == "" {
		for _, line := range strings.Split(body, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				if len(line) > 60 {
					description = line[:60] + "..."
				} else {
					description = line
				}
				break
			}
		}
	}

	return &PromptTemplate{
		Name:         name,
		Description:  description,
		ArgumentHint: metadata["argument-hint"],
		Content:      body,
		SourceInfo:   sourceInfo,
		FilePath:     filePath,
	}
}

// LoadTemplatesFromDir scans a directory for .md files and loads them as templates.
func LoadTemplatesFromDir(dir string, getSourceInfo func(filePath string) SourceInfo) []PromptTemplate {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var templates []PromptTemplate
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}

		filePath := filepath.Join(dir, e.Name())
		if link, err := os.Readlink(filePath); err == nil {
			if !filepath.IsAbs(link) {
				link = filepath.Join(dir, link)
			}
			filePath = link
		}

		tmpl := LoadTemplateFromFile(filePath, getSourceInfo(filePath))
		if tmpl != nil {
			templates = append(templates, *tmpl)
		}
	}

	return templates
}

// LoadPromptTemplates loads prompt templates from multiple sources.
func LoadPromptTemplates(opts LoadPromptTemplatesOptions) []PromptTemplate {
	var templates []PromptTemplate

	cwd := filepath.Clean(opts.Cwd)
	agentDir := filepath.Clean(opts.AgentDir)

	globalDir := filepath.Join(agentDir, "prompts")
	projectDir := filepath.Join(cwd, ".miniclaude", "prompts")

	getGlobalSourceInfo := func(filePath string) SourceInfo {
		return SourceInfo{Source: "user", BaseDir: globalDir}
	}
	getProjectSourceInfo := func(filePath string) SourceInfo {
		return SourceInfo{Source: "project", BaseDir: projectDir}
	}

	if opts.IncludeDefaults {
		templates = append(templates, LoadTemplatesFromDir(globalDir, getGlobalSourceInfo)...)
	}

	if opts.IncludeDefaults {
		templates = append(templates, LoadTemplatesFromDir(projectDir, getProjectSourceInfo)...)
	}

	for _, path := range opts.PromptPaths {
		resolved := path
		if !filepath.IsAbs(path) {
			resolved = filepath.Join(cwd, path)
		}

		info, err := os.Stat(resolved)
		if err != nil {
			continue
		}

		if info.IsDir() {
			templates = append(templates, LoadTemplatesFromDir(resolved, func(filePath string) SourceInfo {
				return SourceInfo{Source: "local", BaseDir: resolved}
			})...)
		} else if strings.HasSuffix(resolved, ".md") {
			tmpl := LoadTemplateFromFile(resolved, SourceInfo{Source: "local", BaseDir: filepath.Dir(resolved)})
			if tmpl != nil {
				templates = append(templates, *tmpl)
			}
		}
	}

	return templates
}

// ParseCommandArgs parses a command argument string respecting bash-style quotes.
func ParseCommandArgs(argsString string) []string {
	var args []string
	var current strings.Builder
	inSingleQuote := false
	inDoubleQuote := false

	for _, r := range argsString {
		switch r {
		case '\'':
			if !inDoubleQuote {
				inSingleQuote = !inSingleQuote
			}
		case '"':
			if !inSingleQuote {
				inDoubleQuote = !inDoubleQuote
			}
		case ' ', '\t':
			if !inSingleQuote && !inDoubleQuote {
				if current.Len() > 0 {
					args = append(args, current.String())
					current.Reset()
				}
			} else {
				current.WriteRune(r)
			}
		default:
			current.WriteRune(r)
		}
	}
	if current.Len() > 0 {
		args = append(args, current.String())
	}

	return args
}

var (
	positionalRe = regexp.MustCompile(`\$(\d+)`)
	sliceRe      = regexp.MustCompile(`\$\{@:(\d+)(?::(\d+))?\}`)
	argumentsRe  = regexp.MustCompile(`\$ARGUMENTS`)
	atRe         = regexp.MustCompile(`\$@`)
)

// SubstituteArgs replaces argument placeholders in template content.
// Supports: $1, $2, ... (positional), ${@:N} and ${@:N:L} (bash slices),
// $ARGUMENTS, $@ (all args).
func SubstituteArgs(content string, args []string) string {
	result := content

	// 1. Bash-style slices: ${@:N:L} and ${@:N}
	result = sliceRe.ReplaceAllStringFunc(result, func(match string) string {
		sub := sliceRe.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		start, _ := strconv.Atoi(sub[1])
		start = start - 1 // 1-indexed to 0-indexed
		if start < 0 {
			start = 0
		}
		if sub[2] != "" {
			length, _ := strconv.Atoi(sub[2])
			end := start + length
			if end > len(args) {
				end = len(args)
			}
			if start >= len(args) {
				return ""
			}
			return strings.Join(args[start:end], " ")
		}
		if start >= len(args) {
			return ""
		}
		return strings.Join(args[start:], " ")
	})

	// 2. Positional arguments: $1, $2, ...
	result = positionalRe.ReplaceAllStringFunc(result, func(match string) string {
		numStr := match[1:]
		num, _ := strconv.Atoi(numStr)
		idx := num - 1
		if idx >= 0 && idx < len(args) {
			return args[idx]
		}
		return ""
	})

	// 3. $ARGUMENTS (all args)
	allArgs := strings.Join(args, " ")
	result = argumentsRe.ReplaceAllLiteralString(result, allArgs)

	// 4. $@ (all args)
	result = atRe.ReplaceAllLiteralString(result, allArgs)

	return result
}

// ExpandPromptTemplate expands a prompt template if the input matches /templateName [args...].
func ExpandPromptTemplate(text string, templates []PromptTemplate) string {
	if !strings.HasPrefix(text, "/") {
		return text
	}

	parts := strings.SplitN(strings.TrimPrefix(text, "/"), " ", 2)
	name := parts[0]
	argString := ""
	if len(parts) > 1 {
		argString = parts[1]
	}

	for _, tmpl := range templates {
		if tmpl.Name == name {
			args := ParseCommandArgs(argString)
			return SubstituteArgs(tmpl.Content, args)
		}
	}

	return text
}

// FindTemplateByName finds a template by name.
func FindTemplateByName(name string, templates []PromptTemplate) *PromptTemplate {
	for i := range templates {
		if templates[i].Name == name {
			return &templates[i]
		}
	}
	return nil
}