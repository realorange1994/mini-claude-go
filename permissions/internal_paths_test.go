package permissions

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// ─── Tests not covered by path_validation_test.go ────────────────────────────

// ─── isLaunchConfig ──────────────────────────────────────────────────────────

func TestIsLaunchConfigMatch(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-only: Windows requires absolute paths with drive letters")
	}
	if !isLaunchConfig("/project/.claude/launch.json", "/project") {
		t.Error("launch.json in project dir should match")
	}
}

func TestIsLaunchConfigWrongDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-only: Windows requires absolute paths with drive letters")
	}
	if isLaunchConfig("/other/.claude/launch.json", "/project") {
		t.Error("launch.json in different dir should not match")
	}
}

func TestIsLaunchConfigEmptyCWD(t *testing.T) {
	if isLaunchConfig("/project/.claude/launch.json", "") {
		t.Error("empty cwd should not match")
	}
}

// ─── isInJobsDir ─────────────────────────────────────────────────────────────

func TestIsInJobsDir(t *testing.T) {
	home := homeDir()
	if home == "" {
		t.Skip("no home directory")
	}
	path := filepath.Join(home, ".claude", "jobs", "job-1")
	if !isInJobsDir(path) {
		t.Error("should detect jobs dir")
	}
}

func TestIsInJobsDirFalse(t *testing.T) {
	if isInJobsDir("/home/user/documents") {
		t.Error("random path should not be jobs dir")
	}
}

// ─── isAgentMemoryPath ───────────────────────────────────────────────────────

func TestIsAgentMemoryPath(t *testing.T) {
	home := homeDir()
	if home == "" {
		t.Skip("no home directory")
	}
	path := filepath.Join(home, ".claude", "projects", "abc", "agent-memory", "mem.md")
	if !isAgentMemoryPath(path) {
		t.Error("should detect agent memory path")
	}
}

func TestIsAgentMemoryPathFalse(t *testing.T) {
	if isAgentMemoryPath("/home/user/documents/file.md") {
		t.Error("random path should not be agent memory")
	}
}

// ─── isInAutoMemoryDir ───────────────────────────────────────────────────────

func TestIsInAutoMemoryDir(t *testing.T) {
	home := homeDir()
	if home == "" {
		t.Skip("no home directory")
	}
	path := filepath.Join(home, ".claude", "auto-memory", "notes.md")
	if !isInAutoMemoryDir(path) {
		t.Error("should detect auto memory dir")
	}
}

func TestIsInAutoMemoryDirFalse(t *testing.T) {
	home := homeDir()
	if home == "" {
		t.Skip("no home directory")
	}
	path := filepath.Join(home, "auto-memory", "notes.md")
	if isInAutoMemoryDir(path) {
		t.Error("auto memory outside .claude should not match")
	}
}

// ─── isInProjectTempDir ──────────────────────────────────────────────────────

func TestIsInProjectTempDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		path := filepath.Join(os.TempDir(), "claude-123", "session", "data")
		if !isInProjectTempDir(path) {
			t.Error("should detect project temp dir on Windows")
		}
	} else {
		if !isInProjectTempDir("/tmp/claude-123/session") {
			t.Error("should detect project temp dir on Unix")
		}
	}
}

func TestIsInProjectTempDirFalse(t *testing.T) {
	if isInProjectTempDir("/home/user/documents") {
		t.Error("random path should not be project temp dir")
	}
}

// ─── isInTasksDir ────────────────────────────────────────────────────────────

func TestIsInTasksDir(t *testing.T) {
	home := homeDir()
	if home == "" {
		t.Skip("no home directory")
	}
	path := filepath.Join(home, ".claude", "tasks", "task.json")
	if !isInTasksDir(path) {
		t.Error("should detect tasks dir")
	}
}

func TestIsInTasksDirFalse(t *testing.T) {
	home := homeDir()
	if home == "" {
		t.Skip("no home directory")
	}
	path := filepath.Join(home, "tasks", "task.json")
	if isInTasksDir(path) {
		t.Error("tasks outside .claude should not match")
	}
}

// ─── isInTeamsDir ────────────────────────────────────────────────────────────

func TestIsInTeamsDir(t *testing.T) {
	home := homeDir()
	if home == "" {
		t.Skip("no home directory")
	}
	path := filepath.Join(home, ".claude", "teams", "team.json")
	if !isInTeamsDir(path) {
		t.Error("should detect teams dir")
	}
}

func TestIsInTeamsDirFalse(t *testing.T) {
	home := homeDir()
	if home == "" {
		t.Skip("no home directory")
	}
	path := filepath.Join(home, "teams", "team.json")
	if isInTeamsDir(path) {
		t.Error("teams outside .claude should not match")
	}
}

// ─── isInProjectsDir ─────────────────────────────────────────────────────────

func TestIsInProjectsDirFalse(t *testing.T) {
	if isInProjectsDir("/home/user/documents") {
		t.Error("random path should not be projects dir")
	}
}

// ─── isPlanFile wrong dir ────────────────────────────────────────────────────

func TestIsPlanFileWrongDir(t *testing.T) {
	home := homeDir()
	if home == "" {
		t.Skip("no home directory")
	}
	path := filepath.Join(home, "plans", "my-plan.md")
	if isPlanFile(path) {
		t.Error("plan file outside .claude/plans should not match")
	}
}

// ─── IsInternalEditablePath edge cases ───────────────────────────────────────

func TestIsInternalEditablePathJobsDir(t *testing.T) {
	home := homeDir()
	if home == "" {
		t.Skip("no home directory")
	}
	path := filepath.Join(home, ".claude", "jobs", "job-123", "template.json")
	if !IsInternalEditablePath(path, "") {
		t.Error("jobs dir path should be internal editable")
	}
}

func TestIsInternalEditablePathAgentMemory(t *testing.T) {
	home := homeDir()
	if home == "" {
		t.Skip("no home directory")
	}
	path := filepath.Join(home, ".claude", "projects", "test", "agent-memory", "memory.md")
	if !IsInternalEditablePath(path, "") {
		t.Error("agent memory path should be internal editable")
	}
}

func TestIsInternalEditablePathAutoMemory(t *testing.T) {
	home := homeDir()
	if home == "" {
		t.Skip("no home directory")
	}
	path := filepath.Join(home, ".claude", "auto-memory", "notes.md")
	if !IsInternalEditablePath(path, "") {
		t.Error("auto memory path should be internal editable")
	}
}

func TestIsInternalEditablePathLaunchConfig(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-only: Windows requires absolute paths with drive letters")
	}
	cwd := "/project"
	path := filepath.Join(cwd, ".claude", "launch.json")
	if !IsInternalEditablePath(path, cwd) {
		t.Error("launch.json in project dir should be internal editable")
	}
}

// ─── IsInternalReadablePath edge cases ───────────────────────────────────────

func TestIsInternalReadablePathTeamsDir(t *testing.T) {
	home := homeDir()
	if home == "" {
		t.Skip("no home directory")
	}
	path := filepath.Join(home, ".claude", "teams", "team.json")
	if !IsInternalReadablePath(path) {
		t.Error("teams dir path should be internal readable")
	}
}

func TestIsInternalReadablePathTasksDir(t *testing.T) {
	home := homeDir()
	if home == "" {
		t.Skip("no home directory")
	}
	path := filepath.Join(home, ".claude", "tasks", "task.json")
	if !IsInternalReadablePath(path) {
		t.Error("tasks dir path should be internal readable")
	}
}

// ─── homeDir env precedence ──────────────────────────────────────────────────

func TestHomeDirUSERPROFILE(t *testing.T) {
	original := os.Getenv("USERPROFILE")
	os.Setenv("USERPROFILE", "/test/home")
	defer os.Setenv("USERPROFILE", original)

	home := homeDir()
	if home != "/test/home" {
		t.Errorf("expected /test/home, got %q", home)
	}
}

// ─── Path traversal attack prevention (security edge cases) ──────────────────

func TestHasPathPrefixExactMatch(t *testing.T) {
	if !hasPathPrefix("/home/user/.claude/projects", "/home/user/.claude/projects") {
		t.Error("exact match should be accepted")
	}
}

func TestHasPathPrefixWithSeparator(t *testing.T) {
	if !hasPathPrefix("/home/user/.claude/projects/test", "/home/user/.claude/projects") {
		t.Error("path with separator prefix should be accepted")
	}
}

func TestHasPathPrefixSubstringAttack(t *testing.T) {
	// my-.claude/projects/ should NOT match .claude/projects
	if hasPathPrefix("/home/user/my-.claude/projects/evil", "/home/user/.claude/projects") {
		t.Error("substring prefix attack should be rejected")
	}
}

func TestHasPathPrefixDifferentBase(t *testing.T) {
	if hasPathPrefix("/other/path/file", "/home/user/.claude/projects") {
		t.Error("different base should be rejected")
	}
}

func TestHasPathComponentAsComponent(t *testing.T) {
	if !hasPathComponent("/home/user/.claude/projects/test/session-memory/file", "session-memory") {
		t.Error("session-memory as path component should match")
	}
}

func TestHasPathComponentSubstringAttack(t *testing.T) {
	// my-session-memory-evil should NOT match session-memory
	if hasPathComponent("/home/user/.claude/projects/test/my-session-memory-evil/file", "session-memory") {
		t.Error("session-memory substring in other component should not match")
	}
}

func TestHasPathComponentAtStart(t *testing.T) {
	if !hasPathComponent("session-memory/file", "session-memory") {
		t.Error("component at start should match")
	}
}

func TestHasPathComponentAtEnd(t *testing.T) {
	if !hasPathComponent("/path/to/session-memory", "session-memory") {
		t.Error("component at end should match")
	}
}

func TestHasPathComponentExact(t *testing.T) {
	if !hasPathComponent("session-memory", "session-memory") {
		t.Error("exact match should match")
	}
}

// ─── ADS colon checks ────────────────────────────────────────────────────────

func TestHasSuspiciousColonDriveLetter(t *testing.T) {
	// Drive letter colon should be allowed
	if hasSuspiciousColon("C:\\Users\\test\\file.txt") {
		t.Error("drive letter colon should be allowed")
	}
}

func TestHasSuspiciousColonADS(t *testing.T) {
	// ADS colon should be rejected
	if !hasSuspiciousColon("C:\\Users\\test\\file.txt:Zone.Identifier") {
		t.Error("ADS colon after drive letter should be detected")
	}
}

func TestHasSuspiciousColonUnixHidden(t *testing.T) {
	if !hasSuspiciousColon("/home/user/.bashrc:hidden") {
		t.Error("hidden ADS colon should be detected on Unix")
	}
}

func TestHasSuspiciousColonNone(t *testing.T) {
	if hasSuspiciousColon("/home/user/file.txt") {
		t.Error("no colon should not be detected")
	}
}

func TestInternalEditablePathRejectsADS(t *testing.T) {
	home, _ := os.UserHomeDir()
	if home == "" {
		t.Skip("no home directory")
	}
	// A valid plan file path with ADS colon should be rejected
	adsPath := filepath.Join(home, ".claude", "plans", "test.md:Zone.Identifier")
	if IsInternalEditablePath(adsPath, "") {
		t.Error("plan file with ADS should be rejected")
	}
}

func TestInternalReadablePathRejectsADS(t *testing.T) {
	home, _ := os.UserHomeDir()
	if home == "" {
		t.Skip("no home directory")
	}
	// A valid session memory path with ADS colon should be rejected
	adsPath := filepath.Join(home, ".claude", "projects", "test", "session-memory", "file:Zone.Identifier")
	if IsInternalReadablePath(adsPath) {
		t.Error("session memory file with ADS should be rejected")
	}
}

// ─── Symlink escape checks ───────────────────────────────────────────────────

func TestSymlinkEscapeWithinParent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-only: Windows path handling differs")
	}
	// Create a temp dir without symlinks — should not be an escape
	tmp := t.TempDir()
	if isSymlinkEscape(tmp, tmp) {
		t.Error("path within parent should not be escape")
	}
}

func TestSymlinkEscapeNonexistentPath(t *testing.T) {
	// A nonexistent path that can't be resolved is treated as potential escape
	if !isSymlinkEscape("/nonexistent/path/that/does/not/exist", "/nonexistent") {
		t.Error("nonexistent path should be treated as potential escape")
	}
}

// ─── Path traversal rejection tests ──────────────────────────────────────────

func TestAgentMemoryPathRejectsFakeProjects(t *testing.T) {
	home, _ := os.UserHomeDir()
	if home == "" {
		t.Skip("no home directory")
	}
	// Path outside ~/.claude/projects that contains "agent-memory"
	fakePath := filepath.Join(home, "documents", "agent-memory", "file.md")
	if isAgentMemoryPath(fakePath) {
		t.Error("agent-memory outside projects dir should be rejected")
	}
}

func TestSessionMemoryRejectsSubstring(t *testing.T) {
	home, _ := os.UserHomeDir()
	if home == "" {
		t.Skip("no home directory")
	}
	// my-session-memory-backup/ should not match
	fakePath := filepath.Join(home, ".claude", "projects", "test", "my-session-memory-backup", "file")
	if isInSessionMemoryDir(fakePath) {
		t.Error("my-session-memory-backup should not match session-memory")
	}
}

func TestToolResultsRejectsSubstring(t *testing.T) {
	home, _ := os.UserHomeDir()
	if home == "" {
		t.Skip("no home directory")
	}
	// my-tool-results/ should not match
	fakePath := filepath.Join(home, ".claude", "projects", "test", "my-tool-results", "file")
	if isInToolResultsDir(fakePath) {
		t.Error("my-tool-results should not match tool-results")
	}
}

func TestBundledSkillsRejectsOutsideTemp(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-only")
	}
	// /home/user/bundled-skills/ should not match
	if isInBundledSkillsDir("/home/user/bundled-skills/skill1") {
		t.Error("bundled-skills outside /tmp/claude- should be rejected")
	}
}

func TestProjectTempRejectsFakeClaude(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-only")
	}
	// /tmp/my-claude-evil/ does not start with /tmp/claude-
	if isInProjectTempDir("/tmp/my-claude-evil/file") {
		t.Error("/tmp/my-claude-evil should not match /tmp/claude-*")
	}
}
