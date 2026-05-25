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
	fmt.Printf("Path: %s\n", fwdPath)

	// Test lisp-write-file
	expr := fmt.Sprintf(`(lisp-write-file %s %s)`, lispStr(fwdPath), lispStr("hello world"))
	fmt.Printf("expr = %s\n", expr)
	r, e := microlisp.SafeEvalString(expr)
	fmt.Printf("result = %s, err = %v\n", r, e)

	// Check if file exists
	if _, err := os.Stat(testFile); err != nil {
		fmt.Printf("File stat error: %v\n", err)
	} else {
		fmt.Println("File exists!")
		data, _ := os.ReadFile(testFile)
		fmt.Printf("Content: %s\n", string(data))
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
;; String join helper
(define (string-join lst sep)
  (if (null lst) ""
      (if (null (cdr lst)) (car lst)
          (string-append (car lst) sep (string-join (cdr lst) sep)))))

;; Split string by delimiter
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

;; Count substring occurrences
(define (count-substring str sub)
  (let ((slen (string-length sub)))
    (if (= slen 0) 0
        (count-substring-loop str sub slen 0))))

(define (count-substring-loop s sub slen count)
  (let ((pos (string-find s sub)))
    (if (not pos) count
        (count-substring-loop (substring s (+ pos slen)) sub slen (+ count 1)))))

;; Replace first N occurrences
(define (replace-first-n str old new n)
  (let ((olen (string-length old)))
    (if (= olen 0) str
        (replace-first-n-loop str old new n olen "" 0))))

(define (replace-first-n-loop s old new n olen result count)
  (if (and (> n 0) (>= count n))
      (string-append result s)
      (let ((pos (string-find s old)))
        (if (not pos)
            (string-append result s)
            (replace-first-n-loop (substring s (+ pos olen)) old new n olen
                                  (string-append result (substring s 0 pos) new)
                                  (+ count 1))))))

;; Read entire file as string
(define (read-file-fully path)
  (let ((stream (open path :direction :input)))
    (read-file-fully-loop stream '())))

(define (read-file-fully-loop stream lines)
  (let ((line (read-line stream nil)))
    (if (not line)
        (progn (close stream) (string-join (reverse lines) "\n"))
        (read-file-fully-loop stream (cons line lines)))))

;; Read file lines as list
(define (read-file-lines path)
  (let ((stream (open path :direction :input)))
    (read-file-lines-loop stream '())))

(define (read-file-lines-loop stream lines)
  (let ((line (read-line stream nil)))
    (if (not line)
        (progn (close stream) (reverse lines))
        (read-file-lines-loop stream (cons line lines)))))

;; Format lines with line numbers
(define (format-lines lines start)
  (format-lines-loop lines start '()))

(define (format-lines-loop lst num acc)
  (if (null lst)
      (string-join (reverse acc) "\n")
      (format-lines-loop (cdr lst) (+ num 1)
                         (cons (format nil "~A\t~A" num (car lst)) acc))))

;; Path safety check
(define (safe-path? path)
  (let ((lp (string-downcase path)))
    (if (string-find lp "/dev/") (error "unsafe path: device file")
        (if (string-find lp "\\dev\\") (error "unsafe path: device file")
            (if (string-find lp "/proc/") (error "unsafe path: proc filesystem")
                (if (string-find lp "\\proc\\") (error "unsafe path: proc filesystem")
                    path))))))

;; Write content to file
(define (lisp-write-file path content)
  (handler-case
    (progn
      (ensure-directories-exist (safe-path? path))
      (let ((stream (open path :direction :output :if-exists :supersede)))
        (write-string content stream)
        (close stream))
      "ok")
    (condition (c) (format nil "Error: ~A" c))))
`
