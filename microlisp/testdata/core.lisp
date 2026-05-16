;; ============================================================
;; Core Language Tests - arithmetic, special forms, etc.
;; ============================================================

(load "tests/framework.lisp")
(start-suite "Core Arithmetic")

;; Basic arithmetic
(assert-equal 3 (+ 1 2) "1 + 2 = 3")
(assert-equal 7 (+ 1 2 3 1) "1 + 2 + 3 + 1 = 7")
(assert-equal 5 (- 10 5) "10 - 5 = 5")
(assert-equal 2 (- 10 3 5) "10 - 3 - 5 = 2")
(assert-equal -5 (- 5) "negation: -5")
(assert-equal 12 (* 3 4) "3 * 4 = 12")
(assert-equal 120 (* 2 3 4 5) "2 * 3 * 4 * 5 = 120")
(assert-equal 5 (/ 15 3) "15 / 3 = 5")
(assert-equal 3 (/ 24 2 4) "24 / 2 / 4 = 3")

;; Comparisons
(assert-true (= 5 5) "=: 5 = 5")
(assert-false (= 5 6) "=: 5 != 6")
(assert-true (= 1 1 1) "=: 1 = 1 = 1")
(assert-false (= 1 2 1) "=: 1 != 2")
(assert-true (< 1 2) "<: 1 < 2")
(assert-false (< 2 1) "<: 2 < 1")
(assert-true (< 1 2 3) "<: 1 < 2 < 3")
(assert-false (< 1 3 2) "<: 1 < 3 !< 2")
(assert-true (> 3 2) ">: 3 > 2")
(assert-false (> 2 3) ">: 2 !> 3")
(assert-true (> 3 2 1) ">: 3 > 2 > 1")
(assert-true (<= 1 1) "<=: 1 <= 1")
(assert-true (<= 1 2) "<=: 1 <= 2")
(assert-false (<= 2 1) "<=: 2 !<= 1")
(assert-true (>= 1 1) ">=: 1 >= 1")
(assert-true (>= 2 1) ">=: 2 >= 1")
(assert-false (>= 1 2) ">=: 1 !>= 2")

;; Float arithmetic
(assert-equal 3.5 (+ 1.5 2.0) "float addition: 1.5 + 2.0 = 3.5")
(assert-equal 2.0 (/ 5 2.5) "float division: 5 / 2.5 = 2.0")

(end-suite)

(start-suite "Special Forms: if")

;; if with both branches
(assert-equal 1 (if #t 1 2) "if #t then 1 else 2 = 1")
(assert-equal 2 (if #f 1 2) "if #f then 1 else 2 = 2")

;; if with single branch
(assert-equal 1 (if #t 1) "if #t then 1 = 1")
(assert-equal '() (if #f 1) "if #f then 1 = ()")

;; Nested if
(assert-equal "positive"
  (if (> 5 0) "positive"
    (if (< 5 0) "negative" "zero"))
  "nested if: positive")

(assert-equal "zero"
  (if (= 0 0) "zero"
    (if (< 0 0) "negative" "positive"))
  "nested if: zero")

;; Truthiness: everything except #f and nil/() is truthy
(assert-equal 1 (if 0 1 2) "0 is truthy in if")
(assert-equal 2 (if '() 1 2) "() is nil/falsy in if")
(assert-equal 1 (if "hello" 1 2) "string is truthy in if")
(assert-equal 1 (if '(1 2) 1 2) "pair is truthy in if")

(end-suite)

(start-suite "Special Forms: begin")

(assert-equal 3 (begin 1 2 3) "begin returns last value")
(assert-equal '() (begin) "begin with no args returns nil")
(assert-equal 42 (begin (define y 42) y) "begin with define + lookup")

;; define inside begin
(define begin-test-val 0)
(begin
  (set! begin-test-val 10)
  (set! begin-test-val 20)
  (set! begin-test-val 30))
(assert-equal 30 begin-test-val "begin evaluates all subforms")

(end-suite)

(start-suite "Special Forms: cond")

(assert-equal 2 (cond (#f 1) (#t 2)) "cond: second clause matches")
(assert-equal 1 (cond (#t 1) (#t 2)) "cond: first clause matches")
(assert-equal '() (cond) "cond: no clauses returns nil")
(assert-equal '() (cond (#f 1)) "cond: no matching clause returns nil")

(define (test-cond n)
  (cond
    ((< n 0) "negative")
    ((= n 0) "zero")
    ((> n 0) "positive")))

(assert-equal "negative" (test-cond -5) "cond: negative")
(assert-equal "zero" (test-cond 0) "cond: zero")
(assert-equal "positive" (test-cond 5) "cond: positive")

;; cond with else
(define (test-cond-else n)
  (cond
    ((< n 0) "negative")
    ((= n 0) "zero")
    (else "positive")))

(assert-equal "positive" (test-cond-else 100) "cond with else")
(assert-equal "negative" (test-cond-else -1) "cond with else: negative")

;; cond single expression returns test value
(assert-true (cond (#t)) "cond single clause returns test value")
(assert-equal 42 (cond ((= 1 1) 42)) "cond single clause with body")

(end-suite)

(start-suite "Special Forms: and/or")

;; and
(assert-true (and) "and with no args = #t")
(assert-equal 1 (and 1) "and with single arg = arg")
(assert-equal 3 (and 1 2 3) "and with all truthy = last")
(assert-false (and #f #t) "and with false = #f")
(assert-false (and 1 2 #f 4) "and short-circuits at #f")

;; side effects not reached after short circuit
(define and-side-effect #f)
(and #f (set! and-side-effect #t))
(assert-false and-side-effect "and short-circuits: side effect not reached")

;; or
(assert-false (or) "or with no args = #f")
(assert-equal 1 (or 1) "or with single arg = arg")
(assert-equal 1 (or #f 1) "or returns first truthy")
(assert-equal 1 (or 1 2 3) "or with all truthy = first")
(assert-false (or #f #f) "or all false = #f")
(assert-false (or #f #f #f) "or all false = #f")

;; short circuit
(define or-side-effect #f)
(or #t (set! or-side-effect #t))
(assert-false or-side-effect "or short-circuits: side effect not reached")

(end-suite)

(start-suite "Quote and Quasiquote")

(assert-equal '(1 2 3) (quote (1 2 3)) "quote list")
(assert-equal 'x (quote x) "quote symbol")
(assert-equal '() (quote ()) "quote empty list")

;; Quote reader macro
(assert-equal '(1 2 3) '(1 2 3) "quote reader macro: '(1 2 3)")
(assert-equal 'x 'x "quote reader macro: 'x")
(assert-equal '() '() "quote reader macro: '()")
(assert-equal '((1 2) 3) '((1 2) 3) "nested quoting")

;; Quasiquote
(assert-equal '(1 2 3) `(1 2 3) "quasiquote without unquote")
(assert-equal '(1 2 3) `(1 2 ,(+ 1 2)) "quasiquote with unquote")
(assert-equal '(1 2 3 4) `(1 ,(+ 1 1) ,(+ 2 1) 4) "multiple unquotes")
(assert-equal 4 `,(+ 1 1 2) "quasiquote: full unquote")

;; Quasiquote with unquote-splicing
(assert-equal '(1 2 3 4) `(1 ,@'(2 3) 4) "unquote-splicing basic")
(assert-equal '(a b c) `(a ,@'(b c)) "unquote-splicing at end")
(assert-equal '(a b c d) `(,@'(a b) ,@'(c d)) "multiple unquote-splicing")

;; Nested quasiquote
(assert-equal '(1 2 3) (let ((x 3)) `(1 2 ,x)) "nested quasiquote: simple with let")

(end-suite)

(start-suite "let and lexical scoping")

;; Basic let
(assert-equal 5 (let ((x 2) (y 3)) (+ x y)) "let: basic binding")
(assert-equal 60 (let ((x 2) (y 3) (z 10)) (* x y z)) "let: multiple bindings")

;; let with body expressions
(assert-equal 5 (let ((x 2)) (set! x 5) x) "let: set! inside body")

;; let shadowing
(define outer-var 10)
(assert-equal 20 (let ((outer-var 20)) outer-var) "let: shadowing outer var")
(assert-equal 10 outer-var "let: outer var unchanged after let")

;; let scoping: bindings not visible to each other (standard let)
;; In standard Scheme, let is parallel, so (let ((x 1) (y (+ x 1))) y)
;; would evaluate (+ x 1) in the outer scope where x may be undefined.
(assert-equal 3 (let ((x 1)) (let ((y (+ x 2))) y)) "let: inner let sees outer binding")

;; Nested let
(assert-equal 30
  (let ((x 10))
    (let ((y 20))
      (+ x y)))
  "nested let")

(end-suite)

(start-suite "lambda and functions")

;; Basic lambda
(assert-equal 9 ((lambda (x) (* x x)) 3) "lambda: square")
(assert-equal 10 ((lambda (x y) (+ x y)) 4 6) "lambda: two args")

;; Lambda with multiple body forms
(assert-equal 6
  ((lambda (x)
     (set! x (+ x 1))
     (set! x (* x 2))
     x)
   2)
  "lambda: multiple body forms")

;; Variadic lambda
(assert-equal '(1 2 3) ((lambda x x) 1 2 3) "variadic lambda: all args")
(assert-equal '(2 3 4) ((lambda (a . rest) rest) 1 2 3 4) "dotted lambda: rest args")

;; Named function define
(define (square x) (* x x))
(assert-equal 25 (square 5) "define: named function (square)")
(assert-equal 49 (square 7) "define: named function (square) again")

(define (add a b) (+ a b))
(assert-equal 9 (add 4 5) "define: add function")
(assert-equal 0 (add -1 1) "define: add with negatives")

(define (factorial n)
  (if (<= n 1) 1
    (* n (factorial (- n 1)))))
(assert-equal 120 (factorial 5) "recursive factorial")
(assert-equal 1 (factorial 0) "factorial of 0")
(assert-equal 1 (factorial 1) "factorial of 1")

;; Lambda as immediate value
(assert-equal 6 ((lambda (x y z) (+ x y z)) 1 2 3) "lambda: three args")

(end-suite)

(start-suite "set! and mutation")

(define m 10)
(set! m 20)
(assert-equal 20 m "set!: simple mutation")

(define n 0)
(set! n (+ n 1))
(set! n (+ n 1))
(set! n (+ n 1))
(assert-equal 3 n "set!: increment three times")

;; set! on captured variable (closure mutation)
(define (make-account balance)
  (lambda (amount)
    (set! balance (+ balance amount))
    balance))
(define acc (make-account 100))
(assert-equal 150 (acc 50) "account: deposit 50")
(assert-equal 130 (acc -20) "account: withdraw 20")
(assert-equal 180 (acc 50) "account: deposit 50 again")

(end-suite)

;; Final summary
(test-summary)
