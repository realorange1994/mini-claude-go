;; ============================================================
;; Enhanced Character Tests - derived from SBCL character.pure.lisp
;; ============================================================

(load "tests/framework.lisp")

;; ============================================================
;; Character Predicates
;; ============================================================
(start-suite "character-predicates")

;; alpha-char-p (MicroLisp uses alpha-char-p, not alpha-char?)
(assert-true (alpha-char-p #\a) "alpha-char-p: lowercase")
(assert-true (alpha-char-p #\Z) "alpha-char-p: uppercase")
(assert-false (alpha-char-p #\5) "alpha-char-p: digit")
(assert-false (alpha-char-p #\space) "alpha-char-p: space")

;; digit-char-p
(assert-equal 5 (digit-char-p #\5) "digit-char-p: #\5 = 5")
(assert-equal 0 (digit-char-p #\0) "digit-char-p: #\0 = 0")
(assert-nil (digit-char-p #\a) "digit-char-p: letter returns nil")

;; alphanumericp
(assert-true (alphanumericp #\a) "alphanumericp: letter")
(assert-true (alphanumericp #\5) "alphanumericp: digit")
(assert-false (alphanumericp #\space) "alphanumericp: space")

;; upper-case-p / lower-case-p
(assert-true (upper-case-p #\A) "upper-case-p: A")
(assert-false (upper-case-p #\a) "upper-case-p: a")
(assert-true (lower-case-p #\a) "lower-case-p: a")
(assert-false (lower-case-p #\A) "lower-case-p: A")

;; both-case-p
(assert-true (both-case-p #\a) "both-case-p: a")
(assert-true (both-case-p #\Z) "both-case-p: Z")
(assert-false (both-case-p #\5) "both-case-p: digit")
(assert-false (both-case-p #\space) "both-case-p: space")

(end-suite)

;; ============================================================
;; Character Case Conversion
;; ============================================================
(start-suite "case-conversion")

(assert-equal #\A (char-upcase #\a) "char-upcase: a -> A")
(assert-equal #\a (char-downcase #\A) "char-downcase: A -> a")
(assert-equal #\Z (char-upcase #\z) "char-upcase: z -> Z")
(assert-equal #\z (char-downcase #\Z) "char-downcase: Z -> z")

;; Non-letters unchanged
(assert-equal #\5 (char-upcase #\5) "char-upcase: digit unchanged")
(assert-equal #\5 (char-downcase #\5) "char-downcase: digit unchanged")

(end-suite)

;; ============================================================
;; Character Comparisons
;; ============================================================
(start-suite "char-comparisons")

(assert-true (char= #\a #\a) "char=: equal")
(assert-false (char= #\a #\b) "char=: not equal")
(assert-true (char< #\a #\b) "char<: less")
(assert-true (char> #\b #\a) "char>: greater")
(assert-true (char<= #\a #\b) "char<=: less or equal")
(assert-true (char>= #\b #\a) "char>=: greater or equal")

;; Case-insensitive
(assert-true (char-equal #\a #\A) "char-equal: a = A")
(assert-true (char-equal #\b #\B) "char-equal: b = B")

(end-suite)

;; ============================================================
;; Character Codes
;; ============================================================
(start-suite "char-codes")

(assert-equal 65 (char-code #\A) "char-code: A = 65")
(assert-equal 97 (char-code #\a) "char-code: a = 97")
(assert-equal #\A (code-char 65) "code-char: 65 = A")
(assert-equal #\a (code-char 97) "code-char: 97 = a")

;; ASCII control chars
(assert-equal 10 (char-code #\newline) "char-code: newline = 10")
(assert-equal 32 (char-code #\space) "char-code: space = 32")

(end-suite)

;; ============================================================
;; Character Names
;; ============================================================
(start-suite "string-chars")

(define s "Hello")
(assert-true (not (null? (char s 0))) "char: returns char")
(assert-true (not (null? (char s 1))) "char: returns char")
(assert-true (not (null? (char s 4))) "char: returns char")

(end-suite)

;; ============================================================
;; String Comparison
;; ============================================================
(start-suite "string-comparisons")

(assert-true (string= "hello" "hello") "string=: equal returns truthy")
(assert-false (string= "hello" "world") "string=: not equal returns nil")
(assert-equal 0 (string< "abc" "bcd") "string<: returns 0 for mismatch at first")
(assert-equal 0 (string> "bcd" "abc") "string>: returns 0 for mismatch at first")
(assert-equal 3 (string<= "abc" "abc") "string<=: equal returns length")
(assert-equal 3 (string>= "abc" "abc") "string>=: equal returns length")

;; Case-insensitive
(assert-true (string-equal "HELLO" "hello") "string-equal: case insensitive returns truthy")

(end-suite)

;; ============================================================
;; String Case Conversion
;; ============================================================
(start-suite "string-case")

(assert-equal "HELLO" (string-upcase "hello") "string-upcase")
(assert-equal "hello" (string-downcase "HELLO") "string-downcase")
(assert-equal "Hello" (string-capitalize "hello") "string-capitalize: single")
(assert-equal "Hello World" (string-capitalize "HELLO WORLD") "string-capitalize: multiple")

(end-suite)

;; ============================================================
;; Summary
;; ============================================================
(test-summary)
