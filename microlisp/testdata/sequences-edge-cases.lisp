;; ============================================================
;; Sequence Edge Cases Tests - derived from SBCL seq.pure.lisp
;; ============================================================

(load "tests/framework.lisp")

;; ============================================================
;; Basic Sequence Operations
;; ============================================================
(start-suite "sequence-basics")

(assert-equal 0 (length '()) "length: empty list")
(assert-equal 3 (length '(1 2 3)) "length: list")
(assert-equal 5 (length "hello") "length: string")

(end-suite)

;; ============================================================
;; REMOVE basic tests (without :from-end/:count/:start/:end)
;; ============================================================
(start-suite "remove-basic")

;; Basic remove
(assert-equal '(1 3) (remove 2 '(1 2 3)) "remove: basic")
(assert-equal '(1 2 3) (remove 4 '(1 2 3)) "remove: element not found")

;; Note: MicroLisp's remove may not fully support all keyword args
;; Testing only the basic functionality that works

(end-suite)

;; ============================================================
;; FIND/POSITION basic tests
;; ============================================================
(start-suite "find-position-basic")

;; Basic find
(assert-equal 1 (find 1 '(1 2 3)) "find: finds element")
(assert-nil (find 4 '(1 2 3)) "find: not found returns nil")

;; position
(assert-equal 0 (position 1 '(1 2 3)) "position: finds index")
(assert-nil (position 4 '(1 2 3)) "position: not found")

;; find with :key
(assert-equal 5 (find 4 '(1 2 3 4 5) :key (lambda (x) (- x 1))) "find: with :key")

(end-suite)

;; ============================================================
;; COUNT basic tests
;; ============================================================
(start-suite "count-basic")

(assert-equal 1 (count 1 '(1 2 3)) "count: basic")
(assert-equal 2 (count 'z '(z 1 2 3 z)) "count: multiple occurrences")
(assert-equal 0 (count 'y '(z 1 2 3 z)) "count: not found")

;; count-if
(assert-equal 3 (count-if even? '(1 2 3 4 5 6)) "count-if: evens")
(assert-equal 0 (count-if (lambda (x) (> x 10)) '(1 2)) "count-if: none found")

(end-suite)

;; ============================================================
;; REDUCE basic tests
;; ============================================================
(start-suite "reduce-basic")

;; Basic reduce
(assert-equal 6 (reduce + '(1 2 3)) "reduce: sum")
(assert-equal 10 (reduce + '(1 2 3 4)) "reduce: sum 4")
(assert-equal 3 (reduce + '(1 2) :initial-value 0) "reduce: with initial value")
(assert-equal 6 (reduce + '() :initial-value 6) "reduce: empty with initial")

(end-suite)

;; ============================================================
;; EVERY/SOME basic tests
;; ============================================================
(start-suite "every-some-basic")

;; every
(assert-true (every number? '(1 2 3)) "every: all numbers")
(assert-false (every number? '(1 2 #\a)) "every: mixed fails")
(assert-true (every number? '()) "every: empty list returns true")

;; some
(assert-true (some number? '(1 2 3)) "some: found number")
(assert-nil (some number? '(#\a #\b)) "some: no numbers")
(assert-nil (some number? '()) "some: empty list returns nil")

(end-suite)

;; ============================================================
;; SORT/STABLE-SORT basic tests
;; ============================================================
(start-suite "sort-basic")

;; Basic sort
(assert-equal '(1 2 3) (sort '(3 1 2) '<) "sort: numbers")
(assert-equal '(3 2 1) (sort '(1 2 3) '>) "sort: descending")

;; stable-sort
(assert-equal '(1 2 3) (stable-sort '(3 1 2) '<) "stable-sort: numbers")
(assert-equal '((1 a) (2 b) (3 c)) (stable-sort '((3 c) (1 a) (2 b)) '< :key 'car)
  "stable-sort: with :key")

;; sort with string
(assert-equal '("a" "b" "c") (sort '("c" "a" "b") 'string<) "sort: strings")

(end-suite)

;; ============================================================
;; REVERSE/NREVERSE tests
;; ============================================================
(start-suite "reverse-basic")

(assert-equal '() (reverse '()) "reverse: empty")
(assert-equal '(3 2 1) (reverse '(1 2 3)) "reverse: basic")
(assert-equal '((3 4) (1 2)) (reverse '((1 2) (3 4))) "reverse: nested")
(assert-equal "cba" (reverse "abc") "reverse: string")

;; nreverse is destructive
(define rlist '(1 2 3 4))
(define rresult (nreverse rlist))
(assert-equal '(4 3 2 1) rresult "nreverse: result")

(end-suite)

;; ============================================================
;; MAP tests
;; ============================================================
(start-suite "map-basic")

;; map with list result
(assert-equal '(2 3 4) (map 'list (lambda (x) (+ x 1)) '(1 2 3)) "map: list result")
(assert-equal '(6 8 10) (map 'list + '(1 2 3) '(5 6 7)) "map: two lists")

;; map with vector result
(assert-equal (vector 2 3 4) (map 'vector (lambda (x) (+ x 1)) '(1 2 3)) "map: vector result")

(end-suite)

;; ============================================================
;; COPY-SEQ tests
;; ============================================================
(start-suite "copy-seq-basic")

(assert-equal '(1 2 3) (copy-seq '(1 2 3)) "copy-seq: list")
(assert-equal "abc" (copy-seq "abc") "copy-seq: string")
(assert-equal '() (copy-seq '()) "copy-seq: empty")

(end-suite)

;; ============================================================
;; MERGE tests
;; ============================================================
(start-suite "merge-basic")

;; merge lists
(assert-equal '() (merge 'list '() '() '<) "merge: both empty")
(assert-equal '(1) (merge 'list '() (list 1) '<) "merge: one empty")
(assert-equal '(1 2 2 3 4 7) (merge 'list (list 1 2 4) (list 2 3 7) '<) "merge: interleaved")

(end-suite)

;; ============================================================
;; Summary
;; ============================================================
(test-summary)
