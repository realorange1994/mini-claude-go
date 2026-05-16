;; math-edge-cases.lisp — tests for math edge cases
;; Covers: round, floor, ceiling, truncate, signum, abs, evenp, oddp,
;;         zerop, plusp, minusp, gcd, lcm, max, min (variadic)

(load "tests/framework.lisp")
(start-suite "Math Edge Cases")

;; --- round ---
(define r (round 7 3))
(assert-equal 2 (car r) "round 7 3 quotient = 2")
(assert-equal 1 (cadr r) "round 7 3 remainder = 1")

(define r2 (round 10 2))
(assert-equal 5 (car r2) "round 10 2 quotient = 5")
(assert-equal 0 (cadr r2) "round 10 2 remainder = 0")

(define r3 (round -7 3))
(assert-equal -2 (car r3) "round -7 3 quotient = -2")

(define r4 (round 7.5))
(assert-equal 8 (car r4) "round 7.5 = 8 (rounds to even)")

;; --- floor ---
(define f (floor 7 3))
(assert-equal 2 (car f) "floor 7 3 = 2")
(assert-equal 1 (cadr f) "floor 7 3 remainder = 1")

(define f2 (floor -7 3))
(assert-equal -3 (car f2) "floor -7 3 = -3")

(define f3 (floor 3.5))
(assert-equal 3 (car f3) "floor 3.5 = 3")

(define f4 (floor 7 3))
(assert-equal 2 (car f4) "floor 7 3 quotient = 2")

;; --- ceiling ---
(define c (ceiling 7 3))
(assert-equal 3 (car c) "ceiling 7 3 = 3")
(assert-equal -2 (cadr c) "ceiling 7 3 remainder = -2")

(define c2 (ceiling 7 7))
(assert-equal 1 (car c2) "ceiling 7 7 = 1")

(define c3 (ceiling 3.2))
(assert-equal 4 (car c3) "ceiling 3.2 = 4")

;; --- truncate ---
(define t (truncate 7 3))
(assert-equal 2 (car t) "truncate 7 3 = 2")
(assert-equal 1 (cadr t) "truncate 7 3 remainder = 1")

(define t2 (truncate -7 3))
(assert-equal -2 (car t2) "truncate -7 3 = -2")

(define t3 (truncate 3.9))
(assert-equal 3 (car t3) "truncate 3.9 = 3")

(define t4 (truncate 10 2))
(assert-equal 5 (car t4) "truncate 10 2 = 5")

;; --- signum ---
(assert-equal 1 (signum 5) "signum 5 = 1")
(assert-equal -1 (signum -5) "signum -5 = -1")
(assert-equal 0 (signum 0) "signum 0 = 0")
(assert-equal 1 (signum 0.5) "signum 0.5 = 1")
(assert-equal -1 (signum -0.5) "signum -0.5 = -1")
(assert-equal 1 (signum 1000000) "signum large positive = 1")
(assert-equal -1 (signum -1000000) "signum large negative = -1")

;; --- abs ---
(assert-equal 5 (abs 5) "abs 5 = 5")
(assert-equal 5 (abs -5) "abs -5 = 5")
(assert-equal 0 (abs 0) "abs 0 = 0")
(assert-equal 3.5 (abs 3.5) "abs 3.5 = 3.5")
(assert-equal 3.5 (abs -3.5) "abs -3.5 = 3.5")

;; --- evenp ---
(assert-true (evenp 0) "evenp 0 = true")
(assert-true (evenp 2) "evenp 2 = true")
(assert-true (evenp -2) "evenp -2 = true")
(assert-true (evenp 4) "evenp 4 = true")
(assert-false (evenp 1) "evenp 1 = false")
(assert-false (evenp 3) "evenp 3 = false")
(assert-false (evenp -1) "evenp -1 = false")
(assert-false (evenp 5) "evenp 5 = false")

;; --- oddp ---
(assert-true (oddp 1) "oddp 1 = true")
(assert-true (oddp 3) "oddp 3 = true")
(assert-true (oddp -1) "oddp -1 = true")
(assert-false (oddp 0) "oddp 0 = false")
(assert-false (oddp 2) "oddp 2 = false")
(assert-false (oddp -2) "oddp -2 = false")
(assert-false (oddp 4) "oddp 4 = false")

;; --- zerop ---
(assert-true (zerop 0) "zerop 0 = true")
(assert-false (zerop 1) "zerop 1 = false")
(assert-false (zerop -1) "zerop -1 = false")
(assert-true (zerop 0.0) "zerop 0.0 = true")

;; --- plusp ---
(assert-true (plusp 1) "plusp 1 = true")
(assert-true (plusp 0.5) "plusp 0.5 = true")
(assert-false (plusp 0) "plusp 0 = false")
(assert-false (plusp -1) "plusp -1 = false")

;; --- minusp ---
(assert-false (minusp 1) "minusp 1 = false")
(assert-true (minusp -1) "minusp -1 = true")
(assert-false (minusp 0) "minusp 0 = false")
(assert-true (minusp -0.5) "minusp -0.5 = true")

;; --- gcd ---
(assert-equal 4 (gcd 12 8) "gcd 12 8 = 4")
(assert-equal 6 (gcd 12 18) "gcd 12 18 = 6")
(assert-equal 1 (gcd 13 17) "gcd 13 17 = 1")
(assert-equal 7 (gcd 0 7) "gcd 0 7 = 7")
(assert-equal 7 (gcd 7 0) "gcd 7 0 = 7")
(assert-equal 10 (gcd 10 20 30) "gcd 10 20 30 = 10")

;; --- lcm ---
(assert-equal 12 (lcm 4 6) "lcm 4 6 = 12")
(assert-equal 6 (lcm 2 3) "lcm 2 3 = 6")
(assert-equal 10 (lcm 10 5) "lcm 10 5 = 10")
(assert-equal 1 (lcm 1 1) "lcm 1 1 = 1")

;; --- max (variadic) ---
(assert-equal 5 (max 1 5 3) "max 1 5 3 = 5")
(assert-equal 10 (max 10) "max 10 = 10")
(assert-equal 3 (max 1 2 3) "max 1 2 3 = 3")
(assert-equal 0 (max 0 -1 -2) "max 0 -1 -2 = 0")
(assert-equal -1 (max -5 -1 -3) "max -5 -1 -3 = -1")

;; --- min (variadic) ---
(assert-equal 1 (min 1 5 3) "min 1 5 3 = 1")
(assert-equal 10 (min 10) "min 10 = 10")
(assert-equal 1 (min 1 2 3) "min 1 2 3 = 1")
(assert-equal -5 (min 0 -1 -5) "min 0 -1 -5 = -5")
(assert-equal -5 (min -5 -1 -3) "min -5 -1 -3 = -5")

(end-suite)
(test-summary)
