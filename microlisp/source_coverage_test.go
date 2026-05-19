package microlisp

import (
	"fmt"
	"strings"
	"testing"
)

func TestAuditSourceCoverage(t *testing.T) {
	// Check every entry in sourceIndex - can GetSource actually show code?
	noCode := []string{}
	hasCode := []string{}

	for name, entry := range sourceIndex {
		out := GetSource(name, 0, 0)
		// Check if output actually contains source lines (numbered lines like " 123  ")
		hasLines := strings.Contains(out, "  ") && !strings.Contains(out, "not available") && !strings.Contains(out, "No source found")

		switch entry.Kind {
		case "builtin", "helper":
			if entry.Start == 0 || entry.End == 0 {
				noCode = append(noCode, fmt.Sprintf("%-25s kind=%-8s file=%-20s Start=0 End=0", name, entry.Kind, entry.File))
			} else if !hasLines {
				noCode = append(noCode, fmt.Sprintf("%-25s kind=%-8s file=%-20s lines=%d-%d (no source output)", name, entry.Kind, entry.File, entry.Start, entry.End))
			} else {
				hasCode = append(hasCode, name)
			}
		case "special":
			if entry.Start == 0 || entry.End == 0 {
				noCode = append(noCode, fmt.Sprintf("%-25s kind=%-8s (no line range)", name, entry.Kind))
			} else if !hasLines {
				noCode = append(noCode, fmt.Sprintf("%-25s kind=%-8s lines=%d-%d (no source output)", name, entry.Kind, entry.Start, entry.End))
			} else {
				hasCode = append(hasCode, name)
			}
		case "stdlib":
			if strings.Contains(out, "not found in stdlib") {
				noCode = append(noCode, fmt.Sprintf("%-25s kind=%-8s (snippet not found)", name, entry.Kind))
			} else {
				hasCode = append(hasCode, name)
			}
		}
	}

	t.Logf("=== COVERAGE SUMMARY ===")
	t.Logf("Total entries: %d", len(sourceIndex))
	t.Logf("Has source code: %d", len(hasCode))
	t.Logf("No source code: %d", len(noCode))

	if len(noCode) > 0 {
		t.Logf("\n=== MISSING SOURCE (%d entries) ===", len(noCode))
		for _, s := range noCode {
			t.Log(s)
		}
	}
}
