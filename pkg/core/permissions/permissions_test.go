package permissions

import "testing"

func TestMatchPattern_Exact(t *testing.T) {
	if !matchPattern("main.go", "main.go") {
		t.Error("should match exact")
	}
}

func TestMatchPattern_Star(t *testing.T) {
	if !matchPattern("*", "anything.go") {
		t.Error("* should match everything")
	}
}

func TestMatchPattern_Extension(t *testing.T) {
	if !matchPattern("*.go", "main.go") {
		t.Error("should match *.go")
	}
	if matchPattern("*.go", "main.rs") {
		t.Error("should not match .rs")
	}
}

func TestMatchPattern_PathGlob(t *testing.T) {
	if !matchPattern("src/*", "src/main.go") {
		t.Error("should match src/*")
	}
	if matchPattern("src/*", "lib/main.go") {
		t.Error("should not match lib path")
	}
}

func TestMatchPattern_QuestionMark(t *testing.T) {
	if !matchPattern("file?.txt", "file1.txt") {
		t.Error("? should match single char")
	}
	if matchPattern("file?.txt", "file12.txt") {
		t.Error("? should not match two chars")
	}
}

func TestMatchPattern_ChacterClass(t *testing.T) {
	if !matchPattern("[abc].txt", "a.txt") {
		t.Error("[abc] should match a")
	}
	if matchPattern("[abc].txt", "d.txt") {
		t.Error("[abc] should not match d")
	}
}

func TestMatchPattern_InvalidPattern(t *testing.T) {
	// Invalid glob pattern (unclosed [) — should fall back to exact match
	if !matchPattern("[invalid", "[invalid") {
		t.Error("invalid pattern should fall back to exact match")
	}
}

func TestMatchPattern_NoMatch(t *testing.T) {
	if matchPattern("*.rs", "main.go") {
		t.Error("should not match")
	}
}
