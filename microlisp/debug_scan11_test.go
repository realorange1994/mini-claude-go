package microlisp

import (
	"fmt"
	"testing"
)

func TestDebugScan11(t *testing.T) {
	// Check the nth-value entry before and after scanBuiltinFunctions
	// by checking if it's a special form (from specialOpNames) that was overwritten
	entry := sourceIndex["nth-value"]
	fmt.Printf("nth-value: Kind=%s File=%s Start=%d End=%d\n",
		entry.Kind, entry.File, entry.Start, entry.End)
	
	// Check if "nth-value" is in specialOpNames
	for _, name := range specialOpNames {
		if name == "NTH-VALUE" || name == "nth-value" {
			fmt.Printf("Found in specialOpNames: %q\n", name)
		}
	}
}
