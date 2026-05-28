// Package prompttmpl manages prompt templates aligned to pi's prompt-templates.ts.
// Templates are markdown files with optional frontmatter that define reusable prompts.
package prompttmpl

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// PromptTemplate is a reusable prompt loaded from a markdown file.
type PromptTemplate struct {
	Name         string
	Description  string
	ArgumentHint string // optional, from frontmatter
	Content      string
	FilePath     string // absolute path to the template file
}

// Registry manages prompt templates.
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
	for i := 0; i < len(out)-1; i++ {
		for j := 0; j < len(out)-i-1; j++ {
			if out[j].Name > out[j+1].Name {
				out[j], out[j+1] = out[j+1], out[j]
			}
		}
	}
	return out
}

// LoadFromDir loads .md templates from a directory.
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
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		t := parseMarkdown(string(data))
		t.Name = strings.TrimSuffix(entry.Name(), ".md")
		t.FilePath = path
		r.Register(t)
	}
	return nil
}

// parseMarkdown extracts frontmatter fields and body from markdown content.
func parseMarkdown(content string) PromptTemplate {
	t := PromptTemplate{}

	// Extract frontmatter
	trimmed := strings.TrimSpace(content)
	if strings.HasPrefix(trimmed, "---") {
		parts := strings.SplitN(trimmed[3:], "---", 2)
		if len(parts) == 2 {
			fm := parseFrontmatter(strings.TrimSpace(parts[0]))
			t.Description = fm["description"]
			t.ArgumentHint = fm["argument-hint"]
			t.Content = strings.TrimSpace(parts[1])
			return t
		}
	}

	// No frontmatter: use first non-empty line as description
	lines := strings.SplitN(trimmed, "\n", 2)
	if len(lines) > 0 && strings.TrimSpace(lines[0]) != "" {
		desc := lines[0]
		if len(desc) > 60 {
			desc = desc[:60] + "..."
		}
		t.Description = desc
	}
	t.Content = trimmed
	return t
}

func parseFrontmatter(text string) map[string]string {
	out := make(map[string]string)
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		kv := strings.SplitN(line, ":", 2)
		if len(kv) == 2 {
			key := strings.TrimSpace(kv[0])
			val := strings.TrimSpace(kv[1])
			// Strip quotes
			if len(val) >= 2 && val[0] == '"' && val[len(val)-1] == '"' {
				val = val[1 : len(val)-1]
			}
			out[key] = val
		}
	}
	return out
}

// Expand finds a template by name in the text (format: /name args...)
// and substitutes arguments, returning the expanded content.
func (r *Registry) Expand(text string) string {
	if !strings.HasPrefix(text, "/") {
		return text
	}

	// Parse /name args...
	spaceIdx := strings.Index(text, " ")
	var name, argsStr string
	if spaceIdx == -1 {
		name = strings.TrimPrefix(text, "/")
	} else {
		name = strings.TrimPrefix(text[1:spaceIdx], "/")
		argsStr = strings.TrimSpace(text[spaceIdx+1:])
	}

	t, ok := r.Get(name)
	if !ok {
		return text
	}

	args := ParseCommandArgs(argsStr)
	return SubstituteArgs(t.Content, args)
}

// ParseCommandArgs parses command arguments respecting quoted strings (bash-style).
func ParseCommandArgs(s string) []string {
	var args []string
	var current strings.Builder
	var inQuote byte

	for i := 0; i < len(s); i++ {
		c := s[i]
		if inQuote != 0 {
			if c == inQuote {
				inQuote = 0
			} else {
				current.WriteByte(c)
			}
		} else if c == '"' || c == '\'' {
			inQuote = c
		} else if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			if current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
			}
		} else {
			current.WriteByte(c)
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

// SubstituteArgs replaces argument placeholders in template content:
// - $1, $2, ... for positional args (1-indexed)
// - ${@:N} for args from Nth onwards
// - ${@:N:L} for L args starting from Nth
// - $ARGUMENTS for all args joined
// - $@ for all args joined
func SubstituteArgs(content string, args []string) string {
	result := content

	// Replace positional args FIRST ($1, $2, ...)
	result = positionalRe.ReplaceAllStringFunc(result, func(match string) string {
		numStr := match[1:]
		num, _ := strconv.Atoi(numStr)
		idx := num - 1
		if idx >= 0 && idx < len(args) {
			return args[idx]
		}
		return ""
	})

	// Replace ${@:N} and ${@:N:L}
	result = sliceRe.ReplaceAllStringFunc(result, func(match string) string {
		sub := sliceRe.FindStringSubmatch(match)
		start, _ := strconv.Atoi(sub[1])
		// Convert 1-indexed (user-facing) to 0-indexed (Go slice)
		start = start - 1
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

	// Replace $ARGUMENTS with all args joined
	allArgs := strings.Join(args, " ")
	result = argumentsRe.ReplaceAllLiteralString(result, allArgs)

	// Replace $@ with all args joined
	result = atRe.ReplaceAllLiteralString(result, allArgs)

	return result
}
