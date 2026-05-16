;; ============================================================
;; Function Application Edge Cases - simplified for MicroLisp
;; ============================================================

(load "tests/framework.lisp")

;; ============================================================
;; Function Application Basics
;; ============================================================
(start-suite "funcall-basics")

(define (add-two x) (+ x 2))
(assert-equal 5 (funcall add-two 3) "funcall: calls function")
(assert-equal 7 (funcall (lambda (x) (+ x 4)) 3) "funcall: lambda")

(end-suite)

;; ============================================================
;; Apply Basics
;; ============================================================
(start-suite "apply-basics")

(assert-equal 6 (apply + '(1 2 3)) "apply: with list")
(assert-equal 10 (apply + 1 '(2 3 4)) "apply: with extra args")
(assert-equal 7 (apply (lambda (x y z) (+ x y z)) '(1 2 4)) "apply: lambda")

(end-suite)

;; ============================================================
;; Required/Optional Arguments
;; ============================================================
(start-suite "optional-args")

(define (opt-func a . rest) (list a rest))
(assert-equal '(1 (2 3)) (opt-func 1 2 3) "optional: rest args")

(end-suite)

;; ============================================================
;; Closures
;; ============================================================
(start-suite "closure-basics")

(define (make-adder n) (lambda (x) (+ x n)))
(define add5 (make-adder 5))
(assert-equal 10 (add5 5) "closure: captured variable")
(assert-equal 15 (add5 10) "closure: same function")

(end-suite)

;; ============================================================
;; Tail Call Optimization
;; ============================================================
(start-suite "tco")

(define (loop n acc)
  (if (= n 0)
      acc
      (loop (- n 1) (+ acc 1))))
(assert-equal 1000 (loop 1000 0) "tco: tail recursive call")

(end-suite)

;; ============================================================
;; Summary
;; ============================================================
(test-summary)
