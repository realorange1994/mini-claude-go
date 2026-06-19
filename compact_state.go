package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// CompactionState persists compaction history to disk so it survives restarts.
// Inspired by MiMo-Code's checkpoint threshold tracking pattern.
type CompactionState struct {
	mu sync.Mutex

	// CompactionCount tracks total compactions in this session.
	CompactionCount int `json:"compactionCount"`

	// CrossedThresholds records which threshold percentages have been compacted at.
	// Key: threshold percentage (e.g. 75, 80, 85), Value: timestamp of last compaction at that level.
	CrossedThresholds map[int]int64 `json:"crossedThresholds"`

	// LastCompactionTokens records the token count at the time of last compaction.
	LastCompactionTokens int `json:"lastCompactionTokens"`

	// LastCompressionLevel records the compression level used last (1=full, 2=concise, 3=minimal, 4+=ultra).
	LastCompressionLevel int `json:"lastCompressionLevel"`

	// LastCompactionTime records when the last compaction happened.
	LastCompactionTime int64 `json:"lastCompactionTime"`

	// ToolUsageSummary records which tools were used and how frequently in this session.
	// Key: tool name, Value: count of uses.
	ToolUsageSummary map[string]int `json:"toolUsageSummary"`

	// FileTypesTouched records file extensions encountered in this session.
	// Key: extension (e.g. ".go", ".ts"), Value: count.
	FileTypesTouched map[string]int `json:"fileTypesTouched"`

	filePath string
	dirty    bool
	stopCh   chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

// NewCompactionState creates or loads a compaction state from disk.
func NewCompactionState(projectDir string, sessionID string) *CompactionState {
	dir := filepath.Join(projectDir, ".claude", "compact_state")
	os.MkdirAll(dir, 0o755)
	path := filepath.Join(dir, sessionID+".json")

	cs := &CompactionState{
		CrossedThresholds: make(map[int]int64),
		ToolUsageSummary:  make(map[string]int),
		FileTypesTouched:  make(map[string]int),
		filePath:          path,
		stopCh:            make(chan struct{}),
	}

	// Load existing state from disk
	data, err := os.ReadFile(path)
	if err == nil {
		json.Unmarshal(data, cs)
	}

	return cs
}

// RecordCompaction records a compaction event at the given threshold percentage.
func (cs *CompactionState) RecordCompaction(thresholdPct int, tokenCount int, compressionLevel int) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	cs.CompactionCount++
	cs.CrossedThresholds[thresholdPct] = time.Now().UnixMilli()
	cs.LastCompactionTokens = tokenCount
	cs.LastCompressionLevel = compressionLevel
	cs.LastCompactionTime = time.Now().UnixMilli()
	cs.dirty = true
}

// RecordToolUsage records a tool usage event for dynamic prompt generation.
func (cs *CompactionState) RecordToolUsage(toolName string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	cs.ToolUsageSummary[toolName]++
	cs.dirty = true
}

// RecordFileTouch records a file extension encounter.
func (cs *CompactionState) RecordFileTouch(ext string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if ext == "" {
		return
	}
	cs.FileTypesTouched[ext]++
	cs.dirty = true
}

// HasCrossedThreshold returns true if the given threshold has already been compacted at.
func (cs *CompactionState) HasCrossedThreshold(thresholdPct int) bool {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	_, ok := cs.CrossedThresholds[thresholdPct]
	return ok
}

// GetCompactionCount returns the total number of compactions.
func (cs *CompactionState) GetCompactionCount() int {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	return cs.CompactionCount
}

// GetToolUsageSummary returns a copy of the tool usage summary.
func (cs *CompactionState) GetToolUsageSummary() map[string]int {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	result := make(map[string]int, len(cs.ToolUsageSummary))
	for k, v := range cs.ToolUsageSummary {
		result[k] = v
	}
	return result
}

// GetFileTypesTouched returns a copy of the file types touched.
func (cs *CompactionState) GetFileTypesTouched() map[string]int {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	result := make(map[string]int, len(cs.FileTypesTouched))
	for k, v := range cs.FileTypesTouched {
		result[k] = v
	}
	return result
}

// TopTools returns the top N most-used tools.
func (cs *CompactionState) TopTools(n int) []string {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	type toolCount struct {
		name  string
		count int
	}
	var tools []toolCount
	for k, v := range cs.ToolUsageSummary {
		tools = append(tools, toolCount{k, v})
	}
	// Simple insertion sort for small N
	for i := 1; i < len(tools); i++ {
		for j := i; j > 0 && tools[j].count > tools[j-1].count; j-- {
			tools[j], tools[j-1] = tools[j-1], tools[j]
		}
	}
	result := make([]string, 0, n)
	for i := 0; i < len(tools) && i < n; i++ {
		result = append(result, tools[i].name)
	}
	return result
}

// TopFileTypes returns the top N most-touched file extensions.
func (cs *CompactionState) TopFileTypes(n int) []string {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	type ftCount struct {
		ext   string
		count int
	}
	var fts []ftCount
	for k, v := range cs.FileTypesTouched {
		fts = append(fts, ftCount{k, v})
	}
	for i := 1; i < len(fts); i++ {
		for j := i; j > 0 && fts[j].count > fts[j-1].count; j-- {
			fts[j], fts[j-1] = fts[j-1], fts[j]
		}
	}
	result := make([]string, 0, n)
	for i := 0; i < len(fts) && i < n; i++ {
		result = append(result, fts[i].ext)
	}
	return result
}

// StartFlushLoop starts a background goroutine that flushes state to disk periodically.
func (cs *CompactionState) StartFlushLoop() {
	cs.wg.Add(1)
	go func() {
		defer cs.wg.Done()
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-cs.stopCh:
				return
			case <-ticker.C:
				cs.FlushToDisk()
			}
		}
	}()
}

// FlushToDisk writes the state to disk if dirty.
func (cs *CompactionState) FlushToDisk() {
	cs.mu.Lock()
	if !cs.dirty {
		cs.mu.Unlock()
		return
	}
	cs.mu.Unlock()

	cs.mu.Lock()
	data, err := json.MarshalIndent(cs, "", "  ")
	if err != nil {
		cs.mu.Unlock()
		return
	}
	cs.dirty = false
	cs.mu.Unlock()

	// Atomic write: temp file + rename
	tmpPath := cs.filePath + ".tmp"
	os.WriteFile(tmpPath, data, 0o644)
	os.Rename(tmpPath, cs.filePath)
}

// Close stops the flush loop and saves final state.
func (cs *CompactionState) Close() {
	cs.stopOnce.Do(func() {
		close(cs.stopCh)
	})
	cs.wg.Wait()
	cs.FlushToDisk()
}
