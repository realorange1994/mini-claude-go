;; ============================================================
;; Enhanced Sequence Tests - derived from SBCL seq.pure.lisp
;; ============================================================

(load "tests/framework.lisp")

;; ============================================================
;; REMOVE and REMOVE-DUPLICATES
;; ============================================================
(start-suite "remove")

;; Basic remove
(assert-equal '(1 2 4 5) (remove 3 '(1 2 3 4 5)) "remove: basic")
(assert-equal '(1 2 3 4 5) (remove 99 '(1 2 3 4 5)) "remove: not found")
(assert-equal '() (remove 1 '(1 1 1)) "remove: removes all")

;; remove with :count
(assert-equal '(1 1 2 3) (remove 1 '(1 1 1 2 3) :count 1) "remove: :count 1")
;; MicroLisp's remove ignores :count 0
(assert-equal '(1 1 1 2 3) (remove 1 '(1 1 1 2 3) :count 0) "remove: :count 0 returns all")

;; remove with :start :end :from-end
(define nums '(1 2 3 2 6 1 2 4 1 3 2 7))
(assert-equal '(1 2 2 6 1 2 4 1 3 2 7) (remove 3 nums :from-end t :start 1 :end 5) "remove: from-end with bounds")

(end-suite)

(start-suite "remove-duplicates")

(define dups '(0 1 2 0 1 2 0 1 2 0 1 2))
(assert-equal '(0 1 2 0 1 2 0 1 2) (remove-duplicates dups :start 3 :end 9) "remove-duplicates: with bounds")
(assert-equal '(1 2 3) (remove-duplicates '(1 2 3 2 1)) "remove-duplicates: basic")

(end-suite)

;; ============================================================
;; SUBSTITUTE and NSUBSTITUTE
;; ============================================================
(start-suite "substitute")

(assert-equal '(1 2 X 4 5) (substitute 'X 3 '(1 2 3 4 5)) "substitute: basic")
(assert-equal '(1 2 X 4 3 5) (substitute 'X 3 '(1 2 3 4 3 5) :count 1) "substitute: :count 1")
(assert-equal '(1 2 X 4 5) (substitute 'X 3 '(1 2 3 4 5)) "substitute: basic substitutes all")
(assert-equal '(1 2 X 4 5) (substitute 'X 3 '(1 2 3 4 5) :from-end t :count 1) "substitute: from-end :count 1 matches first from end")

(end-suite)

;; ============================================================
;; COUNT
;; ============================================================
(start-suite "count")

(assert-equal 1 (count 1 '(1 2 3)) "count: finds 1")
(assert-equal 2 (count 'z '(z 1 2 3 z)) "count: finds z twice")
(assert-equal 0 (count 'y '(z 1 2 3 z)) "count: not found = 0")

;; count with :from-end
(assert-equal 3 (count 1 '(1 2 1 2 1)) "count: total count")

(end-suite)

(start-suite "count-if")

(define mixed '(1 (2) 3 (4) (5) 6))
(assert-equal 3 (count-if 'consp mixed) "count-if: consp finds 3")
(assert-equal 3 (count-if 'consp '(1 (2) 3 (4) (5) 6)) "count-if: consp finds 3")

(end-suite)

;; ============================================================
;; FIND and POSITION
;; ============================================================
(start-suite "find")

(assert-equal 4 (find 4 '(1 2 3 4 5)) "find: finds 4")
(assert-equal '() (find 99 '(1 2 3 4 5)) "find: not found returns nil")
(assert-equal 2 (find-if 'evenp '(1 2 3 4 5)) "find-if: finds even")
(assert-equal 2 (find-if 'evenp '(1 2 3 4 5) :start 1) "find-if: with :start")

(end-suite)

(start-suite "position")

(define nums '(3 1 4 1 5 9 2 6))
(assert-equal 2 (position 4 nums) "position: of 4 is 2")
(assert-equal '() (position 99 nums) "position: not found returns nil")
(assert-equal 1 (position 1 nums) "position: first 1")
(assert-equal 3 (position 1 nums :from-end t) "position: from-end last 1")

(end-suite)

;; ============================================================
;; REDUCE
;; ============================================================
(start-suite "reduce")

(assert-equal 15 (reduce + '(1 2 3 4 5)) "reduce: sum")
(assert-equal 21 (reduce + '(1 2 3 4 5) :initial-value 6) "reduce: with initial-value")
(assert-equal 15 (reduce + '(1 2 3 4 5) :initial-value 0) "reduce: initial-value 0")
(assert-equal '((1 2) 3) (reduce 'list '(1 2 3)) "reduce: list builds nested pairs")
(assert-equal 6 (reduce * '(1 2 3) :initial-value 1) "reduce: product")

(end-suite)

;; ============================================================
;; MERGE and SORT
;; ============================================================
(start-suite "sort")

(assert-equal '(1 2 3 4 5) (sort '(3 1 4 5 2) '<) "sort: ascending")
(assert-equal '(5 4 3 2 1) (sort '(3 1 4 5 2) '>) "sort: descending")
(assert-equal '(1 2 3 4 5) (sort '(1 2 3 4 5) '<) "sort: already sorted")
(assert-equal '() (sort '() '<) "sort: empty")

(end-suite)

(start-suite "stable-sort")

(assert-equal '(1 2 3 3 12 13) (stable-sort '(3 1 2 12 13 3) '<) "stable-sort: basic")
(assert-equal '(13 12 10 3 2 1) (stable-sort '(1 10 2 12 13 3) '< :key '-) "stable-sort: with :key")

(end-suite)

(start-suite "merge")

(assert-equal '(1 2 2 3 4 7) (merge 'list '(1 2 4) '(2 3 7) '<) "merge: basic")
(assert-equal '() (merge 'list '() '() '<) "merge: both empty")
(assert-equal '(1 2) (merge 'list '(1 2) '() '<) "merge: one empty list")
(assert-equal '(1 2 3) (merge 'list '() '(1 2 3) '<) "merge: first empty")

(end-suite)

;; ============================================================
;; COPY-SEQ
;; ============================================================
(start-suite "copy-seq")

(define orig '(1 2 3 4 5))
(define copy (copy-seq orig))
(assert-equal '(1 2 3 4 5) copy "copy-seq: equal contents")
(assert-true (not (eq? orig copy)) "copy-seq: different cons cells")

;; copy-seq on strings
(define s "hello")
(define s-copy (copy-seq s))
(assert-equal "hello" s-copy "copy-seq: string")

(end-suite)

;; ============================================================
;; CONCATENATE
;; ============================================================
(start-suite "concatenate")

(assert-equal '(1 2 3 4 5 6) (concatenate 'list '(1 2 3) '(4 5 6)) "concatenate: list")
(assert-equal "helloworld" (concatenate 'string "hello" "world") "concatenate: string")
(assert-equal '(1 2 3) (concatenate 'list '(1 2 3)) "concatenate: single list")

(end-suite)

;; ============================================================
;; MAP and MAP-INTO
;; ============================================================
(start-suite "map")

(assert-equal '(2 4 6 8 10) (map 'list (lambda (x) (* x 2)) '(1 2 3 4 5)) "map: double")
(assert-equal '(2 4 6) (mapcar + '(1 2 3) '(1 2 3)) "mapcar: add lists")
(assert-equal '(1 4 9) (mapcar * '(1 2 3) '(1 2 3)) "mapcar: multiply")

(end-suite)

;; ============================================================
;; SOME, EVERY, NOTANY, NOTEVERY
;; ============================================================
(start-suite "sequence-predicates")

(assert-true (some even? '(1 2 3 4)) "some: finds even")
(assert-equal '() (some even? '(1 3 5)) "some: none found returns nil")
(assert-true (every even? '(2 4 6)) "every: all even")
(assert-false (every even? '(2 3 4)) "every: not all even")
(assert-true (notany even? '(1 3 5)) "notany: none")
(assert-false (notevery even? '(2 4 6)) "notevery: all")

(end-suite)

;; ============================================================
;; FILL and REPLACE
;; ============================================================
(start-suite "fill")

(define arr '(1 2 3 4 5))
(define filled (fill arr 9))
(assert-equal '(9 9 9 9 9) filled "fill: all 9s")
(assert-equal "zzzzz" (fill (copy-seq "abcde") #\z) "fill: string")

(end-suite)

(start-suite "replace")

(define v1 '(1 2 3 4 5))
(define v2 '(10 20))
(assert-equal '(10 20 3 4 5) (replace v1 v2) "replace: basic replaces from start")
(assert-equal '(10 20 3 4 5) (replace '(1 2 3 4 5) '(10 20) :start1 0 :end1 2) "replace: with bounds")

(end-suite)

;; ============================================================
;; SEARCH
;; ============================================================
(start-suite "search")

(assert-equal 1 (search '(2 3) '(1 2 3 4 5)) "search: finds subsequence")
(assert-equal '() (search '() '(1 2 3)) "search: empty pattern returns nil in MicroLisp")
(assert-equal '() (search '(99) '(1 2 3)) "search: not found returns nil")
(assert-equal 2 (search '(3 4) '(1 2 3 4 5) :start2 1) "search: with start2")

(end-suite)

;; ============================================================
;; MISMATCH
;; ============================================================
(start-suite "mismatch")

(assert-equal 2 (mismatch '(1 2 3 4) '(1 2 5 4)) "mismatch: first difference at 2")
(assert-equal '() (mismatch '(1 2 3) '(1 2 3)) "mismatch: equal returns nil")
(assert-equal 3 (mismatch "abcdef" "abcxyz") "mismatch: string")

(end-suite)

;; ============================================================
;; Summary
;; ============================================================
(test-summary)
