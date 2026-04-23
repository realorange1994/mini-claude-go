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
	return "Execute Git version control operations. Supports clone, init, add, commit, push, pull, fetch, branch, checkout, merge, rebase, stash, reset, tag, status, diff, log, remote, show, describe, ls-files, ls-tree, rev-parse, rev-list, and worktree operations."
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
				},
			},
			"repo": map[string]interface{}{
				"type":        "string",
				"description": "Repository URL (for clone)",
			},
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Local path (clone destination, or target for init/worktree)",
			},
			"branch": map[string]interface{}{
				"type":        "string",
				"description": "Branch name (for checkout, branch, push, pull, worktree)",
			},
			"message": map[string]interface{}{
				"type":        "string",
				"description": "Commit message (for commit)",
			},
			"files": map[string]interface{}{
				"type":        "array",
				"description": "Files to stage (for add), show diff (for diff), or list (for ls-files)",
				"items":       map[string]interface{}{"type": "string"},
			},
			"remote": map[string]interface{}{
				"type":        "string",
				"description": "Remote name (default: origin)",
			},
			"target": map[string]interface{}{
				"type":        "string",
				"description": "Target branch or commit (for merge, rebase, describe)",
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

	cmd, err := buildGitCommand(params)
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("Error: %v", err), IsError: true}
	}

	out, err := runGitCommand(ctx, cmd)
	if err != nil {
		return ToolResult{Output: out, IsError: true}
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
		if path, _ := params["path"].(string); path != "" {
			args = append(args, path)
		}
		args = append(args, repo)

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
		args = []string{"commit", "-m", message}

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
		if branch, _ := params["branch"].(string); branch != "" {
			args = append(args, branch)
		}

	case "merge":
		args = []string{"merge"}
		if target, _ := params["target"].(string); target != "" {
			args = append(args, target)
		}

	case "rebase":
		args = []string{"rebase"}
		if target, _ := params["target"].(string); target != "" {
			args = append(args, target)
		}

	case "stash":
		args = []string{"stash"}

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

	default:
		return nil, fmt.Errorf("unknown operation: %s", operation)
	}

	if flags := getStringArray(params, "flags"); len(flags) > 0 {
		args = append(args, flags...)
	}

	return args, nil
}

func runGitCommand(ctx context.Context, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
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
