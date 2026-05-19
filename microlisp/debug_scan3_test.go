package microlisp

import (
	"fmt"
	"testing"
)

func TestDebugScan3(t *testing.T) {
	builtinMap := loadBuiltinRegistry()
	for goFunc, lispName := range builtinMap {
		if lispName == "nth-value" || goFunc == "builtinNthValue" {
			fmt.Printf("builtinMap: %s -> %s\n", goFunc, lispName)
		}
	}
	
	// Check if nth-value is in specialOpNames
	for _, name := range specialOpNames {
		if name == "NTH-VALUE" || name == "NTH_VALUE" {
			fmt.Printf("Found in specialOpNames: %s\n", name)
		}
	}
}
