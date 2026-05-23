package tools

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	defaultMaxMemory   = 8 * 1024 * 1024 // 8MB default max memory (matching upstream)
	circularBufferSize = 1000            // recent lines buffer size (matching upstream)
	progressPollMs     = 1000            // progress polling interval
)

// CircularBuffer is a fixed-size ring buffer for strings.
// It keeps the most recent N entries and drops the oldest when full.
type CircularBuffer struct {
	mu       sync.Mutex
	buf      []string
	head     int // next write position
	size     int // number of valid entries
	capacity int
}

// NewCircularBuffer creates a circular buffer with the given capacity.
func NewCircularBuffer(capacity int) *CircularBuffer {
	if capacity <= 0 {
		capacity = circularBufferSize
	}
	return &CircularBuffer{
		buf:      make([]string, capacity),
		capacity: capacity,
	}
}

// Append adds a string to the buffer, overwriting the oldest entry if full.
func (cb *CircularBuffer) Append(s string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.buf[cb.head] = s
	cb.head = (cb.head + 1) % cb.capacity
	if cb.size < cb.capacity {
		cb.size++
	}
}

// AppendMany adds multiple strings in one call (more efficient for bulk writes).
func (cb *CircularBuffer) AppendMany(ss []string) {
	for _, s := range ss {
		cb.Append(s)
	}
}

// GetAll returns all entries in order (oldest first).
func (cb *CircularBuffer) GetAll() []string {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if cb.size == 0 {
		return nil
	}
	result := make([]string, 0, cb.size)
	start := (cb.head - cb.size + cb.capacity) % cb.capacity
	for i := 0; i < cb.size; i++ {
		result = append(result, cb.buf[(start+i)%cb.capacity])
	}
	return result
}

// Tail returns the last N entries (or fewer if not enough entries exist).
func (cb *CircularBuffer) Tail(n int) []string {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if n <= 0 || cb.size == 0 {
		return nil
	}
	if n > cb.size {
		n = cb.size
	}
	result := make([]string, 0, n)
	start := (cb.head - n + cb.capacity) % cb.capacity
	for i := 0; i < n; i++ {
		result = append(result, cb.buf[(start+i)%cb.capacity])
	}
	return result
}

// Len returns the number of entries currently in the buffer.
func (cb *CircularBuffer) Len() int {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.size
}

// BgTaskProgress is a snapshot of a background task's progress.
type BgTaskProgress struct {
	TotalLines   int64
	TotalBytes   int64
	LastActivity time.Time
	IsComplete   bool
	ExitCode     int32  // -1 means not yet exited
	Last5Lines   string // last 5 lines for quick preview
	Last100Lines string // last 100 lines for recent output
	Description  string // current activity description from agent
	TokenCount   int    // total token usage from agent
	ToolUseCount int    // number of tool calls made
}

// BgTaskOutput manages output for background tasks (exec, agent).
//
// Dual-mode architecture (matching upstream TaskOutput.ts):
//   - File mode: output goes directly to a file on disk (exec background tasks)
//   - Pipe mode: data flows through WriteStdout/WriteStderr, buffered in memory
//     with spillover to disk at maxMemory bytes
//
// Key features:
//   - CircularBuffer for recent lines (fast tail access without full history)
//   - Memory-to-disk spillover at configurable limit (default 8MB)
//   - Progress polling via file tail reading for file-mode tasks
//   - Thread-safe concurrent writes
type BgTaskOutput struct {
	mu         sync.Mutex
	taskID     string
	outputPath string   // file path for output
	stdoutFile *os.File // non-nil in file mode
	stderrFile *os.File // non-nil in file mode

	// Pipe mode buffers
	memoryBuffer *CircularBuffer // recent lines (pipe mode only)
	memoryBytes  int64           // total bytes in memory mode
	spillFile    *os.File        // file for overflow (pipe mode)
	spillPath    string

	// Progress tracking (atomic-safe fields)
	totalLines     int64  // atomic: total newline count
	totalBytes     int64  // atomic: total bytes written
	lastActivityMs int64  // atomic: unix millis of last write
	description    string // current activity description
	isComplete     int32  // atomic: 1 = task finished
	exitCode       int32  // atomic: process exit code (-1 = not exited)

	// Tool usage stats
	tokenCount   int
	toolUseCount int

	// Mode flags
	fileMode    bool // true = output goes directly to file
	initialized bool
}

// BgTaskOutputConfig configures how BgTaskOutput is created.
type BgTaskOutputConfig struct {
	TaskID     string // required: unique task identifier
	OutputPath string // file path for output (required for file mode)
	MaxMemory  int    // max bytes in memory before spillover (default 8MB)
	FileMode   bool   // true = write directly to file, false = pipe mode
}

// NewBgTaskOutput creates a new BgTaskOutput.
// If config.FileMode is true, opens the output file for direct writes.
// If config.FileMode is false, initializes in-memory buffering with disk spillover.
func NewBgTaskOutput(config BgTaskOutputConfig) *BgTaskOutput {
	maxMem := config.MaxMemory
	if maxMem <= 0 {
		maxMem = defaultMaxMemory
	}

	b := &BgTaskOutput{
		taskID:   config.TaskID,
		fileMode: config.FileMode,
		exitCode: -1,
	}

	if config.FileMode {
		b.outputPath = config.OutputPath
		// Open/create output file for direct writes
		if b.outputPath != "" {
			dir := filepath.Dir(b.outputPath)
			os.MkdirAll(dir, 0o755)
			f, err := os.OpenFile(b.outputPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
			if err == nil {
				b.stdoutFile = f
			}
		}
	} else {
		// Pipe mode: in-memory buffer with disk spillover
		b.memoryBuffer = NewCircularBuffer(circularBufferSize)
		b.outputPath = config.OutputPath
	}

	b.initialized = true
	return b
}

// WriteStdout writes data to stdout output stream.
// In file mode, writes directly to the output file.
// In pipe mode, appends to memory buffer, spilling to disk if needed.
func (b *BgTaskOutput) WriteStdout(data string) {
	if !b.initialized {
		return
	}

	now := time.Now()
	atomic.AddInt64(&b.totalBytes, int64(len(data)))
	atomic.StoreInt64(&b.lastActivityMs, now.UnixMilli())

	// Count newlines
	newLines := int64(strings.Count(data, "\n"))
	atomic.AddInt64(&b.totalLines, newLines)

	if b.fileMode {
		b.mu.Lock()
		if b.stdoutFile != nil {
			b.stdoutFile.WriteString(data)
		}
		b.mu.Unlock()
		return
	}

	// Pipe mode: write to memory buffer
	b.mu.Lock()
	defer b.mu.Unlock()

	b.memoryBuffer.Append(data)
	b.memoryBytes += int64(len(data))

	// Check if we need to spill to disk
	if b.memoryBytes > defaultMaxMemory && b.spillFile == nil {
		b.spillToDisk()
	}

	// If already spilled, write new data to spill file
	if b.spillFile != nil {
		b.spillFile.WriteString(data)
	}
}

// WriteStderr writes data to stderr output stream.
// For simplicity, stderr goes to the same output as stdout but prefixed.
func (b *BgTaskOutput) WriteStderr(data string) {
	if !b.initialized {
		return
	}

	formatted := "STDERR:\n" + data
	b.WriteStdout(formatted)
}

// spillToDisk flushes the memory buffer to a spill file and switches to file mode
// for subsequent writes. Called when memory usage exceeds the limit.
func (b *BgTaskOutput) spillToDisk() {
	if b.outputPath == "" {
		return
	}

	spillPath := b.outputPath + ".spill"
	f, err := os.OpenFile(spillPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return
	}

	// Flush all buffered data to the spill file
	lines := b.memoryBuffer.GetAll()
	for _, line := range lines {
		f.WriteString(line)
	}

	b.spillFile = f
	b.spillPath = spillPath
	// Keep memoryBuffer for recent incoming data (tail of the stream)
}

// GetStdout returns the captured stdout content.
// In file mode, reads from the output file.
// In pipe mode, combines memory buffer and spill file content.
func (b *BgTaskOutput) GetStdout() string {
	if !b.initialized {
		return ""
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.fileMode {
		// Read from file
		if b.stdoutFile == nil {
			return ""
		}
		// Close for reading, but keep a reference — we need to reopen
		// Actually, on Windows we can't read while the file is open for writing.
		// So we just return recent lines from the circular buffer if available.
		if b.memoryBuffer != nil {
			return strings.Join(b.memoryBuffer.GetAll(), "")
		}
		// For file mode, the file is kept open. We do a separate open for reading.
		data, err := os.ReadFile(b.outputPath)
		if err != nil {
			return ""
		}
		return string(data)
	}

	// Pipe mode
	var sb strings.Builder

	// If spilled to disk, read the spill file
	if b.spillFile != nil && b.spillPath != "" {
		data, err := os.ReadFile(b.spillPath)
		if err == nil {
			sb.Write(data)
		}
	}

	// Append recent memory buffer content
	if b.memoryBuffer != nil {
		for _, line := range b.memoryBuffer.GetAll() {
			sb.WriteString(line)
		}
	}

	return sb.String()
}

// Tail returns the last N lines of output.
// In file mode, reads the tail of the output file.
// In pipe mode, uses the circular buffer for fast access.
func (b *BgTaskOutput) Tail(n int) []string {
	if !b.initialized {
		return nil
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.memoryBuffer != nil {
		return b.memoryBuffer.Tail(n)
	}

	// File mode without circular buffer: read file tail
	if b.outputPath != "" {
		return b.tailFile(n)
	}

	return nil
}

// tailFile reads the last N lines from the output file.
// Reads the last 4096 bytes and extrapolates (matching upstream behavior).
func (b *BgTaskOutput) tailFile(n int) []string {
	if b.outputPath == "" {
		return nil
	}

	f, err := os.Open(b.outputPath)
	if err != nil {
		return nil
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil || info.Size() == 0 {
		return nil
	}

	// Read from the end of the file
	bufSize := int64(4096)
	if info.Size() < bufSize {
		bufSize = info.Size()
	}

	_, err = f.Seek(-bufSize, io.SeekEnd)
	if err != nil {
		return nil
	}

	reader := bufio.NewReader(f)
	var lines []string
	for {
		line, err := reader.ReadString('\n')
		if err != nil && line == "" {
			break
		}
		if line != "" {
			lines = append(lines, strings.TrimSuffix(line, "\n"))
		}
		if err != nil {
			break
		}
	}

	// Return last N lines
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
}

// GetProgress returns a progress snapshot of the task.
// This is safe to call concurrently and provides a consistent view.
func (b *BgTaskOutput) GetProgress() BgTaskProgress {
	if !b.initialized {
		return BgTaskProgress{ExitCode: -1}
	}

	totalLines := atomic.LoadInt64(&b.totalLines)
	totalBytes := atomic.LoadInt64(&b.totalBytes)
	lastActivityMs := atomic.LoadInt64(&b.lastActivityMs)
	isComplete := atomic.LoadInt32(&b.isComplete) == 1
	exitCode := atomic.LoadInt32(&b.exitCode)

	b.mu.Lock()
	description := b.description
	tokenCount := b.tokenCount
	toolUseCount := b.toolUseCount
	b.mu.Unlock()

	lastActivity := time.UnixMilli(lastActivityMs)

	// Get tail lines
	var last5, last100 string
	if b.memoryBuffer != nil {
		last5Lines := b.memoryBuffer.Tail(5)
		last100Lines := b.memoryBuffer.Tail(100)
		last5 = strings.Join(last5Lines, "")
		last100 = strings.Join(last100Lines, "")
	} else if b.outputPath != "" {
		lines := b.tailFile(100)
		if len(lines) > 0 {
			last100 = strings.Join(lines, "\n")
		}
		if len(lines) > 5 {
			last5 = strings.Join(lines[len(lines)-5:], "\n")
		} else {
			last5 = last100
		}
	}

	return BgTaskProgress{
		TotalLines:   totalLines,
		TotalBytes:   totalBytes,
		LastActivity: lastActivity,
		IsComplete:   isComplete,
		ExitCode:     exitCode,
		Last5Lines:   last5,
		Last100Lines: last100,
		Description:  description,
		TokenCount:   tokenCount,
		ToolUseCount: toolUseCount,
	}
}

// UpdateProgress updates the progress tracker from agent messages.
// This is the equivalent of upstream's updateProgressFromMessage().
// Call this whenever an agent produces a new message to keep progress current.
func (b *BgTaskOutput) UpdateProgress(description string, tokens int, toolUses int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if description != "" {
		b.description = description
	}
	if tokens > 0 {
		b.tokenCount = tokens
	}
	if toolUses > 0 {
		b.toolUseCount = toolUses
	}
	atomic.StoreInt64(&b.lastActivityMs, time.Now().UnixMilli())
}

// SetComplete marks the task as completed and records the exit code.
func (b *BgTaskOutput) SetComplete(code int) {
	atomic.StoreInt32(&b.isComplete, 1)
	atomic.StoreInt32(&b.exitCode, int32(code))
	atomic.StoreInt64(&b.lastActivityMs, time.Now().UnixMilli())

	b.mu.Lock()
	defer b.mu.Unlock()

	// Close files
	if b.stdoutFile != nil {
		b.stdoutFile.Close()
		b.stdoutFile = nil
	}
	if b.spillFile != nil {
		b.spillFile.Close()
		b.spillFile = nil
	}
}

// IsComplete returns whether the task has finished.
func (b *BgTaskOutput) IsComplete() bool {
	return atomic.LoadInt32(&b.isComplete) == 1
}

// GetExitCode returns the exit code (-1 if not yet exited).
func (b *BgTaskOutput) GetExitCode() int32 {
	return atomic.LoadInt32(&b.exitCode)
}

// GetTaskID returns the task identifier.
func (b *BgTaskOutput) GetTaskID() string {
	return b.taskID
}

// GetOutputPath returns the file path for the task's output.
func (b *BgTaskOutput) GetOutputPath() string {
	return b.outputPath
}

// Close releases all resources associated with the task output.
func (b *BgTaskOutput) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.stdoutFile != nil {
		b.stdoutFile.Close()
		b.stdoutFile = nil
	}
	if b.stderrFile != nil {
		b.stderrFile.Close()
		b.stderrFile = nil
	}
	if b.spillFile != nil {
		b.spillFile.Close()
		b.spillFile = nil
	}
}

// BgTaskOutputStore is a thread-safe registry of background task outputs.
// It provides a central place to look up task output by ID.
type BgTaskOutputStore struct {
	mu    sync.RWMutex
	tasks map[string]*BgTaskOutput
}

// NewBgTaskOutputStore creates an empty store.
func NewBgTaskOutputStore() *BgTaskOutputStore {
	return &BgTaskOutputStore{
		tasks: make(map[string]*BgTaskOutput),
	}
}

// Register adds a task output to the store.
func (s *BgTaskOutputStore) Register(task *BgTaskOutput) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tasks[task.taskID] = task
}

// Get retrieves a task output by ID, or nil if not found.
func (s *BgTaskOutputStore) Get(taskID string) *BgTaskOutput {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.tasks[taskID]
}

// Remove removes a task from the store and closes its resources.
func (s *BgTaskOutputStore) Remove(taskID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if task, ok := s.tasks[taskID]; ok {
		task.Close()
		delete(s.tasks, taskID)
	}
}

// List returns all registered task IDs.
func (s *BgTaskOutputStore) List() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]string, 0, len(s.tasks))
	for id := range s.tasks {
		ids = append(ids, id)
	}
	return ids
}

// GenerateOutputPath creates a standard output file path for a background task.
// Matches upstream's getTaskOutputPath(taskId) pattern.
// Returns a path like: .claude/task-outputs/{taskID}.txt
func GenerateOutputPath(baseDir string, taskID string) string {
	outputDir := filepath.Join(baseDir, ".claude", "task-outputs")
	return filepath.Join(outputDir, fmt.Sprintf("%s.txt", taskID))
}

// ReadFileTail reads the last N bytes from a file and returns them as a string.
// Used for progress polling on file-mode tasks.
func ReadFileTail(path string, byteCount int) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return "", err
	}

	size := info.Size()
	if size == 0 {
		return "", nil
	}

	if int64(byteCount) > size {
		byteCount = int(size)
	}

	_, err = f.Seek(-int64(byteCount), io.SeekEnd)
	if err != nil {
		return "", err
	}

	data := make([]byte, byteCount)
	n, err := f.Read(data)
	if err != nil && n == 0 {
		return "", err
	}
	return string(data[:n]), nil
}

// CountFileLines counts the total number of lines in a file.
// A file "a\nb\nc\n" has 3 lines (trailing newline is not an extra line).
// An empty file has 0 lines.
func CountFileLines(path string) (int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return 0, err
	}
	if info.Size() == 0 {
		return 0, nil
	}

	var count int64
	reader := bufio.NewReader(f)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			// Count non-empty last line even without trailing newline
			if line != "" {
				count++
			}
			if err == io.EOF {
				break
			}
			return count, err
		}
		count++
	}
	return count, nil
}
