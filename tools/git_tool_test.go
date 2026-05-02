package tools

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

func TestBuildGitCommand_NewOperations(t *testing.T) {
	tests := []struct {
		name      string
		params    map[string]interface{}
		wantArgs  []string
		wantError bool
	}{
		// --- rm ---
		{
			name:     "rm basic",
			params:   map[string]interface{}{"operation": "rm", "files": []interface{}{"file.txt"}},
			wantArgs: []string{"rm", "file.txt"},
		},
		{
			name:     "rm with force",
			params:   map[string]interface{}{"operation": "rm", "files": []interface{}{"file.txt"}, "force": true},
			wantArgs: []string{"rm", "-f", "file.txt"},
		},
		{
			name:     "rm with staged",
			params:   map[string]interface{}{"operation": "rm", "files": []interface{}{"file.txt"}, "staged": true},
			wantArgs: []string{"rm", "--cached", "file.txt"},
		},
		{
			name:     "rm with force and staged",
			params:   map[string]interface{}{"operation": "rm", "files": []interface{}{"file.txt"}, "force": true, "staged": true},
			wantArgs: []string{"rm", "-f", "--cached", "file.txt"},
		},
		{
			name:      "rm missing files",
			params:    map[string]interface{}{"operation": "rm"},
			wantError: true,
		},

		// --- mv ---
		{
			name:     "mv basic",
			params:   map[string]interface{}{"operation": "mv", "source": "old.txt", "path": "new.txt"},
			wantArgs: []string{"mv", "old.txt", "new.txt"},
		},
		{
			name:     "mv with force",
			params:   map[string]interface{}{"operation": "mv", "source": "old.txt", "path": "new.txt", "force": true},
			wantArgs: []string{"mv", "-f", "old.txt", "new.txt"},
		},
		{
			name:      "mv missing source",
			params:    map[string]interface{}{"operation": "mv", "path": "new.txt"},
			wantError: true,
		},
		{
			name:      "mv missing dest",
			params:    map[string]interface{}{"operation": "mv", "source": "old.txt"},
			wantError: true,
		},

		// --- restore ---
		{
			name:     "restore basic (default .)",
			params:   map[string]interface{}{"operation": "restore"},
			wantArgs: []string{"restore", "."},
		},
		{
			name:     "restore specific file",
			params:   map[string]interface{}{"operation": "restore", "files": []interface{}{"file.txt"}},
			wantArgs: []string{"restore", "file.txt"},
		},
		{
			name:     "restore staged",
			params:   map[string]interface{}{"operation": "restore", "files": []interface{}{"file.txt"}, "staged": true},
			wantArgs: []string{"restore", "--staged", "file.txt"},
		},
		{
			name:     "restore force",
			params:   map[string]interface{}{"operation": "restore", "files": []interface{}{"file.txt"}, "force": true},
			wantArgs: []string{"restore", "--force", "file.txt"},
		},

		// --- switch ---
		{
			name:     "switch with branch",
			params:   map[string]interface{}{"operation": "switch", "branch": "feature"},
			wantArgs: []string{"switch", "feature"},
		},
		{
			name:     "switch with source",
			params:   map[string]interface{}{"operation": "switch", "source": "main"},
			wantArgs: []string{"switch", "main"},
		},
		{
			name:     "switch with force",
			params:   map[string]interface{}{"operation": "switch", "branch": "feature", "force": true},
			wantArgs: []string{"switch", "--force", "feature"},
		},
		{
			name:      "switch missing branch",
			params:    map[string]interface{}{"operation": "switch"},
			wantError: true,
		},

		// --- cherry-pick ---
		{
			name:     "cherry-pick basic",
			params:   map[string]interface{}{"operation": "cherry-pick", "target": "abc1234"},
			wantArgs: []string{"cherry-pick", "abc1234"},
		},
		{
			name:     "cherry-pick with mainline",
			params:   map[string]interface{}{"operation": "cherry-pick", "target": "abc1234", "mainline": float64(1)},
			wantArgs: []string{"cherry-pick", "-m", "1", "abc1234"},
		},
		{
			name:     "cherry-pick with no-commit",
			params:   map[string]interface{}{"operation": "cherry-pick", "target": "abc1234", "no_commit": true},
			wantArgs: []string{"cherry-pick", "--no-commit", "abc1234"},
		},
		{
			name:      "cherry-pick missing target",
			params:    map[string]interface{}{"operation": "cherry-pick"},
			wantError: true,
		},

		// --- revert ---
		{
			name:     "revert basic",
			params:   map[string]interface{}{"operation": "revert", "target": "abc1234"},
			wantArgs: []string{"revert", "abc1234"},
		},
		{
			name:     "revert with mainline",
			params:   map[string]interface{}{"operation": "revert", "target": "abc1234", "mainline": float64(1)},
			wantArgs: []string{"revert", "-m", "1", "abc1234"},
		},
		{
			name:     "revert with no-edit",
			params:   map[string]interface{}{"operation": "revert", "target": "abc1234", "no_edit": true},
			wantArgs: []string{"revert", "--no-edit", "abc1234"},
		},
		{
			name:      "revert missing target",
			params:    map[string]interface{}{"operation": "revert"},
			wantError: true,
		},

		// --- clean ---
		{
			name:     "clean basic",
			params:   map[string]interface{}{"operation": "clean"},
			wantArgs: []string{"clean"},
		},
		{
			name:     "clean with force",
			params:   map[string]interface{}{"operation": "clean", "force": true},
			wantArgs: []string{"clean", "-f"},
		},
		{
			name:     "clean dry-run",
			params:   map[string]interface{}{"operation": "clean", "dry_run": true},
			wantArgs: []string{"clean", "-n"},
		},
		{
			name:     "clean recursive",
			params:   map[string]interface{}{"operation": "clean", "recursive": true},
			wantArgs: []string{"clean", "-d"},
		},
		{
			name:     "clean force dry-run recursive",
			params:   map[string]interface{}{"operation": "clean", "force": true, "dry_run": true, "recursive": true},
			wantArgs: []string{"clean", "-f", "-n", "-d"},
		},

		// --- blame ---
		{
			name:     "blame basic",
			params:   map[string]interface{}{"operation": "blame", "path": "main.go"},
			wantArgs: []string{"blame", "main.go"},
		},
		{
			name:      "blame missing path",
			params:    map[string]interface{}{"operation": "blame"},
			wantError: true,
		},

		// --- log with duplicate --oneline in flags (should deduplicate) ---
			{
				name:     "log with duplicate --oneline in flags",
				params:   map[string]interface{}{"operation": "log", "flags": []interface{}{"--oneline"}},
				wantArgs: []string{"log", "-20", "--oneline"},
			},

			// --- reflog ---
		{
			name:     "reflog default",
			params:   map[string]interface{}{"operation": "reflog"},
			wantArgs: []string{"reflog", "show", "-20"},
		},
		{
			name:     "reflog with branch",
			params:   map[string]interface{}{"operation": "reflog", "branch": "feature"},
			wantArgs: []string{"reflog", "show", "feature", "-20"},
		},
		{
			name:     "reflog with max_count",
			params:   map[string]interface{}{"operation": "reflog", "max_count": float64(10)},
			wantArgs: []string{"reflog", "show", "-10"},
		},

		// --- rev-list ---
			{
				name:     "rev-list default",
				params:   map[string]interface{}{"operation": "rev-list"},
				wantArgs: []string{"rev-list", "-20", "--count"},
			},
			{
				name:     "rev-list with max_count",
				params:   map[string]interface{}{"operation": "rev-list", "max_count": float64(10)},
				wantArgs: []string{"rev-list", "-10", "--count"},
			},
			{
				name:     "rev-list with duplicate --count in flags (should deduplicate)",
				params:   map[string]interface{}{"operation": "rev-list", "max_count": float64(20), "flags": []interface{}{"--count"}},
				wantArgs: []string{"rev-list", "-20", "--count"},
			},

			// --- shortlog ---
		{
			name:     "shortlog default",
			params:   map[string]interface{}{"operation": "shortlog"},
			wantArgs: []string{"shortlog", "-sn", "-20", "HEAD"},
		},
		{
			name:     "shortlog with max_count",
			params:   map[string]interface{}{"operation": "shortlog", "max_count": float64(5)},
			wantArgs: []string{"shortlog", "-sn", "-5", "HEAD"},
		},

		// --- checkout -b ---
		{
			name:     "checkout -b create branch",
			params:   map[string]interface{}{"operation": "checkout", "branch": "feature", "flags": []interface{}{"-b"}},
			wantArgs: []string{"checkout", "-b", "feature"},
		},
		{
			name:     "checkout -B force create",
			params:   map[string]interface{}{"operation": "checkout", "branch": "feature", "flags": []interface{}{"-B"}},
			wantArgs: []string{"checkout", "-b", "feature"},
		},
		{
			name:     "checkout -b with --ours",
			params:   map[string]interface{}{"operation": "checkout", "branch": "feature", "flags": []interface{}{"-b"}, "ours_theirs": "ours"},
			wantArgs: []string{"checkout", "-b", "--ours", "feature"},
		},

		// --- merge with message ---
		{
			name:     "merge basic",
			params:   map[string]interface{}{"operation": "merge", "target": "feature"},
			wantArgs: []string{"merge", "feature"},
		},
		{
			name:     "merge with custom message",
			params:   map[string]interface{}{"operation": "merge", "target": "feature", "message": "Merge feature branch"},
			wantArgs: []string{"merge", "-m", "Merge feature branch", "feature"},
		},

		// --- blame with files fallback ---
		{
			name:     "blame with files array",
			params:   map[string]interface{}{"operation": "blame", "files": []interface{}{"main.go"}},
			wantArgs: []string{"blame", "main.go"},
		},

		// --- stash subcommands ---
		{
			name:     "stash default",
			params:   map[string]interface{}{"operation": "stash"},
			wantArgs: []string{"stash"},
		},
		{
			name:     "stash pop",
			params:   map[string]interface{}{"operation": "stash", "stash_subcommand": "pop"},
			wantArgs: []string{"stash", "pop"},
		},
		{
			name:     "stash list",
			params:   map[string]interface{}{"operation": "stash", "stash_subcommand": "list"},
			wantArgs: []string{"stash", "list"},
		},
		{
			name:     "stash with untracked",
			params:   map[string]interface{}{"operation": "stash", "stash_include_untracked": true},
			wantArgs: []string{"stash", "-u"},
		},
		{
			name:     "stash pop with untracked",
			params:   map[string]interface{}{"operation": "stash", "stash_subcommand": "pop", "stash_include_untracked": true},
			wantArgs: []string{"stash", "pop", "-u"},
		},
	}

	// --- gh operations ---
	ghTests := []struct {
		name      string
		params    map[string]interface{}
		wantArgs  []string
		wantError bool
	}{
		// gh pr view
		{
			name:     "gh pr view basic",
			params:   map[string]interface{}{"operation": "gh", "gh_command": []interface{}{"pr", "view", "123"}},
			wantArgs: []string{"pr", "view", "123"},
		},
		{
			name:     "gh pr view with --json",
			params:   map[string]interface{}{"operation": "gh", "gh_command": []interface{}{"pr", "view", "123", "--json", "title,state,author"}},
			wantArgs: []string{"pr", "view", "123", "--json", "title,state,author"},
		},
		{
			name:     "gh pr view with --comments",
			params:   map[string]interface{}{"operation": "gh", "gh_command": []interface{}{"pr", "view", "123", "--comments"}},
			wantArgs: []string{"pr", "view", "123", "--comments"},
		},
		{
			name:     "gh pr view with --repo",
			params:   map[string]interface{}{"operation": "gh", "gh_command": []interface{}{"pr", "view", "123", "--repo", "owner/repo"}},
			wantArgs: []string{"pr", "view", "123", "--repo", "owner/repo"},
		},
		{
			name:     "gh pr view with --web",
			params:   map[string]interface{}{"operation": "gh", "gh_command": []interface{}{"pr", "view", "123", "--web"}},
			wantArgs: []string{"pr", "view", "123", "--web"},
		},

		// gh pr list
		{
			name:     "gh pr list basic",
			params:   map[string]interface{}{"operation": "gh", "gh_command": []interface{}{"pr", "list"}},
			wantArgs: []string{"pr", "list"},
		},
		{
			name:     "gh pr list with state and limit",
			params:   map[string]interface{}{"operation": "gh", "gh_command": []interface{}{"pr", "list", "--state", "open", "--limit", "50"}},
			wantArgs: []string{"pr", "list", "--state", "open", "--limit", "50"},
		},
		{
			name:     "gh pr list with author and label",
			params:   map[string]interface{}{"operation": "gh", "gh_command": []interface{}{"pr", "list", "--author", "me", "--label", "bug"}},
			wantArgs: []string{"pr", "list", "--author", "me", "--label", "bug"},
		},

		// gh pr diff
		{
			name:     "gh pr diff basic",
			params:   map[string]interface{}{"operation": "gh", "gh_command": []interface{}{"pr", "diff", "123"}},
			wantArgs: []string{"pr", "diff", "123"},
		},
		{
			name:     "gh pr diff with --name-only",
			params:   map[string]interface{}{"operation": "gh", "gh_command": []interface{}{"pr", "diff", "123", "--name-only"}},
			wantArgs: []string{"pr", "diff", "123", "--name-only"},
		},

		// gh pr checks
		{
			name:     "gh pr checks basic",
			params:   map[string]interface{}{"operation": "gh", "gh_command": []interface{}{"pr", "checks", "123"}},
			wantArgs: []string{"pr", "checks", "123"},
		},
		{
			name:     "gh pr checks with --required",
			params:   map[string]interface{}{"operation": "gh", "gh_command": []interface{}{"pr", "checks", "123", "--required"}},
			wantArgs: []string{"pr", "checks", "123", "--required"},
		},

		// gh pr status
		{
			name:     "gh pr status basic",
			params:   map[string]interface{}{"operation": "gh", "gh_command": []interface{}{"pr", "status"}},
			wantArgs: []string{"pr", "status"},
		},

		// gh issue view
		{
			name:     "gh issue view basic",
			params:   map[string]interface{}{"operation": "gh", "gh_command": []interface{}{"issue", "view", "456"}},
			wantArgs: []string{"issue", "view", "456"},
		},
		{
			name:     "gh issue view with --comments",
			params:   map[string]interface{}{"operation": "gh", "gh_command": []interface{}{"issue", "view", "456", "--comments"}},
			wantArgs: []string{"issue", "view", "456", "--comments"},
		},

		// gh issue list
		{
			name:     "gh issue list basic",
			params:   map[string]interface{}{"operation": "gh", "gh_command": []interface{}{"issue", "list"}},
			wantArgs: []string{"issue", "list"},
		},
		{
			name:     "gh issue list with state",
			params:   map[string]interface{}{"operation": "gh", "gh_command": []interface{}{"issue", "list", "--state", "closed", "--limit", "20"}},
			wantArgs: []string{"issue", "list", "--state", "closed", "--limit", "20"},
		},

		// gh issue status
		{
			name:     "gh issue status basic",
			params:   map[string]interface{}{"operation": "gh", "gh_command": []interface{}{"issue", "status"}},
			wantArgs: []string{"issue", "status"},
		},

		// gh run list
		{
			name:     "gh run list basic",
			params:   map[string]interface{}{"operation": "gh", "gh_command": []interface{}{"run", "list"}},
			wantArgs: []string{"run", "list"},
		},
		{
			name:     "gh run list with status and workflow",
			params:   map[string]interface{}{"operation": "gh", "gh_command": []interface{}{"run", "list", "--status", "success", "--workflow", "CI", "--limit", "10"}},
			wantArgs: []string{"run", "list", "--status", "success", "--workflow", "CI", "--limit", "10"},
		},
		{
			name:     "gh run list with branch",
			params:   map[string]interface{}{"operation": "gh", "gh_command": []interface{}{"run", "list", "--branch", "main"}},
			wantArgs: []string{"run", "list", "--branch", "main"},
		},

		// gh run view
		{
			name:     "gh run view basic",
			params:   map[string]interface{}{"operation": "gh", "gh_command": []interface{}{"run", "view", "123456"}},
			wantArgs: []string{"run", "view", "123456"},
		},
		{
			name:     "gh run view with --log",
			params:   map[string]interface{}{"operation": "gh", "gh_command": []interface{}{"run", "view", "123456", "--log"}},
			wantArgs: []string{"run", "view", "123456", "--log"},
		},
		{
			name:     "gh run view with --log-failed",
			params:   map[string]interface{}{"operation": "gh", "gh_command": []interface{}{"run", "view", "123456", "--log-failed"}},
			wantArgs: []string{"run", "view", "123456", "--log-failed"},
		},
		{
			name:     "gh run view with --job",
			params:   map[string]interface{}{"operation": "gh", "gh_command": []interface{}{"run", "view", "123456", "--job", "987"}},
			wantArgs: []string{"run", "view", "123456", "--job", "987"},
		},

		// gh auth status
		{
			name:     "gh auth status basic",
			params:   map[string]interface{}{"operation": "gh", "gh_command": []interface{}{"auth", "status"}},
			wantArgs: []string{"auth", "status"},
		},

		// gh release list
		{
			name:     "gh release list basic",
			params:   map[string]interface{}{"operation": "gh", "gh_command": []interface{}{"release", "list"}},
			wantArgs: []string{"release", "list"},
		},
		{
			name:     "gh release list with limit",
			params:   map[string]interface{}{"operation": "gh", "gh_command": []interface{}{"release", "list", "--limit", "5"}},
			wantArgs: []string{"release", "list", "--limit", "5"},
		},

		// gh release view
		{
			name:     "gh release view basic",
			params:   map[string]interface{}{"operation": "gh", "gh_command": []interface{}{"release", "view", "v1.0.0"}},
			wantArgs: []string{"release", "view", "v1.0.0"},
		},

		// gh search repos
		{
			name:     "gh search repos basic",
			params:   map[string]interface{}{"operation": "gh", "gh_command": []interface{}{"search", "repos", "kubernetes"}},
			wantArgs: []string{"search", "repos", "kubernetes"},
		},
		{
			name:     "gh search repos with flags",
			params:   map[string]interface{}{"operation": "gh", "gh_command": []interface{}{"search", "repos", "kubernetes", "--language", "Go", "--stars", ">10000", "--limit", "10"}},
			wantArgs: []string{"search", "repos", "kubernetes", "--language", "Go", "--stars", ">10000", "--limit", "10"},
		},

		// gh search issues
		{
			name:     "gh search issues basic",
			params:   map[string]interface{}{"operation": "gh", "gh_command": []interface{}{"search", "issues", "memory leak"}},
			wantArgs: []string{"search", "issues", "memory leak"},
		},
		{
			name:     "gh search issues with state",
			params:   map[string]interface{}{"operation": "gh", "gh_command": []interface{}{"search", "issues", "bug", "--state", "open", "--limit", "20"}},
			wantArgs: []string{"search", "issues", "bug", "--state", "open", "--limit", "20"},
		},

		// gh search prs
		{
			name:     "gh search prs basic",
			params:   map[string]interface{}{"operation": "gh", "gh_command": []interface{}{"search", "prs", "feature request"}},
			wantArgs: []string{"search", "prs", "feature request"},
		},
		{
			name:     "gh search prs with state",
			params:   map[string]interface{}{"operation": "gh", "gh_command": []interface{}{"search", "prs", "fix", "--state", "closed", "--limit", "10"}},
			wantArgs: []string{"search", "prs", "fix", "--state", "closed", "--limit", "10"},
		},

		// -- Security: dangerous --repo values should be rejected --
		{
			name:      "gh pr view --repo with URL",
			params:    map[string]interface{}{"operation": "gh", "gh_command": []interface{}{"pr", "view", "123", "--repo", "https://evil.com/owner/repo"}},
			wantError: true,
		},
		{
			name:      "gh pr view --repo with SSH-style",
			params:    map[string]interface{}{"operation": "gh", "gh_command": []interface{}{"pr", "view", "123", "--repo", "git@evil.com:owner/repo"}},
			wantError: true,
		},
		{
			name:      "gh pr view --repo with HOST/OWNER/REPO (3 segments)",
			params:    map[string]interface{}{"operation": "gh", "gh_command": []interface{}{"pr", "view", "123", "--repo", "evil.com/SECRET/repo"}},
			wantError: true,
		},
		{
			name:      "gh pr view with --repo= form with URL",
			params:    map[string]interface{}{"operation": "gh", "gh_command": []interface{}{"pr", "view", "123", "--repo=https://evil.com/owner/repo"}},
			wantError: true,
		},
		{
			name:      "gh pr view with --repo= form with HOST/OWNER/REPO",
			params:    map[string]interface{}{"operation": "gh", "gh_command": []interface{}{"pr", "view", "123", "--repo=evil.com/SECRET/repo"}},
			wantError: true,
		},
		{
			name:      "gh pr view with unknown flag",
			params:    map[string]interface{}{"operation": "gh", "gh_command": []interface{}{"pr", "view", "123", "--unknown"}},
			wantError: true,
		},
		{
			name:      "gh missing gh_command",
			params:    map[string]interface{}{"operation": "gh"},
			wantError: true,
		},
		{
			name:      "gh with empty gh_command",
			params:    map[string]interface{}{"operation": "gh", "gh_command": []interface{}{}},
			wantError: true,
		},
		{
			name:      "gh with write-only subcommand (pr create)",
			params:    map[string]interface{}{"operation": "gh", "gh_command": []interface{}{"pr", "create"}},
			wantError: true,
		},
		{
			name:      "gh with non-numeric limit",
			params:    map[string]interface{}{"operation": "gh", "gh_command": []interface{}{"pr", "list", "--limit", "abc"}},
			wantError: true,
		},
	}

	for _, tt := range append(tests, ghTests...) {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildGitCommand(tt.params)
			if tt.wantError {
				if err == nil {
					t.Errorf("buildGitCommand() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("buildGitCommand() unexpected error: %v", err)
				return
			}
			if !reflect.DeepEqual(got, tt.wantArgs) {
				t.Errorf("buildGitCommand() = %v, want %v", got, tt.wantArgs)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Test helpers for git utility functions
// ---------------------------------------------------------------------------

// createTestRepo creates a temporary git repo with an initial commit and returns
// the repo path. The caller should clean up with os.RemoveAll.
func createTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("command %v failed: %v\noutput: %s", args, err, out)
		}
	}
	// Create an initial file and commit
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "add", ".")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add failed: %v\noutput: %s", err, out)
	}
	cmd = exec.Command("git", "commit", "-m", "initial commit")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit failed: %v\noutput: %s", err, out)
	}
	return dir
}

// ---------------------------------------------------------------------------
// Tests for FindGitRoot
// ---------------------------------------------------------------------------

func TestFindGitRoot(t *testing.T) {
	t.Run("find from repo root", func(t *testing.T) {
		dir := createTestRepo(t)
		got, err := FindGitRoot(dir)
		if err != nil {
			t.Fatalf("FindGitRoot() error = %v", err)
		}
		if got != dir {
			t.Errorf("FindGitRoot() = %q, want %q", got, dir)
		}
	})

	t.Run("find from subdirectory", func(t *testing.T) {
		dir := createTestRepo(t)
		sub := filepath.Join(dir, "a", "b")
		if err := os.MkdirAll(sub, 0755); err != nil {
			t.Fatal(err)
		}
		got, err := FindGitRoot(sub)
		if err != nil {
			t.Fatalf("FindGitRoot() error = %v", err)
		}
		if got != dir {
			t.Errorf("FindGitRoot() = %q, want %q", got, dir)
		}
	})

	t.Run("error not in git repo", func(t *testing.T) {
		dir := t.TempDir()
		_, err := FindGitRoot(dir)
		if err == nil {
			t.Fatal("FindGitRoot() expected error, got nil")
		}
	})

	t.Run("find git worktree (file containing gitdir)", func(t *testing.T) {
		// Create a parent repo
		parentDir := t.TempDir()
		cmds := [][]string{
			{"git", "init"},
			{"git", "config", "user.email", "test@test.com"},
			{"git", "config", "user.name", "Test"},
		}
		for _, args := range cmds {
			cmd := exec.Command(args[0], args[1:]...)
			cmd.Dir = parentDir
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%v failed: %v\n%s", args, err, out)
			}
		}
		// Create initial commit
		if err := os.WriteFile(filepath.Join(parentDir, "init.txt"), []byte("init\n"), 0644); err != nil {
			t.Fatal(err)
		}
		run := func(args ...string) {
			cmd := exec.Command(args[0], args[1:]...)
			cmd.Dir = parentDir
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%v failed: %v\n%s", args, err, out)
			}
		}
		run("git", "add", ".")
		run("git", "commit", "-m", "init")

		// Create a fake worktree directory with a .git file
		worktreeDir := t.TempDir()
		gitFile := filepath.Join(worktreeDir, ".git")
		gitContent := "gitdir: " + filepath.Join(parentDir, ".git", "worktrees", "wt1") + "\n"
		if err := os.WriteFile(gitFile, []byte(gitContent), 0644); err != nil {
			t.Fatal(err)
		}

		got, err := FindGitRoot(worktreeDir)
		if err != nil {
			t.Fatalf("FindGitRoot() error = %v", err)
		}
		if got != worktreeDir {
			t.Errorf("FindGitRoot() = %q, want %q", got, worktreeDir)
		}
	})
}

// ---------------------------------------------------------------------------
// Tests for GetBranch
// ---------------------------------------------------------------------------

func TestGetBranch(t *testing.T) {
	t.Run("get current branch", func(t *testing.T) {
		dir := createTestRepo(t)
		branch, err := GetBranch(dir)
		if err != nil {
			t.Fatalf("GetBranch() error = %v", err)
		}
		// Default branch could be "main" or "master" depending on git config
		if branch != "main" && branch != "master" {
			t.Errorf("GetBranch() = %q, want 'main' or 'master'", branch)
		}
	})

	t.Run("error not a git repo", func(t *testing.T) {
		dir := t.TempDir()
		_, err := GetBranch(dir)
		if err == nil {
			t.Fatal("GetBranch() expected error, got nil")
		}
	})
}

// ---------------------------------------------------------------------------
// Tests for IsBareRepo
// ---------------------------------------------------------------------------

func TestIsBareRepo(t *testing.T) {
	t.Run("false for normal repo", func(t *testing.T) {
		dir := createTestRepo(t)
		if IsBareRepo(dir) {
			t.Error("IsBareRepo() = true for normal repo, want false")
		}
	})

	t.Run("true for bare repo", func(t *testing.T) {
		bareDir := filepath.Join(t.TempDir(), "bare.git")
		cmd := exec.Command("git", "init", "--bare", bareDir)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git init --bare failed: %v\n%s", err, out)
		}
		if !IsBareRepo(bareDir) {
			t.Error("IsBareRepo() = false for bare repo, want true")
		}
	})
}

// ---------------------------------------------------------------------------
// Tests for IsGitRepo
// ---------------------------------------------------------------------------

func TestIsGitRepo(t *testing.T) {
	t.Run("true inside git repo", func(t *testing.T) {
		dir := createTestRepo(t)
		if !IsGitRepo(dir) {
			t.Error("IsGitRepo() = false inside git repo, want true")
		}
	})

	t.Run("false outside git repo", func(t *testing.T) {
		dir := t.TempDir()
		if IsGitRepo(dir) {
			t.Error("IsGitRepo() = true outside git repo, want false")
		}
	})
}

// ---------------------------------------------------------------------------
// Tests for GetGitStatus
// ---------------------------------------------------------------------------

func TestGetGitStatus(t *testing.T) {
	t.Run("empty for clean repo", func(t *testing.T) {
		dir := createTestRepo(t)
		status, err := GetGitStatus(dir)
		if err != nil {
			t.Fatalf("GetGitStatus() error = %v", err)
		}
		if len(status) != 0 {
			t.Errorf("GetGitStatus() returned %d entries for clean repo, want 0", len(status))
		}
	})

	t.Run("shows modified file", func(t *testing.T) {
		dir := createTestRepo(t)
		// Modify a tracked file
		if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("modified\n"), 0644); err != nil {
			t.Fatal(err)
		}
		status, err := GetGitStatus(dir)
		if err != nil {
			t.Fatalf("GetGitStatus() error = %v", err)
		}
		code, ok := status["hello.txt"]
		if !ok {
			t.Fatalf("GetGitStatus() missing entry for hello.txt, status = %v", status)
		}
		// Unstaged modification shows " M" (space in first column, M in second)
		if strings.TrimSpace(code) != "M" {
			t.Errorf("GetGitStatus() for modified file = %q, want something with 'M'", code)
		}
	})

	t.Run("shows new untracked file", func(t *testing.T) {
		dir := createTestRepo(t)
		// Create an untracked file
		if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("new\n"), 0644); err != nil {
			t.Fatal(err)
		}
		status, err := GetGitStatus(dir)
		if err != nil {
			t.Fatalf("GetGitStatus() error = %v", err)
		}
		code, ok := status["new.txt"]
		if !ok {
			t.Fatalf("GetGitStatus() missing entry for new.txt, status = %v", status)
		}
		// Untracked files show "??"
		if code != "??" {
			t.Errorf("GetGitStatus() for untracked file = %q, want '??'", code)
		}
	})

	t.Run("shows deleted file", func(t *testing.T) {
		dir := createTestRepo(t)
		// Delete a tracked file
		if err := os.Remove(filepath.Join(dir, "hello.txt")); err != nil {
			t.Fatal(err)
		}
		status, err := GetGitStatus(dir)
		if err != nil {
			t.Fatalf("GetGitStatus() error = %v", err)
		}
		code, ok := status["hello.txt"]
		if !ok {
			t.Fatalf("GetGitStatus() missing entry for hello.txt, status = %v", status)
		}
		// Deleted file shows " D"
		if strings.TrimSpace(code) != "D" {
			t.Errorf("GetGitStatus() for deleted file = %q, want something with 'D'", code)
		}
	})

	t.Run("shows staged file", func(t *testing.T) {
		dir := createTestRepo(t)
		// Create and stage a new file
		if err := os.WriteFile(filepath.Join(dir, "staged.txt"), []byte("staged\n"), 0644); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("git", "add", "staged.txt")
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git add failed: %v\n%s", err, out)
		}
		status, err := GetGitStatus(dir)
		if err != nil {
			t.Fatalf("GetGitStatus() error = %v", err)
		}
		code, ok := status["staged.txt"]
		if !ok {
			t.Fatalf("GetGitStatus() missing entry for staged.txt, status = %v", status)
		}
		// Staged new file shows "A "
		if strings.TrimSpace(code) != "A" {
			t.Errorf("GetGitStatus() for staged file = %q, want something with 'A'", code)
		}
	})
}

// ---------------------------------------------------------------------------
// Tests for HasUncommittedChanges
// ---------------------------------------------------------------------------

func TestHasUncommittedChanges(t *testing.T) {
	t.Run("false for clean repo", func(t *testing.T) {
		dir := createTestRepo(t)
		if HasUncommittedChanges(dir) {
			t.Error("HasUncommittedChanges() = true for clean repo, want false")
		}
	})

	t.Run("true after modifying tracked file", func(t *testing.T) {
		dir := createTestRepo(t)
		// Modify a tracked file
		if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("modified\n"), 0644); err != nil {
			t.Fatal(err)
		}
		if !HasUncommittedChanges(dir) {
			t.Error("HasUncommittedChanges() = false after modification, want true")
		}
	})

	t.Run("true after staging changes", func(t *testing.T) {
		dir := createTestRepo(t)
		// Modify and stage a tracked file
		if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("staged\n"), 0644); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("git", "add", "hello.txt")
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git add failed: %v\n%s", err, out)
		}
		if !HasUncommittedChanges(dir) {
			t.Error("HasUncommittedChanges() = false after staging, want true")
		}
	})
}

// ---------------------------------------------------------------------------
// Tests for GetDefaultBranch
// ---------------------------------------------------------------------------

func TestGetDefaultBranch(t *testing.T) {
	t.Run("returns main or master for local-only repo", func(t *testing.T) {
		dir := createTestRepo(t)
		branch, err := GetDefaultBranch(dir)
		if err != nil {
			t.Fatalf("GetDefaultBranch() error = %v", err)
		}
		// Without origin/HEAD, should fall back to "main"
		if branch != "main" && branch != "master" {
			t.Errorf("GetDefaultBranch() = %q, want 'main' or 'master'", branch)
		}
	})

	t.Run("falls back to main when no remote", func(t *testing.T) {
		dir := createTestRepo(t)
		branch, err := GetDefaultBranch(dir)
		if err != nil {
			t.Fatalf("GetDefaultBranch() error = %v", err)
		}
		if branch != "main" {
			t.Errorf("GetDefaultBranch() = %q, want 'main' as fallback", branch)
		}
	})
}

// ---------------------------------------------------------------------------
// Tests for GetCurrentCommitHash
// ---------------------------------------------------------------------------

func TestGetCurrentCommitHash(t *testing.T) {
	t.Run("returns valid hex hash", func(t *testing.T) {
		dir := createTestRepo(t)
		hash, err := GetCurrentCommitHash(dir)
		if err != nil {
			t.Fatalf("GetCurrentCommitHash() error = %v", err)
		}
		// SHA-1 hashes are 40 hex characters
		matched, err := regexp.MatchString(`^[0-9a-f]{40}$`, hash)
		if err != nil {
			t.Fatal(err)
		}
		if !matched {
			t.Errorf("GetCurrentCommitHash() = %q, want a 40-char hex hash", hash)
		}
	})

	t.Run("error not a git repo", func(t *testing.T) {
		dir := t.TempDir()
		_, err := GetCurrentCommitHash(dir)
		if err == nil {
			t.Fatal("GetCurrentCommitHash() expected error, got nil")
		}
	})

	t.Run("returns consistent hash on repeated calls", func(t *testing.T) {
		dir := createTestRepo(t)
		hash1, err := GetCurrentCommitHash(dir)
		if err != nil {
			t.Fatalf("GetCurrentCommitHash() error = %v", err)
		}
		hash2, err := GetCurrentCommitHash(dir)
		if err != nil {
			t.Fatalf("GetCurrentCommitHash() error = %v", err)
		}
		if hash1 != hash2 {
			t.Errorf("GetCurrentCommitHash() returned different hashes: %q vs %q", hash1, hash2)
		}
	})
}

// ---------------------------------------------------------------------------
// Tests for IsDirty
// ---------------------------------------------------------------------------

func TestIsDirty(t *testing.T) {
	t.Run("false for clean repo", func(t *testing.T) {
		dir := createTestRepo(t)
		if IsDirty(dir) {
			t.Error("IsDirty() = true for clean repo, want false")
		}
	})

	t.Run("true after unstaged modification", func(t *testing.T) {
		dir := createTestRepo(t)
		if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("modified\n"), 0644); err != nil {
			t.Fatal(err)
		}
		if !IsDirty(dir) {
			t.Error("IsDirty() = false after unstaged modification, want true")
		}
	})

	t.Run("true after staging changes", func(t *testing.T) {
		dir := createTestRepo(t)
		if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("staged\n"), 0644); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("git", "add", "hello.txt")
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git add failed: %v\n%s", err, out)
		}
		if !IsDirty(dir) {
			t.Error("IsDirty() = false after staging, want true")
		}
	})

	t.Run("false with untracked files only", func(t *testing.T) {
		dir := createTestRepo(t)
		// Create an untracked file — untracked files should NOT make the repo dirty
		// because git checkout/switch don't fail due to untracked files
		if err := os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("new\n"), 0644); err != nil {
			t.Fatal(err)
		}
		if IsDirty(dir) {
			t.Error("IsDirty() = true with only untracked file, want false")
		}
	})
}

// ---------------------------------------------------------------------------
// Tests for info operation
// ---------------------------------------------------------------------------

func TestExecuteGitInfo(t *testing.T) {
	t.Run("returns info for git repo", func(t *testing.T) {
		dir := createTestRepo(t)
		result := executeGitInfo(dir)
		if result.IsError {
			t.Fatalf("executeGitInfo() returned error: %s", result.Output)
		}
		if !strings.Contains(result.Output, "Git Repository Information:") {
			t.Errorf("executeGitInfo() missing header, got: %s", result.Output)
		}
		if !strings.Contains(result.Output, "Root:") {
			t.Errorf("executeGitInfo() missing Root, got: %s", result.Output)
		}
		if !strings.Contains(result.Output, "Branch:") {
			t.Errorf("executeGitInfo() missing Branch, got: %s", result.Output)
		}
		if !strings.Contains(result.Output, "Commit:") {
			t.Errorf("executeGitInfo() missing Commit, got: %s", result.Output)
		}
		if !strings.Contains(result.Output, "Dirty:") {
			t.Errorf("executeGitInfo() missing Dirty, got: %s", result.Output)
		}
		if !strings.Contains(result.Output, "Dirty: false") {
			t.Errorf("executeGitInfo() clean repo should show Dirty: false, got: %s", result.Output)
		}
	})

	t.Run("shows dirty for modified repo", func(t *testing.T) {
		dir := createTestRepo(t)
		if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("modified\n"), 0644); err != nil {
			t.Fatal(err)
		}
		result := executeGitInfo(dir)
		if result.IsError {
			t.Fatalf("executeGitInfo() returned error: %s", result.Output)
		}
		if !strings.Contains(result.Output, "Dirty: true") {
			t.Errorf("executeGitInfo() dirty repo should show Dirty: true, got: %s", result.Output)
		}
		if !strings.Contains(result.Output, "Changes:") {
			t.Errorf("executeGitInfo() dirty repo should show Changes summary, got: %s", result.Output)
		}
	})

	t.Run("not a git repo", func(t *testing.T) {
		dir := t.TempDir()
		result := executeGitInfo(dir)
		if result.IsError {
			t.Fatalf("executeGitInfo() should not return error for non-repo, got: %s", result.Output)
		}
		if !strings.Contains(result.Output, "Not a git repository") {
			t.Errorf("executeGitInfo() non-repo should say so, got: %s", result.Output)
		}
	})
}

// ---------------------------------------------------------------------------
// Tests for GetGitContext
// ---------------------------------------------------------------------------

func TestGetGitContext(t *testing.T) {
	t.Run("empty for non-git dir", func(t *testing.T) {
		origWd, _ := os.Getwd()
		defer os.Chdir(origWd)

		dir := t.TempDir()
		os.Chdir(dir)
		ctx := GetGitContext()
		if ctx != "" {
			t.Errorf("GetGitContext() = %q, want empty for non-git dir", ctx)
		}
	})

	t.Run("returns context for git repo", func(t *testing.T) {
		origWd, _ := os.Getwd()
		defer os.Chdir(origWd)

		dir := createTestRepo(t)
		os.Chdir(dir)
		ctx := GetGitContext()
		if ctx == "" {
			t.Fatal("GetGitContext() returned empty for git repo")
		}
		if !strings.Contains(ctx, "Git Branch:") {
			t.Errorf("GetGitContext() missing Git Branch, got: %s", ctx)
		}
		if !strings.Contains(ctx, "Git Commit:") {
			t.Errorf("GetGitContext() missing Git Commit, got: %s", ctx)
		}
		if !strings.Contains(ctx, "Git Dirty:") {
			t.Errorf("GetGitContext() missing Git Dirty, got: %s", ctx)
		}
		if !strings.Contains(ctx, "Git Dirty: false") {
			t.Errorf("GetGitContext() clean repo should show Git Dirty: false, got: %s", ctx)
		}
	})
}

// ---------------------------------------------------------------------------
// Tests for info operation in CheckPermissions
// ---------------------------------------------------------------------------

func TestGitTool_InfoCheckPermissions(t *testing.T) {
	tool := &GitTool{}
	params := map[string]interface{}{"operation": "info"}
	warning := tool.CheckPermissions(params)
	if warning != "" {
		t.Errorf("CheckPermissions(info) = %q, want empty (read-only)", warning)
	}
}

// ---------------------------------------------------------------------------
// Tests for info operation via GitTool.Execute
// ---------------------------------------------------------------------------

func TestGitTool_ExecuteInfo(t *testing.T) {
	t.Run("info operation works via Execute", func(t *testing.T) {
		dir := createTestRepo(t)
		tool := &GitTool{}
		params := map[string]interface{}{
			"operation": "info",
			"directory": dir,
		}
		result := tool.Execute(params)
		if result.IsError {
			t.Fatalf("Execute(info) returned error: %s", result.Output)
		}
		if !strings.Contains(result.Output, "Git Repository Information:") {
			t.Errorf("Execute(info) missing header, got: %s", result.Output)
		}
	})

	t.Run("info operation with path param", func(t *testing.T) {
		dir := createTestRepo(t)
		tool := &GitTool{}
		params := map[string]interface{}{
			"operation": "info",
			"path":      dir,
		}
		result := tool.Execute(params)
		if result.IsError {
			t.Fatalf("Execute(info) returned error: %s", result.Output)
		}
		if !strings.Contains(result.Output, "Root:") {
			t.Errorf("Execute(info) missing Root, got: %s", result.Output)
		}
	})

	t.Run("info not a git repo", func(t *testing.T) {
		tool := &GitTool{}
		params := map[string]interface{}{
			"operation": "info",
			"directory": t.TempDir(),
		}
		result := tool.Execute(params)
		if !strings.Contains(result.Output, "Not a git repository") {
			t.Errorf("Execute(info) for non-repo should say so, got: %s", result.Output)
		}
	})
}

// ---------------------------------------------------------------------------
// Tests for buildGitCommand rejects info (handled separately)
// ---------------------------------------------------------------------------

func TestBuildGitCommand_InfoRejected(t *testing.T) {
	params := map[string]interface{}{"operation": "info"}
	_, err := buildGitCommand(params)
	if err == nil {
		t.Error("buildGitCommand(info) should return error (info is handled before buildGitCommand)")
	}
}
