package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"miniclaudecode-go/pkg/core/agent"
	"miniclaudecode-go/pkg/core/repl"
)

// config mirrors the ~/.miniclaude/config.json structure.
type config struct {
	APIKey    string `json:"api_key,omitempty"`
	BaseURL   string `json:"base_url,omitempty"`
	Model     string `json:"model,omitempty"`
	MaxTokens int    `json:"max_tokens,omitempty"`
	Timeout   string `json:"timeout,omitempty"` // e.g. "5m", "120s"

	// REPL preferences
	Stream      bool   `json:"stream,omitempty"`
	AutoCompact bool   `json:"auto_compact,omitempty"`
	Prompt      string `json:"prompt,omitempty"`        // custom system prompt
	ProjectDir  string `json:"project_dir,omitempty"`   // default working directory
	Editor      string `json:"editor,omitempty"`         // editor for /edit command
	HistoryFile string `json:"history_file,omitempty"`  // readline history path
}

// readConfig loads config from the standard locations:
// 1. ~/.miniclaude/config.json (global)
// 2. .miniclaude.json in the project directory (project-local, overrides global)
func readConfig(projectDir string) *config {
	cfg := &config{}

	// Global config: ~/.miniclaude/config.json
	home, _ := os.UserHomeDir()
	if home != "" {
		loadConfigFile(filepath.Join(home, ".miniclaude", "config.json"), cfg)
	}

	// Project-local config: .miniclaude.json in project dir
	if projectDir != "" {
		loadConfigFile(filepath.Join(projectDir, ".miniclaude.json"), cfg)
	}

	return cfg
}

// loadConfigFile reads a JSON config file and merges non-zero fields into cfg.
func loadConfigFile(path string, cfg *config) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var overlay config
	if err := json.Unmarshal(data, &overlay); err != nil {
		fmt.Fprintf(os.Stderr, "[!] Invalid config file %s: %v\n", path, err)
		return
	}
	// Merge: overlay non-zero fields
	if overlay.APIKey != "" {
		cfg.APIKey = overlay.APIKey
	}
	if overlay.BaseURL != "" {
		cfg.BaseURL = overlay.BaseURL
	}
	if overlay.Model != "" {
		cfg.Model = overlay.Model
	}
	if overlay.MaxTokens != 0 {
		cfg.MaxTokens = overlay.MaxTokens
	}
	if overlay.Timeout != "" {
		cfg.Timeout = overlay.Timeout
	}
	if overlay.Stream {
		cfg.Stream = overlay.Stream
	}
	if overlay.AutoCompact {
		cfg.AutoCompact = overlay.AutoCompact
	}
	if overlay.Prompt != "" {
		cfg.Prompt = overlay.Prompt
	}
	if overlay.ProjectDir != "" {
		cfg.ProjectDir = overlay.ProjectDir
	}
	if overlay.Editor != "" {
		cfg.Editor = overlay.Editor
	}
	if overlay.HistoryFile != "" {
		cfg.HistoryFile = overlay.HistoryFile
	}
}

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
	timeout := flag.Duration("timeout", 0, "Global timeout for the entire agent session (0 = no timeout)")
	systemPrompt := flag.String("system-prompt", "", "Custom system prompt (overrides config file)")

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

	// Get message from positional args.
	args := flag.Args()
	var msg string
	if len(args) > 0 {
		msg = strings.Join(args, " ")
	}

	// ── Resolve working directory ──────────────────────────────────────
	cwd := *projectDir
	if cwd == "" {
		cwdEnv := os.Getenv("PWD")
		if cwdEnv == "" {
			cwdEnv, _ = os.Getwd()
		}
		cwd = cwdEnv
	}

	// ── Read config file ───────────────────────────────────────────────
	cfg := readConfig(cwd)

	// Change working directory if --dir is specified or config specifies one
	resolveDir := *projectDir
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

	// ── Resolve API key: flag > env > config > error ───────────────────
	key := *apiKey
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
		fmt.Fprintln(os.Stderr, "Error: no API key. Set ANTHROPIC_API_KEY env, ANTHROPIC_AUTH_TOKEN env, use --api-key, or add api_key to ~/.miniclaude/config.json")
		os.Exit(1)
	}

	// ── Resolve base URL: flag > env > config > Anthropic default ──────
	url := *baseURL
	if url == "" {
		url = os.Getenv("ANTHROPIC_BASE_URL")
	}
	if url == "" {
		url = cfg.BaseURL
	}
	if url == "" {
		url = "https://api.anthropic.com"
	}
	// Ensure URL ends with /v1/messages for Anthropic-compatible APIs
	if !strings.HasSuffix(url, "/v1/messages") {
		if strings.HasSuffix(url, "/v1/") {
			url = url + "messages"
		} else if strings.HasSuffix(url, "/v1") {
			url = url + "/messages"
		} else {
			url = strings.TrimSuffix(url, "/") + "/v1/messages"
		}
	}

	// ── Resolve model: flag > env > config > default ───────────────────
	modelVal := *model
	if modelVal == "" {
		modelVal = os.Getenv("ANTHROPIC_MODEL")
	}
	if modelVal == "" {
		modelVal = cfg.Model
	}
	if modelVal == "" {
		modelVal = "claude-sonnet-4-20250514"
	}

	// ── Resolve max tokens: flag > config > default ────────────────────
	maxTokensVal := *maxTokens
	if maxTokensVal == 0 && cfg.MaxTokens != 0 {
		maxTokensVal = cfg.MaxTokens
	}
	if maxTokensVal == 0 {
		maxTokensVal = 8192
	}

	// ── Resolve timeout: flag > config > default (0 = no timeout) ───────
	timeoutVal := *timeout
	if *timeout == 0 && cfg.Timeout != "" {
		if d, err := time.ParseDuration(cfg.Timeout); err == nil {
			timeoutVal = d
		}
	}

	// ── Resolve streaming: flag > config ───────────────────────────────
	streamVal := *stream || cfg.Stream

	// ── Resolve auto-compact: flag > config ────────────────────────────
	autoCompactVal := *autoCompact || cfg.AutoCompact

	// ── Resolve system prompt: flag > config ───────────────────────────
	systemPromptVal := *systemPrompt
	if systemPromptVal == "" {
		systemPromptVal = cfg.Prompt
	}

	// ── Session path ───────────────────────────────────────────────────
	sessPath := *sessionPath
	if sessPath == "" {
		home, _ := os.UserHomeDir()
		sessPath = fmt.Sprintf("%s/.miniclaude/sessions", home)
	}

	// ── Create agent session ───────────────────────────────────────────
	config := agent.AgentConfig{
		Model:        modelVal,
		Cwd:          cwd,
		MaxTurns:     *maxTurns,
		CompactAfter: *compactAfter,
		AutoCompact:  autoCompactVal,
		SessionPath:  sessPath,
		Timeout:      timeoutVal,
	}

	runtime2, err := agent.NewAgentSessionRuntime(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating runtime: %v\n", err)
		os.Exit(1)
	}

	sess, err := runtime2.NewSession(modelVal, cwd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating session: %v\n", err)
		os.Exit(1)
	}

	// Wire LLM client (HTTP-based, Anthropic-compatible API)
	llmClient := agent.NewHTTPClient(agent.HTTPClientConfig{
		BaseURL:      url,
		APIKey:       key,
		DefaultModel: modelVal,
		MaxTokens:    maxTokensVal,
	})
	sess.SetLLMClient(llmClient)

	// Streaming output
	if streamVal {
		sess.SetStreamCallback(func(text string) {
			fmt.Fprint(os.Stdout, text)
		})
	}

	// ── One-shot mode (message provided as CLI arg) ────────────────────
	if msg != "" {
		if err := sess.Run(msg); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if last := sess.GetLastMessage(); last != "" && !streamVal {
			fmt.Println(last)
		}
		return
	}

	// ── Interactive REPL ────────────────────────────────────────────────
	runREPL(sess, streamVal, modelVal, cwd)
}

// runREPL starts an interactive read-eval-print loop with Ctrl+C signal handling.
func runREPL(sess *agent.AgentSession, stream bool, modelVal string, cwd string) {
	fmt.Fprintf(os.Stderr, "miniClaude Code — model %s — cwd %s\n", modelVal, cwd)
	fmt.Fprintf(os.Stderr, "Type your message, /help for commands, /exit or Ctrl+D to quit.\n")
	fmt.Fprintf(os.Stderr, "Ctrl+C to interrupt, double Ctrl+C within 1.5s to exit.\n\n")

	history := newREPLHistory()
	var inputCancelFn context.CancelFunc // set before reading, nil during agent run

	// Install Ctrl+C handler (Windows: SetConsoleCtrlHandler, non-Windows: no-op).
	// Must be called from main(), not init(), because Go runtime registers its own
	// handler during initialization which would override ours.
	repl.InstallCtrlCHandler()

	// Interrupt callback — called by SetConsoleCtrlHandler (Windows) and signal handler (SIGTERM).
	// Note: double-press detection is handled in the SetConsoleCtrlHandler callback itself.
	interruptFn := func() {
		// Cancel input context if reading
		if inputCancelFn != nil {
			inputCancelFn()
		}
		// Cancel current agent turn
		sess.CancelCurrentTurn()
	}

	// On Windows, SetConsoleCtrlHandler catches Ctrl+C and calls SetInterruptHandler directly.
	// On non-Windows, we rely on signal.Notify for SIGINT.
	repl.SetInterruptHandler(func() {
		fmt.Fprintln(os.Stderr, "\n[Interrupted. Press Ctrl+C again within 1.5s to exit.]")
		interruptFn()
	})

	// Signal channel for SIGTERM only (for non-Windows or external signals).
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM)

	go func() {
		for range sigCh {
			fmt.Fprintln(os.Stderr, "\n[Interrupted. Press Ctrl+C again within 1.5s to exit.]")
			interruptFn()
		}
	}()

	// Ensure console is in correct mode before starting REPL.
	repl.EnsureConsoleInputMode()

	for {
		repl.EnsureConsoleInputMode()
		fmt.Fprintf(os.Stderr, "> ")

		// Read input - using simple ReadLine that works reliably on Windows
		line, err := repl.ReadLine()

		if err != nil {
			// Ctrl+C causes Scanln to return EOF - just continue the loop
			// SetConsoleCtrlHandler prevents process termination
			fmt.Fprintln(os.Stderr) // newline after interrupt message
			continue
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Handle slash commands
		if strings.HasPrefix(line, "/") {
			if handleSlashCommand(line, sess, nil, stream, &history, cwd) {
				return // exit
			}
			continue
		}

		// Add to history
		history.Add(line)

		// Run agent
		if err := sess.Run(line); err != nil {
			if errors.Is(err, context.Canceled) {
				fmt.Fprintln(os.Stderr, "[Turn interrupted.]")
			} else {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			}
			continue
		}

		// Print response (if not streaming, print the full response)
		if !stream {
			if last := sess.GetLastMessage(); last != "" {
				fmt.Println(last)
			}
		}
		fmt.Fprintln(os.Stderr) // blank line between turns
	}
}

// replHistory tracks conversation history for the REPL.
type replHistory struct {
	entries []string
	maxSize int
}

func newREPLHistory() replHistory {
	return replHistory{maxSize: 1000}
}

func (h *replHistory) Add(entry string) {
	h.entries = append(h.entries, entry)
	if len(h.entries) > h.maxSize {
		h.entries = h.entries[len(h.entries)-h.maxSize:]
	}
}

func (h *replHistory) Last() string {
	if len(h.entries) == 0 {
		return ""
	}
	return h.entries[len(h.entries)-1]
}

// handleSlashCommand processes REPL slash commands.
// Returns true if the user wants to exit.
func handleSlashCommand(cmd string, sess *agent.AgentSession, reader *bufio.Reader, stream bool, history *replHistory, cwd string) bool {
	parts := strings.SplitN(cmd, " ", 2)
	command := parts[0]
	arg := ""
	if len(parts) > 1 {
		arg = strings.TrimSpace(parts[1])
	}

	switch command {
	case "/exit", "/quit", "/q":
		fmt.Fprintln(os.Stderr, "Goodbye!")
		return true

	case "/help", "/h", "/?":
		fmt.Fprintln(os.Stderr, slashHelp())

	case "/model":
		if arg != "" {
			sess.SetModel(arg)
			fmt.Fprintf(os.Stderr, "Model set to: %s\n", arg)
		} else {
			fmt.Fprintf(os.Stderr, "Current model: %s\n", sess.GetModel())
		}

	case "/cwd", "/cd":
		if arg != "" {
			if err := os.Chdir(arg); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			} else {
				if actual, _ := os.Getwd(); actual != "" {
					fmt.Fprintf(os.Stderr, "Working directory: %s\n", actual)
				}
			}
		} else {
			fmt.Fprintf(os.Stderr, "Working directory: %s\n", cwd)
		}

	case "/compact":
		fmt.Fprintln(os.Stderr, "Compacting conversation...")
		if err := sess.Compact(); err != nil {
			fmt.Fprintf(os.Stderr, "Compact error: %v\n", err)
		} else {
			fmt.Fprintln(os.Stderr, "Conversation compacted.")
		}

	case "/clear":
		if err := sess.ClearHistory(); err != nil {
			fmt.Fprintf(os.Stderr, "Clear error: %v\n", err)
		} else {
			fmt.Fprintln(os.Stderr, "Conversation history cleared.")
		}

	case "/history":
		if len(history.entries) == 0 {
			fmt.Fprintln(os.Stderr, "No history.")
		} else {
			for i, entry := range history.entries {
				fmt.Fprintf(os.Stderr, "  %d: %s\n", i+1, entry)
			}
		}

	case "/redo":
		last := history.Last()
		if last == "" {
			fmt.Fprintln(os.Stderr, "No previous message to redo.")
		} else {
			// Remove last from history (it'll be re-added by Run)
			history.entries = history.entries[:len(history.entries)-1]
			if err := sess.Run(last); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			} else if !stream {
				if resp := sess.GetLastMessage(); resp != "" {
					fmt.Println(resp)
				}
			}
		}

	case "/edit":
		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = os.Getenv("VISUAL")
		}
		if editor == "" {
			if runtime.GOOS == "windows" {
				editor = "notepad"
			} else {
				editor = "vi"
			}
		}
		// Create temp file for multi-line input
		tmpFile, err := os.CreateTemp("", "miniclaude-input-*.txt")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating temp file: %v\n", err)
			break
		}
		tmpPath := tmpFile.Name()
		tmpFile.Close()
		defer os.Remove(tmpPath)

		editCmd := exec.Command(editor, tmpPath)
		editCmd.Stdin = os.Stdin
		editCmd.Stdout = os.Stdout
		editCmd.Stderr = os.Stderr
		if err := editCmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Editor error: %v\n", err)
			break
		}

		content, err := os.ReadFile(tmpPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading editor output: %v\n", err)
			break
		}

		input := strings.TrimSpace(string(content))
		if input == "" {
			fmt.Fprintln(os.Stderr, "Empty input, skipping.")
			break
		}

		history.Add(input)
		if err := sess.Run(input); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		} else if !stream {
			if resp := sess.GetLastMessage(); resp != "" {
				fmt.Println(resp)
			}
		}

	case "/multiline", "/m":
		fmt.Fprintln(os.Stderr, "Enter multi-line input. End with a line containing only '.' or press Ctrl+D.")
		var lines []string
		for {
			fmt.Fprintf(os.Stderr, "... ")
			l, err := reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					break
				}
				fmt.Fprintf(os.Stderr, "Input error: %v\n", err)
				break
			}
			l = strings.TrimRight(l, "\r\n")
			if l == "." {
				break
			}
			lines = append(lines, l)
		}
		if len(lines) > 0 {
			input := strings.Join(lines, "\n")
			history.Add(input)
			if err := sess.Run(input); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			} else if !stream {
				if resp := sess.GetLastMessage(); resp != "" {
					fmt.Println(resp)
				}
			}
		}

	case "/config":
		fmt.Fprintf(os.Stderr, "Model: %s\n", sess.GetModel())
		fmt.Fprintf(os.Stderr, "CWD: %s\n", cwd)
		fmt.Fprintf(os.Stderr, "Streaming: %v\n", stream)

	case "/shell", "/!":
		if arg == "" {
			fmt.Fprintln(os.Stderr, "Usage: /shell <command>")
			break
		}
		shell := "/bin/bash"
		shellArg := "-c"
		if runtime.GOOS == "windows" {
			shell = "bash"
			shellArg = "-c"
			if _, err := exec.LookPath("bash"); err != nil {
				shell = "cmd.exe"
				shellArg = "/c"
			}
		}
		cmd := exec.Command(shell, shellArg, arg)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Run()

	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s. Type /help for available commands.\n", command)
	}

	return false
}

func slashHelp() string {
	return `Available commands:
  /help, /h        Show this help
  /exit, /quit, /q Exit the REPL
  /model [NAME]    Show or set the model
  /cwd, /cd [DIR]  Show or change working directory
  /compact         Compact conversation history
  /clear           Clear conversation history
  /history         Show input history
  /redo            Re-run the last message
  /edit            Open $EDITOR for multi-line input
  /multiline, /m   Enter multi-line mode (end with '.' or Ctrl+D)
  /config          Show current configuration
  /shell, /! CMD   Run a shell command
`
}
