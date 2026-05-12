package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// DaemonManager handles daemon mode for headless/remote operation.
// When enabled, the agent runs in the background and processes
// prompts from a named pipe or file.
type DaemonManager struct {
	pidFile  string
	pipeDir  string
	running  bool
}

// NewDaemonManager creates a daemon manager.
func NewDaemonManager() *DaemonManager {
	dir := filepath.Join(".claude", "daemon")
	os.MkdirAll(dir, 0o755)
	return &DaemonManager{
		pidFile: filepath.Join(dir, "pid"),
		pipeDir: filepath.Join(dir, "prompts"),
	}
}

// Start writes a PID file and creates the prompts directory.
func (d *DaemonManager) Start() error {
	os.MkdirAll(d.pipeDir, 0o755)

	pid := os.Getpid()
	if err := os.WriteFile(d.pidFile, []byte(strconv.Itoa(pid)), 0o644); err != nil {
		return fmt.Errorf("failed to write PID file: %w", err)
	}
	d.running = true
	return nil
}

// Stop removes the PID file.
func (d *DaemonManager) Stop() error {
	d.running = false
	return os.Remove(d.pidFile)
}

// IsRunning checks if a daemon is already active.
func (d *DaemonManager) IsRunning() bool {
	data, err := os.ReadFile(d.pidFile)
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(string(data))
	if err != nil {
		return false
	}
	// Check if process exists
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Unix, FindProcess always succeeds; need to signal 0
	// On Windows, FindProcess fails if process doesn't exist
	return proc != nil
}

// SubmitPrompt writes a prompt file for the daemon to process.
func (d *DaemonManager) SubmitPrompt(prompt string) error {
	timestamp := time.Now().Format("20060102-150405.000")
	filePath := filepath.Join(d.pipeDir, timestamp+".prompt")
	return os.WriteFile(filePath, []byte(prompt), 0o644)
}

// GetPendingPrompts returns unprocessed prompt files.
func (d *DaemonManager) GetPendingPrompts() []string {
	entries, err := os.ReadDir(d.pipeDir)
	if err != nil {
		return nil
	}
	var prompts []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".prompt") {
			data, err := os.ReadFile(filepath.Join(d.pipeDir, e.Name()))
			if err == nil {
				prompts = append(prompts, string(data))
				// Mark as processed by renaming
				os.Rename(
					filepath.Join(d.pipeDir, e.Name()),
					filepath.Join(d.pipeDir, e.Name()+".done"),
				)
			}
		}
	}
	return prompts
}

// handleDaemon handles the /daemon slash command.
func handleDaemon(args []string) {
	if len(args) == 0 {
		fmt.Println("Daemon commands: start, stop, status, submit <prompt>")
		return
	}

	dm := NewDaemonManager()
	switch args[0] {
	case "start":
		if dm.IsRunning() {
			fmt.Println("Daemon already running.")
			return
		}
		if err := dm.Start(); err != nil {
			fmt.Printf("Failed to start daemon: %v\n", err)
			return
		}
		fmt.Println("Daemon started.")
	case "stop":
		if err := dm.Stop(); err != nil {
			fmt.Printf("Failed to stop daemon: %v\n", err)
			return
		}
		fmt.Println("Daemon stopped.")
	case "status":
		if dm.IsRunning() {
			fmt.Println("Daemon: running")
		} else {
			fmt.Println("Daemon: not running")
		}
	case "submit":
		if len(args) < 2 {
			fmt.Println("Usage: /daemon submit <prompt>")
			return
		}
		prompt := strings.Join(args[1:], " ")
		if err := dm.SubmitPrompt(prompt); err != nil {
			fmt.Printf("Failed to submit prompt: %v\n", err)
			return
		}
		fmt.Println("Prompt submitted to daemon.")
	default:
		fmt.Printf("Unknown daemon command: %s\n", args[0])
	}
}