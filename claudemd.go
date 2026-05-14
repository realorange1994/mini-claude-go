package main

import (
	"regexp"
	"strings"
)

// claudemd utilities ported from upstream: src/utils/claudemd.ts

// Recommended max character count for a memory file.
const MaxMemoryCharacterCount = 40000

// MemoryFileInfo holds information about a memory file.
type MemoryFileInfo struct {
	Path    string
	Type    string // "Project", "User", "Local", "Managed", "AutoMem", "TeamMem"
	Content string
}

// StripHtmlComments strips block-level HTML comments from markdown content.
// Inline comments within paragraphs are preserved (CommonMark paragraph semantics).
// Unclosed comments are left in place.
// Content inside fenced code blocks (``` ... ```) is fully preserved.
func StripHtmlComments(content string) (result string, stripped bool) {
	if !strings.Contains(content, "<!--") {
		return content, false
	}

	lines := strings.Split(content, "\n")
	var resultLines []string
	stripped = false
	inCodeBlock := false

	for _, line := range lines {
		trimmed := strings.TrimLeft(line, " \t")
		// Toggle fenced code block tracking
		if strings.HasPrefix(trimmed, "```") {
			inCodeBlock = !inCodeBlock
			resultLines = append(resultLines, line)
			continue
		}

		if inCodeBlock {
			// Inside a code block, preserve the line as-is
			resultLines = append(resultLines, line)
		} else if isHtmlBlockLine(line) {
			residue := stripCommentSpans(line)
			if residue != "" {
				resultLines = append(resultLines, residue)
			}
			stripped = true
		} else {
			resultLines = append(resultLines, line)
		}
	}

	return strings.Join(resultLines, "\n"), stripped
}

// isHtmlBlockLine returns true if the line starts with <!-- and contains -->
// (CommonMark type-2 HTML block: comment at beginning of line).
func isHtmlBlockLine(line string) bool {
	trimmed := strings.TrimLeft(line, " \t")
	return strings.HasPrefix(trimmed, "<!--") && strings.Contains(trimmed, "-->")
}

var commentSpanRegex = regexp.MustCompile(`<!--[\s\S]*?-->`)

// stripCommentSpans removes well-formed HTML comment spans from a line.
func stripCommentSpans(line string) string {
	return commentSpanRegex.ReplaceAllString(line, "")
}

// IsMemoryFilePath checks if a file path is a memory file (CLAUDE.md, CLAUDE.local.md,
// or .md files in .claude/rules/ directories).
func IsMemoryFilePath(filePath string) bool {
	// Extract basename
	name := filePath
	if idx := strings.LastIndex(filePath, "/"); idx >= 0 {
		name = filePath[idx+1:]
	} else if idx = strings.LastIndex(filePath, `\`); idx >= 0 {
		name = filePath[idx+1:]
	}

	// CLAUDE.md or CLAUDE.local.md anywhere (case-sensitive)
	if name == "CLAUDE.md" || name == "CLAUDE.local.md" {
		return true
	}

	// .md files in .claude/rules/ directories
	if strings.HasSuffix(name, ".md") && strings.Contains(filePath, "/.claude/rules/") {
		return true
	}

	return false
}

// GetLargeMemoryFiles returns files whose content exceeds MAX_MEMORY_CHARACTER_COUNT.
func GetLargeMemoryFiles(files []MemoryFileInfo) []MemoryFileInfo {
	var result []MemoryFileInfo
	for _, f := range files {
		if len(f.Content) > MaxMemoryCharacterCount {
			result = append(result, f)
		}
	}
	return result
}
