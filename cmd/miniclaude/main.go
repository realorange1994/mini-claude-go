package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"miniclaudecode-go/pkg/core/agent"
)

func main() {
	model := flag.String("model", "", "Anthropic model to use")
	apiKey := flag.String("api-key", "", "API key (overrides ANTHROPIC_API_KEY/ANTHROPIC_AUTH_TOKEN env and config file)")
	baseURL := flag.String("base-url", "", "Custom API base URL (overrides config file)")
	_ = flag.String("mode", "ask", "Permission mode (ask|auto|bypass|plan)")
	maxTurns := flag.Int("max-turns", 0, "Max agent loop turns per message (0 = unlimited)")
	stream := flag.Bool("stream", false, "Enable streaming output")
	projectDir := flag.String("dir", "", "Project directory (change working directory before starting)")
	_ = flag.String("resume", "", "Resume from a transcript file path or 'last' for most recent")
	compactAfter := flag.Int("compact-after", 50, "Auto-compact after N turns")
	autoCompact := flag.Bool("auto-compact", false, "Enable auto-compaction")
	sessionPath := flag.String("session-path", "", "Session storage path")
	maxTokens := flag.Int("max-tokens", 8192, "Max tokens to generate per turn")

	// Rearrange os.Args so flags come before positional args.
	// Go's flag package stops parsing at the first non-flag argument,
	// so "exe "msg" --model M2.7" would treat --model as a positional arg.
	var flags, positional []string
	for i := 1; i < len(os.Args); i++ {
		if strings.HasPrefix(os.Args[i], "-") {
			flags = append(flags, os.Args[i])
			// If this flag takes a value (next arg doesn't start with "-"),
			// grab it too — but only if it exists and isn't another flag.
			if i+1 < len(os.Args) && !strings.HasPrefix(os.Args[i+1], "-") {
				// Check if this flag is a bool flag (doesn't take a value)
				f := flag.Lookup(strings.TrimLeft(os.Args[i], "-"))
				if f == nil {
					f = flag.Lookup(strings.TrimLeft(os.Args[i], "--"))
				}
				if f != nil && f.DefValue == "true" || f.DefValue == "false" {
					// Bool flag, don't consume next arg
				} else {
					i++
					flags = append(flags, os.Args[i])
				}
			}
		} else {
			positional = append(positional, os.Args[i])
		}
	}
	os.Args = append([]string{os.Args[0]}, append(flags, positional...)...)

	flag.Parse()

	// Get message from positional args.
	args := flag.Args()
	var msg string
	if len(args) > 0 {
		msg = strings.Join(args, " ")
	}
	if msg == "" {
		fmt.Fprintln(os.Stderr, "Error: no message provided")
		fmt.Fprintln(os.Stderr, "Usage: miniclaude --model M2.7 --max-turns 1 \"hello world\"")
		os.Exit(1)
	}

	// Resolve working directory: --dir flag > PWD > os.Getwd()
	cwd := *projectDir
	if cwd == "" {
		cwdEnv := os.Getenv("PWD")
		if cwdEnv == "" {
			cwdEnv, _ = os.Getwd()
		}
		cwd = cwdEnv
	}

	// Change working directory if --dir is specified
	if *projectDir != "" {
		if err := os.Chdir(*projectDir); err != nil {
			fmt.Fprintf(os.Stderr, "[!] Failed to change working directory to %s: %v\n", *projectDir, err)
		}
	}

	if *sessionPath == "" {
		home, _ := os.UserHomeDir()
		*sessionPath = fmt.Sprintf("%s/.miniclaude/sessions", home)
	}

	// Resolve API key: flag > env > error
	key := *apiKey
	if key == "" {
		key = os.Getenv("ANTHROPIC_API_KEY")
	}
	if key == "" {
		key = os.Getenv("ANTHROPIC_AUTH_TOKEN")
	}
	if key == "" {
		fmt.Fprintln(os.Stderr, "Error: no API key. Set ANTHROPIC_API_KEY env, ANTHROPIC_AUTH_TOKEN env, or pass --api-key")
		os.Exit(1)
	}

	// Resolve base URL: flag > env > Anthropic default
	url := *baseURL
	if url == "" {
		url = os.Getenv("ANTHROPIC_BASE_URL")
	}
	if url == "" {
		url = "https://api.anthropic.com"
	}
	// Ensure URL ends with /v1/messages for Anthropic-compatible APIs
	if !strings.HasSuffix(url, "/v1/messages") {
		url = strings.TrimSuffix(url, "/") + "/v1/messages"
	}

	// Resolve model: flag > ANTHROPIC_MODEL env > default
	modelVal := *model
	if modelVal == "" {
		modelVal = os.Getenv("ANTHROPIC_MODEL")
	}
	if modelVal == "" {
		modelVal = "claude-sonnet-4-20250514"
	}


	config := agent.AgentConfig{
		Model:        modelVal,
		Cwd:          cwd,
		MaxTurns:     *maxTurns,
		CompactAfter: *compactAfter,
		AutoCompact:  *autoCompact,
		SessionPath:  *sessionPath,
	}

	runtime, err := agent.NewAgentSessionRuntime(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating runtime: %v\n", err)
		os.Exit(1)
	}

	sess, err := runtime.NewSession(modelVal, cwd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating session: %v\n", err)
		os.Exit(1)
	}

	// Wire LLM client (HTTP-based, Anthropic-compatible API)
	llmClient := agent.NewHTTPClient(agent.HTTPClientConfig{
		BaseURL:      url,
		APIKey:       key,
		DefaultModel: modelVal,
		MaxTokens:    *maxTokens,
	})
	sess.SetLLMClient(llmClient)

	// Optional: streaming output
	if *stream {
		sess.SetStreamCallback(func(text string) {
			fmt.Fprint(os.Stderr, text)
		})
	}


	if err := sess.Run(msg); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Print final assistant response
	if last := sess.GetLastMessage(); last != "" {
		fmt.Println(last)
	}
}
