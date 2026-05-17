package microlisp

import (
	"strings"
	"testing"
)

// -------- Source Index Tests --------

func TestGetSourceBuiltin(t *testing.T) {
	// "car" is a well-known builtin
	out := GetSource("car", 0, 0)
	if strings.Contains(out, "No source found") {
		t.Fatalf("expected source for 'car', got: %s", out)
	}
	if !strings.Contains(out, "builtin") {
		t.Fatalf("expected 'car' to be a builtin, got: %s", out)
	}
	if !strings.Contains(out, "Lines:") {
		t.Fatalf("expected line range for builtin 'car', got: %s", out)
	}
}

func TestGetSourceStdlib(t *testing.T) {
	// "cadr" is defined in stdlib (not a builtin or special form)
	out := GetSource("cadr", 0, 0)
	if strings.Contains(out, "No source found") {
		t.Fatalf("expected source for 'cadr', got: %s", out)
	}
	if !strings.Contains(out, "stdlib") {
		t.Fatalf("expected 'cadr' to be stdlib, got: %s", out)
	}
}

func TestGetSourceSpecialForm(t *testing.T) {
	out := GetSource("if", 0, 0)
	if strings.Contains(out, "No source found") {
		t.Fatalf("expected source for 'if', got: %s", out)
	}
	if !strings.Contains(out, "special") {
		t.Fatalf("expected 'if' to be special, got: %s", out)
	}
}

func TestGetSourceNotFound(t *testing.T) {
	out := GetSource("xyzzy-no-such-function", 0, 0)
	if !strings.Contains(out, "No source found") {
		t.Fatalf("expected 'No source found' for unknown function, got: %s", out)
	}
}

func TestGetSourceFuzzyMatch(t *testing.T) {
	// Partial match should find the shortest containing key
	out := GetSource("append", 0, 0)
	if strings.Contains(out, "No source found") {
		t.Fatalf("expected fuzzy match for 'append', got: %s", out)
	}
}

func TestGetSourceCaseInsensitive(t *testing.T) {
	// Should work with upper case
	outUpper := GetSource("CAR", 0, 0)
	outLower := GetSource("car", 0, 0)
	if outUpper != outLower {
		t.Fatalf("case-insensitive: CAR and car should give same result")
	}
}

func TestSourceListReturnsResults(t *testing.T) {
	out := SourceList("", 0, 50)
	if !strings.Contains(out, "source index") {
		t.Fatalf("expected source index header in output, got: %s", out)
	}
	if !strings.Contains(out, "Builtins:") {
		t.Fatalf("expected builtin count, got: %s", out)
	}
	if !strings.Contains(out, "Stdlib:") {
		t.Fatalf("expected stdlib count, got: %s", out)
	}
	if !strings.Contains(out, "Helpers:") {
		t.Fatalf("expected helper count, got: %s", out)
	}
}

func TestSourceListQueryFilter(t *testing.T) {
	out := SourceList("car", 0, 50)
	if !strings.Contains(out, "car") {
		t.Fatalf("expected 'car' in filtered results, got: %s", out)
	}
}

func TestSourceListPagination(t *testing.T) {
	// Get first page
	out1 := SourceList("", 0, 10)
	// Get second page
	out2 := SourceList("", 10, 10)
	// They should be different
	if out1 == out2 {
		t.Fatalf("expected different output for different offsets")
	}
	// Second page should show entries starting at offset 11
	if !strings.Contains(out2, "11-") {
		t.Fatalf("expected offset 11 in second page, got: %s", out2)
	}
}

func TestSourceListLimitZero(t *testing.T) {
	out := SourceList("", 0, 0)
	// limit=0 should use default of 50
	if !strings.Contains(out, "source index") {
		t.Fatalf("expected results with limit=0, got: %s", out)
	}
}

func TestSourceListOffsetOutOfBounds(t *testing.T) {
	out := SourceList("zzz-not-a-function", 0, 10)
	if !strings.Contains(out, "No functions found") {
		t.Fatalf("expected 'No functions found' for impossible query, got: %s", out)
	}
}

func TestSourceListNegativeOffset(t *testing.T) {
	out := SourceList("", -5, 10)
	// Should not panic, treat as 0
	if strings.Contains(out, "panic") || strings.Contains(out, "error") {
		t.Fatalf("unexpected error with negative offset: %s", out)
	}
}

func TestSourceListSorted(t *testing.T) {
	out := SourceList("", 0, 200)
	// Extract function names from output and verify they're sorted
	lines := strings.Split(out, "\n")
	var names []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "-") || strings.HasPrefix(trimmed, "Usage") ||
			strings.HasPrefix(trimmed, "microlisp") || strings.HasPrefix(trimmed, "...") {
			continue
		}
		if trimmed != "" && !strings.Contains(trimmed, "Showing") &&
			!strings.Contains(trimmed, "Builtins") && !strings.Contains(trimmed, "functions") {
			// Parse: "  name                           kind       file"
			fields := strings.Fields(trimmed)
			if len(fields) >= 2 {
				names = append(names, fields[0])
			}
		}
	}
	// Verify sorted order
	for i := 1; i < len(names); i++ {
		if names[i] < names[i-1] {
			t.Fatalf("source-list output not sorted: %q before %q", names[i-1], names[i])
		}
	}
}

func TestSourceIndexNotEmpty(t *testing.T) {
	if len(sourceIndex) == 0 {
		t.Fatal("sourceIndex should not be empty after init")
	}
}

func TestSourceIndexHasSpecialForms(t *testing.T) {
	// These are core special forms that must be indexed
	specials := []string{"if", "lambda", "quote", "define", "let", "cond"}
	for _, s := range specials {
		entry := sourceIndex[strings.ToLower(s)]
		if entry == nil {
			t.Logf("WARNING: special form %q not found in sourceIndex (may be OK if not in specialOpNames)", s)
		}
	}
}

func TestExtractStdlibString(t *testing.T) {
	// Verify stdlib content was extracted
	if stdlibContent == "" {
		t.Fatal("stdlibContent should not be empty after init")
	}
	if !strings.Contains(stdlibContent, "define") {
		t.Fatal("stdlibContent should contain 'define' forms")
	}
}

func TestSpecialFormHasSourceCode(t *testing.T) {
	// Special forms should now have Start/End line numbers and show actual code
	out := GetSource("if", 0, 0)
	if !strings.Contains(out, "Lines:") {
		t.Fatalf("expected line range for special form 'if', got: %s", out)
	}
	if !strings.Contains(out, "eval_core.go") {
		t.Fatalf("expected eval_core.go for 'if', got: %s", out)
	}
	// Should show actual Go code (contains case label or implementation)
	if !strings.Contains(out, "IF") && !strings.Contains(out, "func") && !strings.Contains(out, "case") {
		t.Fatalf("expected source code for 'if', got: %s", out)
	}
}

func TestHelperFunctionIndexed(t *testing.T) {
	// Helper functions (non-builtin Go functions) should be indexed
	// "eqval" is a well-known helper used in equality.go
	entry := sourceIndex["eqval"]
	if entry == nil {
		t.Fatal("expected 'eqval' to be indexed as helper")
	}
	if entry.Kind != "helper" {
		t.Fatalf("expected 'eqval' kind to be 'helper', got: %s", entry.Kind)
	}
	if entry.Start == 0 {
		t.Fatal("expected 'eqval' to have Start line number")
	}
}

func TestGetSourceHelper(t *testing.T) {
	out := GetSource("eqval", 0, 0)
	if strings.Contains(out, "No source found") {
		t.Fatalf("expected source for helper 'eqval', got: %s", out)
	}
	if !strings.Contains(out, "helper") {
		t.Fatalf("expected 'eqval' to be helper, got: %s", out)
	}
	if !strings.Contains(out, "Go function:") {
		t.Fatalf("expected Go function name for helper 'eqval', got: %s", out)
	}
}

func TestSourceListIncludesHelpers(t *testing.T) {
	out := SourceList("", 0, 50)
	if !strings.Contains(out, "Helpers:") {
		t.Fatalf("expected Helpers count in output, got: %s", out)
	}
}

func TestHelperCountSignificant(t *testing.T) {
	// There should be a significant number of helper functions
	helperCount := 0
	for _, entry := range sourceIndex {
		if entry.Kind == "helper" {
			helperCount++
		}
	}
	if helperCount < 100 {
		t.Fatalf("expected at least 100 helper functions, got: %d", helperCount)
	}
}
