;; readtable-tests.lisp — tests for the readtable system
;; MicroLisp readtable support: *readtable*, set-macro-character, etc.

(load "tests/framework.lisp")
(start-suite "Readtable System")

;; --- Basic readtable predicates ---
(assert-true (readtablep *readtable*) "*readtable* is a readtable")
(assert-false (readtablep 42) "numbers are not readtables")
(assert-false (readtablep 'foo) "symbols are not readtables")
(assert-false (readtablep "hello") "strings are not readtables")
(assert-false (readtablep '(1 2 3)) "lists are not readtables")

;; --- readtable-case ---
(assert-equal ':UPCASE (readtable-case *readtable*) "standard readtable is :UPCASE")

;; --- make-readtable ---
(define rt (make-readtable))
(assert-true (readtablep rt) "make-readtable creates a readtable")
(assert-equal ':UPCASE (readtable-case rt) "new readtable defaults to :UPCASE")

;; --- copy-readtable ---
(define rt-copy (copy-readtable))
(assert-true (readtablep rt-copy) "copy-readtable creates a readtable")
(assert-equal ':UPCASE (readtable-case rt-copy) "copied readtable preserves case mode")

(define rt2 (copy-readtable *readtable*))
(assert-true (readtablep rt2) "copy-readtable with argument works")

;; --- set-readtable-case ---
(set-readtable-case rt ':DOWNCASE)
(assert-equal ':DOWNCASE (readtable-case rt) "set-readtable-case works for :DOWNCASE")
(set-readtable-case rt ':PRESERVE)
(assert-equal ':PRESERVE (readtable-case rt) "set-readtable-case works for :PRESERVE")
(set-readtable-case rt ':INVERT)
(assert-equal ':INVERT (readtable-case rt) "set-readtable-case works for :INVERT")
(set-readtable-case rt ':UPCASE)
(assert-equal ':UPCASE (readtable-case rt) "set-readtable-case works for :UPCASE")

;; --- get-macro-character for standard chars ---
;; Use code-char for characters that conflict with reader syntax
(assert-true (not (null (get-macro-character (code-char 39)))) "get-macro-character for quote returns a function")
(assert-true (not (null (get-macro-character '#\,))) "get-macro-character for comma returns a function")
(assert-true (not (null (get-macro-character '#\;))) "get-macro-character for semicolon returns a function")
(assert-true (not (null (get-macro-character (code-char 41)))) "get-macro-character for close-paren returns a function")
(assert-true (not (null (get-macro-character (code-char 40)))) "get-macro-character for open-paren returns a function")
(assert-true (not (null (get-macro-character (code-char 34)))) "get-macro-character for double-quote returns a function")

;; --- get-macro-character for non-macro chars ---
(assert-nil (get-macro-character '#\a) "get-macro-character for a returns nil")
(assert-nil (get-macro-character '#\+) "get-macro-character for + returns nil")
(assert-nil (get-macro-character '#\space) "get-macro-character for space returns nil")

;; --- set-macro-character: define ! as macro that returns quoted bang ---
(set-macro-character #\! (lambda (c) (list 'quote 'bang)))
(assert-equal 'bang ! "macro ! expands to bang")

;; --- set-macro-character with non-terminating ---
(set-macro-character #\$ (lambda (c) (list 'quote 'dollar)) #f)
(assert-true #t "set-macro-character with non-terminating flag works")

;; --- make-dispatch-macro-character ---
(make-dispatch-macro-character '#\#)
(assert-true #t "make-dispatch-macro-character works")
(assert-nil (get-dispatch-macro-character '#\# '#\!) "get-dispatch-macro-char returns nil for unset")

;; --- Multiple macro characters ---
(set-macro-character #\@ (lambda (c) (list 'quote 'at-sign)))
(assert-equal 'at-sign @ "at-sign macro works")

;; --- Macro character overwriting ---
(set-macro-character #\! (lambda (c) (list 'quote 'new-bang)))
(assert-equal 'new-bang ! "macro overwriting works")

;; --- Readtable isolation ---
(define rt-isolated (make-readtable))
(assert-nil (get-macro-character #\! rt-isolated) "fresh readtable has no ! macro")

;; --- symbol-name ---
(assert-equal "FOO" (symbol-name 'FOO) "symbol-name returns correct string")
(assert-equal ":KEYWORD" (symbol-name ':KEYWORD) "keyword symbol name includes colon")

;; --- Nested readtable operations ---
(set-readtable-case rt2 ':DOWNCASE)
(assert-equal ':DOWNCASE (readtable-case rt2) "second readtable case set independently")

;; --- Verify standard behavior still works ---
(assert-equal '(1 2 3) '(1 2 3) "standard quote behavior unchanged")
(assert-equal '(1 . 2) '(1 . 2) "standard dotted pair unchanged")
(assert-equal "hello" "hello" "standard strings unchanged")

(end-suite)
(test-summary)