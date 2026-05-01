package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// GitTool provides Git version control and GitHub CLI (gh) operations.
type GitTool struct{}

func (*GitTool) Name() string    { return "git" }
func (*GitTool) Description() string {
	return "Execute Git version control operations (clone, init, add, commit, push, pull, fetch, branch, checkout, merge, rebase, stash, reset, tag, status, diff, log, remote, show, describe, ls-files, ls-tree, rev-parse, rev-list, worktree, rm, mv, restore, switch, cherry-pick, revert, clean, blame, reflog, shortlog, info) and read-only GitHub CLI (gh) operations (pr view/list/diff/checks/status, issue view/list/status, run list/view, auth status, release list/view, search repos/issues/prs). Use operation='info' to get current repository state (branch, commit, dirty status, default branch, git root)."
}

func (*GitTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"operation": map[string]interface{}{
				"type":        "string",
				"description": "Git operation to perform",
				"enum": []string{
					"clone", "init", "add", "commit", "push", "pull", "fetch",
					"branch", "checkout", "merge", "rebase", "stash", "reset",
					"tag", "status", "diff", "log", "remote", "show", "describe",
					"ls-files", "ls-tree", "rev-parse", "rev-list", "worktree",
					"rm", "mv", "restore", "switch", "cherry-pick", "revert",
					"clean", "blame", "reflog", "shortlog", "gh", "info",
				},
			},
			"repo": map[string]interface{}{
				"type":        "string",
				"description": "Repository URL (only for clone)",
			},
			"path": map[string]interface{}{
				"type":        "string",
				"description": "For clone: destination directory path. For init/worktree: target path. For mv: destination path. For blame: file path to blame (also supports 'files' array fallback). NOT used as working directory (use 'directory' for that)",
			},
			"directory": map[string]interface{}{
				"type":        "string",
				"description": "Working directory to run the git command in. For clone, this is where git clone runs (path is the clone destination). For other ops, this is the repo directory",
			},
			"branch": map[string]interface{}{
				"type":        "string",
				"description": "Branch name for checkout/branch/push/pull/worktree. Also used as tag name for tag operation. checkout does NOT support 'files' param - use 'restore' to unstage files",
			},
			"message": map[string]interface{}{
				"type":        "string",
				"description": "Commit message (only for commit)",
			},
			"files": map[string]interface{}{
				"type":        "array",
				"description": "File paths for add/rm/restore/diff/ls-files ONLY. NOT for checkout, commit, or mv. checkout has NO files support. mv uses 'source' param",
				"items":       map[string]interface{}{"type": "string"},
			},
			"remote": map[string]interface{}{
				"type":        "string",
				"description": "Remote name for push/pull/fetch only (default: origin). NOT for 'remote' operation itself",
			},
			"target": map[string]interface{}{
				"type":        "string",
				"description": "Target branch or commit (for merge, rebase, describe, show, cherry-pick, revert)",
			},
			"flags": map[string]interface{}{
				"type":        "array",
				"description": "Additional git flags (e.g. [--force], [--soft])",
				"items":       map[string]interface{}{"type": "string"},
			},
			"all": map[string]interface{}{
				"type":        "boolean",
				"description": "Stage all changed files (for add, commit -a)",
			},
			"worktree_name": map[string]interface{}{
				"type":        "string",
				"description": "Worktree name (for worktree operation)",
			},
			"worktree_branch": map[string]interface{}{
				"type":        "string",
				"description": "Branch for new worktree (for worktree add)",
			},
			"worktree_remove": map[string]interface{}{
				"type":        "boolean",
				"description": "Remove a worktree (for worktree remove)",
			},
			"max_count": map[string]interface{}{
				"type":        "integer",
				"description": "Maximum number of entries to return (for log, rev-list, default: 20)",
			},
			"staged": map[string]interface{}{
				"type":        "boolean",
				"description": "For restore: restore from staging area (--staged). For rm: remove from index only (--cached). NOTE: there is NO separate 'cached' param for rm - use 'staged' instead",
			},
			"force": map[string]interface{}{
				"type":        "boolean",
				"description": "Force the operation (for switch, clean, rm, restore, mv)",
			},
			"ours_theirs": map[string]interface{}{
				"type":        "string",
				"description": "Checkout ours or theirs during conflict (for checkout --ours/--theirs). checkout does NOT support 'files' param",
			},
			"dry_run": map[string]interface{}{
				"type":        "boolean",
				"description": "Show what would be done without making changes (for clean)",
			},
			"mainline": map[string]interface{}{
				"type":        "integer",
				"description": "Mainline parent number for cherry-picking/reverting a merge commit (for cherry-pick, revert)",
			},
			"author": map[string]interface{}{
				"type":        "string",
				"description": "Author string (e.g. 'Name <email>') (only for commit)",
			},
			"cached": map[string]interface{}{
				"type":        "boolean",
				"description": "Show staged changes instead of working tree (only for diff). NOT for rm - use 'staged' param for rm --cached",
			},
			"recursive": map[string]interface{}{
				"type":        "boolean",
				"description": "Recursive removal (only for clean, adds -d flag)",
			},
			"source": map[string]interface{}{
				"type":        "string",
				"description": "Source file for mv, or source branch/commit for switch. NOT for restore (restore uses 'files' param)",
			},
			"no_commit": map[string]interface{}{
				"type":        "boolean",
				"description": "Apply changes without committing (only for cherry-pick)",
			},
			"no_edit": map[string]interface{}{
				"type":        "boolean",
				"description": "Don't edit the commit message (only for revert)",
			},
			"stash_subcommand": map[string]interface{}{
				"type":        "string",
				"description": "Stash subcommand: pop, apply, drop, list, show (for stash operation). Default is 'push' (just 'git stash')",
			},
			"stash_include_untracked": map[string]interface{}{
				"type":        "boolean",
				"description": "Include untracked files in stash (for stash push, adds -u flag)",
			},
			"proxy": map[string]interface{}{
				"type":        "string",
				"description": "HTTP/SOCKS proxy URL for git operations (e.g. 'http://127.0.0.1:7890', 'socks5://127.0.0.1:1080'). Sets https_proxy and http_proxy environment variables for the git command.",
			},
			"gh_command": map[string]interface{}{
				"type":        "array",
				"description": "gh CLI subcommand and arguments (only for gh operation). Example: [\"pr\", \"view\", \"123\"] or [\"issue\", \"list\", \"--state\", \"open\"]. Only read-only commands are allowed.",
				"items":       map[string]interface{}{"type": "string"},
			},
		},
		"required": []string{"operation"},
	}
}

// CheckPermissions checks if the git operation is allowed and returns a warning message if not.
// Returns empty string if allowed, or a non-empty warning message to show the user.
func (*GitTool) CheckPermissions(params map[string]interface{}) string {
	operation, _ := params["operation"].(string)
	flags := getStringArray(params, "flags")

	// Check for explicitly dangerous operations/flags
	if dangerous, msg := isDangerousOperation(operation, flags); dangerous {
		return msg
	}

	// Classify the operation
	switch operation {
	case "status", "diff", "log", "show", "describe", "blame", "shortlog",
		"ls-files", "ls-tree", "rev-parse", "rev-list", "reflog", "info":
		// Read-only operations: no warning needed
		return ""

	case "commit":
		return "Warning: This commit will modify repository history."
	case "push":
		return "Warning: This push will transfer commits to the remote repository."
	case "pull":
		return "Warning: This pull will fetch and merge commits from the remote."
	case "fetch":
		return "Warning: This fetch will update remote-tracking references."
	case "merge":
		return "Warning: This merge will create a new commit integrating changes."
	case "rebase":
		return "Warning: This rebase will rewrite commit history."
	case "reset":
		return "Warning: This reset will move HEAD and potentially modify the index."
	case "clean":
		return "Warning: This clean will remove untracked files from the working tree."
	case "checkout":
		return "Warning: This checkout will modify the working tree."
	case "switch":
		return "Warning: This switch will change the current branch."
	case "restore":
		return "Warning: This restore will overwrite files in the working tree."
	case "stash":
		return "Warning: This stash will temporarily shelve changes."
	case "branch":
		// Branch listing is safe, creation is a write
		hasListFlag := false
		for _, f := range flags {
			if f == "-l" || f == "--list" || f == "-a" || f == "--all" ||
				f == "-r" || f == "--remotes" {
				hasListFlag = true
				break
			}
		}
		if hasListFlag || len(flags) == 0 {
			return "" // List-only is read-only
		}
		return "Warning: This branch operation will modify repository state."
	case "tag":
		// Tag listing is safe
		for _, f := range flags {
			if f == "-l" || f == "--list" {
				return ""
			}
		}
		return "Warning: This tag operation will modify repository state."
	case "remote":
		// Remote listing is safe
		return ""
	case "add":
		return "Warning: This add will stage changes for commit."
	case "rm":
		return "Warning: This rm will remove files from the working tree and/or index."
	case "mv":
		return "Warning: This mv will move/rename files."
	case "cherry-pick":
		return "Warning: This cherry-pick will apply commits to the current branch."
	case "revert":
		return "Warning: This revert will create new commits that undo changes."
	case "worktree":
		return "Warning: This worktree operation will modify linked working trees."
	default:
		return ""
	}
}

func (*GitTool) Execute(params map[string]interface{}) ToolResult {
	return gitExecute(context.Background(), params)
}

func (*GitTool) ExecuteContext(ctx context.Context, params map[string]interface{}) ToolResult {
	return gitExecute(ctx, params)
}

func gitExecute(ctx context.Context, params map[string]interface{}) ToolResult {
	operation, _ := params["operation"].(string)
	if operation == "" {
		return ToolResult{Output: "Error: operation is required", IsError: true}
	}

	// Handle "info" operation specially — it uses utility functions, not git CLI
	if operation == "info" {
		var workDir string
		if dir, _ := params["directory"].(string); dir != "" {
			workDir = dir
		} else {
			workDir, _ = params["path"].(string)
		}
		return executeGitInfo(workDir)
	}

	// Validate flags before executing (git operations only, not gh)
	if operation != "gh" {
		flags := getStringArray(params, "flags")
		if err := validateGitFlags(operation, flags); err != nil {
			return ToolResult{Output: fmt.Sprintf("Flag validation error: %v", err), IsError: true}
		}
	}

	// Determine working directory:
	// - For clone: use directory param (path is the clone destination, not workdir)
	// - For other operations: use directory param if set, otherwise path param
	var workDir string
	if operation == "clone" {
		workDir, _ = params["directory"].(string)
	} else {
		if dir, _ := params["directory"].(string); dir != "" {
			workDir = dir
		} else {
			workDir, _ = params["path"].(string)
		}
	}

	// Check remote configuration for operations that need it
	if operation == "push" || operation == "pull" || operation == "fetch" {
		remote, _ := params["remote"].(string)
		if remote == "" {
			// Check if there's an origin remote configured
			checkOut, _, _ := runGitCommandWithExitCode(ctx, []string{"remote"}, workDir, "")
			if checkOut == "" {
				return ToolResult{
					Output:  "Error: cannot determine git remotes",
					IsError: true,
				}
			}
			if !strings.Contains(checkOut, "origin") {
				return ToolResult{
					Output:  fmt.Sprintf("Error: no remote specified and no 'origin' remote found. Available remotes:\n%s", checkOut),
					IsError: true,
				}
			}
		}
	}

	cmd, err := buildGitCommand(params)
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("Error building command: %v", err), IsError: true}
	}

	// Get proxy from params
	proxy, _ := params["proxy"].(string)

	var out string
	var exitCode int

	if operation == "gh" {
		out, exitCode, _ = runGHCommand(ctx, cmd, workDir, proxy)
	} else {
		out, exitCode, _ = runGitCommandWithExitCode(ctx, cmd, workDir, proxy)
	}

	if exitCode != 0 {
		binary := "git"
		if operation == "gh" {
			binary = "gh"
		}
		return ToolResult{
			Output:  fmt.Sprintf("Error executing '%s %s' (exit code: %d)\n\nOutput:\n%s", binary, strings.Join(cmd, " "), exitCode, out),
			IsError: true,
		}
	}

	return ToolResult{Output: out, IsError: false}
}

// ---------------------------------------------------------------------------
// Git flag-level safety validation
// ---------------------------------------------------------------------------

// gitFlagType classifies how a flag's argument should be validated
type gitFlagType int

const (
	gitFlagNone   gitFlagType = iota // No argument (--stat, --oneline)
	gitFlagNumber                    // Integer argument (-n, --max-count=N)
	gitFlagString                    // Any string argument (--author, --grep)
)

// gitFlagConfig maps flag names to their expected argument types
type gitFlagConfig map[string]gitFlagType

// Per-subcommand safe flag maps
var (
	gitDiffFlags = gitFlagConfig{
		"--cached":               gitFlagNone,
		"--staged":               gitFlagNone,
		"--stat":                 gitFlagNone,
		"--stat-width":           gitFlagString,
		"--name-only":            gitFlagNone,
		"--name-status":          gitFlagNone,
		"--stat-only":            gitFlagNone,
		"--color":                gitFlagString,
		"--no-color":             gitFlagNone,
		"--color=always":         gitFlagNone,
		"--color=never":          gitFlagNone,
		"--unified":              gitFlagString,
		"--diff-filter":          gitFlagString,
		"--ignore-space-at-eol":  gitFlagNone,
		"--ignore-space-change":  gitFlagNone,
		"--ignore-all-space":     gitFlagNone,
		"--ignore-blank-lines":   gitFlagNone,
		"--inter-hunk-context":   gitFlagString,
		"--function-context":     gitFlagNone,
		"--exit-code":            gitFlagNone,
		"--quiet":                gitFlagNone,
		"--patch-with-stat":      gitFlagNone,
		"--word-diff":            gitFlagNone,
		"--word-diff-regex":      gitFlagString,
		"--color-words":          gitFlagNone,
		"--no-renames":           gitFlagNone,
		"--check":                gitFlagNone,
		"--full-index":           gitFlagNone,
		"--binary":               gitFlagNone,
		"--abbrev":               gitFlagString,
		"--find-renames":         gitFlagNone,
		"--find-copies":          gitFlagNone,
		"--find-copies-harder":   gitFlagNone,
		"--irreversible-delete":  gitFlagNone,
		"--diff-algorithm":       gitFlagString,
		"--histogram":            gitFlagNone,
		"--patience":             gitFlagNone,
		"--minimal":              gitFlagNone,
		"--relative":             gitFlagString,
		"-p":                     gitFlagNone,
		"-u":                     gitFlagNone,
		"-s":                     gitFlagNone,
		"-M":                     gitFlagNone,
		"-C":                     gitFlagNone,
		"-B":                     gitFlagNone,
		"-D":                     gitFlagNone,
		"-l":                     gitFlagNone,
		"-R":                     gitFlagNone,
		"-S":                     gitFlagString, // pickaxe search
		"-G":                     gitFlagString, // pickaxe regex
	}

	gitLogFlags = gitFlagConfig{
		"--oneline":       gitFlagNone,
		"--graph":         gitFlagNone,
		"--decorate":      gitFlagNone,
		"--no-decorate":   gitFlagNone,
		"--date":          gitFlagString,
		"--relative-date": gitFlagNone,
		"--all":           gitFlagNone,
		"--branches":      gitFlagNone,
		"--tags":          gitFlagNone,
		"--remotes":       gitFlagNone,
		"--max-count":     gitFlagString,
		"-n":              gitFlagString,
		"--since":         gitFlagString,
		"--after":         gitFlagString,
		"--until":         gitFlagString,
		"--before":        gitFlagString,
		"--author":        gitFlagString,
		"--committer":     gitFlagString,
		"--grep":          gitFlagString,
		"--no-merges":     gitFlagNone,
		"--merges":        gitFlagNone,
		"--format":        gitFlagString,
		"--pretty":        gitFlagString,
		"--stat":          gitFlagNone,
		"--name-status":   gitFlagNone,
		"--name-only":     gitFlagNone,
		"--diff-filter":   gitFlagString,
		"--reverse":       gitFlagNone,
		"--skip":          gitFlagString,
		"--follow":        gitFlagNone,
		"--left-right":    gitFlagNone,
		"--source":        gitFlagNone,
		"--first-parent":  gitFlagNone,
		"--topo-order":    gitFlagNone,
		"--date-order":    gitFlagNone,
		"--abbrev-commit": gitFlagNone,
		"--patch":         gitFlagNone,
		"-p":              gitFlagNone,
		"-s":              gitFlagNone,
		"--no-patch":      gitFlagNone,
		"-S":              gitFlagString,
		"-G":              gitFlagString,
	}

	gitShowFlags = gitFlagConfig{
		"--oneline":         gitFlagNone,
		"--graph":           gitFlagNone,
		"--decorate":         gitFlagNone,
		"--date":            gitFlagString,
		"--stat":            gitFlagNone,
		"--name-status":     gitFlagNone,
		"--name-only":       gitFlagNone,
		"--color":           gitFlagString,
		"--no-color":        gitFlagNone,
		"--patch":           gitFlagNone,
		"-p":                gitFlagNone,
		"--no-patch":        gitFlagNone,
		"--diff-filter":     gitFlagString,
		"--format":          gitFlagString,
		"--pretty":          gitFlagString,
		"--abbrev-commit":   gitFlagNone,
		"--first-parent":    gitFlagNone,
		"--raw":             gitFlagNone,
		"-m":                gitFlagNone,
		"--quiet":           gitFlagNone,
		"-s":                gitFlagNone,
		"--word-diff":       gitFlagNone,
		"--word-diff-regex": gitFlagString,
		"--color-words":     gitFlagNone,
		"-S":                gitFlagString,
		"-G":                gitFlagString,
	}

	gitStatusFlags = gitFlagConfig{
		"--short":             gitFlagNone,
		"-s":                 gitFlagNone,
		"--branch":            gitFlagNone,
		"-b":                 gitFlagNone,
		"--porcelain":         gitFlagNone,
		"--porcelain=2":       gitFlagNone,
		"--long":              gitFlagNone,
		"--verbose":           gitFlagNone,
		"-v":                  gitFlagNone,
		"--untracked-files":   gitFlagString,
		"-u":                  gitFlagString,
		"--ignored":           gitFlagNone,
		"--ignore-submodules": gitFlagString,
		"--renames":           gitFlagNone,
		"--no-renames":        gitFlagNone,
		"--find-renames":      gitFlagString,
		"-M":                 gitFlagString,
		"--column":            gitFlagNone,
		"--no-column":         gitFlagNone,
		"--ahead-behind":      gitFlagNone,
		"--no-ahead-behind":   gitFlagNone,
	}

	gitBranchFlags = gitFlagConfig{
		"-l":             gitFlagNone,
		"--list":         gitFlagNone,
		"-a":             gitFlagNone,
		"--all":          gitFlagNone,
		"-r":             gitFlagNone,
		"--remotes":      gitFlagNone,
		"-v":             gitFlagNone,
		"-vv":            gitFlagNone,
		"--verbose":      gitFlagNone,
		"--color":        gitFlagNone,
		"--no-color":     gitFlagNone,
		"--column":       gitFlagNone,
		"--no-column":    gitFlagNone,
		"--contains":     gitFlagString,
		"--no-contains":  gitFlagString,
		"--merged":       gitFlagNone,
		"--no-merged":    gitFlagNone,
		"--points-at":    gitFlagString,
		"--sort":         gitFlagString,
		"--show-current": gitFlagNone,
		"--abbrev":       gitFlagString,
	}

	gitStashListFlags = gitFlagConfig{
		"--oneline":   gitFlagNone,
		"--graph":    gitFlagNone,
		"--decorate": gitFlagNone,
		"--all":      gitFlagNone,
		"-n":         gitFlagString,
		"--max-count": gitFlagString,
	}

	gitReflogShowFlags = gitFlagConfig{
		"--oneline":   gitFlagNone,
		"--graph":     gitFlagNone,
		"--decorate":  gitFlagNone,
		"--date":      gitFlagString,
		"--all":       gitFlagNone,
		"-n":          gitFlagString,
		"--max-count": gitFlagString,
		"--author":    gitFlagString,
		"--committer": gitFlagString,
		"--grep":      gitFlagString,
	}

	gitTagFlags = gitFlagConfig{
		"-l":            gitFlagNone,
		"--list":        gitFlagNone,
		"-n":            gitFlagString,
		"--contains":    gitFlagString,
		"--no-contains": gitFlagString,
		"--merged":      gitFlagString,
		"--no-merged":   gitFlagString,
		"--sort":        gitFlagString,
		"--format":      gitFlagString,
		"--points-at":   gitFlagString,
		"--column":      gitFlagNone,
		"--no-column":   gitFlagNone,
	}

	gitRemoteFlags = gitFlagConfig{
		"-v":        gitFlagNone,
		"--verbose": gitFlagNone,
	}

	// reset flags: --soft, --mixed, --merge allowed; --hard intentionally excluded
	gitResetFlags = gitFlagConfig{
		"--soft":   gitFlagNone,
		"--mixed":  gitFlagNone,
		"--merge":  gitFlagNone,
		"--keep":   gitFlagNone,
		"--quiet":  gitFlagNone,
		"-q":       gitFlagNone,
		"--patch":  gitFlagNone,
	}

	gitMergeFlags = gitFlagConfig{
		"--no-ff":        gitFlagNone,
		"--squash":       gitFlagNone,
		"--abort":        gitFlagNone,
		"--continue":     gitFlagNone,
		"--quit":         gitFlagNone,
		"--no-commit":    gitFlagNone,
		"--edit":         gitFlagNone,
		"-e":             gitFlagNone,
		"--no-edit":      gitFlagNone,
		"--message":      gitFlagString,
		"-m":             gitFlagString,
		"--quiet":        gitFlagNone,
		"-q":             gitFlagNone,
		"--verbose":      gitFlagNone,
		"-v":             gitFlagNone,
		"--no-verify":    gitFlagNone,
		"-X":             gitFlagString,
		"--strategy-option": gitFlagString,
	}

	gitRebaseFlags = gitFlagConfig{
		"--interactive":   gitFlagNone,
		"-i":              gitFlagNone,
		"--onto":          gitFlagString,
		"--continue":      gitFlagNone,
		"--abort":         gitFlagNone,
		"--skip":          gitFlagNone,
		"--quit":          gitFlagNone,
		"--autosquash":    gitFlagNone,
		"--no-autosquash": gitFlagNone,
		"--autostash":     gitFlagNone,
		"--no-autostash":  gitFlagNone,
		"--whitespace":    gitFlagString,
		"--verify":        gitFlagNone,
		"--no-verify":     gitFlagNone,
		"-q":              gitFlagNone,
		"--quiet":         gitFlagNone,
		"--verbose":       gitFlagNone,
		"-v":              gitFlagNone,
		"--force-rebase":  gitFlagNone,
		"-m":              gitFlagNone,
		"--root":          gitFlagNone,
	}

	gitStashPushFlags = gitFlagConfig{
		"--index":              gitFlagNone,
		"--no-index":           gitFlagNone,
		"--include-untracked":  gitFlagNone,
		"-u":                   gitFlagNone,
		"--all":                gitFlagNone,
		"--patch":              gitFlagNone,
		"--message":            gitFlagString,
		"--keep-index":         gitFlagNone,
		"--no-keep-index":      gitFlagNone,
		"--quiet":              gitFlagNone,
		"-q":                   gitFlagNone,
		"--staged":             gitFlagNone,
	}

	// push flags: --force intentionally excluded, --force-with-lease allowed
	gitPushFlags = gitFlagConfig{
		"--force-with-lease":     gitFlagNone,
		"--dry-run":              gitFlagNone,
		"--quiet":                gitFlagNone,
		"-q":                     gitFlagNone,
		"--verbose":              gitFlagNone,
		"-v":                     gitFlagNone,
		"--delete":               gitFlagNone,
		"-d":                     gitFlagNone,
		"--all":                  gitFlagNone,
		"--mirror":               gitFlagNone,
		"--tags":                 gitFlagNone,
		"--set-upstream":         gitFlagNone,
		"-u":                     gitFlagNone,
		"--set-upstream-to":      gitFlagString,
		"--force-if-includes":    gitFlagNone,
		"--no-force-if-includes": gitFlagNone,
		"--receive-pack":         gitFlagString,
		"--exec":                 gitFlagString,
		"--push-option":          gitFlagString,
		"-o":                     gitFlagString,
		"--thin":                 gitFlagNone,
		"--no-thin":              gitFlagNone,
		"--follow-tags":          gitFlagNone,
		"--signed":               gitFlagString,
		"--no-signed":            gitFlagNone,
		"--atomic":               gitFlagNone,
		"--no-atomic":            gitFlagNone,
	}

	gitPullFlags = gitFlagConfig{
		"--rebase":                gitFlagNone,
		"--no-rebase":             gitFlagNone,
		"-r":                      gitFlagNone,
		"--ff-only":               gitFlagNone,
		"--ff":                    gitFlagNone,
		"--no-ff":                 gitFlagNone,
		"--rerere-autoupdate":     gitFlagNone,
		"--no-rerere-autoupdate":  gitFlagNone,
		"--quiet":                 gitFlagNone,
		"-q":                      gitFlagNone,
		"--verbose":               gitFlagNone,
		"-v":                      gitFlagNone,
		"--no-commit":             gitFlagNone,
		"--no-edit":               gitFlagNone,
		"--edit":                  gitFlagNone,
		"-e":                      gitFlagNone,
		"--autostash":             gitFlagNone,
		"--no-autostash":          gitFlagNone,
		"--verify":                gitFlagNone,
		"--no-verify":             gitFlagNone,
		"--stat":                  gitFlagNone,
		"--no-stat":               gitFlagNone,
		"-s":                      gitFlagString,
		"--strategy":              gitFlagString,
		"-X":                      gitFlagString,
		"--strategy-option":       gitFlagString,
		"--gpg-sign":              gitFlagString,
		"--no-gpg-sign":           gitFlagNone,
		"--allow-unrelated-histories":    gitFlagNone,
		"--no-allow-unrelated-histories": gitFlagNone,
	}
)

// gitFlagRegex matches valid flag tokens
var gitFlagRegex = regexp.MustCompile(`^-[a-zA-Z0-9_-]`)

// getGitFlagConfig returns the safe flags config for a given git operation
func getGitFlagConfig(operation string) gitFlagConfig {
	switch operation {
	case "diff":
		return gitDiffFlags
	case "log":
		return gitLogFlags
	case "show":
		return gitShowFlags
	case "status":
		return gitStatusFlags
	case "branch":
		return gitBranchFlags
	case "reflog":
		return gitReflogShowFlags
	case "tag":
		return gitTagFlags
	case "remote":
		return gitRemoteFlags
	case "reset":
		return gitResetFlags
	case "merge":
		return gitMergeFlags
	case "rebase":
		return gitRebaseFlags
	case "push":
		return gitPushFlags
	case "pull":
		return gitPullFlags
	default:
		return nil
	}
}

// validateGitFlags validates flags from the `flags` parameter against the
// per-subcommand whitelist. Returns nil if all flags are valid.
func validateGitFlags(operation string, flags []string) error {
	config := getGitFlagConfig(operation)
	if config == nil {
		// Operations without a specific flag config (clone, init, add, etc.)
		// only accept flags from params, not from arbitrary user input.
		return nil
	}

	for i, flag := range flags {
		if flag == "" {
			continue
		}

		// Handle -- separator: everything after is positional args
		if flag == "--" {
			break
		}

		// Handle numeric shorthand like -20 for log/show/reflog
		if len(flag) > 1 && flag[0] == '-' && flag[1] >= '0' && flag[1] <= '9' {
			// Pure numeric: -20, -5, etc.
			if operation == "log" || operation == "show" || operation == "reflog" {
				continue
			}
			return fmt.Errorf("invalid flag '%s' for git %s: numeric shorthand not allowed", flag, operation)
		}

		// Skip non-flag tokens (positional arguments like file paths, commit refs)
		if !strings.HasPrefix(flag, "-") {
			continue
		}

		// Parse the flag
		flagName := flag
		hasEquals := strings.Contains(flag, "=")
		if hasEquals {
			flagName = strings.SplitN(flag, "=", 2)[0]
		}

		// Check against the whitelist (exact match first, then prefix)
		expectedType, exists := config[flag]
		if !exists {
			// Try the flagName without = (for --flag=value)
			expectedType, exists = config[flagName]
		}
		if !exists {
			return fmt.Errorf("invalid flag '%s' for git %s: flag not in allowed list", flag, operation)
		}

		// Validate based on expected type
		switch expectedType {
		case gitFlagNone:
			if hasEquals {
				return fmt.Errorf("flag '%s' does not take an argument", flagName)
			}
		case gitFlagNumber:
			var argValue string
			if hasEquals {
				argValue = strings.TrimPrefix(flag, flagName+"=")
			} else if i+1 < len(flags) {
				argValue = flags[i+1]
				i++ // consume the argument token
			}
			if argValue == "" {
				return fmt.Errorf("flag '%s' requires a numeric argument", flagName)
			}
			for _, c := range argValue {
				if c < '0' || c > '9' {
					return fmt.Errorf("flag '%s' requires a numeric argument, got '%s'", flagName, argValue)
				}
			}
		case gitFlagString:
			// String args are always valid
			if !hasEquals && i+1 < len(flags) {
				i++ // consume the argument token
			}
		}
	}

	return nil
}

// isDangerousOperation checks if the operation + flags combination is explicitly dangerous
func isDangerousOperation(operation string, flags []string) (bool, string) {
	switch operation {
	case "reset":
		for _, f := range flags {
			if f == "--hard" || f == "-h" {
				return true, "DANGEROUS: git reset --hard will discard ALL uncommitted changes. This is irreversible and will cause data loss."
			}
		}
	case "push":
		for _, f := range flags {
			if f == "--force" || f == "-f" {
				return true, "DANGEROUS: git push --force will overwrite remote history, potentially discarding commits from collaborators. Use --force-with-lease instead."
			}
		}
	case "clean":
		for _, f := range flags {
			if f == "-f" || f == "--force" {
				return true, "DANGEROUS: git clean -f will permanently delete untracked files from the working directory. This action cannot be undone."
			}
		}
	case "branch":
		for _, f := range flags {
			if f == "-D" {
				return true, "DANGEROUS: git branch -D will force-delete a branch without checking if it is merged. This can cause data loss."
			}
		}
	}
	return false, ""
}

func buildGitCommand(params map[string]interface{}) ([]string, error) {
	operation, _ := params["operation"].(string)
	var args []string

	switch operation {
	case "clone":
		repo, _ := params["repo"].(string)
		if repo == "" {
			return nil, fmt.Errorf("repo is required for clone")
		}
		args = []string{"clone"}
		args = append(args, repo)
		if path, _ := params["path"].(string); path != "" {
			args = append(args, path)
		}

	case "init":
		args = []string{"init"}
		if path, _ := params["path"].(string); path != "" {
			args = append(args, path)
		}

	case "add":
		args = []string{"add"}
		if all, _ := params["all"].(bool); all {
			args = append(args, "-A")
		} else if files := getStringArray(params, "files"); len(files) > 0 {
			args = append(args, files...)
		} else {
			args = append(args, ".")
		}

	case "commit":
		message, _ := params["message"].(string)
		if message == "" {
			return nil, fmt.Errorf("commit message is required")
		}
		if all, _ := params["all"].(bool); all {
			args = []string{"commit", "-a", "-m", message}
		} else {
			args = []string{"commit", "-m", message}
		}
		if author, _ := params["author"].(string); author != "" {
			args = append(args, "--author", author)
		}

	case "push":
		args = []string{"push"}
		if remote, _ := params["remote"].(string); remote != "" {
			args = append(args, remote)
			if branch, _ := params["branch"].(string); branch != "" {
				args = append(args, branch)
			}
		}

	case "pull":
		args = []string{"pull"}
		if remote, _ := params["remote"].(string); remote != "" {
			args = append(args, remote)
			if branch, _ := params["branch"].(string); branch != "" {
				args = append(args, branch)
			}
		}

	case "fetch":
		args = []string{"fetch"}
		if remote, _ := params["remote"].(string); remote != "" {
			args = append(args, remote)
		}

	case "branch":
		args = []string{"branch"}
		if name, _ := params["branch"].(string); name != "" {
			args = append(args, name)
		}

	case "checkout":
		args = []string{"checkout"}
		// Handle -b/-B early so they come before branch name: `git checkout -b <branch>`
		if flags := getStringArray(params, "flags"); len(flags) > 0 {
			for _, f := range flags {
				if f == "-b" || f == "-B" {
					args = append(args, "-b")
					break
				}
			}
		}
		if ot, _ := params["ours_theirs"].(string); ot != "" {
			args = append(args, "--"+ot)
		}
		if branch, _ := params["branch"].(string); branch != "" {
			args = append(args, branch)
		}
		return args, nil // Return early to skip generic flags loop

	case "merge":
		args = []string{"merge"}
		if message, _ := params["message"].(string); message != "" {
			args = append(args, "-m", message)
		}
		if target, _ := params["target"].(string); target != "" {
			args = append(args, target)
		}

	case "rebase":
		args = []string{"rebase"}
		if target, _ := params["target"].(string); target != "" {
			args = append(args, target)
		}

	case "reset":
		args = []string{"reset"}

	case "tag":
		args = []string{"tag"}
		if branch, _ := params["branch"].(string); branch != "" {
			args = append(args, branch)
		}

	case "remote":
		args = []string{"remote", "-v"}

	case "status":
		args = []string{"status"}

	case "diff":
		args = []string{"diff"}
		if getCached, ok := params["cached"].(bool); ok && getCached {
			args = append(args, "--cached")
		}
		if files := getStringArray(params, "files"); len(files) > 0 {
			args = append(args, files...)
		}

	case "log":
		n := 20
		if mc, ok := params["max_count"].(float64); ok {
			n = int(mc)
		}
		args = []string{"log", fmt.Sprintf("-%d", n), "--oneline"}

	case "show":
		args = []string{"show"}
		if target, _ := params["target"].(string); target != "" {
			args = append(args, target)
		}

	case "describe":
		args = []string{"describe"}
		if target, _ := params["target"].(string); target != "" {
			args = append(args, target)
		}

	case "ls-files":
		args = []string{"ls-files"}
		if files := getStringArray(params, "files"); len(files) > 0 {
			args = append(args, files...)
		}

	case "ls-tree":
		args = []string{"ls-tree"}
		if target, _ := params["target"].(string); target != "" {
			args = append(args, target)
		} else {
			args = append(args, "HEAD")
		}

	case "rev-parse":
		args = []string{"rev-parse"}
		if target, _ := params["target"].(string); target != "" {
			args = append(args, target)
		}

	case "rev-list":
		n := 20
		if mc, ok := params["max_count"].(float64); ok {
			n = int(mc)
		}
		args = []string{"rev-list", fmt.Sprintf("-%d", n), "--count"}

	case "worktree":
		subOp := ""
		if remove, _ := params["worktree_remove"].(bool); remove {
			subOp = "remove"
		} else if name, _ := params["worktree_name"].(string); name != "" {
			subOp = "add"
		} else {
			subOp = "list"
		}
		args = []string{"worktree", subOp}
		switch subOp {
		case "add":
			if path, _ := params["path"].(string); path != "" {
				args = append(args, path)
			}
			if branch, _ := params["worktree_branch"].(string); branch != "" {
				args = append(args, "-b", branch)
			}
		case "remove":
			if name, _ := params["worktree_name"].(string); name != "" {
				args = append(args, name)
			}
		}

	case "rm":
		args = []string{"rm"}
		if force, _ := params["force"].(bool); force {
			args = append(args, "-f")
		}
		if staged, _ := params["staged"].(bool); staged {
			args = append(args, "--cached")
		}
		if files := getStringArray(params, "files"); len(files) > 0 {
			args = append(args, files...)
		} else {
			return nil, fmt.Errorf("files are required for rm")
		}

	case "mv":
		source, _ := params["source"].(string)
		if source == "" {
			return nil, fmt.Errorf("source is required for mv")
		}
		dest, _ := params["path"].(string)
		if dest == "" {
			return nil, fmt.Errorf("path (destination) is required for mv")
		}
		args = []string{"mv"}
		if force, _ := params["force"].(bool); force {
			args = append(args, "-f")
		}
		args = append(args, source, dest)

	case "restore":
		args = []string{"restore"}
		if staged, _ := params["staged"].(bool); staged {
			args = append(args, "--staged")
		}
		if force, _ := params["force"].(bool); force {
			args = append(args, "--force")
		}
		if files := getStringArray(params, "files"); len(files) > 0 {
			args = append(args, files...)
		} else {
			args = append(args, ".")
		}

	case "switch":
		args = []string{"switch"}
		if force, _ := params["force"].(bool); force {
			args = append(args, "--force")
		}
		if source, _ := params["source"].(string); source != "" {
			args = append(args, source)
		} else if branch, _ := params["branch"].(string); branch != "" {
			args = append(args, branch)
		} else {
			return nil, fmt.Errorf("branch or source is required for switch")
		}

	case "cherry-pick":
		target, _ := params["target"].(string)
		if target == "" {
			return nil, fmt.Errorf("target (commit) is required for cherry-pick")
		}
		args = []string{"cherry-pick"}
		if mainline, ok := params["mainline"].(float64); ok {
			args = append(args, "-m", fmt.Sprintf("%d", int(mainline)))
		}
		if noCommit, _ := params["no_commit"].(bool); noCommit {
			args = append(args, "--no-commit")
		}
		args = append(args, target)

	case "revert":
		target, _ := params["target"].(string)
		if target == "" {
			return nil, fmt.Errorf("target (commit) is required for revert")
		}
		args = []string{"revert"}
		if mainline, ok := params["mainline"].(float64); ok {
			args = append(args, "-m", fmt.Sprintf("%d", int(mainline)))
		}
		if noEdit, _ := params["no_edit"].(bool); noEdit {
			args = append(args, "--no-edit")
		}
		args = append(args, target)

	case "clean":
		args = []string{"clean"}
		if force, _ := params["force"].(bool); force {
			args = append(args, "-f")
		}
		if dryRun, _ := params["dry_run"].(bool); dryRun {
			args = append(args, "-n")
		}
		if recursive, _ := params["recursive"].(bool); recursive {
			args = append(args, "-d")
		}

	case "blame":
		if file, _ := params["path"].(string); file != "" {
			args = []string{"blame", file}
		} else if files := getStringArray(params, "files"); len(files) > 0 {
			args = []string{"blame"}
			args = append(args, files...)
		} else {
			return nil, fmt.Errorf("path or files is required for blame (file path)")
		}

	case "reflog":
		args = []string{"reflog"}
		if sub, _ := params["branch"].(string); sub != "" {
			args = append(args, "show", sub)
		} else {
			args = append(args, "show")
		}
		n := 20
		if mc, ok := params["max_count"].(float64); ok {
			n = int(mc)
		}
		args = append(args, fmt.Sprintf("-%d", n))

	case "stash":
		args = []string{"stash"}
		if sub, _ := params["stash_subcommand"].(string); sub != "" {
			if sub == "pop" || sub == "apply" || sub == "drop" || sub == "list" || sub == "show" {
				args = append(args, sub)
			}
		}
		if includeUntracked, _ := params["stash_include_untracked"].(bool); includeUntracked {
			args = append(args, "-u")
		}

	case "shortlog":
		n := 20
		if mc, ok := params["max_count"].(float64); ok {
			n = int(mc)
		}
		args = []string{"shortlog", "-sn", fmt.Sprintf("-%d", n), "HEAD"}

	case "gh":
		ghCmd := getStringArray(params, "gh_command")
		if len(ghCmd) == 0 {
			return nil, fmt.Errorf("gh_command is required for gh operation (e.g. [\"pr\", \"view\", \"123\"])")
		}
		return buildGHCommand(ghCmd)

	default:
		return nil, fmt.Errorf("unknown operation: %s", operation)
	}

	if flags := getStringArray(params, "flags"); len(flags) > 0 {
		args = append(args, flags...)
	}

	return args, nil
}

// ---------------------------------------------------------------------------
// Shared utility functions
// ---------------------------------------------------------------------------

func runGitCommand(ctx context.Context, args []string, workDir string, proxy string) (string, int, error) {
	return runGitCommandWithExitCode(ctx, args, workDir, proxy)
}

func runGitCommandWithExitCode(ctx context.Context, args []string, workDir string, proxy string) (string, int, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	if proxy != "" {
		cmd.Env = append(cmd.Environ(),
			"https_proxy="+proxy,
			"http_proxy="+proxy,
			"HTTPS_PROXY="+proxy,
			"HTTP_PROXY="+proxy,
		)
	}
	if workDir != "" {
		cmd.Dir = workDir
	}
	out, err := cmd.CombinedOutput()

	exitCode := 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		exitCode = exitErr.ExitCode()
	}
	// Don't return err -- let caller decide how to handle non-zero exit codes
	return strings.TrimSpace(string(out)), exitCode, nil
}

func getStringArray(params map[string]interface{}, key string) []string {
	if v, ok := params[key]; ok {
		switch arr := v.(type) {
		case []interface{}:
			result := make([]string, 0, len(arr))
			for _, item := range arr {
				if s, ok := item.(string); ok {
					result = append(result, s)
				}
			}
			return result
		case []string:
			return arr
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// GitHub CLI (gh) read-only command support
// ---------------------------------------------------------------------------

// ghFlagType classifies how a flag's argument should be validated.
type ghFlagType int

const (
	ghFlagNone   ghFlagType = iota // No argument (e.g. --comments, --web)
	ghFlagString                   // Any string argument
	ghFlagNumber                   // Integer argument
)

// ghFlagSpec describes a single allowed flag.
type ghFlagSpec struct {
	argType ghFlagType
}

// GHSafeFlags maps "subcommand" -> map of allowed flags.
// Only flags listed here are permitted; all others are rejected.
var GHSafeFlags = map[string]map[string]ghFlagSpec{
	"pr view": {
		"--json":     {ghFlagString},
		"--comments": {ghFlagNone},
		"--web":      {ghFlagNone},
		"--repo":     {ghFlagString},
		"-R":         {ghFlagString},
	},
	"pr list": {
		"--state":  {ghFlagString},
		"-s":       {ghFlagString},
		"--author": {ghFlagString},
		"--label":  {ghFlagString},
		"--limit":  {ghFlagNumber},
		"-L":       {ghFlagNumber},
		"--json":   {ghFlagString},
		"--repo":   {ghFlagString},
		"-R":       {ghFlagString},
	},
	"pr diff": {
		"--name-only": {ghFlagNone},
		"--json":      {ghFlagString},
		"--repo":      {ghFlagString},
		"-R":          {ghFlagString},
	},
	"pr checks": {
		"--required": {ghFlagNone},
		"--json":     {ghFlagString},
		"--repo":     {ghFlagString},
		"-R":         {ghFlagString},
	},
	"pr status": {
		"--conflict-status": {ghFlagNone},
		"-c":                {ghFlagNone},
		"--json":            {ghFlagString},
		"--repo":            {ghFlagString},
		"-R":                {ghFlagString},
	},
	"issue view": {
		"--json":     {ghFlagString},
		"--comments": {ghFlagNone},
		"--repo":     {ghFlagString},
		"-R":         {ghFlagString},
	},
	"issue list": {
		"--state":  {ghFlagString},
		"-s":       {ghFlagString},
		"--author": {ghFlagString},
		"--label":  {ghFlagString},
		"--limit":  {ghFlagNumber},
		"-L":       {ghFlagNumber},
		"--json":   {ghFlagString},
		"--repo":   {ghFlagString},
		"-R":       {ghFlagString},
	},
	"issue status": {
		"--json": {ghFlagString},
		"--repo": {ghFlagString},
		"-R":     {ghFlagString},
	},
	"run list": {
		"--status":   {ghFlagString},
		"-s":         {ghFlagString},
		"--workflow": {ghFlagString},
		"-w":         {ghFlagString},
		"--limit":    {ghFlagNumber},
		"-L":         {ghFlagNumber},
		"--json":     {ghFlagString},
		"--repo":     {ghFlagString},
		"-R":         {ghFlagString},
		"--branch":   {ghFlagString},
		"-b":         {ghFlagString},
	},
	"run view": {
		"--log":         {ghFlagNone},
		"--log-failed":  {ghFlagNone},
		"--json":        {ghFlagString},
		"--repo":        {ghFlagString},
		"-R":            {ghFlagString},
		"--job":         {ghFlagString},
		"-j":            {ghFlagString},
	},
	"auth status": {
		"--json": {ghFlagString},
	},
	"release list": {
		"--json":  {ghFlagString},
		"--limit": {ghFlagNumber},
		"-L":      {ghFlagNumber},
		"--repo":  {ghFlagString},
		"-R":      {ghFlagString},
	},
	"release view": {
		"--json": {ghFlagString},
		"--repo": {ghFlagString},
		"-R":     {ghFlagString},
	},
	"search repos": {
		"--json":     {ghFlagString},
		"--limit":    {ghFlagNumber},
		"-L":         {ghFlagNumber},
		"--owner":    {ghFlagString},
		"--language": {ghFlagString},
		"--stars":    {ghFlagString},
	},
	"search issues": {
		"--json":   {ghFlagString},
		"--limit":  {ghFlagNumber},
		"-L":       {ghFlagNumber},
		"--state":  {ghFlagString},
		"--repo":   {ghFlagString},
		"-R":       {ghFlagString},
		"--owner":  {ghFlagString},
		"--label":  {ghFlagString},
		"--author": {ghFlagString},
	},
	"search prs": {
		"--json":   {ghFlagString},
		"--limit":  {ghFlagNumber},
		"-L":       {ghFlagNumber},
		"--state":  {ghFlagString},
		"--repo":   {ghFlagString},
		"-R":       {ghFlagString},
		"--owner":  {ghFlagString},
		"--label":  {ghFlagString},
		"--author": {ghFlagString},
	},
}

// ghSubcommand extracts the subcommand key from gh_command tokens.
func ghSubcommand(tokens []string) string {
	if len(tokens) >= 3 {
		candidate := tokens[0] + " " + tokens[1]
		if _, ok := GHSafeFlags[candidate]; ok {
			return candidate
		}
	}
	if len(tokens) >= 2 {
		return tokens[0] + " " + tokens[1]
	}
	if len(tokens) == 1 {
		return tokens[0]
	}
	return ""
}

// ghIsDangerousRepo checks if any token contains a dangerous --repo value.
func ghIsDangerousRepo(tokens []string) bool {
	for _, token := range tokens {
		if token == "" {
			continue
		}
		value := token
		if strings.HasPrefix(token, "-") {
			eqIdx := strings.Index(token, "=")
			if eqIdx == -1 {
				continue
			}
			value = token[eqIdx+1:]
			if value == "" {
				continue
			}
		}
		if !strings.Contains(value, "/") && !strings.Contains(value, "://") && !strings.Contains(value, "@") {
			continue
		}
		if strings.Contains(value, "://") {
			return true
		}
		if strings.Contains(value, "@") {
			return true
		}
		slashCount := strings.Count(value, "/")
		if slashCount >= 2 {
			return true
		}
	}
	return false
}

// ghFlagPattern matches valid flag names
var ghFlagPattern = regexp.MustCompile(`^-[a-zA-Z0-9_-]`)

// validateGHFlags validates that all flags in the token list are in the
// safe-flags whitelist for the given subcommand.
func validateGHFlags(subcmd string, tokens []string) error {
	allowed, ok := GHSafeFlags[subcmd]
	if !ok {
		return fmt.Errorf("gh subcommand not allowed: %q (only read-only commands are permitted)", subcmd)
	}

	skip := 2
	if strings.Count(subcmd, " ") == 2 {
		skip = 3
	}

	if ghIsDangerousRepo(tokens) {
		return fmt.Errorf("gh command contains dangerous --repo value: URLs, SSH-style, or HOST/OWNER/REPO formats are not allowed")
	}

	i := skip
	for i < len(tokens) {
		token := tokens[i]
		if token == "" {
			i++
			continue
		}

		if !strings.HasPrefix(token, "-") || !ghFlagPattern.MatchString(token) {
			i++
			continue
		}

		hasEquals := strings.Contains(token, "=")
		var flag string
		var inlineValue string
		if hasEquals {
			parts := strings.SplitN(token, "=", 2)
			flag = parts[0]
			inlineValue = parts[1]
		} else {
			flag = token
		}

		spec, found := allowed[flag]
		if !found {
			return fmt.Errorf("gh flag not allowed: %q (not in safe flags for %q)", flag, subcmd)
		}

		switch spec.argType {
		case ghFlagNone:
			if hasEquals {
				return fmt.Errorf("gh flag %q does not take an argument", flag)
			}
			i++
		case ghFlagString:
			if hasEquals {
				i++
			} else {
				if i+1 >= len(tokens) {
					return fmt.Errorf("gh flag %q requires an argument", flag)
				}
				i += 2
			}
		case ghFlagNumber:
			var argValue string
			if hasEquals {
				argValue = inlineValue
				i++
			} else {
				if i+1 >= len(tokens) {
					return fmt.Errorf("gh flag %q requires a numeric argument", flag)
				}
				argValue = tokens[i+1]
				i += 2
			}
			if _, err := strconv.Atoi(argValue); err != nil {
				return fmt.Errorf("gh flag %q requires a numeric argument, got %q", flag, argValue)
			}
		}
	}

	return nil
}

// buildGHCommand constructs the gh CLI args from the given gh_command tokens,
// validates them against the safe-flags whitelist, and returns the validated args.
func buildGHCommand(ghCmd []string) ([]string, error) {
	if len(ghCmd) == 0 {
		return nil, fmt.Errorf("gh_command is required for gh operation")
	}

	subcmd := ghSubcommand(ghCmd)
	if subcmd == "" {
		return nil, fmt.Errorf("gh_command must specify a subcommand (e.g. [\"pr\", \"view\", \"123\"])")
	}

	if err := validateGHFlags(subcmd, ghCmd); err != nil {
		return nil, err
	}

	return ghCmd, nil
}

// runGHCommand executes a gh CLI command with the given args.
func runGHCommand(ctx context.Context, args []string, workDir string, proxy string) (string, int, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	if proxy != "" {
		cmd.Env = append(cmd.Environ(),
			"https_proxy="+proxy,
			"http_proxy="+proxy,
			"HTTPS_PROXY="+proxy,
			"HTTP_PROXY="+proxy,
		)
	}
	if workDir != "" {
		cmd.Dir = workDir
	}
	out, err := cmd.CombinedOutput()

	exitCode := 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		exitCode = exitErr.ExitCode()
	}
	return strings.TrimSpace(string(out)), exitCode, nil
}

// ---------------------------------------------------------------------------
// Git utility functions
// ---------------------------------------------------------------------------

// FindGitRoot walks up from dir looking for .git directory or file (worktree case)
func FindGitRoot(dir string) (string, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path: %w", err)
	}

	for {
		gitPath := filepath.Join(absDir, ".git")
		fi, err := os.Stat(gitPath)
		if err == nil {
			if fi.IsDir() {
				return absDir, nil
			}
			// .git is a file (worktree case), read it to check for gitdir:
			data, readErr := os.ReadFile(gitPath)
			if readErr == nil && strings.HasPrefix(string(data), "gitdir:") {
				return absDir, nil
			}
		}

		parent := filepath.Dir(absDir)
		if parent == absDir {
			return "", fmt.Errorf("not in a git repository")
		}
		absDir = parent
	}
}

// GetBranch runs git branch --show-current to get the current branch name
func GetBranch(dir string) (string, error) {
	cmd := exec.Command("git", "-C", dir, "branch", "--show-current")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get branch: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// IsBareRepo checks if the repository is a bare repository
func IsBareRepo(dir string) bool {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--is-bare-repository")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "true"
}

// IsGitRepo checks if the directory is inside a git work tree
func IsGitRepo(dir string) bool {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--is-inside-work-tree")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "true"
}

// GetGitStatus runs git status --porcelain and returns a map of filename -> status code
func GetGitStatus(dir string) (map[string]string, error) {
	cmd := exec.Command("git", "-C", dir, "status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get git status: %w", err)
	}

	result := make(map[string]string)
	// Split on newlines; do NOT TrimSpace the whole output as that would
	// remove the leading space from status codes like " M" (modified in
	// worktree), breaking the fixed-column porcelain format.
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		// Trim trailing whitespace (e.g. \r on Windows) but preserve leading spaces
		line = strings.TrimRight(line, " \r\t")
		if len(line) < 3 {
			continue
		}
		status := line[:2]
		filename := line[3:] // Skip "XY " prefix (2 status chars + 1 space separator)
		result[filename] = status
	}
	return result, nil
}

// HasUncommittedChanges checks if there are uncommitted changes (staged or unstaged)
func HasUncommittedChanges(dir string) bool {
	// Check unstaged changes
	cmd := exec.Command("git", "-C", dir, "diff", "--quiet")
	if err := cmd.Run(); err != nil {
		return true
	}

	// Check staged changes
	cmd = exec.Command("git", "-C", dir, "diff", "--cached", "--quiet")
	if err := cmd.Run(); err != nil {
		return true
	}

	return false
}

// GetDefaultBranch returns the default branch name (origin/HEAD), falling back to "main"
func GetDefaultBranch(dir string) (string, error) {
	// Try symbolic-ref first
	cmd := exec.Command("git", "-C", dir, "symbolic-ref", "refs/remotes/origin/HEAD", "--short")
	out, err := cmd.Output()
	if err == nil {
		return strings.TrimSpace(string(out)), nil
	}

	// Try rev-parse
	cmd = exec.Command("git", "-C", dir, "rev-parse", "--abbrev-ref", "origin/HEAD")
	out, err = cmd.Output()
	if err == nil {
		return strings.TrimSpace(string(out)), nil
	}

	// Default to main
	return "main", nil
}

// GetCurrentCommitHash returns the full commit hash of HEAD
func GetCurrentCommitHash(dir string) (string, error) {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get current commit hash: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// IsDirty checks if the repository has any changes (unstaged, staged, or untracked files)
func IsDirty(dir string) bool {
	// Check unstaged changes
	cmd := exec.Command("git", "-C", dir, "diff", "--quiet")
	if err := cmd.Run(); err != nil {
		return true
	}

	// Check staged changes
	cmd = exec.Command("git", "-C", dir, "diff", "--cached", "--quiet")
	if err := cmd.Run(); err != nil {
		return true
	}

	// Check for untracked files
	cmd = exec.Command("git", "-C", dir, "ls-files", "--others", "--exclude-standard")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) != ""
}

// ---------------------------------------------------------------------------
// Git info operation — returns comprehensive repository state
// ---------------------------------------------------------------------------

// executeGitInfo returns a formatted summary of git repository state using
// the utility functions. Works without a directory (uses current working dir).
func executeGitInfo(dir string) ToolResult {
	root, err := FindGitRoot(dir)
	if err != nil || root == "" {
		return ToolResult{Output: "Not a git repository", IsError: false}
	}

	var sb strings.Builder
	sb.WriteString("Git Repository Information:\n")
	sb.WriteString(fmt.Sprintf("  Root: %s\n", root))

	branch, err := GetBranch(root)
	if err != nil {
		sb.WriteString(fmt.Sprintf("  Branch: (error: %s)\n", err))
	} else if branch == "" {
		sb.WriteString("  Branch: (detached HEAD)\n")
	} else {
		sb.WriteString(fmt.Sprintf("  Branch: %s\n", branch))
	}

	defaultBranch, err := GetDefaultBranch(root)
	if err != nil {
		sb.WriteString(fmt.Sprintf("  Default Branch: (error: %s)\n", err))
	} else {
		sb.WriteString(fmt.Sprintf("  Default Branch: %s\n", defaultBranch))
	}

	commit, err := GetCurrentCommitHash(root)
	if err != nil {
		sb.WriteString(fmt.Sprintf("  Commit: (error: %s)\n", err))
	} else {
		if len(commit) > 12 {
			sb.WriteString(fmt.Sprintf("  Commit: %s (%s...)\n", commit, commit[:7]))
		} else {
			sb.WriteString(fmt.Sprintf("  Commit: %s\n", commit))
		}
	}

	dirty := IsDirty(root)
	sb.WriteString(fmt.Sprintf("  Dirty: %t\n", dirty))

	bare := IsBareRepo(root)
	if bare {
		sb.WriteString("  Bare Repo: true\n")
	}

	// If dirty, show a summary of changes
	if dirty {
		status, err := GetGitStatus(root)
		if err == nil && len(status) > 0 {
			sb.WriteString(fmt.Sprintf("  Changes: %d file(s) modified\n", len(status)))
		} else if err == nil {
			// HasUncommittedChanges or IsDirty said true but porcelain is empty
			// — may have untracked files
			sb.WriteString("  Changes: untracked files\n")
		}
	}

	return ToolResult{Output: strings.TrimRight(sb.String(), "\n"), IsError: false}
}

// GetGitContext returns a short git context string for system prompt injection.
// Returns empty string if not in a git repo. Designed to be called at startup
// and injected into the "Environment" section of the system prompt.
func GetGitContext() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	root, err := FindGitRoot(wd)
	if err != nil || root == "" {
		return ""
	}

	var sb strings.Builder
	branch, _ := GetBranch(root)
	if branch != "" {
		sb.WriteString(fmt.Sprintf("- Git Branch: %s", branch))
	}
	commit, err := GetCurrentCommitHash(root)
	if err == nil {
		sha := commit
		if len(commit) > 12 {
			sha = commit[:12]
		}
		if sb.Len() > 0 {
			sb.WriteString(fmt.Sprintf("\n- Git Commit: %s", sha))
		}
	}
	dirty := IsDirty(root)
	sb.WriteString(fmt.Sprintf("\n- Git Dirty: %t", dirty))

	return sb.String()
}

