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

func TestDebugScan12(t *testing.T) {
	dir := findMicrolispDir()
	goFiles, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	
	builtinMap := loadBuiltinRegistry()
	funcRe := regexp.MustCompile(`^func\s+(builtin[A-Za-z0-9_]+)\s*\(`)
	
	// Collect ALL entries like scanBuiltinFunctions does
	testIndex := make(map[string]*SourceEntry)
	
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
				lispName, known := builtinMap[goFunc]
				if !known {
					continue
				}
				start := i + 1
				end := findFuncEndFromLines(lines, i)
				
				testIndex[strings.ToLower(lispName)] = &SourceEntry{
					Kind:   "builtin",
					File:   base,
					Start:  start,
					End:    end,
					GoFunc: goFunc,
				}
				
				if strings.Contains(lispName, "nth") {
					fmt.Printf("Setting %s: File=%s Start=%d End=%d (file has %d lines)\n",
						lispName, base, start, end, len(lines))
				}
			}
		}
	}
	
	entry := testIndex["nth-value"]
	fmt.Printf("Final from testIndex: nth-value Start=%d End=%d\n", entry.Start, entry.End)
}
