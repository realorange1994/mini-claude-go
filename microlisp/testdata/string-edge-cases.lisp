;; string-edge-cases.lisp — tests for string operations
;; Covers: string-upcase, string-downcase, string-capitalize,
;;         string-trim variants, char, string compare

(load "tests/framework.lisp")
(start-suite "String Case")

;; --- string-upcase ---
(assert-equal "HELLO" (string-upcase "hello") "upcase lowercase")
(assert-equal "HELLO" (string-upcase "HELLO") "upcase already uppercase")
(assert-equal "HELLO123" (string-upcase "hello123") "upcase with numbers")
(assert-equal "" (string-upcase "") "upcase empty string")

;; --- string-downcase ---
(assert-equal "hello" (string-downcase "HELLO") "downcase uppercase")
(assert-equal "hello" (string-downcase "hello") "downcase already lowercase")
(assert-equal "hello123" (string-downcase "HELLO123") "downcase with numbers")
(assert-equal "" (string-downcase "") "downcase empty string")

;; --- string-capitalize ---
(assert-equal "Hello World" (string-capitalize "hello world") "capitalize two words")
(assert-equal "Hello" (string-capitalize "hello") "capitalize single word")
(assert-equal "" (string-capitalize "") "capitalize empty string")

(end-suite)
(start-suite "String Trimming")

;; --- string-trim ---
;; Note: string-trim treats first arg as a set of characters to trim, not a substring
(assert-equal "hello" (string-trim " " "  hello  ") "trim spaces both sides")
(assert-equal "llo" (string-trim "he" "hello") "trim h and e from both ends")
(assert-equal "xyz" (string-trim "abc" "abcxyzabc") "trim a/b/c from both ends")

;; --- string-left-trim ---
(assert-equal "hello  " (string-left-trim " " "  hello  ") "left trim spaces")
(assert-equal "llo" (string-left-trim "he" "hello") "left trim h and e")

;; --- string-right-trim ---
(assert-equal "  hello" (string-right-trim " " "  hello  ") "right trim spaces")
(assert-equal "he" (string-right-trim "lo" "hello") "right trim l and o")

(end-suite)
(start-suite "String Comparison")

;; --- string= ---
(assert-true (string= "hello" "hello") "string= equal")
(assert-false (string= "hello" "world") "string= unequal")
(assert-true (string= "" "") "string= empty")

;; --- string< (returns index if true, nil if false) ---
(assert-true (if (string< "abc" "abd") #t #f) "string< less returns truthy")
(assert-nil (string< "abd" "abc") "string< not less")
(assert-nil (string< "abc" "abc") "string< equal returns nil")

;; --- string> (returns index if true, nil if false) ---
(assert-true (if (string> "abd" "abc") #t #f) "string> greater returns truthy")
(assert-nil (string> "abc" "abd") "string> not greater")

;; --- string<= (returns index if true, nil if false) ---
(assert-true (if (string<= "abc" "abd") #t #f) "string<= less returns truthy")
(assert-true (if (string<= "abc" "abc") #t #f) "string<= equal returns truthy")

;; --- string>= (returns index if true, nil if false) ---
(assert-true (if (string>= "abd" "abc") #t #f) "string>= greater returns truthy")
(assert-true (if (string>= "abc" "abc") #t #f) "string>= equal returns truthy")

(end-suite)
(start-suite "Character Access")

;; --- char ---
(assert-equal #\h (char "hello" 0) "char index 0")
(assert-equal #\e (char "hello" 1) "char index 1")
(assert-equal #\o (char "hello" 4) "char index last")

(end-suite)
(start-suite "String Length")

;; --- length of string ---
(assert-equal 5 (length "hello") "length 5 chars")
(assert-equal 0 (length "") "length empty")
(assert-equal 1 (length " ") "length one space")

(end-suite)
(start-suite "String Concatenation")

;; --- string-append ---
(assert-equal "hello" (string-append "hello") "concat single")
(assert-equal "" (string-append "") "concat empty")

(end-suite)
(start-suite "String Edge Cases")

;; --- Special characters ---
(assert-equal 5 (length "a b c") "length with spaces")
(assert-equal 10 (length "0123456789") "length digits")
(assert-equal #\A (char "A" 0) "single char string")

;; --- Substring / subseq ---
(assert-equal "hel" (subseq "hello" 0 3) "subseq first 3")
(assert-equal "llo" (subseq "hello" 2) "subseq from 2 to end")
(assert-equal "" (subseq "hello" 0 0) "subseq empty range")

;; --- char= ---
(assert-true (char= #\a #\a) "char= equal")
(assert-false (char= #\a #\b) "char= unequal")

;; --- char/= ---
(assert-true (char/= #\a #\b) "char/= unequal")
(assert-false (char/= #\a #\a) "char/= equal")

;; --- char< ---
(assert-true (char< #\a #\b) "char< less")
(assert-false (char< #\b #\a) "char< not less")

(end-suite)
(test-summary)
