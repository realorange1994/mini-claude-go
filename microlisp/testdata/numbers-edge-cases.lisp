;; ============================================================
;; Numeric Edge Cases Tests - derived from SBCL arith.pure.lisp
;; ============================================================

(load "tests/framework.lisp")

;; ============================================================
;; Integer Arithmetic Edge Cases
;; ============================================================
(start-suite "integer-arithmetic-basic")

;; GCD/LCM basic tests
(assert-equal 0 (gcd) "gcd: no args returns 0")
(assert-equal 4 (gcd 8 12) "gcd: basic")
(assert-equal 4 (gcd 0 4) "gcd: with zero")
(assert-equal 4 (gcd 4 0) "gcd: zero first")

;; MOD basic tests
(assert-equal 1 (mod 7 3) "mod: positive")
(assert-equal 2 (mod -7 3) "mod: negative dividend")

;; REM basic tests
(assert-equal 1 (rem 7 3) "rem: positive")
(assert-equal -1 (rem -7 3) "rem: negative dividend")

(end-suite)

;; ============================================================
;; Float Basic Tests
;; ============================================================
(start-suite "float-basic")

;; Basic float operations
(assert-equal 2.5 (+ 1.5 1.0) "float: addition")
(assert-equal 3.0 (* 1.5 2.0) "float: multiplication")
(assert-equal 0.5 (/ 1.0 2.0) "float: division")

;; sqrt edge cases
(assert-equal 2.0 (sqrt 4.0) "sqrt: sqrt(4) = 2")
(assert-equal 3.0 (sqrt 9.0) "sqrt: sqrt(9) = 3")
(assert-equal 0.0 (sqrt 0.0) "sqrt: sqrt(0) = 0")

;; exp/log basic
(assert-equal 1.0 (exp 0.0) "exp: e^0 = 1")
(assert-equal 0.0 (log 1.0) "log: ln(1) = 0")

;; expt basic
(assert-equal 8.0 (expt 2.0 3.0) "expt: 2^3 = 8")
(assert-equal 1.0 (expt 5.0 0.0) "expt: x^0 = 1")
(assert-equal 4.0 (expt 2.0 2.0) "expt: 2^2 = 4")

(end-suite)

;; ============================================================
;; Trigonometric Basic Tests
;; ============================================================
(start-suite "trigonometry-basic")

;; sin edge cases
(assert-equal 0.0 (sin 0.0) "sin: sin(0) = 0")

;; cos edge cases
(assert-equal 1.0 (cos 0.0) "cos: cos(0) = 1")

;; tan edge cases
(assert-equal 0.0 (tan 0.0) "tan: tan(0) = 0")

;; atan edge cases
(assert-equal 0.0 (atan 0.0) "atan: atan(0) = 0")

(end-suite)

;; ============================================================
;; Comparison Basic Tests
;; ============================================================
(start-suite "comparison-basic")

;; Equality
(assert-true (= 5 5) "=: equal numbers")
(assert-false (= 5 6) "=: unequal")
(assert-true (= 5 5.0) "=: int = float")
(assert-true (= 0.0 -0.0) "=: 0.0 = -0.0")

;; Ordering
(assert-true (< 3 5) "<: less than")
(assert-true (> 5 3) ">: greater than")
(assert-true (<= 3 5) "<=: less or equal")
(assert-true (<= 5 5) "<=: equal")
(assert-true (>= 5 3) ">=: greater or equal")
(assert-true (>= 5 5) ">=: equal")

;; Mixed types
(assert-true (< 3 5.0) "<: int < float")
(assert-true (= 5 5.0) "=: int = float")

(end-suite)

;; ============================================================
;; Number Predicates Basic Tests
;; ============================================================
(start-suite "number-predicates-basic")

(assert-true (even? 4) "even?: 4")
(assert-false (even? 3) "even?: 3")
(assert-true (even? 0) "even?: 0")

(assert-true (odd? 3) "odd?: 3")
(assert-false (odd? 4) "odd?: 4")
(assert-false (odd? 0) "odd?: 0")

(assert-true (zero? 0) "zero?: 0")
(assert-false (zero? 1) "zero?: 1")
(assert-true (zero? 0.0) "zero?: 0.0")

(assert-true (positive? 5) "positive?: 5")
(assert-false (positive? -5) "positive?: -5")
(assert-false (positive? 0) "positive?: 0")

(assert-true (negative? -5) "negative?: -5")
(assert-false (negative? 5) "negative?: 5")
(assert-false (negative? 0) "negative?: 0")

(end-suite)

;; ============================================================
;; Floor/Ceiling/Round/Truncate Basic Tests
;; ============================================================
(start-suite "floor-ceiling-basic")

;; floor
(assert-equal 5 (car (floor 5.7)) "floor: 5.7 -> 5")
(assert-equal -6 (car (floor -5.7)) "floor: -5.7 -> -6")

;; ceiling
(assert-equal 6 (car (ceiling 5.2)) "ceiling: 5.2 -> 6")
(assert-equal -5 (car (ceiling -5.2)) "ceiling: -5.2 -> -5")

;; truncate
(assert-equal 5 (car (truncate 5.7)) "truncate: 5.7 -> 5")
(assert-equal -5 (car (truncate -5.7)) "truncate: -5.7 -> -5")

;; round
(assert-equal 6 (car (round 5.5)) "round: 5.5 -> 6")
(assert-equal 6 (car (round 5.7)) "round: 5.7 -> 6")

(end-suite)

;; ============================================================
;; Min/Max Basic Tests
;; ============================================================
(start-suite "min-max-basic")

(assert-equal 1 (min 1 2 3) "min: of 1, 2, 3")
(assert-equal 3 (max 1 2 3) "max: of 1, 2, 3")
(assert-equal -5 (min 1 -5 3) "min: with negative")
(assert-equal 10 (max 10 -5 3) "max: with negative")
(assert-equal 5 (min 5) "min: single arg")
(assert-equal 5 (max 5) "max: single arg")

(end-suite)

;; ============================================================
;; Abs/Signum Basic Tests
;; ============================================================
(start-suite "abs-signum-basic")

(assert-equal 5 (abs -5) "abs: |-5| = 5")
(assert-equal 5.0 (abs -5.0) "abs: float |-5.0| = 5.0")
(assert-equal 0 (abs 0) "abs: |0| = 0")
(assert-equal 0.0 (abs 0.0) "abs: float |0.0| = 0.0")

(assert-equal 1 (signum 5) "signum: signum(5) = 1")
(assert-equal -1 (signum -5) "signum: signum(-5) = -1")
(assert-equal 0 (signum 0) "signum: signum(0) = 0")

(end-suite)

;; ============================================================
;; Bitwise Basic Operations
;; ============================================================
(start-suite "bitwise-basic")

;; logand
(assert-equal 0 (logand 1 2) "logand: 1 & 2 = 0")
(assert-equal 1 (logand 1 3) "logand: 1 & 3 = 1")

;; logior
(assert-equal 3 (logior 1 2) "logior: 1 | 2 = 3")
(assert-equal 1 (logior 1 0) "logior: 1 | 0 = 1")

;; logxor
(assert-equal 3 (logxor 1 2) "logxor: 1 ^ 2 = 3")
(assert-equal 0 (logxor 1 1) "logxor: 1 ^ 1 = 0")
(assert-equal 0 (logxor) "logxor: no args = 0")

;; lognot
(assert-equal -2 (lognot 1) "lognot: ~1 = -2")
(assert-equal -1 (lognot 0) "lognot: ~0 = -1")

(end-suite)

;; ============================================================
;; Summary
;; ============================================================
(test-summary)
