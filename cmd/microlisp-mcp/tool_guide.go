package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type LispGuideTool struct {
	LLM       *AnthropicClient
	SourceDir string
	index     string
	indexOnce sync.Once
}

func RegisterGuideTool(s *MCPServer, llm *AnthropicClient, sourceDir string) {
	tool := &LispGuideTool{LLM: llm, SourceDir: sourceDir}
	s.RegisterTool(ToolDef{
		Name:        "lisp_guide",
		Description: "Ask the LLM about Lisp/FFI syntax and usage with source code context.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"question":     map[string]any{"type": "string", "description": "Your Lisp/FFI question."},
				"source_files": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Optional files to read."},
				"max_tokens":   map[string]any{"type": "integer", "description": "Max tokens. Default: 4096."},
			},
			"required": []string{"question"},
		},
	}, tool.handle)
}

func (t *LispGuideTool) handle(params json.RawMessage) (ToolCallResult, error) {
	var p struct {
		Question    string   `json:"question"`
		SourceFiles []string `json:"source_files"`
		MaxTokens   int      `json:"max_tokens"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return ToolCallResult{}, err
	}
	if p.Question == "" {
		return textResult("Error: question is required", true), nil
	}
	if p.MaxTokens == 0 {
		p.MaxTokens = 4096
	}

	t.buildIndex()

	var sourceFiles []string
	if len(p.SourceFiles) > 0 {
		sourceFiles = p.SourceFiles
	} else {
		sourceFiles = autoSelectFiles(p.Question)
	}

	var sourceContext []string
	for _, f := range sourceFiles {
		data, err := os.ReadFile(filepath.Join(t.SourceDir, f))
		if err != nil {
			data, err = os.ReadFile(filepath.Join(t.SourceDir, "testdata", f))
		}
		if err != nil {
			continue
		}
		content := string(data)
		if len(content) > 8000 {
			content = content[:8000] + "\n... (truncated)"
		}
		sourceContext = append(sourceContext, fmt.Sprintf("--- %s ---\n%s", f, content))
	}

	systemPrompt := "You are a Lisp/FFI expert helping an AI agent use a Common Lisp interpreter with Go FFI.\n" +
		"The interpreter is embedded in a Go application (miniClaudeCode-go).\n" +
		"Answer concisely and accurately with code examples. Focus on exact syntax."

	userPrompt := fmt.Sprintf("Symbol index:\n%s\n\nSource code:\n%s\n\nQuestion: %s",
		t.index, strings.Join(sourceContext, "\n\n"), p.Question)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	answer, err := t.LLM.Call(ctx, systemPrompt, userPrompt, p.MaxTokens)
	if err != nil {
		return textResult(fmt.Sprintf("Error: LLM call failed: %v", err), true), nil
	}
	return textResult(answer, false), nil
}

func (t *LispGuideTool) buildIndex() {
	t.indexOnce.Do(func() {
		var sb strings.Builder
		entries, err := os.ReadDir(t.SourceDir)
		if err != nil {
			t.index = "(no source files)"
			return
		}
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".go") {
				sb.WriteString(e.Name() + "\n")
			}
		}
		t.index = sb.String()
	})
}

func autoSelectFiles(question string) []string {
	q := strings.ToLower(question)
	selected := make(map[string]bool)

	mappings := []struct {
		keywords []string
		files    []string
	}{
		{[]string{"ffi", "go:", "struct", "reflect", "goval", "interop"}, []string{"go_ffi.go", "go_struct.go"}},
		{[]string{"time", "duration", "now"}, []string{"ffi_time.go", "time.go"}},
		{[]string{"channel", "select", "spawn", "goroutine"}, []string{"go_channels.go"}},
		{[]string{"format", "~a", "~d"}, []string{"format.go"}},
		{[]string{"list", "car", "cdr", "cons"}, []string{"list_ops.go"}},
		{[]string{"string", "char", "substring"}, []string{"strings.go"}},
		{[]string{"array", "make-array", "aref"}, []string{"data_structures.go"}},
		{[]string{"clos", "defclass", "defmethod"}, []string{"clos.go"}},
		{[]string{"condition", "error", "handler-case"}, []string{"conditions.go"}},
		{[]string{"hash", "hash-table", "gethash"}, []string{"data_structures.go"}},
		{[]string{"macro", "defmacro", "macroexpand"}, []string{"macroexpand.go"}},
		{[]string{"sequence", "mapcar", "filter", "reduce"}, []string{"seq_construct.go", "seq_search.go"}},
		{[]string{"number", "arithmetic", "math"}, []string{"arithmetic.go", "numbers.go"}},
		{[]string{"type", "type-of", "typep"}, []string{"type_system.go"}},
		{[]string{"stream", "read", "print", "io"}, []string{"streams.go", "io.go"}},
		{[]string{"package", "defpackage", "in-package"}, []string{"packages.go"}},
	}

	for _, m := range mappings {
		for _, kw := range m.keywords {
			if strings.Contains(q, kw) {
				for _, f := range m.files {
					selected[f] = true
				}
				break
			}
		}
	}

	if len(selected) == 0 {
		for _, f := range []string{"go_ffi.go", "go_struct.go", "eval_entry.go", "stdlib.go"} {
			selected[f] = true
		}
	}

	selected["go_interop.go"] = true

	files := make([]string, 0, len(selected))
	for f := range selected {
		files = append(files, f)
	}
	sort.Strings(files)
	return files
}
