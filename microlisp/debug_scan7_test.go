package microlisp

import (
	"fmt"
	"testing"
)

func TestDebugScan7(t *testing.T) {
	// Just print the current sourceIndex entry for nth-value
	entry, ok := sourceIndex["nth-value"]
	if !ok {
		fmt.Println("nth-value NOT in sourceIndex")
		return
	}
	fmt.Printf("nth-value: Kind=%s File=%s Start=%d End=%d GoFunc=%s\n",
		entry.Kind, entry.File, entry.Start, entry.End, entry.GoFunc)
	
	// Also check what the actual source lookup returns
	out := GetSource("nth-value", 0, 0)
	fmt.Println(out[:200])
}
