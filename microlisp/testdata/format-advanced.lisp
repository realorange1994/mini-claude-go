;; format-advanced.lisp — tests for format directives
;; Tests what MicroLisp format actually supports

(load "tests/framework.lisp")
(start-suite "Integer Formatting")

;; --- ~d: Decimal ---
(assert-equal "42" (format nil "~d" 42) "decimal")
(assert-equal "-42" (format nil "~d" -42) "negative decimal")
(assert-equal "0" (format nil "~d" 0) "zero")

;; --- ~x: Hexadecimal ---
(assert-equal "2a" (format nil "~x" 42) "hex lowercase")
(assert-equal "#x2a" (format nil "~@X" 42) "hex uppercase with at-flag (includes 0x prefix)")
(assert-equal "0" (format nil "~x" 0) "zero hex")

;; --- ~o: Octal ---
(assert-equal "52" (format nil "~o" 42) "octal")
(assert-equal "0" (format nil "~o" 0) "zero octal")

;; --- ~b: Binary ---
(assert-equal "101010" (format nil "~b" 42) "binary")
(assert-equal "0" (format nil "~b" 0) "zero binary")

;; --- ~@d: Plus sign ---
(assert-equal "+42" (format nil "~@d" 42) "plus sign positive")
(assert-equal "-42" (format nil "~@d" -42) "plus sign negative")

(end-suite)
(start-suite "Float Formatting")

;; --- ~f: Fixed-point ---
(assert-equal "3.14" (format nil "~f" 3.14) "fixed-point")
(assert-equal "0.5" (format nil "~2f" 0.5) "fixed-point width 2 (microLisp ignores)")

;; --- ~e: Exponential ---
(assert-equal "3.14E+0" (format nil "~e" 3.14) "exponential")

;; --- ~g: General ---
(assert-equal "3.14" (format nil "~g" 3.14) "general float")

(end-suite)
(start-suite "String and Symbol Formatting")

;; --- ~a: Aesthetic ---
(assert-equal "hello" (format nil "~a" "hello") "aesthetic string")
(assert-equal "HELLO" (format nil "~a" 'hello) "aesthetic symbol")
(assert-equal "42" (format nil "~a" 42) "aesthetic number")
(assert-equal "(1 2 3)" (format nil "~a" (list 1 2 3)) "aesthetic list")
(assert-equal ":KEY" (format nil "~a" :key) "aesthetic keyword")
(assert-equal "A" (format nil "~a" #\A) "aesthetic character")

;; --- ~s: Standard (readably) ---
(assert-equal "\"hello\"" (format nil "~s" "hello") "standard string")
(assert-equal "42" (format nil "~s" 42) "standard number")
(assert-equal "(1 2 3)" (format nil "~s" (list 1 2 3)) "standard list")
(assert-equal "#\\A" (format nil "~s" #\A) "standard character")

;; --- ~c: Character ---
(assert-equal "A" (format nil "~c" #\A) "character format")

(end-suite)
(start-suite "List Formatting: ~{ ~}")

;; --- ~{ ~}: Iterate over list ---
(assert-equal "1, 2, 3, " (format nil "~{~a, ~}" (list 1 2 3)) "iterate list")
(assert-equal "" (format nil "~{~a~}" '()) "iterate empty list")

(end-suite)
(start-suite "Conditional Formatting: ~[ ~]")

;; --- ~[ ~]: Conditional ---
(assert-equal "no" (format nil "~[no~;yes~]" 0) "conditional 0")
(assert-equal "yes" (format nil "~[no~;yes~]" 1) "conditional 1")
(assert-equal "maybe" (format nil "~[no~;yes~;maybe~]" 2) "conditional 2")

(end-suite)
(start-suite "Radix to English: ~R")

;; --- ~R: English number ---
(assert-equal "zero" (format nil "~r" 0) "English zero")
(assert-equal "one" (format nil "~r" 1) "English one")
(assert-equal "five" (format nil "~r" 5) "English five")
(assert-equal "forty-two" (format nil "~r" 42) "English forty-two")
(assert-equal "ninety-nine" (format nil "~r" 99) "English ninety-nine")

(end-suite)
(start-suite "Whitespace and Control")

;; --- ~~: Literal tilde ---
(assert-equal "hello~world" (format nil "hello~~world") "literal tilde")

;; --- Empty format string ---
(assert-equal "" (format nil "") "empty format")

;; --- Format with no args ---
(assert-equal "hello" (format nil "hello") "format no args")

;; --- Large numbers ---
(assert-equal "1000000" (format nil "~d" 1000000) "large number")
(assert-equal "-999999" (format nil "~d" -999999) "large negative")

;; --- Multiple arguments ---
(assert-equal "42 2a 52 101010" (format nil "~d ~x ~o ~b" 42 42 42 42) "multi args")
(assert-equal "hello world" (format nil "~a ~a" "hello" "world") "multi aesthetic")

(end-suite)
(test-summary)
