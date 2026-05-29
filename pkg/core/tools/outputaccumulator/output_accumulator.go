// Package outputaccumulator provides bounded streaming output accumulation.
// Aligned to pi's tools/output-accumulator.ts.
package outputaccumulator

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"miniclaudecode-go/pkg/core/tools/truncate"
)

// OutputSnapshot holds a snapshot of accumulated output.
type OutputSnapshot struct {
	Content         string
	Truncation      truncate.TruncationResult
	FullOutputPath  string // Path to temp file with full output, if created
}

// OutputAccumulatorOptions configures the accumulator.
type OutputAccumulatorOptions struct {
	MaxLines        int
	MaxBytes        int
	TempFilePrefix  string
}

// OutputAccumulator accumulates streaming output with bounded memory.
// It keeps a decoded tail for display snapshots and optionally writes
// full output to a temp file when thresholds are exceeded.
type OutputAccumulator struct {
	mu sync.Mutex

	maxLines       int
	maxBytes       int
	maxRollingBytes int
	tempFilePrefix string

	// Raw chunks stored before temp file creation
	rawChunks [][]byte

	// Decoded tail buffer (rolling, bounded)
	tailText                 string
	tailBytes                int
	tailStartsAtLineBoundary bool

	// Counters
	totalRawBytes     int
	totalDecodedBytes int
	completedLines    int
	totalLines        int
	currentLineBytes  int
	hasOpenLine       bool

	// State
	finished      bool
	tempFilePath  string
	tempFile      *os.File
}

// NewOutputAccumulator creates a new output accumulator.
func NewOutputAccumulator(opts OutputAccumulatorOptions) *OutputAccumulator {
	maxLines := opts.MaxLines
	if maxLines <= 0 {
		maxLines = truncate.DefaultMaxLines
	}
	maxBytes := opts.MaxBytes
	if maxBytes <= 0 {
		maxBytes = truncate.DefaultMaxBytes
	}
	prefix := opts.TempFilePrefix
	if prefix == "" {
		prefix = "pi-output"
	}

	return &OutputAccumulator{
		maxLines:        maxLines,
		maxBytes:        maxBytes,
		maxRollingBytes: max(maxBytes*2, 1),
		tempFilePrefix:  prefix,
	}
}

// Append adds a data chunk to the accumulator.
func (oa *OutputAccumulator) Append(data []byte) {
	oa.mu.Lock()
	defer oa.mu.Unlock()

	if oa.finished {
		return
	}

	oa.totalRawBytes += len(data)

	// Decode chunk (Go strings are UTF-8, data is already bytes)
	decoded := string(data)
	oa.appendDecodedText(decoded)

	// If temp file exists or should use it, write raw data
	if oa.tempFile != nil || oa.shouldUseTempFile() {
		oa.ensureTempFile()
		if oa.tempFile != nil {
			oa.tempFile.Write(data)
		}
	} else {
		oa.rawChunks = append(oa.rawChunks, data)
	}
}

// Finish signals that no more data will be appended.
func (oa *OutputAccumulator) Finish() {
	oa.mu.Lock()
	defer oa.mu.Unlock()

	if oa.finished {
		return
	}
	oa.finished = true

	// Ensure temp file if threshold exceeded
	if oa.shouldUseTempFile() {
		oa.ensureTempFile()
	}
}

// Snapshot returns a truncated view of the accumulated output.
func (oa *OutputAccumulator) Snapshot(persistIfTruncated bool) OutputSnapshot {
	oa.mu.Lock()
	defer oa.mu.Unlock()

	text := oa.getSnapshotText()
	truncResult := truncate.TruncateTail(text, truncate.TruncationOptions{
		MaxLines: oa.maxLines,
		MaxBytes: oa.maxBytes,
	})

	// Determine if we need the full output path
	isTruncated := oa.totalLines > oa.maxLines || oa.totalDecodedBytes > oa.maxBytes

	if persistIfTruncated && isTruncated {
		oa.ensureTempFile()
	}

	fullPath := ""
	if oa.tempFilePath != "" {
		fullPath = oa.tempFilePath
	}

	return OutputSnapshot{
		Content:        truncResult.Content,
		Truncation:     truncResult,
		FullOutputPath: fullPath,
	}
}

// CloseTempFile closes the temp file if one was created.
func (oa *OutputAccumulator) CloseTempFile() error {
	oa.mu.Lock()
	defer oa.mu.Unlock()

	if oa.tempFile == nil {
		return nil
	}

	err := oa.tempFile.Close()
	oa.tempFile = nil
	return err
}

// GetLastLineBytes returns the byte count of the current incomplete line.
func (oa *OutputAccumulator) GetLastLineBytes() int {
	oa.mu.Lock()
	defer oa.mu.Unlock()
	return oa.currentLineBytes
}

// GetTempFilePath returns the path to the temp file, if created.
func (oa *OutputAccumulator) GetTempFilePath() string {
	oa.mu.Lock()
	defer oa.mu.Unlock()
	return oa.tempFilePath
}

// GetTotalLines returns the total number of lines accumulated.
func (oa *OutputAccumulator) GetTotalLines() int {
	oa.mu.Lock()
	defer oa.mu.Unlock()
	return oa.totalLines
}

// GetTotalBytes returns the total number of bytes accumulated.
func (oa *OutputAccumulator) GetTotalBytes() int {
	oa.mu.Lock()
	defer oa.mu.Unlock()
	return oa.totalDecodedBytes
}

// --- Internal methods (must be called with lock held) ---

func (oa *OutputAccumulator) appendDecodedText(text string) {
	if text == "" {
		return
	}

	byteLen := len(text)
	oa.totalDecodedBytes += byteLen
	oa.tailText += text
	oa.tailBytes += byteLen

	// Trim tail if exceeds 2x rolling limit
	if oa.tailBytes > oa.maxRollingBytes*2 {
		oa.trimTail()
	}

	// Count newlines
	newlineCount := 0
	lastNewline := -1
	for i := 0; i < len(text); i++ {
		if text[i] == '\n' {
			newlineCount++
			lastNewline = i
		}
	}

	if newlineCount == 0 {
		oa.currentLineBytes += byteLen
		oa.hasOpenLine = true
	} else {
		oa.completedLines += newlineCount
		// Bytes after last newline belong to the current open line
		if lastNewline < len(text)-1 {
			oa.currentLineBytes = len(text) - lastNewline - 1
			oa.hasOpenLine = true
		} else {
			oa.currentLineBytes = 0
			oa.hasOpenLine = false
		}
	}

	oa.totalLines = oa.completedLines + (func() int {
		if oa.hasOpenLine {
			return 1
		}
		return 0
	}())
}

func (oa *OutputAccumulator) trimTail() {
	if oa.tailBytes <= oa.maxRollingBytes {
		return
	}

	// Find start position
	start := len(oa.tailText) - oa.maxRollingBytes
	if start <= 0 {
		return
	}

	// Skip UTF-8 continuation bytes
	for start < len(oa.tailText) {
		if (oa.tailText[start] & 0xC0) != 0x80 {
			break
		}
		start++
	}

	// Check if we start at a line boundary
	oa.tailStartsAtLineBoundary = start > 0 && oa.tailText[start-1] == '\n'

	oa.tailText = oa.tailText[start:]
	oa.tailBytes = len(oa.tailText)
}

func (oa *OutputAccumulator) getSnapshotText() string {
	if oa.tailStartsAtLineBoundary {
		return oa.tailText
	}
	// Find first newline and return text after it (avoid partial first line)
	idx := strings.Index(oa.tailText, "\n")
	if idx >= 0 {
		return oa.tailText[idx+1:]
	}
	return oa.tailText
}

func (oa *OutputAccumulator) shouldUseTempFile() bool {
	return oa.totalRawBytes > oa.maxBytes ||
		oa.totalDecodedBytes > oa.maxBytes ||
		oa.totalLines > oa.maxLines
}

func (oa *OutputAccumulator) ensureTempFile() {
	if oa.tempFilePath != "" {
		return
	}

	// Generate unique temp file path
	id := generateRandomHex(8)
	path := filepath.Join(os.TempDir(), fmt.Sprintf("%s-%s.log", oa.tempFilePrefix, id))

	f, err := os.Create(path)
	if err != nil {
		return
	}

	oa.tempFilePath = path
	oa.tempFile = f

	// Flush stored raw chunks
	for _, chunk := range oa.rawChunks {
		oa.tempFile.Write(chunk)
	}
	oa.rawChunks = nil
}

func generateRandomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}