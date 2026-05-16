;; ============================================================
;; Array Edge Cases - simplified for MicroLisp
;; ============================================================

(load "tests/framework.lisp")

;; ============================================================
;; Basic Array Creation
;; ============================================================
(start-suite "array-basics")

;; Test that make-array creates array with correct dimensions
(assert-equal '(3) (array-dimensions (make-array 3)) "make-array: correct dimensions")
(assert-equal 3 (array-total-size (make-array 3)) "make-array: correct total size")

(end-suite)

;; ============================================================
;; Array Initialization
;; ============================================================
(start-suite "array-initialization")

;; :initial-element
(define arr (make-array 3 :initial-element 42))
(assert-equal 42 (aref arr 0) "initial-element: first")
(assert-equal 42 (aref arr 1) "initial-element: second")
(assert-equal 42 (aref arr 2) "initial-element: third")

(end-suite)

;; ============================================================
;; Array AREF and SETF
;; ============================================================
(start-suite "array-access")

(define arr (make-array 5 :initial-contents '(10 20 30 40 50)))

(assert-equal 10 (aref arr 0) "aref: index 0")
(assert-equal 30 (aref arr 2) "aref: index 2")
(assert-equal 50 (aref arr 4) "aref: index 4")

;; setf aref
(setf (aref arr 1) 99)
(assert-equal 99 (aref arr 1) "setf aref: updated value")

(end-suite)

;; ============================================================
;; Summary
;; ============================================================
(test-summary)
