;; ============================================================
;; Case Macro Tests - derived from SBCL case.pure.lisp
;; ============================================================

(load "tests/framework.lisp")
(start-suite "Case Macro Tests")

;; ============================================================
;; Basic CASE Tests
;; ============================================================
(start-suite "Basic Case")

;; Basic case with single else clause
(assert-equal 'thing
  (case 'a
    (else 'thing))
  "case: else clause always matches")

;; Case with multiple keys
(assert-equal 'two
  (case 2
    (1 'one)
    (2 'two)
    (3 'three))
  "case: match second key")

;; Case with no match returns nil
(assert-equal '()
  (case 0
    (1 'one)
    (2 'two))
  "case: no match returns nil")

;; Case with else clause
(assert-equal 'default
  (case 'other
    (a 'apple)
    (b 'banana)
    (else 'default))
  "case: else clause")

(end-suite)

;; ============================================================
;; Type CASE Tests (etypecase)
;; ============================================================
(start-suite "Type Case")

;; etypecase requires a match
(define (test-etypecase x)
  (etypecase x
    (number  'is-number)
    (symbol  'is-symbol)
    (string  'is-string)))

(assert-equal 'is-number (test-etypecase 42) "etypecase: number")
(assert-equal 'is-symbol (test-etypecase 'hello) "etypecase: symbol")
(assert-equal 'is-string (test-etypecase "world") "etypecase: string")

(end-suite)

;; ============================================================
;; Typecase Tests
;; ============================================================
(start-suite "Typecase")

(define (test-typecase x)
  (typecase x
    (number 'number-type)
    (symbol 'symbol-type)
    (string 'string-type)
    (t 'other-type)))

(assert-equal 'number-type (test-typecase 123) "typecase: number")
(assert-equal 'symbol-type (test-typecase 'foo) "typecase: symbol")
(assert-equal 'string-type (test-typecase "bar") "typecase: string")
(assert-equal 'other-type (test-typecase '(a b)) "typecase: list falls through to t")

(end-suite)

;; ============================================================
;; Key tests
;; ============================================================
(start-suite "Key Tests")

;; Multiple values in key
(assert-equal 'match
  (case 1
    ((1 2 3) 'match)
    (4 'other))
  "case: key list matches")

;; Otherwise/else as key
(assert-equal 'result
  (case 'otherwise
    (otherwise 'result)
    (t 'other))
  "case: otherwise as key")

(end-suite)

(test-summary)
