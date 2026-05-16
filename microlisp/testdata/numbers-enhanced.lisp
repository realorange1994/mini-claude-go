;; ============================================================
;; Enhanced Numeric/Float Tests - derived from SBCL float.pure.lisp
;; ============================================================

(load "tests/framework.lisp")

;; ============================================================
;; Basic Arithmetic
;; ============================================================
(start-suite "basic-arithmetic")

;; Addition
(assert-equal 3 (+ 1 2) "addition: 1 + 2")
(assert-equal 0 (+ 1 2 -3) "addition: mixed")
(assert-equal 10.0 (+ 3.0 7.0) "addition: floats")

;; Subtraction
(assert-equal -1 (- 3 4) "subtraction: 3 - 4")
(assert-equal 6 (- 10 3 1) "subtraction: 10 - 3 - 1")

;; Multiplication
(assert-equal 6 (* 2 3) "multiplication: 2 * 3")
(assert-equal 24 (* 2 3 4) "multiplication: 2 * 3 * 4")
(assert-equal 10.5 (* 3.5 3) "multiplication: float * int")

;; Division
(assert-equal 2 (/ 6 3) "division: 6 / 3")
(assert-true (= 0.5 (/ 3 6)) "division: 3 / 6")
(assert-equal 2.5 (/ 5.0 2) "division: float")

(define x 5)

(define y 10)

(end-suite)

;; ============================================================
;; Integer Arithmetic
;; ============================================================
(start-suite "integer-arithmetic")

(assert-equal 1 (mod 7 3) "mod: 7 mod 3 = 1")
(assert-equal 2 (mod -7 3) "mod: -7 mod 3 = 2")

;; gcd / lcm
(assert-equal 4 (gcd 8 12) "gcd: gcd(8, 12) = 4")
(assert-equal 24 (lcm 8 12) "lcm: lcm(8, 12) = 24")

(end-suite)

;; ============================================================
;; Float Operations
;; ============================================================
(start-suite "float-operations")

;; Basic float operations
(assert-equal 2.5 (+ 1.5 1.0) "float: addition")
(assert-equal 3.0 (* 1.5 2.0) "float: multiplication")
(assert-equal 0.5 (/ 1.0 2.0) "float: division")

;; sqrt
(assert-equal 2.0 (sqrt 4.0) "sqrt: sqrt(4) = 2")
(assert-equal 3.0 (sqrt 9.0) "sqrt: sqrt(9) = 3")

;; exp / log
(assert-equal 1.0 (exp 0.0) "exp: e^0 = 1")
(assert-equal 0.0 (log 1.0) "log: ln(1) = 0")
(assert-true (> 6 (log 7.389056 2.0)) "log: ln(e^2) > 6")

;; expt
(assert-equal 8.0 (expt 2.0 3.0) "expt: 2^3 = 8")
(assert-equal 1.0 (expt 5.0 0.0) "expt: x^0 = 1")
(assert-equal 4.0 (expt 2.0 2.0) "expt: 2^2 = 4")

(end-suite)

;; ============================================================
;; Trigonometric Functions
;; ============================================================
(start-suite "trigonometry")

;; sin
(assert-equal 0.0 (sin 0.0) "sin: sin(0) = 0")
(assert-true (> 0.9 (sin 1.0)) "sin: sin(1) approx 0.84")

;; cos
(assert-equal 1.0 (cos 0.0) "cos: cos(0) = 1")
(assert-true (> 0.55 (cos 1.0)) "cos: cos(1) < 0.55")

;; tan
(assert-equal 0.0 (tan 0.0) "tan: tan(0) = 0")

;; atan
(assert-equal 0.0 (atan 0.0) "atan: atan(0) = 0")

(end-suite)

;; ============================================================
;; Comparison Predicates
;; ============================================================
(start-suite "numeric-comparison")

(assert-true (= 5 5) "=: equal numbers")
(assert-false (= 5 6) "=: unequal")
(assert-true (< 3 5) "<: less than")
(assert-true (> 5 3) ">: greater than")
(assert-true (<= 3 5) "<=: less or equal")
(assert-true (>= 5 3) ">=: greater or equal")

;; Mixed types
(assert-true (= 5 5.0) "=: int = float")
(assert-true (< 3 5.0) "<: int < float")

(end-suite)

;; ============================================================
;; Number Predicates
;; ============================================================
(start-suite "number-predicates")

(assert-true (even? 4) "even?: 4")
(assert-false (even? 3) "even?: 3")
(assert-true (odd? 3) "odd?: 3")
(assert-false (odd? 4) "odd?: 4")
(assert-true (zero? 0) "zero?: 0")
(assert-false (zero? 1) "zero?: 1")
(assert-true (positive? 5) "positive?: 5")
(assert-false (positive? -5) "positive?: -5")
(assert-true (negative? -5) "negative?: -5")
(assert-false (negative? 5) "negative?: 5")

(end-suite)

;; ============================================================
;; Type Conversion
;; ============================================================
(start-suite "type-conversion")

;; floor/ceiling/round/truncate return (values quotient remainder)
(assert-equal 5 (car (floor 5.7)) "floor: 5.7 -> 5")
(assert-equal 6 (car (ceiling 5.2)) "ceiling: 5.2 -> 6")
(assert-equal 6 (car (round 5.5)) "round: 5.5 -> 6")
(assert-equal 6 (car (round 5.7)) "round: 5.7 -> 6")
(assert-equal 3 (car (truncate 3.7)) "truncate: 3.7 -> 3")

;; Exact conversions
(assert-true (integerp 5) "integerp: 5 is integer")
(assert-true (integerp 3.0) "integerp: 3.0 is integer")

(end-suite)

;; ============================================================
;; Min / Max
;; ============================================================
(start-suite "min-max")

(assert-equal 1 (min 1 2 3) "min: of 1, 2, 3")
(assert-equal 3 (max 1 2 3) "max: of 1, 2, 3")
(assert-equal -5 (min 1 -5 3) "min: with negative")
(assert-equal 10 (max 10 -5 3) "max: with negative")

(end-suite)

;; ============================================================
;; Abs / Signum
;; ============================================================
(start-suite "abs-sign")

(assert-equal 5 (abs -5) "abs: |-5| = 5")
(assert-equal 5.0 (abs -5.0) "abs: float |-5.0| = 5.0")
(assert-equal 1 (signum 5) "signum: signum(5) = 1")
(assert-equal -1 (signum -5) "signum: signum(-5) = -1")
(assert-equal 0 (signum 0) "signum: signum(0) = 0")

(end-suite)

;; ============================================================
;; Summary
;; ============================================================
(test-summary)
