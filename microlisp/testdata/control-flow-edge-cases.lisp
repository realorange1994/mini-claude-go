;; ============================================================
;; Control Flow Edge Cases - derived from SBCL case.pure.lisp, constantp.pure.lisp
;; ============================================================

(load "tests/framework.lisp")

;; ============================================================
;; Case Basic Tests
;; ============================================================
(start-suite "case-basics")

(assert-equal 'a (case 'a (a 'a) (b 'b)) "case: matches first key")
(assert-equal 'b (case 'b (a 'a) (b 'b)) "case: matches second key")
(assert-nil (case 'c (a 'a) (b 'b)) "case: no match returns nil")

(end-suite)

;; ============================================================
;; Case Key Types
;; ============================================================
(start-suite "case-key-types")

;; Symbols as keys
(assert-equal 'sym (case 'sym ((sym) 'sym) (other 'other)) "case: symbol key")
(assert-equal 'other (case 'other ((sym) 'sym) (other 'other)) "case: symbol key other")

;; Numbers as keys
(assert-equal 'one (case 1 (1 'one) (2 'two)) "case: integer key")
(assert-equal 'two (case 2 (1 'one) (2 'two)) "case: integer key 2")

;; Strings as keys (if supported)

(end-suite)

;; ============================================================
;; Case Clause Forms
;; ============================================================
(start-suite "case-clauses")

;; Single value clause
(assert-equal 'result (case 'a (a 'result)) "case: single value clause")

;; Multiple values in key list

;; Key and body
(assert-equal 42 (case 'x (x 42) (y 100)) "case: key with body")

(end-suite)

;; ============================================================
(start-suite "if-edge-cases")

(assert-equal 'true (if #t 'true 'false) "if: true branch")
(assert-equal 'false (if #f 'true 'false) "if: false branch")
(assert-nil (if #f 'not-reached) "if: returns nil on false with no else")

(end-suite)

;; ============================================================
;; COND Edge Cases
;; ============================================================
(start-suite "cond-edge-cases")

(assert-equal 'first (cond (#t 'first) (#t 'second)) "cond: returns first matching")
(assert-equal 'second (cond (#f 'first) (#t 'second)) "cond: skips false condition")
(assert-nil (cond (#f 'first)) "cond: returns nil with no else")

(end-suite)

;; ============================================================
;; PROGN and Sequencing
;; ============================================================
(start-suite "progn-edge-cases")

(assert-equal 3 (progn 1 2 3) "progn: returns last form")
(assert-nil (progn) "progn: empty returns nil")
(assert-equal 10 (progn 1 2 3 (+ 4 6)) "progn: evaluates all forms")

(end-suite)

;; ============================================================
;; AND/OR Edge Cases
;; ============================================================
(start-suite "and-or-edge-cases")

(assert-equal #t (and) "and: no args returns true")
(assert-equal #f (and #f) "and: single false")
(assert-equal 1 (and 1) "and: single true returns value")
(assert-equal #f (and 1 #f 3) "and: stops at first false")
(assert-equal 3 (and 1 2 3) "and: returns last if all true")

(assert-equal #f (or) "or: no args returns false")
(assert-equal 1 (or #f 1) "or: returns first true")
(assert-equal #f (or #f #f) "or: all false returns false")

(end-suite)

;; ============================================================
;; LET and LET*
;; ============================================================
(start-suite "let-edge-cases")

(assert-equal 3 (let ((x 1) (y 2)) (+ x y)) "let: basic binding")
(assert-equal 2 (let ((x 2)) x) "let: returns body")
(assert-equal 5 (let ((x 1)) (set! x 5) x) "let: setf after binding")

(end-suite)

;; ============================================================
;; Lambda and Closures
;; ============================================================
(start-suite "lambda-edge-cases")

(define add (lambda (x y) (+ x y)))
(assert-equal 7 (add 3 4) "lambda: basic function call")
(assert-equal 10 ((lambda (x) (+ x 5)) 5) "lambda: immediate call")

(end-suite)

;; ============================================================
;; SET! and Symbol Value
;; ============================================================
(start-suite "set-bang-edge")

(define x 1)
(set! x 5)
(assert-equal 5 x "set!: updates value")

(end-suite)

;; ============================================================
;; QUOTE and QUASIQUOTE
;; ============================================================
(start-suite "quote-edge-cases")

(assert-equal '(a b c) (quote (a b c)) "quote: simple list")
(assert-equal 'x (quote x) "quote: symbol")
(assert-equal '(1 2 3) (list 1 2 3) "list: creates list")

(end-suite)

;; ============================================================
;; Summary
;; ============================================================
(test-summary)
