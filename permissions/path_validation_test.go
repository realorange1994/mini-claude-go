package permissions

import (
	"os"
	"path/filepath"
	"testing"
)

// ─── IsInternalEditablePath ─────────────────────────────────────────────────

func TestIsInternalEditablePathPlanFile(t *testing.T) {
	home, _ := os.UserHomeDir()
	if home == "" {
		t.Skip("no home directory")
	}
	planPath := filepath.Join(home, ".claude", "plans", "test.md")
	if !IsInternalEditablePath(planPath, "") {
		t.Error("plan file should be internal editable path")
	}
}

func TestIsInternalEditablePathNonPlan(t *testing.T) {
	if IsInternalEditablePath("/tmp/random.txt", "") {
		t.Error("random path should not be internal editable")
	}
}

// ─── IsInternalReadablePath ─────────────────────────────────────────────────

func TestIsInternalReadablePathPlanFile(t *testing.T) {
	home, _ := os.UserHomeDir()
	if home == "" {
		t.Skip("no home directory")
	}
	planPath := filepath.Join(home, ".claude", "plans", "test.md")
	if !IsInternalReadablePath(planPath) {
		t.Error("plan file should be internal readable path")
	}
}

func TestIsInternalReadablePathProjectsDir(t *testing.T) {
	home, _ := os.UserHomeDir()
	if home == "" {
		t.Skip("no home directory")
	}
	projPath := filepath.Join(home, ".claude", "projects", "test")
	if !IsInternalReadablePath(projPath) {
		t.Error("projects dir should be internal readable")
	}
}

func TestIsInternalReadablePathToolResults(t *testing.T) {
	// Tool results dir contains "tool-results" in path
	if !IsInternalReadablePath("/some/tool-results/file") {
		t.Error("tool-results path should be internal readable")
	}
}

func TestIsInternalReadablePathBundledSkills(t *testing.T) {
	if !IsInternalReadablePath("/tmp/bundled-skills/skill1") {
		t.Error("bundled-skills path should be internal readable")
	}
}

func TestIsInternalReadablePathSessionMemory(t *testing.T) {
	if !IsInternalReadablePath("/some/session-memory/file") {
		t.Error("session-memory path should be internal readable")
	}
}

func TestIsInternalReadablePathRandom(t *testing.T) {
	if IsInternalReadablePath("/tmp/random.txt") {
		t.Error("random path should not be internal readable")
	}
}

// ─── isPlanFile ─────────────────────────────────────────────────────────────

func TestIsPlanFile(t *testing.T) {
	home, _ := os.UserHomeDir()
	if home == "" {
		t.Skip("no home directory")
	}
	planPath := filepath.Join(home, ".claude", "plans", "test.md")
	if !isPlanFile(planPath) {
		t.Error("should be a plan file")
	}
}

func TestIsPlanFileNonMD(t *testing.T) {
	home, _ := os.UserHomeDir()
	if home == "" {
		t.Skip("no home directory")
	}
	planPath := filepath.Join(home, ".claude", "plans", "test.txt")
	if isPlanFile(planPath) {
		t.Error("non-.md file should not be a plan file")
	}
}

// ─── isInScratchpadDir ──────────────────────────────────────────────────────

func TestIsInScratchpadDir(t *testing.T) {
	tmp := os.TempDir()
	path := filepath.Join(tmp, "scratchpad", "file.txt")
	if !isInScratchpadDir(path) {
		t.Error("scratchpad path should be detected")
	}
}

func TestIsInScratchpadDirNo(t *testing.T) {
	if isInScratchpadDir("/tmp/random/file.txt") {
		t.Error("random path should not be scratchpad")
	}
}

// ─── isInSessionMemoryDir ───────────────────────────────────────────────────

func TestIsInSessionMemoryDir(t *testing.T) {
	if !isInSessionMemoryDir("/path/to/session-memory/file") {
		t.Error("should detect session-memory dir")
	}
}

func TestIsInSessionMemoryDirNo(t *testing.T) {
	if isInSessionMemoryDir("/path/to/other/file") {
		t.Error("should not detect non-session-memory dir")
	}
}

// ─── isInToolResultsDir ─────────────────────────────────────────────────────

func TestIsInToolResultsDir(t *testing.T) {
	if !isInToolResultsDir("/path/to/tool-results/file") {
		t.Error("should detect tool-results dir")
	}
}

// ─── isInBundledSkillsDir ───────────────────────────────────────────────────

func TestIsInBundledSkillsDir(t *testing.T) {
	if !isInBundledSkillsDir("/tmp/bundled-skills/skill1") {
		t.Error("should detect bundled-skills dir")
	}
}

// ─── homeDir ────────────────────────────────────────────────────────────────

func TestHomeDir(t *testing.T) {
	dir := homeDir()
	if dir == "" {
		t.Log("homeDir returned empty (no HOME or USERPROFILE)")
	}
}

// ─── resolvePath ────────────────────────────────────────────────────────────

func TestResolvePathAbs(t *testing.T) {
	absPath := filepath.Join(os.TempDir(), "file.go")
	result := resolvePath(absPath, "/home/user")
	if result != filepath.Clean(absPath) {
		t.Errorf("expected %q, got %q", filepath.Clean(absPath), result)
	}
}

func TestResolvePathRel(t *testing.T) {
	result := resolvePath("file.go", "/home/user/project")
	expected := filepath.Clean(filepath.FromSlash("/home/user/project/file.go"))
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

// ─── expandTilde ────────────────────────────────────────────────────────────

func TestExpandTilde(t *testing.T) {
	home, _ := os.UserHomeDir()
	if home == "" {
		t.Skip("no home directory")
	}
	result := expandTilde("~/file.go")
	expected := filepath.Join(home, "file.go")
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestExpandTildeNoTilde(t *testing.T) {
	result := expandTilde("file.go")
	if result != "file.go" {
		t.Errorf("expected 'file.go', got %q", result)
	}
}

func TestExpandTildeJustTilde(t *testing.T) {
	home, _ := os.UserHomeDir()
	if home == "" {
		t.Skip("no home directory")
	}
	result := expandTilde("~")
	if result != home {
		t.Errorf("expected %q, got %q", home, result)
	}
}

// ─── isUncPath ──────────────────────────────────────────────────────────────

func TestIsUncPathBackslash(t *testing.T) {
	if !isUncPath(`\\server\share`) {
		t.Error("UNC path with backslash should be detected")
	}
}

func TestIsUncPathSlash(t *testing.T) {
	if !isUncPath("//server/share") {
		t.Error("UNC path with forward slash should be detected")
	}
}

func TestIsUncPathNo(t *testing.T) {
	if isUncPath("/home/user/file") {
		t.Error("regular path should not be UNC")
	}
}

// ─── hasSuspiciousWindowsPathPattern ────────────────────────────────────────

func TestSuspiciousShortName(t *testing.T) {
	suspicious, _ := hasSuspiciousWindowsPathPattern("file~1.txt")
	if !suspicious {
		t.Error("8.3 short name should be suspicious")
	}
}

func TestSuspiciousLongPathPrefix(t *testing.T) {
	suspicious, _ := hasSuspiciousWindowsPathPattern(`\\?\C:\path`)
	if !suspicious {
		t.Error("long path prefix should be suspicious")
	}
}

func TestSuspiciousDOSDevice(t *testing.T) {
	suspicious, _ := hasSuspiciousWindowsPathPattern("file.CON")
	if !suspicious {
		t.Error("DOS device name extension should be suspicious")
	}
}

func TestSuspiciousUNC(t *testing.T) {
	suspicious, _ := hasSuspiciousWindowsPathPattern(`\\server\share`)
	if !suspicious {
		t.Error("UNC path should be suspicious")
	}
}

func TestSuspiciousNormalPath(t *testing.T) {
	suspicious, _ := hasSuspiciousWindowsPathPattern("/home/user/file.go")
	if suspicious {
		t.Error("normal path should not be suspicious")
	}
}

// ─── ValidatePath ───────────────────────────────────────────────────────────

func TestValidatePathUNC(t *testing.T) {
	result := ValidatePath(`\\server\share`, OpWrite, nil, "")
	if result.Allowed {
		t.Error("UNC path should not be allowed")
	}
	if result.Reason != "other" {
		t.Errorf("expected reason 'other', got %q", result.Reason)
	}
}

func TestValidatePathShellExpand(t *testing.T) {
	result := ValidatePath("$HOME/file", OpWrite, nil, "")
	if result.Allowed {
		t.Error("shell expansion should not be allowed")
	}
}

func TestValidatePathGlobWrite(t *testing.T) {
	result := ValidatePath("*.go", OpWrite, nil, "")
	if result.Allowed {
		t.Error("glob in write should not be allowed")
	}
}

func TestValidatePathGlobRead(t *testing.T) {
	// Glob is allowed for read operations
	result := ValidatePath("*.go", OpRead, nil, "")
	// Read operations don't block glob patterns
	_ = result
}

// ─── ValidateReadPath ───────────────────────────────────────────────────────

func TestValidateReadPathUNC(t *testing.T) {
	result := ValidateReadPath(`\\server\share`, nil)
	if result.Allowed {
		t.Error("UNC path should not be allowed for read")
	}
}

func TestValidateReadPathSuspicious(t *testing.T) {
	result := ValidateReadPath("file~1.txt", nil)
	if result.Allowed {
		t.Error("suspicious path should not be allowed")
	}
}

func TestValidateReadPathInternal(t *testing.T) {
	// Internal readable paths should be allowed
	result := ValidateReadPath("/some/session-memory/file", nil)
	if !result.Allowed {
		t.Error("internal readable path should be allowed")
	}
}
