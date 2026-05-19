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

func TestDebugScanBuiltin(t *testing.T) {
	dir := findMicrolispDir()
	t.Logf("microlisp dir: %s", dir)

	builtinMap := loadBuiltinRegistry()
	t.Logf("builtinMap has %d entries", len(builtinMap))

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
	t.Logf("seq_map.go has %d lines", len(lines))

	funcRe := regexp.MustCompile(`^func\s+(builtin[A-Za-z0-9_]+)\s*\(`)

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if matches := funcRe.FindStringSubmatch(trimmed); len(matches) > 1 {
			goFunc := matches[1]
			lispName, known := builtinMap[goFunc]
			if goFunc == "builtinNthValue" {
				start := i + 1
				end := findFuncEndFromLines(lines, i)
				fmt.Printf("builtinNthValue at i=%d (line %d), end=%d, lisp=%s, known=%v\n", i, start, end, lispName, known)
			}
		}
	}
}
