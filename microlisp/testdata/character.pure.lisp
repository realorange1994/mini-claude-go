;; ============================================================
;; Character Tests - simplified for MicroLisp
;; ============================================================

(load "tests/framework.lisp")

;; ============================================================
;; Character Basics
;; ============================================================
(start-suite "character-basics")

(assert-true (characterp #\a) "characterp: lowercase letter")
(assert-true (characterp #\space) "characterp: space")
(assert-true (characterp #\newline) "characterp: newline")
(assert-false (characterp "a") "characterp: string not character")
(assert-false (characterp 'a) "characterp: symbol not character")

(end-suite)

;; ============================================================
;; Character Names
;; ============================================================
(start-suite "character-names")

;; Standard character names
(assert-equal #\space (name-char "Space") "name-char: Space")
(assert-equal #\newline (name-char "Newline") "name-char: Newline")

;; Invalid name returns nil
(assert-equal '() (name-char "Foo") "name-char: invalid returns nil")

(end-suite)

;; ============================================================
;; Character Case Operations
;; ============================================================
(start-suite "character-case")

(assert-equal #\A (char-upcase #\a) "char-upcase: a -> A")
(assert-equal #\Z (char-upcase #\z) "char-upcase: z -> Z")
(assert-equal #\a (char-downcase #\A) "char-downcase: A -> a")
(assert-equal #\z (char-downcase #\Z) "char-downcase: Z -> z")

;; Non-letter characters unchanged
(assert-equal #\1 (char-upcase #\1) "char-upcase: digit unchanged")
(assert-equal #\1 (char-downcase #\1) "char-downcase: digit unchanged")

(end-suite)

;; ============================================================
;; Character Classification
;; ============================================================
(start-suite "character-classification")

;; Alphabetic
(assert-true (alpha-char-p #\a) "alpha-char-p: lowercase")
(assert-true (alpha-char-p #\Z) "alpha-char-p: uppercase")
(assert-false (alpha-char-p #\5) "alpha-char-p: digit")
(assert-false (alpha-char-p #\space) "alpha-char-p: space")

;; Digits
(assert-equal 0 (digit-char-p #\0) "digit-char-p: zero")
(assert-equal 9 (digit-char-p #\9) "digit-char-p: nine")
(assert-equal '() (digit-char-p #\a) "digit-char-p: letter")

;; Upper/Lower case
(assert-true (upper-case-p #\A) "upper-case-p: uppercase")
(assert-true (lower-case-p #\a) "lower-case-p: lowercase")
(assert-false (upper-case-p #\a) "upper-case-p: not lowercase")
(assert-false (lower-case-p #\A) "lower-case-p: not uppercase")

;; Both case (letters with both cases)
(assert-true (both-case-p #\a) "both-case-p: lowercase")
(assert-true (both-case-p #\Z) "both-case-p: uppercase")
(assert-false (both-case-p #\5) "both-case-p: digit")

;; Alphanumeric
(assert-true (alphanumericp #\a) "alphanumericp: letter")
(assert-true (alphanumericp #\9) "alphanumericp: digit")
(assert-false (alphanumericp #\space) "alphanumericp: space")

(end-suite)

;; ============================================================
;; Character Comparisons
;; ============================================================
(start-suite "character-comparisons")

;; Basic equality
(assert-true (char= #\a #\a) "char=: equal")
(assert-false (char= #\a #\b) "char=: not equal")

;; Ordering
(assert-true (char< #\a #\b) "char<: a < b")
(assert-true (char< #\A #\Z) "char<: A < Z")
(assert-false (char< #\b #\a) "char<: reversed")

(assert-true (char> #\b #\a) "char>: b > a")
(assert-false (char> #\a #\b) "char>: reversed")

(assert-true (char<= #\a #\b) "char<=: a <= b")
(assert-true (char<= #\a #\a) "char<=: equal")

(assert-true (char>= #\b #\a) "char>=: b >= a")
(assert-true (char>= #\a #\a) "char>=: equal")

;; Case-insensitive comparisons
(assert-true (char-equal #\A #\a) "char-equal: A = a")
(assert-true (char-equal #\Z #\z) "char-equal: Z = z")

(end-suite)

;; ============================================================
;; Character Codes
;; ============================================================
(start-suite "character-codes")

(assert-equal 65 (char-code #\A) "char-code: A = 65")
(assert-equal 97 (char-code #\a) "char-code: a = 97")
(assert-equal 48 (char-code #\0) "char-code: 0 = 48")

(assert-equal #\A (code-char 65) "code-char: 65 = A")
(assert-equal #\a (code-char 97) "code-char: 97 = a")

;; Round trip
(assert-equal #\x (code-char (char-code #\x)) "code-char round trip")

(end-suite)

(test-summary)
