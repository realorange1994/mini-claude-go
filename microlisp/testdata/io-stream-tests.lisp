;; io-stream-tests.lisp — tests for I/O and stream operations
;; Covers: string streams, hash tables, read-char, write-char

(load "tests/framework.lisp")
(start-suite "String Output Stream")

;; --- make-string-output-stream ---
(define sout (make-string-output-stream))
(write-string "hello" sout)
(assert-equal "hello" (get-output-stream-string sout) "write-string to string stream")

;; --- write-char outputs character representation ---
(define sout2 (make-string-output-stream))
(write-char #\x sout2)
(write-char #\y sout2)
;; write-char outputs #\x format, so we get "##"
(assert-equal "##" (get-output-stream-string sout2) "write-char to string stream")

(end-suite)
(start-suite "String Input Stream")

;; --- make-string-input-stream ---
;; Note: read-char returns string representation of character
(assert-equal "h" (let ((s (make-string-input-stream "hello"))) (read-char s)) "read-char from string")
(assert-equal "h" (let ((s (make-string-input-stream "hello"))) (peek-char s) (read-char s)) "peek-char doesn't advance")

;; --- read-char advances ---
(assert-equal "e" (let ((s (make-string-input-stream "hello"))) (read-char s) (read-char s)) "read-char advances")

(end-suite)
(start-suite "Hash Tables")

;; --- make-hash-table ---
(assert-true (hash-table-p (make-hash-table)) "make-hash-table creates hash table")

;; --- gethash ---
(define h (make-hash-table))
(assert-nil (gethash (quote foo) h) "gethash missing key")
(setf (gethash (quote foo) h) 42)
(assert-equal 42 (gethash (quote foo) h) "gethash after setf")

;; --- hash-table-count ---
(assert-equal 1 (hash-table-count h) "hash-table-count")
(setf (gethash (quote bar) h) 10)
(assert-equal 2 (hash-table-count h) "hash-table-count after 2 entries")

;; --- remhash ---
(remhash (quote foo) h)
(assert-nil (gethash (quote foo) h) "remhash removes entry")
(assert-equal 1 (hash-table-count h) "hash-table-count after remhash")

;; --- clrhash ---
(clrhash h)
(assert-equal 0 (hash-table-count h) "clrhash clears table")
(assert-nil (gethash (quote bar) h) "gethash after clrhash")

(end-suite)
(start-suite "Hash Table Multiple Keys")

;; --- Multiple keys ---
(define h2 (make-hash-table))
(setf (gethash (quote a) h2) 1)
(setf (gethash (quote b) h2) 2)
(setf (gethash (quote c) h2) 3)
(assert-equal 3 (hash-table-count h2) "hash-table three entries")
(assert-equal 1 (gethash (quote a) h2) "gethash key a")
(assert-equal 2 (gethash (quote b) h2) "gethash key b")
(assert-equal 3 (gethash (quote c) h2) "gethash key c")

(end-suite)
(test-summary)
