package microlisp

import (
	"fmt"
	"testing"
)

func TestDebugScan2(t *testing.T) {
	entry := sourceIndex["nth-value"]
	fmt.Printf("sourceIndex[nth-value]: Kind=%s File=%s Start=%d End=%d GoFunc=%s\n",
		entry.Kind, entry.File, entry.Start, entry.End, entry.GoFunc)
	
	// Check all entries with "nth" in the name
	for k, v := range sourceIndex {
		if v.GoFunc == "builtinNthValue" {
			fmt.Printf("Entry with GoFunc=builtinNthValue: key=%s Kind=%s File=%s Start=%d End=%d\n",
				k, v.Kind, v.File, v.Start, v.End)
		}
	}
	
	// Also check what seq_map.go's line 1266 would be
	// Check if another file is being scanned as seq_map.go
}
