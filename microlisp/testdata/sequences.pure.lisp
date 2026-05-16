;; ============================================================
;; Sequence Tests - simplified for MicroLisp capabilities
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
;; Sequence Iteration
;; ============================================================
(start-suite "sequence-iteration")

;; REDUCE
(assert-equal 6 (reduce + '(1 2 3)) "reduce: sum")
(assert-equal 10 (reduce + '(1 2 3 4)) "reduce: sum 4")
(assert-equal 3 (reduce + '(1 2) :initial-value 0) "reduce: with initial value")

;; EVERY and SOME
(assert-true (every number? '(1 2 3)) "every: all numbers")
(assert-false (every number? '(1 2 #\a)) "every: mixed")
(assert-true (some number? '(1 2 3)) "some: found number")
(assert-nil (some number? '(#\a #\b)) "some: no numbers")

(end-suite)

;; ============================================================
;; Sequence Filtering
;; ============================================================
(start-suite "sequence-filtering")

;; REMOVE
(assert-equal '(1 3) (remove 2 '(1 2 3)) "remove: basic")

;; SUBSTITUTE
(assert-equal '(1 x 3) (substitute 'x 2 '(1 2 3)) "substitute: basic")

(end-suite)

;; ============================================================
;; Sequence Modification
;; ============================================================
(start-suite "sequence-modification")

;; REVERSE
(assert-equal '(3 2 1) (reverse '(1 2 3)) "reverse: list")
(assert-equal "cba" (reverse "abc") "reverse: string")

;; APPEND
(assert-equal '(1 2 3 4) (append '(1 2) '(3 4)) "append: two lists")
(assert-equal '(1) (append '() '(1)) "append: with empty")

(end-suite)

;; ============================================================
;; Sequence Sorting
;; ============================================================
(start-suite "sequence-sorting")

(assert-equal '(1 2 3) (sort '(3 1 2) '<) "sort: numbers")
(assert-equal '(3 2 1) (sort '(1 2 3) '>) "sort: descending")

;; STABLE-SORT
(assert-equal '(1 2 3) (stable-sort '(3 1 2) '<) "stable-sort: numbers")

(end-suite)

;; ============================================================
;; Subsequences
;; ============================================================
(start-suite "subsequences")

(assert-equal '(2 3) (subseq '(1 2 3 4) 1 3) "subseq: list")

;; COPY-SEQ
(assert-equal '(1 2 3) (copy-seq '(1 2 3)) "copy-seq: list")
(assert-equal "abc" (copy-seq "abc") "copy-seq: string")

(end-suite)

;; ============================================================
;; Sequence Predicates
;; ============================================================
(start-suite "sequence-predicates")

(assert-true (list? '(1 2 3)) "list?: proper list")
(assert-true (list? '(1 . 2)) "list?: dotted pair returns t in MicroLisp")
(assert-false (list? "abc") "list?: string")

(end-suite)

(test-summary)
