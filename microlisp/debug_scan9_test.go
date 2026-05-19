package microlisp

import (
	"fmt"
	"testing"
)

func TestDebugScan9(t *testing.T) {
	// Print ALL entries that have seq_map.go as File
	for name, entry := range sourceIndex {
		if entry.File == "seq_map.go" {
			fmt.Printf("  %s: Kind=%s Start=%d End=%d GoFunc=%s\n", name, entry.Kind, entry.Start, entry.End, entry.GoFunc)
		}
	}
}
