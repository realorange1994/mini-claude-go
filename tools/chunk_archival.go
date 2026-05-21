package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ChunkInfo holds metadata about an archived conversation chunk.
type ChunkInfo struct {
	Index  int    // chunk sequence number (1-based)
	Path   string // absolute path to the .md file
	Topics string // topics extracted from frontmatter
}

// ArchiveChunk writes discarded conversation messages to a chunk .md file
// with YAML frontmatter. Returns the absolute path of the written file.
//
// The file format matches openclacky's chunk archival:
//
//	---
//	session_id: abc123
//	chunk: 1
//	compression_level: 1
//	archived_at: 2026-05-22T10:30:00+08:00
//	message_count: 42
//	topics: Git analysis, refactor, tests
//	---
//
//	# Session Chunk 1
//
//	## User
//	...
//
// Messages are rendered as Markdown sections. Tool results are truncated
// to 500 chars to keep chunk files manageable.
func ArchiveChunk(dir, sessionID string, chunkIndex, compressionLevel int, topics string, messages []ChunkMessage) (string, error) {
	if dir == "" {
		return "", fmt.Errorf("chunk archival: dir is empty")
	}
	chunkDir := filepath.Join(dir, "chunks")
	if err := os.MkdirAll(chunkDir, 0o700); err != nil {
		return "", fmt.Errorf("chunk archival: mkdir: %w", err)
	}

	filename := fmt.Sprintf("chunk-%d.md", chunkIndex)
	chunkPath := filepath.Join(chunkDir, filename)

	var b strings.Builder

	// YAML frontmatter
	b.WriteString("---\n")
	b.WriteString(fmt.Sprintf("session_id: %s\n", sessionID))
	b.WriteString(fmt.Sprintf("chunk: %d\n", chunkIndex))
	b.WriteString(fmt.Sprintf("compression_level: %d\n", compressionLevel))
	b.WriteString(fmt.Sprintf("archived_at: %s\n", time.Now().Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("message_count: %d\n", len(messages)))
	if topics != "" {
		b.WriteString(fmt.Sprintf("topics: %s\n", topics))
	}
	b.WriteString("---\n\n")

	// Markdown body
	b.WriteString(fmt.Sprintf("# Session Chunk %d\n\n", chunkIndex))
	b.WriteString("> This file contains the original conversation archived during compression.\n")
	b.WriteString("> Use `file_reader` to recall specific details from this conversation.\n\n")

	for _, msg := range messages {
		role := strings.ToUpper(msg.Role[:1]) + msg.Role[1:]
		b.WriteString(fmt.Sprintf("## %s\n\n", role))
		content := msg.Content
		if len(content) > 500 {
			content = content[:500] + "\n[... truncated ...]"
		}
		b.WriteString(content)
		b.WriteString("\n\n")
	}

	if err := os.WriteFile(chunkPath, []byte(b.String()), 0o600); err != nil {
		return "", fmt.Errorf("chunk archival: write: %w", err)
	}

	return chunkPath, nil
}

// ChunkMessage is a simplified message representation for archival.
type ChunkMessage struct {
	Role    string // "user", "assistant", "tool"
	Content string
}

// ListChunks discovers all chunk .md files in the session's chunks directory.
// Returns chunks sorted by index (ascending).
func ListChunks(dir, sessionID string) []ChunkInfo {
	chunkDir := filepath.Join(dir, "chunks")
	entries, err := os.ReadDir(chunkDir)
	if err != nil {
		return nil
	}

	chunkFileRegex := regexp.MustCompile(`^chunk-(\d+)\.md$`)

	var chunks []ChunkInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		m := chunkFileRegex.FindStringSubmatch(entry.Name())
		if m == nil {
			continue
		}
		var idx int
		fmt.Sscanf(m[1], "%d", &idx)

		path := filepath.Join(chunkDir, entry.Name())
		topics := ReadChunkTopics(path)

		chunks = append(chunks, ChunkInfo{
			Index:  idx,
			Path:   path,
			Topics: topics,
		})
	}

	sort.Slice(chunks, func(i, j int) bool {
		return chunks[i].Index < chunks[j].Index
	})

	return chunks
}

// ReadChunkTopics reads the topics line from a chunk file's YAML frontmatter.
// Returns empty string if topics not found or file doesn't exist.
func ReadChunkTopics(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.Split(string(data), "\n")
	inFrontmatter := false
	for _, line := range lines {
		if strings.TrimSpace(line) == "---" {
			if inFrontmatter {
				break // end of frontmatter
			}
			inFrontmatter = true
			continue
		}
		if inFrontmatter && strings.HasPrefix(line, "topics:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "topics:"))
		}
	}
	return ""
}

// BuildPreviousChunksIndex formats a list of chunk metadata into a
// human-readable index string for inclusion in the compressed summary.
// The model can then use file_reader to recall specific chunks on demand.
//
// Format (matching openclacky):
//
//	---
//	Previous Session Archives (2 chunks available)
//	[CHUNK-1] /path/to/chunk-1.md
//	  Topics: Git analysis, refactor
//	[CHUNK-2] /path/to/chunk-2.md
//	  Topics: tests, deployment
//
//	Use file_reader to load a chunk file when you need original conversation details.
func BuildPreviousChunksIndex(chunks []ChunkInfo) string {
	if len(chunks) == 0 {
		return ""
	}

	var b strings.Builder
	plural := ""
	if len(chunks) > 1 {
		plural = "s"
	}
	b.WriteString(fmt.Sprintf("\n\n---\nPrevious Session Archives (%d chunk%s available)\n", len(chunks), plural))

	for i, chunk := range chunks {
		b.WriteString(fmt.Sprintf("[CHUNK-%d] %s\n", i+1, chunk.Path))
		if chunk.Topics != "" {
			b.WriteString(fmt.Sprintf("  Topics: %s\n", chunk.Topics))
		}
	}

	b.WriteString("Use file_reader to load a chunk file when you need original conversation details.")

	return b.String()
}

// NextChunkIndex returns the next available chunk index (1-based)
// by examining existing chunk files on disk.
func NextChunkIndex(dir, sessionID string) int {
	chunks := ListChunks(dir, sessionID)
	if len(chunks) == 0 {
		return 1
	}
	return chunks[len(chunks)-1].Index + 1
}