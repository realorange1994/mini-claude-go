package microlisp

import (
	"fmt"
	"testing"
)

func TestDebugScan13(t *testing.T) {
	builtinMap := loadBuiltinRegistry()
	
	// Check if nth-value is in the map
	for goFunc, lispName := range builtinMap {
		if lispName == "nth-value" {
			fmt.Printf("builtinMap has: goFunc=%s -> lispName=%s\n", goFunc, lispName)
		}
		if goFunc == "builtinNthValue" {
			fmt.Printf("builtinMap has: goFunc=%s -> lispName=%s\n", goFunc, lispName)
		}
	}
	
	// Check the specialOpNames list
	for _, name := range specialOpNames {
		if name == "nth-value" {
			fmt.Printf("specialOpNames has: %s\n", name)
		}
	}
}
