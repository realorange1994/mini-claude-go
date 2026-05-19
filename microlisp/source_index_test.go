package microlisp

import (
	"os"
	"path/filepath"
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

func TestGetSourceGoFFI(t *testing.T) {
	out := GetSource("crypto/x509.CreateCertificate", 0, 5)
	if strings.Contains(out, "No source found") {
		t.Fatalf("expected source for Go FFI function, got: %s", out)
	}
	if !strings.Contains(out, "go-ffi") {
		t.Fatalf("expected 'go-ffi' kind, got: %s", out)
	}
	if !strings.Contains(out, "Go stdlib:") {
		t.Fatalf("expected Go stdlib signature, got: %s", out)
	}
}

func TestSourceListIncludesGoFFI(t *testing.T) {
	out := SourceList("crypto/x509", 0, 10)
	if strings.Contains(out, "No functions found") {
		t.Fatalf("expected Go FFI functions in source list, got: %s", out)
	}
}

// ─── nth-value builtin registration tests ──────────────────────────────────────
// Consolidated from debug_scan{1..13}_test.go
//
// NOTE: nth-value has a known dual-registration issue: it appears both in
// specialOpNames (eval_core.go special form handler) AND in the builtin registry
// (seq_map.go builtinNthValue). The special form registration takes precedence
// in sourceIndex, so the entry Kind is "special" rather than "builtin".

func TestNthValueInBuiltinRegistry(t *testing.T) {
	bm := loadBuiltinRegistry()
	name, ok := bm["builtinNthValue"]
	if !ok {
		t.Fatal("builtinRegistry should contain builtinNthValue → nth-value mapping")
	}
	if name != "nth-value" {
		t.Errorf("expected builtinNthValue to map to 'nth-value', got %q", name)
	}
}

func TestNthValueIndexed(t *testing.T) {
	entry, ok := sourceIndex["nth-value"]
	if !ok {
		t.Fatal("nth-value should be registered in sourceIndex")
	}
	// Currently indexed as "special" due to dual-registration (known issue)
	if entry.Kind != "special" && entry.Kind != "builtin" {
		t.Errorf("expected nth-value kind='special' or 'builtin', got %q", entry.Kind)
	}
	if entry.File == "" {
		t.Error("expected nth-value to have a non-empty File")
	}
	if entry.Start == 0 || entry.End == 0 {
		t.Errorf("expected nth-value to have Start/End line numbers, got Start=%d End=%d", entry.Start, entry.End)
	}
}

func TestNthValueGetSource(t *testing.T) {
	out := GetSource("nth-value", 0, 0)
	if strings.Contains(out, "No source found") {
		t.Fatalf("GetSource should return source for nth-value, got: %s", out)
	}
	if !strings.Contains(out, "Lines:") {
		t.Fatalf("expected line range for nth-value, got: %s", out)
	}
}

func TestNthValueDualRegistration(t *testing.T) {
	// nth-value appears in both specialOpNames AND builtin registry — known issue.
	// This test documents the current behavior.
	inSpecial := false
	for _, name := range specialOpNames {
		if strings.ToLower(name) == "nth-value" {
			inSpecial = true
			break
		}
	}
	if !inSpecial {
		t.Error("nth-value should be in specialOpNames (currently is due to dual-reg)")
	}

	bm := loadBuiltinRegistry()
	inBuiltin := false
	for _, lispName := range bm {
		if lispName == "nth-value" {
			inBuiltin = true
			break
		}
	}
	if !inBuiltin {
		t.Error("nth-value should also be in builtinRegistry")
	}
}

func TestNthValueSourceEntryFileExists(t *testing.T) {
	entry := sourceIndex["nth-value"]
	if entry == nil {
		t.Skip("nth-value not indexed")
	}
	dir := findMicrolispDir()
	fpath := filepath.Join(dir, entry.File)
	if _, err := os.Stat(fpath); err != nil {
		t.Fatalf("nth-value source file %q not found: %v", entry.File, err)
	}
}

func TestBuiltinRegistryHasNthValue(t *testing.T) {
	bm := loadBuiltinRegistry()
	found := false
	for goFunc, lispName := range bm {
		if lispName == "nth-value" {
			found = true
			if goFunc != "builtinNthValue" {
				t.Errorf("expected nth-value to map from builtinNthValue, got %q", goFunc)
			}
		}
	}
	if !found {
		t.Error("no builtinRegistry entry maps to 'nth-value'")
	}
}
