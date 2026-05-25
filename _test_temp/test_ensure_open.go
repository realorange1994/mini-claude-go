package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"miniclaudecode-go/microlisp"
)

func main() {
	microlisp.ResetGlobalEnv()
	dir, _ := os.MkdirTemp("", "test_lisp_write_*")
	defer os.RemoveAll(dir)

	// Load the lispToolsLib
	if _, err := microlisp.SafeEvalString(lispToolsLib); err != nil {
		fmt.Printf("Load lib error: %v\n", err)
		return
	}
	fmt.Println("Lib loaded OK")

	testFile := filepath.Join(dir, "test_write.txt")
	fwdPath := filepath.ToSlash(testFile)
	fmt.Printf("Temp dir: %s\n", dir)
	fmt.Printf("Full path: %s\n", fwdPath)

	// Test ensure-directories-exist
	ensureExpr := fmt.Sprintf(`(ensure-directories-exist %s)`, lispStr(fwdPath))
	fmt.Printf("ensure expr = %s\n", ensureExpr)
	r1, e1 := microlisp.SafeEvalString(ensureExpr)
	fmt.Printf("ensure result = %s, err = %v\n", r1, e1)

	// Check if dir was created
	subdir := filepath.Dir(testFile)
	info, err := os.Stat(subdir)
	if err != nil {
		fmt.Printf("Subdir stat error: %v\n", err)
	} else {
		fmt.Printf("Subdir exists: %v, isDir: %v\n", info != nil, info.IsDir())
	}

	// Now try to open the file using just the open builtin
	openExpr := fmt.Sprintf(`(open %s :direction :output :if-exists :supersede)`, lispStr(fwdPath))
	fmt.Printf("open expr = %s\n", openExpr)
	r2, e2 := microlisp.SafeEvalString(openExpr)
	fmt.Printf("open result = %s, err = %v\n", r2, e2)

	// Check if file was created
	if _, err := os.Stat(testFile); err != nil {
		fmt.Printf("File stat error: %v\n", err)
	} else {
		fmt.Println("File exists!")
	}
}

func lispStr(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\t", "\\t")
	return "\"" + s + "\""
}

const lispToolsLib = `
(define (string-join lst sep)
  (if (null lst) ""
      (if (null (cdr lst)) (car lst)
          (string-append (car lst) sep (string-join (cdr lst) sep)))))

(define (split-string str delim)
  (let ((dlen (string-length delim)))
    (if (= dlen 0) (list str)
        (split-string-loop str delim dlen '()))))

(define (split-string-loop s delim dlen acc)
  (let ((pos (string-find s delim)))
    (if (not pos)
        (reverse (cons s acc))
        (split-string-loop (substring s (+ pos dlen)) delim dlen
                           (cons (substring s 0 pos) acc)))))
`
