;; ============================================================
;; MicroLisp Test Framework
;; ============================================================
;; Simple test framework using basic Lisp forms.

(define *tests-run* 0)
(define *tests-passed* 0)
(define *tests-failed* 0)

(define (assert-equal expected actual msg)
  (set! *tests-run* (+ *tests-run* 1))
  (if (equal? expected actual)
    (begin
      (set! *tests-passed* (+ *tests-passed* 1))
      (display "  PASS: ")
      (display msg)
      (newline))
    (begin
      (set! *tests-failed* (+ *tests-failed* 1))
      (display "  FAIL: ")
      (display msg)
      (display " -- expected: ")
      (display expected)
      (display ", got: ")
      (display actual)
      (newline))))

(define (assert-true actual msg)
  (assert-equal #t actual msg))

(define (assert-false actual msg)
  (assert-equal #f actual msg))

(define (assert-nil actual msg)
  (assert-equal '() actual msg))

(define (assert-nil actual msg)
  (assert-equal '() actual msg))

(define (assert-type expected-type val msg)
  (assert-equal expected-type (type-of val) msg))

(define (assert-error form msg)
  (set! *tests-run* (+ *tests-run* 1))
  (handler-case
    (begin
      (handler-eval form)
      (set! *tests-failed* (+ *tests-failed* 1))
      (display "  FAIL: ")
      (display msg)
      (display " -- expected error, but succeeded")
      (newline))
    (condition (e)
      (set! *tests-passed* (+ *tests-passed* 1))
      (display "  PASS: ")
      (display msg)
      (newline))))

;; Convenience list accessors not built-in
(define (cadddr x) (car (cdddr x)))
(define (cdddr x) (cddr (cdr x)))

(define (start-suite name)
  (newline)
  (display "=== ")
  (display name)
  (display " ===")
  (newline))

(define (end-suite))

(define (test-summary)
  (newline)
  (display "==========================================")
  (newline)
  (display "Tests run:   ") (display *tests-run*) (newline)
  (display "Tests passed:") (display *tests-passed*) (newline)
  (display "Tests failed:") (display *tests-failed*) (newline)
  (display "==========================================")
  (newline)
  (if (= *tests-failed* 0)
    (display "ALL TESTS PASSED!")
    (begin
      (display "SOME TESTS FAILED: ")
      (display *tests-failed*)
      (display " failure(s)")))
  (newline)
  *tests-failed*)
