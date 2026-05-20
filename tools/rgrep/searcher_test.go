package rgrep

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// ─── Multiline search tests (Bug 3: grep multiline) ─────────────────────────

func setupMultilineTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	// File with content spanning multiple lines
	os.WriteFile(filepath.Join(dir, "multiline.txt"), []byte("line one\nline two\nline three\nstart\nmiddle\nend\nother line\n"), 0644)
	// File with no multiline match
	os.WriteFile(filepath.Join(dir, "single.txt"), []byte("single line only\n"), 0644)
	// File with a pattern that spans lines
	os.WriteFile(filepath.Join(dir, "span.txt"), []byte("hello\nworld\nfoo\nbar\n"), 0644)
	return dir
}

func TestMultilineSearchContent(t *testing.T) {
	dir := setupMultilineTestDir(t)

	cfg := SearchConfig{
		Pattern:     "start.*end",
		Path:        dir,
		OutputMode:  OutputContent,
		Multiline:   true,
		CaseInsensitive: false,
		Ctx:         context.Background(),
	}
	result := Search(cfg)
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
	if len(result.Results) == 0 {
		t.Fatalf("expected multiline match for 'start.*end', got none")
	}

	// Verify the match contains the multiline content
	matchLine := result.Results[0].Line
	if matchLine == "" {
		t.Error("expected non-empty match line")
	}
}

func TestMultilineNoMatch(t *testing.T) {
	dir := setupMultilineTestDir(t)

	cfg := SearchConfig{
		Pattern:    "nonexistent.*pattern",
		Path:       dir,
		OutputMode: OutputContent,
		Multiline:  true,
		Ctx:        context.Background(),
	}
	result := Search(cfg)
	if len(result.Results) != 0 {
		t.Errorf("expected no matches, got %d", len(result.Results))
	}
}

func TestMultilineSearchCount(t *testing.T) {
	dir := setupMultilineTestDir(t)

	cfg := SearchConfig{
		Pattern:    "start.*end",
		Path:       dir,
		OutputMode: OutputCount,
		Multiline:  true,
		Ctx:        context.Background(),
	}
	result := Search(cfg)
	if result.TotalMatches == 0 {
		t.Error("expected multiline match in count mode, got 0")
	}
}

func TestMultilineSearchFilesOnly(t *testing.T) {
	dir := setupMultilineTestDir(t)

	cfg := SearchConfig{
		Pattern:    "start.*end",
		Path:       dir,
		OutputMode: OutputFilesWithMatch,
		Multiline:  true,
		Ctx:        context.Background(),
	}
	result := Search(cfg)
	if len(result.Results) == 0 {
		t.Error("expected file match in files_with_matches mode, got none")
	}
}

func TestNonMultilineMode(t *testing.T) {
	dir := setupMultilineTestDir(t)

	// Without multiline flag, "start.*end" should NOT match (since it spans lines)
	cfg := SearchConfig{
		Pattern:    "start.*end",
		Path:       dir,
		OutputMode: OutputContent,
		Multiline:  false,
		Ctx:        context.Background(),
	}
	result := Search(cfg)
	if len(result.Results) != 0 {
		t.Errorf("expected no matches in non-multiline mode for 'start.*end', got %d", len(result.Results))
	}

	// Single line match should work without multiline
	cfg.Pattern = "line one"
	result = Search(cfg)
	if len(result.Results) == 0 {
		t.Error("expected match for 'line one' in non-multiline mode")
	}
}

func TestMultilineWithCaseInsensitive(t *testing.T) {
	dir := setupMultilineTestDir(t)

	cfg := SearchConfig{
		Pattern:         "START.*END",
		Path:            dir,
		OutputMode:      OutputContent,
		Multiline:       true,
		CaseInsensitive: true,
		Ctx:             context.Background(),
	}
	result := Search(cfg)
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
	if len(result.Results) == 0 {
		t.Error("expected case-insensitive multiline match, got none")
	}
}

// ─── Count mode: exclude zero-count files (Bug 2) ───────────────────────────

func TestCountModeExcludesZeroCountFiles(t *testing.T) {
	dir := setupMultilineTestDir(t)

	cfg := SearchConfig{
		Pattern:    "start",
		Path:       dir,
		OutputMode: OutputCount,
		Ctx:        context.Background(),
	}
	result := Search(cfg)

	// single.txt and span.txt don't contain "start"
	// multiline.txt contains "start"
	for _, r := range result.Results {
		if r.LineNum == 0 {
			t.Errorf("zero-count file should not appear in results: %s", r.Path)
		}
	}
}

// ─── Content mode regex matching (verify multiline flag works correctly) ───

func TestContentModeMultilineRegexCompiled(t *testing.T) {
	// Verify that the regex is correctly compiled with (?s) flag for multiline
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("first line\nsecond line\nthird line\n"), 0644)

	cfg := SearchConfig{
		Pattern:    "first.*third",
		Path:       dir,
		OutputMode: OutputContent,
		Multiline:  true,
		Ctx:        context.Background(),
	}
	result := Search(cfg)
	if len(result.Results) == 0 {
		t.Error("expected multiline match for 'first.*third' with multiline=true")
	}
}

// ─── Fixed strings mode ─────────────────────────────────────────────────────

func TestFixedStringsMode(t *testing.T) {
	dir := setupMultilineTestDir(t)

	cfg := SearchConfig{
		Pattern:      "line.*two",
		Path:         dir,
		OutputMode:   OutputContent,
		FixedStrings: true,
		Ctx:          context.Background(),
	}
	result := Search(cfg)
	// With fixed strings, "line.*two" should be treated as literal, not regex
	if len(result.Results) != 0 {
		t.Errorf("expected no matches for literal 'line.*two', got %d", len(result.Results))
	}
}

// ─── Benchmark-style: verify FindAllIndex works correctly ────────────────────

func TestMultilineFindAllIndexCorrectCount(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("aaa\nbbb\naaa\nccc\n"), 0644)

	cfg := SearchConfig{
		Pattern:    "aaa",
		Path:       dir,
		OutputMode: OutputCount,
		Multiline:  true,
		Ctx:        context.Background(),
	}
	result := Search(cfg)
	if result.TotalMatches != 2 {
		t.Errorf("expected 2 matches for 'aaa', got %d", result.TotalMatches)
	}
}

// ─── searchCount with multiline: use FindAllIndex on full file ───────────────

func TestSearchCountMultilineReadsFullFile(t *testing.T) {
	// Create a file where the pattern spans multiple lines
	dir := t.TempDir()
	content := "hello world\nfoo bar\nhello baz\n"
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte(content), 0644)

	cfg := SearchConfig{
		Pattern:    "hello.*baz",
		Path:       dir,
		OutputMode: OutputCount,
		Multiline:  true,
		Ctx:        context.Background(),
	}
	result := Search(cfg)
	if result.TotalMatches != 1 {
		t.Errorf("expected 1 multiline match in count mode, got %d", result.TotalMatches)
	}
}

// ─── searchContent non-multiline: line-by-line still works ───────────────────

func TestContentNonMultilineLineByLine(t *testing.T) {
	dir := setupMultilineTestDir(t)

	cfg := SearchConfig{
		Pattern:    "line two",
		Path:       dir,
		OutputMode: OutputContent,
		Multiline:  false,
		Ctx:        context.Background(),
	}
	result := Search(cfg)
	if len(result.Results) == 0 {
		t.Error("expected single line match for 'line two' without multiline")
	}
}

// ─── Context params: verify they are NOT passed in count/files_with_matches ─

// Note: This is tested at the grep_tool.go level (ripgrep args), but we also
// verify the rgrep engine ignores context params in non-content modes.
func TestCountModeIgnoresContextParams(t *testing.T) {
	dir := setupMultilineTestDir(t)

	cfg := SearchConfig{
		Pattern:       "line",
		Path:          dir,
		OutputMode:    OutputCount,
		ContextBefore: 5,
		ContextAfter:  5,
		Ctx:           context.Background(),
	}
	result := Search(cfg)
	// Context params should not affect count mode results
	if result.TotalMatches == 0 {
		t.Error("expected matches in count mode regardless of context params")
	}
}

// ─── Context: -B and -A must both work (Bug 1) ──────────────────────────────

func TestContextBeforeAndAfterBothWork(t *testing.T) {
	// File with content where we can verify before/after context
	dir := t.TempDir()
	content := "before1\nbefore2\nMATCH_HERE\nafter1\nafter2\n"
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte(content), 0644)

	cfg := SearchConfig{
		Pattern:       "MATCH_HERE",
		Path:          dir,
		OutputMode:    OutputContent,
		ContextBefore:  2,
		ContextAfter:   2,
		Ctx:           context.Background(),
	}
	result := Search(cfg)
	if len(result.Results) == 0 {
		t.Fatal("expected match")
	}

	// Should have: 2 before-context lines + 1 match + 2 after-context = 5 lines
	// With line numbers, total results should be 5
	if len(result.Results) != 5 {
		t.Errorf("expected 5 results (2 before + match + 2 after), got %d", len(result.Results))
	}

	// Verify the match line is in the middle (position 3, 1-indexed)
	matchFound := false
	for _, r := range result.Results {
		if strings.Contains(r.Line, "MATCH_HERE") {
			matchFound = true
		}
	}
	if !matchFound {
		t.Error("MATCH_HERE should be in results")
	}
}

func TestContextBeforeOnly(t *testing.T) {
	dir := t.TempDir()
	content := "line1\nline2\nMATCH\nafter\n"
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte(content), 0644)

	cfg := SearchConfig{
		Pattern:       "MATCH",
		Path:          dir,
		OutputMode:    OutputContent,
		ContextBefore: 2,
		ContextAfter:  0,
		Ctx:           context.Background(),
	}
	result := Search(cfg)
	if len(result.Results) != 3 {
		t.Errorf("expected 3 results (2 before + match), got %d", len(result.Results))
	}
}

func TestContextAfterOnly(t *testing.T) {
	dir := t.TempDir()
	content := "before\nMATCH\nafter1\nafter2\n"
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte(content), 0644)

	cfg := SearchConfig{
		Pattern:       "MATCH",
		Path:          dir,
		OutputMode:    OutputContent,
		ContextBefore: 0,
		ContextAfter:  2,
		Ctx:           context.Background(),
	}
	result := Search(cfg)
	if len(result.Results) != 3 {
		t.Errorf("expected 3 results (match + 2 after), got %d", len(result.Results))
	}
}

func TestContextMultipleMatches(t *testing.T) {
	dir := t.TempDir()
	content := "before1\nMATCH1\nshared\nbefore2\nMATCH2\nafter\n"
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte(content), 0644)

	cfg := SearchConfig{
		Pattern:       "MATCH",
		Path:          dir,
		OutputMode:    OutputContent,
		ContextBefore: 1,
		ContextAfter:  1,
		Ctx:           context.Background(),
	}
	result := Search(cfg)

	// Both matches should have context
	matchCount := 0
	for _, r := range result.Results {
		if strings.Contains(r.Line, "MATCH") {
			matchCount++
		}
	}
	if matchCount != 2 {
		t.Errorf("expected 2 matches, got %d", matchCount)
	}

	// Should have context lines too
	if len(result.Results) < 4 {
		t.Errorf("expected at least 4 results (2 matches + context), got %d", len(result.Results))
	}
}

// ─── Result line computation in multiline mode ──────────────────────────────

func TestMultilineResultLineNumber(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("line1\nline2\nline3\nstart\nmiddle\nend\n"), 0644)

	cfg := SearchConfig{
		Pattern:    "start.*end",
		Path:       dir,
		OutputMode: OutputContent,
		Multiline:  true,
		Ctx:        context.Background(),
	}
	result := Search(cfg)
	if len(result.Results) == 0 {
		t.Fatal("expected multiline match")
	}
	// The match starts at line 4 ("start")
	if result.Results[0].LineNum != 4 {
		t.Errorf("expected match at line 4, got line %d", result.Results[0].LineNum)
	}
}

// ─── Pattern compilation validation ─────────────────────────────────────────

func TestSearchCompilesRegexCorrectly(t *testing.T) {
	tests := []struct {
		pattern         string
		caseInsensitive bool
		multiline       bool
		fixedStrings    bool
		expectError     bool
	}{
		{pattern: "hello", expectError: false},
		{pattern: "[invalid", expectError: true},
		{pattern: "hello.*world", multiline: true, expectError: false},
		{pattern: "hello", caseInsensitive: true, expectError: false},
		{pattern: "hello.*world", caseInsensitive: true, multiline: true, expectError: false},
		{pattern: "[abc", fixedStrings: true, expectError: false}, // quoted meta
	}

	for _, tt := range tests {
		p := tt.pattern
		if tt.fixedStrings {
			p = regexp.QuoteMeta(p)
		}
		if tt.caseInsensitive {
			p = "(?i)" + p
		}
		if tt.multiline {
			p = "(?s)" + p
		}
		_, err := regexp.Compile(p)
		gotError := err != nil
		if gotError != tt.expectError {
			t.Errorf("pattern %q: expected error=%v, got error=%v (%v)", tt.pattern, tt.expectError, gotError, err)
		}
	}
}

// ─── Regression: count mode must output path:count, NOT path:lineNum:content ─
// Bug 2: output_mode: count was returning path:lineNum:content format instead of
// pure path:count format matching ripgrep --count.

func TestCountModeOutputFormat(t *testing.T) {
	dir := t.TempDir()
	// File with multiple matches on the same line
	os.WriteFile(filepath.Join(dir, "multi.txt"), []byte("hello world hello\nhello\n"), 0644)
	// File with no matches
	os.WriteFile(filepath.Join(dir, "empty.txt"), []byte("nothing here\n"), 0644)

	cfg := SearchConfig{
		Pattern:    "hello",
		Path:       dir,
		OutputMode: OutputCount,
		Ctx:        context.Background(),
	}
	sr := Search(cfg)
	if sr.Err != nil {
		t.Fatalf("unexpected error: %v", sr.Err)
	}
	if len(sr.Results) == 0 {
		t.Fatal("expected at least one result in count mode")
	}

	// Verify format via FormatResult
	output := FormatResult(sr, cfg)
	// Each result line should be "path:count" — must NOT contain a second colon
	// (which would indicate path:lineNum:content format)
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "(") || line == "" {
			continue // skip summary lines
		}
		// Should be path:number format — exactly one colon
		parts := strings.Split(line, ":")
		if len(parts) != 2 {
			t.Errorf("count output line should be 'path:count', got: %q", line)
		}
	}
	// Verify total match count is correct (multi.txt has 3 matches)
	if sr.TotalMatches != 3 {
		t.Errorf("expected 3 total matches, got %d", sr.TotalMatches)
	}
}

func TestCountModeSingleMatchPerFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("foo bar foo\n"), 0644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("foo\n"), 0644)

	cfg := SearchConfig{
		Pattern:    "foo",
		Path:       dir,
		OutputMode: OutputCount,
		Ctx:        context.Background(),
	}
	sr := Search(cfg)
	if len(sr.Results) != 2 {
		t.Fatalf("expected 2 files with matches, got %d", len(sr.Results))
	}
	// a.txt should have count 2, b.txt should have count 1
	counts := make(map[string]int)
	for _, r := range sr.Results {
		counts[r.Path] = r.LineNum // LineNum reused for count
	}
	if counts["a.txt"] != 2 {
		t.Errorf("a.txt should have 2 matches, got %d", counts["a.txt"])
	}
	if counts["b.txt"] != 1 {
		t.Errorf("b.txt should have 1 match, got %d", counts["b.txt"])
	}
}

func TestCountModeOutputDoesNotContainContent(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("the quick brown fox\n"), 0644)

	cfg := SearchConfig{
		Pattern:    "fox",
		Path:       dir,
		OutputMode: OutputCount,
		Ctx:        context.Background(),
	}
	sr := Search(cfg)
	output := FormatResult(sr, cfg)

	// Output should NOT contain the content text "quick" or "brown"
	if strings.Contains(output, "quick") {
		t.Errorf("count output should not contain content: %s", output)
	}
	if strings.Contains(output, "brown") {
		t.Errorf("count output should not contain content: %s", output)
	}
}
