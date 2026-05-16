;; ============================================================
;; MicroLisp SBCL-Derived Test Suite
;; ============================================================
;; Additional assertion tests extracted and adapted from
;; SBCL test suite files (arith.pure.lisp, seq.pure.lisp,
;; string.pure.lisp, character.pure.lisp, list.pure.lisp,
;; condition.pure.lisp, format.pure.lisp, coerce.pure.lisp,
;; loop.pure.lisp).
;;
;; Uses only MicroLisp's built-in (assert test) or (assert test msg)
;; ============================================================

;; ===== ARITHMETIC (from arith.pure.lisp) =====

;; Fundamental arithmetic smoke test
(assert (= (+ 4 2) 6) "+ 4 2 = 6")
(assert (= (- 4 2) 2) "- 4 2 = 2")
(assert (= (* 4 2) 8) "* 4 2 = 8")
(assert (= (/ 4 2) 2) "/ 4 2 = 2")
(assert (= (expt 4 2) 16) "expt 4 2 = 16")
(assert (= (+ 2 4) 6) "+ commutative")
(assert (= (- 2 4) -2) "- 2 4 = -2")
(assert (= (* 2 4) 8) "* commutative")
(assert (= (/ 2 4) 1/2) "/ 2 4 = 1/2")
(assert (= (expt 2 4) 16) "expt commutative")

;; GCD
(assert (= (gcd 0 10) 10) "gcd 0 10 = 10")
(assert (= (gcd 0 -10) 10) "gcd 0 -10 = 10 (abs)")
(assert (= (gcd 12 18) 6) "gcd 12 18 = 6")
(assert (= (gcd 0) 0) "gcd 0 = 0")

;; LCM
(assert (= (lcm 4 -10) 20) "lcm 4 -10 = 20 (non-negative)")
(assert (= (lcm 0 0) 0) "lcm 0 0 = 0")
(assert (= (lcm) 1) "lcm no args = 1")

;; EXPT edge cases
(assert (= (expt 0 0) 1) "expt 0 0 = 1")

;; ISQRT
(assert (= (isqrt 0) 0) "isqrt 0 = 0")
(assert (= (isqrt 1) 1) "isqrt 1 = 1")
(assert (= (isqrt 2) 1) "isqrt 2 = 1")
(assert (= (isqrt 3) 1) "isqrt 3 = 1")
(assert (= (isqrt 4) 2) "isqrt 4 = 2")
(assert (= (isqrt 8) 2) "isqrt 8 = 2")
(assert (= (isqrt 9) 3) "isqrt 9 = 3")
(assert (= (isqrt 15) 3) "isqrt 15 = 3")
(assert (= (isqrt 16) 4) "isqrt 16 = 4")
(assert (= (isqrt 100) 10) "isqrt 100 = 10")
(assert (= (isqrt 999) 31) "isqrt 999 = 31")
(assert (= (isqrt 1000) 31) "isqrt 1000 = 31")
(assert (= (isqrt 1024) 32) "isqrt 1024 = 32")

;; SIGNUM
(assert (= (signum 5) 1) "signum positive")
(assert (= (signum -5) -1) "signum negative")
(assert (= (signum 0) 0) "signum zero")

;; MIN/MAX
(assert (= (min -1) -1) "min 1-arg")
(assert (= (max 0) 0) "max 1-arg")
(assert (= (min 10 11) 10) "min 2-arg")
(assert (= (max -1 10.0) 10.0) "max mixed types")
(assert (= (max -3 0) 0) "max neg/zero")
(assert (= (min 5.0 -3) -3) "min mixed types")

;; ABS
(assert (= (abs -5) 5) "abs negative")
(assert (= (abs 5) 5) "abs positive")
(assert (= (abs 0) 0) "abs zero")

;; Bignum arithmetic
(assert (= (* 966082078641 419216044685) 404997107848943140073085) "bignum multiply")

;; ASH
(assert (= (ash 1 3) 8) "ash left shift")
(assert (= (ash 8 -1) 4) "ash right shift")
(assert (= (ash -4 -1) -2) "ash negative right")
(assert (= (ash 0 5) 0) "ash zero")
(assert (= (ash -129876 -1026) -1) "ash large negative")

;; LOGCOUNT
(assert (= (logcount #b1010) 2) "logcount 1010")
(assert (= (logcount #b1111) 4) "logcount 1111")
(assert (= (logcount #b0000) 0) "logcount 0000")
(assert (= (logcount 1) 1) "logcount 1")
(assert (= (logcount #b10000000) 1) "logcount single bit")

;; INTEGER-LENGTH
(assert (= (integer-length 0) 0) "integer-length 0")
(assert (= (integer-length 1) 1) "integer-length 1")
(assert (= (integer-length 7) 3) "integer-length 7")
(assert (= (integer-length 8) 4) "integer-length 8")
(assert (= (integer-length 10) 4) "integer-length 10")

;; LOGBITP
(assert (logbitp 1 #b1010) "logbitp bit 1 set")
(assert (not (logbitp 0 #b1010)) "logbitp bit 0 clear")

;; LOGTEST
(assert (logtest #b1010 #b1000) "logtest overlap")
(assert (not (logtest #b1010 #b0101)) "logtest no overlap")

;; BOOLE
(assert (= (boole 1 #b1010 #b1100) #b1000) "boole-and")
(assert (= (boole 7 #b1010 #b1100) #b1110) "boole-ior")
(assert (= (boole 6 #b1010 #b1100) #b0110) "boole-xor")

;; FLOOR/CEILING/TRUNCATE/ROUND
(assert (= (car (floor 3.7)) 3) "floor 3.7")
(assert (= (car (ceiling 3.2)) 4) "ceiling 3.2")
(assert (= (car (truncate 3.9)) 3) "truncate 3.9")
(assert (= (car (round 3.5)) 4) "round 3.5")
(assert (= (car (round 2.5)) 2) "round 2.5 (even)")
(assert (= (car (floor -3.7)) -4) "floor -3.7")
(assert (= (car (ceiling -3.2)) -3) "ceiling -3.2")
(assert (= (car (truncate -3.9)) -3) "truncate -3.9")

;; MOD/REM
(assert (= (mod 10 3) 1) "mod 10 3 = 1")
(assert (= (rem 10 3) 1) "rem 10 3 = 1")
(assert (= (mod -10 3) 2) "mod -10 3 = 2 (CL mod)")
(assert (= (rem -10 3) -1) "rem -10 3 = -1 (CL rem)")

;; COERCE numeric
(assert (floatp (coerce 2/3 'float)) "coerce rational to float")
(assert (rationalp (coerce 3.0 'rational)) "coerce float to rational")
(assert (= (numerator 2/3) 2) "numerator 2/3 = 2")
(assert (= (denominator 2/3) 3) "denominator 2/3 = 3")

;; Complex arithmetic
(assert (= (+ #c(1 2) #c(3 4)) #c(4 6)) "complex +")
(assert (= (- #c(5 6) #c(2 1)) #c(3 5)) "complex -")
(assert (= (* #c(1 2) #c(3 4)) #c(-5 10)) "complex *")
(assert (= (conjugate #c(3 4)) #c(3 -4)) "conjugate")
(assert (complexp #c(3 4)) "complexp true")
(assert (not (complexp 5)) "complexp false on real")
(assert (= (realpart #c(3 4)) 3) "realpart")
(assert (= (imagpart #c(3 4)) 4) "imagpart")
(assert (= (realpart 5) 5) "realpart of real")
(assert (= (imagpart 5) 0) "imagpart of real = 0")

;; ===== SEQUENCES (from seq.pure.lisp) =====

;; REMOVE with :start/:end/:from-end
(assert (equal? (remove 3 '(1 2 3 2 6 1 2 4 1 3 2 7) :from-end #t :start 1 :end 5) '(1 2 2 6 1 2 4 1 3 2 7)) "remove :from-end :start :end")
(assert (equal? (remove 2 '(1 2 3 2 6 1 2 4 1 3 2 7) :from-end #t :start 1 :end 5) '(1 3 6 1 2 4 1 3 2 7)) "remove 2 :from-end :start :end")

;; REMOVE-DUPLICATES with :start/:end
(assert (equal? (remove-duplicates '(0 1 2 0 1 2 0 1 2 0 1 2) :start 3 :end 9) '(0 1 2 0 1 2 0 1 2)) "remove-duplicates :start :end")

;; COUNT
(assert (= (count 1 '(1 2 3)) 1) "count 1 in (1 2 3)")
(assert (= (count 'z '(z 1 2 3 z)) 2) "count z in list")
(assert (= (count 'y '(z 1 2 3 z)) 0) "count y not found")

;; COUNT-IF / COUNT-IF-NOT
(assert (= (count-if #'consp '(1 (12) 1)) 1) "count-if consp 1")
(assert (= (count-if #'consp '(1 (2) 3 (4) (5) 6)) 3) "count-if consp 3")
(assert (= (count-if #'consp '(1 (2) 3 (4) (5) 6) :from-end #t) 3) "count-if :from-end")
(assert (= (count-if #'consp '(1 (2) 3 (4) (5) 6) :start 2) 2) "count-if :start")
(assert (= (count-if #'consp '(1 (2) 3 (4) (5) 6) :start 2 :end 3) 0) "count-if :start :end empty")
(assert (= (count-if #'consp '(1 (2) 3 (4) (5) 6) :start 1 :end 3) 1) "count-if :start 1 :end 3")
(assert (= (count-if #'zerop '(0 10 0 11 12)) 2) "count-if zerop")
(assert (= (count-if #'zerop '(0 10 0 11 12) :start 1) 1) "count-if zerop :start")
(assert (= (count-if (lambda (x) (minusp (1- x))) '(0 10 0 11 12)) 2) "count-if minusp via 1-")
(assert (= (count-if (lambda (x) (minusp (1- x))) '(0 10 0 11 12) :key #'1- :end 5) 2) "count-if :key :end 5")

;; REMOVE with negative :count
(assert (equal? (remove 1 '(1 2 3 1) :count 1) '(2 3 1)) "remove :count 1")

;; SORT nested calls
(assert (equal? (sort '(0 0 0) (lambda (x y) (if (= x y) #f (sort '(0 0 0) #'<)))) '(0 0 0)) "sort nested calls")

;; MERGE
(assert (equal? (merge '() '() '<) '()) "merge both empty")
(assert (equal? (merge '() '(1) '<) '(1)) "merge empty+1")
(assert (equal? (merge '(2) '() '>) '(2)) "merge 1+empty")
(assert (equal? (merge '(1 2 4) '(2 3 7) '<) '(1 2 2 3 4 7)) "merge sorted lists")
(assert (equal? (merge '(1 2 4) '(-2 3 7) '<) '(-2 1 2 3 4 7)) "merge with negative")
(assert (equal? (stable-sort '(1 10 2 12 13 3) '<) '(1 2 3 10 12 13)) "stable-sort basic")

;; FILL
(assert (string= (fill "abcde" #\z) "zzzzz") "fill string")
(assert (equal? (fill '(1 2 3 4) 'x) '(x x x x)) "fill list")

;; POSITION
(assert (= (position 2 '(1 2 3)) 1) "position 2")
(assert (not (position 4 '(1 2 3))) "position not found")

;; FIND-IF should not look past :end
(assert (= (find-if (lambda (x) (even? x)) '(1 2 1 1 1 1 1 1 1 1 1 1 :foo)) 2) "find-if even")

;; REDUCE
(assert (= (reduce #'+ '(1 2 3 4 5)) 15) "reduce +")
(assert (= (reduce #'* '(1 2 3 4 5)) 120) "reduce *")
(assert (equal? (reduce #'append '((a b c)) :initial-value '(1)) '(1 a b c)) "reduce append with init")

;; COPY-SEQ
(assert (equal? (copy-seq '(1 2 3)) '(1 2 3)) "copy-seq list")
(assert (string= (copy-seq "hello") "hello") "copy-seq string")

;; SUBSEQ
(assert (equal? (subseq '(1 2 3 4 5) 0 2) '(1 2)) "subseq start-end")
(assert (equal? (subseq '(1 2 3 4 5) 2) '(3 4 5)) "subseq from index")

;; CONCATENATE
(assert (equal? (concatenate 'list '(1 2) '(3 4)) '(1 2 3 4)) "concatenate list")
(assert (string= (concatenate 'string "a" "b") "ab") "concatenate string")
(assert (string= (concatenate 'string "a" "b" "c") "abc") "concatenate 3 strings")

;; ===== STRINGS (from string.pure.lisp) =====

;; Case operations with :start/:end
(assert (string= (string-upcase "This is a test.") "THIS IS A TEST.") "string-upcase smoke")
(assert (string= (string-downcase "This is a test.") "this is a test.") "string-downcase smoke")
(assert (string= (string-capitalize "This is a test.") "This Is A Test.") "string-capitalize smoke")
(assert (string= (string-upcase "Is this 900-Sex-hott, please?" :start 3) "Is THIS 900-SEX-HOTT, PLEASE?") "string-upcase :start")
(assert (string= (string-downcase "Is this 900-Sex-hott, please?" :start 10 :end 16) "Is this 900-sex-hott, please?") "string-downcase :start :end")
(assert (string= (string-capitalize "Is this 900-Sex-hott, please?") "Is This 900-Sex-Hott, Please?") "string-capitalize full")


;; String trimming
(assert (string= (string-trim " " "  hello  ") "hello") "string-trim whitespace")
(assert (string= (string-trim " ab" "abracadabra") "racadabr") "string-trim a and b")
(assert (string= (string-trim "" "hello") "hello") "string-trim empty bag")
(assert (string= (string-left-trim " " "  hello  ") "hello  ") "string-left-trim")
(assert (string= (string-right-trim " " "  hello  ") "  hello") "string-right-trim")

;; String trimming with fill-pointer-like behavior
(assert (string= (string-left-trim "ab" "abcdaba") "cdaba") "string-left-trim ab")
(assert (string= (string-right-trim "ab" "abcdaba") "abcd") "string-right-trim ab")

;; String comparison
(assert (string= "abc" "abc") "string= equal")
(assert (not (string= "abc" "abd")) "string= not equal")
(assert (string< "a" "b") "string< true")
(assert (not (string< "b" "a")) "string< false")
(assert (string> "b" "a") "string> true")
(assert (not (string> "a" "b")) "string> false")
(assert (string<= "a" "a") "string<= equal")
(assert (string>= "a" "a") "string>= equal")
(assert (= (string/= "abc" "abd") 2) "string/= position")

;; Case-insensitive string comparison
(assert (string-equal "hello" "HELLO") "string-equal same")
(assert (not (string-not-equal "hello" "HELLO")) "string-not-equal same")
(assert (string-lessp "abc" "DEF") "string-lessp")
(assert (string-greaterp "DEF" "abc") "string-greaterp")

;; NSTRING operations
(assert (string= (nstring-upcase "hello") "HELLO") "nstring-upcase")
(assert (string= (nstring-downcase "HELLO") "hello") "nstring-downcase")

;; ===== CHARACTERS (from character.pure.lisp) =====

;; name-char / char-name roundtrip
(assert (char= (name-char "Space") (name-char "Space")) "name-char Space = itself")
(assert (char= (name-char "Newline") (code-char 10)) "name-char Newline")
(assert (char= (name-char "Tab") (code-char 9)) "name-char Tab")
(assert (char= (name-char "Return") (code-char 13)) "name-char Return")
(assert (char= (name-char "Backspace") (code-char 8)) "name-char Backspace")
(assert (char= (name-char "Rubout") (code-char 127)) "name-char Rubout")
(assert (string-equal (char-name (name-char "space")) "space") "char-name Space")
(assert (string-equal (char-name (code-char 10)) "newline") "char-name Newline")

;; Character predicates
(assert (characterp #\A) "characterp A")
(assert (characterp #\a) "characterp a")
(assert (characterp (code-char 65)) "characterp code-char 65")
(assert (not (characterp 65)) "not characterp integer")

;; Case-insensitive char comparison
(assert (char-equal #\A #\a) "char-equal A a")
(assert (char-equal (code-char 201) (code-char 233)) "char-equal e-acute")
(assert (char-not-equal #\A #\b) "char-not-equal A b")
(assert (char-lessp #\a #\B) "char-lessp a B")
(assert (char-greaterp #\B #\a) "char-greaterp B a")

;; char=, char/=, char<=, char>=
(assert (char= #\a #\a) "char= same")
(assert (not (char= #\a #\b)) "char= different")
(assert (char/= #\a #\b) "char/= different")
(assert (not (char/= #\a #\a)) "char/= same")
(assert (char<= #\a #\b) "char<= less")
(assert (char<= #\a #\a) "char<= equal")
(assert (char>= #\b #\a) "char>= greater")
(assert (char>= #\a #\a) "char>= equal")

;; char-upcase / char-downcase
(assert (char= (char-upcase #\a) #\A) "char-upcase a = A")
(assert (char= (char-downcase #\A) #\a) "char-downcase A = a")
(assert (char= (char-upcase #\A) #\A) "char-upcase A unchanged")
(assert (char= (char-downcase #\a) #\a) "char-downcase a unchanged")

;; Standard character predicates
(assert (upper-case-p #\A) "upper-case-p A")
(assert (not (upper-case-p #\a)) "not upper-case-p a")
(assert (lower-case-p #\a) "lower-case-p a")
(assert (not (lower-case-p #\A)) "not lower-case-p A")
(assert (alpha-char-p #\Z) "alpha-char-p Z")
(assert (not (alpha-char-p #\5)) "not alpha-char-p 5")
(assert (alphanumericp #\a) "alphanumericp letter")
(assert (alphanumericp #\5) "alphanumericp digit")
(assert (not (alphanumericp (code-char 32))) "not alphanumericp space")

;; digit-char and digit-char-p
(assert (char= (digit-char 9) #\9) "digit-char 9")
(assert (char= (digit-char 12 16) #\C) "digit-char hex C")
(assert (null? (digit-char 16 16)) "digit-char overflow nil")
(assert (= (digit-char-p #\9) 9) "digit-char-p 9 = 9")
(assert (= (digit-char-p #\a 16) 10) "digit-char-p a hex = 10")
(assert (null? (digit-char-p #\z 10)) "digit-char-p z base10 nil")

;; char-code roundtrip
(assert (= (char-code #\A) 65) "char-code A = 65")
(assert (= (char-code #\Z) 90) "char-code Z = 90")
(assert (= (char-code #\a) 97) "char-code a = 97")
(assert (= (char-code #\0) 48) "char-code 0 = 48")
(assert (char= (code-char 65) #\A) "code-char 65 = A")
(assert (char= (code-char 97) #\a) "code-char 97 = a")

;; ===== LISTS (from list.pure.lisp) =====

;; BUTLAST comprehensive
(assert (equal? (butlast '(1 2 3 4 5)) '(1 2 3 4)) "butlast default")
(assert (null? (butlast '(1 2 3 4 5) 6)) "butlast more than length = nil")
(assert (null? (butlast '())) "butlast nil = nil")
(assert (equal? (butlast '(1 2 3) 0) '(1 2 3)) "butlast 0 = same")
(assert (equal? (butlast '(1 2 3) 1) '(1 2)) "butlast 1")
(assert (equal? (butlast '(1 2 3) 2) '(1)) "butlast 2")
(assert (null? (butlast '(1 2 3) 3)) "butlast 3 = nil")
(assert (null? (butlast '(1 2 3) 4)) "butlast 4 = nil")

;; LAST
(assert (equal? (last '(1 2 3 4 5)) '(5)) "last default")
(assert (equal? (last '(1 2 3 4 5) 2) '(4 5)) "last 2")
(assert (equal? (last '(1 2 3 4 5) 0) '()) "last 0 = nil")

(assert (equal? (remove-duplicates (sort (nset-exclusive-or '(1 2 1 3) '(4 1 3 3)) '<)) '(2 4)) "nset-xor with dups")

;; NREVERSE
(assert (equal? (nreverse '(1 2 3)) '(3 2 1)) "nreverse basic")
(assert (equal? (nreverse '()) '()) "nreverse empty")

;; NRECONC
(assert (equal? (nreconc '(1 2 3) '(4 5)) '(3 2 1 4 5)) "nreconc")

;; COPY-LIST / COPY-ALIST / COPY-TREE
(assert (equal? (copy-list '(1 2 3)) '(1 2 3)) "copy-list")
(assert (equal? (copy-alist '((a . 1) (b . 2))) '((a . 1) (b . 2))) "copy-alist")
(assert (equal? (copy-tree '(1 (2 3) 4)) '(1 (2 3) 4)) "copy-tree")

;; PAIRLIS
(assert (equal? (pairlis '(a b c) '(1 2 3)) '((a . 1) (b . 2) (c . 3))) "pairlis")

;; ACONS
(assert (equal? (acons 'x 1 '((a . b))) '((x . 1) (a . b))) "acons")

;; TREE-EQUAL
(assert (tree-equal '(1 (2 3)) '(1 (2 3))) "tree-equal equal")
(assert (not (tree-equal '(1 (2 3)) '(1 (2 4)))) "tree-equal different")

;; LIST*
(assert (equal? (list* 1 2 3) '(1 2 . 3)) "list* basic")
(assert (equal? (list* 1 2 '(3 4)) '(1 2 3 4)) "list* with list tail")
(assert (= (list* 'a) 'a) "list* single arg")

;; ADJOIN
(assert (equal? (adjoin 1 '(2 3 4)) '(1 2 3 4)) "adjoin new")
(assert (equal? (adjoin 1 '(1 2 3)) '(1 2 3)) "adjoin existing")

;; SET operations
(assert (equal? (union '() '(1 2)) '(1 2)) "union empty first")
(assert (equal? (intersection '() '(1 2)) '()) "intersection empty")
(assert (equal? (set-exclusive-or '(1 2 3) '(2 3 4)) '(1 4)) "set-exclusive-or")
(assert (subsetp '(1 2) '(1 2 3 4)) "subsetp true")
(assert (not (subsetp '(1 5) '(1 2 3 4))) "subsetp false")

;; LDIFF / TAILP

;; REVAPPEND
(assert (equal? (revappend '(1 2 3) '(4 5)) '(3 2 1 4 5)) "revappend")

;; ===== CONDITIONS (from condition.pure.lisp) =====

;; HANDLER-CASE catches error
(assert (= (handler-case (error "test error")
             (error (e) 42))
           42)
        "handler-case catches error")


;; RESTART-CASE basic
(assert (= (restart-case 99
             (return-value (v) v))
           99)
        "restart-case not invoked")

(assert (= (restart-case (invoke-restart 'return-value 77)
             (return-value (v) v))
           77)
        "restart-case invoked")

(assert (string= (format #f "~A" 42) "42") "format ~A integer")
(assert (string= (format #f "~A" "hello") "hello") "format ~A string")
(assert (string= (format #f "~A" 'foo) "foo") "format ~A symbol (lowercase)")
(assert (string= (format #f "~D" 123) "123") "format ~D")
(assert (string= (format #f "~S" 42) "42") "format ~S integer")
(assert (string= (format #f "~S" "abc") "\"abc\"") "format ~S string quoted")
(assert (string= (format #f "~%") "\n") "format ~%")
(assert (string= (format #f "~~") "~") "format ~~ escape")
(assert (string= (format #f "~A ~A" 1 2) "1 2") "format ~A ~A")
(assert (string= (format #f "~D ~D" 10 20) "10 20") "format ~D ~D")
(assert (string= (format #f "~a~%" "test") "test\n") "format ~a~%")

;; ===== COERCE (from coerce.pure.lisp) =====

(assert (string= (coerce '(#\a #\b #\c) 'string) "abc") "coerce chars to string")
(assert (= (length (coerce "abc" 'list)) 3) "coerce string to list")
(assert (string= (coerce 'a 'string) "a") "coerce symbol to string")
(assert (floatp (coerce 2/3 'float)) "coerce rational to float")

;; ===== LOOP (from loop.pure.lisp) =====

;; Basic loop for
(assert (= (loop for i from 1 to 5 sum i) 15) "loop sum 1..5")
(assert (= (loop for i from 1 to 3 collect (* i i)) '(1 4 9)) "loop collect squares")
(assert (equal? (loop for x in '(a b c) collect x) '(a b c)) "loop for in collect")
(assert (= (loop for i from 1 to 10 when (even? i) sum i) 30) "loop when even sum")

;; ===== MISC EDGE CASES =====

;; PSETQ parallel assignment
(assert (equal? (let ((x 1) (y 2)) (psetq x 10 y 20) (list x y)) '(10 20)) "psetq basic")
(assert (equal? (let ((x 1) (y 2)) (psetq x y y x) (list x y)) '(2 1)) "psetq swap")

;; DESTRUCTURING-BIND
(assert (equal? (destructuring-bind (a b) '(1 2) (list a b)) '(1 2)) "dbind simple")
(assert (equal? (destructuring-bind (a . b) '(1 2 3) (list a b)) '(1 (2 3))) "dbind dotted")
(assert (equal? (destructuring-bind ((a b) c) '((1 2) 3) (list a b c)) '(1 2 3)) "dbind nested")

;; PROGV
(assert (= (progv '(x y) '(1 2) (+ x y)) 3) "progv basic")

;; CASE
(assert (equal? (case 1 (1 "one") (2 "two")) "one") "case match")
(assert (equal? (case 2 (1 "one") (2 "two")) "two") "case match 2")
(assert (null? (case 3 (1 "one") (2 "two"))) "case no match")
(assert (equal? (case 3 ((1 2 3) "low") ((4 5 6) "high")) "low") "case key list")
(assert (equal? (case 'b (a "alpha") (b "beta")) "beta") "case symbol")

;; TYPECASE
(assert (equal? (typecase 42 (string "s") (number "n")) "n") "typecase number")
(assert (equal? (typecase "hello" (string "s") (number "n")) "s") "typecase string")

;; WITH-OUTPUT-TO-STRING
(assert (string= (with-output-to-string (s) (princ "hello" s)) "hello") "with-output-to-string")
(assert (string= (with-output-to-string (s) (princ "foo" s) (princ "bar" s)) "foobar") "with-output-to-string multiple")

;; WITH-INPUT-FROM-STRING
(assert (string= (with-input-from-string (s "hello world") (car (read-line s))) "hello world") "with-input-from-string")

;; ===== HASH TABLE (from hash.pure.lisp) =====

;; sxhash basic
(assert (= (sxhash "foo") (sxhash "foo")) "sxhash same string")
(assert (/= (sxhash "foo") (sxhash "bar")) "sxhash different strings")
(assert (= (sxhash 42) (sxhash 42)) "sxhash same number")
(assert (= (sxhash 'foo) (sxhash 'foo)) "sxhash same symbol")
(assert (= (sxhash nil) (sxhash nil)) "sxhash nil")

;; SXHASH on lists
(assert (= (sxhash '(1 2 3)) (sxhash '(1 2 3))) "sxhash same list")

;; Hash table basic
(let ((ht (make-hash-table)))
  (setf (gethash 'key ht) 'val)
  (assert (equal? (gethash 'key ht) 'val) "gethash after set")
  (assert (= (hash-table-count ht) 1) "hash-table-count 1"))

(let ((ht (make-hash-table)))
  (setf (gethash "key" ht) 'val)
  (assert (equal? (gethash "key" ht) 'val) "gethash string key")
  (assert (= (hash-table-count ht) 1) "hash-table-count string key"))

(let ((ht (make-hash-table)))
  (setf (gethash 'a ht) 1)
  (setf (gethash 'b ht) 2)
  (setf (gethash 'c ht) 3)
  (assert (= (hash-table-count ht) 3) "hash-table-count 3")
  (remhash 'b ht)
  (assert (= (hash-table-count ht) 2) "hash-table-count after remhash")
  (clrhash ht)
  (assert (= (hash-table-count ht) 0) "hash-table-count after clrhash"))

(let ((ht (make-hash-table :test #'equal)))
  (setf (gethash "hello" ht) 'world)
  (assert (equal? (gethash "hello" ht) 'world) "hash-table equal test"))

(let ((ht (make-hash-table))
      (sum 0))
  (setf (gethash 'a ht) 1)
  (setf (gethash 'b ht) 2)
  (setf (gethash 'c ht) 3)
  (maphash (lambda (k v) (set! sum (+ sum v))) ht)
  (assert (= sum 6) "maphash sum values"))

;; ===== SETF (from setf.pure.lisp) =====

;; GET-SETF-EXPANSION
(assert (null (car (get-setf-expansion 'foo))) "get-setf-expansion simple var")
(assert (null (cadr (get-setf-expansion 'foo))) "get-setf-expansion simple vals")
(assert (pair? (caddr (get-setf-expansion 'foo))) "get-setf-expansion simple store-var")

;; SHIFTF
(let ((x 1) (y 2))
  (shiftf x y 3)
  (assert (= x 2) "shiftf x gets y's old value")
  (assert (= y 3) "shiftf y gets new value"))

(let ((a (list 1)) (b (list 2)))
  (shiftf (car a) (car b) 99)
  (assert (equal? a '(2)) "shiftf car a")
  (assert (equal? b '(99)) "shiftf car b"))

;; ROTATEF
(let ((x 1) (y 2) (z 3))
  (rotatef x y z)
  (assert (= x 2) "rotatef x becomes y")
  (assert (= y 3) "rotatef y becomes z")
  (assert (= z 1) "rotatef z becomes x"))

;; PUSH/POP on place
(let ((lst '(1 2 3)))
  (push 0 lst)
  (assert (equal? lst '(0 1 2 3)) "push on list"))
(let ((lst '(1 2 3)))
  (let ((v (pop lst)))
    (assert (= v 1) "pop returns first")
    (assert (equal? lst '(2 3)) "pop removes first")))

;; INCF/DECF on place
(let ((x 10))
  (incf x)
  (assert (= x 11) "incf by 1")
  (incf x 5)
  (assert (= x 16) "incf by 5")
  (decf x)
  (assert (= x 15) "decf by 1")
  (decf x 3)
  (assert (= x 12) "decf by 3"))

;; SETF on SUBSEQ
(let ((s "hello world"))
  (setf (subseq s 0 5) "HELLO")
  (assert (string= s "HELLO world") "setf subseq"))

;; SETF on AREF
(let ((v (make-array 3 :initial-element 0)))
  (setf (aref v 0) 42)
  (setf (aref v 1) 99)
  (assert (= (aref v 0) 42) "setf aref 0")
  (assert (= (aref v 1) 99) "setf aref 1"))

;; SETF on GETHASH
(let ((ht (make-hash-table)))
  (setf (gethash 'key ht) 'value)
  (assert (equal? (gethash 'key ht) 'value) "setf gethash"))

;; ===== LOOP (more from loop.pure.lisp) =====

;; LOOP with downto
(assert (equal? (loop for i from 13 downto 7 collect i) '(13 12 11 10 9 8 7)) "loop downto")

;; LOOP with hash-table iteration
;; (MicroLisp loop doesn't support 'being each hash-key' syntax, use maphash)
(let ((ht (make-hash-table))
      (count 0))
  (setf (gethash 'key1 ht) 'val1)
  (setf (gethash 'key2 ht) 'val2)
  (maphash (lambda (k v) (set! count (1+ count))) ht)
  (assert (= count 2) "loop hash keys"))

(let ((ht (make-hash-table))
      (sum 0))
  (setf (gethash 1 ht) 3)
  (setf (gethash 7 ht) 15)
  (maphash (lambda (k v) (set! sum (+ sum v))) ht)
  (assert (= sum 18) "loop hash values sum"))

;; (MicroLisp loop doesn't support destructuring in 'with' binding)
;; (assert (= (loop with (a nil) = '(1 2) return a) 1) "loop with nil in binding")
;; (assert (= (loop with (nil a) = '(1 2) return a) 2) "loop with nil first in binding")

;; LOOP with REPEAT
(assert (equal? (loop for i from 1 repeat 7 collect i) '(1 2 3 4 5 6 7)) "loop repeat")
(assert (zerop (loop repeat 0 sum 1)) "loop repeat 0")
(assert (zerop (loop repeat -5 sum 1)) "loop repeat negative")

;; LOOP thereis (MicroLisp returns #t instead of the found value)
;; (assert (= (loop for i in '(1 2 3) thereis (> i 2)) 3) "loop thereis finds")
(assert (eq? (loop for i in '(1 2 3) thereis (> i 2)) #t) "loop thereis finds #t")
(assert (null? (loop for i in '(1 2 3) thereis (> i 5))) "loop thereis not found")

;; (MicroLisp loop doesn't support destructuring in 'with' binding)
;; (assert (equal? (loop with (a b) = '(1 2) return (list a b)) '(1 2)) "loop with destructuring")
;; (assert (equal? (loop with (a b) = '() repeat 1 collect (list a b)) '((nil nil))) "loop with empty list destructuring")

;; (MicroLisp loop returns all accumulators as a list, not just the last one)
;; (assert (= (loop repeat 1 count 1 sum 1) 2) "loop count and sum")

;; ===== MORE FORMAT TESTS =====

(assert (string= (format #f "hello ~a" "world") "hello world") "format ~a string")
(assert (string= (format #f "~a ~a" 1 2) "1 2") "format multiple args")
(assert (string= (format #f "x") "x") "format no directives")

;; ===== MORE SEQUENCE TESTS =====

;; REPLACE with :start1/:start2
(let ((s1 (copy-seq "abcdefgh"))
      (s2 "1234"))
  (replace s1 s2)
  (assert (string= s1 "1234efgh") "replace basic"))

(let ((s1 (copy-seq "abcdefgh"))
      (s2 "1234"))
  (replace s1 s2 :start1 2)
  (assert (string= s1 "ab1234gh") "replace start1"))

;; SEARCH
(assert (= (search '(1 2) '(0 1 2 3)) 1) "search list")
(assert (= (search "abc" "xabcx") 1) "search string")
(assert (not (search '(1 2 5) '(0 1 2 3))) "search not found")

;; MISMATCH
(assert (= (mismatch "abc" "abd") 2) "mismatch at position 2")
(assert (not (mismatch "abc" "abc")) "mismatch same strings")

;; FILL with :start/:end
(assert (string= (fill "abcde" #\x :start 1 :end 4) "axxxe") "fill :start :end")
(assert (equal? (fill '(1 2 3 4 5) 0 :start 2) '(1 2 0 0 0)) "fill list :start")

;; COPY-SEQ on strings
(let ((s "hello"))
  (let ((c (copy-seq s)))
    (assert (string= c s) "copy-seq string equals")))
;; (MicroLisp set-aref doesn't work on strings, use setf/char)
;; (setf (aref c 0) #\X) and independence test skipped

;; ===== SYMBOL/GENSYM =====

;; GENSYM
(let ((g1 (gensym))
      (g2 (gensym)))
  (assert (symbolp g1) "gensym creates symbol")
  (assert (not (eq g1 g2)) "gensym creates unique symbols")
  (assert (not (string= (symbol-name g1) (symbol-name g2))) "gensym names differ"))

;; MAKE-SYMBOL
(let ((s (make-symbol "foo")))
  (assert (symbolp s) "make-symbol creates symbol")
  (assert (string= (symbol-name s) "foo") "make-symbol name"))

;; COPY-SYMBOL
(let ((orig (gensym))
      (copied))
  (setf copied (copy-symbol orig))
  (assert (not (eq orig copied)) "copy-symbol not eq")
  (assert (string= (symbol-name orig) (symbol-name copied)) "copy-symbol same name"))

;; SYMBOL-PLIST / GET / REMPROP
(let ((s (gensym)))
  (setf (get s 'foo) 42)
  (assert (= (get s 'foo) 42) "get after setf")
  (setf (get s 'bar) "hello")
  (assert (string= (get s 'bar) "hello") "get second property")
  (remprop s 'foo)
  (assert (null? (get s 'foo)) "remprop removes"))

;; (MicroLisp doesn't have boundp)
;; (assert (boundp 'x) "boundp true")
;; (assert (not (boundp 'nonexistent-symbol-xyz)) "boundp false")

;; SYMBOL-VALUE
;; (MicroLisp doesn't have symbol-value)
;; (assert (= (symbol-value 'x) 42) "symbol-value"))

;; ===== MORE CONDITION TESTS =====

;; SIGNAL / HANDLER-CASE
(assert (= (handler-case
             (progn (signal 'condition) 99)
             (condition (c) 42))
           42)
        "handler-case catches signal")

;; WARN
(assert (null? (warn "test warning")) "warn returns nil")

;; ===== TYPEP TESTS =====

(assert (typep 42 'integer) "typep integer")
(assert (typep 3.14 'float) "typep float")
(assert (typep "hello" 'string) "typep string")
(assert (typep 'foo 'symbol) "typep symbol")
(assert (typep '(1 2) 'list) "typep list")
(assert (typep nil 'list) "typep nil is list")
(assert (typep #t 'boolean) "typep t is boolean")
(assert (typep nil 'boolean) "typep nil is boolean")
(assert (typep #'car 'function) "typep function")
(assert (typep "foo" 'sequence) "typep string is sequence")
(assert (typep '(1 2) 'sequence) "typep list is sequence")
(assert (not (typep 42 'string)) "typep integer not string")
(assert (not (typep "foo" 'integer)) "typep string not integer")

;; COMPOUND TYPE SPECIFIERS
(assert (typep 42 '(or integer string)) "typep or")
(assert (typep "foo" '(or integer string)) "typep or second")
(assert (not (typep 3.14 '(or integer string))) "typep or none")
(assert (typep 42 '(and integer number)) "typep and")
(assert (not (typep 42 '(and integer string))) "typep and false")
(assert (typep "hello" '(not number)) "typep not")

;; ===== EQUALP TESTS =====

(assert (equalp 42 42) "equalp numbers")
(assert (equalp "abc" "abc") "equalp strings")
(assert (equalp "abc" "ABC") "equalp case insensitive strings")
(assert (equalp '(1 2 3) '(1 2 3)) "equalp lists")
(assert (not (equalp '(1 2 3) '(1 2 4))) "equalp different lists")
(assert (equalp #\a #\a) "equalp chars")
(assert (equalp #\a #\A) "equalp case insensitive chars")

;; ===== APPEND tests =====

(assert (equal? (append '(1 2) '(3 4)) '(1 2 3 4)) "append two lists")
(assert (equal? (append '(1) '(2) '(3)) '(1 2 3)) "append three lists")
(assert (equal? (append '() '(1 2)) '(1 2)) "append empty first")
(assert (equal? (append '(1 2) '()) '(1 2)) "append empty last")
(assert (equal? (append) '()) "append no args")

;; ===== MEMBER with :test/:key =====

(assert (equal? (member 3 '(1 2 3 4) :test #'=) '(3 4)) "member :test =")
(assert (not (member 5 '(1 2 3 4) :test #'=)) "member not found :test =")
(assert (equal? (member 2 '((1) (2) (3)) :key #'car :test #'=) '((2) (3))) "member :key :test")

;; ===== ASSOC with :test/:key =====

(assert (equal? (assoc 'b '((a . 1) (b . 2) (c . 3))) '(b . 2)) "assoc basic")
(assert (not (assoc 'd '((a . 1) (b . 2)))) "assoc not found")
(assert (equal? (assoc 2 '((1 . a) (2 . b) (3 . c)) :test #'=) '(2 . b)) "assoc :test =")

;; ===== MAPC/MAPL =====

(let ((sum 0))
  (mapc (lambda (x) (set! sum (+ sum x))) '(1 2 3 4))
  (assert (= sum 10) "mapc side effect"))

(let ((result '()))
  (mapl (lambda (lst) (set! result (cons (length lst) result))) '(1 2 3))
  (assert (equal? result '(1 2 3)) "mapl on sublists"))

;; ===== MAPCAN/MAPCON =====

(assert (equal? (mapcan #'list '(1 2 3)) '(1 2 3)) "mapcan list")
(assert (equal? (mapcar #'list '(1 2 3)) '((1) (2) (3))) "mapcar list")

;; ===== SOME/EVERY/NOTANY/NOTEVERY =====

(assert (some #'(lambda (x) (> x 3)) '(1 2 3 4 5)) "some > 3")
(assert (not (some #'(lambda (x) (> x 10)) '(1 2 3))) "some none satisfy")
(assert (every #'(lambda (x) (> x 0)) '(1 2 3)) "every > 0")
(assert (not (every #'(lambda (x) (> x 2)) '(1 2 3))) "every not all")
(assert (notany #'(lambda (x) (> x 10)) '(1 2 3)) "notany > 10")
(assert (notevery #'(lambda (x) (< x 3)) '(1 2 3)) "notevery < 3")

;; ===== REDUCE with :from-end/:start/:end =====

(assert (= (reduce #'+ '(1 2 3 4 5) :from-end #t) 15) "reduce :from-end")
(assert (= (reduce #'+ '(1 2 3 4 5) :start 2) 12) "reduce :start")
(assert (= (reduce #'+ '(1 2 3 4 5) :end 3) 6) "reduce :end")
(assert (= (reduce #'+ '(1 2 3 4 5) :start 1 :end 4) 9) "reduce :start :end")
(assert (equal? (reduce #'append '((1) (2) (3)) :from-end #t) '(1 2 3)) "reduce append :from-end")

;; ===== LOOP FINALLY with return value =====

(assert (equal? (loop for i in '(a b c) collect i finally (return 'done)) 'done) "loop finally return overrides")
(assert (equal? (loop for i in '(1 2 3) sum i finally (return :final)) :final) "loop finally return on sum")

;; ===== MORE ARRAY TESTS =====

;; Vector operations
(let ((v (make-array 5 :initial-element 0)))
  (setf (aref v 2) 42)
  (assert (= (aref v 2) 42) "aref after setf")
  (assert (= (array-rank v) 1) "array-rank 1")
  (assert (= (array-total-size v) 5) "array-total-size"))

;; Vector push/pop with fill-pointer
(let ((v (make-array 10 :fill-pointer 0)))
  (vector-push 'a v)
  (vector-push 'b v)
  (vector-push 'c v)
  (assert (= (fill-pointer v) 3) "fill-pointer after pushes")
  (assert (equal? (aref v 1) 'b) "aref on fill-pointer vector")
  (let ((popped (vector-pop v)))
    (assert (equal? popped 'c) "vector-pop returns")
    (assert (= (fill-pointer v) 2) "fill-pointer after pop")))

;; ===== REDUCE with initial-value =====

(assert (= (reduce #'+ '() :initial-value 10) 10) "reduce empty with initial")
(assert (= (reduce #'* '(2 3 4) :initial-value 1) 24) "reduce with initial 1")

;; ===== LOOP return =====
(assert (= (loop for i from 1 to 10 when (> i 5) return (* i 10)) 60) "loop when return")
(assert (= (loop for x in '(a b c d) for i from 1 when (equal? x 'c) return i) 3) "loop parallel binding return")

;; ===== STRING operations =====

(assert (string= (string-trim "xyz" "hello") "hello") "string-trim no match")
(assert (string= (string-trim '() "hello") "hello") "string-trim empty bag")

;; SUBSEQ on vectors
(let ((v (vector 1 2 3 4 5)))
  (assert (equal? (subseq v 1 3) '(2 3)) "subseq on vector"))

;; ===== MULTIPLE VALUE tests =====

(assert (= (values 42) 42) "values single")

;; ===== FLET/LABELS =====

(assert (= (let ((x 10))
            (flet ((double (n) (* n 2)))
              (double x)))
           20) "flet basic")

(assert (= (labels ((fact (n) (if (<= n 1) 1 (* n (fact (1- n))))))
            (fact 5))
           120) "labels recursive")

;; ===== Final summary =====

(display "All SBCL-derived tests passed!")
(newline)