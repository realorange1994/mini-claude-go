package tools

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// GitTool provides Git version control operations.
type GitTool struct{}

func (*GitTool) Name() string    { return "git" }
func (*GitTool) Description() string {
	return "Execute Git version control operations. Supports clone, init, add, commit, push, pull, fetch, branch, checkout, merge, rebase, stash, reset, tag, status, diff, log, remote, show, describe, ls-files, ls-tree, rev-parse, rev-list, worktree, rm, mv, restore, switch, cherry-pick, revert, clean, blame, reflog, and shortlog operations."
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
					"clean", "blame", "reflog", "shortlog",
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
		},
		"required": []string{"operation"},
	}
}

func (*GitTool) CheckPermissions(params map[string]interface{}) string {
	return ""
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
	if (operation == "push" || operation == "pull" || operation == "fetch") {
		remote, _ := params["remote"].(string)
		if remote == "" {
			// Check if there's an origin remote configured
			checkOut, _, _ := runGitCommandWithExitCode(ctx, []string{"remote"}, workDir, "")
			if checkOut == "" {
				return ToolResult{
					Output:   "Error: cannot determine git remotes",
					IsError: true,
				}
			}
			if !strings.Contains(checkOut, "origin") {
				return ToolResult{
					Output:   fmt.Sprintf("Error: no remote specified and no 'origin' remote found. Available remotes:\n%s", checkOut),
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

	out, exitCode, _ := runGitCommandWithExitCode(ctx, cmd, workDir, proxy)

	if exitCode != 0 {
		return ToolResult{
			Output:  fmt.Sprintf("Error executing 'git %s' (exit code: %d)\n\nOutput:\n%s", strings.Join(cmd, " "), exitCode, out),
			IsError: true,
		}
	}

	return ToolResult{Output: out, IsError: false}
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

	default:
		return nil, fmt.Errorf("unknown operation: %s", operation)
	}

	if flags := getStringArray(params, "flags"); len(flags) > 0 {
		args = append(args, flags...)
	}

	return args, nil
}

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
	// Don't return err — let caller decide how to handle non-zero exit codes
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