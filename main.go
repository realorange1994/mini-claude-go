package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
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

	var agent *AgentLoop

	// Resume from transcript or new session
	if *resumeFile != "" {
		path, err := findTranscript(*resumeFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[!] Resume failed: %v\n", err)
			fmt.Fprintln(os.Stderr, "[*] Starting a new session instead")
			agent = NewAgentLoop(cfg, registry, *stream)
		} else {
			var err2 error
			agent, err2 = NewAgentLoopFromTranscript(cfg, registry, *stream, path)
			if err2 != nil {
				fmt.Fprintf(os.Stderr, "[!] Resume failed: %v\n", err2)
				fmt.Fprintln(os.Stderr, "[*] Starting a new session instead")
				agent = NewAgentLoop(cfg, registry, *stream)
			} else {
				fmt.Printf("[+] Resumed session from transcript: %s\n", path)
			}
		}
	} else {
		agent = NewAgentLoop(cfg, registry, *stream)
	}

	// One-shot mode: positional args are appended as prompt (works with --resume too)
	args := flag.Args()
	if len(args) > 0 {
		prompt := strings.Join(args, " ")
		result := agent.Run(prompt)
		fmt.Println(result)
		agent.Close()
		return
	}

	// Interactive REPL
	runInteractive(agent)
}

func runInteractive(agent *AgentLoop) {
	// Set up Ctrl+C signal handler
	// Only active during agent.Run() -- sets interrupted flag so agent can abort
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT)
	go func() {
		for range signalCh {
			agent.SetInterrupted(true)
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

	stdinReader := bufio.NewReader(os.Stdin)

	// reopenStdin reopens stdin after Ctrl+C breaks it (Windows closes stdin on SIGINT)
	reopenStdin := func() *bufio.Reader {
		var f *os.File
		var err error
		if runtime.GOOS == "windows" {
			f, err = os.OpenFile("CONIN$", os.O_RDWR, 0)
		} else {
			f, err = os.OpenFile("/dev/tty", os.O_RDWR, 0)
		}
		if err != nil {
			// Fallback: can't reopen, program should exit
			return nil
		}
		return bufio.NewReader(f)
	}

	var lastCtrlC time.Time

	for {
		fmt.Print("\n> ")

		line, err := stdinReader.ReadString('\n')
		if err != nil {
			if interactive {
				now := time.Now()
				if !lastCtrlC.IsZero() && now.Sub(lastCtrlC) < 2*time.Second {
					// Double Ctrl+C at prompt -- exit
					printResumeHint(agent)
					return
				}
				lastCtrlC = now
				agent.SetInterrupted(false)
				fmt.Fprintf(os.Stderr, "\n[WARN] Interrupting... (press Ctrl+C again to exit)\n")
				if newReader := reopenStdin(); newReader != nil {
					stdinReader = newReader
					continue
				}
			}
			// Piped input ended or can't reopen -- exit
			printResumeHint(agent)
			break
		}

		lastCtrlC = time.Time{} // reset on successful input

		userInput := strings.TrimSpace(line)
		if userInput == "" {
			continue
		}

		// Check for exact command match — only treat as command if the first
		// word is a known command. Unknown /xxx is passed through as prompt text.
		if strings.HasPrefix(userInput, "/") {
			parts := strings.Fields(userInput)
			cmd := strings.ToLower(parts[0])

			isKnownCmd := cmd == "/quit" || cmd == "/exit" || cmd == "/q" ||
				cmd == "/tools" || cmd == "/mode" || cmd == "/help" || cmd == "/resume"

			if !isKnownCmd {
				// Not a recognized command — treat as normal prompt
			} else {
				switch cmd {
				case "/quit", "/exit", "/q":
					fmt.Println("Goodbye!")
					printResumeHint(agent)
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
					fmt.Println("Commands: /tools, /mode, /resume, /help, /quit")
					continue
				case "/resume":
					if len(parts) > 1 {
						target := parts[1]
						path, err := findTranscript(target)
						if err != nil {
							fmt.Printf("Error: %v\n", err)
							continue
						}
						newAgent, err := NewAgentLoopFromTranscript(agent.config, agent.registry, agent.useStream, path)
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
		// If agent was interrupted, set lastCtrlC so next Ctrl+C at prompt
		// within 2 seconds counts as double-press (exit)
		if agent.IsInterrupted() {
			lastCtrlC = time.Now()
			agent.SetInterrupted(false)
		} else {
			lastCtrlC = time.Time{}
		}
		fmt.Println(result)
		fmt.Println()
	}
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

// findTranscript resolves a transcript reference (number, filename, or 'last').
func findTranscript(target string) (string, error) {
	dir := ".claude/transcripts"
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("no transcripts directory: %w", err)
	}

	type entry struct {
		name string
		mod  time.Time
	}
	var files []entry
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".jsonl") {
			info, _ := e.Info()
			files = append(files, entry{name: e.Name(), mod: info.ModTime()})
		}
	}
	if len(files) == 0 {
		return "", fmt.Errorf("no transcripts found")
	}

	// Sort by modification time, most recent first
	for i := 0; i < len(files); i++ {
		for j := i + 1; j < len(files); j++ {
			if files[j].mod.After(files[i].mod) {
				files[i], files[j] = files[j], files[i]
			}
		}
	}

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
	dir := ".claude/transcripts"
	entries, err := os.ReadDir(dir)
	if err != nil {
		fmt.Println("No transcripts found.")
		return
	}

	type entry struct {
		name string
		mod  time.Time
	}
	var files []entry
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".jsonl") {
			info, _ := e.Info()
			files = append(files, entry{name: e.Name(), mod: info.ModTime()})
		}
	}

	if len(files) == 0 {
		fmt.Println("No transcripts found.")
		return
	}

	// Sort by modification time, most recent first
	for i := 0; i < len(files); i++ {
		for j := i + 1; j < len(files); j++ {
			if files[j].mod.After(files[i].mod) {
				files[i], files[j] = files[j], files[i]
			}
		}
	}

	fmt.Println("\nAvailable transcripts:")
	for i, f := range files {
		fmt.Printf("  %d. %s\n", i+1, f.name)
	}
	fmt.Println("\nUsage: /resume <number>, /resume <filename>, or /resume last")
}
