package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ─── Sophisticated Tool Output Truncation Service (MiMo-Code 2A) ────────────
//
// Enhanced truncation with:
// - Error-aware head+tail splitting
// - Pressure-based caps
// - Full output saved to disk with retention
// - Context-aware hints
//
// MiMo-Code source: tool/truncate.ts (201 lines)

const (
	// DefaultMaxChars default maximum characters for tool output
	DefaultMaxChars = 8000
	// PressureMaxChars reduced max when under pressure
	PressureMaxChars = 4000
	// HeadRatio percentage of budget for head when error found
	HeadRatio = 70
	// TailRatio percentage of budget for tail when error found
	TailRatio = 30
	// RetentionDays days to keep saved tool outputs
	RetentionDays = 7
)

// TruncationDirection controls which part of output to keep.
type TruncationDirection string

const (
	TruncHead     TruncationDirection = "head"      // keep head only
	TruncTail     TruncationDirection = "tail"      // keep tail only
	TruncHeadTail TruncationDirection = "head+tail" // keep both (error-aware)
)

// TruncationResult holds the result of a truncation.
type TruncationResult struct {
	Truncated    bool
	Output       string
	OriginalSize int
	TruncatedTo  int
	HasError     bool
	Direction    TruncationDirection
	SavedPath    string // path where full output was saved
	Hint         string // hint for recovering full output
}

// TruncationService provides sophisticated tool output truncation.
type TruncationService struct {
	mu           sync.Mutex
	maxChars     int
	pressure     bool
	saveDir      string
	direction    TruncationDirection
	errorPatterns []string
}

// NewTruncationService creates a new truncation service.
func NewTruncationService(saveDir string) *TruncationService {
	return &TruncationService{
		maxChars:  DefaultMaxChars,
		saveDir:   saveDir,
		direction: TruncHeadTail,
		errorPatterns: []string{
			"error", "fail", "panic", "fatal",
			"exception", "traceback", "exception",
		},
	}
}

// SetPressure enables or disables pressure mode (halves max chars).
func (s *TruncationService) SetPressure(pressure bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pressure = pressure
}

// SetDirection sets the truncation direction.
func (s *TruncationService) SetDirection(dir TruncationDirection) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.direction = dir
}

// Truncate truncates tool output with sophisticated strategies.
func (s *TruncationService) Truncate(toolName, output string) TruncationResult {
	s.mu.Lock()
	maxChars := s.maxChars
	if s.pressure {
		maxChars = PressureMaxChars
	}
	direction := s.direction
	s.mu.Unlock()

	// Check if truncation needed
	if len(output) <= maxChars {
		return TruncationResult{
			Truncated:    false,
			Output:       output,
			OriginalSize: len(output),
			TruncatedTo:  len(output),
			Direction:    direction,
		}
	}

	// Scan for error patterns in tail
	hasError := s.scanForErrors(output)

	// Save full output to disk
	savedPath := s.saveToDisk(toolName, output)

	// Truncate based on direction and error presence
	var truncated string
	var hint string

	switch direction {
	case TruncTail:
		truncated = s.truncateTail(output, maxChars)
		hint = s.buildHint(toolName, "head", savedPath)
	case TruncHead:
		truncated = s.truncateHead(output, maxChars)
		hint = s.buildHint(toolName, "tail", savedPath)
	default: // head+tail
		if hasError {
			truncated = s.truncateHeadTail(output, maxChars)
			hint = s.buildHint(toolName, "full", savedPath)
		} else {
			truncated = s.truncateHead(output, maxChars)
			hint = s.buildHint(toolName, "tail", savedPath)
		}
	}

	return TruncationResult{
		Truncated:    true,
		Output:       truncated,
		OriginalSize: len(output),
		TruncatedTo:  len(truncated),
		HasError:     hasError,
		Direction:    direction,
		SavedPath:    savedPath,
		Hint:         hint,
	}
}

// scanForErrors checks if output contains error patterns in the tail.
func (s *TruncationService) scanForErrors(output string) bool {
	lines := strings.Split(output, "\n")
	tailStart := len(lines) * 70 / 100
	if tailStart < 10 {
		tailStart = 10
	}

	for i := tailStart; i < len(lines); i++ {
		lower := strings.ToLower(lines[i])
		for _, pattern := range s.errorPatterns {
			if strings.Contains(lower, pattern) {
				return true
			}
		}
	}
	return false
}

// truncateHead keeps the head of the output.
func (s *TruncationService) truncateHead(output string, maxChars int) string {
	truncated := output[:maxChars]
	if idx := strings.LastIndex(truncated, "\n"); idx > maxChars/2 {
		truncated = truncated[:idx]
	}
	return truncated + "\n[... truncated ...]"
}

// truncateTail keeps the tail of the output.
func (s *TruncationService) truncateTail(output string, maxChars int) string {
	tail := output[len(output)-maxChars:]
	if idx := strings.Index(tail, "\n"); idx < len(tail)/2 {
		tail = tail[idx+1:]
	}
	return "[... truncated ...]\n" + tail
}

// truncateHeadTail keeps head and tail (error-aware).
func (s *TruncationService) truncateHeadTail(output string, maxChars int) string {
	headChars := maxChars * HeadRatio / 100
	tailChars := maxChars * TailRatio / 100

	head := output[:headChars]
	if idx := strings.LastIndex(head, "\n"); idx > headChars/2 {
		head = head[:idx]
	}

	tail := output[len(output)-tailChars:]
	if idx := strings.Index(tail, "\n"); idx < len(tail)/2 {
		tail = tail[idx+1:]
	}

	return head + "\n\n[... truncated — error context preserved from tail ...]\n\n" + tail
}

// saveToDisk saves full output to disk for later retrieval.
func (s *TruncationService) saveToDisk(toolName, output string) string {
	if s.saveDir == "" {
		return ""
	}

	// Ensure directory exists
	os.MkdirAll(s.saveDir, 0755)

	// Generate filename
	timestamp := time.Now().Format("20060102-150405")
	filename := fmt.Sprintf("%s_%s.txt", toolName, timestamp)
	path := filepath.Join(s.saveDir, filename)

	// Write output
	os.WriteFile(path, []byte(output), 0644)

	return path
}

// buildHint builds a hint for recovering the full output.
func (s *TruncationService) buildHint(toolName, recoverPart, savedPath string) string {
	if savedPath != "" {
		return fmt.Sprintf("Full output saved to %s. Use Read(\"%s\") to recover %s.",
			savedPath, savedPath, recoverPart)
	}
	return fmt.Sprintf("Output truncated. Use Grep or Read with offset to recover %s.", recoverPart)
}

// CleanupOldOutputs removes saved outputs older than RetentionDays.
func (s *TruncationService) CleanupOldOutputs() int {
	if s.saveDir == "" {
		return 0
	}

	entries, err := os.ReadDir(s.saveDir)
	if err != nil {
		return 0
	}

	cutoff := time.Now().AddDate(0, 0, -RetentionDays)
	removed := 0

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			os.Remove(filepath.Join(s.saveDir, entry.Name()))
			removed++
		}
	}

	return removed
}
