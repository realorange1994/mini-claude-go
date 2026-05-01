package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

func main() {
	model := flag.String("model", "", "Anthropic model to use")
	apiKey := flag.String("api-key", "", "API key (overrides ANTHROPIC_API_KEY/ANTHROPIC_AUTH_TOKEN env and config file)")
	baseURL := flag.String("base-url", "", "Custom API base URL (overrides config file)")
	mode := flag.String("mode", "ask", "Permission mode (ask|auto|plan)")
	maxTurns := flag.Int("max-turns", 90, "Max agent loop turns per message")
	stream := flag.Bool("stream", false, "Enable streaming output")
	projectDir := flag.String("dir", "", "Project directory (change working directory before starting)")
	resumeFile := flag.String("resume", "", "Resume from a transcript file path or 'last' for most recent")
	flag.Parse()

	// Change working directory if --dir is specified
	if *projectDir != "" {
		if err := os.Chdir(*projectDir); err != nil {
			fmt.Fprintf(os.Stderr, "[!] Failed to change working directory to %s: %v\n", *projectDir, err)
		}
	}

	// Priority: flags > env > .claude/settings.json > defaults
	cfg := DefaultConfig()

	// Load from .claude/settings.json and .mcp.json (project-level config)
	if wd, err := os.Getwd(); err == nil {
		if fileCfg, found := LoadConfigFromFile(wd); found {
			if fileCfg.APIKey != "" {
				cfg.APIKey = fileCfg.APIKey
			}
			if fileCfg.BaseURL != "" {
				cfg.BaseURL = fileCfg.BaseURL
			}
			if fileCfg.Model != "" {
				cfg.Model = fileCfg.Model
			}
			// Carry over MCP and skills from file config
			if fileCfg.MCPManager != nil {
				cfg.MCPManager = fileCfg.MCPManager
			}
			if fileCfg.SkillLoader != nil {
				cfg.SkillLoader = fileCfg.SkillLoader
			}
		}
	}

	// Environment variables override settings file
	if envKey := os.Getenv("ANTHROPIC_API_KEY"); envKey != "" {
		cfg.APIKey = envKey
	} else if envKey := os.Getenv("ANTHROPIC_AUTH_TOKEN"); envKey != "" {
		cfg.APIKey = envKey
	}
	if envURL := os.Getenv("ANTHROPIC_BASE_URL"); envURL != "" {
		cfg.BaseURL = envURL
	}
	if envModel := os.Getenv("ANTHROPIC_MODEL"); envModel != "" {
		cfg.Model = envModel
	}

	// Flags override everything
	if *model != "" {
		cfg.Model = *model
	}
	if *apiKey != "" {
		cfg.APIKey = *apiKey
	}
	if *baseURL != "" {
		cfg.BaseURL = *baseURL
	}
	cfg.PermissionMode = PermissionMode(*mode)
	cfg.MaxTurns = *maxTurns

	// Validate: model is required
	if cfg.Model == "" {
		fmt.Fprintln(os.Stderr, "[!] No model specified. Set it via --model flag, ANTHROPIC_MODEL env, or model in .claude/settings.json")
		os.Exit(1)
	}

	defer cfg.Close()

	// Create FileHistory (used by both tools and agent loop)
	if wd, err := os.Getwd(); err == nil {
		cfg.FileHistory = NewSnapshotHistory(wd)
	}

	registry := DefaultRegistry()
	RegisterMCPAndSkills(registry, &cfg)
	RegisterFileHistoryTools(registry, cfg.FileHistory)

	// Initialize SessionMemory
	if wd, err := os.Getwd(); err == nil {
		sm := NewSessionMemory(wd)
		cfg.SessionMemory = sm
		RegisterMemoryTools(registry, sm)
		// Mark cached prompt dirty when memory is updated so it appears in next turn
		if cfg.cachedPrompt != nil {
			sm.SetOnAdd(func() {
				cfg.cachedPrompt.MarkDirty()
			})
		}
		sm.StartFlushLoop()
	}

	var agent *AgentLoop
	var err error

	// Resume from transcript or new session
	if *resumeFile != "" {
		path, err := findTranscript(*resumeFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[!] Resume failed: %v\n", err)
			fmt.Fprintln(os.Stderr, "[*] Starting a new session instead")
			agent, err = NewAgentLoop(cfg, registry, *stream)
			if err != nil {
				fmt.Fprintf(os.Stderr, "[!] %v\n", err)
				return
			}
		} else {
			agent, err = NewAgentLoopFromTranscript(cfg, registry, *stream, path, true)
			if err != nil {
				fmt.Fprintf(os.Stderr, "[!] Resume failed: %v\n", err)
				fmt.Fprintln(os.Stderr, "[*] Starting a new session instead")
				agent, err = NewAgentLoop(cfg, registry, *stream)
				if err != nil {
					fmt.Fprintf(os.Stderr, "[!] %v\n", err)
					return
				}
			} else {
				fmt.Printf("[+] Resumed session from transcript: %s\n", path)
			}
		}
	} else {
		agent, err = NewAgentLoop(cfg, registry, *stream)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[!] %v\n", err)
			return
		}
	}

	// One-shot mode: positional args are appended as prompt (works with --resume too)
	args := flag.Args()
	if len(args) > 0 {
		prompt := strings.Join(args, " ")
		result := agent.Run(prompt)

		// When streaming is enabled and stdout is a terminal, TerminalHandler
		// already displayed the text via stderr. Printing again would duplicate it.
		stdoutIsTerm := func() bool {
			fi, err := os.Stdout.Stat()
			if err != nil {
				return false
			}
			return fi.Mode()&os.ModeCharDevice != 0
		}
		if !agent.IsStreaming() || !stdoutIsTerm() {
			fmt.Println(result)
		}

		// Drain any pending sub-agent notifications before exit
		drainOneShotNotifications(agent)

		agent.Close()
		return
	}

	// Interactive REPL
	runInteractive(agent)
}

func runInteractive(agent *AgentLoop) {
	// Track Ctrl+C timing for double-press exit (works during agent.Run too)
	var lastCtrlC atomic.Int64 // stores UnixNano of last Ctrl+C

	// Set up Ctrl+C signal handler
	// The handler goroutine consumes signalCh; REPL select uses ctrlCh for
	// Ctrl+C notification (avoids two goroutines competing on signalCh).
	signalCh := make(chan os.Signal, 1)
	ctrlCh := make(chan struct{}, 1)
	signal.Notify(signalCh, syscall.SIGINT)
	go func() {
		for range signalCh {
			now := time.Now().UnixNano()
			prev := lastCtrlC.Load()
			if prev != 0 && time.Duration(now-prev) < 2*time.Second {
				// Double Ctrl+C within 2s -- exit immediately (clean up first)
				printResumeHint(agent)
				agent.Close()
				os.Exit(0)
			}
			lastCtrlC.Store(now)
			agent.SetInterrupted(true)
			// Non-blocking notify to REPL select (may already be processing)
			select {
			case ctrlCh <- struct{}{}:
			default:
			}
		}
	}()

	defer agent.Close()

	// Detect if stdin is a terminal (interactive) or piped
	isTerminal := func() bool {
		fi, err := os.Stdin.Stat()
		if err != nil {
			return false
		}
		return fi.Mode()&os.ModeCharDevice != 0
	}
	interactive := isTerminal()

	// Detect if stdout is a terminal (for streaming output decision)
	stdoutIsTerminal := func() bool {
		fi, err := os.Stdout.Stat()
		if err != nil {
			return false
		}
		return fi.Mode()&os.ModeCharDevice != 0
	}
	stdoutIsTerm := stdoutIsTerminal()

	// Run REPL read loop in a goroutine so the main loop can handle
	// Ctrl+C signals without being blocked on ReadString.
	// On Windows, Ctrl+C closes the stdin handle which can make
	// reopenStdin unreliable -- this approach avoids needing it entirely.
	type readResult struct {
		line string
		err  error
	}
	inputCh := make(chan readResult, 1)

	readStdin := func(reader *bufio.Reader) {
		line, err := reader.ReadString('\n')
		inputCh <- readResult{line: line, err: err}
	}

	stdinReader := bufio.NewReader(os.Stdin)
	go readStdin(stdinReader)

	loop:
	for {
		// Always restart the stdin reader goroutine at the start of each
		// iteration. This is necessary because:
		// 1. After agent.Run(), the goroutine may have died (Ctrl+C on Windows
		//    closes stdin, producing EOF). We must create a fresh one.
		// 2. After slash commands (which use `continue`), the old goroutine is
		//    also dead (same reason). Moving restart here ensures it runs for
		//    both code paths, fixing the bug where /help, /tools etc. caused
		//    the REPL to deadlock waiting for input on the next iteration.
		// The old goroutine (if still alive) will just block forever on the
		// old, unreferenced channel.
		stdinReader = bufio.NewReader(os.Stdin)
		inputCh = make(chan readResult, 1)
		go readStdin(stdinReader)

		// Drain async sub-agent notifications and display them
		if notifications := agent.DrainNotifications(); len(notifications) > 0 {
			fmt.Println("\n--- Sub-agent notifications ---")
			for _, n := range notifications {
				fmt.Println(n)
			}
			fmt.Println("------------------------------")

			// Inject into LLM context so model can act on them
			agent.InjectNotifications(notifications)
		}

		fmt.Print("\n> ")

		// Wait for either user input or Ctrl+C signal.
		// On Windows, Ctrl+C produces BOTH a SIGINT and may close stdin
		// (EOF). The select picks whichever arrives first. To correctly
		// detect Ctrl+C-triggered EOF, we check the lastCtrlC timestamp
		// (set atomically by the signal handler) instead of ctrlCh.
		var line string
		var isEOF bool
		select {
		case <-ctrlCh:
			// Ctrl+C while waiting for input at the ">" prompt.
			fmt.Fprintf(os.Stderr, "\n[WARN] Interrupting... (press Ctrl+C again within 2s to exit)\n")
			continue
		case r := <-inputCh:
			line = r.line
			isEOF = r.err == io.EOF
			if r.err != nil && !isEOF {
				// Read error (not EOF) -- try to recover
				if interactive {
					stdinReader = bufio.NewReader(os.Stdin)
					go readStdin(stdinReader)
					continue
				}
				break loop
			}
			if isEOF && strings.TrimSpace(line) == "" {
				// On Windows, Ctrl+C closes stdin (producing EOF)
				// at the same time as SIGINT. The signal handler goroutine
				// may not have processed SIGINT yet, so lastCtrlC might
				// still be 0. Wait briefly for either the ctrlCh signal
				// or the lastCtrlC update, then decide.
				select {
				case <-ctrlCh:
					// SIGINT confirmed -- reopen stdin and continue
					fmt.Fprintf(os.Stderr, "\n[WARN] Interrupting... (press Ctrl+C again within 2s to exit)\n")
					stdinReader = bufio.NewReader(os.Stdin)
					go readStdin(stdinReader)
					continue
				case <-time.After(200 * time.Millisecond):
					// No ctrlCh signal -- check lastCtrlC one more time
					// (handler may have run but ctrlCh was already consumed)
					prev := lastCtrlC.Load()
					if prev != 0 && time.Since(time.Unix(0, prev)) < 2*time.Second {
						fmt.Fprintf(os.Stderr, "\n[WARN] Interrupting... (press Ctrl+C again within 2s to exit)\n")
						stdinReader = bufio.NewReader(os.Stdin)
						go readStdin(stdinReader)
						continue
					}
					// True EOF (piped input closed, Ctrl+D)
					break loop
				}
			}
			// Got real input -- drain any stale Ctrl+C so it doesn't
			// fire on the next iteration and confuse the REPL.
			select {
			case <-ctrlCh:
			default:
			}
		}

		userInput := strings.TrimSpace(line)
		if userInput == "" {
			if !interactive {
				break loop
			}
			continue
		}

		// Check for exact command match -- only treat as command if the first
		// word is a known command. Unknown /xxx is passed through as prompt text.
		if strings.HasPrefix(userInput, "/") {
			parts := strings.Fields(userInput)
			cmd := strings.ToLower(parts[0])

			isKnownCmd := cmd == "/quit" || cmd == "/exit" || cmd == "/q" ||
				cmd == "/tools" || cmd == "/mode" || cmd == "/help" || cmd == "/resume" ||
				cmd == "/compact" || cmd == "/clear" || cmd == "/partialcompact"

			if !isKnownCmd {
				// Not a recognized command -- treat as normal prompt
			} else {
				switch cmd {
				case "/quit", "/exit", "/q":
					fmt.Println("Goodbye!")
					break loop
				case "/tools":
					fmt.Println("\nAvailable tools:")
					for _, t := range agent.registry.AllTools() {
						fmt.Printf("  - %s: %s\n", t.Name(), t.Description())
					}
					continue
				case "/mode":
					if len(parts) > 1 {
						modeVal := strings.ToLower(parts[1])
						switch modeVal {
						case "ask", "auto", "plan":
							agent.config.PermissionMode = PermissionMode(modeVal)
							fmt.Printf("Mode changed to: %s\n", modeVal)
						default:
							fmt.Printf("Unknown mode: %s\n", parts[1])
						}
					} else {
						fmt.Printf("Current mode: %s\n", agent.config.PermissionMode)
						fmt.Println("Usage: /mode [ask|auto|plan]")
					}
					continue
				case "/help":
					fmt.Println("Commands:")
					fmt.Println("  /help           -- Show available commands")
					fmt.Println("  /compact        -- Force context compaction")
					fmt.Println("  /partialcompact -- Directional partial compaction (up_to|from, [pivot])")
					fmt.Println("  /clear          -- Clear conversation history")
					fmt.Println("  /mode           -- Switch permission mode (ask|auto|plan)")
					fmt.Println("  /resume         -- Resume a previous session")
					fmt.Println("  /tools          -- List available tools")
					fmt.Println("  /quit           -- Exit")
					continue
				case "/compact":
					agent.ForceCompact()
					continue
				case "/partialcompact":
					dir := "up_to"
					pivot := 0
					if len(parts) > 1 {
						dir = strings.ToLower(parts[1])
					}
					if len(parts) > 2 {
						fmt.Sscanf(parts[2], "%d", &pivot)
					}
					agent.ForcePartialCompact(dir, pivot)
					continue
				case "/clear":
					count := agent.ClearHistory()
					agent.registry.ClearFilesRead()
					if count > 0 {
						fmt.Printf("[clear] Cleared %d messages.\n", count)
					} else {
						fmt.Println("[clear] No messages to clear.")
					}
					continue
				case "/resume":
					if len(parts) > 1 {
						target := parts[1]
						path, err := findTranscript(target)
						if err != nil {
							fmt.Printf("Error: %v\n", err)
							continue
						}
						newAgent, err := NewAgentLoopFromTranscript(agent.config, agent.registry, agent.useStream, path, true)
						if err != nil {
							fmt.Printf("Error resuming transcript: %v\n", err)
							continue
						}
						// Swap the agent (close old one first)
						agent.Close()
						agent = newAgent
						fmt.Printf("[+] Resumed session from transcript: %s\n", path)
					} else {
						// List available transcripts
						listTranscripts()
					}
					continue
				}
			}
		}
		fmt.Println()
		agent.SetInterrupted(false) // ensure clear before running
		result := agent.Run(userInput)
		agent.SetInterrupted(false) // clear after run

		// Drain any stale Ctrl+C signal from the channel. When the user
		// presses Ctrl+C to interrupt the agent, the signal handler sends
		// to ctrlCh. The agent detects IsInterrupted() and returns, but the
		// ctrlCh message is unconsumed. If we don't drain it, the next REPL
		// loop will pick it up and print an extraneous "[WARN] Interrupting...".
		select {
		case <-ctrlCh:
		default:
		}

		// In streaming mode, TerminalHandler displays output on stderr.
		// When stdout is a terminal, skip printing to avoid duplication.
		// When stdout is piped (not a terminal), always print so the
		// result is available on stdout for programmatic consumption.
		if !agent.IsStreaming() || !stdoutIsTerm {
			fmt.Println(result)
		}
		fmt.Println()
	}

	// Print resume hint exactly once at final exit
	printResumeHint(agent)
}

// printResumeHint prints a short resume hint on exit.
func printResumeHint(agent *AgentLoop) {
	tp := agent.TranscriptPath()
	if tp == "" {
		return
	}
	// Extract just the filename (e.g. 20260425-231623.jsonl)
	name := filepath.Base(tp)
	// Strip .jsonl extension
	stem := strings.TrimSuffix(name, ".jsonl")
	fmt.Fprintf(os.Stderr, "\nTo resume this session: --resume %s\n", stem)
}

// drainOneShotNotifications drains sub-agent notifications in one-shot mode.
// It waits briefly for any pending notifications from background agents.
func drainOneShotNotifications(agent *AgentLoop) {
	// Brief wait to allow any in-flight notifications to arrive
	time.Sleep(100 * time.Millisecond)
	if notifications := agent.DrainNotifications(); len(notifications) > 0 {
		fmt.Println("\n--- Sub-agent notifications ---")
		for _, n := range notifications {
			fmt.Println(n)
		}
		fmt.Println("------------------------------")
		agent.InjectNotifications(notifications)
	}
}

// findTranscript resolves a transcript reference (number, filename, or 'last').
func findTranscript(target string) (string, error) {
	files, err := loadTranscriptList()
	if err != nil {
		return "", fmt.Errorf("no transcripts directory: %w", err)
	}
	if len(files) == 0 {
		return "", fmt.Errorf("no transcripts found")
	}

	dir := ".claude/transcripts"

	// "last" -> most recent
	if target == "last" {
		return filepath.Join(dir, files[0].name), nil
	}

	// Try as number (1-indexed)
	if num, err := strconv.Atoi(target); err == nil {
		if num > 0 && num <= len(files) {
			return filepath.Join(dir, files[num-1].name), nil
		}
		return "", fmt.Errorf("index %d out of range (1-%d)", num, len(files))
	}

	// Try as exact filename
	path := filepath.Join(dir, target)
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}

	// Try with .jsonl extension
	if !strings.HasSuffix(target, ".jsonl") {
		path = filepath.Join(dir, target+".jsonl")
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("transcript not found: %s", target)
}

// listTranscripts lists available transcript files.
func listTranscripts() {
	files, err := loadTranscriptList()
	if err != nil || len(files) == 0 {
		fmt.Println("No transcripts found.")
		return
	}

	fmt.Println("\nAvailable transcripts:")
	for i, f := range files {
		fmt.Printf("  %d. %s\n", i+1, f.name)
	}
	fmt.Println("\nUsage: /resume <number>, /resume <filename>, or /resume last")
}

// transcriptEntry holds a transcript file's name and modification time.
type transcriptEntry struct {
	name string
	mod  time.Time
}

// loadTranscriptList reads and sorts transcript files from the .claude/transcripts directory.
func loadTranscriptList() ([]transcriptEntry, error) {
	dir := ".claude/transcripts"
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []transcriptEntry
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".jsonl") {
			info, err := e.Info()
			if err != nil {
				continue
			}
			files = append(files, transcriptEntry{name: e.Name(), mod: info.ModTime()})
		}
	}
	sort.Slice(files, func(i, j int) bool {
		return files[j].mod.Before(files[i].mod)
	})
	return files, nil
}
