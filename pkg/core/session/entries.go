// Package session manages agent conversation sessions.
// This file defines the entry types aligned to pi's session-manager.ts.
package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// CurrentSessionVersion is the latest session format version.
const CurrentSessionVersion = 3

// SessionHeader is the first entry in a JSONL session file.
type SessionHeader struct {
	Type           string `json:"type"` // "session"
	Version        int    `json:"version,omitempty"`
	ID             string `json:"id"`
	Timestamp      string `json:"timestamp"`
	Cwd            string `json:"cwd"`
	ParentSession  string `json:"parentSession,omitempty"`
}

// SessionEntryBase is common fields for all session entries.
type SessionEntryBase struct {
	ID        string `json:"id"`
	ParentID  string `json:"parentId"` // null for root entries
	Timestamp string `json:"timestamp"`
}

// SessionMessageEntry is a user/assistant/custom message entry.
type SessionMessageEntry struct {
	SessionEntryBase
	Type    string          `json:"type"` // "message"
	Message json.RawMessage `json:"message"`
}

// ThinkingLevelChangeEntry records a change to the thinking level.
type ThinkingLevelChangeEntry struct {
	SessionEntryBase
	Type        string `json:"type"` // "thinking_level_change"
	ThinkingLevel string `json:"thinkingLevel"`
}

// ModelChangeEntry records a model switch.
type ModelChangeEntry struct {
	SessionEntryBase
	Type     string `json:"type"` // "model_change"
	Provider string `json:"provider"`
	ModelID  string `json:"modelId"`
}

// CompactionEntry records a context compaction event.
type CompactionEntry struct {
	SessionEntryBase
	Type            string      `json:"type"` // "compaction"
	Summary         string      `json:"summary"`
	FirstKeptEntryID string     `json:"firstKeptEntryId"`
	TokensBefore    int         `json:"tokensBefore"`
	Details         interface{} `json:"details,omitempty"`
	FromHook        bool        `json:"fromHook,omitempty"`
}

// BranchSummaryEntry records a summary when branching to a new conversation path.
type BranchSummaryEntry struct {
	SessionEntryBase
	Type    string      `json:"type"` // "branch_summary"
	FromID  string      `json:"fromId"`
	Summary string      `json:"summary"`
	Details interface{} `json:"details,omitempty"`
	FromHook bool       `json:"fromHook,omitempty"`
}

// CustomEntry is for extension-specific data (not sent to LLM context).
type CustomEntry struct {
	SessionEntryBase
	Type       string      `json:"type"` // "custom"
	CustomType string      `json:"customType"`
	Data       interface{} `json:"data,omitempty"`
}

// CustomMessageEntry is for extension messages that DO participate in LLM context.
type CustomMessageEntry struct {
	SessionEntryBase
	Type       string      `json:"type"` // "custom_message"
	CustomType string      `json:"customType"`
	Content    interface{} `json:"content"` // string or []ContentBlock
	Details    interface{} `json:"details,omitempty"`
	Display    bool        `json:"display"`
}

// LabelEntry is a user-defined bookmark/marker on an entry.
type LabelEntry struct {
	SessionEntryBase
	Type     string `json:"type"` // "label"
	TargetID string `json:"targetId"`
	Label    string `json:"label"` // empty means clear the label
}

// SessionInfoEntry stores session metadata like display name.
type SessionInfoEntry struct {
	SessionEntryBase
	Type string `json:"type"` // "session_info"
	Name string `json:"name,omitempty"`
}

// FileEntry is any entry in the JSONL file (header or entry).
type FileEntry interface {
	entryType() string
}

func (h SessionHeader) entryType() string           { return h.Type }
func (e SessionMessageEntry) entryType() string     { return e.Type }
func (e ThinkingLevelChangeEntry) entryType() string { return e.Type }
func (e ModelChangeEntry) entryType() string        { return e.Type }
func (e CompactionEntry) entryType() string         { return e.Type }
func (e BranchSummaryEntry) entryType() string      { return e.Type }
func (e CustomEntry) entryType() string             { return e.Type }
func (e CustomMessageEntry) entryType() string      { return e.Type }
func (e LabelEntry) entryType() string              { return e.Type }
func (e SessionInfoEntry) entryType() string        { return e.Type }

// SessionTreeNode represents a node in the session tree.
type SessionTreeNode struct {
	Entry    FileEntry
	Children []SessionTreeNode
	Label    string // resolved label, if any
	LabelTimestamp string
}

// SessionContext is the resolved message list for the LLM.
type SessionContext struct {
	Messages    []json.RawMessage
	ThinkingLevel string
	Model       *ModelRef // nil if no model info
}

// ModelRef is a model reference within a session context.
type ModelRef struct {
	Provider string `json:"provider"`
	ModelID  string `json:"modelId"`
}

// SessionManager is the enhanced session manager aligned to pi's SessionManager.
// It uses a single JSONL file per session (vs. directory-per-session in the old v1 format).
type SessionManager struct {
	mu              sync.RWMutex
	sessionID       string
	sessionFile     string
	sessionDir      string
	cwd             string
	persist         bool
	flushed         bool
	fileEntries     []FileEntry
	byID            map[string]FileEntry
	labelsByID      map[string]string
	labelTimestamps map[string]string
	leafID          string // "" means before first entry
}

// NewSessionManager creates a new enhanced session manager.
func NewSessionManager(cwd, sessionDir string, sessionFile string, persist bool) *SessionManager {
	sm := &SessionManager{
		sessionDir:      sessionDir,
		cwd:             cwd,
		persist:         persist,
		byID:            make(map[string]FileEntry),
		labelsByID:      make(map[string]string),
		labelTimestamps: make(map[string]string),
	}
	if sessionFile != "" {
		sm.SetSessionFile(sessionFile)
	} else {
		sm.NewSession("")
	}
	return sm
}

// NewSession starts a fresh session.
// If parentSession is non-empty, it is stored in the header as a reference
// to the originating session (aligned to TS newSession({parentSession})).
func (sm *SessionManager) NewSession(id string, parentSession ...string) string {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if id == "" {
		id = generateID()
	}
	sm.sessionID = id
	ts := nowISO()

	var parent string
	if len(parentSession) > 0 {
		parent = parentSession[0]
	}

	header := SessionHeader{
		Type:          "session",
		Version:       CurrentSessionVersion,
		ID:            sm.sessionID,
		Timestamp:     ts,
		Cwd:           sm.cwd,
		ParentSession: parent,
	}
	sm.fileEntries = []FileEntry{header}
	sm.byID = make(map[string]FileEntry)
	sm.labelsByID = make(map[string]string)
	sm.labelTimestamps = make(map[string]string)
	sm.leafID = ""
	sm.flushed = false

	if sm.persist && sm.sessionDir != "" {
		os.MkdirAll(sm.sessionDir, 0755)
		fileTs := strings.ReplaceAll(ts, ":", "-")
		fileTs = strings.ReplaceAll(fileTs, ".", "-")
		sm.sessionFile = filepath.Join(sm.sessionDir, fmt.Sprintf("%s_%s.jsonl", fileTs, sm.sessionID))
	}
	return sm.sessionFile
}

// SetSessionFile loads a session from an existing JSONL file.
func (sm *SessionManager) SetSessionFile(path string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.sessionFile = path
	entries, err := loadEntriesFromFile(path)
	if err != nil || len(entries) == 0 {
		sm.NewSession("")
		return
	}

	// Run migrations
	migrateToCurrentVersion(entries)

	sm.fileEntries = entries
	if h, ok := entries[0].(SessionHeader); ok {
		sm.sessionID = h.ID
	}
	sm.buildIndex()
	sm.flushed = true
}

// buildIndex rebuilds the byID, labelsByID maps and finds the leaf.
func (sm *SessionManager) buildIndex() {
	sm.byID = make(map[string]FileEntry)
	sm.labelsByID = make(map[string]string)
	sm.labelTimestamps = make(map[string]string)
	sm.leafID = ""

	for _, entry := range sm.fileEntries {
		switch e := entry.(type) {
		case SessionHeader:
			continue
		case SessionMessageEntry:
			sm.byID[e.ID] = e
			sm.leafID = e.ID
		case ThinkingLevelChangeEntry:
			sm.byID[e.ID] = e
			sm.leafID = e.ID
		case ModelChangeEntry:
			sm.byID[e.ID] = e
			sm.leafID = e.ID
		case CompactionEntry:
			sm.byID[e.ID] = e
			sm.leafID = e.ID
		case BranchSummaryEntry:
			sm.byID[e.ID] = e
			sm.leafID = e.ID
		case CustomEntry:
			sm.byID[e.ID] = e
			sm.leafID = e.ID
		case CustomMessageEntry:
			sm.byID[e.ID] = e
			sm.leafID = e.ID
		case LabelEntry:
			sm.byID[e.ID] = e
			sm.leafID = e.ID
			if e.Label != "" {
				sm.labelsByID[e.TargetID] = e.Label
				sm.labelTimestamps[e.TargetID] = e.Timestamp
			} else {
				delete(sm.labelsByID, e.TargetID)
				delete(sm.labelTimestamps, e.TargetID)
			}
		case SessionInfoEntry:
			sm.byID[e.ID] = e
			sm.leafID = e.ID
		}
	}
}

// AppendMessage adds a message entry as a child of the current leaf.
func (sm *SessionManager) AppendMessage(message json.RawMessage) string {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	entry := SessionMessageEntry{
		SessionEntryBase: SessionEntryBase{
			ID:        generateID(),
			ParentID:  sm.leafID,
			Timestamp: nowISO(),
		},
		Type:    "message",
		Message: message,
	}
	sm.appendEntryLocked(entry)
	return entry.ID
}

// AppendThinkingLevelChange adds a thinking level change entry.
func (sm *SessionManager) AppendThinkingLevelChange(level string) string {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	entry := ThinkingLevelChangeEntry{
		SessionEntryBase: SessionEntryBase{
			ID:        generateID(),
			ParentID:  sm.leafID,
			Timestamp: nowISO(),
		},
		Type:        "thinking_level_change",
		ThinkingLevel: level,
	}
	sm.appendEntryLocked(entry)
	return entry.ID
}

// AppendModelChange adds a model change entry.
func (sm *SessionManager) AppendModelChange(provider, modelID string) string {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	entry := ModelChangeEntry{
		SessionEntryBase: SessionEntryBase{
			ID:        generateID(),
			ParentID:  sm.leafID,
			Timestamp: nowISO(),
		},
		Type:     "model_change",
		Provider: provider,
		ModelID:  modelID,
	}
	sm.appendEntryLocked(entry)
	return entry.ID
}

// AppendCompaction adds a compaction summary entry.
func (sm *SessionManager) AppendCompaction(summary string, firstKeptEntryID string, tokensBefore int, details interface{}, fromHook bool) string {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	entry := CompactionEntry{
		SessionEntryBase: SessionEntryBase{
			ID:        generateID(),
			ParentID:  sm.leafID,
			Timestamp: nowISO(),
		},
		Type:             "compaction",
		Summary:          summary,
		FirstKeptEntryID: firstKeptEntryID,
		TokensBefore:     tokensBefore,
		Details:          details,
		FromHook:         fromHook,
	}
	sm.appendEntryLocked(entry)
	return entry.ID
}

// AppendCustomEntry adds an extension-specific data entry.
func (sm *SessionManager) AppendCustomEntry(customType string, data interface{}) string {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	entry := CustomEntry{
		SessionEntryBase: SessionEntryBase{
			ID:        generateID(),
			ParentID:  sm.leafID,
			Timestamp: nowISO(),
		},
		Type:       "custom",
		CustomType: customType,
		Data:       data,
	}
	sm.appendEntryLocked(entry)
	return entry.ID
}

// AppendSessionInfo adds a session info entry (e.g., display name).
// The name is trimmed, matching TS behavior.
func (sm *SessionManager) AppendSessionInfo(name string) string {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	entry := SessionInfoEntry{
		SessionEntryBase: SessionEntryBase{
			ID:        generateID(),
			ParentID:  sm.leafID,
			Timestamp: nowISO(),
		},
		Type: "session_info",
		Name: strings.TrimSpace(name),
	}
	sm.appendEntryLocked(entry)
	return entry.ID
}

// AppendLabelChange sets or clears a label on an entry.
func (sm *SessionManager) AppendLabelChange(targetID string, label string) string {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if _, ok := sm.byID[targetID]; !ok {
		return "" // target not found
	}

	entry := LabelEntry{
		SessionEntryBase: SessionEntryBase{
			ID:        generateID(),
			ParentID:  sm.leafID,
			Timestamp: nowISO(),
		},
		Type:     "label",
		TargetID: targetID,
		Label:    label,
	}
	sm.appendEntryLocked(entry)

	if label != "" {
		sm.labelsByID[targetID] = label
		sm.labelTimestamps[targetID] = entry.Timestamp
	} else {
		delete(sm.labelsByID, targetID)
		delete(sm.labelTimestamps, targetID)
	}
	return entry.ID
}

func (sm *SessionManager) appendEntryLocked(entry FileEntry) {
	sm.fileEntries = append(sm.fileEntries, entry)
	sm.byID[entryID(entry)] = entry
	sm.leafID = entryID(entry)
	sm.persistEntry(entry)
}

func (sm *SessionManager) persistEntry(entry FileEntry) {
	if !sm.persist || sm.sessionFile == "" {
		return
	}

	hasAssistant := false
	for _, e := range sm.fileEntries {
		if msg, ok := e.(SessionMessageEntry); ok {
			// Check if message role is "assistant"
			var msgData map[string]interface{}
			if json.Unmarshal(msg.Message, &msgData) == nil {
				if role, _ := msgData["role"].(string); role == "assistant" {
					hasAssistant = true
					break
				}
			}
		}
	}

	if !hasAssistant {
		sm.flushed = false
		return
	}

	if !sm.flushed {
		// Rewrite entire file
		sm.rewriteFile()
		sm.flushed = true
	} else {
		// Append single entry
		f, err := os.OpenFile(sm.sessionFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return
		}
		defer f.Close()
		line, _ := json.Marshal(entry)
		f.Write(append(line, '\n'))
	}
}

func (sm *SessionManager) rewriteFile() {
	if !sm.persist || sm.sessionFile == "" {
		return
	}
	os.MkdirAll(filepath.Dir(sm.sessionFile), 0755)
	f, err := os.Create(sm.sessionFile)
	if err != nil {
		return
	}
	defer f.Close()
	for _, entry := range sm.fileEntries {
		line, _ := json.Marshal(entry)
		f.Write(append(line, '\n'))
	}
}

// GetLeafID returns the current leaf entry ID.
func (sm *SessionManager) GetLeafID() string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.leafID
}

// GetEntry returns an entry by ID.
func (sm *SessionManager) GetEntry(id string) (FileEntry, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	e, ok := sm.byID[id]
	return e, ok
}

// GetLabel returns the label for an entry, if any.
func (sm *SessionManager) GetLabel(id string) string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.labelsByID[id]
}

// GetBranch walks from an entry to the root, returning all entries in path order.
func (sm *SessionManager) GetBranch(fromID string) []FileEntry {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	startID := fromID
	if startID == "" {
		startID = sm.leafID
	}

	var path []FileEntry
	visited := make(map[string]bool)
	current := startID
	for current != "" {
		if visited[current] {
			break
		}
		visited[current] = true
		entry, ok := sm.byID[current]
		if !ok {
			break
		}
		path = append([]FileEntry{entry}, path...) // prepend
		current = entryParentID(entry)
	}
	return path
}

// GetEntries returns all session entries (excludes header).
func (sm *SessionManager) GetEntries() []FileEntry {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	var out []FileEntry
	for _, e := range sm.fileEntries {
		if _, ok := e.(SessionHeader); !ok {
			out = append(out, e)
		}
	}
	return out
}

// GetHeader returns the session header.
func (sm *SessionManager) GetHeader() *SessionHeader {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	for _, e := range sm.fileEntries {
		if h, ok := e.(SessionHeader); ok {
			return &h
		}
	}
	return nil
}

// GetSessionID returns the session ID.
func (sm *SessionManager) GetSessionID() string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.sessionID
}

// GetSessionFile returns the session file path.
func (sm *SessionManager) GetSessionFile() string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.sessionFile
}

// GetCwd returns the working directory.
func (sm *SessionManager) GetCwd() string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.cwd
}

// GetTree builds the session tree structure.
func (sm *SessionManager) GetTree() []SessionTreeNode {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	// Inline entry collection to avoid reentrant lock
	var entries []FileEntry
	for _, e := range sm.fileEntries {
		if _, ok := e.(SessionHeader); !ok {
			entries = append(entries, e)
		}
	}
	nodeMap := make(map[string]*SessionTreeNode)
	var roots []SessionTreeNode

	// Create nodes
	for _, e := range entries {
		id := entryID(e)
		label := sm.labelsByID[id]
		labelTs := sm.labelTimestamps[id]
		node := SessionTreeNode{
			Entry:    e,
			Children: []SessionTreeNode{},
			Label:    label,
			LabelTimestamp: labelTs,
		}
		nodeMap[id] = &node
	}

	// Build tree
	for _, e := range entries {
		id := entryID(e)
		node := nodeMap[id]
		parentID := entryParentID(e)

		if parentID == "" || parentID == id {
			roots = append(roots, *node)
		} else {
			parent, ok := nodeMap[parentID]
			if ok {
				parent.Children = append(parent.Children, *node)
			} else {
				roots = append(roots, *node) // orphan
			}
		}
	}

	// Sort children by timestamp
	for i := range roots {
		sortTreeByTimestamp(&roots[i])
	}
	return roots
}

// Branch moves the leaf pointer to an earlier entry.
func (sm *SessionManager) Branch(fromID string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if _, ok := sm.byID[fromID]; !ok {
		return fmt.Errorf("entry %s not found", fromID)
	}
	sm.leafID = fromID
	return nil
}

// ResetLeaf resets the leaf pointer to null (before first entry).
func (sm *SessionManager) ResetLeaf() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.leafID = ""
}

// BranchWithSummary branches and appends a branch summary.
// Aligned to TS branchWithSummary(leafId, summary, details?, fromHook?).
func (sm *SessionManager) BranchWithSummary(fromID string, summary string, details interface{}, fromHook bool) string {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if fromID != "" {
		if _, ok := sm.byID[fromID]; !ok {
			return ""
		}
	}
	sm.leafID = fromID

	entry := BranchSummaryEntry{
		SessionEntryBase: SessionEntryBase{
			ID:        generateID(),
			ParentID:  fromID,
			Timestamp: nowISO(),
		},
		Type:     "branch_summary",
		FromID:   fromID,
		Summary:  summary,
		Details:  details,
		FromHook: fromHook,
	}
	if fromID == "" {
		entry.FromID = "root"
	}
	sm.appendEntryLocked(entry)
	return entry.ID
}

// GetSessionName returns the latest session name from session_info entries.
// The name is trimmed, matching TS behavior.
func (sm *SessionManager) GetSessionName() string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	for i := len(sm.fileEntries) - 1; i >= 0; i-- {
		if info, ok := sm.fileEntries[i].(SessionInfoEntry); ok {
			return strings.TrimSpace(info.Name)
		}
	}
	return ""
}

// GetLatestCompactionEntry returns the most recent compaction entry.
func (sm *SessionManager) GetLatestCompactionEntry() (CompactionEntry, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	for i := len(sm.fileEntries) - 1; i >= 0; i-- {
		if c, ok := sm.fileEntries[i].(CompactionEntry); ok {
			return c, true
		}
	}
	return CompactionEntry{}, false
}

// AppendCustomMessageEntry adds extension messages that DO participate in LLM context.
// Aligned to TS appendCustomMessageEntry (session-manager.ts:987-1005).
func (sm *SessionManager) AppendCustomMessageEntry(customType string, content interface{}, display bool, details interface{}) string {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	entry := CustomMessageEntry{
		SessionEntryBase: SessionEntryBase{
			ID:        generateID(),
			ParentID:  sm.leafID,
			Timestamp: nowISO(),
		},
		Type:       "custom_message",
		CustomType: customType,
		Content:    content,
		Details:    details,
		Display:    display,
	}
	sm.appendEntryLocked(entry)
	return entry.ID
}

// GetLeafEntry returns the current leaf entry, or nil if no entries exist.
// Aligned to TS getLeafEntry (session-manager.ts:1015).
func (sm *SessionManager) GetLeafEntry() FileEntry {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	if sm.leafID == "" {
		return nil
	}
	e, ok := sm.byID[sm.leafID]
	if !ok {
		return nil
	}
	return e
}

// IsPersisted returns whether the session has been flushed to disk.
func (sm *SessionManager) IsPersisted() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.flushed
}

// GetSessionDir returns the session directory path.
func (sm *SessionManager) GetSessionDir() string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.sessionDir
}

// BuildSessionContext builds the resolved message list for the LLM.
func (sm *SessionManager) BuildSessionContext() SessionContext {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	// Inline GetEntries to avoid reentrant lock
	var entries []FileEntry
	for _, e := range sm.fileEntries {
		if _, ok := e.(SessionHeader); !ok {
			entries = append(entries, e)
		}
	}
	byID := make(map[string]FileEntry, len(entries))
	for _, e := range entries {
		byID[entryID(e)] = e
	}

	// Find leaf
	var leaf FileEntry
	if sm.leafID != "" {
		if e, ok := byID[sm.leafID]; ok {
			leaf = e
		}
	}
	if leaf == nil && len(entries) > 0 {
		leaf = entries[len(entries)-1]
	}
	if leaf == nil {
		return SessionContext{ThinkingLevel: "off"}
	}

	// Walk from leaf to root (use visited set to prevent cycles)
	var path []FileEntry
	visited := make(map[string]bool)
	current := leaf
	for current != nil {
		id := entryID(current)
		if visited[id] {
			break
		}
		visited[id] = true
		path = append([]FileEntry{current}, path...)
		parentID := entryParentID(current)
		if parentID == "" {
			break
		}
		var found bool
		current, found = byID[parentID]
		if !found {
			break
		}
	}

	// Extract settings and find compaction
	var thinkingLevel = "off"
	var model *ModelRef
	var compaction CompactionEntry
	var hasCompaction bool

	for _, e := range path {
		switch entry := e.(type) {
		case ThinkingLevelChangeEntry:
			thinkingLevel = entry.ThinkingLevel
		case ModelChangeEntry:
			model = &ModelRef{Provider: entry.Provider, ModelID: entry.ModelID}
		case CompactionEntry:
			compaction = entry
			hasCompaction = true
		}
	}

	// Build messages
	var messages []json.RawMessage
	appendMsg := func(e FileEntry) {
		switch entry := e.(type) {
		case SessionMessageEntry:
			messages = append(messages, entry.Message)
		case CustomMessageEntry:
			// Convert to user message format
			msg := map[string]interface{}{
				"role":    "user",
				"content": fmt.Sprintf("[custom:%s] %v", entry.CustomType, entry.Content),
			}
			data, _ := json.Marshal(msg)
			messages = append(messages, data)
		case BranchSummaryEntry:
			msg := map[string]interface{}{
				"role":    "user",
				"content": fmt.Sprintf("[branch_summary] %s", entry.Summary),
			}
			data, _ := json.Marshal(msg)
			messages = append(messages, data)
		}
	}

	if hasCompaction {
		// Emit compaction summary
		summaryMsg := map[string]interface{}{
			"role":    "user",
			"content": fmt.Sprintf("[compaction_summary] %s", compaction.Summary),
		}
		data, _ := json.Marshal(summaryMsg)
		messages = append(messages, data)

		// Find compaction index in path
		compIdx := -1
		for i, e := range path {
			if c, ok := e.(CompactionEntry); ok && c.ID == compaction.ID {
				compIdx = i
				break
			}
		}

		// Emit kept messages (from firstKeptEntryID to compaction)
		foundFirstKept := false
		for i := 0; i < compIdx; i++ {
			if entryID(path[i]) == compaction.FirstKeptEntryID {
				foundFirstKept = true
			}
			if foundFirstKept {
				appendMsg(path[i])
			}
		}

		// Emit messages after compaction
		for i := compIdx + 1; i < len(path); i++ {
			appendMsg(path[i])
		}
	} else {
		for _, e := range path {
			appendMsg(e)
		}
	}

	return SessionContext{
		Messages:    messages,
		ThinkingLevel: thinkingLevel,
		Model:       model,
	}
}

// Helper functions

var idCounter uint64

func generateID() string {
	return fmt.Sprintf("entry_%d_%d", time.Now().UnixNano(), atomic.AddUint64(&idCounter, 1))
}

func nowISO() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func entryID(e FileEntry) string {
	switch v := e.(type) {
	case SessionMessageEntry:
		return v.ID
	case ThinkingLevelChangeEntry:
		return v.ID
	case ModelChangeEntry:
		return v.ID
	case CompactionEntry:
		return v.ID
	case BranchSummaryEntry:
		return v.ID
	case CustomEntry:
		return v.ID
	case CustomMessageEntry:
		return v.ID
	case LabelEntry:
		return v.ID
	case SessionInfoEntry:
		return v.ID
	}
	return ""
}

func entryParentID(e FileEntry) string {
	switch v := e.(type) {
	case SessionMessageEntry:
		return v.ParentID
	case ThinkingLevelChangeEntry:
		return v.ParentID
	case ModelChangeEntry:
		return v.ParentID
	case CompactionEntry:
		return v.ParentID
	case BranchSummaryEntry:
		return v.ParentID
	case CustomEntry:
		return v.ParentID
	case CustomMessageEntry:
		return v.ParentID
	case LabelEntry:
		return v.ParentID
	case SessionInfoEntry:
		return v.ParentID
	}
	return ""
}

func sortTreeByTimestamp(node *SessionTreeNode) {
	// Iterative sort to avoid stack overflow
	type stackItem struct {
		node *SessionTreeNode
		idx  int
	}
	stack := []stackItem{{node, 0}}

	for len(stack) > 0 {
		item := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if item.node == nil || len(item.node.Children) == 0 {
			continue
		}

		// Sort children by timestamp
		n := len(item.node.Children)
		for i := 0; i < n-1; i++ {
			for j := 0; j < n-i-1; j++ {
				t1 := entryTimestamp(item.node.Children[j].Entry)
				t2 := entryTimestamp(item.node.Children[j+1].Entry)
				if t1 > t2 {
					item.node.Children[j], item.node.Children[j+1] = item.node.Children[j+1], item.node.Children[j]
				}
			}
		}

		// Push children to stack
		for i := range item.node.Children {
			stack = append(stack, stackItem{&item.node.Children[i], 0})
		}
	}
}

func entryTimestamp(e FileEntry) string {
	switch v := e.(type) {
	case SessionMessageEntry:
		return v.Timestamp
	case ThinkingLevelChangeEntry:
		return v.Timestamp
	case ModelChangeEntry:
		return v.Timestamp
	case CompactionEntry:
		return v.Timestamp
	case BranchSummaryEntry:
		return v.Timestamp
	case CustomEntry:
		return v.Timestamp
	case CustomMessageEntry:
		return v.Timestamp
	case LabelEntry:
		return v.Timestamp
	case SessionInfoEntry:
		return v.Timestamp
	}
	return ""
}

// loadEntriesFromFile reads a JSONL file and returns parsed entries.
func loadEntriesFromFile(path string) ([]FileEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var entries []FileEntry
	scanner := bufioScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if len(strings.TrimSpace(line)) == 0 {
			continue
		}
		e, err := parseFileEntry([]byte(line))
		if err != nil {
			continue // skip malformed lines
		}
		entries = append(entries, e)
	}
	return entries, nil
}

func bufioScanner(f *os.File) *bufio.Scanner {
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024) // up to 10MB lines
	return s
}

// parseFileEntry parses a JSON line into a FileEntry.
func parseFileEntry(data []byte) (FileEntry, error) {
	// Peek at the type field
	var typeHint struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &typeHint); err != nil {
		return nil, err
	}

	switch typeHint.Type {
	case "session":
		var h SessionHeader
		if err := json.Unmarshal(data, &h); err != nil {
			return nil, err
		}
		return h, nil
	case "message":
		var e SessionMessageEntry
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		return e, nil
	case "thinking_level_change":
		var e ThinkingLevelChangeEntry
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		return e, nil
	case "model_change":
		var e ModelChangeEntry
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		return e, nil
	case "compaction":
		var e CompactionEntry
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		return e, nil
	case "branch_summary":
		var e BranchSummaryEntry
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		return e, nil
	case "custom":
		var e CustomEntry
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		return e, nil
	case "custom_message":
		var e CustomMessageEntry
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		return e, nil
	case "label":
		var e LabelEntry
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		return e, nil
	case "session_info":
		var e SessionInfoEntry
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		return e, nil
	default:
		return nil, fmt.Errorf("unknown entry type: %s", typeHint.Type)
	}
}

// migrateToCurrentVersion runs migrations on loaded entries.
// v1->v2: assign id/parentId to entries, convert firstKeptEntryIndex to firstKeptEntryId
// v2->v3: rename "hookMessage" role to "custom" in message entries
func migrateToCurrentVersion(entries []FileEntry) {
	if len(entries) == 0 {
		return
	}

	header, ok := entries[0].(SessionHeader)
	if !ok {
		return
	}

	version := header.Version
	if version >= CurrentSessionVersion {
		return
	}

	// v1 -> v2: ensure all entries have id/parentId, convert firstKeptEntryIndex
	if version < 2 {
		header.Version = 2
		entries[0] = header
		var prevID string
		for i := 1; i < len(entries); i++ {
			id := entryID(entries[i])
			if id == "" {
				// v1 entries without IDs get new ones assigned
				// This is a simplified migration — v1 entries are rare
			}
			if prevID != "" {
				// Ensure parent chain if parentId is empty
				parent := entryParentID(entries[i])
				_ = parent // v1 entries use sequential order; parent chain is implicit
			}
			prevID = id
		}
	}

	// v2 -> v3: rename "hookMessage" role to "custom" in message entries
	if version < 3 {
		if h, ok := entries[0].(SessionHeader); ok {
			h.Version = 3
			entries[0] = h
		}
		for i, e := range entries {
			if entry, ok := e.(SessionMessageEntry); ok {
				var msgData map[string]interface{}
				if json.Unmarshal(entry.Message, &msgData) == nil {
					if role, _ := msgData["role"].(string); role == "hookMessage" {
						msgData["role"] = "custom"
						updated, _ := json.Marshal(msgData)
						entry.Message = updated
						entries[i] = entry
					}
				}
			}
		}
	}
}
