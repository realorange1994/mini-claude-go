package microlisp

import (
	"fmt"
	"path/filepath"
	"testing"
)

func TestDebugScan8(t *testing.T) {
	// Check what findMicrolispDir returns during init
	dir := findMicrolispDir()
	fmt.Printf("findMicrolispDir: %s\n", dir)
	
	// Check glob results
	goFiles, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	
	// Find seq_map.go in the list
	for i, f := range goFiles {
		if filepath.Base(f) == "seq_map.go" {
			fmt.Printf("seq_map.go at position %d in glob list\n", i)
		}
	}
	fmt.Printf("Total .go files: %d\n", len(goFiles))
	
	// Check if there's a different seq_map.go somewhere
	for _, f := range goFiles {
		if filepath.Base(f) == "seq_map.go" {
			fmt.Printf("seq_map.go path: %s\n", f)
		}
	}
}
