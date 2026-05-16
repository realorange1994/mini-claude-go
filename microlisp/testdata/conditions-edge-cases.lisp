;; ============================================================
;; Condition/Exception Handling Edge Cases - derived from SBCL condition.pure.lisp
;; ============================================================

(load "tests/framework.lisp")

;; ============================================================
;; Basic Condition Types
;; ============================================================
(start-suite "condition-basics")

;; Simple error handling
(define (test-error) (error "test error"))
(assert-error '(test-error) "error: basic error function")

;; Error with format message
(define (test-error-fmt) (error "value: ~A" 42))
(assert-error '(test-error-fmt) "error: format error")

(end-suite)

;; ============================================================
;; Handler Cases
;; ============================================================
(start-suite "handler-case-basics")

(define (test-handler-case)
  (handler-case
    (error "test")
    (error (e) 'caught-error)))

(assert-equal 'caught-error (test-handler-case) "handler-case: catches error")

(end-suite)

;; ============================================================
;; Multiple Error Types
;; ============================================================
(start-suite "handler-case-types")

(define (test-handler-case-types)
  (handler-case
    (error "test")
    (simple-error (e) 'simple)
    (error (e) 'general)))

(assert-equal 'simple (test-handler-case-types) "handler-case: matches most specific error type")

(end-suite)

;; ============================================================
;; Condition Variables
;; ============================================================
(start-suite "condition-accessors")

(define (test-condition-slot)
  (handler-case
    (error "test message")
    (error (e)
      (condition-message e))))

(define result (test-condition-slot))
(assert-equal "test message" result "condition-accessors: returns message")

(end-suite)

;; ============================================================
;; Restart Cases
;; ============================================================
(start-suite "restart-case-basics")

(define (test-restart-case)
  (restart-case
    (invoke-restart 'retry)
    (retry () 'retry-called)))

(assert-equal 'retry-called (test-restart-case) "restart-case: basic restart")

(end-suite)

;; ============================================================
;; Summary
;; ============================================================
(test-summary)
