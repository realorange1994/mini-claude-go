// Package prompttemplates loads and manages prompt template files.
// Aligned to pi's prompt-templates.ts.
package prompttemplates

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
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
	Source string // "user", "project", "local"
	BaseDir string
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
		// Find closing ---
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
		// Fallback: first non-empty body line, truncated to 60 chars
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
		// Handle symlinks
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

	// 1. Global templates
	if opts.IncludeDefaults {
		templates = append(templates, LoadTemplatesFromDir(globalDir, getGlobalSourceInfo)...)
	}

	// 2. Project templates
	if opts.IncludeDefaults {
		templates = append(templates, LoadTemplatesFromDir(projectDir, getProjectSourceInfo)...)
	}

	// 3. Explicit paths
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

// SubstituteArgs replaces argument placeholders in template content.
// Supports: $1, $2, ... (positional), ${@:N} and ${@:N:L} (bash slices),
// $ARGUMENTS, $@ (all args).
func SubstituteArgs(content string, args []string) string {
	// 1. Bash-style slices: ${@:N:L} and ${@:N}
	sliceRegex := regexp.MustCompile(`\$\{@:(\d+)(?::(\d+))?\}`)
	content = sliceRegex.ReplaceAllStringFunc(content, func(match string) string {
		submatch := sliceRegex.FindStringSubmatch(match)
		if len(submatch) < 2 {
			return match
		}
		n, _ := strconv.Atoi(submatch[1])
		l := -1
		if len(submatch) > 2 && submatch[2] != "" {
			l, _ = strconv.Atoi(submatch[2])
		}

		// Convert 1-indexed to 0-indexed
		start := n - 1
		if start < 0 || start >= len(args) {
			return ""
		}

		if l > 0 {
			end := start + l
			if end > len(args) {
				end = len(args)
			}
			return strings.Join(args[start:end], " ")
		}
		return strings.Join(args[start:], " ")
	})

	// 2. Positional arguments: $1, $2, ... $9 (single digit only)
	posRegex := regexp.MustCompile(`\$(\d)`)
	content = posRegex.ReplaceAllStringFunc(content, func(match string) string {
		n, _ := strconv.Atoi(match[1:])
		if n > 0 && n <= len(args) {
			return args[n-1]
		}
		return match
	})

	// 3. $ARGUMENTS (all args)
	content = strings.ReplaceAll(content, "$ARGUMENTS", strings.Join(args, " "))

	// 4. $@ (all args)
	content = strings.ReplaceAll(content, "$@", strings.Join(args, " "))

	return content
}

// ExpandPromptTemplate expands a prompt template if the input matches /templateName [args...].
// Returns the expanded content if matched, or the original text if not.
func ExpandPromptTemplate(text string, templates []PromptTemplate) string {
	if !strings.HasPrefix(text, "/") {
		return text
	}

	// Extract template name and arguments
	parts := strings.SplitN(strings.TrimPrefix(text, "/"), " ", 2)
	name := parts[0]
	argString := ""
	if len(parts) > 1 {
		argString = parts[1]
	}

	// Find matching template
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
