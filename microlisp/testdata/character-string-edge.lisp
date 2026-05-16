;; ============================================================
;; Character & String Edge Cases - derived from SBCL character.pure.lisp
;; ============================================================

(load "tests/framework.lisp")

;; ============================================================
;; Character Tests
;; ============================================================
(start-suite "character-basic")

;; Basic character operations
(assert-equal #\a (char "abc" 0) "char: first char")
(assert-equal #\c (char "abc" 2) "char: last char")

;; Character comparison
(assert-true (char= #\a #\a) "char=: equal")
(assert-false (char= #\a #\b) "char=: not equal")
(assert-true (char/= #\a #\b) "char/=: not equal")
(assert-true (char< #\a #\b) "char<: less than")
(assert-true (char<= #\a #\b) "char<=: less or equal")
(assert-true (char<= #\a #\a) "char<=: equal")
(assert-true (char> #\b #\a) "char>: greater than")
(assert-true (char>= #\b #\a) "char>=: greater or equal")

(end-suite)

;; ============================================================
;; String Tests
;; ============================================================
(start-suite "string-basic")

;; Basic string operations
(assert-equal 5 (length "hello") "string: length")
(assert-equal #\h (char "hello" 0) "string: char at 0")
(assert-equal #\o (char "hello" 4) "string: char at 4")

;; String comparison - MicroLisp returns index or nil
(assert-true (string= "abc" "abc") "string=: equal")
(assert-false (string= "abc" "abd") "string=: not equal")
;; Note: MicroLisp's string< returns index (truthy), not boolean
(assert-true (if (string< "abc" "abd") #t #f) "string<: less than returns index")
(assert-true (if (string<= "abc" "abd") #t #f) "string<=: less or equal returns index")
(assert-true (if (string> "abd" "abc") #t #f) "string>: greater than returns index")
(assert-true (if (string>= "abd" "abc") #t #f) "string>=: greater or equal returns index")

;; String conversion
(assert-equal "HELLO" (string-upcase "hello") "string-upcase")
(assert-equal "hello" (string-downcase "HELLO") "string-downcase")

(end-suite)

;; ============================================================
;; Symbol Name Tests
;; ============================================================
(start-suite "symbol-name-basic")

;; Symbol to string - CL reader uppercases, symbol-name returns uppercase
(assert-equal "HELLO" (symbol-name 'hello) "symbol-name returns uppercase")

(end-suite)

;; ============================================================
;; Summary
;; ============================================================
(test-summary)
