package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"miniclaudecode-go/tools"
)

func main() {
	model := flag.String("model", "", "Anthropic model to use")
	apiKey := flag.String("api-key", "", "API key (overrides ANTHROPIC_API_KEY/ANTHROPIC_AUTH_TOKEN env and config file)")
	baseURL := flag.String("base-url", "", "Custom API base URL (overrides config file)")
	mode := flag.String("mode", "ask", "Permission mode (ask|auto|bypass|plan)")
	maxTurns := flag.Int("max-turns", 0, "Max agent loop turns per message (0 = unlimited)")
	stream := flag.Bool("stream", false, "Enable streaming output")
	projectDir := flag.String("dir", "", "Project directory (change working directory before starting)")
	resumeFile := flag.String("resume", "", "Resume from a transcript file path or 'last' for most recent")
	flag.Parse()

	// Change working directory if --dir is specified
	// Normalize the path: filepath.FromSlash converts forward slashes to backslashes
	// on Windows (no-op on Unix), and filepath.Clean handles . and .. elements.
	// This ensures --dir values like "E:/workspace/project" work on Windows,
	// and guards against shell argument processing that may strip backslashes.
	if *projectDir != "" {
		normalizedDir := filepath.Clean(filepath.FromSlash(*projectDir))
		if err := os.Chdir(normalizedDir); err != nil {
			fmt.Fprintf(os.Stderr, "[!] Failed to change working directory to %s (original: %s): %v\n", normalizedDir, *projectDir, err)
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
	// Apply config-derived runtime settings (config file > env > defaults)
	if cfg.PreferNonStreaming {
		*stream = false
	}
	if cfg.EffortLevel == "fast" {
		if *model == "" {
			*model = getDefaultSonnetModel()
		}
	}

	// Apply config-derived default model settings (settings.json > env > hard-coded defaults)
	SetDefaultModels(&cfg)

	// When --model is specified but no individual default models are configured,
	// use --model for all defaults so alias resolution, fallback, and sub-agents
	// all use the same model
	if *model != "" {
		if cfg.DefaultOpusModel == "" {
			SetDefaultOpusModel(*model)
		}
		if cfg.DefaultSonnetModel == "" {
			SetDefaultSonnetModel(*model)
		}
		if cfg.DefaultHaikuModel == "" {
			SetDefaultHaikuModel(*model)
		}
	}

	// Wire session ID to streaming_executor package
	if cfg.SessionID != "" {
		packageSessionID = cfg.SessionID
	}

	// Wire git bash path to hooks + exec_tool packages
	if cfg.GitBashPath != "" {
		packageGitBashPath = cfg.GitBashPath
		tools.SetGitBashPath(cfg.GitBashPath)
	}
	// Resolve model alias (e.g. "sonnet" → "claude-sonnet-4-20250514")
	if cfg.Model != "" {
		if resolved, ok := ResolveModelAlias(cfg.Model); ok {
			cfg.Model = resolved
		}
	}

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

	// Initialize prompt history
	sessionID := generateSessionID()
	history := NewPromptHistory(sessionID)

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

	// Initialize cron scheduler (runs background ticker for scheduled prompts)
	agent.SetCronScheduler(createCronScheduler(getProjectDir(), nil))

	// Detect if stdin is a terminal (interactive) or piped.
	// This affects both one-shot mode (positional args) and pipe input mode
	// inside runInteractive.
	isInteractive := func() bool {
		fi, err := os.Stdin.Stat()
		if err != nil {
			return false
		}
		return fi.Mode()&os.ModeCharDevice != 0
	}()

	// Non-interactive (pipe or one-shot): default to auto permission mode if ask
	// was the default. Without this, the agent gets stuck waiting for user
	// confirmation on tool calls, but non-interactive mode has no user to respond.
	if !isInteractive && cfg.PermissionMode == ModeAsk && *mode == "ask" {
		cfg.PermissionMode = ModeAuto
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
	runInteractive(agent, history, sessionID, cfg)
}

func runInteractive(agent *AgentLoop, history *PromptHistory, sessionID string, cfg Config) {
	defer agent.Close()

	// On Windows, ensure console input mode has ENABLE_PROCESSED_INPUT and
	// ENABLE_LINE_INPUT. MCP child processes may have altereded the console mode.
	// This is a no-op on Unix (see main_unix.go).
	ensureConsoleInputMode()

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

	// Idle timeout: exit gracefully after inactivity.
	// Controlled via config file (runtime.exit_after_stop_delay) or CLAUDE_CODE_EXIT_AFTER_STOP_DELAY env var.
	// Accepts duration strings like "5m", "30s", or milliseconds as plain number.
	idleDelay := cfg.ExitAfterStopDelay
	idleTimerCh := make(chan struct{}, 1)
	startIdleTimer := func() {
		if idleDelay <= 0 {
			return
		}
		// Drain any previous timer signal before re-arming
		select {
		case <-idleTimerCh:
		default:
		}
		time.AfterFunc(idleDelay, func() {
			select {
			case idleTimerCh <- struct{}{}:
			default:
			}
		})
	}
	// Background goroutine: monitors idle timer and exits when it fires.
	// Since ReadString blocks the main thread, we can't use select in the
	// REPL loop. This goroutine watches the channel and calls os.Exit.
	go func() {
		if idleDelay <= 0 {
			return
		}
		<-idleTimerCh
		printResumeHint(agent)
		fmt.Fprintf(os.Stderr, "[idle] Exiting after %s of inactivity.\n", idleDelay)
		agent.Close()
		os.Exit(0)
	}()

	// Set up signal handler for Ctrl+C (SIGINT) and termination signals.
	// SIGTERM/SIGHUP are POSIX signals — they work on Unix but are not delivered
	// by Windows OS. We keep them for cross-platform compatibility.
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		for sig := range signalCh {
			if sig == syscall.SIGINT {
				agent.SetInterrupted(true)
				// Set SIGINT flag for Unix input loop to detect.
				// On Windows, this is a no-op (Ctrl+C unblocks ReadString directly).
				interruptStdin()
			} else {
				// SIGTERM/SIGHUP -- graceful shutdown
				printResumeHint(agent)
				fmt.Fprintf(os.Stderr, "\n[idle] Received %v, shutting down gracefully.\n", sig)
				agent.Close()
				exitCode := 128
				if sig == syscall.SIGTERM {
					exitCode = 143
				} else if sig == syscall.SIGHUP {
					exitCode = 129
				}
				os.Exit(exitCode)
			}
		}
	}()

	// Use a single bufio.Reader for input reading.
	stdinReader := bufio.NewReader(os.Stdin)

	// Piped input mode: read all stdin at once as a single prompt.
	// Without this, ReadString('\n') splits multi-line input into separate
	// agent.Run() calls, causing the first (often empty/incomplete) line
	// to be sent as a prompt with no task context.
	if !interactive {
		data, err := io.ReadAll(stdinReader)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[!] Failed to read piped input: %v\n", err)
			agent.Close()
			return
		}
		prompt := strings.TrimSpace(string(data))
		if prompt == "" {
			fmt.Fprintln(os.Stderr, "[!] Empty piped input.")
			agent.Close()
			return
		}
		// Record prompt to history
		if history != nil {
			history.Record(prompt, sessionID)
		}
		result := agent.Run(prompt)
		if !agent.IsStreaming() || !stdoutIsTerm {
			fmt.Println(result)
		}
		fmt.Println()

		// Drain any pending sub-agent notifications before exit
		drainOneShotNotifications(agent)

		printResumeHint(agent)
		agent.Close()
		return
	}

	var lastCtrlC time.Time

	for {
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

		// Start idle timeout after agent finishes (only when waiting at prompt)
		startIdleTimer()

		fmt.Print("\n> ")

		// Read input. On Windows, ReadString blocks and Ctrl+C
		// unblocks it directly. On Unix interactive mode, we use
		// select-based polling to detect SIGINT via sigintFlag.
		// Non-interactive (piped) mode uses simple ReadString.
		var line string
		var readErr error
		if !interactive {
			// Non-interactive (piped input): use simple ReadString
			line, readErr = stdinReader.ReadString('\n')
		} else if checkAndClearSigint() {
			// SIGINT was received before we started reading
			readErr = fmt.Errorf("interrupted")
		} else {
			line, readErr = readLineInterruptible(stdinReader)
		}
		if readErr != nil {
			// Read error or interrupt
			if interactive {
				// Check if this was a recent Ctrl+C
				now := time.Now()
				if !lastCtrlC.IsZero() && now.Sub(lastCtrlC) < 3*time.Second {
					// Double Ctrl+C within 3s -- exit
					printResumeHint(agent)
					agent.Close()
					os.Exit(0)
				}
				lastCtrlC = now
				agent.SetInterrupted(false)
				fmt.Fprintf(os.Stderr, "\n[WARN] Interrupting... (press Ctrl+C again within 3s to exit)\n")
				// Reopen stdin (platform-specific: CONIN$ on Windows, /dev/tty on Unix)
				if newReader := reopenStdin(); newReader != nil {
					stdinReader = newReader
				}
				continue
			}
			// Piped input ended — exit
			printResumeHint(agent)
			break
		}
		// Reset lastCtrlC on successful input
		lastCtrlC = time.Time{}

		userInput := strings.TrimSpace(line)
		if userInput == "" {
			if !interactive {
				break
			}
			continue
		}
		// Record prompt to history (P2-18: prompt history persistence)
		if history != nil {
			history.Record(userInput, sessionID)
		}

		// Check for exact command match -- only treat as command if the first
		// word is a known command. Unknown /xxx is passed through as prompt text.
		if strings.HasPrefix(userInput, "/") {
			parts := strings.Fields(userInput)
			cmd := strings.ToLower(parts[0])

			isKnownCmd := cmd == "/quit" || cmd == "/exit" || cmd == "/q" ||
				cmd == "/tools" || cmd == "/mode" || cmd == "/help" || cmd == "/resume" ||
				cmd == "/compact" || cmd == "/clear" || cmd == "/partialcompact" || cmd == "/agents" ||
				cmd == "/tasks" ||
				cmd == "/doctor" || cmd == "/history" || cmd == "/cleanup" || cmd == "/branch" ||
				cmd == "/status" || cmd == "/model"

			if !isKnownCmd {
				fmt.Fprintf(os.Stderr, "Unknown command: %s. Type /help for available commands.\n", cmd)
				continue
			} else {
				switch cmd {
				case "/quit", "/exit", "/q":
					fmt.Println("Goodbye!")
					return
				case "/tools":
					allTools := agent.registry.AllTools()
					builtinCount := 0
					mcpCount := 0
					for _, t := range allTools {
						if strings.HasPrefix(t.Name(), "mcp_") {
							mcpCount++
						} else {
							builtinCount++
						}
					}
					fmt.Printf("\nTools (%d total: %d built-in, %d MCP):\n", len(allTools), builtinCount, mcpCount)

					// Categorize built-in tools by name prefix
					categories := map[string][]int{}
					var mcpIndices []int
					for i, t := range allTools {
						if strings.HasPrefix(t.Name(), "mcp_") {
							mcpIndices = append(mcpIndices, i)
							continue
						}
						cat := categorizeTool(t.Name())
						categories[cat] = append(categories[cat], i)
					}

					// Print categories in a consistent order
					for _, cat := range []string{"file", "search", "exec", "git", "agent", "code", "system", "other"} {
						if indices, ok := categories[cat]; ok && len(indices) > 0 {
							fmt.Printf("\n  [%s]:\n", cat)
							for _, idx := range indices {
								t := allTools[idx]
								fmt.Printf("    - %s: %s\n", t.Name(), t.Description())
							}
						}
					}
					if len(mcpIndices) > 0 {
						fmt.Printf("\n  [mcp]:\n")
						for _, idx := range mcpIndices {
							t := allTools[idx]
							fmt.Printf("    - %s: %s\n", t.Name(), t.Description())
						}
					}
					continue
				case "/mode":
					if len(parts) > 1 {
						modeVal := strings.ToLower(parts[1])
						switch modeVal {
						case "ask", "auto", "bypass", "plan":
							agent.config.PermissionMode = PermissionMode(modeVal)
							fmt.Printf("Mode changed to: %s\n", modeVal)
						default:
							fmt.Printf("Unknown mode: %s\n", parts[1])
						}
					} else {
						fmt.Printf("Current mode: %s\n", agent.config.PermissionMode)
						fmt.Println("Usage: /mode [ask|auto|bypass|plan]")
					}
					continue
				case "/help":
					if len(parts) > 1 {
						// Show detailed help for a specific command
						showDetailedHelp(parts[1])
					} else {
						fmt.Println("Commands:")
						fmt.Println("  Session:")
						fmt.Println("    /compact        -- Force context compaction")
						fmt.Println("    /partialcompact -- Directional partial compaction (up_to|from, [pivot])")
						fmt.Println("    /clear          -- Clear conversation history")
						fmt.Println("    /status         -- Show session status (tokens, cache, cost)")
						fmt.Println("    /model          -- View/switch model")
						fmt.Println("    /resume         -- Resume a previous session")
						fmt.Println("    /branch         -- Create a conversation branch")
						fmt.Println("  Development:")
						fmt.Println("    /tools          -- List available tools (by category)")
						fmt.Println("    /agents         -- Manage background agents")
						fmt.Println("    /tasks          -- View background tasks")
						fmt.Println("    /history        -- Show recent prompts")
						fmt.Println("  Configuration:")
						fmt.Println("    /mode           -- Switch permission mode (ask|auto|bypass|plan)")
						fmt.Println("  Operations:")
						fmt.Println("    /doctor         -- Run installation diagnostics")
						fmt.Println("    /cleanup        -- Remove stale session files")
						fmt.Println("  Other:")
						fmt.Println("    /help [cmd]     -- Show this help (or detailed help for cmd)")
						fmt.Println("    /quit           -- Exit")
					}
					continue
				case "/compact":
					agent.ForceCompact()
					continue
				case "/partialcompact":
					if len(parts) < 2 {
						fmt.Printf("Total messages: %d\n", agent.context.Len())
						fmt.Println("Usage: /partialcompact <up_to|from> [pivot_index]")
						fmt.Println("  up_to  — summarize messages before pivot, keep pivot onwards")
						fmt.Println("  from   — keep messages before pivot, summarize pivot onwards")
						fmt.Println("  pivot_index defaults to midpoint if omitted")
						continue
					}
					dir := strings.ToLower(parts[1])
					pivot := 0
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
					handleResume(&agent, parts[1:])
					continue
				case "/agents":
					handleAgentsCommand(agent, parts[1:])
					continue
				case "/tasks":
					handleTasksCommand(agent, parts[1:])
					continue
				case "/doctor":
					runDoctor(agent)
					continue
				case "/history":
					handleHistory(history, parts[1:])
					continue
				case "/cleanup":
					if wd, err := os.Getwd(); err == nil {
						handleCleanup(wd, parts[1:])
					}
					continue
				case "/branch":
					newAgent, err := handleBranch(agent, parts[1:])
					if err != nil {
						fmt.Printf("Branch error: %v\n", err)
					}
					if newAgent != agent {
						agent = newAgent
					}
					continue
				case "/status":
					handleStatus(agent)
					continue
				case "/model":
					handleModelCommand(agent, parts[1:])
					continue
				}
			}
		}
		fmt.Println()
		agent.SetInterrupted(false) // ensure clear before running
		result := agent.Run(userInput)
		agent.SetInterrupted(false) // clear after run

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

// handleResume handles /resume command.
// /resume list     - List recent transcripts (max 16)
// /resume <id>     - Resume a transcript by number, filename, or 'last'
func handleResume(agent **AgentLoop, args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: /resume list | /resume <number|filename|last>")
		return
	}

	if strings.ToLower(args[0]) == "list" {
		listTranscripts()
		return
	}

	// /resume <id>
	target := args[0]
	path, err := findTranscript(target)
	if err != nil {
		fmt.Printf("[resume] Error: %v\n", err)
		return
	}
	newAgent, err := NewAgentLoopFromTranscript((*agent).config, (*agent).registry, (*agent).useStream, path, true)
	if err != nil {
		fmt.Printf("[resume] Error resuming transcript: %v\n", err)
		return
	}
	(*agent).Close()
	*agent = newAgent
	fmt.Printf("[resume] Resumed transcript: %s\n", path)
}
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

	dir := ".claude/transcripts"
	now := time.Now()

	fmt.Println("\nAvailable transcripts (max 16):")
	limit := 16
	if len(files) > limit {
		files = files[:limit]
	}
	for i, f := range files {
		path := filepath.Join(dir, f.name)
		info, err := os.Stat(path)
		sizeStr := "?"
		if err == nil {
			sizeStr = formatFileSize(int(info.Size()))
		}
		ageStr := formatAge(f.mod, now)
		fmt.Printf("  %d. %s  (%s, %s)\n", i+1, f.name, ageStr, sizeStr)
	}
	fmt.Println("\nUsage: /resume <number>, /resume <filename>, or /resume last")
}

// formatAge returns a human-readable age string.
func formatAge(mod, now time.Time) string {
	diff := now.Sub(mod)
	switch {
	case diff < time.Minute:
		return "just now"
	case diff < time.Hour:
		return fmt.Sprintf("%dm ago", int(diff.Minutes()))
	case diff < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(diff.Hours()))
	case diff < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(diff.Hours()/24))
	default:
		return mod.Format("2006-01-02")
	}
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

// handleAgentsCommand handles the /agents slash command.
// Supports:
//
//	/agents          - List all agents
//	/agents list     - Same as above
//	/agents show <id> - Show details of a specific agent
//	/agents stop <id> - Kill a running agent
//	/agents help     - Show usage
func handleAgentsCommand(agent *AgentLoop, args []string) {
	if agent.agentTaskStore == nil {
		fmt.Println("No agent task store available.")
		return
	}

	if len(args) == 0 {
		fmt.Println("Usage: /agents [list|show <id>|stop <id>|help]")
		return
	}

	subcmd := strings.ToLower(args[0])

	switch subcmd {
	case "list", "ls", "":
		tasks := agent.agentTaskStore.List()
		if len(tasks) == 0 {
			fmt.Println("No agents found.")
			return
		}
		fmt.Println()
		fmt.Println("Background Agents:")
		fmt.Printf("  %-10s %-12s %-30s %-15s %s\n", "ID", "Status", "Description", "Model", "Started")
		fmt.Println("  " + strings.Repeat("-", 80))
		for _, t := range tasks {
			desc := t.Description
			if len(desc) > 28 {
				desc = desc[:25] + "..."
			}
			model := t.Model
			if model == "" {
				model = "-"
			}
			fmt.Printf("  %-10s %-12s %-30s %-15s %s\n",
				t.ID, t.Status, desc, model,
				t.StartTime.Format("15:04:05"))
		}
		fmt.Printf("\n  %d agent(s) total\n", len(tasks))

	case "show":
		if len(args) < 2 {
			fmt.Println("Usage: /agents show <id>")
			return
		}
		taskID := args[1]
		task := agent.agentTaskStore.Get(taskID)
		if task == nil {
			fmt.Printf("Agent %s not found.\n", taskID)
			return
		}
		fmt.Printf("\nAgent: %s\n", task.ID)
		fmt.Printf("  Status:        %s\n", task.Status)
		fmt.Printf("  Type:          %s\n", task.Type)
		fmt.Printf("  Description:   %s\n", task.Description)
		fmt.Printf("  SubagentType:  %s\n", task.SubagentType)
		fmt.Printf("  Model:         %s\n", task.Model)
		fmt.Printf("  Prompt:        %s\n", truncateString(task.Prompt, 100))
		fmt.Printf("  Started:       %s\n", task.StartTime.Format(time.RFC3339))
		if !task.EndTime.IsZero() {
			fmt.Printf("  Ended:         %s\n", task.EndTime.Format(time.RFC3339))
			fmt.Printf("  Duration:      %s\n", task.EndTime.Sub(task.StartTime).Round(time.Second))
		}
		if task.ToolsUsed > 0 {
			fmt.Printf("  Tools Used:    %d\n", task.ToolsUsed)
		}
		if task.DurationMs > 0 {
			fmt.Printf("  Duration (ms): %d\n", task.DurationMs)
		}
		if task.TranscriptPath != "" {
			fmt.Printf("  Transcript:    %s\n", task.TranscriptPath)
		}
		// Show output
		output := task.GetOutput()
		if output != "" {
			lines := strings.Split(output, "\n")
			maxLines := 50
			if len(lines) > maxLines {
				skipped := len(lines) - maxLines
				fmt.Printf("\n  Output (last %d of %d lines):\n", maxLines, len(lines))
				fmt.Printf("  ... (%d earlier lines omitted) ...\n", skipped)
				lines = lines[len(lines)-maxLines:]
			} else {
				fmt.Printf("\n  Output (%d lines):\n", len(lines))
			}
			for _, line := range lines {
				fmt.Println("  " + line)
			}
		} else {
			fmt.Println("\n  Output: (none yet)")
		}

	case "stop":
		if len(args) < 2 {
			fmt.Println("Usage: /agents stop <id>")
			return
		}
		taskID := args[1]
		task := agent.agentTaskStore.Get(taskID)
		if task == nil {
			fmt.Printf("Agent %s not found.\n", taskID)
			return
		}
		if task.IsTerminal() {
			fmt.Printf("Agent %s is not running (status: %s)\n", taskID, task.Status)
			return
		}
		if agent.agentTaskStore.Kill(taskID) {
			fmt.Printf("Agent %s has been killed.\n", taskID)
		} else {
			fmt.Printf("Failed to kill agent %s.\n", taskID)
		}

	case "help":
		fmt.Println("Agents commands:")
		fmt.Println("  /agents           -- List all background agents")
		fmt.Println("  /agents show <id> -- Show details and output of an agent")
		fmt.Println("  /agents stop <id> -- Kill a running agent")
		fmt.Println("  /agents help      -- Show this help")

	default:
		fmt.Printf("Unknown /agents subcommand: %s\n", subcmd)
		fmt.Println("Use /agents help for usage information.")
	}
}

// handleTasksCommand handles the /tasks slash command.
// Supports:
//
//	/tasks          - List all background tasks
//	/tasks list     - Same as above
//	/tasks show <id> - Show details of a specific task
//	/tasks stop <id> - Kill a running task
//	/tasks message <id> <msg> - Send a message to a running task
//	/tasks help     - Show usage
func handleTasksCommand(agent *AgentLoop, args []string) {
	if agent.agentTaskStore == nil {
		fmt.Println("No task store available.")
		return
	}

	if len(args) == 0 {
		fmt.Println("Usage: /tasks [list|show <id>|stop <id>|message <id> <msg>|help]")
		return
	}

	subcmd := strings.ToLower(args[0])

	switch subcmd {
	case "list", "ls", "":
		tasks := agent.agentTaskStore.List()
		if len(tasks) == 0 {
			fmt.Println("No tasks found.")
			return
		}
		fmt.Println()
		fmt.Println("Background Tasks:")
		fmt.Printf("  %-10s %-12s %-30s %-15s %s\n", "ID", "Status", "Description", "Model", "Started")
		fmt.Println("  " + strings.Repeat("-", 80))
		for _, t := range tasks {
			desc := t.Description
			if len(desc) > 28 {
				desc = desc[:25] + "..."
			}
			model := t.Model
			if model == "" {
				model = "-"
			}
			fmt.Printf("  %-10s %-12s %-30s %-15s %s\n",
				t.ID, t.Status, desc, model,
				t.StartTime.Format("15:04:05"))
		}
		fmt.Printf("\n  %d task(s) total\n", len(tasks))

	case "show":
		if len(args) < 2 {
			fmt.Println("Usage: /tasks show <id>")
			return
		}
		taskID := args[1]
		task := agent.agentTaskStore.Get(taskID)
		if task == nil {
			fmt.Printf("Task %s not found.\n", taskID)
			return
		}
		fmt.Printf("\nTask: %s\n", task.ID)
		fmt.Printf("  Status:        %s\n", task.Status)
		fmt.Printf("  Type:          %s\n", task.Type)
		fmt.Printf("  Description:   %s\n", task.Description)
		fmt.Printf("  SubagentType:  %s\n", task.SubagentType)
		fmt.Printf("  Model:         %s\n", task.Model)
		fmt.Printf("  Prompt:        %s\n", truncateString(task.Prompt, 100))
		fmt.Printf("  Started:       %s\n", task.StartTime.Format(time.RFC3339))
		if !task.EndTime.IsZero() {
			fmt.Printf("  Ended:         %s\n", task.EndTime.Format(time.RFC3339))
			fmt.Printf("  Duration:      %s\n", task.EndTime.Sub(task.StartTime).Round(time.Second))
		}
		if task.ToolsUsed > 0 {
			fmt.Printf("  Tools Used:    %d\n", task.ToolsUsed)
		}
		if task.DurationMs > 0 {
			fmt.Printf("  Duration (ms): %d\n", task.DurationMs)
		}
		if task.TranscriptPath != "" {
			fmt.Printf("  Transcript:    %s\n", task.TranscriptPath)
		}
		// Show tool stats
		readCount, searchCount, bashCount, editCount, toolsUsed, tokenCount := task.GetToolStats()
		if toolsUsed > 0 {
			fmt.Printf("  Tool Stats:    read:%d search:%d bash:%d edit:%d total:%d\n",
				readCount, searchCount, bashCount, editCount, toolsUsed)
		}
		if tokenCount > 0 {
			fmt.Printf("  Tokens:        %d\n", tokenCount)
		}
		// Show output
		output := task.GetOutput()
		if output != "" {
			lines := strings.Split(output, "\n")
			maxLines := 50
			if len(lines) > maxLines {
				skipped := len(lines) - maxLines
				fmt.Printf("\n  Output (last %d of %d lines):\n", maxLines, len(lines))
				fmt.Printf("  ... (%d earlier lines omitted) ...\n", skipped)
				lines = lines[len(lines)-maxLines:]
			} else {
				fmt.Printf("\n  Output (%d lines):\n", len(lines))
			}
			for _, line := range lines {
				fmt.Println("  " + line)
			}
		} else {
			fmt.Println("\n  Output: (none yet)")
		}

	case "stop":
		if len(args) < 2 {
			fmt.Println("Usage: /tasks stop <id>")
			return
		}
		taskID := args[1]
		task := agent.agentTaskStore.Get(taskID)
		if task == nil {
			fmt.Printf("Task %s not found.\n", taskID)
			return
		}
		if task.IsTerminal() {
			fmt.Printf("Task %s is not running (status: %s)\n", taskID, task.Status)
			return
		}
		if agent.agentTaskStore.Kill(taskID) {
			fmt.Printf("Task %s has been killed.\n", taskID)
		} else {
			fmt.Printf("Failed to kill task %s.\n", taskID)
		}

	case "message":
		if len(args) < 3 {
			fmt.Println("Usage: /tasks message <id> <msg>")
			return
		}
		taskID := args[1]
		msg := strings.Join(args[2:], " ")
		if agent.agentTaskStore.AddPendingMessage(taskID, msg) {
			fmt.Printf("Message sent to task %s.\n", taskID)
		} else {
			fmt.Printf("Task %s not found or not running.\n", taskID)
		}

	case "help":
		fmt.Println("Tasks commands:")
		fmt.Println("  /tasks              -- List all background tasks")
		fmt.Println("  /tasks show <id>    -- Show details and output of a task")
		fmt.Println("  /tasks stop <id>    -- Kill a running task")
		fmt.Println("  /tasks message <id> <msg> -- Send a message to a running task")
		fmt.Println("  /tasks help         -- Show this help")

	default:
		fmt.Printf("Unknown /tasks subcommand: %s\n", subcmd)
		fmt.Println("Use /tasks help for usage information.")
	}
}

// truncateString truncates a string to maxLen characters, adding "..." if truncated.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// runDoctor runs installation diagnostics.
func runDoctor(agent *AgentLoop) {
	fmt.Println("\n=== Doctor Diagnostics ===")

	// Version
	fmt.Printf("Version: %s\n", "miniClaudeCode (Go)")

	// Model
	fmt.Printf("Model: %s\n", agent.config.Model)

	// API key configured
	if agent.config.APIKey != "" {
		masked := agent.config.APIKey[:4] + "..." + agent.config.APIKey[len(agent.config.APIKey)-4:]
		fmt.Printf("API Key: configured (%s)\n", masked)
	} else {
		fmt.Println("API Key: NOT configured")
	}

	// Base URL
	baseURL := agent.config.BaseURL
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	if baseURL != "https://api.anthropic.com" {
		fmt.Printf("Base URL: %s (custom)\n", baseURL)
	} else {
		fmt.Println("Base URL: default (api.anthropic.com)")
	}

	// API connectivity test
	if agent.config.APIKey != "" {
		fmt.Print("API Connectivity: ")
		startTest := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		reqURL := baseURL + "/v1/messages"
		// Use the current configured model for the connectivity test
		testModel := agent.config.Model
		if testModel == "" {
			testModel = getDefaultHaikuModel()
		}
		body := fmt.Sprintf(`{"model":"%s","max_tokens":1,"messages":[{"role":"user","content":"hi"}]}`, testModel)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, strings.NewReader(body))
		if err != nil {
			fmt.Printf("request error: %v\n", err)
		} else {
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("x-api-key", agent.config.APIKey)
			req.Header.Set("anthropic-version", "2023-06-01")
			client := &http.Client{Timeout: 10 * time.Second}
			resp, err := client.Do(req)
			latency := time.Since(startTest)
			if err != nil {
				fmt.Printf("connection error: %v (%s)\n", err, latency.Round(time.Millisecond))
			} else {
				body, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				if resp.StatusCode == 200 || resp.StatusCode == 400 {
					// 200 = success, 400 = auth/API ok but bad request
					fmt.Printf("OK (latency %s, HTTP %d)\n", latency.Round(time.Millisecond), resp.StatusCode)
				} else if resp.StatusCode == 503 {
					// 503 often means the proxy doesn't support the requested model
					errMsg := string(body)
					if len(errMsg) > 80 {
						errMsg = errMsg[:80] + "..."
					}
					fmt.Printf("HTTP %d (%s) — %s\n", resp.StatusCode, latency.Round(time.Millisecond), errMsg)
					fmt.Printf("  Hint: model %q may not be available on this proxy. Try /model sonnet or a full model ID.\n", testModel)
				} else {
					errMsg := string(body)
					if len(errMsg) > 80 {
						errMsg = errMsg[:80] + "..."
					}
					fmt.Printf("HTTP %d (%s) — %s\n", resp.StatusCode, latency.Round(time.Millisecond), errMsg)
				}
			}
		}
	}

	// Permission mode
	fmt.Printf("Permission Mode: %s\n", agent.config.PermissionMode)

	// Ripgrep check
	if _, err := exec.LookPath("rg"); err == nil {
		fmt.Println("Ripgrep: found")
	} else {
		fmt.Println("Ripgrep: NOT found (grep fallback will be used)")
	}

	// Python check
	if _, err := exec.LookPath("python3"); err == nil {
		fmt.Println("Python: found (python3)")
	} else if _, err := exec.LookPath("python"); err == nil {
		fmt.Println("Python: found (python)")
	} else {
		fmt.Println("Python: NOT found")
	}

	// Node.js check
	if _, err := exec.LookPath("node"); err == nil {
		fmt.Println("Node.js: found")
	} else {
		fmt.Println("Node.js: NOT found")
	}

	// Git check
	if _, err := exec.LookPath("git"); err == nil {
		fmt.Println("Git: found")
	} else {
		fmt.Println("Git: NOT found")
	}

	// Shell config detection
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = os.Getenv("COMSPEC")
	}
	if shell != "" {
		fmt.Printf("Shell: %s\n", shell)
	}

	// MCP servers
	if agent.config.MCPManager != nil {
		servers := agent.config.MCPManager.ListServers()
		if len(servers) > 0 {
			fmt.Printf("MCP Servers: %d registered\n", len(servers))
		} else {
			fmt.Println("MCP Servers: none")
		}
	}

	// Skills
	if agent.config.SkillLoader != nil {
		skills := agent.config.SkillLoader.ListSkills(false)
		if len(skills) > 0 {
			fmt.Printf("Skills: %d loaded\n", len(skills))
		} else {
			fmt.Println("Skills: none")
		}
	}

	// Transcripts
	transcriptCount := 0
	if entries, err := loadTranscriptList(); err == nil {
		transcriptCount = len(entries)
	}
	fmt.Printf("Transcripts: %d\n", transcriptCount)

	// Working directory
	if wd, err := os.Getwd(); err == nil {
		fmt.Printf("Working Dir: %s\n", wd)
	}

	// CLAUDE.md files
	for _, f := range []string{"CLAUDE.md", ".claude/CLAUDE.md", "CLAUDE.local.md"} {
		if _, err := os.Stat(f); err == nil {
			fmt.Printf("Config File: %s (exists)\n", f)
		}
	}

	fmt.Println("==========================")
}

// generateSessionID creates a unique session identifier based on timestamp.
func generateSessionID() string {
	return time.Now().Format("20060102-150405")
}

// handleHistory handles the /history slash command.
// Supports:
//
//	/history          - Show last 20 prompts
//	/history N        - Show last N prompts
//	/history clear    - Clear history file
func handleHistory(history *PromptHistory, args []string) {
	if history == nil {
		fmt.Println("History not available.")
		return
	}

	if len(args) == 0 {
		fmt.Println("Usage: /history [N|clear]")
		return
	}

	if strings.ToLower(args[0]) == "clear" {
		if err := os.Remove(history.filePath); err != nil && !os.IsNotExist(err) {
			fmt.Printf("Error clearing history: %v\n", err)
		} else {
			fmt.Println("History cleared.")
		}
		return
	}

	n := 20
	if parsed, err := strconv.Atoi(args[0]); err == nil && parsed > 0 {
		n = parsed
	}

	entries := history.LoadRecent(n)
	if len(entries) == 0 {
		fmt.Println("No history found.")
		return
	}

	fmt.Printf("\nRecent prompts (%d):\n", len(entries))
	for i, e := range entries {
		text := e.Text
		if len(text) > 80 {
			text = text[:77] + "..."
		}
		fmt.Printf("  %d. [%s] %s\n", i+1, e.Timestamp[11:16], text)
	}
}

// handleBranch handles the /branch slash command.
// Supports:
//
//	/branch             - Create a new branch from current transcript
//	/branch list        - List all branches with timestamps
//	/branch switch <name> - Switch to a branch (resume from that transcript)
func handleBranch(agent *AgentLoop, args []string) (*AgentLoop, error) {
	transcriptDir := filepath.Join(".claude", "branches")
	os.MkdirAll(transcriptDir, 0o755)

	if len(args) == 0 {
		fmt.Println("Usage: /branch [list|switch <name>]")
		return agent, nil
	}

	subcmd := strings.ToLower(args[0])
	switch subcmd {
	case "list", "ls":
		entries, err := os.ReadDir(transcriptDir)
		if err != nil || len(entries) == 0 {
			fmt.Println("No branches found.")
			return agent, nil
		}
		fmt.Println("\nBranches:")
		now := time.Now()
		for i, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".jsonl") {
				info, err := e.Info()
				if err != nil {
					continue
				}
				name := strings.TrimSuffix(e.Name(), ".jsonl")
				fmt.Printf("  %d. %s  (%s, %s)\n",
					i+1, name, formatAge(info.ModTime(), now), formatFileSize(int(info.Size())))
			}
		}
		return agent, nil

	case "switch", "sw", "checkout":
		if len(args) < 2 {
			fmt.Println("Usage: /branch switch <name>")
			return agent, nil
		}
		branchName := args[1]
		if !strings.HasSuffix(branchName, ".jsonl") {
			branchName += ".jsonl"
		}
		branchFile := filepath.Join(transcriptDir, branchName)
		if _, err := os.Stat(branchFile); os.IsNotExist(err) {
			// Try without .jsonl
			branchName2 := strings.TrimSuffix(branchName, ".jsonl")
			branchFile2 := filepath.Join(transcriptDir, branchName2+".jsonl")
			if _, err := os.Stat(branchFile2); os.IsNotExist(err) {
				return agent, fmt.Errorf("branch not found: %s", args[1])
			}
			branchFile = branchFile2
		}
		// Resume from the branch transcript
		newAgent, err := NewAgentLoopFromTranscript(agent.config, agent.registry, agent.useStream, branchFile, true)
		if err != nil {
			return agent, fmt.Errorf("failed to switch branch: %w", err)
		}
		agent.Close()
		fmt.Printf("Switched to branch: %s\n", strings.TrimSuffix(filepath.Base(branchFile), ".jsonl"))
		return newAgent, nil

	default:
		fmt.Printf("Unknown /branch subcommand: %s\n", subcmd)
		fmt.Println("Usage: /branch [list|switch <name>]")
		return agent, nil
	}
}

// categorizeTool assigns a built-in tool to a human-readable category based on its name.
func categorizeTool(name string) string {
	switch {
	case strings.HasPrefix(name, "read") || strings.HasPrefix(name, "write") ||
		strings.HasPrefix(name, "edit") || strings.HasPrefix(name, "list") ||
		strings.HasPrefix(name, "file_"):
		return "file"
	case name == "grep" || name == "glob" || strings.HasPrefix(name, "search"):
		return "search"
	case name == "exec" || name == "bash" || name == "shell" || name == "run" ||
		strings.HasPrefix(name, "kill_") || strings.HasPrefix(name, "process"):
		return "exec"
	case strings.HasPrefix(name, "git_") || name == "git":
		return "git"
	case strings.HasPrefix(name, "agent_") || strings.HasPrefix(name, "sub_"):
		return "agent"
	case strings.HasPrefix(name, "lisp_") || strings.HasPrefix(name, "format"):
		return "code"
	case name == "compact" || name == "clear" || strings.HasPrefix(name, "system"):
		return "system"
	default:
		return "other"
	}
}

// showDetailedHelp shows detailed help for a specific command.
func showDetailedHelp(cmd string) {
	cmd = strings.ToLower(cmd)
	if !strings.HasPrefix(cmd, "/") {
		cmd = "/" + cmd
	}

	details := map[string]string{
		"/compact":        "Force context compaction. Summarizes conversation history to reduce token usage.\nUsage: /compact",
		"/partialcompact": "Directional partial compaction. Keep recent or old messages and compact the rest.\nUsage: /partialcompact <up_to|from> [pivot_index]",
		"/clear":          "Clear all conversation history. Files read cache is also cleared.\nUsage: /clear",
		"/status":         "Show comprehensive session status: current model, permission mode,\nmessage count, token usage (input/output/cache), cache hit rate, cost tracking, and turn count.\nUsage: /status",
		"/model":          "View and switch models.\nUsage: /model [list|sonnet|opus|haiku|<full_model_id>]",
		"/resume":         "Resume a previous session from a transcript.\nUsage: /resume [list|<number>|<filename>|last]",
		"/branch":         "Create a conversation branch at the current point.\nUsage: /branch [list|switch <name>]",
		"/tools":          "List all available tools, grouped by category (file, search, exec, git, agent, code, MCP).\nUsage: /tools",
		"/agents":         "Manage background agents.\nUsage: /agents [list|show <id>|stop <id>|help]",
		"/tasks":          "View background tasks.\nUsage: /tasks [list|show <id>|stop <id>|message <id> <msg>|help]",
		"/history":        "Show recent prompts from the session history.\nUsage: /history [N|clear]",
		"/mode":           "Switch permission mode.\nUsage: /mode [ask|auto|bypass|plan]",
		"/doctor":         "Run installation diagnostics: API key, base URL, tools (rg/python/node/git),\nMCP servers, skills, transcripts, CLAUDE.md files.\nUsage: /doctor",
		"/cleanup":        "Remove stale session files older than cutoff days.\nUsage: /cleanup [days]",
		"/quit":           "Exit the interactive session.\nAliases: /exit, /q",
		"/help":           "Show this help. Pass a command name for detailed help.\nUsage: /help [command]",
	}

	if detail, ok := details[cmd]; ok {
		fmt.Println(detail)
	} else {
		fmt.Printf("No detailed help available for: %s\n", cmd)
		fmt.Println("Try /help without arguments for the full command list.")
	}
}
