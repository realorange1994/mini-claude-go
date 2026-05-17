package rgrep

import "context"

// OutputMode controls what the search returns.
type OutputMode string

const (
	OutputContent        OutputMode = "content"
	OutputFilesWithMatch OutputMode = "files_with_matches"
	OutputCount          OutputMode = "count"
)

// SearchConfig holds all search parameters.
type SearchConfig struct {
	Pattern        string
	Path           string     // file or directory to search in (default ".")
	Glob           string     // glob filter, e.g. "*.py"
	TypeFilter     string     // language type, e.g. "go", "py"
	CaseInsensitive bool
	FixedStrings   bool
	OutputMode     OutputMode // default: files_with_matches
	ShowLineNums   bool       // show line numbers in content mode (default true)
	Multiline      bool       // multiline regex mode
	ContextBefore  int        // lines before match
	ContextAfter   int        // lines after match
	HeadLimit      int        // max results (0 = unlimited)
	Offset         int        // skip first N results
	MaxDepth       int        // max directory depth (0 = unlimited)
	MaxFilesize    int64      // max file size in bytes (0 = unlimited)
	Ctx            context.Context
	Excludes       []string   // exclude patterns (glob, supports **)
}

// Result is a single match from the search.
type Result struct {
	Path    string // relative path
	LineNum int    // 1-based line number (0 for files/count mode)
	Line    string // the matching line content
}

// SearchResult holds the full result of a search.
type SearchResult struct {
	Results        []Result
	FilesSearched  int
	TotalMatches   int
	Truncated      bool // true if results were truncated by head_limit
	Err            error
}
