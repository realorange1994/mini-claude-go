package microlisp

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDebugScan6(t *testing.T) {
	dir := findMicrolispDir()
	goFiles, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	
	for _, f := range goFiles {
		base := filepath.Base(f)
		if strings.HasSuffix(base, "_test.go") {
			continue
		}
		var lines []string
		fh, err := os.Open(f)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(fh)
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}
		fh.Close()
		
		for i, line := range lines {
			if strings.Contains(strings.TrimSpace(line), "builtinNthValue") {
				fmt.Printf("Found builtinNthValue in %s at line %d (file has %d lines)\n", base, i+1, len(lines))
			}
		}
	}
}
