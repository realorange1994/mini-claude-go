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

	for _, tt := range tests {
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
