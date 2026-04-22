package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"
)

const banner = `
  +--------------------------------------+
  |       miniClaudeCode v0.1.0         |
  |  Distilled Agent Loop Framework     |
  +--------------------------------------+

  Type your message to start. Commands:
    /tools   -- list available tools
    /mode    -- show/change permission mode
    /help    -- show help
    /quit    -- exit
`

func main() {
	model := flag.String("model", "", "Anthropic model to use")
	apiKey := flag.String("api-key", "", "API key (overrides ANTHROPIC_API_KEY env and config file)")
	baseURL := flag.String("base-url", "", "Custom API base URL (overrides config file)")
	mode := flag.String("mode", "ask", "Permission mode (ask|auto|plan)")
	maxTurns := flag.Int("max-turns", 30, "Max agent loop turns per message")
	stream := flag.Bool("stream", false, "Enable streaming output")
	flag.Parse()

	// Priority: flags > env > .claude/settings.json > defaults
	cfg := DefaultConfig()

	// Load from .claude/settings.json (project-level config)
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
			cfg.MCPManager = fileCfg.MCPManager
			cfg.SkillLoader = fileCfg.SkillLoader
		}
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

	defer cfg.Close()

	registry := DefaultRegistry()
	RegisterMCPAndSkills(registry, &cfg)
	agent := NewAgentLoop(cfg, registry, *stream)

	// One-shot mode
	args := flag.Args()
	if len(args) > 0 {
		prompt := strings.Join(args, " ")
		result := agent.Run(prompt)
		fmt.Println(result)
		return
	}

	// Interactive REPL
	runInteractive(agent)
}

func runInteractive(agent *AgentLoop) {
	defer agent.Close()
	fmt.Print(banner)

	scanner := bufio.NewScanner(os.Stdin)
	// Increase buffer size for long inputs
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	for {
		fmt.Print("\n> ")
		if !scanner.Scan() {
			fmt.Println("\nGoodbye!")
			break
		}

		userInput := strings.TrimSpace(scanner.Text())
		if userInput == "" {
			continue
		}

		if strings.HasPrefix(userInput, "/") {
			parts := strings.Fields(strings.ToLower(userInput))
			cmd := parts[0]

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
					switch parts[1] {
					case "ask", "auto", "plan":
						agent.config.PermissionMode = PermissionMode(parts[1])
						fmt.Printf("Mode changed to: %s\n", parts[1])
					default:
						fmt.Printf("Unknown mode: %s\n", parts[1])
					}
				} else {
					fmt.Printf("Current mode: %s\n", agent.config.PermissionMode)
					fmt.Println("Usage: /mode [ask|auto|plan]")
				}
				continue
			case "/help":
				fmt.Print(banner)
				continue
			default:
				fmt.Printf("Unknown command: %s. Type /help for help.\n", cmd)
				continue
			}
		}

		fmt.Println()
		result := agent.Run(userInput)
		fmt.Println(result)
		fmt.Println()
	}
}
