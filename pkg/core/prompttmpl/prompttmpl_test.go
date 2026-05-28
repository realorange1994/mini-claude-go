package prompttmpl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	if r == nil {
		t.Fatal("registry should not be nil")
	}
}

func TestRegisterAndGet(t *testing.T) {
	r := NewRegistry()
	r.Register(PromptTemplate{
		Name:        "greet",
		Description: "Greet someone",
		Content:     "Hello, $1!",
	})

	temp, ok := r.Get("greet")
	if !ok {
		t.Fatal("should find greet")
	}
	if temp.Name != "greet" {
		t.Errorf("Name = %q", temp.Name)
	}
}

func TestExpand(t *testing.T) {
	r := NewRegistry()
	r.Register(PromptTemplate{
		Name:    "review",
		Content: "Review the PR at $1 and focus on $2",
	})

	got := r.Expand("/review https://example.com security")
	if got != "Review the PR at https://example.com and focus on security" {
		t.Errorf("Expanded = %q", got)
	}
}

func TestExpand_NoMatch(t *testing.T) {
	r := NewRegistry()
	got := r.Expand("/nonexistent")
	if got != "/nonexistent" {
		t.Errorf("should return original, got %q", got)
	}
}

func TestExpand_NotSlash(t *testing.T) {
	r := NewRegistry()
	got := r.Expand("Hello world")
	if got != "Hello world" {
		t.Errorf("non-slash text should be unchanged, got %q", got)
	}
}

func TestParseCommandArgs(t *testing.T) {
	tests := []struct {
		input  string
		expect []string
	}{
		{"hello world", []string{"hello", "world"}},
		{`"hello world" foo`, []string{"hello world", "foo"}},
		{`'one two' three`, []string{"one two", "three"}},
		{"", nil},
		{"  ", nil},
	}

	for _, tt := range tests {
		got := ParseCommandArgs(tt.input)
		if len(got) != len(tt.expect) {
			t.Errorf("ParseCommandArgs(%q) = %v, want %v", tt.input, got, tt.expect)
			continue
		}
		for i := range got {
			if got[i] != tt.expect[i] {
				t.Errorf("ParseCommandArgs(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.expect[i])
			}
		}
	}
}

func TestSubstituteArgs(t *testing.T) {
	tests := []struct {
		template string
		args     []string
		expect   string
	}{
		{"Hello $1, welcome to $2", []string{"Alice", "Wonderland"}, "Hello Alice, welcome to Wonderland"},
		{"Args: $@", []string{"a", "b", "c"}, "Args: a b c"},
		{"Args: $ARGUMENTS", []string{"x", "y"}, "Args: x y"},
		{"${@:1:2}", []string{"a", "b", "c"}, "a b"},
		{"${@:2}", []string{"a", "b", "c"}, "b c"},
		{"Missing $3 arg", []string{"a"}, "Missing  arg"},
	}

	for _, tt := range tests {
		got := SubstituteArgs(tt.template, tt.args)
		if got != tt.expect {
			t.Errorf("SubstituteArgs(%q, %v) = %q, want %q", tt.template, tt.args, got, tt.expect)
		}
	}
}

func TestLoadFromDir(t *testing.T) {
	tmp := t.TempDir()
	tmpl := `---
description: Greet a user
argument-hint: <name>
---
Hello, $1! Welcome to our platform.`

	if err := os.WriteFile(filepath.Join(tmp, "greet.md"), []byte(tmpl), 0666); err != nil {
		t.Fatal(err)
	}

	r := NewRegistry()
	r.LoadFromDir(tmp)

	temp, ok := r.Get("greet")
	if !ok {
		t.Fatal("greet template should be loaded")
	}
	if temp.ArgumentHint != "<name>" {
		t.Errorf("ArgumentHint = %q, want <name>", temp.ArgumentHint)
	}
	if !strings.Contains(temp.Content, "Hello, $1!") {
		t.Errorf("Content = %q", temp.Content)
	}
}

func TestLoadFromDir_NonExistent(t *testing.T) {
	r := NewRegistry()
	r.LoadFromDir("/nonexistent")
	if len(r.All()) != 0 {
		t.Error("should not load from non-existent dir")
	}
}

func TestAllSorted(t *testing.T) {
	r := NewRegistry()
	r.Register(PromptTemplate{Name: "z-template", Content: "z"})
	r.Register(PromptTemplate{Name: "a-template", Content: "a"})

	all := r.All()
	if all[0].Name != "a-template" {
		t.Errorf("first should be a-template, got %q", all[0].Name)
	}
}

func TestUnregister(t *testing.T) {
	r := NewRegistry()
	r.Register(PromptTemplate{Name: "temp", Content: "temp"})
	r.Unregister("temp")
	_, ok := r.Get("temp")
	if ok {
		t.Error("should not find unregistered template")
	}
}

func TestParseMarkdown(t *testing.T) {
	// With frontmatter
	got := parseMarkdown("---\ndescription: test\n---\nBody text")
	if got.Description != "test" {
		t.Errorf("Description = %q", got.Description)
	}
	if got.Content != "Body text" {
		t.Errorf("Content = %q", got.Content)
	}

	// Without frontmatter
	got2 := parseMarkdown("First line is description\nMore text here")
	if got2.Description == "" {
		t.Error("should auto-detect description from first line")
	}
}
