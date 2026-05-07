package skills

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// SkillTracker tracks which skills have been shown/read/used across agent turns.
// This enables a progressive discovery system where newly available skills are
// announced each turn until the model has seen them.
type SkillTracker struct {
	mu          sync.Mutex
	shownSkills map[string]struct{}
	readSkills  map[string]time.Time // name → time when read_skill was called
	usedSkills  map[string]struct{}
}

// NewSkillTracker creates a new empty skill tracker.
func NewSkillTracker() *SkillTracker {
	return &SkillTracker{
		shownSkills: make(map[string]struct{}),
		readSkills:  make(map[string]time.Time),
		usedSkills:  make(map[string]struct{}),
	}
}

// IsNewSkill returns true if the skill has NOT been shown in system prompt yet.
func (t *SkillTracker) IsNewSkill(name string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	_, ok := t.shownSkills[name]
	return !ok
}

// MarkShown marks a skill as announced in system prompt.
func (t *SkillTracker) MarkShown(name string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.shownSkills[name] = struct{}{}
}

// MarkRead marks a skill as read by the model (read_skill tool called).
// Records the current timestamp for time-based ordering during post-compact recovery.
func (t *SkillTracker) MarkRead(name string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.readSkills[name] = time.Now()
}

// MarkUsed marks a skill as used (model performed actions after reading it).
func (t *SkillTracker) MarkUsed(name string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.usedSkills[name] = struct{}{}
}

// WasRead checks if a specific skill was read.
func (t *SkillTracker) WasRead(name string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	_, ok := t.readSkills[name]
	return ok
}

// WasUsed checks if a specific skill was used.
func (t *SkillTracker) WasUsed(name string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	_, ok := t.usedSkills[name]
	return ok
}

// GetUnsentSkills returns SkillInfo for skills not yet shown in system prompt,
// plus always-on skills (which are always included).
func (t *SkillTracker) GetUnsentSkills(allSkills []SkillInfo) []SkillInfo {
	t.mu.Lock()
	defer t.mu.Unlock()

	var result []SkillInfo
	for _, s := range allSkills {
		if s.Always {
			result = append(result, s)
			continue
		}
		if _, shown := t.shownSkills[s.Name]; !shown {
			result = append(result, s)
		}
	}
	return result
}

// GenerateDiscoveryReminder returns a hint text if there are unread skills.
func (t *SkillTracker) GenerateDiscoveryReminder(allSkills []SkillInfo) string {
	t.mu.Lock()
	defer t.mu.Unlock()

	unsentCount := 0
	for _, s := range allSkills {
		if s.Always {
			continue
		}
		if _, shown := t.shownSkills[s.Name]; !shown {
			unsentCount++
		}
	}

	if unsentCount == 0 {
		return ""
	}

	return fmt.Sprintf("You have %d unread skill(s). Use search_skills to find skills by topic, or list_skills to see all available skills.\nUse read_skill to load a skill's full instructions.", unsentCount)
}

// ReadCount returns the number of skills that have been read.
func (t *SkillTracker) ReadCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.readSkills)
}

// UsedCount returns the number of skills that have been used.
func (t *SkillTracker) UsedCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.usedSkills)
}

// GetReadSkillNames returns the names of skills that have been read,
// sorted by read time descending (most recently read first).
// Matches upstream's invokedAt-based ordering.
func (t *SkillTracker) GetReadSkillNames() []string {
	t.mu.Lock()
	defer t.mu.Unlock()

	type nameTime struct {
		name string
		time time.Time
	}
	entries := make([]nameTime, 0, len(t.readSkills))
	for name, t := range t.readSkills {
		entries = append(entries, nameTime{name, t})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].time.After(entries[j].time) // most recent first
	})

	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.name
	}
	return names
}

// ResetPostCompact resets the skill discovery state after compaction.
// This clears:
//   - shownSkills: system prompt will be rebuilt, all skills re-announced
//   - readSkills: skill content was injected via attachments; re-injecting
//     full skill listing is pure cache creation, so we reset read state too
//
// usedSkills is preserved because a skill being "used" (model performed
// actions after reading it) is a durable fact about the conversation.
func (t *SkillTracker) ResetPostCompact() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.shownSkills = make(map[string]struct{})
	t.readSkills = make(map[string]time.Time)
}

// RestoreReadSkills restores the readSkills map from persisted state.
// Used on resume to re-populate skill tracking from transcript attachments.
func (t *SkillTracker) RestoreReadSkills(skills map[string]time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for name, ts := range skills {
		t.readSkills[name] = ts
	}
}
