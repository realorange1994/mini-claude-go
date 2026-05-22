package rgrep

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// splitGlobPatterns splits a glob string on commas and whitespace,
// respecting brace groups. E.g. "*.ts, *.js" -> ["*.ts", "*.js"].
func splitGlobPatterns(glob string) []string {
	var parts []string
	var current strings.Builder
	inBrace := false
	for _, c := range glob {
		switch c {
		case '{':
			inBrace = true
			current.WriteRune(c)
		case '}':
			inBrace = false
			current.WriteRune(c)
		case ',', ' ':
			if !inBrace && current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(c)
		}
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	return parts
}

// Search performs a full search using the given config.
func Search(cfg SearchConfig) SearchResult {
	if cfg.Ctx == nil {
		cfg.Ctx = context.Background()
	}

	searchPattern := cfg.Pattern
	if cfg.FixedStrings {
		searchPattern = regexp.QuoteMeta(searchPattern)
	}
	if cfg.CaseInsensitive {
		searchPattern = "(?i)" + searchPattern
	}
	if cfg.Multiline {
		searchPattern = "(?s)" + searchPattern
	}

	re, err := regexp.Compile(searchPattern)
	if err != nil {
		return SearchResult{Err: fmt.Errorf("invalid regex: %w", err)}
	}

	info, err := os.Stat(cfg.Path)
	if err != nil {
		return SearchResult{Err: fmt.Errorf("path not found: %v", err)}
	}

	// Pre-allocate reusable buffers to avoid per-file allocations
	scannerBuf := make([]byte, 64*1024)
	binCheckBuf := make([]byte, 8192)
	readBuf := make([]byte, 32*1024) // for io.ReadAll in multiline mode
	s := &searcher{
		re:            re,
		cfg:           cfg,
		scannerBuf:    scannerBuf,
		binCheckBuf:   binCheckBuf,
		readBuf:       readBuf,
		multiline:     cfg.Multiline,
		literalPrefix: extractLiteralPrefix(re),
	}

	if info.Mode().IsRegular() {
		return s.searchSingleFile(cfg.Path, cfg.Path)
	}

	globs := splitGlobPatterns(cfg.Glob)
	walkOpts := WalkOptions{
		Root:             cfg.Path,
		MaxDepth:         cfg.MaxDepth,
		Globs:            globs,
		TypeFilter:       cfg.TypeFilter,
		Excludes:         cfg.Excludes,
		RespectGitIgnore: true,
		Ctx:              cfg.Ctx,
		MaxFilesize:      cfg.MaxFilesize,
	}

	switch cfg.OutputMode {
	case OutputFilesWithMatch:
		return s.searchDirFilesOnly(walkOpts)
	case OutputCount:
		return s.searchDirCount(walkOpts)
	default:
		return s.searchDirContent(walkOpts)
	}
}

// searcher holds reusable buffers across multiple file searches,
// eliminating per-file allocations like ripgrep's SearchWorker pattern.
type searcher struct {
	re          *regexp.Regexp
	cfg         SearchConfig
	multiline   bool
	scannerBuf  []byte // reusable scanner buffer (64KB)
	binCheckBuf []byte // reusable binary check buffer (8KB)
	readBuf     []byte // reusable read buffer for multiline (32KB)
	literalPrefix []byte // fast-scan literal prefix extracted from regex
}

// extractLiteralPrefix extracts a literal prefix from the regex pattern.
// This enables a two-phase search like ripgrep: first do a fast bytes.Index
// scan for the literal prefix, then run the full regex only on lines/files
// that contain the prefix. Returns nil if no useful prefix can be extracted.
func extractLiteralPrefix(re *regexp.Regexp) []byte {
	prefix, complete := re.LiteralPrefix()
	if len(prefix) >= 2 {
		return []byte(prefix)
	}
	// For single-char prefixes, only use if the regex is simple enough
	// that the prefix scan would actually help (not a complex alternation)
	if len(prefix) == 1 && complete {
		return []byte(prefix)
	}
	return nil
}

// ---------- single file search ----------

func (s *searcher) searchSingleFile(path, relPath string) SearchResult {
	st := &searchState{cfg: s.cfg, root: s.cfg.Path, results: make([]Result, 0, 32)}
	entry := WalkEntry{Path: path, RelPath: relPath}
	st.filesSearched = 1

	switch s.cfg.OutputMode {
	case OutputFilesWithMatch:
		if s.fileHasMatch(path) {
			st.totalMatches = 1
			st.results = append(st.results, Result{Path: relPath})
		}
	case OutputCount:
		count := s.countInFile(path)
		if count > 0 {
			st.totalMatches = count
			st.results = append(st.results, Result{Path: relPath, LineNum: count})
		}
	default:
		s.searchFileContent(entry, st)
	}

	return SearchResult{
		Results:       st.results,
		FilesSearched: st.filesSearched,
		TotalMatches:  st.totalMatches,
		Truncated:     s.cfg.HeadLimit > 0 && len(st.results) >= s.cfg.HeadLimit,
	}
}

// ---------- directory search: files-only ----------

func (s *searcher) searchDirFilesOnly(walkOpts WalkOptions) SearchResult {
	st := &searchState{cfg: s.cfg, root: walkOpts.Root}

	WalkDirStream(walkOpts, func(entry WalkEntry) error {
		st.filesSearched++
		if s.fileHasMatch(entry.Path) {
			if st.skipped < s.cfg.Offset {
				st.skipped++
			} else {
				st.totalMatches++
				st.addResult(Result{Path: makeRelative(st.root, entry)})
			}
		}
		if st.isDone() {
			return context.Canceled
		}
		return nil
	})

	return SearchResult{
		Results:       st.results,
		FilesSearched: st.filesSearched,
		TotalMatches:  st.totalMatches,
		Truncated:     s.cfg.HeadLimit > 0 && len(st.results) >= s.cfg.HeadLimit,
	}
}

// ---------- directory search: count ----------

func (s *searcher) searchDirCount(walkOpts WalkOptions) SearchResult {
	st := &searchState{cfg: s.cfg, root: walkOpts.Root}

	WalkDirStream(walkOpts, func(entry WalkEntry) error {
		st.filesSearched++
		count := s.countInFile(entry.Path)
		if count > 0 {
			st.totalMatches += count
			st.addResult(Result{
				Path:    makeRelative(st.root, entry),
				LineNum: count,
			})
		}
		if st.isDone() {
			return context.Canceled
		}
		return nil
	})

	if s.cfg.Offset > 0 && s.cfg.Offset < len(st.results) {
		st.results = st.results[s.cfg.Offset:]
	}

	return SearchResult{
		Results:       st.results,
		FilesSearched: st.filesSearched,
		TotalMatches:  st.totalMatches,
		Truncated:     s.cfg.HeadLimit > 0 && len(st.results) >= s.cfg.HeadLimit,
	}
}

// ---------- directory search: content ----------

func (s *searcher) searchDirContent(walkOpts WalkOptions) SearchResult {
	st := &searchState{cfg: s.cfg, root: walkOpts.Root}

	WalkDirStream(walkOpts, func(entry WalkEntry) error {
		st.filesSearched++
		s.searchFileContent(entry, st)
		if st.isDone() {
			return context.Canceled
		}
		return nil
	})

	return SearchResult{
		Results:       st.results,
		FilesSearched: st.filesSearched,
		TotalMatches:  st.totalMatches,
		Truncated:     s.cfg.HeadLimit > 0 && len(st.results) >= s.cfg.HeadLimit,
	}
}

// ---------- core search operations ----------

// fileHasMatch checks if a file contains a regex match.
// Two-phase search: first fast-scan for literal prefix with bytes.Index,
// then run full regex only on candidate files (like ripgrep's fast_line_regex).
func (s *searcher) fileHasMatch(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	if s.isBinaryFile(f) {
		return false
	}
	f.Seek(0, io.SeekStart)

	if s.multiline {
		data := s.readWholeFile(f)
		if data == nil {
			return false
		}
		// Phase 1: fast literal scan
		if s.literalPrefix != nil && bytes.Index(data, s.literalPrefix) < 0 {
			return false
		}
		return s.re.Match(data)
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(s.scannerBuf, 1024*1024)

	// Phase 1: fast literal prefix scan (skip regex on lines without prefix)
	if s.literalPrefix != nil {
		for scanner.Scan() {
			if bytes.Index(scanner.Bytes(), s.literalPrefix) >= 0 {
				// Phase 2: run full regex on candidate line
				if s.re.Match(scanner.Bytes()) {
					return true
				}
			}
		}
		return false
	}

	// No literal prefix — must run regex on every line
	for scanner.Scan() {
		if s.re.Match(scanner.Bytes()) {
			return true
		}
	}
	return false
}

// countInFile counts regex matches in a file.
// Two-phase: fast literal scan first, full regex only on candidates.
func (s *searcher) countInFile(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()

	if s.isBinaryFile(f) {
		return 0
	}
	f.Seek(0, io.SeekStart)

	if s.multiline {
		data := s.readWholeFile(f)
		if data == nil {
			return 0
		}
		// Phase 1: fast literal scan
		if s.literalPrefix != nil && bytes.Index(data, s.literalPrefix) < 0 {
			return 0
		}
		return len(s.re.FindAllIndex(data, -1))
	}

	count := 0
	scanner := bufio.NewScanner(f)
	scanner.Buffer(s.scannerBuf, 1024*1024)

	if s.literalPrefix != nil {
		// Phase 1: only regex-check lines containing the prefix
		for scanner.Scan() {
			lineBytes := scanner.Bytes()
			if bytes.Index(lineBytes, s.literalPrefix) >= 0 {
				if len(lineBytes) > MaxGrepLineLen {
					lineBytes = lineBytes[:MaxGrepLineLen]
				}
				matches := s.re.FindAllIndex(lineBytes, -1)
				count += len(matches)
			}
		}
	} else {
		// No prefix — run regex on every line
		for scanner.Scan() {
			lineBytes := scanner.Bytes()
			if len(lineBytes) > MaxGrepLineLen {
				lineBytes = lineBytes[:MaxGrepLineLen]
			}
			matches := s.re.FindAllIndex(lineBytes, -1)
			count += len(matches)
		}
	}
	return count
}

// searchFileContent searches a file and adds results to the state.
// For non-multiline: uses byte-level ring buffer for context (avoids string alloc).
// For multiline: reads whole file, maps byte offsets to line numbers.
func (s *searcher) searchFileContent(entry WalkEntry, st *searchState) {
	f, err := os.Open(entry.Path)
	if err != nil {
		return
	}
	defer f.Close()

	if s.isBinaryFile(f) {
		return
	}
	f.Seek(0, io.SeekStart)

	relPath := makeRelative(st.root, entry)

	if s.multiline {
		s.searchFileContentMultiline(f, relPath, st)
		return
	}

	ctxBefore := s.cfg.ContextBefore
	ctxAfter := s.cfg.ContextAfter

	// Byte-level ring buffer for before-context (avoids string allocation)
	// Each entry stores the raw line bytes from scanner
	ringBuf := make([][]byte, ctxBefore)
	ringLineNums := make([]int, ctxBefore)
	ringIdx := 0
	ringCount := 0

	scanner := bufio.NewScanner(f)
	scanner.Buffer(s.scannerBuf, 1024*1024)

	lineNum := 0
	afterLeft := 0

	for scanner.Scan() {
		lineBytes := scanner.Bytes()
		lineNum++

		// Truncate long lines
		truncated := false
		if len(lineBytes) > MaxGrepLineLen {
			lineBytes = lineBytes[:MaxGrepLineLen]
			truncated = true
		}

		// Phase 1: fast literal prefix scan — skip regex on non-candidate lines
		matched := false
		if s.literalPrefix != nil {
			if bytes.Index(lineBytes, s.literalPrefix) >= 0 {
				matched = s.re.Match(lineBytes)
			}
		} else {
			matched = s.re.Match(lineBytes)
		}

		if matched {
			st.totalMatches++
			if st.skipped < st.cfg.Offset {
				st.skipped++
				afterLeft = 0
				// Still store in ring buffer for potential later use
				if ctxBefore > 0 {
					lineCopy := make([]byte, len(lineBytes))
					copy(lineCopy, lineBytes)
					if truncated {
						lineCopy = append(lineCopy, '.', '.', '.')
					}
					ringBuf[ringIdx] = lineCopy
					ringLineNums[ringIdx] = lineNum
					ringIdx = (ringIdx + 1) % ctxBefore
					ringCount++
				}
				continue
			}

			// Emit before-context from ring buffer
			if ctxBefore > 0 && ringCount > 0 {
				n := min(ringCount, ctxBefore)
				startIdx := ringIdx - n
				if startIdx < 0 {
					startIdx += ctxBefore
				}
				for j := 0; j < n; j++ {
					idx := (startIdx + j) % ctxBefore
					if ringBuf[idx] != nil {
						st.addResult(Result{
							Path:    relPath,
							LineNum: ringLineNums[idx],
							Line:    string(ringBuf[idx]),
						})
						if st.isDone() {
							return
						}
					}
				}
			}

			// Emit the match line
			lineStr := string(lineBytes)
			if truncated {
				lineStr += "..."
			}
			st.addResult(Result{
				Path:    relPath,
				LineNum: lineNum,
				Line:    lineStr,
			})
			if st.isDone() {
				return
			}

			afterLeft = ctxAfter
		} else if afterLeft > 0 {
			lineStr := string(lineBytes)
			if len(lineBytes) > MaxGrepLineLen {
				lineStr = string(lineBytes[:MaxGrepLineLen]) + "..."
			}
			st.addResult(Result{
				Path:    relPath,
				LineNum: lineNum,
				Line:    lineStr,
			})
			if st.isDone() {
				return
			}
			afterLeft--
		}

		// Store line bytes in ring buffer for before-context
		if ctxBefore > 0 {
			// Copy bytes since scanner reuses its buffer on next Scan()
			lineCopy := make([]byte, len(lineBytes))
			copy(lineCopy, lineBytes)
			if len(lineBytes) > MaxGrepLineLen {
				lineCopy = append(lineCopy[:MaxGrepLineLen], '.', '.', '.')
			}
			ringBuf[ringIdx] = lineCopy
			ringLineNums[ringIdx] = lineNum
			ringIdx = (ringIdx + 1) % ctxBefore
			ringCount++
		}
	}
}

// searchFileContentMultiline handles multiline mode by reading the entire file
// and finding matches that may span multiple lines.
func (s *searcher) searchFileContentMultiline(f *os.File, relPath string, st *searchState) {
	data := s.readWholeFile(f)
	if data == nil {
		return
	}

	// Phase 1: fast literal scan — skip regex if prefix not found
	if s.literalPrefix != nil && bytes.Index(data, s.literalPrefix) < 0 {
		return
	}

	ctxBefore := s.cfg.ContextBefore
	ctxAfter := s.cfg.ContextAfter

	// Find all matches with their byte offsets
	matches := s.re.FindAllIndex(data, -1)
	if len(matches) == 0 {
		return
	}

	// Build byte-offset → line-number lookup using newline positions
	// This avoids strings.Split which creates a full string slice
	newlineOffsets := buildNewlineIndex(data)

	// lineForByteOffset returns 1-based line number for a byte offset
	lineForByteOffset := func(byteOff int) int {
		lo, hi := 0, len(newlineOffsets)-1
		for lo < hi {
			mid := (lo + hi + 1) / 2
			if newlineOffsets[mid] <= byteOff {
				lo = mid
			} else {
				hi = mid - 1
			}
		}
		return lo + 1
	}

	// extractLine extracts line content (1-based) from data using newline offsets
	extractLine := func(lineNum int) string {
		if lineNum < 1 || lineNum > len(newlineOffsets) {
			return ""
		}
		start := newlineOffsets[lineNum-1]
		end := len(data)
		if lineNum < len(newlineOffsets) {
			end = newlineOffsets[lineNum]
		}
		// Strip trailing newline
		if end > start && data[end-1] == '\n' {
			end--
		}
		if end > start && data[end-1] == '\r' {
			end--
		}
		line := string(data[start:end])
		if len(line) > MaxGrepLineLen {
			line = line[:MaxGrepLineLen] + "..."
		}
		return line
	}

	emitted := make(map[int]bool)

	for _, m := range matches {
		startLine := lineForByteOffset(m[0])
		endByte := m[1]
		if endByte > 0 {
			endByte--
		}
		endLine := lineForByteOffset(endByte)
		if endLine < startLine {
			endLine = startLine
		}

		st.totalMatches++
		if st.skipped < st.cfg.Offset {
			st.skipped++
			continue
		}

		// Emit before-context lines
		for i := max(1, startLine-ctxBefore); i < startLine; i++ {
			if !emitted[i] {
				emitted[i] = true
				st.addResult(Result{Path: relPath, LineNum: i, Line: extractLine(i)})
				if st.isDone() {
					return
				}
			}
		}

		// Emit match lines
		for i := startLine; i <= endLine; i++ {
			if !emitted[i] {
				emitted[i] = true
				st.addResult(Result{Path: relPath, LineNum: i, Line: extractLine(i)})
				if st.isDone() {
					return
				}
			}
		}

		// Emit after-context lines
		totalLines := len(newlineOffsets)
		for i := endLine + 1; i <= min(totalLines, endLine+ctxAfter); i++ {
			if !emitted[i] {
				emitted[i] = true
				st.addResult(Result{Path: relPath, LineNum: i, Line: extractLine(i)})
				if st.isDone() {
					return
				}
			}
		}
	}
}

// ---------- helper functions ----------

// buildNewlineIndex creates an array of byte offsets for each newline in data.
// Index 0 = 0 (start of first line), then each subsequent index is the
// byte offset after each \n. This enables O(log n) line-number lookup.
func buildNewlineIndex(data []byte) []int {
	// Pre-allocate: average line is ~80 bytes, so estimate len(data)/80 lines
	estimated := len(data) / 40 + 2
	offsets := make([]int, 0, estimated)
	offsets = append(offsets, 0) // line 1 starts at offset 0
	for i, b := range data {
		if b == '\n' {
			offsets = append(offsets, i+1)
		}
	}
	// Add sentinel for end of file
	if len(data) > 0 && data[len(data)-1] != '\n' {
		offsets = append(offsets, len(data))
	}
	return offsets
}

// readWholeFile reads an entire file into a buffer, reusing the searcher's
// readBuf as initial capacity. Pre-allocates based on file size to avoid
// incremental growth.
func (s *searcher) readWholeFile(f *os.File) []byte {
	// Get file size for pre-allocation
	info, err := f.Stat()
	if err != nil {
		return nil
	}
	size := int(info.Size()) + 1 // +1 for trailing newline safety

	buf := make([]byte, 0, max(size, len(s.readBuf)))
	buf, err = io.ReadAll(f)
	if err != nil {
		return nil
	}
	return buf
}

// isBinaryFile checks the first 8KB of a file for null bytes.
// Uses the reusable binCheckBuf to avoid per-file allocation.
func (s *searcher) isBinaryFile(f *os.File) bool {
	n, _ := f.Read(s.binCheckBuf)
	return n > 0 && bytes.IndexByte(s.binCheckBuf[:n], 0) >= 0
}

// ---------- shared types ----------

type searchState struct {
	results       []Result
	filesSearched int
	totalMatches  int
	skipped       int
	cfg           SearchConfig
	done          bool
	root          string
}

func (st *searchState) addResult(r Result) {
	if st.cfg.HeadLimit > 0 && len(st.results) >= st.cfg.HeadLimit {
		st.done = true
		st.results = st.results[:st.cfg.HeadLimit]
		return
	}
	st.results = append(st.results, r)
}

func (st *searchState) isDone() bool {
	return st.done
}

// ---------- utility functions ----------

// MaxGrepLineLen is the maximum length of a line in content output.
const MaxGrepLineLen = 500

// TruncateLine truncates a line to MaxGrepLineLen characters.
func TruncateLine(line string) string {
	if len(line) <= MaxGrepLineLen {
		return line
	}
	return line[:MaxGrepLineLen] + "..."
}

// makeRelative returns a relative path for display.
func makeRelative(root string, entry WalkEntry) string {
	if entry.RelPath != "" && entry.RelPath != "." {
		return filepathToSlash(entry.RelPath)
	}
	rel, err := filepath.Rel(root, entry.Path)
	if err != nil {
		return entry.Path
	}
	return filepathToSlash(rel)
}

func filepathToSlash(p string) string {
	return strings.ReplaceAll(p, "\\", "/")
}

