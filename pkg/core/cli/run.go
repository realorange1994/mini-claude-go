package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"

	"miniclaudecode-go/pkg/core/agent"
	"miniclaudecode-go/pkg/core/repl"
	"miniclaudecode-go/pkg/core/slashcmd"
)

// Run is the main entry point for the CLI application.
// It creates the agent session, handles one-shot mode, and starts the REPL.
func Run(rc *ResolvedConfig) error {
	// ── Create agent session ──
	config := agent.AgentConfig{
		Model:        rc.Model,
		Cwd:          rc.Cwd,
		MaxTurns:     rc.MaxTurns,
		CompactAfter: rc.CompactAfter,
		AutoCompact:  rc.AutoCompact,
		SessionPath:  rc.SessionPath,
		Timeout:      rc.Timeout,
	}

	runtime2, err := agent.NewAgentSessionRuntime(config)
	if err != nil {
		return fmt.Errorf("creating runtime: %w", err)
	}

	sess, err := runtime2.NewSession(rc.Model, rc.Cwd)
	if err != nil {
		return fmt.Errorf("creating session: %w", err)
	}

	// Wire LLM client
	llmClient := agent.NewHTTPClient(agent.HTTPClientConfig{
		BaseURL:      rc.BaseURL,
		APIKey:       rc.APIKey,
		DefaultModel: rc.Model,
		MaxTokens:    rc.MaxTokens,
	})
	sess.SetLLMClient(llmClient)

	// Streaming output
	if rc.Stream {
		sess.SetStreamCallback(func(text string) {
			fmt.Fprint(os.Stdout, text)
		})
	}

	// ── One-shot mode (message provided as CLI arg) ──
	if rc.Message != "" {
		if err := sess.Run(rc.Message); err != nil {
			return fmt.Errorf("agent error: %w", err)
		}
		if last := sess.GetLastMessage(); last != "" && !rc.Stream {
			fmt.Println(last)
		}
		return nil
	}

	// ── Interactive REPL ──
	runREPL(sess, rc)
	return nil
}

// runREPL starts an interactive read-eval-print loop with Ctrl+C signal handling.
func runREPL(sess *agent.AgentSession, rc *ResolvedConfig) {
	fmt.Fprintf(os.Stderr, "miniClaude Code — model %s — cwd %s\n", rc.Model, rc.Cwd)
	fmt.Fprintf(os.Stderr, "Type your message, /help for commands, /exit or Ctrl+D to quit.\n")
	fmt.Fprintf(os.Stderr, "Ctrl+C to interrupt, double Ctrl+C within 1.5s to exit.\n\n")

	history := newREPLHistory()
	var inputCancelFn context.CancelFunc

	// Install Ctrl+C handler (Windows: SetConsoleCtrlHandler, non-Windows: no-op).
	repl.InstallCtrlCHandler()

	// Interrupt callback
	interruptFn := func() {
		if inputCancelFn != nil {
			inputCancelFn()
		}
		sess.CancelCurrentTurn()
	}

	// SetInterruptHandler is called after the double-press check
	repl.SetInterruptHandler(func() {
		fmt.Fprintln(os.Stderr, "[Interrupted. Press Ctrl+C again within 1.5s to exit.]")
		interruptFn()
	})

	// Signal channel for SIGINT (non-Windows) and SIGTERM.
	sigCh := make(chan os.Signal, 1)
	if runtime.GOOS == "windows" {
		signal.Notify(sigCh, syscall.SIGTERM)
	} else {
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	}

	go func() {
		for sig := range sigCh {
			if runtime.GOOS != "windows" && sig == os.Interrupt {
				if repl.CheckDoubleInterrupt() {
					fmt.Fprintln(os.Stderr, "[Exiting...]")
					os.Exit(0)
				}
			}
			if sig == syscall.SIGTERM {
				fmt.Fprintln(os.Stderr, "[Received SIGTERM, exiting...]")
				os.Exit(0)
			}
			fmt.Fprintln(os.Stderr, "[Interrupted. Press Ctrl+C again within 1.5s to exit.]")
			interruptFn()
		}
	}()

	// Check if stdin has data (piped input) and process it before entering REPL.
	if stat, _ := os.Stdin.Stat(); stat.Mode()&os.ModeCharDevice == 0 {
		data, err := io.ReadAll(os.Stdin)
		if err == nil && len(data) > 0 {
			piped := strings.TrimSpace(string(data))
			if piped != "" {
				if err := sess.Run(piped); err != nil {
					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
					os.Exit(1)
				}
				if last := sess.GetLastMessage(); last != "" && !rc.Stream {
					fmt.Println(last)
				}
				return
			}
		}
	}

	// Ensure console is in correct mode before starting REPL.
	repl.EnsureConsoleInputMode()

	for {
		repl.EnsureConsoleInputMode()
		fmt.Fprintf(os.Stderr, "> ")

		line, err := repl.ReadLine()

		if err != nil {
			if errors.Is(err, io.EOF) {
				fmt.Fprintln(os.Stderr)
				return
			}
			fmt.Fprintln(os.Stderr)
			continue
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Handle slash commands
		if strings.HasPrefix(line, "/") {
			if handleSlashCommand(line, sess, rc, &history) {
				return
			}
			continue
		}

		history.Add(line)

		if err := sess.Run(line); err != nil {
			if errors.Is(err, context.Canceled) {
				fmt.Fprintln(os.Stderr, "[Turn interrupted.]")
			} else {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			}
			continue
		}

		if !rc.Stream {
			if last := sess.GetLastMessage(); last != "" {
				fmt.Println(last)
			}
		}
		fmt.Fprintln(os.Stderr)
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
func handleSlashCommand(cmd string, sess *agent.AgentSession, rc *ResolvedConfig, history *replHistory) bool {
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
			fmt.Fprintf(os.Stderr, "Working directory: %s\n", rc.Cwd)
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
			history.entries = history.entries[:len(history.entries)-1]
			if err := sess.Run(last); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			} else if !rc.Stream {
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
			if rc.Editor != "" {
				editor = rc.Editor
			} else if runtime.GOOS == "windows" {
				editor = "notepad"
			} else {
				editor = "vi"
			}
		}
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
		} else if !rc.Stream {
			if resp := sess.GetLastMessage(); resp != "" {
				fmt.Println(resp)
			}
		}

	case "/multiline", "/m":
		fmt.Fprintln(os.Stderr, "Enter multi-line input. End with a line containing only '.' or press Ctrl+D.")
		var lines []string
		for {
			fmt.Fprintf(os.Stderr, "... ")
			l, err := repl.ReadLine()
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
			} else if !rc.Stream {
				if resp := sess.GetLastMessage(); resp != "" {
					fmt.Println(resp)
				}
			}
		}

	case "/config":
		fmt.Fprintf(os.Stderr, "Model: %s\n", sess.GetModel())
		fmt.Fprintf(os.Stderr, "CWD: %s\n", rc.Cwd)
		fmt.Fprintf(os.Stderr, "Streaming: %v\n", rc.Stream)

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

	case "/commands":
		// List available slash commands (aligned to TS /commands)
		for _, c := range slashcmd.BuiltinSlashCommands() {
			fmt.Fprintf(os.Stderr, "  /%-18s %s\n", c.Name, c.Description)
		}

	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s. Type /help for available commands.\n", command)
	}

	return false
}

func slashHelp() string {
	// Build help text from slashcmd.BuiltinSlashCommands plus REPL-only commands
	var b strings.Builder
	b.WriteString("Available commands:\n")

	// REPL-only commands (not in slashcmd registry)
	replCommands := []struct {
		name, desc string
	}{
		{"help, /h", "Show this help"},
		{"exit, /quit, /q", "Exit the REPL"},
		{"model [NAME]", "Show or set the model"},
		{"cwd, /cd [DIR]", "Show or change working directory"},
		{"compact", "Compact conversation history"},
		{"clear", "Clear conversation history"},
		{"history", "Show input history"},
		{"redo", "Re-run the last message"},
		{"edit", "Open $EDITOR for multi-line input"},
		{"multiline, /m", "Enter multi-line mode (end with '.' or Ctrl+D)"},
		{"config", "Show current configuration"},
		{"shell, /! CMD", "Run a shell command"},
		{"commands", "List all built-in slash commands"},
	}
	for _, c := range replCommands {
		fmt.Fprintf(&b, "  /%-22s %s\n", c.name, c.desc)
	}
	return b.String()
}