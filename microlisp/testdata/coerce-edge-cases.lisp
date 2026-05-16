;; ============================================================
;; Type Coercion Edge Cases - derived from SBCL coerce.pure.lisp
;; ============================================================

(load "tests/framework.lisp")

;; ============================================================
;; Basic COERCE Tests
;; ============================================================
(start-suite "coerce-basics")

;; Integer to integer
(assert-equal 1 (coerce 1 'integer) "coerce: integer to integer")
(assert-equal 42 (coerce 42 'integer) "coerce: integer to integer same")

;; Integer to float
(assert-equal 1.0 (coerce 1 'float) "coerce: integer to float")
(assert-equal 3.14 (coerce 3.14 'float) "coerce: float to float")

;; Integer to complex
(assert-true (= 1 (coerce 1 'complex)) "coerce: integer to complex simplifies")
(assert-true (= 1.0 (coerce 1.0 '(complex float))) "coerce: float to complex float simplifies")

;; Rational to float
(assert-equal 0.5 (coerce 1/2 'float) "coerce: rational to float")

(end-suite)

;; ============================================================
;; COERCE to LIST
;; ============================================================
(start-suite "coerce-list")

(assert-equal '(1 2 3) (coerce '(1 2 3) 'list) "coerce: list to list")
(assert-equal '() (coerce '() 'list) "coerce: empty list")

(end-suite)

;; ============================================================
;; COERCE to SEQUENCE
;; ============================================================
(start-suite "coerce-sequence")

(assert-equal '(1 2 3) (coerce '(1 2 3) 'sequence) "coerce: list to sequence")
(assert-equal "abc" (coerce '(#\a #\b #\c) 'string) "coerce: char list to string")

(end-suite)

;; ============================================================
;; COERCE to CHARACTER
;; ============================================================
(start-suite "coerce-character")

(assert-equal #\a (coerce #\a 'character) "coerce: character to character")
(assert-equal #\x (coerce #\x 'character) "coerce: same character")

(end-suite)

;; ============================================================
;; COERCE to FUNCTION
;; ============================================================
(start-suite "coerce-function")

(define sym '+)
(assert-true (procedure? (coerce sym 'function)) "coerce: symbol to function")
(assert-equal 3 (funcall (coerce sym 'function) 1 2) "coerce: + function works")

(end-suite)

;; ============================================================
;; Complex Number Coercion
;; ============================================================
(start-suite "coerce-complex")

(assert-true (= 1 (coerce 1 'complex)) "coerce: int to complex simplifies")
(assert-true (= 1.5 (coerce 1.5 '(complex float))) "coerce: float to complex float numerically")
(assert-true (= 1/2 (coerce 1/2 '(complex rational))) "coerce: rational to complex rational numerically")

(end-suite)

;; ============================================================
;; Summary
;; ============================================================
(test-summary)
