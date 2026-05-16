;; ============================================================
;; String Operations Tests
;; ============================================================

(load "tests/framework.lisp")
(start-suite "string / string-append / string-length")

(assert-equal 'string (type-of "hello") "type: string")

(assert-equal 'hello (string->symbol (symbol->string 'hello))
  "symbol->string then string->symbol round-trip")

(assert-equal 0 (string-length "") "string-length: empty")
(assert-equal 5 (string-length "hello") "string-length: hello")
(assert-equal 11 (string-length "hello world") "string-length: with space")

(assert-equal "" (string-append) "string-append: no args")
(assert-equal "ab" (string-append "a" "b") "string-append: two strings")
(assert-equal "hello world"
  (string-append "hello " "world") "string-append: with space")
(assert-equal "abc"
  (string-append "a" "b" "c") "string-append: three strings")

(end-suite)

(start-suite "number->string / string->number")

(assert-equal 'string (type-of (number->string 42)) "number->string: returns string")
(assert-equal "42" (number->string 42) "number->string: 42")
(assert-equal "3.14" (number->string 3.14) "number->string: 3.14")

(assert-equal 42 (string->number "42") "string->number: 42")
(assert-equal 3.14 (string->number "3.14") "string->number: 3.14")
(assert-equal '() (string->number "not-a-number") "string->number: invalid")

;; Round trip
(assert-equal 100
  (string->number (number->string 100)) "number->string->number round trip")

(end-suite)

(start-suite "symbol->string / string->symbol")

(assert-equal "HELLO" (symbol->string 'hello) "symbol->string: hello")
(assert-equal "X" (symbol->string 'x) "symbol->string: single char")
(assert-equal "HELLO-WORLD" (symbol->string 'hello-world)
  "symbol->string: hyphenated")

(assert-equal 'hello (string->symbol "hello") "string->symbol: hello")
(assert-equal 'x (string->symbol "x") "string->symbol: single")
(assert-equal 'hello-world (string->symbol "hello-world")
  "string->symbol: hyphenated")

;; Round trip
(assert-equal 'my-symbol
  (string->symbol (symbol->string 'my-symbol))
  "symbol->string->symbol round trip")

(end-suite)

(start-suite "string / display / newline")

;; string function concatenates string representations
(assert-equal "" (string) "string: no args = empty")
(assert-equal "42" (string 42) "string: number to string")
(assert-equal "hello" (string "hello") "string: string passthrough")
(assert-equal "42hello" (string 42 "hello") "string: mixed args")

(assert-equal "SYMBOL" (string 'symbol) "string: symbol to string")
(assert-equal "#t" (string #t) "string: boolean to string")

(end-suite)

(start-suite "type-of")

(assert-equal 'integer (type-of 42) "type: number")
(assert-equal 'string (type-of "hi") "type: string")
(assert-equal 'symbol (type-of 'x) "type: symbol")
(assert-equal 'boolean (type-of #t) "type: boolean true")
(assert-equal 'boolean (type-of #f) "type: boolean false")
(assert-equal 'null (type-of '()) "type: nil")
(assert-equal 'pair (type-of '(1 2)) "type: pair")
(assert-equal 'procedure (type-of +) "type: primitive procedure")
(assert-equal 'procedure (type-of (lambda (x) x)) "type: lambda/procedure")
(define-macro (test-macro-type) '())
(assert-equal 'macro (type-of test-macro-type) "type: macro")

(end-suite)

(start-suite "gensym")

;; Basic gensym functionality
(define gs1 (gensym))
(define gs2 (gensym))
(assert-true (symbol? gs1) "gensym: produces symbol")
(assert-false (eq? gs1 gs2) "gensym: unique each call")

;; gensym with prefix
(define gs3 (gensym "VAR"))
(define gs-str (symbol->string gs3))
;; The symbol string should contain "VAR"
(assert-true (> (string-length gs-str) 0) "gensym: returns symbol")

;; gensym symbols can be used as bindings
(define gs4 (gensym))
(eval `(define ,gs4 99))
(assert-equal 99 (eval gs4) "gensym: can be used as variable name")

(end-suite)

(start-suite "eval / apply")

;; eval: evaluate a quoted expression
(assert-equal 6 (eval '(+ 1 2 3)) "eval: basic eval")
(assert-equal 42 (eval '(* 6 7)) "eval: multiplication")

;; apply: apply function to arguments
(assert-equal 10 (apply + '(3 7)) "apply: + to list")
(assert-equal 15 (apply * '(3 5)) "apply: * to list")
(assert-equal '(1 2 3) (apply list '(1 2 3)) "apply: list to args")
(assert-equal '(3 5 7) (mapcar (lambda (x) (+ x 2)) '(1 3 5)) "apply: mapcar with lambda")

;; eval with environment
(define env-test-var 42)
(assert-equal 42 (eval 'env-test-var) "eval: lookup symbol in environment")

(end-suite)

(start-suite "Literal Edge Cases")

;; Very large numbers
(assert-equal 1000000 (* 1000 1000) "large multiplication")
(assert-equal 100000000 (* 10000 10000) "larger multiplication")

;; Zero
(assert-equal 0 (+ 0 0) "0+0=0")
(assert-equal 0 (* 0 100) "0*100=0")
(assert-equal 1 (* 1 1 1) "1*1*1=1")

;; Order of operations (Lisp is prefix, so all explicit)
(assert-equal 14 (+ (* 2 3) (* 4 2)) "compound: 2*3 + 4*2 = 14")
(assert-equal 71 (+ 1 (* 2 (+ 3 4) 5)) "complex nesting")

;; Empty list operations
(assert-equal '() (cdr '(1)) "cdr of single = ()")
(assert-equal '() (cdr '(())) "cdr of (()) = ()")

;; Single element list
(assert-equal '(1) (list 1) "list: single = (1)")
(assert-true (pair? '(1)) "pair?: single element list")

;; Nested empty lists
(assert-equal '(()) (list '()) "list: (())")
(assert-equal '(() ()) (list '() '()) "list: (() ())")
(assert-true (null? (car '(()))) "null?: car of (())")

(end-suite)

(test-summary)
