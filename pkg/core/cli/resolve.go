package cli

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
)

// ResolvedConfig holds the final resolved configuration after merging
// CLI flags, environment variables, and config file values.
type ResolvedConfig struct {
	APIKey       string
	BaseURL      string
	Model        string
	MaxTokens    int
	Timeout      time.Duration
	Stream       bool
	AutoCompact  bool
	SystemPrompt string
	Cwd          string
	SessionPath  string
	MaxTurns     int
	CompactAfter int
	Mode         string
	Resume       string
	Editor       string
	Message      string
}

// ParsedArgs holds the raw parsed CLI arguments.
type ParsedArgs struct {
	Model        *string
	APIKey       *string
	BaseURL      *string
	Mode         *string
	MaxTurns     *int
	Stream       *bool
	ProjectDir   *string
	Resume       *string
	CompactAfter *int
	AutoCompact  *bool
	SessionPath  *string
	MaxTokens    *int
	TurnTimeout  *time.Duration
	SystemPrompt *string
}

// ParseFlags parses command-line flags, rearranging args so flags come before
// positional arguments (Go's flag package stops at the first non-flag arg).
func ParseFlags() *ParsedArgs {
	a := &ParsedArgs{
		Model:        flag.String("model", "", "Anthropic model to use"),
		APIKey:       flag.String("api-key", "", "API key (overrides ANTHROPIC_API_KEY/ANTHROPIC_AUTH_TOKEN env and config file)"),
		BaseURL:      flag.String("base-url", "", "Custom API base URL (overrides config file)"),
		Mode:         flag.String("mode", "ask", "Permission mode (ask|auto|bypass|plan)"),
		MaxTurns:     flag.Int("max-turns", 0, "Max agent loop turns per message (0 = unlimited)"),
		Stream:       flag.Bool("stream", false, "Enable streaming output"),
		ProjectDir:   flag.String("dir", "", "Project directory (change working directory before starting)"),
		Resume:       flag.String("resume", "", "Resume from a transcript file path or 'last' for most recent"),
		CompactAfter: flag.Int("compact-after", 50, "Auto-compact after N turns"),
		AutoCompact:  flag.Bool("auto-compact", false, "Enable auto-compaction"),
		SessionPath:  flag.String("session-path", "", "Session storage path"),
		MaxTokens:    flag.Int("max-tokens", 8192, "Max tokens to generate per turn"),
		TurnTimeout:  flag.Duration("turn-timeout", 0, "Per-turn timeout for LLM calls (0 = no timeout)"),
		SystemPrompt: flag.String("system-prompt", "", "Custom system prompt (overrides config file)"),
	}

	// Rearrange os.Args so flags come before positional args.
	var flags, positional []string
	for i := 1; i < len(os.Args); i++ {
		if strings.HasPrefix(os.Args[i], "-") {
			flags = append(flags, os.Args[i])
			if i+1 < len(os.Args) && !strings.HasPrefix(os.Args[i+1], "-") {
				f := flag.Lookup(strings.TrimLeft(os.Args[i], "-"))
				if f == nil {
					f = flag.Lookup(strings.TrimLeft(os.Args[i], "--"))
				}
				if f != nil && (f.DefValue == "true" || f.DefValue == "false") {
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

	return a
}

// Message returns the positional args joined as a single message.
func (a *ParsedArgs) Message() string {
	args := flag.Args()
	if len(args) > 0 {
		return strings.Join(args, " ")
	}
	return ""
}

// ResolveConfig merges CLI flags, environment variables, and config file values
// to produce the final ResolvedConfig.
func ResolveConfig(a *ParsedArgs, cfg *UserConfig) (*ResolvedConfig, error) {
	// ── Resolve working directory ──
	cwd := *a.ProjectDir
	if cwd == "" {
		cwdEnv := os.Getenv("PWD")
		if cwdEnv == "" {
			cwdEnv, _ = os.Getwd()
		}
		cwd = cwdEnv
	}

	// Change working directory if --dir is specified or config specifies one
	resolveDir := *a.ProjectDir
	if resolveDir == "" && cfg.ProjectDir != "" {
		resolveDir = cfg.ProjectDir
	}
	if resolveDir != "" {
		if err := os.Chdir(resolveDir); err != nil {
			fmt.Fprintf(os.Stderr, "[!] Failed to change working directory to %s: %v\n", resolveDir, err)
		}
		if actual, _ := os.Getwd(); actual != "" {
			cwd = actual
		}
	}

	// ── Resolve API key: flag > env > config > error ──
	key := *a.APIKey
	if key == "" {
		key = os.Getenv("ANTHROPIC_API_KEY")
	}
	if key == "" {
		key = os.Getenv("ANTHROPIC_AUTH_TOKEN")
	}
	if key == "" {
		key = cfg.APIKey
	}
	if key == "" {
		return nil, fmt.Errorf("no API key. Set ANTHROPIC_API_KEY env, ANTHROPIC_AUTH_TOKEN env, use --api-key, or add api_key to ~/.miniclaude/config.json")
	}

	// ── Resolve base URL: flag > env > config > Anthropic default ──
	url := *a.BaseURL
	if url == "" {
		url = os.Getenv("ANTHROPIC_BASE_URL")
	}
	if url == "" {
		url = cfg.BaseURL
	}
	if url == "" {
		url = "https://api.anthropic.com"
	}
	if !strings.HasSuffix(url, "/v1/messages") {
		if strings.HasSuffix(url, "/v1/") {
			url = url + "messages"
		} else if strings.HasSuffix(url, "/v1") {
			url = url + "/messages"
		} else {
			url = strings.TrimSuffix(url, "/") + "/v1/messages"
		}
	}

	// ── Resolve model: flag > env > config > default ──
	modelVal := *a.Model
	if modelVal == "" {
		modelVal = os.Getenv("ANTHROPIC_MODEL")
	}
	if modelVal == "" {
		modelVal = cfg.Model
	}
	if modelVal == "" {
		modelVal = "claude-sonnet-4-20250514"
	}

	// ── Resolve max tokens: flag > config > default ──
	maxTokensVal := *a.MaxTokens
	if maxTokensVal == 0 && cfg.MaxTokens != 0 {
		maxTokensVal = cfg.MaxTokens
	}
	if maxTokensVal == 0 {
		maxTokensVal = 8192
	}

	// ── Resolve per-turn timeout: flag > config ──
	timeoutVal := *a.TurnTimeout
	if timeoutVal == 0 && cfg.Timeout != "" {
		if d, err := time.ParseDuration(cfg.Timeout); err == nil {
			timeoutVal = d
		}
	}

	// ── Resolve streaming: flag > config ──
	streamVal := *a.Stream || cfg.Stream

	// ── Resolve auto-compact: flag > config ──
	autoCompactVal := *a.AutoCompact || cfg.AutoCompact

	// ── Resolve system prompt: flag > config ──
	systemPromptVal := *a.SystemPrompt
	if systemPromptVal == "" {
		systemPromptVal = cfg.Prompt
	}

	// ── Session path ──
	sessPath := *a.SessionPath
	if sessPath == "" {
		home, _ := os.UserHomeDir()
		sessPath = fmt.Sprintf("%s/.miniclaude/sessions", home)
	}

	return &ResolvedConfig{
		APIKey:       key,
		BaseURL:      url,
		Model:        modelVal,
		MaxTokens:    maxTokensVal,
		Timeout:      timeoutVal,
		Stream:       streamVal,
		AutoCompact:  autoCompactVal,
		SystemPrompt: systemPromptVal,
		Cwd:          cwd,
		SessionPath:  sessPath,
		MaxTurns:     *a.MaxTurns,
		CompactAfter: *a.CompactAfter,
		Mode:         *a.Mode,
		Resume:       *a.Resume,
		Editor:       cfg.Editor,
		Message:      a.Message(),
	}, nil
}
