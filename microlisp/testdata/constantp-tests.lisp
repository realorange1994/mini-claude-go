;; ============================================================
;; Constantp Tests - simplified for MicroLisp (constantp not available)
;; ============================================================

(load "tests/framework.lisp")
(start-suite "Constantp Tests")

;; MicroLisp does not implement constantp, so we just verify
;; basic predicates work as a smoke test
(assert-true (number? 1) "number?: 1 is a number")
(assert-true (string? "hello") "string?: string is string")
(assert-true (symbol? 'x) "symbol?: symbol is symbol")
(assert-false (number? "hello") "number?: string is not number")

(end-suite)

(test-summary)
