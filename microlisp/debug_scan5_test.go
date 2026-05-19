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

func TestDebugScan5(t *testing.T) {
	dir := findMicrolispDir()
	goFiles, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	
	builtinMap := loadBuiltinRegistry()
	funcRe := regexp.MustCompile(`^func\s+(builtin[A-Za-z0-9_]+)\s*\(`)
	
	// Simulate exactly what scanBuiltinFunctions does
	testIndex := make(map[string]*SourceEntry)
	
	// First register special forms
	for _, name := range specialOpNames {
		testIndex[strings.ToLower(name)] = &SourceEntry{Kind: "special"}
	}
	
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
					fmt.Printf("Setting %s: File=%s Start=%d End=%d (from %s i=%d total=%d)\n",
						lispName, base, start, end, fpath, i, len(lines))
				}
			}
		}
	}
	
	entry := testIndex["nth-value"]
	fmt.Printf("Final nth-value entry: Kind=%s File=%s Start=%d End=%d\n",
		entry.Kind, entry.File, entry.Start, entry.End)
}
