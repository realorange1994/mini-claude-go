;; ============================================================
;; Advanced Test: Numeric Tower (Rationals, Complexes, Reader Macros)
;;
;; Tests:
;;   1. Rational arithmetic and simplification
;;   2. Complex arithmetic and zero-simplification
;;   3. Mixed-type operations (promotion rules)
;;   4. Reader radix macros (#b, #o, #x)
;;   5. Edge cases: negative rationals, complex with rational parts
;; ============================================================

(load "tests/framework.lisp")

(start-suite "Rational Basic Operations")

;; Basic rational creation via reader syntax
(assert-equal 'rational (type-of 1/2) "type: 1/2 is rational")
(assert-equal 'rational (type-of 3/4) "type: 3/4 is rational")
(assert-equal 'rational (type-of -2/3) "type: -2/3 is rational")

;; Integer results simplify back to number
(assert-equal 'integer (type-of 4/2) "type: 4/2 simplifies to integer")
(assert-equal 2 4/2 "4/2 = 2")

;; Simplification
(assert-equal 1/2 2/4 "2/4 = 1/2")
(assert-equal 2/3 4/6 "4/6 = 2/3")

(end-suite)

(start-suite "Rational Arithmetic")

;; Addition
(assert-equal 3/4 (+ 1/4 1/2) "1/4 + 1/2 = 3/4")
(assert-equal 1 (+ 1/3 2/3) "1/3 + 2/3 = 1")
(assert-equal 5/6 (+ 1/2 1/3) "1/2 + 1/3 = 5/6")

;; Subtraction
(assert-equal 1/4 (- 1/2 1/4) "1/2 - 1/4 = 1/4")
(assert-equal 1/6 (- 1/2 1/3) "1/2 - 1/3 = 1/6")

;; Multiplication
(assert-equal 1/6 (* 1/2 1/3) "1/2 * 1/3 = 1/6")
(assert-equal 3/8 (* 3/4 1/2) "3/4 * 1/2 = 3/8")

;; Division
(assert-equal 3/2 (/ 3/4 1/2) "(3/4) / (1/2) = 3/2")
(assert-equal 2 (/ 1/2 1/4) "(1/2) / (1/4) = 2")

;; Mixed rational and integer (VNum promoted to rational when integer-valued)
(assert-equal 3/2 (+ 1 1/2) "1 + 1/2 = 3/2")
(assert-equal 1/2 (- 1 1/2) "1 - 1/2 = 1/2")
(assert-equal 3/2 (* 3 1/2) "3 * 1/2 = 3/2")

;; Negative rationals
(assert-equal -1/2 (- 0 1/2) "0 - 1/2 = -1/2")
(assert-equal -1/2 (- 1/2 1) "1/2 - 1 = -1/2")
(assert-equal -1/6 (* -1/2 1/3) "-1/2 * 1/3 = -1/6")
(assert-equal -3/2 (/ -3/4 1/2) "(-3/4) / (1/2) = -3/2")

;; Denominator normalization
(assert-true (= -0.5 (/ 1 -2)) "1 / -2 = -0.5 (VNum division produces float)")
(assert-true (= 0.5 (/ -1 -2)) "-1 / -2 = 0.5 (VNum division produces float)")

(end-suite)

(start-suite "Complex Basic Operations")

;; Complex creation
(assert-equal 'complex (type-of #c(1 2)) "type: #c(1 2) is complex")
(assert-equal 'complex (type-of #c(0 1)) "type: #c(0 1) is complex")

;; Zero imaginary part simplifies to real
(assert-equal 'integer (type-of #c(5 0)) "type: #c(5 0) simplifies to integer")
(assert-equal 5 #c(5 0) "#c(5 0) = 5")

;; Cross-type numeric equality via =
(assert-true (= 5 #c(5 0)) "#c(5 0) numerically = 5")
(assert-true (= 1.5 3/2) "1.5 = 3/2")
(assert-true (= 0.5 1/2) "0.5 = 1/2")

(end-suite)

(start-suite "Complex Arithmetic")

;; Addition
(assert-equal #c(4 6) (+ #c(1 2) #c(3 4)) "#c(1 2) + #c(3 4) = #c(4 6)")
(assert-equal #c(3 2) (+ #c(1 2) 2) "#c(1 2) + 2 = #c(3 2)")
(assert-equal #c(1.5 2) (+ #c(1 2) 1/2) "#c(1 2) + 1/2 = #c(1.5 2)")

;; Subtraction
(assert-equal #c(-2 -2) (- #c(1 2) #c(3 4)) "#c(1 2) - #c(3 4) = #c(-2 -2)")
(assert-equal #c(-1 2) (- #c(1 2) 2) "#c(1 2) - 2 = #c(-1 2)")

;; Multiplication
(assert-equal #c(-5 10) (* #c(1 2) #c(3 4)) "#c(1 2) * #c(3 4) = #c(-5 10)")

;; Division
(assert-true (= 0.5 (/ #c(1 0) #c(2 0))) "#c(1 0) / #c(2 0) = 0.5 numerically")

(end-suite)

(start-suite "Mixed Numeric Type Operations")

;; VNum + VRat: VNum is integer-valued, promoted to rational
(assert-equal 3/2 (+ 1 1/2) "1 + 1/2 = 3/2")
(assert-equal 5/2 (+ 1/2 2) "1/2 + 2 = 5/2")

;; Cross-type numeric comparisons
(assert-true (= 1.5 (+ 1 1/2)) "numerically: 1 + 1/2 = 1.5")
(assert-true (= 2.5 (+ 1/2 2)) "numerically: 1/2 + 2 = 2.5")

;; float + complex
(assert-equal #c(3.5 2) (+ #c(1 2) 2.5) "#c(1 2) + 2.5 = #c(3.5 2)")

(end-suite)

(start-suite "Reader Radix Macros")

;; Binary
(assert-equal 5 #b101 "#b101 = 5")
(assert-equal 255 #b11111111 "#b11111111 = 255")
(assert-equal 0 #b0 "#b0 = 0")

;; Octal
(assert-equal 8 #o10 "#o10 = 8")
(assert-equal 511 #o777 "#o777 = 511")
(assert-equal 0 #o0 "#o0 = 0")

;; Hexadecimal
(assert-equal 255 #xFF "#xFF = 255")
(assert-equal 4095 #xFFF "#xFFF = 4095")
(assert-equal 16 #x10 "#x10 = 16")
(assert-equal 0 #x0 "#x0 = 0")
(assert-equal 3735928559 #xDEADBEEF "#xDEADBEEF = 3735928559")

;; Type is number (not rational)
(assert-equal 'integer (type-of #b101) "type: #b101 is integer")
(assert-equal 'integer (type-of #xFF) "type: #xFF is integer")

(end-suite)

(start-suite "Type Predicates with New Types")

;; type-of returns correct type names
(assert-equal 'rational (type-of 1/3) "type: rational")
(assert-equal 'rational (type-of -7/8) "type: negative rational")
(assert-equal 'complex (type-of #c(0 1)) "type: complex")
(assert-equal 'complex (type-of #c(3.5 -2.1)) "type: complex with decimals")

(end-suite)

(start-suite "Complex Edge Cases")

;; Complex with zero imaginary
(assert-equal 3 #c(3 0) "#c(3 0) = 3")
(assert-equal -2 #c(-2 0) "#c(-2 0) = -2")

;; Complex negation
(assert-equal #c(-1 -2) (- #c(1 2)) "-(#c(1 2)) = #c(-1 -2)")

;; Complex unary minus on real
(assert-equal -3 (- #c(3 0)) "-(#c(3 0)) = -3")

;; Complex with rational parts
(assert-true (= 0.5 (/ 1 2)) "1/2 = 0.5 numerically")

(end-suite)

(start-suite "Rational Comparison")

(assert-true (= 1/2 2/4) "1/2 = 2/4")
(assert-false (= 1/2 2/3) "1/2 != 2/3")
(assert-true (< 1/2 3/4) "1/2 < 3/4")
(assert-true (> 3/4 1/2) "3/4 > 1/2")
(assert-true (<= 1/2 2/4 3/4) "1/2 <= 2/4 <= 3/4")
(assert-true (>= 3/4 2/4 1/2) "3/4 >= 2/4 >= 1/2")

;; Mixed rational and integer comparison
(assert-true (= 1 2/2) "1 = 2/2")
(assert-true (< 1/2 1) "1/2 < 1")
(assert-true (> 3/2 1) "3/2 > 1")

;; Mixed rational and float comparison
(assert-true (= 0.5 1/2) "0.5 = 1/2")
(assert-true (= 0.75 3/4) "0.75 = 3/4")

(end-suite)

(test-summary)
