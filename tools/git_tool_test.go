package tools

import (
	"reflect"
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
