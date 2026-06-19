package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"miniclaudecode-go/microlisp"
)

func main() {
	sourceDir := flag.String("source-dir", "", "Path to microlisp source directory (for lisp_guide tool)")
	flag.Parse()

	microlisp.InitGlobalEnv()

	s := NewMCPServer()

	RegisterEvalTool(s)
	RegisterExecTool(s)
	RegisterToolsTool(s)

	// Resolve source directory for lisp_guide
	if *sourceDir == "" {
		// Auto-detect: ../../microlisp/ relative to binary
		exe, err := os.Executable()
		if err == nil {
			*sourceDir = filepath.Join(filepath.Dir(exe), "..", "..", "microlisp")
		}
		if _, err := os.Stat(*sourceDir); err != nil {
			// Fallback: try relative to CWD
			*sourceDir = filepath.Join("..", "..", "microlisp")
		}
	}

	llm := NewAnthropicClient()
	RegisterGuideTool(s, llm, *sourceDir)

	if err := s.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "microlisp-mcp: %v\n", err)
		os.Exit(1)
	}
}
