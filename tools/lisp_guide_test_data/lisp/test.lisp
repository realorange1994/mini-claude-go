;; Test file for lisp_guide testing

(start-suite "Basic Tests")
(assert-equal 3 (+ 1 2))

(define (square x) (* x x))
(define (cube x) (* x x x))

(defmacro (my-when condition &rest body)
  (list 'if condition (cons 'progn body)))

(start-suite "Advanced Tests")
(assert-equal 9 (square 3))

(end-suite)
