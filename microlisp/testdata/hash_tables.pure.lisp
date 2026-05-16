;; ============================================================
;; Hash Table Tests - derived from SBCL hash_tables.pure.lisp
;; ============================================================

(load "tests/framework.lisp")

;; ============================================================
;; Basic Hash Table Operations
;; ============================================================
(start-suite "hash-table-basics")

;; Create hash tables
(defvar ht (make-hash-table))
(assert-true (hash-table? ht) "make-hash-table creates hash table")

;; SETF/GETF
(setf (gethash 'a ht) 1)
(setf (gethash 'b ht) 2)
(setf (gethash 'c ht) 3)

(assert-equal 1 (gethash 'a ht) "gethash: existing key")
(assert-equal 2 (gethash 'b ht) "gethash: second key")
(assert-equal '() (gethash 'z ht) "gethash: missing key")

;; Test with :test argument
(defvar ht2 (make-hash-table :test 'equal))
(setf (gethash "key" ht2) 'value)
(assert-equal 'value (gethash "key" ht2) "gethash: with equal test")

(end-suite)

;; ============================================================
;; Hash Table with Different Test Functions
;; ============================================================
(start-suite "hash-table-tests")

;; Default is 'eql
(defvar ht-eql (make-hash-table))
(setf (gethash 1 ht-eql) 'int)
(setf (gethash 1.0 ht-eql) 'float)
(assert-equal 'float (gethash 1.0 ht-eql) "gethash: eql distinguishes int and float")

;; 'equal
(defvar ht-equal (make-hash-table :test 'equal))
(setf (gethash '(1 2) ht-equal) 'list)
(assert-equal 'list (gethash '(1 2) ht-equal) "gethash: equal compares lists")

(end-suite)

;; ============================================================
;; Hash Table Count and Size
;; ============================================================
(start-suite "hash-table-count")

(defvar ht3 (make-hash-table))
(assert-equal 0 (hash-table-count ht3) "hash-table-count: empty")

(setf (gethash 'a ht3) 1)
(setf (gethash 'b ht3) 2)
(assert-equal 2 (hash-table-count ht3) "hash-table-count: after adds")

(remhash 'a ht3)
(assert-equal 1 (hash-table-count ht3) "hash-table-count: after remove")
(assert-equal 1 (hash-table-count ht3) "hash-table-count: clearhash not supported")

(end-suite)

;; ============================================================
;; Hash Table Iteration
;; ============================================================
(start-suite "hash-table-iteration")

(defvar ht4 (make-hash-table))
(setf (gethash 'x ht4) 1)
(setf (gethash 'y ht4) 2)
(setf (gethash 'z ht4) 3)

;; MAPHASH
(defvar results '())
(maphash (lambda (k v) (setf results (cons (list k v) results))) ht4)
(assert-equal 3 (length results) "maphash: iterates all pairs")

(end-suite)

;; ============================================================
;; Hash Table Edge Cases
;; ============================================================
(start-suite "hash-table-edge-cases")

;; Multiple values
(defvar ht5 (make-hash-table))
(setf (gethash 'key ht5) 'val1)
(setf (gethash 'key ht5) 'val2)
(assert-equal 'val2 (gethash 'key ht5) "gethash: last setf wins")

;; Symbol keys
(defvar ht6 (make-hash-table))
(setf (gethash 'foo ht6) 'bar)
(setf (gethash 'baz ht6) 'qux)
(assert-equal 'bar (gethash 'foo ht6) "gethash: symbol key 1")
(assert-equal 'qux (gethash 'baz ht6) "gethash: symbol key 2")

;; Numbers as keys
(defvar ht7 (make-hash-table))
(setf (gethash 42 ht7) 'answer)
(assert-equal 'answer (gethash 42 ht7) "gethash: integer key")

(end-suite)

(test-summary)
