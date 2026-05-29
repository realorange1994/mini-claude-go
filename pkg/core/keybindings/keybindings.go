// Package keybindings provides keybinding configuration and management.
// Aligned to pi's keybindings.ts.
package keybindings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// KeyBinding represents a single key binding with optional when condition.
type KeyBinding struct {
	Key   string `json:"key"`   // e.g., "ctrl+c", "escape"
	When  string `json:"when,omitempty"` // e.g., "inputFocused", "confirmDialog"
}

// AppKeybindings maps keybinding IDs to their key combinations.
// Aligned to TS AppKeybindings.
type AppKeybindings struct {
	// Global
	ExitApplication  KeyBinding `json:"exitApplication"`
	CycleModel       KeyBinding `json:"cycleModel"`
	CycleThinkingLevel KeyBinding `json:"cycleThinkingLevel"`
	ToggleFastMode   KeyBinding `json:"toggleFastMode"`
	NewSession       KeyBinding `json:"newSession"`
	ContinueSession  KeyBinding `json:"continueSession"`
	Help             KeyBinding `json:"help"`

	// Prompt input
	SubmitPrompt     KeyBinding `json:"submitPrompt"`
	NewlineInPrompt  KeyBinding `json:"newlineInPrompt"`
	HistoryPrevious  KeyBinding `json:"historyPrevious"`
	HistoryNext      KeyBinding `json:"historyNext"`
	ClearPrompt      KeyBinding `json:"clearPrompt"`

	// Message list
	ScrollUp         KeyBinding `json:"scrollUp"`
	ScrollDown       KeyBinding `json:"scrollDown"`
	ScrollToTop      KeyBinding `json:"scrollToTop"`
	ScrollToBottom   KeyBinding `json:"scrollToBottom"`
	PageUp           KeyBinding `json:"pageUp"`
	PageDown         KeyBinding `json:"pageDown"`

	// Tool actions
	AcceptToolUse    KeyBinding `json:"acceptToolUse"`
	RejectToolUse    KeyBinding `json:"rejectToolUse"`
	AcceptAllTools   KeyBinding `json:"acceptAllTools"`
	CancelToolUse    KeyBinding `json:"cancelToolUse"`

	// Navigation
	GoToFile        KeyBinding `json:"goToFile"`
	GoToDiff        KeyBinding `json:"goToDiff"`
	GoToTerminal    KeyBinding `json:"goToTerminal"`
	FocusPrompt     KeyBinding `json:"focusPrompt"`

	// Undo
	UndoLastCommand KeyBinding `json:"undoLastCommand"`

	// Escape
	Escape         KeyBinding `json:"escape"`
	InterruptAgent KeyBinding `json:"interruptAgent"`
}

// DefaultKeybindings is the default keybinding configuration.
var DefaultKeybindings = AppKeybindings{
	ExitApplication:    KeyBinding{Key: "ctrl+q"},
	CycleModel:         KeyBinding{Key: "ctrl+m"},
	CycleThinkingLevel: KeyBinding{Key: "ctrl+t"},
	ToggleFastMode:     KeyBinding{Key: "ctrl+f"},
	NewSession:         KeyBinding{Key: "ctrl+n"},
	ContinueSession:    KeyBinding{Key: "ctrl+o"},
	Help:               KeyBinding{Key: "ctrl+h"},

	SubmitPrompt:    KeyBinding{Key: "enter", When: "!inputFocused"},
	NewlineInPrompt: KeyBinding{Key: "shift+enter"},
	HistoryPrevious: KeyBinding{Key: "up", When: "promptEmpty"},
	HistoryNext:     KeyBinding{Key: "down", When: "promptEmpty"},
	ClearPrompt:     KeyBinding{Key: "ctrl+l"},

	ScrollUp:       KeyBinding{Key: "up", When: "!promptFocused"},
	ScrollDown:     KeyBinding{Key: "down", When: "!promptFocused"},
	ScrollToTop:    KeyBinding{Key: "home"},
	ScrollToBottom: KeyBinding{Key: "end"},
	PageUp:         KeyBinding{Key: "pageup"},
	PageDown:       KeyBinding{Key: "pagedown"},

	AcceptToolUse:  KeyBinding{Key: "y"},
	RejectToolUse:  KeyBinding{Key: "n"},
	AcceptAllTools: KeyBinding{Key: "a"},
	CancelToolUse:  KeyBinding{Key: "escape"},

	GoToFile:     KeyBinding{Key: "ctrl+g"},
	GoToDiff:     KeyBinding{Key: "ctrl+d"},
	GoToTerminal: KeyBinding{Key: "ctrl+`"},
	FocusPrompt:  KeyBinding{Key: "escape"},

	UndoLastCommand: KeyBinding{Key: "ctrl+z"},

	Escape:         KeyBinding{Key: "escape"},
	InterruptAgent: KeyBinding{Key: "ctrl+c"},
}

// KeybindingsManager manages keybinding configuration with file persistence.
type KeybindingsManager struct {
	mu         sync.RWMutex
	configPath string
	keybindings AppKeybindings
	dirty      bool
}

// NewKeybindingsManager creates a new keybindings manager.
func NewKeybindingsManager(agentDir string) *KeybindingsManager {
	return &KeybindingsManager{
		configPath:  filepath.Join(agentDir, "keybindings.json"),
		keybindings: DefaultKeybindings,
	}
}

// Load reads keybindings from the config file.
func (km *KeybindingsManager) Load() error {
	km.mu.Lock()
	defer km.mu.Unlock()

	data, err := os.ReadFile(km.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Use defaults
		}
		return err
	}

	// Merge with defaults (partial config files are OK)
	result := DefaultKeybindings
	if err := json.Unmarshal(data, &result); err != nil {
		return err
	}

	km.keybindings = result
	return nil
}

// Save writes current keybindings to the config file.
func (km *KeybindingsManager) Save() error {
	km.mu.Lock()
	defer km.mu.Unlock()

	data, err := json.MarshalIndent(km.keybindings, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(km.configPath)
	os.MkdirAll(dir, 0755)

	return os.WriteFile(km.configPath, data, 0644)
}

// Get returns the current keybindings.
func (km *KeybindingsManager) Get() AppKeybindings {
	km.mu.RLock()
	defer km.mu.RUnlock()
	return km.keybindings
}

// Set updates the keybindings.
func (km *KeybindingsManager) Set(kb AppKeybindings) {
	km.mu.Lock()
	defer km.mu.Unlock()
	km.keybindings = kb
	km.dirty = true
}

// Reset restores default keybindings.
func (km *KeybindingsManager) Reset() {
	km.mu.Lock()
	defer km.mu.Unlock()
	km.keybindings = DefaultKeybindings
	km.dirty = true
}

// IsDirty returns whether there are unsaved changes.
func (km *KeybindingsManager) IsDirty() bool {
	km.mu.RLock()
	defer km.mu.RUnlock()
	return km.dirty
}
