// Package footerdetect provides git branch and extension status data
// for UI footer display.
// Aligned to pi's footer-data-provider.ts.
package footerdetect

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// GitPaths contains paths related to the git repository.
type GitPaths struct {
	RepoDir    string // Path to the repo root
	CommonGitDir string // Path to the common git dir (for worktrees)
	HeadPath   string // Path to the HEAD file
}

// FindGitPaths walks up the directory tree from cwd to find the nearest .git directory.
// Returns empty GitPaths if no git repo is found.
func FindGitPaths(cwd string) GitPaths {
	dir := cwd
	for {
		gitDir := filepath.Join(dir, ".git")
		if _, err := os.Stat(gitDir); err == nil {
			headPath := filepath.Join(gitDir, "HEAD")
			return GitPaths{
				RepoDir:    dir,
				CommonGitDir: gitDir,
				HeadPath:   headPath,
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root
			return GitPaths{}
		}
		dir = parent
	}
}

// ResolveBranchWithGitSync resolves the current git branch name synchronously.
// Returns empty string if git is not available or the directory is not a git repo.
func ResolveBranchWithGitSync(repoDir string) string {
	if repoDir == "" {
		return ""
	}

	cmd := exec.Command("git", "-C", repoDir, "branch", "--show-current")
	output, err := cmd.Output()
	if err != nil {
		// Fallback: try symbolic-ref
		cmd = exec.Command("git", "-C", repoDir, "symbolic-ref", "--short", "HEAD")
		output, err = cmd.Output()
		if err != nil {
			return ""
		}
	}

	branch := strings.TrimSpace(string(output))
	if branch == "" {
		// Check if we're in detached HEAD
		cmd = exec.Command("git", "-C", repoDir, "rev-parse", "--short", "HEAD")
		output, err = cmd.Output()
		if err == nil {
			return "detached @" + strings.TrimSpace(string(output))
		}
	}

	return branch
}

// ResolveBranchAsync resolves the current git branch name asynchronously.
// Returns a channel that will receive the branch name.
func ResolveBranchAsync(repoDir string) <-chan string {
	ch := make(chan string, 1)
	go func() {
		ch <- ResolveBranchWithGitSync(repoDir)
	}()
	return ch
}

// GetGitBranch walks up from cwd to find git repo and returns the branch name.
// This is the main convenience function.
func GetGitBranch(cwd string) string {
	paths := FindGitPaths(cwd)
	if paths.RepoDir == "" {
		return ""
	}
	return ResolveBranchWithGitSync(paths.RepoDir)
}
