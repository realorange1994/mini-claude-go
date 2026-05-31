package main

import (
	"fmt"
	"os"

	"miniclaudecode-go/pkg/core/cli"
)

func main() {
	// Parse CLI flags (rearranges args so flags come before positional args)
	a := cli.ParseFlags()

	// Load user config from ~/.miniclaude/config.json and project-local .miniclaude.json
	projectDir := *a.ProjectDir
	if projectDir == "" {
		pwd := os.Getenv("PWD")
		if pwd == "" {
			pwd, _ = os.Getwd()
		}
		projectDir = pwd
	}
	cfg := cli.ReadUserConfig(projectDir)

	// Resolve: flags > env > config > defaults
	rc, err := cli.ResolveConfig(a, cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}

	// Run the application (one-shot or REPL)
	if err := cli.Run(rc); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
