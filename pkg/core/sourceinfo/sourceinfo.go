package sourceinfo

import (
	"path/filepath"
	"strings"
)

// SourceInfo holds source file location information for edit/write operations.
// Mirrors pi's source tracking for knowing where code came from.
type SourceInfo struct {
	// File path relative to project root
	RelativePath string

	// Optional: source tool call ID
	ToolCallId string

	// Whether this edit came from an MCP server
	FromMcp bool

	// Original text before edit (for tracking changes)
	OriginalText string

	// Line number where content starts
	LineOffset int

	// Tags for categorizing the source
	Tags []string
}

// New creates a new SourceInfo
func New(relPath string) *SourceInfo {
	return &SourceInfo{
		RelativePath: relPath,
		Tags:         []string{},
	}
}

// WithToolCallId sets the tool call ID and returns the receiver for chaining.
func (si *SourceInfo) WithToolCallId(id string) *SourceInfo {
	si.ToolCallId = id
	return si
}

// WithFromMcp marks this as from an MCP server and returns the receiver for chaining.
func (si *SourceInfo) WithFromMcp(v bool) *SourceInfo {
	si.FromMcp = v
	return si
}

// WithOriginalText sets the original text and returns the receiver for chaining.
func (si *SourceInfo) WithOriginalText(text string) *SourceInfo {
	si.OriginalText = text
	return si
}

// WithLineOffset sets the line offset and returns the receiver for chaining.
func (si *SourceInfo) WithLineOffset(offset int) *SourceInfo {
	si.LineOffset = offset
	return si
}

// AddTag adds a tag.
func (si *SourceInfo) AddTag(tag string) {
	si.Tags = append(si.Tags, tag)
}

// GetFileName returns just the filename from the path.
func (si *SourceInfo) GetFileName() string {
	return filepath.Base(si.RelativePath)
}

// GetExt returns the file extension without the dot.
func (si *SourceInfo) GetExt() string {
	ext := filepath.Ext(si.RelativePath)
	return strings.TrimPrefix(ext, ".")
}

// Clone creates a copy of SourceInfo.
func (si *SourceInfo) Clone() *SourceInfo {
	tags := make([]string, len(si.Tags))
	copy(tags, si.Tags)
	return &SourceInfo{
		RelativePath: si.RelativePath,
		ToolCallId:   si.ToolCallId,
		FromMcp:      si.FromMcp,
		OriginalText: si.OriginalText,
		LineOffset:   si.LineOffset,
		Tags:         tags,
	}
}
