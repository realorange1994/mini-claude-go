;; ============================================================
;; FFI (Foreign Function Interface) Tests
;; ============================================================

(load "tests/framework.lisp")
(start-suite "FFI Math Functions")

;; Basic math FFI
(assert-equal 12 (ffi "math/sqrt" 144) "FFI: sqrt(144) = 12")
(assert-equal 5 (ffi "math/sqrt" 25) "FFI: sqrt(25) = 5")
(assert-equal 1 (ffi "math/sin" 1.5707963267948966) "FFI: sin(pi/2) ~= 1")

;; Could use approximate comparison:
(define (approx-equal a b tolerance)
  (< (abs (- a b)) tolerance))

(define sin-result (ffi "math/sin" 0))
(assert-equal 0 sin-result "FFI: sin(0) = 0")

(define cos-result (ffi "math/cos" 0))
(assert-equal 1 cos-result "FFI: cos(0) = 1")

(define sqrt-2 (ffi "math/sqrt" 2))
(assert-true (approx-equal 1.4142135623730951 sqrt-2 0.0001)
  "FFI: sqrt(2) ~= 1.4142")

;; More math functions
(assert-equal 3 (ffi "math/floor" 3.7) "FFI: floor(3.7) = 3")
(assert-equal 4 (ffi "math/ceil" 3.2) "FFI: ceil(3.2) = 4")
(assert-equal 4 (ffi "math/round" 3.6) "FFI: round(3.6) = 4")
(assert-equal 3 (ffi "math/round" 3.2) "FFI: round(3.2) = 3")

(assert-equal 5 (ffi "math/abs" -5) "FFI: abs(-5) = 5")
(assert-equal 5 (ffi "math/abs" 5) "FFI: abs(5) = 5")

(assert-true (approx-equal 8 (ffi "math/exp" 2.0794415416798357) 0.01)
  "FFI: exp(2.08) ~= 8")

;; pow is approximate
(define pow-result (ffi "math/pow" 2 10))
(assert-true (approx-equal 1024 pow-result 0.01)
  "FFI: pow(2, 10) = 1024")

(define pow2 (ffi "math/pow" 5 3))
(assert-true (approx-equal 125 pow2 0.01)
  "FFI: pow(5, 3) = 125")

(end-suite)

(start-suite "FFI OS Functions")

(define pid (ffi "os/getpid" ))
(assert-true (> pid 0) "FFI: getpid returns positive number")

(define home (ffi "os/getenv" "USERPROFILE"))
;; Just verify it's a string (may be empty if env var not set)
(assert-equal 'string (type-of home) "FFI: getenv returns string")

(end-suite)

(start-suite "FFI Register from Lisp")

(ffi-register "my-value" 42)
(assert-equal 42 (ffi "my-value") "FFI: registered value 42")

(ffi-register "my-string" "hello")
(assert-equal "hello" (ffi "my-string") "FFI: registered string 'hello'")

;; Test mathematical identities
(define (ffi-sin x) (ffi "math/sin" x))
(define (ffi-cos x) (ffi "math/cos" x))

;; sin²x + cos²x ≈ 1
(define (sin2-plus-cos2 x)
  (+ (* (ffi-sin x) (ffi-sin x))
     (* (ffi-cos x) (ffi-cos x))))

(assert-true (approx-equal 1.0 (sin2-plus-cos2 0) 0.0001)
  "FFI: sin²(0)+cos²(0)=1")
(assert-true (approx-equal 1.0 (sin2-plus-cos2 1) 0.0001)
  "FFI: sin²(1)+cos²(1)~=1")
(assert-true (approx-equal 1.0 (sin2-plus-cos2 3.14159) 0.0001)
  "FFI: sin²(pi)+cos²(pi)~=1")

(end-suite)

(start-suite "FFI Edge Cases")

;; FFI with multiple arguments
(define pow-ffi (ffi "math/pow" 2 8))
(assert-true (approx-equal 256 pow-ffi 0.01) "FFI: pow(2,8)=256")

;; FFI on integers vs floats
(assert-equal 3 (ffi "math/floor" 3) "FFI: floor(3) = 3")
(assert-equal 3 (ffi "math/ceil" 3) "FFI: ceil(3) = 3")
(assert-equal 3 (ffi "math/round" 3) "FFI: round(3) = 3")

(end-suite)

(test-summary)
