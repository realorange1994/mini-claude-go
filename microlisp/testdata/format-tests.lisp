;; ============================================================
;; Format Tests - derived from SBCL format.pure.lisp
;; ============================================================

(load "tests/framework.lisp")
(start-suite "Format Tests")

;; ============================================================
;; Basic Format Tests
;; ============================================================
(start-suite "Basic Format")

;; Simple format strings
(assert-equal "hello" (format #f "hello") "format: literal string")
(assert-equal "Hello World" (format #f "Hello ~a" "World") "format: ~a directive")
(assert-equal "Value: 42" (format #f "Value: ~d" 42) "format: ~d for integers")

(end-suite)

;; ============================================================
;; ~A Directive (Aesthetic)
;; ============================================================
(start-suite "~A Directive")

(assert-equal "test" (format #f "~a" "test") "~a: string")
(assert-equal "42" (format #f "~a" 42) "~a: integer")
(assert-equal "(1 2 3)" (format #f "~a" '(1 2 3)) "~a: list")

(end-suite)

;; ============================================================
;; ~D Directive (Decimal)
;; ============================================================
(start-suite "~D Directive")

(assert-equal "123" (format #f "~d" 123) "~d: positive integer")
(assert-equal "-456" (format #f "~d" -456) "~d: negative integer")
(assert-equal "123" (format #f "~d" 123) "~d: decimal")

(end-suite)

;; ============================================================
;; ~S Directive (S-expression)
;; ============================================================
(start-suite "~S Directive")

(assert-equal "FOO" (format #f "~s" 'foo) "~s: symbol")
(assert-equal "\"bar\"" (format #f "~s" "bar") "~s: string with quotes")

(end-suite)

;; ============================================================
;; ~% Directive (Newline)
;; ============================================================
(start-suite "~% Directive")

;; Build expected strings with actual newlines
(define nl (coerce (list #\newline) 'string))
(define expected-nl (concatenate 'string "line1" nl "line2"))
(define expected-multi (concatenate 'string "a" nl "b" nl "c"))

(assert-equal expected-nl (format #f "line1~%line2") "~% inserts newline")
(assert-equal expected-multi (format #f "a~%b~%c") "~% multiple newlines")

(end-suite)

;; ============================================================
;; ~& Directive (Freshline)
;; ============================================================
(start-suite "~& Directive")

;; ~& at start of buffer: no newline (already at start)
(assert-equal "" (format #f "~&") "~&: freshline")
;; ~& after text: adds newline
(define expected-fresh (concatenate 'string "a" nl "b"))
(assert-equal expected-fresh (format #f "a~&b") "~& after text")

(end-suite)

;; ============================================================
;; ~~ Directive (Tilde)
;; ============================================================
(start-suite "~~ Directive")

(assert-equal "~" (format #f "~~") "~~: single tilde")
(assert-equal "~~" (format #f "~~~") "~~~: multiple tildes")

(end-suite)

;; ============================================================
;; ~T Directive (Tab)
;; ============================================================
(start-suite "~T Directive")

;; ~t pads with spaces (default colinc=2, so "a" -> "a ")
(define expected-tab1 (concatenate 'string "a" " " "b"))
(assert-equal expected-tab1 (format #f "a~tb") "~t: tab")
;; ~10t pads to column 10
(define expected-tab2 (concatenate 'string "a" "         " "b"))
(assert-equal expected-tab2 (format #f "a~10tb") "~t: tab to column 10")

(end-suite)

(test-summary)
