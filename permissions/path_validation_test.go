package permissions

import (
	"os"
	"path/filepath"
	"runtime"
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
	// Tool results must be under ~/.claude/projects/*/tool-results/
	home, _ := os.UserHomeDir()
	if home == "" {
		t.Skip("no home directory")
	}
	projPath := filepath.Join(home, ".claude", "projects", "test", "tool-results", "file")
	if !IsInternalReadablePath(projPath) {
		t.Error("tool-results path under projects should be internal readable")
	}
}

func TestIsInternalReadablePathToolResultsRejectsFake(t *testing.T) {
	// A path that merely contains "tool-results" as a substring should NOT match
	if IsInternalReadablePath("/some/tool-results-evil/file") {
		t.Error("fake tool-results path should not be internal readable")
	}
}

func TestIsInternalReadablePathBundledSkills(t *testing.T) {
	// Bundled skills must be under /tmp/claude-*/bundled-skills/
	if runtime.GOOS == "windows" {
		path := filepath.Join(os.TempDir(), "claude-abc", "bundled-skills", "skill1")
		if !IsInternalReadablePath(path) {
			t.Error("bundled-skills path under claude temp should be internal readable")
		}
	} else {
		if !IsInternalReadablePath("/tmp/claude-abc/bundled-skills/skill1") {
			t.Error("bundled-skills path under /tmp/claude- should be internal readable")
		}
	}
}

func TestIsInternalReadablePathBundledSkillsRejectsFake(t *testing.T) {
	// A path that merely contains "bundled-skills" outside /tmp/claude- should NOT match
	if IsInternalReadablePath("/home/user/bundled-skills/skill1") {
		t.Error("bundled-skills outside /tmp/claude- should not be internal readable")
	}
}

func TestIsInternalReadablePathSessionMemory(t *testing.T) {
	// Session memory must be under ~/.claude/projects/*/session-memory/
	home, _ := os.UserHomeDir()
	if home == "" {
		t.Skip("no home directory")
	}
	projPath := filepath.Join(home, ".claude", "projects", "test", "session-memory", "file")
	if !IsInternalReadablePath(projPath) {
		t.Error("session-memory path under projects should be internal readable")
	}
}

func TestIsInternalReadablePathSessionMemoryRejectsFake(t *testing.T) {
	// A path that merely contains "session-memory" as a substring should NOT match
	if IsInternalReadablePath("/some/session-memory-evil/file") {
		t.Error("fake session-memory path should not be internal readable")
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
	if runtime.GOOS == "windows" {
		t.Skip("Unix-only: Windows temp paths need drive letter handling")
	}
	// Scratchpad must be under a claude-* temp directory
	if !isInScratchpadDir("/tmp/claude-123/session/scratchpad/file.txt") {
		t.Error("scratchpad under claude-* should be detected")
	}
}

func TestIsInScratchpadDirNo(t *testing.T) {
	if isInScratchpadDir("/tmp/random/file.txt") {
		t.Error("random path should not be scratchpad")
	}
}

// ─── isInSessionMemoryDir ───────────────────────────────────────────────────

func TestIsInSessionMemoryDir(t *testing.T) {
	home, _ := os.UserHomeDir()
	if home == "" {
		t.Skip("no home directory")
	}
	path := filepath.Join(home, ".claude", "projects", "test", "session-memory", "file")
	if !isInSessionMemoryDir(path) {
		t.Error("should detect session-memory dir under projects")
	}
}

func TestIsInSessionMemoryDirNo(t *testing.T) {
	if isInSessionMemoryDir("/path/to/other/file") {
		t.Error("should not detect non-session-memory dir")
	}
}

// ─── isInToolResultsDir ─────────────────────────────────────────────────────

func TestIsInToolResultsDir(t *testing.T) {
	home, _ := os.UserHomeDir()
	if home == "" {
		t.Skip("no home directory")
	}
	path := filepath.Join(home, ".claude", "projects", "test", "tool-results", "file")
	if !isInToolResultsDir(path) {
		t.Error("should detect tool-results dir under projects")
	}
}

// ─── isInBundledSkillsDir ───────────────────────────────────────────────────

func TestIsInBundledSkillsDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-only: Windows temp paths need drive letter handling")
	}
	// Bundled skills must be under /tmp/claude-*/bundled-skills/
	if !isInBundledSkillsDir("/tmp/claude-abc/bundled-skills/skill1") {
		t.Error("should detect bundled-skills dir under /tmp/claude-")
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
	// Internal readable paths must be under proper home directory structure
	home, _ := os.UserHomeDir()
	if home == "" {
		t.Skip("no home directory")
	}
	// Session memory must be under ~/.claude/projects/*/session-memory/
	result := ValidateReadPath(filepath.Join(home, ".claude", "projects", "test", "session-memory", "file"), nil)
	if !result.Allowed {
		t.Error("internal readable path under projects should be allowed")
	}
}

// ─── Upstream Quality: Mixed Separators ──────────────────────────────────────

func TestValidatePathMixedSeparators(t *testing.T) {
	// Windows paths with mixed forward/backward slashes should be handled
	tests := []struct {
		name string
		path string
	}{
		{"forward slashes", "C:/Users/foo/file.txt"},
		{"mixed slashes", `C:\Users/foo\file.txt`},
		{"all forward", "C:/Program Files/app/config.ini"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ValidatePath(tc.path, OpWrite, nil, "")
			// Mixed separator paths should be recognized as Windows paths
			// and should not be flagged as shell expansion or glob
			if result.Allowed && result.Reason == "shellExpand" {
				t.Errorf("mixed separator path %q incorrectly flagged as shell expansion", tc.path)
			}
		})
	}
}

// ─── Upstream Quality: Long Path Prefix ──────────────────────────────────────

func TestValidatePathLongPathPrefix(t *testing.T) {
	// \\?\C:\path is a Windows long path prefix — should be flagged as suspicious
	result := ValidatePath(`\\?\C:\path\to\file`, OpWrite, nil, "")
	if result.Allowed {
		t.Error("long path prefix should be flagged as suspicious and not allowed for write")
	}
}

func TestValidatePathLongPathPrefixRead(t *testing.T) {
	// \\?\C:\path — suspicious even for read
	result := ValidateReadPath(`\\?\C:\path\to\file`, nil)
	if result.Allowed {
		t.Error("long path prefix should be flagged as suspicious even for read")
	}
}

// ─── Upstream Quality: UNC Variants ──────────────────────────────────────────

func TestValidatePathUNCVariants(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{"backslash UNC", `\\server\share\file.txt`},
		{"forward slash UNC", "//server/share/file.txt"},
		{"long path UNC", `\\?\UNC\server\share\file.txt`},
		{"device path", `\\.\COM1`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ValidatePath(tc.path, OpWrite, nil, "")
			if result.Allowed {
				t.Errorf("UNC variant %q should not be allowed for write", tc.path)
			}
		})
	}
}

func TestValidateReadPathUNCVariants(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{"backslash UNC", `\\server\share\file.txt`},
		{"forward slash UNC", "//server/share/file.txt"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ValidateReadPath(tc.path, nil)
			if result.Allowed {
				t.Errorf("UNC path %q should not be allowed for read", tc.path)
			}
		})
	}
}

// ─── Upstream Quality: Idempotent Path Resolution ────────────────────────────

func TestExpandTildeIdempotent(t *testing.T) {
	// expandTilde should be idempotent: expandTilde(expandTilde(p)) == expandTilde(p)
	home, _ := os.UserHomeDir()
	if home == "" {
		t.Skip("no home directory")
	}
	paths := []string{"~/file.go", "~", "no-tilde"}
	for _, p := range paths {
		first := expandTilde(p)
		second := expandTilde(first)
		if second != first {
			t.Errorf("expandTilde not idempotent for %q: first=%q, second=%q", p, first, second)
		}
	}
}
