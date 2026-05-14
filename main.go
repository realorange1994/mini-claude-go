package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

func main() {
	model := flag.String("model", "", "Anthropic model to use")
	apiKey := flag.String("api-key", "", "API key (overrides ANTHROPIC_API_KEY/ANTHROPIC_AUTH_TOKEN env and config file)")
	baseURL := flag.String("base-url", "", "Custom API base URL (overrides config file)")
	mode := flag.String("mode", "ask", "Permission mode (ask|auto|bypass|plan)")
	maxTurns := flag.Int("max-turns", 90, "Max agent loop turns per message")
	stream := flag.Bool("stream", false, "Enable streaming output")
	projectDir := flag.String("dir", "", "Project directory (change working directory before starting)")
	resumeFile := flag.String("resume", "", "Resume from a transcript file path or 'last' for most recent")
	flag.Parse()

	// Check CLAUDE_CODE_PREFER_NON_STREAMING env var (overrides --stream flag)
	if v := os.Getenv("CLAUDE_CODE_PREFER_NON_STREAMING"); v != "" && v != "0" && v != "false" {
		*stream = false
	}

	// Parse CLAUDE_CODE_EFFORT_LEVEL and apply model/budget overrides
	if v := os.Getenv("CLAUDE_CODE_EFFORT_LEVEL"); v == "fast" {
		// Fast mode: override to a lighter model if not explicitly set
		if *model == "" {
			*model = "claude-sonnet-4-20250514"
		}
	}

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
	runInteractive(agent, history, sessionID)
}

// ensureConsoleInputMode ensures the Windows console input mode has the
// required flags for interactive input. MCP child processes (node.exe) may
// alter the console input mode when they start, breaking ReadString.
func ensureConsoleInputMode() {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getStdHandle := kernel32.NewProc("GetStdHandle")
	getConsoleMode := kernel32.NewProc("GetConsoleMode")
	setConsoleMode := kernel32.NewProc("SetConsoleMode")

	const STD_INPUT_HANDLE = ^uintptr(10) // -10
	const (
		ENABLE_PROCESSED_INPUT    = 0x0001
		ENABLE_LINE_INPUT         = 0x0002
		ENABLE_ECHO_INPUT         = 0x0004
		ENABLE_EXTENDED_FLAGS     = 0x0080
		ENABLE_VIRTUAL_TERMINAL_INPUT = 0x0200
	)

	h, _, _ := getStdHandle.Call(uintptr(STD_INPUT_HANDLE))
	if h == 0 {
		return
	}

	var mode uint32
	ret, _, _ := getConsoleMode.Call(h, uintptr(unsafe.Pointer(&mode)))
	if ret == 0 {
		return
	}

	// Ensure the critical flags are set
	needMode := mode
	needMode |= ENABLE_PROCESSED_INPUT
	needMode |= ENABLE_LINE_INPUT
	needMode |= ENABLE_ECHO_INPUT
	needMode &^= ENABLE_VIRTUAL_TERMINAL_INPUT
	needMode |= ENABLE_EXTENDED_FLAGS

	if needMode != mode {
		setConsoleMode.Call(h, uintptr(needMode))
	}
}

func runInteractive(agent *AgentLoop, history *PromptHistory, sessionID string) {
	defer agent.Close()

	// On Windows, ensure console input mode has ENABLE_PROCESSED_INPUT and
	// ENABLE_LINE_INPUT. MCP child processes may have altereded the console mode.
	if runtime.GOOS == "windows" {
		ensureConsoleInputMode()
	}

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
	// Controlled by CLAUDE_CODE_EXIT_AFTER_STOP_DELAY env var.
	// Accepts duration strings like "5m", "30s", or milliseconds as plain number.
	idleDelay := parseIdleTimeoutEnv()
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

	// Use a single bufio.Reader for the entire REPL lifetime.
	stdinReader := bufio.NewReader(os.Stdin)

	reopenStdin := func() *bufio.Reader {
		var f *os.File
		var err error
		f, err = os.OpenFile("CONIN$", os.O_RDWR, 0)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[!] Failed to reopen stdin (CONIN$): %v\n", err)
			return nil
		}
		return bufio.NewReader(f)
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

		// Read input synchronously — no goroutine, no channel.
		// On Windows console, ReadString blocks until Enter is pressed or
		// Ctrl+C interrupts the blocking call (returning an error).
		// This matches the old working version's approach.
		line, err := stdinReader.ReadString('\n')
		if err != nil {
			// Read error — on Windows Ctrl+C, stdin is closed and we get EOF.
			if interactive {
				// Check if this was a recent Ctrl+C
				now := time.Now()
				if !lastCtrlC.IsZero() && now.Sub(lastCtrlC) < 2*time.Second {
					// Double Ctrl+C within 2s -- exit
					printResumeHint(agent)
					agent.Close()
					os.Exit(0)
				}
				lastCtrlC = now
				agent.SetInterrupted(false)
				fmt.Fprintf(os.Stderr, "\n[WARN] Interrupting... (press Ctrl+C again within 2s to exit)\n")
				// Reopen stdin via CONIN$ (Windows console input device)
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
				cmd == "/doctor" || cmd == "/history" || cmd == "/cleanup" || cmd == "/branch" || cmd == "/daemon" || cmd == "/errors" || cmd == "/feature" || cmd == "/settings" || cmd == "/telemetry"

			if !isKnownCmd {
				// Not a recognized command -- treat as normal prompt
			} else {
				switch cmd {
				case "/quit", "/exit", "/q":
					fmt.Println("Goodbye!")
					return
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
					fmt.Println("Commands:")
					fmt.Println("  /help           -- Show available commands")
					fmt.Println("  /compact        -- Force context compaction")
					fmt.Println("  /partialcompact -- Directional partial compaction (up_to|from, [pivot])")
					fmt.Println("  /clear          -- Clear conversation history")
					fmt.Println("  /mode           -- Switch permission mode (ask|auto|bypass|plan)")
					fmt.Println("  /resume         -- Resume a previous session")
					fmt.Println("  /tools          -- List available tools")
					fmt.Println("  /quit           -- Exit")
					fmt.Println("  /agents         -- Manage background agents")
					fmt.Println("  /doctor         -- Run installation diagnostics")
					fmt.Println("  /history        -- Show recent prompts")
					fmt.Println("  /cleanup        -- Remove stale session files")
					fmt.Println("  /branch         -- Create a conversation branch")
					fmt.Println("  /daemon         -- Manage daemon mode (start/stop/status/submit)")
					fmt.Println("  /errors         -- View error logs (recent/clear)")
					fmt.Println("  /feature        -- Manage feature flags (list/enable/disable)")
					fmt.Println("  /settings       -- View settings hierarchy (sources/get/set)")
					fmt.Println("  /telemetry      -- View telemetry events (recent/enable/disable/clear)")
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
				case "/agents":
					handleAgentsCommand(agent, parts[1:])
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
					if err := handleBranch(agent); err != nil {
						fmt.Printf("Branch error: %v\n", err)
					}
					continue
				case "/daemon":
					handleDaemon(parts[1:])
					continue
				case "/errors":
					handleErrors(parts[1:])
					continue
				case "/feature":
					handleFeature(agent, parts[1:])
					continue
				case "/settings":
					handleSettings(parts[1:])
					continue
				case "/telemetry":
					handleTelemetry(parts[1:])
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
		args = []string{"list"}
	}

	subcmd := strings.ToLower(args[0])

	switch subcmd {
	case "list", "":
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

// truncateString truncates a string to maxLen characters, adding "..." if truncated.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// parseIdleTimeoutEnv reads CLAUDE_CODE_EXIT_AFTER_STOP_DELAY env var.
// Accepts: duration string ("5m", "30s") or milliseconds as plain number.
// Returns 0 if unset or invalid (meaning idle timeout is disabled).
func parseIdleTimeoutEnv() time.Duration {
	val := os.Getenv("CLAUDE_CODE_EXIT_AFTER_STOP_DELAY")
	if val == "" {
		return 0
	}
	// Try as Go duration first (e.g. "5m", "30s")
	if d, err := time.ParseDuration(val); err == nil {
		return d
	}
	// Try as milliseconds (plain number, matching upstream)
	if ms, err := strconv.ParseInt(val, 10, 64); err == nil && ms > 0 {
		return time.Duration(ms) * time.Millisecond
	}
	return 0
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
	if agent.config.BaseURL != "" && agent.config.BaseURL != "https://api.anthropic.com" {
		fmt.Printf("Base URL: %s (custom)\n", agent.config.BaseURL)
	} else {
		fmt.Println("Base URL: default (api.anthropic.com)")
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

	if len(args) > 0 && strings.ToLower(args[0]) == "clear" {
		if err := os.Remove(history.filePath); err != nil && !os.IsNotExist(err) {
			fmt.Printf("Error clearing history: %v\n", err)
		} else {
			fmt.Println("History cleared.")
		}
		return
	}

	n := 20
	if len(args) > 0 {
		if parsed, err := strconv.Atoi(args[0]); err == nil && parsed > 0 {
			n = parsed
		}
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

// handleBranch creates a conversation branch at the current point.
// The branch saves the current conversation state and starts a new branch
// so the user can explore an alternative path without losing the original.
func handleBranch(agent *AgentLoop) error {
	// Save current transcript as a branch point
	branchName := fmt.Sprintf("branch-%s", time.Now().Format("150405"))
	transcriptDir := filepath.Join(".claude", "branches")
	os.MkdirAll(transcriptDir, 0o755)

	branchFile := filepath.Join(transcriptDir, branchName+".jsonl")
	// Copy current transcript to branch
	srcFile := filepath.Join(".claude", "transcript.jsonl")
	if _, err := os.Stat(srcFile); err == nil {
		data, err := os.ReadFile(srcFile)
		if err != nil {
			return fmt.Errorf("failed to read transcript: %w", err)
		}
		if err := os.WriteFile(branchFile, data, 0o644); err != nil {
			return fmt.Errorf("failed to write branch: %w", err)
		}
		fmt.Printf("Created branch: %s\n", branchName)
	} else {
		fmt.Println("No current transcript to branch from.")
	}
	return nil
}
