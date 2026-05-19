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

func TestDebugScan4(t *testing.T) {
	dir := findMicrolispDir()
	goFiles, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	
	builtinMap := loadBuiltinRegistry()
	funcRe := regexp.MustCompile(`^func\s+(builtin[A-Za-z0-9_]+)\s*\(`)
	
	for _, fpath := range goFiles {
		base := filepath.Base(fpath)
		if strings.HasSuffix(base, "_test.go") {
			continue
		}
		
		var lines []string
		f, err := os.Open(fpath)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}
		f.Close()
		
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			if matches := funcRe.FindStringSubmatch(trimmed); len(matches) > 1 {
				goFunc := matches[1]
				if goFunc == "builtinNthValue" {
					lispName, known := builtinMap[goFunc]
					start := i + 1
					end := findFuncEndFromLines(lines, i)
					fmt.Printf("Found builtinNthValue in %s at i=%d (line %d), end=%d, known=%v, lisp=%s, total_lines=%d\n",
						base, i, start, end, known, lispName, len(lines))
				}
			}
		}
	}
}
