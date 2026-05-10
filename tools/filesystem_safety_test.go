package tools

import (
	"testing"
)

// ─── isDangerousFilePath ─────────────────────────────────────────────────────

func TestIsDangerousFileGitconfig(t *testing.T) {
	dangerous, _ := isDangerousFilePath(".gitconfig")
	if !dangerous {
		t.Error(".gitconfig should be dangerous")
	}
}

func TestIsDangerousFileBashrc(t *testing.T) {
	dangerous, _ := isDangerousFilePath(".bashrc")
	if !dangerous {
		t.Error(".bashrc should be dangerous")
	}
}

func TestIsDangerousFileMCP(t *testing.T) {
	dangerous, _ := isDangerousFilePath(".mcp.json")
	if !dangerous {
		t.Error(".mcp.json should be dangerous")
	}
}

func TestIsDangerousFileNormal(t *testing.T) {
	dangerous, _ := isDangerousFilePath("main.go")
	if dangerous {
		t.Error("main.go should not be dangerous")
	}
}

func TestIsDangerousFileInGitDir(t *testing.T) {
	dangerous, _ := isDangerousFilePath(".git/config")
	if !dangerous {
		t.Error(".git/config should be dangerous")
	}
}

func TestIsDangerousFileInVscodeDir(t *testing.T) {
	dangerous, _ := isDangerousFilePath(".vscode/settings.json")
	if !dangerous {
		t.Error(".vscode/settings.json should be dangerous")
	}
}

func TestIsDangerousFileInClaudeDir(t *testing.T) {
	dangerous, _ := isDangerousFilePath(".claude/agents/foo.go")
	if !dangerous {
		t.Error(".claude/agents/foo.go should be dangerous")
	}
}

func TestIsDangerousFileClaudeWorktrees(t *testing.T) {
	// .claude/worktrees/ is allowed (structural path)
	dangerous, _ := isDangerousFilePath(".claude/worktrees/branch1/config")
	if dangerous {
		t.Error(".claude/worktrees should NOT be flagged as dangerous")
	}
}

func TestIsDangerousFileUNC(t *testing.T) {
	dangerous, _ := isDangerousFilePath(`\\server\share\file.txt`)
	if !dangerous {
		t.Error("UNC path should be dangerous")
	}
}

// ─── hasSuspiciousWindowsPathPattern ────────────────────────────────────────

func TestSuspiciousNTFSStream(t *testing.T) {
	if len("file.txt:Zone.Identifier") > 2 {
		suspicious, _ := hasSuspiciousWindowsPathPattern("file.txt:Zone.Identifier")
		if !suspicious {
			t.Error("NTFS ADS should be suspicious")
		}
	}
}

func TestSuspiciousShortName(t *testing.T) {
	suspicious, _ := hasSuspiciousWindowsPathPattern("CLAUDE~1")
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

func TestSuspiciousTrailingDots(t *testing.T) {
	suspicious, _ := hasSuspiciousWindowsPathPattern("file.txt...")
	if !suspicious {
		t.Error("trailing dots should be suspicious")
	}
}

func TestSuspiciousDOSDevice(t *testing.T) {
	suspicious, _ := hasSuspiciousWindowsPathPattern("settings.CON")
	if !suspicious {
		t.Error("DOS device name should be suspicious")
	}
}

func TestSuspiciousTripleDots(t *testing.T) {
	suspicious, _ := hasSuspiciousWindowsPathPattern(".../file.txt")
	if !suspicious {
		t.Error("triple dots should be suspicious")
	}
}

func TestSuspiciousUNC(t *testing.T) {
	suspicious, _ := hasSuspiciousWindowsPathPattern(`\\server\share`)
	if !suspicious {
		t.Error("UNC path should be suspicious")
	}
}

func TestSuspiciousNormalPath(t *testing.T) {
	suspicious, _ := hasSuspiciousWindowsPathPattern("/home/user/main.go")
	if suspicious {
		t.Error("normal path should not be suspicious")
	}
}

// ─── CheckPathSafetyForAutoEdit ─────────────────────────────────────────────

func TestCheckPathSafetySuspicious(t *testing.T) {
	result := CheckPathSafetyForAutoEdit(`\\?\C:\path\file.txt`)
	if result.Behavior != PermissionAsk {
		t.Errorf("suspicious path should result in PermissionAsk, got %v", result.Behavior)
	}
	if result.DecisionReason != "safetyCheck" {
		t.Errorf("reason should be safetyCheck, got %q", result.DecisionReason)
	}
}

func TestCheckPathSafetyClaudeConfig(t *testing.T) {
	result := CheckPathSafetyForAutoEdit(".claude/settings.json")
	if result.Behavior != PermissionAsk {
		t.Error("claude config should result in PermissionAsk")
	}
}

func TestCheckPathSafetyDangerousFile(t *testing.T) {
	result := CheckPathSafetyForAutoEdit(".bashrc")
	if result.Behavior != PermissionAsk {
		t.Error("dangerous file should result in PermissionAsk")
	}
}

func TestCheckPathSafetySafe(t *testing.T) {
	result := CheckPathSafetyForAutoEdit("main.go")
	if result.Behavior != PermissionPassthrough {
		t.Errorf("safe file should passthrough, got %v", result.Behavior)
	}
}

// ─── isClaudeConfigPath ─────────────────────────────────────────────────────

func TestIsClaudeConfigSettings(t *testing.T) {
	if !isClaudeConfigPath(".claude/settings.json") {
		t.Error("settings.json should be claude config")
	}
}

func TestIsClaudeConfigSettingsLocal(t *testing.T) {
	if !isClaudeConfigPath(".claude/settings.local.json") {
		t.Error("settings.local.json should be claude config")
	}
}

func TestIsClaudeConfigCommands(t *testing.T) {
	// isClaudeConfigPath checks for paths ending with the config path or containing /configPath
	if !isClaudeConfigPath("/project/.claude/commands/custom.go") {
		t.Error("commands dir should be claude config when in nested path")
	}
}

func TestIsClaudeConfigAgents(t *testing.T) {
	if !isClaudeConfigPath("/project/.claude/agents/agent1.go") {
		t.Error("agents dir should be claude config")
	}
}

func TestIsClaudeConfigSkills(t *testing.T) {
	// isClaudeConfigPath requires nested path for / prefix match
	if !isClaudeConfigPath("/project/.claude/skills/custom_skill.md") {
		t.Error("skills dir should be claude config when in nested path")
	}
}

func TestIsClaudeConfigNot(t *testing.T) {
	if isClaudeConfigPath("src/main.go") {
		t.Error("main.go should not be claude config")
	}
}

// ─── indexOf ─────────────────────────────────────────────────────────────────

func TestIndexOfFound(t *testing.T) {
	idx := indexOf([]string{"a", "b", "c"}, "b")
	if idx != 1 {
		t.Errorf("expected 1, got %d", idx)
	}
}

func TestIndexOfNotFound(t *testing.T) {
	idx := indexOf([]string{"a", "b", "c"}, "d")
	if idx != -1 {
		t.Errorf("expected -1, got %d", idx)
	}
}

func TestIndexOfEmpty(t *testing.T) {
	idx := indexOf([]string{}, "a")
	if idx != -1 {
		t.Errorf("expected -1 for empty slice, got %d", idx)
	}
}
