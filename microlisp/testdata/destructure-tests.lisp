;; ============================================================
;; Destructuring Tests - derived from SBCL destructure.pure.lisp
;; ============================================================

(load "tests/framework.lisp")
(start-suite "Destructuring Tests")

;; ============================================================
;; Basic Destructuring Bind
;; ============================================================
(start-suite "Basic Destructuring")

;; Simple list destructuring
(define (test-destructure-simple lst)
  (destructuring-bind (a b c) lst
    (list a b c)))

(assert-equal '(1 2 3) (test-destructure-simple '(1 2 3)) "destructuring: three elements")
(assert-equal '(1 2 ()) (test-destructure-simple '(1 2)) "destructuring: two elements (c bound to nil)")

(end-suite)

;; ============================================================
;; Nested Destructuring
;; ============================================================
(start-suite "Nested Destructuring")

(define (test-nested-destructure lst)
  (destructuring-bind ((a b) (c d)) lst
    (list a b c d)))

(assert-equal '(1 2 3 4)
  (test-nested-destructure '((1 2) (3 4)))
  "nested destructuring")

(end-suite)

;; ============================================================
;; Destructuring with Rest
;; ============================================================
(start-suite "Destructuring with Rest")

(define (test-destructure-rest lst)
  (destructuring-bind (a b . rest) lst
    rest))

(assert-equal '(3 4 5)
  (test-destructure-rest '(1 2 3 4 5))
  "destructuring with rest")
(assert-equal '()
  (test-destructure-rest '(1 2))
  "destructuring with rest - empty")

(end-suite)

;; ============================================================
;; Key Destructuring - removed (not supported in MicroLisp)
;; ============================================================

;; ============================================================
;; Optional Destructuring - removed (not supported in MicroLisp)
;; ============================================================

(test-summary)
