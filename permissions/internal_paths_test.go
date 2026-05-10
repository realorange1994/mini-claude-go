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
