package microlisp

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestDebugScan10(t *testing.T) {
	dir := findMicrolispDir()
	fpath := filepath.Join(dir, "seq_map.go")
	f, err := os.Open(fpath)
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	f.Close()
	
	funcRe := regexp.MustCompile(`^func\s+(builtin[A-Za-z0-9_]+)\s*\(`)
	builtinMap := loadBuiltinRegistry()
	
	// Simulate scanBuiltinFunctions
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if matches := funcRe.FindStringSubmatch(trimmed); len(matches) > 1 {
			goFunc := matches[1]
			lispName, known := builtinMap[goFunc]
			if !known {
				continue
			}
			start := i + 1
			end := findFuncEndFromLines(lines, i)
			fmt.Printf("%s (lisp: %s) at i=%d start=%d end=%d\n", goFunc, lispName, i, start, end)
		}
	}
}
