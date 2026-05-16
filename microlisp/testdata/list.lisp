;; ============================================================
;; List & Pair Operations Tests
;; ============================================================

(load "tests/framework.lisp")
(start-suite "Pair Operations")

(define p (cons 1 2))
(assert-equal 1 (car p) "car of (1 . 2) = 1")
(assert-equal 2 (cdr p) "cdr of (1 . 2) = 2")
(assert-equal '(1 . 2) p "dotted pair display")

;; Nested cons
(define p2 (cons 1 (cons 2 3)))
(assert-equal 1 (car p2) "car of nested cons = 1")
(assert-equal 2 (car (cdr p2)) "caadr of nested cons = 2")
(assert-equal 3 (cdr (cdr p2)) "cddr of nested cons = 3")

;; Proper list construction
(define lst (cons 1 (cons 2 (cons 3 '()))))
(assert-equal 1 (car lst) "proper list: car = 1")
(assert-equal 2 (car (cdr lst)) "proper list: cadr = 2")
(assert-equal 3 (car (cdr (cdr lst))) "proper list: caddr = 3")
(assert-equal '() (cdr (cdr (cdr lst))) "proper list: cdddr = ()")
(assert-equal '(1 2 3) lst "proper list display")

;; set-car! / set-cdr!
(define p3 (cons 'a 'b))
(set-car! p3 'x)
(assert-equal 'x (car p3) "set-car!: car changed to x")
(set-cdr! p3 'y)
(assert-equal 'y (cdr p3) "set-cdr!: cdr changed to y")
(assert-equal '(x . y) p3 "set-car!/set-cdr! result")

;; set-car! on list
(define l2 '(1 2 3))
(set-car! l2 99)
(assert-equal 99 (car l2) "set-car! on list: first element changed")
(assert-equal 2 (cadr l2) "set-car! on list: second unchanged")

(end-suite)

(start-suite "List Predicates")

(assert-true (null? '()) "null?: empty list")
(assert-false (null? '(1)) "null?: non-empty list")
(assert-false (null? 1) "null?: number is not null")
(assert-false (null? #f) "null?: #f is not null")

(assert-true (pair? '(1 2)) "pair?: proper list is pair")
(assert-true (pair? '(1 . 2)) "pair?: dotted pair is pair")
(assert-false (pair? '()) "pair?: empty list is not pair")
(assert-false (pair? 42) "pair?: number is not pair")
(assert-false (pair? 'symbol) "pair?: symbol is not pair")
(assert-false (pair? "str") "pair?: string is not pair")

(assert-false (null? '(1 . 2)) "null?: dotted pair not null")
(assert-true (pair? (cons 1 2)) "pair?: cons returns pair")

(end-suite)

(start-suite "Type Predicates")

(assert-true (number? 42) "number?: integer")
(assert-true (number? 3.14) "number?: float")
(assert-false (number? "hi") "number?: string not number")
(assert-false (number? 'x) "number?: symbol not number")

(assert-true (string? "hello") "string?: basic string")
(assert-false (string? 'sym) "string?: symbol not string")
(assert-false (string? 42) "string?: number not string")

(assert-true (symbol? 'x) "symbol?: basic symbol")
(assert-true (symbol? 'hello-world) "symbol?: hyphenated symbol")
(assert-false (symbol? "str") "symbol?: string not symbol")
(assert-false (symbol? 42) "symbol?: number not symbol")

(assert-true (bool? #t) "bool?: true")
(assert-true (bool? #f) "bool?: false")
(assert-false (bool? 0) "bool?: 0 not bool")
(assert-false (bool? '()) "bool?: () not bool")

(assert-true (procedure? +) "procedure?: + is procedure")
(assert-true (procedure? car) "procedure?: car is procedure")
(assert-true (procedure? (lambda (x) x)) "procedure?: lambda is procedure")
(assert-false (procedure? 42) "procedure?: number not procedure")
(assert-false (procedure? 'x) "procedure?: symbol not procedure")

(end-suite)

(start-suite "Equality Predicates")

;; eq? (pointer equality)
(assert-true (eq? 'a 'a) "eq?: same symbol")
(assert-false (eq? 'a 'b) "eq?: different symbols")
(assert-true (eq? #t #t) "eq?: same boolean")
(assert-false (eq? #t #f) "eq?: different booleans")

;; equal? (structural equality)
(assert-true (equal? 'a 'a) "equal?: same symbol")
(assert-false (equal? 'a 'b) "equal?: different symbols")
(assert-true (equal? '(1 2 3) '(1 2 3)) "equal?: same lists")
(assert-false (equal? '(1 2 3) '(1 2 4)) "equal?: different lists")
(assert-true (equal? '(1 . 2) '(1 . 2)) "equal?: same dotted pairs")
(assert-false (equal? '(1 . 2) '(1 . 3)) "equal?: different dotted pairs")
(assert-true (equal? "hello" "hello") "equal?: same strings")
(assert-false (equal? "hello" "world") "equal?: different strings")
(assert-true (equal? 42 42) "equal?: same numbers")
(assert-false (equal? 42 43) "equal?: different numbers")
(assert-true (equal? '() '()) "equal?: same nil")
(assert-true (equal? '(1 (2 3) 4) '(1 (2 3) 4)) "equal?: nested lists")

(end-suite)

(start-suite "List Utilities")

;; length
(assert-equal 0 (length '()) "length: empty list")
(assert-equal 1 (length '(1)) "length: single element")
(assert-equal 5 (length '(1 2 3 4 5)) "length: five elements")
(assert-equal 3 (length '((1 2) (3 4) (5 6))) "length: nested list")

;; list
(assert-equal '() (list) "list: no args = ()")
(assert-equal '(1) (list 1) "list: single element")
(assert-equal '(1 2 3) (list 1 2 3) "list: three elements")
(assert-equal '(#t #f "hi") (list #t #f "hi") "list: mixed types")

;; append
(assert-equal '() (append '() '()) "append: both empty")
(assert-equal '(1) (append '(1) '()) "append: non-empty + empty")
(assert-equal '(1) (append '() '(1)) "append: empty + non-empty")
(assert-equal '(1 2 3 4) (append '(1 2) '(3 4)) "append: basic")
(assert-equal '(1 2 3 4 5 6) (append (append '(1 2) '(3 4)) '(5 6))
  "append: three lists")

(define l3 '(a b c))
(assert-equal '(a b c d) (append l3 '(d)) "append: non-destructive")

;; reverse
(assert-equal '() (reverse '()) "reverse: empty")
(assert-equal '(1) (reverse '(1)) "reverse: single")
(assert-equal '(3 2 1) (reverse '(1 2 3)) "reverse: three elements")
(assert-equal '(5 4 3 2 1) (reverse '(1 2 3 4 5)) "reverse: five elements")
(assert-equal '((3 4) (1 2)) (reverse '((1 2) (3 4))) "reverse: nested lists")

;; list-ref
(assert-equal 'a (list-ref '(a b c) 0) "list-ref: first element")
(assert-equal 'b (list-ref '(a b c) 1) "list-ref: second element")
(assert-equal 'c (list-ref '(a b c) 2) "list-ref: third element")
(assert-equal 99 (list-ref '(1 99 3) 1) "list-ref: middle element")

;; assoc
(assert-nil (assoc 'a '()) "assoc: empty alist")
(assert-equal '(a 1) (assoc 'a '((a 1) (b 2))) "assoc: find a")
(assert-equal '(b 2) (assoc 'b '((a 1) (b 2))) "assoc: find b")
(assert-nil (assoc 'c '((a 1) (b 2))) "assoc: not found returns nil")

;; member? (custom function, from stdlib)
(assert-false (member? 'x '()) "member?: empty list")
(assert-true (member? 'a '(a b c)) "member?: found a")
(assert-true (member? 'c '(a b c)) "member?: found c")
(assert-false (member? 'z '(a b c)) "member?: not found")

(end-suite)

(start-suite "Dotted Pairs and Improper Lists")

(define dp (cons 1 2))
(assert-true (pair? dp) "pair?: dotted pair")
(assert-equal 1 (car dp) "car of dotted pair")
(assert-equal 2 (cdr dp) "cdr of dotted pair")

;; Improper list (a b . c)
(define il (cons 'a (cons 'b 'c)))
(assert-equal 'a (car il) "car of improper list")
(assert-equal 'b (car (cdr il)) "cadr of improper list")
(assert-equal 'c (cdr (cdr il)) "cddr of improper list")
(assert-equal '(a b . c) il "improper list display")

;; Element access - cXr combinations
(assert-equal 'a (caar '((a b) c)) "caar")
(assert-equal 'b (cadar '((a b) c)) "cadar")
(assert-equal '(b) (cdar '((a b) c)) "cdar")
(assert-equal '(b c) (cdr '(a b c)) "cdr")
(assert-equal 'c (caddr '(a b c)) "caddr")

(end-suite)

(test-summary)
