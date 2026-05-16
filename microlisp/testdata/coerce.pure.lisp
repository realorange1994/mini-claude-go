;; ============================================================
;; Coerce Type Conversion Tests - simplified for MicroLisp
;; ============================================================

(load "tests/framework.lisp")

;; ============================================================
;; Basic Coerce Operations
;; ============================================================
(start-suite "coerce-basic")

;; Coerce integer to float
(assert-equal 1.0 (coerce 1 'float) "coerce: int to float")
(assert-true (= 0.5 (coerce 1/2 'float)) "coerce: ratio to float")

(end-suite)

;; ============================================================
;; Symbol to Function Coercion
;; ============================================================
(start-suite "symbol-to-function")

;; Built-in operators work as functions
(assert-true (procedure? +) "+ is a procedure")
(assert-true (procedure? -) "- is a procedure")
(assert-true (procedure? *) "* is a procedure")
(assert-true (procedure? /) "/ is a procedure")
(assert-true (procedure? <) "< is a procedure")
(assert-true (procedure? =) "= is a procedure")

(end-suite)

;; ============================================================
;; Type Predicates
;; ============================================================
(start-suite "type-predicates")

(assert-true (number? 42) "number?: integer")
(assert-true (number? 3.14) "number?: float")
(assert-false (number? "hi") "number?: string not number")
(assert-false (number? 'x) "number?: symbol not number")

(assert-true (integerp 42) "integerp: integer")
(assert-true (integerp -5) "integerp: negative integer")
(assert-true (integerp 3.0) "integerp: 3.0 is integer")

(end-suite)

(test-summary)
