// Package readtool provides the Read tool implementation.
// Aligned to pi's tools/read.ts.
package readtool

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"miniclaudecode-go/pkg/core/tools/truncate"
)

const (
	// DefaultMaxLines is the default maximum lines for Read output.
	DefaultMaxLines = 2000

	// DefaultMaxBytes is the default maximum bytes (50KB).
	DefaultMaxBytes = 50 * 1024

	// ImageMaxBase64Bytes is the maximum base64 size for inline images (4.5MB).
	ImageMaxBase64Bytes = 4.5 * 1024 * 1024
)

// ReadInput is the input for the Read tool.
// Aligned to pi's ReadToolInput.
type ReadInput struct {
	Path   string `json:"path"`
	Offset int    `json:"offset,omitempty"` // 1-indexed start line
	Limit  int    `json:"limit,omitempty"`  // Max lines to read from offset
}

// ReadDetails holds metadata about a read operation.
type ReadDetails struct {
	Truncation *truncate.TruncationResult `json:"truncation,omitempty"`
}

// ImageBlock represents an inline image in tool output.
type ImageBlock struct {
	Type     string `json:"type"`
	Data     string `json:"data"`               // base64-encoded
	MimeType string `json:"mime_type"`
}

// ContentBlock is a union of text and image content.
type ContentBlock struct {
	Type     string `json:"type"` // "text" or "image"
	Text     string `json:"text,omitempty"`
	Image    *ImageBlock `json:"image,omitempty"`
}

// ReadResult holds the result of a Read operation.
type ReadResult struct {
	Content  []ContentBlock `json:"content"`
	Details  ReadDetails    `json:"details,omitempty"`
	Error    string         `json:"error,omitempty"`
}

// ReadOperations allows pluggable I/O for remote execution.
type ReadOperations interface {
	ReadFile(absolutePath string) ([]byte, error)
	Access(absolutePath string) error
	DetectImageMimeType(absolutePath string) (string, error)
}

// LocalReadOperations implements ReadOperations for the local filesystem.
type LocalReadOperations struct{}

func (LocalReadOperations) ReadFile(absolutePath string) ([]byte, error) {
	return os.ReadFile(absolutePath)
}

func (LocalReadOperations) Access(absolutePath string) error {
	_, err := os.Stat(absolutePath)
	return err
}

func (LocalReadOperations) DetectImageMimeType(absolutePath string) (string, error) {
	return DetectImageMimeType(absolutePath), nil
}

// DetectImageMimeType returns the MIME type for supported image formats.
// Returns empty string for unsupported formats.
func DetectImageMimeType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	default:
		return ""
	}
}

// IsImageFile checks if a file is a supported image type.
func IsImageFile(path string) bool {
	return DetectImageMimeType(path) != ""
}

// Execute performs the Read operation.
// Aligned to pi's Read tool execute() function.
func Execute(input ReadInput, cwd string, ops ReadOperations, autoResizeImages bool, modelSupportsImages bool) (*ReadResult, error) {
	if input.Path == "" {
		return nil, fmt.Errorf("missing required parameter: path")
	}

	// Resolve path
	absolutePath := resolveReadPath(input.Path, cwd)

	// Check accessibility
	if err := ops.Access(absolutePath); err != nil {
		return nil, fmt.Errorf("file not found or not accessible: %s", input.Path)
	}

	// Check if image
	mimeType, _ := ops.DetectImageMimeType(absolutePath)
	if mimeType != "" {
		return readImage(absolutePath, mimeType, ops, modelSupportsImages)
	}

	// Read as text
	return readText(absolutePath, input, ops)
}

// readImage reads a file as an image and returns a base64-encoded content block.
func readImage(absolutePath, mimeType string, ops ReadOperations, modelSupportsImages bool) (*ReadResult, error) {
	data, err := ops.ReadFile(absolutePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read image: %w", err)
	}

	b64 := base64.StdEncoding.EncodeToString(data)
	label := fmt.Sprintf("Read image file %s", mimeType)

	var content []ContentBlock
	content = append(content, ContentBlock{
		Type: "text",
		Text: label,
	})

	if !modelSupportsImages {
		content = append(content, ContentBlock{
			Type: "text",
			Text: "[Current model does not support images. The image will be omitted from this request.]",
		})
	} else if len(b64) > ImageMaxBase64Bytes {
		content = append(content, ContentBlock{
			Type: "text",
			Text: "[Image omitted: could not be resized below the inline image size limit.]",
		})
	} else {
		content = append(content, ContentBlock{
			Type: "image",
			Image: &ImageBlock{
				Type:     "base64",
				Data:     b64,
				MimeType: mimeType,
			},
		})
	}

	return &ReadResult{Content: content}, nil
}

// readText reads a file as text with offset/limit support.
func readText(absolutePath string, input ReadInput, ops ReadOperations) (*ReadResult, error) {
	data, err := ops.ReadFile(absolutePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	content := string(data)

	// Normalize line endings to LF for processing
	content = strings.ReplaceAll(content, "\r\n", "\n")

	// Split into lines
	allLines := strings.Split(content, "\n")

	// Remove trailing empty element from split if content ended with newline
	if len(content) > 0 && content[len(content)-1] == '\n' {
		allLines = allLines[:len(allLines)-1]
	}

	// Apply offset (1-indexed → 0-indexed)
	startLine := 0
	if input.Offset > 0 {
		startLine = input.Offset - 1
		if startLine >= len(allLines) {
			return nil, fmt.Errorf("offset %d is beyond end of file (%d lines total)", input.Offset, len(allLines))
		}
	}

	// Apply limit
	endLine := len(allLines)
	if input.Limit > 0 {
		endLine = startLine + input.Limit
		if endLine > len(allLines) {
			endLine = len(allLines)
		}
	}

	selectedLines := allLines[startLine:endLine]
	userLimitedLines := input.Limit > 0 && endLine < len(allLines)

	// Join selected lines
	selectedContent := strings.Join(selectedLines, "\n")
	if len(selectedLines) > 0 {
		selectedContent += "\n"
	}

	// Apply truncation (head-only, to enforce max lines/bytes)
	truncResult := truncate.TruncateHead(selectedContent, truncate.TruncationOptions{
		MaxLines: DefaultMaxLines,
		MaxBytes: DefaultMaxBytes,
	})

	// Build output
	var outputText string

	if truncResult.FirstLineExceeds {
		outputText = fmt.Sprintf("First line of the file is too long to display (%s). Use `sed` as a fallback, e.g. `sed -n '1p' %s`.", truncate.FormatSize(truncResult.TotalBytes), input.Path)
	} else if truncResult.Truncated {
		// Truncated output
		outputText = truncResult.Content
		outputText += fmt.Sprintf("\n[Showing lines %d-%d of %d. Use offset=%d to continue.]",
			startLine+1, startLine+truncResult.OutputLines, len(allLines), startLine+truncResult.OutputLines+1)
	} else if userLimitedLines {
		// User limited lines but file has more
		outputText = truncResult.Content
		remaining := len(allLines) - endLine
		outputText += fmt.Sprintf("\n[%d more lines in file. Use offset=%d to continue.]", remaining, endLine+1)
	} else {
		outputText = truncResult.Content
	}

	result := &ReadResult{
		Content: []ContentBlock{
			{Type: "text", Text: outputText},
		},
		Details: ReadDetails{
			Truncation: &truncResult,
		},
	}

	return result, nil
}

// resolveReadPath resolves a path relative to cwd.
// Handles ~ expansion and relative paths.
func resolveReadPath(path, cwd string) string {
	// Expand ~
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			path = filepath.Join(home, path[2:])
		}
	}

	// If already absolute, return as-is
	if filepath.IsAbs(path) {
		return path
	}

	// Resolve relative to cwd
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	return filepath.Join(cwd, path)
}

// FormatReadOutput extracts plain text from a ReadResult for simple consumers.
func FormatReadOutput(result *ReadResult) string {
	var parts []string
	for _, block := range result.Content {
		if block.Type == "text" {
			parts = append(parts, block.Text)
		}
	}
	return strings.Join(parts, "\n")
}
