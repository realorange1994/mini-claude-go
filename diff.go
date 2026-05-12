package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// StructuredDiff generates a unified diff between two strings.
// Falls back to a simple line-by-line comparison if diff tools are unavailable.
func StructuredDiff(oldContent, newContent, filePath string) string {
	// Try using git diff if available
	if _, err := exec.LookPath("git"); err == nil {
		return gitDiff(oldContent, newContent, filePath)
	}
	// Fallback: simple inline diff
	return simpleDiff(oldContent, newContent, filePath)
}

// gitDiff uses git's diff engine for proper unified diff output.
func gitDiff(oldContent, newContent, filePath string) string {
	// Write old content to a temp file, new content to another, then diff
	tmpDir, err := os.MkdirTemp("", "diff-*")
	if err != nil {
		return simpleDiff(oldContent, newContent, filePath)
	}
	defer os.RemoveAll(tmpDir)

	oldFile := tmpDir + "/old"
	newFile := tmpDir + "/new"

	if err := os.WriteFile(oldFile, []byte(oldContent), 0644); err != nil {
		return simpleDiff(oldContent, newContent, filePath)
	}
	if err := os.WriteFile(newFile, []byte(newContent), 0644); err != nil {
		return simpleDiff(oldContent, newContent, filePath)
	}

	cmd := exec.Command("git", "diff", "--no-index", "--unified=3", oldFile, newFile)
	out, _ := cmd.CombinedOutput()

	// Replace temp file paths with the actual file path
	result := string(out)
	result = strings.Replace(result, oldFile, "a/"+filePath, -1)
	result = strings.Replace(result, newFile, "b/"+filePath, -1)

	return result
}

// simpleDiff produces a basic line-by-line diff when git is unavailable.
func simpleDiff(oldContent, newContent, filePath string) string {
	oldLines := strings.Split(oldContent, "\n")
	newLines := strings.Split(newContent, "\n")

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("--- a/%s\n", filePath))
	sb.WriteString(fmt.Sprintf("+++ b/%s\n", filePath))

	maxLines := len(oldLines)
	if len(newLines) > maxLines {
		maxLines = len(newLines)
	}

	for i := 0; i < maxLines; i++ {
		oldLine := ""
		newLine := ""
		if i < len(oldLines) {
			oldLine = oldLines[i]
		}
		if i < len(newLines) {
			newLine = newLines[i]
		}

		if oldLine != newLine {
			if i < len(oldLines) {
				sb.WriteString(fmt.Sprintf("-%s\n", oldLine))
			}
			if i < len(newLines) {
				sb.WriteString(fmt.Sprintf("+%s\n", newLine))
			}
		}
	}

	return sb.String()
}