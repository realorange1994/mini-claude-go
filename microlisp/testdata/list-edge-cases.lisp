;; ============================================================
;; Enhanced List Tests - simplified for MicroLisp
;; ============================================================

(load "tests/framework.lisp")

;; ============================================================
;; BUTLAST edge cases
;; ============================================================
(start-suite "butlast-edge-cases")

;; butlast with proper lists
(assert-equal '(1 2 3) (butlast '(1 2 3 4 5) 2) "butlast: n=2")
(assert-equal '() (butlast '(1 2 3) 10) "butlast: n > length")

;; butlast should return a copy, not the original
(define butlast-test-list '(1 2 3 4 5))
(define butlast-result (butlast butlast-test-list))
(assert-true (not (eq? butlast-test-list butlast-result)) "butlast: returns a copy")
(assert-equal '(1 2 3 4) butlast-result "butlast: correct result")

(end-suite)

;; ============================================================
;; LAST edge cases
;; ============================================================
(start-suite "last-edge-cases")

;; last with various n values
(assert-equal '(5) (last '(1 2 3 4 5)) "last: default")
(assert-equal '(4 5) (last '(1 2 3 4 5) 2) "last: n=2")
(assert-equal '(3 4 5) (last '(1 2 3 4 5) 3) "last: n=3")
(assert-equal '(1 2 3 4 5) (last '(1 2 3 4 5) 10) "last: n > length")
(assert-equal '() (last '()) "last: empty list")

(end-suite)

;; ============================================================
;; MEMBER edge cases
;; ============================================================
(start-suite "member-edge-cases")

;; member basic
(assert-equal '(2 3) (member 2 '(1 2 3)) "member: basic")
(assert-nil (member 4 '(1 2 3)) "member: not found")
(assert-equal '(a b c) (member 'a '(a b c)) "member: basic symbol find")

;; member with :test
(assert-equal '(2 3) (member 2 '(1 2 3) :test 'eql) "member: with :test eql")
(assert-equal '(2 3) (member 2 '(1 2 3) :test '=) "member: with :test =")

;; member-if
(assert-equal '(2 3 4 5 6) (member-if even? '(1 2 3 4 5 6)) "member-if: finds even")
(assert-equal '(3 4 5 6) (member-if (lambda (x) (> x 2)) '(1 2 3 4 5 6)) "member-if: finds > 2")
(assert-nil (member-if (lambda (x) (> x 10)) '(1 2 3)) "member-if: not found")
(assert-equal '() (member-if number? '()) "member-if: empty list")

(end-suite)

;; ============================================================
;; ASSOC/RASSOC edge cases
;; ============================================================
(start-suite "assoc-edge-cases")

(define alist '((a . 1) (b . 2) (c . 3)))

;; assoc basic
(assert-equal '(a . 1) (assoc 'a alist) "assoc: finds first")
(assert-equal '(b . 2) (assoc 'b alist) "assoc: finds second")
(assert-nil (assoc 'd alist) "assoc: not found returns nil")

;; rassoc basic
(define ralist '((a . 1) (b . 2) (c . 3)))
(assert-equal '(a . 1) (rassoc 1 ralist) "rassoc: finds by value 1")
(assert-equal '(c . 3) (rassoc 3 ralist) "rassoc: finds by value 3")
(assert-equal '() (rassoc 99 ralist) "rassoc: not found returns empty list")

(end-suite)

;; ============================================================
;; NCONC and APPEND edge cases
;; ============================================================
(start-suite "nconc-edge-cases")

;; Basic nconc
(define n1 '(1 2))
(define n2 '(3 4))
(assert-equal '(1 2 3 4) (nconc n1 n2) "nconc: basic")
(assert-equal '(1 2 3 4) n1 "nconc: first list modified")

;; nconc with empty lists
(assert-equal '(1 2) (nconc '(1 2) '()) "nconc: with empty")
(assert-equal '(1 2 3) (nconc '() '(1 2 3)) "nconc: empty + list")
(assert-equal '() (nconc '() '()) "nconc: both empty")

(end-suite)

(start-suite "append-edge-cases")

;; Multiple append
(assert-equal '(1 2 3 4 5 6) (append '(1 2) '(3 4) '(5 6)) "append: three lists")
(assert-equal '(1 2) (append '(1 2) '()) "append: with empty")
(assert-equal '(3 4) (append '() '(3 4)) "append: from empty")

;; append is non-destructive
(define a '(1 2))
(define b '(3 4))
(assert-equal '(1 2 3 4) (append a b) "append: non-destructive")
(assert-equal '(1 2) a "append: original unchanged")
(assert-equal '(3 4) b "append: second unchanged")

(end-suite)

;; ============================================================
;; SET operations edge cases
;; ============================================================
(start-suite "set-operations-edge")

;; union edge cases
(assert-equal '() (union '() '()) "union: both empty")
(assert-equal '(1 2 3) (union '(1 2) '(2 3)) "union: basic")
(assert-equal '(1 2 3 4) (union '(1 2) '(3 4)) "union: disjoint")
(assert-equal '(1) (union '(1) '(1)) "union: duplicates")

;; intersection edge cases
(assert-equal '() (intersection '() '()) "intersection: both empty")
(assert-equal '(2) (intersection '(1 2 3) '(2 4 5)) "intersection: basic")
(assert-equal '() (intersection '(1 2) '(3 4)) "intersection: disjoint")

;; set-difference edge cases
(assert-equal '(1 3) (set-difference '(1 2 3) '(2 4 5)) "set-difference: basic")
(assert-equal '(1 2 3) (set-difference '(1 2 3) '()) "set-difference: empty second")
(assert-equal '() (set-difference '() '(1 2 3)) "set-difference: empty first")

(end-suite)

;; ============================================================
;; SUBSEQ edge cases
;; ============================================================
(start-suite "subseq-edge-cases")

(assert-equal '(1 2 3 4 5) (subseq '(1 2 3 4 5) 0) "subseq: from 0 returns all")
(assert-equal '(3 4 5) (subseq '(1 2 3 4 5) 2) "subseq: from 2")
(assert-equal '(3 4) (subseq '(1 2 3 4 5) 2 4) "subseq: from 2 to 4")
(assert-equal '() (subseq '(1 2 3) 5) "subseq: start beyond length")
(assert-equal '() (subseq '(1 2 3) 2 2) "subseq: equal start end")

(end-suite)

;; ============================================================
;; NTHCDR and LDIFF edge cases
;; ============================================================
(start-suite "nthcdr-ldiff-edge")

;; nthcdr
(assert-equal '(1 2 3 4 5) (nthcdr 0 '(1 2 3 4 5)) "nthcdr: 0 returns same")
(assert-equal '(2 3 4 5) (nthcdr 1 '(1 2 3 4 5)) "nthcdr: 1")
(assert-equal '(3 4 5) (nthcdr 2 '(1 2 3 4 5)) "nthcdr: 2")
(assert-equal '() (nthcdr 5 '(1 2 3 4 5)) "nthcdr: 5 = length")
(assert-equal '() (nthcdr 10 '(1 2 3 4 5)) "nthcdr: beyond length")
(assert-equal '() (nthcdr 100 '()) "nthcdr: empty list")

(end-suite)

;; ============================================================
;; Summary
;; ============================================================
(test-summary)
