;; ============================================================
;; Character Edge Cases - derived from SBCL character.pure.lisp
;; ============================================================

(load "tests/framework.lisp")

;; ============================================================
;; Character Predicates
;; ============================================================
(start-suite "char-predicates")

;; Graphic character tests
(assert-true (graphic-char-p #\a) "graphic-char-p: lowercase")
(assert-true (graphic-char-p #\Z) "graphic-char-p: uppercase")
(assert-true (graphic-char-p #\0) "graphic-char-p: digit")
(assert-true (graphic-char-p #\!) "graphic-char-p: punctuation")

;; Non-graphic characters
(assert-false (graphic-char-p #\Newline) "graphic-char-p: newline is not graphic")
(assert-false (graphic-char-p #\Space) "graphic-char-p: space is not graphic")

;; Alpha character tests
(assert-true (alpha-char-p #\a) "alpha-char-p: lowercase letter")
(assert-true (alpha-char-p #\Z) "alpha-char-p: uppercase letter")
(assert-false (alpha-char-p #\0) "alpha-char-p: digit is not alpha")

;; Case tests
(assert-true (upper-case-p #\A) "upper-case-p: uppercase")
(assert-true (lower-case-p #\a) "lower-case-p: lowercase")
(assert-true (both-case-p #\a) "both-case-p: letter has case")
(assert-false (both-case-p #\0) "both-case-p: digit has no case")

(end-suite)

;; ============================================================
;; Character Comparison
;; ============================================================
(start-suite "char-comparison")

(assert-true (char= #\a #\a) "char=: equal chars")
(assert-false (char= #\a #\b) "char=: different chars")
(assert-true (char/= #\a #\b) "char/=: different chars")
(assert-true (char< #\a #\b) "char<: less than")
(assert-true (char<= #\a #\b) "char<=: less or equal")
(assert-true (char> #\b #\a) "char>: greater than")
(assert-true (char>= #\b #\a) "char>=: greater or equal")

;; Multiple args
(assert-true (char< #\a #\b #\c) "char<: chain")
(assert-true (char= #\a #\a #\a) "char=: multiple equal")

(end-suite)

;; ============================================================
;; Character Code Operations
;; ============================================================
(start-suite "char-code")

(assert-equal 97 (char-code #\a) "char-code: lowercase a")
(assert-equal 65 (char-code #\A) "char-code: uppercase A")
(assert-equal 48 (char-code #\0) "char-code: digit 0")

(assert-equal #\a (code-char 97) "code-char: 97 to a")
(assert-equal #\A (code-char 65) "code-char: 65 to A")

(end-suite)

;; ============================================================
;; Digit Character Operations
;; ============================================================
(start-suite "digit-char")

(assert-equal #\5 (digit-char 5) "digit-char: 5")
(assert-equal #\0 (digit-char 0) "digit-char: 0")
(assert-equal 5 (digit-char-p #\5) "digit-char-p: 5 returns 5")
(assert-equal nil (digit-char-p #\a) "digit-char-p: letter returns nil")
(assert-equal 10 (digit-char-p #\a 16) "digit-char-p: hex a")
(assert-equal 15 (digit-char-p #\f 16) "digit-char-p: hex f")

(end-suite)

;; ============================================================
;; Character Names
;; ============================================================
(start-suite "char-names")

(assert-equal "Space" (char-name #\Space) "char-name: space")
(assert-equal "Newline" (char-name #\Newline) "char-name: newline")
(assert-equal "Tab" (char-name #\Tab) "char-name: tab")

(assert-equal #\Space (name-char "Space") "name-char: space")
(assert-equal #\Newline (name-char "Newline") "name-char: newline")

(end-suite)

;; ============================================================
;; String Characters
;; ============================================================
(start-suite "string-chars")

(define s "Hello")
(assert-equal #\H (char s 0) "char: first char")
(assert-equal #\o (char s 4) "char: last char")

(setf (char s 0) #\J)
(assert-equal #\J (char s 0) "setf char: updates")

(assert-equal "Jello" s "string: after setf")

(end-suite)

;; ============================================================
;; Character Conversion
;; ============================================================
(start-suite "char-case-conversion")

(assert-equal #\A (char-upcase #\a) "char-upcase: a to A")
(assert-equal #\a (char-downcase #\A) "char-downcase: A to a")
(assert-equal #\Z (char-upcase #\z) "char-upcase: z to Z")
(assert-equal #\z (char-downcase #\z) "char-downcase: z stays z")

(end-suite)

;; ============================================================
;; Summary
;; ============================================================
(test-summary)
