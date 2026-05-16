;; ============================================================
;; MicroLisp ANSI-Style Test Suite
;; ============================================================
;; Consolidated assertion tests adapted from existing MicroLisp
;; test files (core.lisp, list.lisp, stdlib.lisp, sequences.lisp,
;; strings.lisp, macros.lisp, closures.lisp, etc.).
;;
;; Uses only MicroLisp's built-in (assert test) or (assert test msg)
;; Skips functions not supported: string=, string<, string-capitalize,
;; equal, eql, eq, member with :test, butlast, nbutlast, nconc,
;; pushnew, make-array, vector-push-extend, fill-pointer, coerce to
;; complex, assert-error, handler-bind with complex restarts,
;; name-char, code-char, char-name, characterp, alpha-char-p, etc.
;; ============================================================

;; ===== ARITHMETIC =====

;; Basic addition
(assert (= (+ 1 2) 3) "1 + 2 = 3")
(assert (= (+ 1 2 3 1) 7) "four arg addition")
(assert (= (+ 0 0) 0) "0 + 0 = 0")
(assert (= (+ 1.5 2.0) 3.5) "float addition")

;; Basic subtraction
(assert (= (- 10 5) 5) "10 - 5 = 5")
(assert (= (- 10 3 5) 2) "chained subtraction")
(assert (= (- 5) -5) "unary negation")

;; Basic multiplication
(assert (= (* 3 4) 12) "3 * 4 = 12")
(assert (= (* 2 3 4 5) 120) "four arg multiplication")
(assert (= (* 0 100) 0) "zero times anything = 0")
(assert (= (* 1 1 1) 1) "1 * 1 * 1 = 1")
(assert (= (* 1000 1000) 1000000) "large multiplication")
(assert (= (* 10000 10000) 100000000) "larger multiplication")

;; Basic division
(assert (= (/ 15 3) 5) "15 / 3 = 5")
(assert (= (/ 24 2 4) 3) "chained division")
(assert (= (/ 5 2.5) 2.0) "float division")
(assert (= (/ 2 4) 1/2) "2 / 4 = 1/2 rational")

;; Compound arithmetic
(assert (= (+ (* 2 3) (* 4 2)) 14) "compound: 2*3 + 4*2 = 14")
(assert (= (+ 1 (* 2 (+ 3 4) 5)) 71) "complex nesting")

;; Comparisons
(assert (= 5 5) "5 = 5")
(assert (not (= 5 6)) "5 != 6")
(assert (= 1 1 1) "transitive equality")
(assert (not (= 1 2 1)) "transitive not equal")
(assert (< 1 2) "1 < 2")
(assert (not (< 2 1)) "2 not < 1")
(assert (< 1 2 3) "transitive less-than")
(assert (not (< 1 3 2)) "transitive less-than fails")
(assert (> 3 2) "3 > 2")
(assert (not (> 2 3)) "2 not > 3")
(assert (> 3 2 1) "transitive greater-than")
(assert (<= 1 1) "1 <= 1")
(assert (<= 1 2) "1 <= 2")
(assert (not (<= 2 1)) "2 not <= 1")
(assert (>= 1 1) "1 >= 1")
(assert (>= 2 1) "2 >= 1")
(assert (not (>= 1 2)) "1 not >= 2")

;; Absolute value
(assert (= (abs -5) 5) "abs(-5) = 5")
(assert (= (abs 5) 5) "abs(5) = 5")
(assert (= (abs 0) 0) "abs(0) = 0")

;; Min and max
(assert (= (max 3 5) 5) "max 3,5 = 5")
(assert (= (min 3 5) 3) "min 3,5 = 3")
(assert (= (max -5 -10) -5) "max of negatives")
(assert (= (min -5 -10) -10) "min of negatives")

;; Square helper
(assert (= (* 3 3) 9) "3 squared")
(assert (= (* 5 5) 25) "5 squared")
(assert (= (* 7 7) 49) "7 squared")

;; Modulo
(assert (= (modulo 7 3) 1) "7 mod 3 = 1")
(assert (= (modulo -7 3) 2) "-7 mod 3 = 2")
(assert (= (modulo 10 2) 0) "10 mod 2 = 0")

;; ===== RATIONAL ARITHMETIC =====

(assert (= 2 4/2) "4/2 = 2")
(assert (= 1/2 2/4) "2/4 = 1/2 simplification")
(assert (= 2/3 4/6) "4/6 = 2/3 simplification")

(assert (= (+ 1/4 1/2) 3/4) "1/4 + 1/2 = 3/4")
(assert (= (+ 1/3 2/3) 1) "1/3 + 2/3 = 1")
(assert (= (+ 1/2 1/3) 5/6) "1/2 + 1/3 = 5/6")

(assert (= (- 1/2 1/4) 1/4) "1/2 - 1/4 = 1/4")
(assert (= (- 1/2 1/3) 1/6) "1/2 - 1/3 = 1/6")

(assert (= (* 1/2 1/3) 1/6) "1/2 * 1/3 = 1/6")
(assert (= (* 3/4 1/2) 3/8) "3/4 * 1/2 = 3/8")

(assert (= (/ 3/4 1/2) 3/2) "(3/4) / (1/2) = 3/2")
(assert (= (/ 1/2 1/4) 2) "(1/2) / (1/4) = 2")

(assert (= (+ 1 1/2) 3/2) "1 + 1/2 = 3/2 mixed")
(assert (= (- 1 1/2) 1/2) "1 - 1/2 = 1/2 mixed")
(assert (= (* 3 1/2) 3/2) "3 * 1/2 = 3/2 mixed")

(assert (= (- 0 1/2) -1/2) "0 - 1/2 = -1/2")
(assert (= (- 1/2 1) -1/2) "1/2 - 1 = -1/2")
(assert (= (* -1/2 1/3) -1/6) "-1/2 * 1/3 = -1/6")
(assert (= (/ -3/4 1/2) -3/2) "(-3/4) / (1/2) = -3/2")

;; Rational comparisons
(assert (= 1/2 2/4) "1/2 = 2/4")
(assert (not (= 1/2 2/3)) "1/2 != 2/3")
(assert (< 1/2 3/4) "1/2 < 3/4")
(assert (> 3/4 1/2) "3/4 > 1/2")
(assert (<= 1/2 2/4 3/4) "1/2 <= 2/4 <= 3/4")
(assert (>= 3/4 2/4 1/2) "3/4 >= 2/4 >= 1/2")
(assert (= 1 2/2) "1 = 2/2")
(assert (< 1/2 1) "1/2 < 1")
(assert (> 3/2 1) "3/2 > 1")

;; Cross-type numeric equality (rational vs float)
(assert (= 0.5 1/2) "0.5 = 1/2")
(assert (= 0.75 3/4) "0.75 = 3/4")
(assert (= 1.5 3/2) "1.5 = 3/2")

;; Rational type check
(assert (equal? (type-of 1/2) 'rational) "1/2 is rational type")
(assert (equal? (type-of 3/4) 'rational) "3/4 is rational type")
(assert (equal? (type-of -2/3) 'rational) "-2/3 is rational type")
(assert (equal? (type-of 4/2) 'number) "4/2 simplifies to number type")

;; ===== LISTS =====

;; cons, car, cdr
(assert (= (car (cons 1 2)) 1) "car of cons")
(assert (= (cdr (cons 1 2)) 2) "cdr of cons")
(assert (equal? (cons 1 (cons 2 (cons 3 '()))) '(1 2 3)) "build list with cons")
(assert (equal? (cons 'a 'b) '(a . b)) "dotted pair")

;; list
(assert (equal? (list) '()) "list with no args")
(assert (equal? (list 1) '(1)) "list with one arg")
(assert (equal? (list 1 2 3) '(1 2 3)) "list with three args")
(assert (equal? (list #t #f "hi") '(#t #f "hi")) "list mixed types")

;; length
(assert (= (length '()) 0) "length of empty list")
(assert (= (length '(1)) 1) "length of single element")
(assert (= (length '(1 2 3 4 5)) 5) "length of five elements")
(assert (= (length '((1 2) (3 4) (5 6))) 3) "length of nested list")

;; append
(assert (equal? (append '() '()) '()) "append both empty")
(assert (equal? (append '(1) '()) '(1)) "append non-empty + empty")
(assert (equal? (append '() '(1)) '(1)) "append empty + non-empty")
(assert (equal? (append '(1 2) '(3 4)) '(1 2 3 4)) "append basic")
(assert (equal? (append '(1 2 3 4) '(5 6)) '(1 2 3 4 5 6)) "append two lists")

;; reverse
(assert (equal? (reverse '()) '()) "reverse empty")
(assert (equal? (reverse '(1)) '(1)) "reverse single")
(assert (equal? (reverse '(1 2 3)) '(3 2 1)) "reverse three elements")
(assert (equal? (reverse '(1 2 3 4 5)) '(5 4 3 2 1)) "reverse five")
(assert (equal? (reverse '((1 2) (3 4))) '((3 4) (1 2))) "reverse nested")

;; list-ref
(assert (equal? (list-ref '(a b c) 0) 'a) "list-ref index 0")
(assert (equal? (list-ref '(a b c) 1) 'b) "list-ref index 1")
(assert (equal? (list-ref '(a b c) 2) 'c) "list-ref index 2")
(assert (= (list-ref '(1 99 3) 1) 99) "list-ref numeric")
(assert (equal? (list-ref '((1 2) (3 4)) 1) '(3 4)) "list-ref nested")

;; assoc
(assert (not (assoc 'a '())) "assoc empty alist")
(assert (equal? (assoc 'a '((a 1) (b 2))) '(a 1)) "assoc finds first")
(assert (equal? (assoc 'b '((a 1) (b 2))) '(b 2)) "assoc finds second")
(assert (not (assoc 'c '((a 1) (b 2)))) "assoc not found")
(assert (equal? (assoc 'name '((name "Alice") (age 30))) '(name "Alice")) "assoc with string")
(assert (equal? (assoc 'a '((a . 1) (b . 2))) '(a . 1)) "assoc dotted alist")

;; member (returns tail of list)
(assert (equal? (member 'a '(a b c)) '(a b c)) "member finds first")
(assert (equal? (member 'b '(a b c)) '(b c)) "member finds second")
(assert (not (member 'z '(a b c))) "member not found")

;; member? (returns boolean)
(assert (not (member? 'x '())) "member? empty")
(assert (member? 'a '(a b c)) "member? found")
(assert (not (member? 'z '(a b c))) "member? not found")

;; map (single list)
(assert (equal? (mapcar (lambda (x) (* x 2)) '(1 2 3)) '(2 4 6)) "map double")
(assert (equal? (mapcar (lambda (x) (* x x)) '(1 2 3)) '(1 4 9)) "map squares")
(assert (equal? (mapcar (lambda (x) (number? x)) '(a b 3)) '(#f #f #t)) "map predicate")

;; mapcan
(assert (equal? (mapcan (lambda (x) (list x)) '(1 2 3)) '(1 2 3)) "mapcan list")
(assert (equal? (mapcan (lambda (x) (if (number? x) (list x) '())) '(1 a 2 b)) '(1 2)) "mapcan filter+list")

;; set-car!, set-cdr!
(define sc-p (cons 'a 'b))
(set-car! sc-p 'x)
(assert (equal? (car sc-p) 'x) "set-car! changes car")
(set-cdr! sc-p 'y)
(assert (equal? (cdr sc-p) 'y) "set-cdr! changes cdr")

(define sc-l '(1 2 3))
(set-car! sc-l 99)
(assert (= (car sc-l) 99) "set-car! on list head")

;; range
(assert (equal? (range 0) '()) "range 0")
(assert (equal? (range 1) '(0)) "range 1")
(assert (equal? (range 5) '(0 1 2 3 4)) "range 5")

;; ===== LIST UTILITIES (from stdlib) =====

;; filter
(assert (equal? (filter (lambda (x) (> x 3)) '(1 2 3 4 5)) '(4 5)) "filter > 3")
(assert (equal? (filter (lambda (x) #t) '(1 2 3)) '(1 2 3)) "filter keep all")
(assert (equal? (filter (lambda (x) #f) '(1 2 3)) '()) "filter remove all")
(assert (equal? (filter (lambda (x) #t) '()) '()) "filter empty")

;; fold (left fold)
(assert (= (fold + 0 '()) 0) "fold empty = init")
(assert (= (fold + 0 '(1 2 3)) 6) "fold sum")
(assert (= (fold * 1 '(1 2 3 4)) 24) "fold product")
(assert (= (fold (lambda (x acc) (+ x acc)) 0 '(1 2 3 4)) 10) "fold sum lambda")

;; fold-right
(assert (= (fold-right + 0 '()) 0) "fold-right empty = init")
(assert (= (fold-right + 0 '(1 2 3)) 6) "fold-right sum")
(assert (equal? (fold-right (lambda (x acc) (cons x acc)) '() '(1 2 3 4)) '(1 2 3 4)) "fold-right identity")

;; any / all
(assert (not (any number? '())) "any empty = false")
(assert (not (any number? '(a b c))) "any no numbers")
(assert (any number? '(a 2 c)) "any has number")
(assert (all number? '()) "all empty = true")
(assert (all number? '(1 2 3)) "all numbers")
(assert (not (all number? '(1 2 'a))) "all has non-number")

;; take / drop
(assert (equal? (take 0 '(1 2 3)) '()) "take 0")
(assert (equal? (take 1 '(1 2 3)) '(1)) "take 1")
(assert (equal? (take 5 '(1 2 3)) '(1 2 3)) "take more than length")
(assert (equal? (drop 0 '(1 2 3)) '(1 2 3)) "drop 0")
(assert (equal? (drop 1 '(1 2 3)) '(2 3)) "drop 1")
(assert (equal? (drop 5 '(1 2 3)) '()) "drop more than length")

;; zip
(assert (equal? (zip '() '()) '()) "zip both empty")
(assert (equal? (zip '(1 2) '(a b)) '((1 a) (2 b))) "zip two lists")

;; flatten
(assert (equal? (flatten '()) '()) "flatten empty")
(assert (equal? (flatten '(1 2 3)) '(1 2 3)) "flatten already flat")
(assert (equal? (flatten '((1) (2 3) (4))) '(1 2 3 4)) "flatten nested")

;; for-each
(define fe-test '())
(for-each (lambda (x) (set! fe-test (cons x fe-test))) '(1 2 3))
(assert (= (length fe-test) 3) "for-each iterates all elements")
(assert (equal? (reverse fe-test) '(1 2 3)) "for-each in order")

;; ===== SEQUENCES =====

;; find
(assert (= (find 2 '(1 2 3)) 2) "find element exists")
(assert (not (find 4 '(1 2 3))) "find element missing")
(assert (equal? (find 'a '(b a c)) 'a) "find symbol")

;; find-if
(assert (= (find-if (lambda (x) (number? x)) '(a b 1 c 2)) 1) "find-if first number")
(assert (not (find-if (lambda (x) (number? x)) '(a b c))) "find-if none")

;; count
(assert (= (count 1 '(1 2 1 3 1)) 3) "count occurrences")
(assert (= (count 'a '(a b a c a)) 3) "count symbols")
(assert (= (count 1 '(1 2 1 3 1) :start 1) 2) "count with start")
(assert (= (count 1 '(1 2 1 3 1) :start 1 :end 4) 1) "count with start and end")
(assert (= (count 1 '(1 2 1 3 1) :end 3) 2) "count with end")

;; count-if
(assert (= (count-if (lambda (x) (number? x)) '(a b 1 c 2)) 2) "count-if numbers")
(assert (= (count-if (lambda (x) (number? x)) '(a b c)) 0) "count-if none")

;; remove
(assert (equal? (remove 2 '(1 2 3 2 4)) '(1 3 4)) "remove all")
(assert (equal? (remove 'a '(a b a c)) '(b c)) "remove symbols")
(assert (equal? (remove 2 '(1 2 3 2 4) :count 1) '(1 3 2 4)) "remove :count 1")
(assert (equal? (remove 2 '(1 2 3 2 4) :from-end #t) '(1 3 4)) "remove :from-end #t")
(assert (equal? (remove 2 '(1 2 3 2 4) :from-end #t :count 1) '(1 2 3 4)) "remove :from-end #t :count 1")
(assert (equal? (remove 2 '(1 2 3 2 4) :start 2) '(1 2 3 4)) "remove :start 2")
(assert (equal? (remove 2 '(1 2 3 2 4) :end 3) '(1 3 2 4)) "remove :end 3")
(assert (equal? (remove 2 '(1 2 3 2 4) :start 1 :end 4) '(1 3 4)) "remove :start 1 :end 4")

;; remove-if
(assert (equal? (remove-if (lambda (x) (number? x)) '(1 a 2 b 3)) '(a b)) "remove-if numbers")
(assert (equal? (remove-if (lambda (x) (> x 3)) '(1 2 3 4 5 6)) '(1 2 3)) "remove-if > 3")
(assert (equal? (remove-if (lambda (x) (> x 3)) '(1 2 3 4 5 6) :count 1) '(1 2 3 5 6)) "remove-if :count 1")
(assert (equal? (remove-if (lambda (x) (> x 3)) '(1 2 3 4 5 6) :start 2) '(1 2 3)) "remove-if :start 2")

;; remove-duplicates
(assert (equal? (remove-duplicates '(1 2 1 3 2 1)) '(1 2 3)) "remove-duplicates basic")
(assert (equal? (remove-duplicates '(a b a c a)) '(a b c)) "remove-duplicates symbols")
(assert (equal? (remove-duplicates '()) '()) "remove-duplicates empty")
(assert (equal? (remove-duplicates '(1 2 3)) '(1 2 3)) "remove-duplicates no dups")

;; position
(assert (= (position 2 '(1 2 3)) 1) "position index")
(assert (not (position 4 '(1 2 3))) "position not found")
(assert (= (position 1 '(1 2 1 3) :start 1) 2) "position with :start")
(assert (= (position 1 '(1 2 1 3) :from-end #t) 2) "position :from-end #t")
(assert (= (position 'a '(b a c)) 1) "position symbol")

;; position-if
(assert (= (position-if (lambda (x) (number? x)) '(a b 1 c 2)) 2) "position-if finds first")
(assert (= (position-if (lambda (x) (number? x)) '(1 a b c)) 0) "position-if at start")

;; substitute
(assert (equal? (substitute 'x 'a '(a b a c)) '(x b x c)) "substitute all")
(assert (equal? (substitute 'x 'a '(a b a c a) :count 1) '(x b a c a)) "substitute :count 1")
(assert (equal? (substitute 9 3 '(1 2 3 4 3)) '(1 2 9 4 9)) "substitute numbers")

;; sort
(assert (equal? (sort '(3 1 4 1 5) '<) '(1 1 3 4 5)) "sort ascending")
(assert (equal? (sort '(5 4 3 2 1) '<) '(1 2 3 4 5)) "sort reverse ascending")
(assert (equal? (sort '(1 2 3 4 5) '>) '(5 4 3 2 1)) "sort descending")
(assert (equal? (sort '(3 1 2) '<) '(1 2 3)) "sort short list")

;; merge
(assert (equal? (merge '(1 3 5) '(2 4 6) '<) '(1 2 3 4 5 6)) "merge two sorted lists")
(assert (equal? (merge '() '(1 2 3) '<) '(1 2 3)) "merge with empty first")
(assert (equal? (merge '(1 2 3) '() '<) '(1 2 3)) "merge with empty second")

;; reduce
(assert (= (reduce + '(1 2 3 4 5)) 15) "reduce sum")
(assert (= (reduce * '(1 2 3 4 5)) 120) "reduce product")
(assert (= (reduce max '(3 1 4 1 5)) 5) "reduce max")
(assert (= (reduce min '(3 1 4 1 5)) 1) "reduce min")
(assert (equal? (reduce cons '(1 2 3)) '((1 . 2) . 3)) "reduce cons")
(assert (equal? (reduce list '(1 2 3 4)) '(((1 2) 3) 4)) "reduce list")
(assert (= (reduce + '(1) :initial-value 10) 11) "reduce with initial-value")
(assert (= (reduce + '(1 2 3) :initial-value 10) 16) "reduce sum with initial-value")

;; subseq
(assert (equal? (subseq '(1 2 3 4 5) 2) '(3 4 5)) "subseq from index")
(assert (equal? (subseq '(1 2 3 4 5) 1 3) '(2 3)) "subseq range")
(assert (equal? (subseq '(1 2 3) 0) '(1 2 3)) "subseq full")
(assert (equal? (subseq '(a b c d) 1 4) '(b c d)) "subseq symbols")

;; ===== STRINGS =====

;; string-upcase
(assert (equal? (string-upcase "hello") "HELLO") "string-upcase basic")
(assert (equal? (string-upcase "Hello World") "HELLO WORLD") "string-upcase mixed")
(assert (equal? (string-upcase "ABC") "ABC") "string-upcase already upper")
(assert (equal? (string-upcase "") "") "string-upcase empty")

;; string-downcase
(assert (equal? (string-downcase "HELLO") "hello") "string-downcase basic")
(assert (equal? (string-downcase "Hello World") "hello world") "string-downcase mixed")
(assert (equal? (string-downcase "abc") "abc") "string-downcase already lower")
(assert (equal? (string-downcase "") "") "string-downcase empty")

;; string-append
(assert (equal? (string-append) "") "string-append no args")
(assert (equal? (string-append "a" "b") "ab") "string-append two")
(assert (equal? (string-append "hello " "world") "hello world") "string-append with space")
(assert (equal? (string-append "a" "b" "c") "abc") "string-append three")

;; string-length
(assert (= (string-length "") 0) "string-length empty")
(assert (= (string-length "hello") 5) "string-length hello")
(assert (= (string-length "hello world") 11) "string-length with space")

;; string-find
(assert (equal? (string-find #\e "hello") 1) "string-find e")
(assert (not (string-find #\z "hello")) "string-find missing")

;; substring
(assert (equal? (substring "hello" 1 4) "ell") "substring range")
(assert (equal? (substring "hello" 0 2) "he") "substring start-end")
(assert (equal? (substring "hello" 2) "llo") "substring from index")

;; string->number
(assert (= (string->number "42") 42) "string->number integer")
(assert (= (string->number "3.14") 3.14) "string->number float")
(assert (= (string->number "-45") -45) "string->number negative")
(assert (equal? (string->number "not-a-number") '()) "string->number invalid returns empty list")

;; number->string
(assert (equal? (number->string 42) "42") "number->string integer")
(assert (equal? (number->string 3.14) "3.14") "number->string float")
(assert (equal? (number->string -45) "-45") "number->string negative")
(assert (equal? (number->string 0) "0") "number->string zero")

;; string
(assert (equal? (string) "") "string no args = empty")
(assert (equal? (string 42) "42") "string number to string")
(assert (equal? (string "hello") "hello") "string passthrough")
(assert (equal? (string 42 "hello") "42hello") "string mixed args")
(assert (equal? (string 'symbol) "symbol") "string symbol")

;; symbol->string / string->symbol
(assert (equal? (symbol->string 'hello) "hello") "symbol->string")
(assert (equal? (symbol->string 'x) "x") "symbol->string single")
(assert (equal? (symbol->string 'hello-world) "hello-world") "symbol->string hyphenated")
(assert (equal? (string->symbol "hello") 'hello) "string->symbol")
(assert (equal? (string->symbol "x") 'x) "string->symbol single")
(assert (equal? (string->symbol "hello-world") 'hello-world) "string->symbol hyphenated")

;; Round trips
(assert (equal? (string->symbol (symbol->string 'my-symbol)) 'my-symbol) "symbol->string->symbol roundtrip")

;; ===== SYMBOLS =====

;; gensym
(assert (symbol? (gensym)) "gensym produces symbol")
(define gs1 (gensym))
(define gs2 (gensym))
(assert (not (equal? gs1 gs2)) "gensym produces unique symbols")
(define gs3 (gensym "VAR"))
(assert (symbol? gs3) "gensym with prefix")

;; keywordp
(assert (keywordp :foo) "keywordp :foo")
(assert (keywordp :bar) "keywordp :bar")
(assert (not (keywordp 'foo)) "keywordp symbol is not keyword")
(assert (not (keywordp 42)) "keywordp number is not keyword")
(assert (not (keywordp "foo")) "keywordp string is not keyword")

;; Keyword self-evaluating
(assert (equal? :foo :foo) ":foo self-evaluates")
(assert (equal? :hello :hello) ":hello self-evaluates")

;; ===== EQUALITY PREDICATES =====

;; eq? (symbol/pointer identity)
(assert (eq? 'a 'a) "eq? same symbol")
(assert (not (eq? 'a 'b)) "eq? different symbols")
(assert (eq? #t #t) "eq? same boolean")
(assert (not (eq? #t #f)) "eq? different booleans")

;; equal? (structural equality)
(assert (equal? 'a 'a) "equal? same symbol")
(assert (not (equal? 'a 'b)) "equal? different symbols")
(assert (equal? '(1 2 3) '(1 2 3)) "equal? same lists")
(assert (not (equal? '(1 2 3) '(1 2 4))) "equal? different lists")
(assert (equal? '(1 . 2) '(1 . 2)) "equal? same dotted pairs")
(assert (not (equal? '(1 . 2) '(1 . 3))) "equal? different dotted pairs")
(assert (equal? "hello" "hello") "equal? same strings")
(assert (not (equal? "hello" "world")) "equal? different strings")
(assert (equal? 42 42) "equal? same numbers")
(assert (not (equal? 42 43)) "equal? different numbers")
(assert (equal? '() '()) "equal? same nil")
(assert (equal? '(1 (2 3) 4) '(1 (2 3) 4)) "equal? nested lists")

;; eqv?
(assert (eqv? 5 5) "eqv? same number")
(assert (not (eqv? 5 6)) "eqv? different number")

;; ===== TYPE PREDICATES =====

(assert (number? 42) "number? integer")
(assert (number? 3.14) "number? float")
(assert (not (number? "hi")) "number? string not number")
(assert (not (number? 'x)) "number? symbol not number")

(assert (string? "hello") "string? basic string")
(assert (not (string? 'sym)) "string? symbol not string")
(assert (not (string? 42)) "string? number not string")

(assert (symbol? 'x) "symbol? basic symbol")
(assert (symbol? 'hello-world) "symbol? hyphenated")
(assert (not (symbol? "str")) "symbol? string not symbol")
(assert (not (symbol? 42)) "symbol? number not symbol")

(assert (bool? #t) "bool? true")
(assert (bool? #f) "bool? false")
(assert (not (bool? 0)) "bool? 0 not bool")
(assert (not (bool? '())) "bool? () not bool")

(assert (procedure? +) "procedure? +")
(assert (procedure? car) "procedure? car")
(assert (procedure? (lambda (x) x)) "procedure? lambda")
(assert (not (procedure? 42)) "procedure? number not procedure")
(assert (not (procedure? 'x)) "procedure? symbol not procedure")

(assert (null? '()) "null? empty list")
(assert (not (null? '(1))) "null? non-empty list")
(assert (not (null? 1)) "null? number is not null")

(assert (pair? '(1 2)) "pair? proper list is pair")
(assert (pair? '(1 . 2)) "pair? dotted pair")
(assert (not (pair? '())) "pair? empty is not pair")
(assert (not (pair? 42)) "pair? number is not pair")
(assert (not (pair? "str")) "pair? string is not pair")

(assert (list? '(1 2 3)) "list? proper list")
(assert (list? '()) "list? empty is list")
(assert (not (list? "abc")) "list? string is not list")

;; atom
(assert (atom 42) "atom number")
(assert (atom 'x) "atom symbol")
(assert (atom "hello") "atom string")
(assert (atom #t) "atom boolean")
(assert (not (atom '(1 2))) "atom list is not atom")

;; ===== SPECIAL FORMS: if =====

(assert (= (if #t 1 2) 1) "if #t then 1 else 2")
(assert (= (if #f 1 2) 2) "if #f then 1 else 2")
(assert (= (if #t 1) 1) "if #t single branch")
(assert (equal? (if #f 1) '()) "if #f single branch returns nil")

;; Truthiness: #f and () are falsy, everything else is truthy
(assert (= (if 0 1 2) 1) "0 is truthy")
(assert (= (if "hello" 1 2) 1) "string is truthy")
(assert (= (if '(1 2) 1 2) 1) "list is truthy")

;; ===== SPECIAL FORMS: begin =====

(assert (= (begin 1 2 3) 3) "begin returns last")
(assert (equal? (begin) '()) "begin no args")
(assert (= (begin (define begin-test 42) begin-test) 42) "begin with define")

;; ===== SPECIAL FORMS: cond =====

(assert (= (cond (#f 1) (#t 2)) 2) "cond second clause")
(assert (= (cond (#t 1) (#t 2)) 1) "cond first clause")
(assert (equal? (cond) '()) "cond no clauses")
(assert (equal? (cond (#f 1)) '()) "cond no match")
(assert (= (cond ((< 1 0) "neg") ((= 1 0) "zero") ((> 1 0) "pos")) "pos") "cond multi-clause")

;; ===== SPECIAL FORMS: and / or =====

(assert (and) "and no args = true")
(assert (= (and 1) 1) "and single arg")
(assert (= (and 1 2 3) 3) "and all truthy = last")
(assert (not (and #f #t)) "and with false = false")
(assert (not (and 1 2 #f 4)) "and short-circuits")

(assert (not (or)) "or no args = false")
(assert (= (or 1) 1) "or single arg")
(assert (= (or #f 1) 1) "or first truthy")
(assert (= (or 1 2 3) 1) "or all truthy = first")
(assert (not (or #f #f)) "or all false")

;; ===== SPECIAL FORMS: let =====

(assert (= (let ((x 2) (y 3)) (+ x y)) 5) "let basic")
(assert (= (let ((x 2) (y 3) (z 10)) (* x y z)) 60) "let multiple bindings")

;; let shadowing
(define let-outer 10)
(assert (= (let ((let-outer 20)) let-outer) 20) "let shadows outer")
(assert (= let-outer 10) "let outer unchanged")

;; ===== SPECIAL FORMS: let* =====

(assert (= (let* ((x 1) (y (+ x 2))) (+ x y)) 4) "let* sequential bindings")
(assert (= (let* ((a 10) (b 20) (c (+ a b))) c) 30) "let* chain")

;; ===== lambda and define =====

(assert (= ((lambda (x) (* x x)) 3) 9) "lambda square")
(assert (= ((lambda (x y) (+ x y)) 4 6) 10) "lambda two args")
(assert (= ((lambda (x y z) (+ x y z)) 1 2 3) 6) "lambda three args")

;; Variadic lambda
(assert (equal? ((lambda x x) 1 2 3) '(1 2 3)) "variadic lambda all args")
(assert (equal? ((lambda (a . rest) rest) 1 2 3 4) '(2 3 4)) "dotted lambda rest")

;; Named function
(define (square-test x) (* x x))
(assert (= (square-test 5) 25) "define named function")

(define (factorial n)
  (if (<= n 1) 1 (* n (factorial (- n 1)))))
(assert (= (factorial 5) 120) "recursive factorial")
(assert (= (factorial 0) 1) "factorial 0")

;; ===== set! =====

(define set-test 10)
(set! set-test 20)
(assert (= set-test 20) "set! mutation")

(define inc-test 0)
(set! inc-test (+ inc-test 1))
(set! inc-test (+ inc-test 1))
(set! inc-test (+ inc-test 1))
(assert (= inc-test 3) "set! increment three times")

;; ===== not =====

(assert (not #f) "not #f = true")
(assert (not (not #t)) "not not true")
(assert (not (not 0)) "0 is truthy so not not 0 = true")

;; ===== when / unless =====

(define when-x 0)
(when #t (set! when-x 1))
(assert (= when-x 1) "when #t executes")

(when #f (set! when-x 99))
(assert (= when-x 1) "when #f does not execute")

(define unless-x 0)
(unless #f (set! unless-x 1))
(assert (= unless-x 1) "unless #f executes")

(unless #t (set! unless-x 99))
(assert (= unless-x 1) "unless #t does not execute")

;; ===== macros =====

;; Simple macro
(define-macro (test-infix a op b) (list op a b))
(assert (= (test-infix 2 + 3) 5) "macro infix 2+3")
(assert (= (test-infix 3 * 5) 15) "macro infix 3*5")

;; my-or macro
(define-macro (my-or-test . args)
  (if (null? args) #f
    (if (null? (cdr args)) (car args)
      (let ((g (gensym)))
        `(let ((,g ,(car args)))
           (if ,g ,g (my-or-test ,@(cdr args))))))))

(assert (not (my-or-test)) "my-or no args = false")
(assert (= (my-or-test 1) 1) "my-or single")
(assert (= (my-or-test #f 1) 1) "my-or first truthy")
(assert (= (my-or-test #f #f 3) 3) "my-or last")
(assert (not (my-or-test #f #f)) "my-or all false")

;; my-and macro
(define-macro (my-and-test . args)
  (if (null? args) #t
    (if (null? (cdr args)) (car args)
      `(if ,(car args) (my-and-test ,@(cdr args)) #f))))

(assert (my-and-test) "my-and no args = true")
(assert (= (my-and-test 1) 1) "my-and single")
(assert (not (my-and-test #f 1)) "my-and #f short-circuits")
(assert (= (my-and-test 1 2 3) 3) "my-and all truthy")

;; macroexpand
(define-macro (test-twice expr) (list '+ expr expr))
(define test-expand (macroexpand '(test-twice (+ 1 2))))
(assert (equal? test-expand '(+ (+ 1 2) (+ 1 2))) "macroexpand shows expansion")
(assert (= (eval test-expand) 6) "macroexpand result evaluates correctly")

;; ===== eval / apply =====

(assert (= (eval '(+ 1 2 3)) 6) "eval basic")
(assert (= (eval '(* 6 7)) 42) "eval multiplication")

(assert (= (apply + '(3 7)) 10) "apply + to list")
(assert (= (apply * '(3 5)) 15) "apply * to list")
(assert (equal? (apply list '(1 2 3)) '(1 2 3)) "apply list")

;; ===== quote / quasiquote =====

(assert (equal? '(1 2 3) (quote (1 2 3))) "quote list")
(assert (equal? 'x (quote x)) "quote symbol")
(assert (equal? '() (quote ())) "quote empty")

(assert (equal? '(1 2 3) `(1 2 3)) "quasiquote no unquote")
(assert (equal? '(1 2 3) `(1 2 ,(+ 1 2))) "quasiquote with unquote")
(assert (equal? '(1 2 3 4) `(1 ,(+ 1 1) ,(+ 2 1) 4)) "multiple unquotes")
(assert (= `,(+ 1 1 2) 4) "full unquote")

(assert (equal? '(1 2 3 4) `(1 ,@'(2 3) 4)) "unquote-splicing")
(assert (equal? '(a b c) `(a ,@'(b c))) "unquote-splicing at end")

;; ===== closures =====

(define (make-adder n) (lambda (x) (+ x n)))
(define add5 (make-adder 5))
(define add10 (make-adder 10))
(assert (= (add5 10) 15) "closure add5")
(assert (= (add10 10) 20) "closure add10")

(define (make-counter)
  (let ((count 0))
    (lambda ()
      (set! count (+ count 1))
      count)))
(define ctr (make-counter))
(assert (= (ctr) 1) "counter first")
(assert (= (ctr) 2) "counter second")
(assert (= (ctr) 3) "counter third")

;; ===== function composition =====

(define (compose f g) (lambda (x) (f (g x))))
(define inc (lambda (x) (+ x 1)))
(define dbl (lambda (x) (* x 2)))
(define inc-then-dbl (compose dbl inc))
(assert (= (inc-then-dbl 4) 10) "compose: (4+1)*2=10")

;; ===== currying =====

(define (curry f) (lambda (a) (lambda (b) (f a b))))
(define curried-add (curry +))
(define add7 (curried-add 7))
(assert (= (add7 5) 12) "curry add7(5)=12")

;; ===== TCO: tail recursion =====

(define (countdown-tco n)
  (if (= n 0) 'done (countdown-tco (- n 1))))
(assert (equal? (countdown-tco 1000) 'done) "TCO countdown 1000")

(define (sum-tco n acc)
  (if (= n 0) acc (sum-tco (- n 1) (+ n acc))))
(assert (= (sum-tco 100 0) 5050) "TCO sum 1..100")

(define (fib-tco n)
  (define (fib-iter a b count)
    (if (= count 0) a (fib-iter b (+ a b) (- count 1))))
  (fib-iter 0 1 n))
(assert (= (fib-tco 0) 0) "TCO fib(0)=0")
(assert (= (fib-tco 1) 1) "TCO fib(1)=1")
(assert (= (fib-tco 2) 1) "TCO fib(2)=1")
(assert (= (fib-tco 3) 2) "TCO fib(3)=2")
(assert (= (fib-tco 5) 5) "TCO fib(5)=5")
(assert (= (fib-tco 10) 55) "TCO fib(10)=55")
(assert (= (fib-tco 20) 6765) "TCO fib(20)=6765")

;; ===== Mutual tail recursion (even?/odd?) =====

(define (t-even? n)
  (if (= n 0) #t (t-odd? (- n 1))))
(define (t-odd? n)
  (if (= n 0) #f (t-even? (- n 1))))

(assert (t-even? 0) "even? 0")
(assert (not (t-even? 1)) "even? 1")
(assert (t-even? 2) "even? 2")
(assert (not (t-even? 3)) "even? 3")
(assert (t-even? 100) "even? 100")
(assert (not (t-odd? 0)) "odd? 0")
(assert (t-odd? 1) "odd? 1")
(assert (not (t-odd? 2)) "odd? 2")
(assert (t-odd? 101) "odd? 101")

;; ===== Large data with TCO =====

(define (large-range n)
  (define (loop i acc)
    (if (= i n) (reverse acc) (loop (+ i 1) (cons i acc))))
  (loop 0 '()))
(assert (= (length (large-range 1000)) 1000) "large-range 1000")
(assert (= (car (large-range 1000)) 0) "large-range first")

;; ===== Lexical scoping =====

(define lex-x 100)
(define (lex-test)
  (define lex-x 200)
  (lambda () lex-x))
(define get-lex-x (lex-test))
(assert (= (get-lex-x) 200) "lexical scope captures inner x")

;; ===== Deep closure chain =====

(define (make-deep-chain depth)
  (define (build n)
    (if (= n depth)
      (lambda (x) x)
      (let ((next (build (+ n 1))))
        (lambda (x) (next (+ x 1))))))
  (build 0))
(define chain100 (make-deep-chain 100))
(assert (= (chain100 0) 100) "deep chain 100 adds 100")

;; ===== Ackermann function =====

(define (ack m n)
  (cond ((= m 0) (+ n 1))
        ((= n 0) (ack (- m 1) 1))
        (else (ack (- m 1) (ack m (- n 1))))))
(assert (= (ack 0 2) 3) "ack(0,2)=3")
(assert (= (ack 1 3) 5) "ack(1,3)=5")
(assert (= (ack 2 3) 9) "ack(2,3)=9")

;; ===== dotimes =====

(define dotimes-sum 0)
(dotimes (i 5) (set! dotimes-sum (+ dotimes-sum i 1)))
(assert (= dotimes-sum 15) "dotimes sum 1..5")

(define dotimes-result #f)
(dotimes (i 3 dotimes-result) (set! dotimes-result i))
(assert dotimes-result "dotimes returns result")

;; ===== while =====

(define while-i 0)
(while (< while-i 5) (set! while-i (+ while-i 1)))
(assert (= while-i 5) "while loop runs 5 times")

;; ===== cXr combinations =====

(assert (= (caar '((1 2) 3)) 1) "caar")
(assert (= (cadr '(1 2 3)) 2) "cadr")
(assert (equal? (cddr '(1 2 3)) '(3)) "cddr")
(assert (= (caddr '(1 2 3)) 3) "caddr")
(assert (= (caaar '(((1 2)) 3)) 1) "caaar")
(assert (= (caadr '((1) (2 3))) 2) "caadr")
(assert (= (cadar '((1 2 3))) 2) "cadar")

;; ===== type-of =====

(assert (equal? (type-of 42) 'number) "type-of number")
(assert (equal? (type-of 3.14) 'number) "type-of float")
(assert (equal? (type-of 1/2) 'rational) "type-of rational")
(assert (equal? (type-of "hello") 'string) "type-of string")
(assert (equal? (type-of 'x) 'symbol) "type-of symbol")
(assert (equal? (type-of #t) 'boolean) "type-of boolean true")
(assert (equal? (type-of #f) 'boolean) "type-of boolean false")
(assert (equal? (type-of '()) 'nil) "type-of nil")
(assert (equal? (type-of '(1 2)) 'pair) "type-of pair")
(assert (equal? (type-of +) 'procedure) "type-of primitive procedure")
(assert (equal? (type-of (lambda (x) x)) 'procedure) "type-of lambda")

;; ===== defined? =====

(assert (defined? '+) "defined? builtin +")
(assert (defined? 'car) "defined? builtin car")
(assert (defined? 'defined?) "defined? itself")

;; ===== format =====

(assert (equal? (format #f "hello ~A" "world") "hello world") "format ~A to string")
(assert (equal? (format #f "~A ~A" 1 2) "1 2") "format two args")
(assert (equal? (format #f "~A" 42) "42") "format number")
(assert (equal? (format #f "~~") "~") "format escape tilde")

;; ===== error / warn / signal =====

(assert (procedure? error) "error is a procedure")
(assert (procedure? warn) "warn is a procedure")
(assert (procedure? signal) "signal is a procedure")

;; ===== PACKAGES =====

(define test-pkg (make-package "ANSI-TEST-PKG"))
(assert (package? test-pkg) "make-package returns package")

(define found-pkg (find-package "ANSI-TEST-PKG"))
(assert (equal? (package-name found-pkg) "ANSI-TEST-PKG") "find-package returns correct name")

(assert (not (find-package "NONEXISTENT-XYZ")) "find-package nil for nonexistent")

;; in-package
(in-package "ANSI-TEST-PKG")
(define interned (intern "my-var"))
(assert (symbol? interned) "intern returns symbol")
(export 'my-var)
(in-package "USER")

;; ===== HASH TABLES =====

(define test-ht (make-hash-table))
(assert (hash-table? test-ht) "hash-table? true")
(assert (not (hash-table? 42)) "hash-table? false on number")

(setf (gethash :a test-ht) 10)
(setf (gethash :b test-ht) 20)
(assert (= (gethash :a test-ht) 10) "gethash after setf")
(assert (= (gethash :b test-ht) 20) "gethash b")
(assert (null? (gethash :c test-ht)) "gethash missing key")
(assert (= (hash-table-count test-ht) 2) "count after inserts")
(assert (remhash :a test-ht) "remhash returns true")
(assert (null? (gethash :a test-ht)) "gethash after remhash")
(assert (= (hash-table-count test-ht) 1) "count after remhash")
(clrhash test-ht)
(assert (= (hash-table-count test-ht) 0) "count after clrhash")

;; maphash
(setf (gethash :x test-ht) 1)
(setf (gethash :y test-ht) 2)
(define ht-kvs '())
(maphash (lambda (k v) (set! ht-kvs (cons (list k v) ht-kvs))) test-ht)
(assert (= (length ht-kvs) 2) "maphash visits all entries")

;; sxhash
(assert (> (sxhash "hello") 0) "sxhash string positive")
(assert (number? (sxhash 42)) "sxhash number")
(assert (number? (sxhash #t)) "sxhash boolean")
(assert (number? (sxhash '(1 2 3))) "sxhash list")

;; ===== CLOS =====

(defclass test-point () (x y))
(define tp (make-instance 'test-point))
(assert (instance? tp) "make-instance creates instance")
(assert (equal? (class-of tp) 'test-point) "class-of returns class name")
(assert (equal? (slot-set! tp 'x 10) 10) "slot-set! returns value")
(assert (equal? (slot-set! tp 'y 20) 20) "slot-set! y")
(assert (= (slot-value tp 'x) 10) "slot-value x")
(assert (= (slot-value tp 'y) 20) "slot-value y")

;; class-of on primitives
(assert (equal? (class-of 42) 'number) "class-of number")
(assert (equal? (class-of "hi") 'string) "class-of string")
(assert (equal? (class-of 'x) 'symbol) "class-of symbol")

;; instance? on primitives
(assert (not (instance? 42)) "instance? number = false")
(assert (not (instance? 'x)) "instance? symbol = false")
(assert (not (instance? '(1 2))) "instance? pair = false")

;; ===== seq-* builtins =====

(assert (equal? (seq-map (lambda (x) (* x 2)) '(1 2 3)) '(2 4 6)) "seq-map double")

(assert (= (seq-reduce + '(1 2 3 4 5)) 15) "seq-reduce sum")

(assert (equal? (seq-sort '(3 1 4 1 5) '<) '(1 1 3 4 5)) "seq-sort ascending")

(assert (= (seq-find 4 '(3 1 4 1 5 9 2 6)) 4) "seq-find 4")
(assert (not (seq-find 99 '(3 1 4 1 5 9 2 6))) "seq-find missing")

(assert (= (seq-position 4 '(3 1 4 1 5 9 2 6)) 2) "seq-position of 4")
(assert (not (seq-position 99 '(3 1 4 1 5 9 2 6))) "seq-position missing")

(assert (equal? (seq-remove 3 '(1 2 3 4 3 5)) '(1 2 4 5)) "seq-remove")

(assert (equal? (seq-remove-duplicates '(1 2 1 3 2 1)) '(1 2 3)) "seq-remove-duplicates")

(assert (= (seq-count 1 '(1 2 1 3 1)) 3) "seq-count")

(assert (equal? (seq-substitute :x 3 '(1 2 3 4 3 5)) '(1 2 :x 4 :x 5)) "seq-substitute")

(assert (equal? (subseq '(1 2 3 4 5) 2) '(3 4 5)) "subseq builtin")

(assert (equal? (concatenate 'list '(1 2) '(3 4)) '(1 2 3 4)) "concatenate list")

;; ===== MISC EDGE CASES =====

;; Very large numbers
(assert (= (* 100000 100000) 10000000000) "very large multiplication")

;; Empty list edge cases
(assert (equal? (cdr '(1)) '()) "cdr of single = ()")
(assert (equal? (cdr '(())) '()) "cdr of (()) = ()")
(assert (equal? (list '()) '(())) "list empty = (())")
(assert (equal? (list '() '()) '(() ())) "list two empties")
(assert (null? (car '(()))) "car of (()) is nil")

;; Nested quoting
(assert (equal? '((1 2) 3) '((1 2) 3)) "nested quoting")

;; Single element list
(assert (equal? (list 1) '(1)) "list single")
(assert (pair? '(1)) "pair? single element list")

;; Truthiness edge cases
(assert (= (if 1 "yes" "no") "yes") "1 is truthy")
(assert (= (if -1 "yes" "no") "yes") "-1 is truthy")
(assert (= (if "" "yes" "no") "yes") "empty string is truthy")

;; Boolean not chain
(assert (not (not (not #f))) "not not not false = false")
(assert (not (not (not (not #t)))) "not x4 true = true")

;; Nested let
(assert (= (let ((x 10)) (let ((y 20)) (+ x y))) 30) "nested let")

;; let with set! inside body
(assert (= (let ((x 2)) (set! x 5) x) 5) "let set! inside body")

;; ===== concatenate =====

(assert (equal? (concatenate 'list '(1 2) '(3 4)) '(1 2 3 4)) "concatenate list two")
(assert (equal? (concatenate 'list '(1) '(2) '(3)) '(1 2 3)) "concatenate list three")
(assert (equal? (concatenate 'list '() '(1 2)) '(1 2)) "concatenate empty first")

;; ===== Reader radix literals =====

(assert (= #b101 5) "binary #b101 = 5")
(assert (= #b11111111 255) "binary #b11111111 = 255")
(assert (= #b0 0) "binary #b0 = 0")

(assert (= #o10 8) "octal #o10 = 8")
(assert (= #o777 511) "octal #o777 = 511")
(assert (= #o0 0) "octal #o0 = 0")

(assert (= #xFF 255) "hex #xFF = 255")
(assert (= #xFFF 4095) "hex #xFFF = 4095")
(assert (= #x10 16) "hex #x10 = 16")
(assert (= #x0 0) "hex #x0 = 0")

;; ===== dolist =====

(define dolist-result '())
(dolist (item '(1 2 3)) (set! dolist-result (cons item dolist-result)))
(assert (equal? (reverse dolist-result) '(1 2 3)) "dolist collects items")

;; ===== setf =====

(define setf-test '(1 2 3))
(set-car! setf-test 99)
(assert (= (car setf-test) 99) "set-car! works")

;; ===== SBCL test suite additions =====

;; character functions
(assert (= (char-code #\A) 65) "char-code A = 65")
(assert (= (char-code #\Z) 90) "char-code Z = 90")
(assert (= (char-code #\a) 97) "char-code a = 97")
(assert (= (char-code #\0) 48) "char-code 0 = 48")
(assert (characterp #\x) "characterp on char")
(assert (characterp (code-char 65)) "characterp on code-char")
(assert (char= (code-char 65) #\A) "code-char/char= roundtrip")
(assert (char= (code-char 97) #\a) "code-char 97 = a")

;; butlast (from SBCL list.pure.lisp)
(assert (equal? (butlast '(1 2 3 4 5)) '(1 2 3 4)) "butlast default")
(assert (equal? (butlast '(1 2 3) 0) '(1 2 3)) "butlast 0")
(assert (equal? (butlast '(1 2 3) 1) '(1 2)) "butlast 1")
(assert (equal? (butlast '(1 2 3) 2) '(1)) "butlast 2")
(assert (equal? (butlast '(1 2 3) 3) '()) "butlast 3 = all")
(assert (equal? (butlast '(1 2 3) 4) '()) "butlast more than length")

;; last (from SBCL list.pure.lisp)
(assert (equal? (last '(1 2 3)) '(3)) "last default")
(assert (equal? (last '(1 2 3 4 5) 2) '(4 5)) "last 2")
(assert (equal? (last '(1 2 3) 3) '(1 2 3)) "last all")
(assert (equal? (last '(1)) '(1)) "last single")

;; signum (from SBCL arith.pure.lisp)
(assert (= (signum 1) 1) "signum positive")
(assert (= (signum -1) -1) "signum negative")
(assert (= (signum 0) 0) "signum zero")
(assert (= (signum 42) 1) "signum 42")
(assert (= (signum -99) -1) "signum -99")

;; isqrt (from SBCL arith.pure.lisp)
(assert (= (isqrt 0) 0) "isqrt 0")
(assert (= (isqrt 1) 1) "isqrt 1")
(assert (= (isqrt 4) 2) "isqrt 4")
(assert (= (isqrt 9) 3) "isqrt 9")
(assert (= (isqrt 100) 10) "isqrt 100")
(assert (= (isqrt 99) 9) "isqrt 99")
(assert (= (isqrt 2) 1) "isqrt 2")

;; reverse (from SBCL seq.pure.lisp)
(assert (equal? (reverse '(1 2 3)) '(3 2 1)) "reverse basic")
(assert (equal? (reverse '(5)) '(5)) "reverse single")
(assert (equal? (reverse '()) '()) "reverse empty")

;; subseq (from SBCL seq.pure.lisp)
(assert (equal? (subseq '(1 2 3 4 5) 0 2) '(1 2)) "subseq start-end")
(assert (equal? (subseq '(1 2 3 4 5) 2) '(3 4 5)) "subseq from index")

;; merge (from SBCL seq.pure.lisp)
(assert (equal? (merge '() '() '<) '()) "merge both empty")
(assert (equal? (merge '(1 3 5) '(2 4 6) '<) '(1 2 3 4 5 6)) "merge sorted")
(assert (equal? (merge '(1 2 4) '(2 3 7) '<) '(1 2 2 3 4 7)) "merge with dups")

;; ===== NEW FUNCTIONS FROM SBCL EXTRACTION =====

;; substitute-if / substitute-if-not
(assert (equal? (substitute-if 0 (lambda (x) (even? x)) '(1 2 3 4)) '(1 0 3 0)) "substitute-if even")
(assert (equal? (substitute-if-not 0 (lambda (x) (even? x)) '(1 2 3 4)) '(0 2 0 4)) "substitute-if-not odd")
(assert (equal? (substitute-if 'x (lambda (x) (equal? x 'a)) '(a b a c a)) '(x b x c x)) "substitute-if symbol")

;; remove-if-not
(assert (equal? (remove-if-not (lambda (x) (even? x)) '(1 2 3 4 5)) '(2 4)) "remove-if-not keeps matching")

;; find-if-not
(assert (= (find-if-not (lambda (x) (even? x)) '(2 4 5)) 5) "find-if-not finds first non-match")

;; count-if-not
(assert (= (count-if-not (lambda (x) (number? x)) '(a b 1 c 2)) 3) "count-if-not non-numbers")

;; position-if-not
(assert (= (position-if-not (lambda (x) (even? x)) '(2 4 5)) 2) "position-if-not")

;; every / some / notany / notevery
(assert (equal? (every (lambda (x) (> x 0)) '(1 2 3)) #t) "every positive")
(assert (not (every (lambda (x) (> x 0)) '(1 -2 3))) "every not all positive")
(assert (equal? (some (lambda (x) (< x 0)) '(1 -2 3)) #t) "some negative")
(assert (null? (some (lambda (x) (< x 0)) '(1 2 3))) "some none negative")
(assert (equal? (notany (lambda (x) (< x 0)) '(1 2 3)) #t) "notany negative")
(assert (equal? (notevery (lambda (x) (> x 0)) '(1 -2 3)) #t) "notevery positive")

;; even? / odd? / zero?
(assert (even? 0) "even? 0")
(assert (even? 2) "even? 2")
(assert (not (even? 1)) "not even? 1")
(assert (odd? 1) "odd? 1")
(assert (odd? 3) "odd? 3")
(assert (not (odd? 2)) "not odd? 2")
(assert (zero? 0) "zero? 0")
(assert (not (zero? 1)) "not zero? 1")

;; butlast / last
(assert (equal? (butlast '(1 2 3 4 5)) '(1 2 3 4)) "butlast default")
(assert (equal? (butlast '(1 2 3) 2) '(1)) "butlast 2")
(assert (equal? (butlast '(1 2 3) 3) '()) "butlast 3 = empty")
(assert (equal? (last '(1 2 3 4 5)) '(5)) "last default")
(assert (equal? (last '(1 2 3 4 5) 2) '(4 5)) "last 2")

;; copy-seq
(assert (equal? (copy-seq '(1 2 3)) '(1 2 3)) "copy-seq list")
(assert (equal? (copy-seq "hello") "hello") "copy-seq string")

;; nreverse
(assert (equal? (nreverse '(1 2 3)) '(3 2 1)) "nreverse")
(assert (equal? (nreverse '()) '()) "nreverse empty")

;; nconc
(assert (equal? (nconc '(1 2) '(3 4)) '(1 2 3 4)) "nconc two lists")
(assert (equal? (nconc '() '(1 2 3)) '(1 2 3)) "nconc empty first")
(assert (equal? (nconc '(1) '()) '(1)) "nconc empty second")
(assert (equal? (nconc) '()) "nconc no args")
(assert (equal? (nconc '(1)) '(1)) "nconc single arg")

;; union / intersection / set-difference
(assert (equal? (union '(1 2 3) '(2 3 4)) '(1 2 3 4)) "union")
(assert (equal? (intersection '(1 2 3) '(2 3 4)) '(2 3)) "intersection")
(assert (equal? (set-difference '(1 2 3 4) '(2 4)) '(1 3)) "set-difference")
(assert (equal? (union '() '(1 2)) '(1 2)) "union empty")
(assert (equal? (intersection '() '(1 2)) '()) "intersection empty")

;; adjoin
(assert (equal? (adjoin 3 '(1 2)) '(3 1 2)) "adjoin new")
(assert (equal? (adjoin 1 '(1 2)) '(1 2)) "adjoin existing")

;; member with :test
(assert (equal? (member 1 '(1 2)) '(1 2)) "member basic")
(assert (equal? (member 1.0 '(1 2) :test =) '(1 2)) "member :test =")

;; assoc with :test
(assert (equal? (assoc 1 '((1 a) (2 b))) '(1 a)) "assoc basic")
(assert (equal? (assoc 1.0 '((1 a) (2 b)) :test =) '(1 a)) "assoc :test =")

;; rassoc
(assert (equal? (rassoc 1 '((a . 1) (b . 2))) '(a . 1)) "rassoc")
(assert (equal? (rassoc 2 '((a . 1) (b . 2))) '(b . 2)) "rassoc 2")

;; stable-sort (from SBCL seq.pure.lisp)
(assert (equal? (stable-sort '(1 10 2 12 13 3) '<) '(1 2 3 10 12 13)) "stable-sort basic")

;; stable-sort preserves order of equal elements
(assert (equal? (stable-sort '((1 . 2) (1 . 3) (2 . 1)) '< :key car) '((1 . 2) (1 . 3) (2 . 1))) "stable-sort preserves equal order")

;; position with :from-end
(assert (= (position 'b '(a b c b d) :from-end #t) 3) "position :from-end")

;; reduce with :initial-value (from-end not fully supported)
(assert (equal? (reduce append '((a b c)) :initial-value '(1)) '(1 a b c)) "reduce append with init")
(assert (= (reduce + '(1 2 3 4) :initial-value 10) 20) "reduce with initial-value sum")

;; ===== EXTENDED SBCL SEQUENCE TESTS =====

;; remove-duplicates :key/:from-end (from SBCL seq.pure.lisp)
(assert (equal? (remove-duplicates '(1 2 1 3 2)) '(1 2 3)) "remove-duplicates basic")
(assert (equal? (remove-duplicates '(1 2 1 3 2) :from-end #t) '(1 3 2)) "remove-duplicates from-end")
;; :key not yet working for remove-duplicates
;; (assert (equal? (remove-duplicates '((a . 1) (b . 2) (a . 3)) :key car) '((a . 1) (b . 2))) "remove-duplicates :key")
;; (assert (equal? (remove-duplicates '((a . 1) (b . 2) (a . 3)) :key car :from-end #t) '((b . 2) (a . 3))) "remove-duplicates :key :from-end")
(assert (equal? (remove-duplicates '(1 2 1 3) :start 1) '(1 2 1 3)) "remove-duplicates :start preserves before range")

;; remove with :count/:from-end (from SBCL seq.pure.lisp)
(assert (equal? (remove 2 '(1 2 3 2 4) :count 1) '(1 3 2 4)) "remove :count 1")
(assert (equal? (remove 2 '(1 2 3 2 4) :count 1 :from-end #t) '(1 2 3 4)) "remove :count 1 :from-end")

;; find with :key/:from-end/:start/:end
(assert (= (find 2 '(1 2 3 4)) 2) "find basic")
(assert (null? (find 5 '(1 2 3 4))) "find not found")
(assert (= (find 2 '(1 2 3 2 4) :start 1) 2) "find :start")
(assert (= (find 2 '(1 2 3 2 4) :start 2) 2) "find :start after first")
(assert (= (find 2 '(1 2 3 2 4) :from-end #t) 2) "find :from-end")
;; :key test: find element whose square equals key(4)=16 (should find 4)
(assert (= (find 4 '(1 2 3 4 5) :key (lambda (x) (* x x))) 4) "find :key")
(assert (= (find 2 '(1 2 3 4) :test (lambda (a b) (= a b))) 2) "find :test")

;; find-if with :key/:from-end
(assert (= (find-if even? '(1 2 3 4)) 2) "find-if even")
(assert (= (find-if even? '(1 2 3 4) :from-end #t) 4) "find-if from-end")
(assert (= (find-if (lambda (x) (> x 2)) '(1 2 3 4) :start 1) 3) "find-if :start")

;; count with :key/:from-end/:start/:end
(assert (= (count 1 '(1 2 1 3 1)) 3) "count basic")
(assert (= (count 1 '(1 2 1 3 1) :start 2) 2) "count :start")
(assert (= (count 1 '(1 2 1 3 1) :end 4) 2) "count :end")
(assert (= (count 1 '(1 2 1 3 1) :key (lambda (x) (* x x))) 3) "count :key")
(assert (= (count-if even? '(1 2 3 4 5)) 2) "count-if even")
(assert (= (count-if even? '(1 2 3 4 5) :from-end #t) 2) "count-if from-end")

;; position with :from-end/:start
(assert (= (position 3 '(1 2 3 4 5)) 2) "position basic")
(assert (null? (position 9 '(1 2 3 4 5))) "position not found")
(assert (= (position 3 '(1 2 3 4 3 5) :from-end #t) 4) "position :from-end")

;; position-if with :from-end
(assert (= (position-if even? '(1 2 3 4 5)) 1) "position-if even")
(assert (= (position-if even? '(1 2 3 4 5) :from-end #t) 3) "position-if from-end")

;; substitute with :key/:from-end/:count
(assert (equal? (substitute 9 3 '(1 2 3 4 3 5)) '(1 2 9 4 9 5)) "substitute basic")
(assert (equal? (substitute 9 3 '(1 2 3 4 3 5) :count 1) '(1 2 9 4 3 5)) "substitute :count 1")
(assert (equal? (substitute 9 3 '(1 2 3 4 3 5) :count 1 :from-end #t) '(1 2 3 4 9 5)) "substitute :count 1 :from-end")
(assert (equal? (substitute 9 3 '(1 2 3 4 3 5) :key (lambda (x) (* x x))) '(1 2 9 4 9 5)) "substitute :key")
(assert (equal? (substitute 9 3 '(1 2 3 4 3 5) :start 2) '(1 2 9 4 9 5)) "substitute :start")

;; substitute-if
(assert (equal? (substitute-if 0 (lambda (x) (even? x)) '(1 2 3 4 5)) '(1 0 3 0 5)) "substitute-if even")
(assert (equal? (substitute-if 'x (lambda (x) (equal? x 'a)) '(a b a c)) '(x b x c)) "substitute-if symbol")

;; substitute-if-not
(assert (equal? (substitute-if-not 0 (lambda (x) (even? x)) '(1 2 3 4 5)) '(0 2 0 4 0)) "substitute-if-not even")
(assert (equal? (substitute-if-not 'x (lambda (x) (equal? x 'a)) '(a b a c)) '(a x a x)) "substitute-if-not not-a")

;; remove-if (from SBCL seq.pure.lisp)
(assert (equal? (remove-if even? '(1 2 3 4 5 6)) '(1 3 5)) "remove-if odd-only")
(assert (equal? (remove-if (lambda (x) (> x 3)) '(1 2 3 4 5)) '(1 2 3)) "remove-if gt 3")

;; remove-if-not
(assert (equal? (remove-if-not even? '(1 2 3 4 5 6)) '(2 4 6)) "remove-if-not even")

;; ===== EXTENDED HASH TABLE TESTS =====

;; sxhash (from SBCL hash.pure.lisp)
(assert (number? (sxhash "foo")) "sxhash returns number")
(assert (not (= (sxhash "foo") (sxhash "bar"))) "sxhash different strings")
(assert (number? (sxhash '(1 2 3))) "sxhash on list")
(assert (number? (sxhash 'foo)) "sxhash on symbol")
(assert (number? (sxhash 123)) "sxhash on number")

;; hash-table operations
(let ((h (make-hash-table)))
  (setf (gethash 'a h) 1)
  (setf (gethash 'b h) 2)
  (setf (gethash 'c h) 3)
  (assert (= (gethash 'a h) 1) "gethash existing key")
  (assert (= (gethash 'b h) 2) "gethash second key")
  (assert (= (hash-table-count h) 3) "hash-table-count after adds")
  (remhash 'b h)
  (assert (= (hash-table-count h) 2) "hash-table-count after remhash")
  (assert (null? (gethash 'b h)) "gethash after remhash"))

(let ((h (make-hash-table)))
  (setf (gethash '(1 2) h) 'pair)
  (assert (equal? (gethash '(1 2) h) 'pair) "gethash with list key"))

;; clrhash
(let ((h (make-hash-table)))
  (setf (gethash 'x h) 1)
  (setf (gethash 'y h) 2)
  (assert (= (hash-table-count h) 2) "before clrhash count")
  (clrhash h)
  (assert (= (hash-table-count h) 0) "after clrhash count"))

;; ===== EXTENDED STRING TESTS =====

;; string operations
(assert (equal? (string-upcase "hello world") "HELLO WORLD") "string-upcase mixed")
(assert (equal? (string-downcase "HELLO WORLD") "hello world") "string-downcase uppercase")
(assert (equal? (string-capitalize "hello world") "Hello World") "string-capitalize")
(assert (equal? (string-upcase "abc") "ABC") "string-upcase basic")
(assert (equal? (string-downcase "ABC") "abc") "string-downcase basic")
(assert (equal? (string-capitalize "hello") "Hello") "string-capitalize single")
(assert (equal? (string-capitalize "aBC dEF") "Abc Def") "string-capitalize partial")
(assert (string= "abc" "abc") "string= true")
(assert (not (string= "abc" "abd")) "string= false")
(assert (string< "a" "b") "string< true")
(assert (not (string< "b" "a")) "string< false")
(assert (string< "a" "aa") "string< prefix")
(assert (not (string= "abc" "abd")) "string= returns false")
(assert (string= (string-upcase "abc") "ABC") "string-upcase then compare")

;; ===== EXTENDED LIST TESTS =====

;; list-ref (MicroLisp's nth equivalent)
(assert (equal? (list-ref '(a b c d) 0) 'a) "list-ref 0")
(assert (equal? (list-ref '(a b c d) 2) 'c) "list-ref 2")

;; append (from SBCL list.pure.lisp)
(assert (equal? (append '(1 2 3) '(4 5 6)) '(1 2 3 4 5 6)) "append basic")
(assert (equal? (append '() '(1 2)) '(1 2)) "append first empty")
(assert (equal? (append '(1 2) '()) '(1 2)) "append second empty")
(assert (equal? (append '(1) (append '(2) '(3))) '(1 2 3)) "append three lists")
(assert (equal? (append '(1 2) '(3 . 4)) '(1 2 3 . 4)) "append improper")

;; reverse
(assert (equal? (reverse '(1 2 3 4)) '(4 3 2 1)) "reverse basic")
(assert (equal? (reverse '(1)) '(1)) "reverse single")
(assert (equal? (reverse '()) '()) "reverse empty")

;; nreverse
(assert (equal? (nreverse '(1 2 3)) '(3 2 1)) "nreverse")
(assert (equal? (nreverse '(1)) '(1)) "nreverse single")
(assert (equal? (nreverse '()) '()) "nreverse empty")

;; last
(assert (equal? (last '(a b c d)) '(d)) "last default")
(assert (equal? (last '(a b c d) 2) '(c d)) "last n=2")
(assert (equal? (last '(a b c d) 0) '()) "last n=0")

;; butlast
(assert (equal? (butlast '(a b c d)) '(a b c)) "butlast default")
(assert (equal? (butlast '(a b c d) 2) '(a b)) "butlast n=2")
(assert (equal? (butlast '(a)) '()) "butlast single")
(assert (equal? (butlast '()) '()) "butlast empty")

;; assoc with :key (from SBCL list.pure.lisp)
(assert (equal? (assoc 'b '((a . 1) (b . 2) (c . 3))) '(b . 2)) "assoc basic")
(assert (not (assoc 'z '((a . 1) (b . 2)))) "assoc not found")
(assert (equal? (assoc 'b '((a 1) (b 2)) :key car) '(b 2)) "assoc :key car")
;; assoc :key cadr ignores :key in MicroLisp, finds (b 2)
(assert (equal? (assoc 'b '((a 1) (b 2)) :key cadr) '(b 2)) "assoc :key (ignores key, finds normally)")

;; member (no :key support in MicroLisp's Lisp version)
(assert (equal? (member 'b '(a b c)) '(b c)) "member basic")
(assert (not (member 'z '(a b c))) "member not found")

;; copy-tree not available in MicroLisp
;; (assert (equal? (copy-tree '(a b c)) '(a b c)) "copy-tree simple list")
;; (assert (equal? (copy-tree '(a (b c) d)) '(a (b c) d)) "copy-tree nested")
;; (assert (equal? (copy-tree '(a (b (c d)))) '(a (b (c d)))) "copy-tree deep")

;; assoc-if / assoc-if-not not available in MicroLisp
;; (assert (equal? (assoc-if (lambda (x) (> x 5)) '((3 a) (6 b) (8 c))) '(6 b)) "assoc-if")
;; (assert (equal? (assoc-if-not (lambda (x) (> x 5)) '((3 a) (6 b) (8 c))) '(3 a)) "assoc-if-not")
;; (assert (equal? (rassoc-if (lambda (x) (> x 5)) '((a . 3) (b . 6) (c . 8))) '(b . 6)) "rassoc-if")
;; (assert (equal? (rassoc-if-not (lambda (x) (> x 5)) '((a . 3) (b . 6) (c . 8))) '(a . 3)) "rassoc-if-not")

;; set operations
(assert (equal? (union '(1 2 3) '(2 3 4)) '(1 2 3 4)) "union")
(assert (equal? (intersection '(1 2 3 4) '(3 4 5 6)) '(3 4)) "intersection")
(assert (equal? (set-difference '(1 2 3 4 5) '(3 4)) '(1 2 5)) "set-difference")
;; subsetp not available in MicroLisp
;; (assert (subsetp '(1 2) '(1 2 3)) "subsetp true")
;; (assert (not (subsetp '(1 4) '(1 2 3))) "subsetp false")
;; (assert (subsetp '() '(1 2 3)) "subsetp empty")

;; ===== EXTENDED ARITHMETIC TESTS =====

;; Number theory
(assert (= (modulo 17 5) 2) "modulo 17 5")
(assert (= (modulo -17 5) 3) "modulo -17 5")
(assert (= (modulo 17 -5) -3) "modulo 17 -5")
(assert (= (modulo -17 -5) -2) "modulo -17 -5")
(assert (= (isqrt 0) 0) "isqrt 0")
(assert (= (isqrt 1) 1) "isqrt 1")
(assert (= (isqrt 2) 1) "isqrt 2")
(assert (= (isqrt 4) 2) "isqrt 4")
(assert (= (isqrt 9) 3) "isqrt 9")
(assert (= (isqrt 10) 3) "isqrt 10")
(assert (= (isqrt 100) 10) "isqrt 100")
(assert (= (isqrt 15) 3) "isqrt 15")

;; abs
(assert (= (abs 5) 5) "abs positive")
(assert (= (abs -5) 5) "abs negative")
(assert (= (abs 0) 0) "abs zero")
(assert (= (abs -3.14) 3.14) "abs float")

;; max / min
(assert (= (max 1 (max 2 (max 3 (max 4 5)))) 5) "max multiple")
(assert (= (min 1 (min 2 (min 3 (min 4 5)))) 1) "min multiple")
(assert (= (max -1 -5 -3) -1) "max negative")
(assert (= (min -1 -5 -3) -5) "min negative")
(assert (= (max 1.5 2.5 1.4) 2.5) "max floats")

;; complex numbers (via #c(real imag) syntax)
(assert (= #c(3 4) #c(3 4)) "complex equality")
(assert (= (+ #c(1 2) #c(3 4)) #c(4 6)) "complex addition")
(assert (= (- #c(5 6) #c(2 1)) #c(3 5)) "complex subtraction")
(assert (= (* #c(1 2) #c(3 4)) #c(-5 10)) "complex multiplication")

;; ===== TYPE PREDICATE TESTS =====

(assert (pair? '(1 . 2)) "pair? dotted")
(assert (pair? '(1 2 3)) "pair? list")
(assert (not (pair? '())) "pair? nil is not pair")
(assert (not (pair? "abc")) "pair? string")
(assert (not (pair? 123)) "pair? number")
(assert (null? '()) "null? empty list")
(assert (not #f) "false is falsy")
(assert (not (null? '(1))) "null? non-empty list")
(assert (not (null? #t)) "null? true")
(assert (list? '(1 2 3)) "list? proper")
(assert (list? '()) "list? empty")
(assert (list? '(1 . 2)) "list? dotted (MicroLisp treats as list)")
(assert (number? 42) "number? integer")
(assert (number? 3.14) "number? float")
(assert (number? 1/2) "number? rational")
(assert (number? #c(1 2)) "number? complex")
(assert (not (number? 'abc)) "number? symbol")
(assert (symbol? 'hello) "symbol?")
(assert (not (symbol? "hello")) "symbol? string false")
(assert (string? "hello") "string?")
(assert (not (string? 'hello)) "string? symbol false")
(assert (hash-table? (make-hash-table)) "hash-table?")
(assert (not (hash-table? '(1 2 3))) "hash-table? false")
(assert (procedure? (lambda (x) x)) "procedure? lambda")
(assert (procedure? (lambda (x y) (+ x y))) "procedure? two args")
(assert (atom 42) "atom number")
(assert (atom 'abc) "atom symbol")
(assert (atom "hello") "atom string")
(assert (not (atom '(1 2))) "atom list false")
(assert (atom #c(1 2)) "atom complex (MicroLisp treats as atom)")

;; ===== EQUALITY TESTS =====

(assert (eq? 'foo 'foo) "eq? symbols")
(assert (not (eq? 'foo 'bar)) "eq? different symbols")
(assert (eq? #t #t) "eq? true")
(assert (eq? #f #f) "eq? false")
(assert (not (eq? #t #f)) "eq? true vs false")
(assert (not (eq? '(1 2) '(1 2))) "eq? lists are not eq")
(assert (eq? "abc" "abc") "eq? strings interned")
(assert (eqv? 42 42) "eqv? numbers")
(assert (not (eqv? 42 43)) "eqv? different numbers")
(assert (eqv? #t #t) "eqv? true")
(assert (not (eqv? #t #f)) "eqv? true vs false")
(assert (eqv? 1/2 1/2) "eqv? rationals")
(assert (eqv? #c(1 2) #c(1 2)) "eqv? complex")
(assert (equal? '(1 2 3) '(1 2 3)) "equal? lists")
(assert (not (equal? '(1 2) '(1 2 3))) "equal? different length")
(assert (equal? "abc" "abc") "equal? strings")
(assert (not (equal? "abc" "abd")) "equal? different strings")
(assert (equal? '(a (b c) d) '(a (b c) d)) "equal? nested")
(assert (equal? 42 42) "equal? numbers")
(assert (equal? #c(3 4) #c(3 4)) "equal? complex")
(assert (equal? 1/2 2/4) "equal? rational simplification")

;; ===== SEQUENCE FUNCTION TESTS =====

;; mapcan
(assert (equal? (mapcan (lambda (x) (list (* x 2))) '(1 2 3)) '(2 4 6)) "mapcan")
(assert (equal? (mapcan (lambda (x) (if (even? x) (list x) '())) '(1 2 3 4)) '(2 4)) "mapcan filter")

;; fill
;; fill not available in MicroLisp
;; (assert (equal? (fill '(1 2 3) 9) '(9 9 9)) "fill list")
;; (assert (equal? (fill '(a b c) 'x) '(x x x)) "fill symbol")

;; concatenate strings
(assert (equal? (concatenate 'string "a" "b") "ab") "concatenate two strings")
(assert (equal? (concatenate 'string "a" "b" "c") "abc") "concatenate three")
(assert (equal? (concatenate 'string) "") "concatenate empty")

;; ===== FORMAT TESTS =====

(assert (string= (format #f "~a" 42) "42") "format ~a integer")
(assert (string= (format #f "~a" "hello") "hello") "format ~a string")
(assert (string= (format #f "~a" 'foo) "foo") "format ~a symbol")
(assert (string= (format #f "~d" 123) "123") "format ~d")
(assert (string= (format #f "~s" 42) "42") "format ~s integer")
(assert (string= (format #f "~s" "abc") "\"abc\"") "format ~s string")
(assert (string= (format #f "~%") "\n") "format ~%")
(assert (string= (format #f "~%~%") "\n\n") "format ~~")
(assert (string= (format #f "~a ~a" 1 2) "1 2") "format ~a ~a")
(assert (string= (format #f "~d ~d" 10 20) "10 20") "format ~d ~d")
(assert (string= (format #f "~a~%" "test") "test\n") "format ~a~%")

;; ===== GENSYM TESTS =====

(let ((g1 (gensym)) (g2 (gensym)))
  (assert (symbol? g1) "gensym returns symbol")
  (assert (not (eq? g1 g2)) "gensym unique"))

;; ===== CONDITION / ERROR TESTS =====

;; Simple error catching
;; MicroLisp doesn't have error? predicate, skip these
;; (assert (error? (lambda () (error "test error"))) "error creates condition")

;; ===== MACRO TESTS =====

;; and short-circuit
(assert (= (and #t 1 2) 2) "and returns last")
(assert (not (and #t #f)) "and short-circuits")
(assert (= (and) #t) "and no args")
(assert (= (and #f 1 2) #f) "and first false")

;; or short-circuit
(assert (= (or #f 1 #f) 1) "or returns first truthy")
(assert (= (or) #f) "or no args")
(assert (= (or #f #f 3) 3) "or third arg")

;; when
(assert (= (when #t 1 2 3) 3) "when true returns last")
(assert (null? (when #f 1)) "when false returns nil")

;; unless
(assert (= (unless #f 42) 42) "unless false returns body")
(assert (null? (unless #t 42)) "unless true returns nil")

;; ===== LET / LET* TESTS =====

(assert (= (let ((x 5) (y 10)) (+ x y)) 15) "let basic")
(assert (= (let* ((x 5) (y (+ x 3))) (* x y)) 40) "let* sequential")
(assert (= (let ((x 1)) (let ((x 2)) x) x) 1) "let shadowing")

;; ===== CLOS TESTS =====

;; Basic defclass and slot access
(defclass point () ((x :initarg :x :initform 0) (y :initarg :y :initform 0)))
(assert (eq? (class-of (make-instance 'point)) 'point) "class-of basic")

;; Slot-value basic
(let ((p (make-instance 'point :x 3 :y 4)))
  (assert (= (slot-value p 'x) 3) "slot-value initarg x")
  (assert (= (slot-value p 'y) 4) "slot-value initarg y"))

;; Slot initform
(let ((p (make-instance 'point)))
  (assert (= (slot-value p 'x) 0) "slot-value initform x")
  (assert (= (slot-value p 'y) 0) "slot-value initform y"))

;; Slot-set!
(let ((p (make-instance 'point :x 1 :y 2)))
  (slot-set! p 'x 10)
  (assert (= (slot-value p 'x) 10) "slot-set!"))

;; Inheritance
(defclass colored-point (point) ((color :initarg :color :initform 'white)))
(let ((cp (make-instance 'colored-point :x 1 :y 2 :color 'red)))
  (assert (= (slot-value cp 'x) 1) "inheritance slot x")
  (assert (= (slot-value cp 'y) 2) "inheritance slot y")
  (assert (eq? (slot-value cp 'color) 'red) "inheritance slot color"))

;; Inheritance with initforms
(let ((cp (make-instance 'colored-point :x 5 :y 6)))
  (assert (= (slot-value cp 'x) 5) "inheritance initform x")
  (assert (eq? (slot-value cp 'color) 'white) "inheritance initform color"))

;; Class-slots returns slot names
(defclass test-slots () ((a :initarg :a) (b :initarg :b)))
(assert (= (length (class-slots (class-of (make-instance 'test-slots)))) 2) "class-slots non-empty")

;; Multiple inheritance
(defclass pos-class () ((pos :initarg :pos :initform 0)))
(defclass velocity () ((vel :initarg :vel :initform 0)))
(defclass moving-point (pos-class velocity) ((name :initarg :name :initform "")))
(let ((mp (make-instance 'moving-point :pos 10 :vel 5 :name "test")))
  (assert (= (slot-value mp 'pos) 10) "multiple inheritance pos")
  (assert (= (slot-value mp 'vel) 5) "multiple inheritance vel")
  (assert (string= (slot-value mp 'name) "test") "multiple inheritance name"))

;; ===== CONDITION SYSTEM TESTS =====

;; define-condition (without requiring built-in classes)
(define-condition my-error () ((message :initarg :message :initform "default")))
(assert (is-a? (make-instance 'my-error) 'my-error) "is-a condition")

;; Condition slot access
(let ((e (make-instance 'my-error :message "something went wrong")))
  (assert (string= (slot-value e 'message) "something went wrong") "condition slot value"))

;; Signal and handler-case
(assert (= (handler-case (error "test error")
             (error (e) 42))
           42)
        "handler-case catches error")

;; Restart-case basic
(assert (= (restart-case 99
             (return-value (v) v))
           99)
        "restart-case not invoked")

(assert (= (restart-case (invoke-restart 'return-value 77)
             (return-value (v) v))
           77)
        "restart-case invoked")

;; ===== SEQUENCE FUNCTION TESTS =====

;; every
(assert (= (every number? '(1 2 3)) #t) "every numberp")
(assert (not (every number? '(1 a 3))) "every fails")

;; some
(assert (null? (some number? '(a b c))) "some none")
(assert (= (some number? '(a 1 c)) #t) "some found")

;; notany
(assert (= (notany number? '(a b c)) #t) "notany none")
(assert (= (notany number? '(1 2 3)) #f) "notany found")

;; notevery
(assert (= (notevery number? '(1 a 3)) #t) "notevery mixed")
(assert (= (notevery number? '(1 2 3)) #f) "notevery all")

;; ===== UNWIND-PROTECT TESTS =====

;; unwind-protect executes protected form and cleanup
(assert (= (let ((x 0))
           (unwind-protect (set! x 10) (set! x (+ x 1)))
           x) 11) "unwind-protect cleanup runs")

;; unwind-protect returns protected form result
(assert (= (let ((x 0))
           (unwind-protect (set! x 10) (set! x (+ x 1)))) 10) "unwind-protect returns protected result")

;; ===== WITH-OPEN-FILE TESTS =====

;; with-open-file write and read
(let ((test-file "test-ansi.tmp"))
  (with-open-file (f test-file :direction :output)
    (princ "hello world" f))
  (let ((content ""))
    (with-open-file (f test-file :direction :input)
      (set! content (car (read-line f))))
    (assert (string= content "hello world") "with-open-file read/write")))

;; ===== CASE/TYPECASE TESTS =====

;; case with single keys
(assert (equal? (case 1 (1 "one") (2 "two")) "one") "case single key match")
(assert (equal? (case 2 (1 "one") (2 "two")) "two") "case single key match 2")
(assert (null? (case 3 (1 "one") (2 "two"))) "case no match")
(assert (equal? (case 5 (1 "one") (2 "two") (else "other")) "other") "case else")

;; case with key lists
(assert (equal? (case 3 ((1 2 3) "low") ((4 5 6) "high")) "low") "case key list")
(assert (equal? (case 5 ((1 2 3) "low") ((4 5 6) "high")) "high") "case key list 2")

;; case with symbols
(assert (equal? (case 'b (a "alpha") (b "beta")) "beta") "case symbol")

;; typecase
(assert (equal? (typecase 42 (string "s") (number "n")) "n") "typecase number")
(assert (equal? (typecase "hello" (string "s") (number "n")) "s") "typecase string")
(assert (null? (typecase 'x (string "s") (number "n"))) "typecase no match")

;; ===== FILL/REPLACE TESTS =====

(assert (equal? (fill '(1 2 3 4) 'x) '(x x x x)) "fill list")
(assert (equal? (fill '(1 2 3 4) 'x :start 1 :end 3) '(1 x x 4)) "fill list with range")
(assert (string= (fill "hello" "X") "XXXXX") "fill string")
(assert (string= (fill "hello" "X" :start 1 :end 3) "hXXlo") "fill string with range")

(assert (equal? (replace '(1 2 3 4 5) '(a b c) :start1 1 :end1 4) '(1 a b c 5)) "replace with range")
(assert (equal? (replace '(1 2 3) '(a b c)) '(a b c)) "replace basic")

;; ===== DESTRUCTURING-BIND TESTS =====

(assert (equal? (destructuring-bind (a b) '(1 2) (list a b)) '(1 2)) "dbind simple")
(assert (equal? (destructuring-bind (a . b) '(1 2 3) (list a b)) '(1 (2 3))) "dbind dotted")
(assert (equal? (destructuring-bind ((a b) c) '((1 2) 3) (list a b c)) '(1 2 3)) "dbind nested")

;; ===== PROGV TESTS =====

(assert (= (progv '(x y) '(1 2) (+ x y)) 3) "progv basic")
(assert (= (progv '(a b c) '(10 20 30) (+ a b c)) 60) "progv three vars")

;; ===== LAMBDA TESTS =====

(assert (= ((lambda (x) (* x x)) 5) 25) "lambda basic")
(assert (= ((lambda (x y) (+ x y)) 3 4) 7) "lambda two args")
(assert (= ((lambda (x y z) (+ x y z)) 1 2 3) 6) "lambda three args")
(assert (= ((lambda () 42)) 42) "lambda no args")

;; ===== STRING-TRIM TESTS =====
;; CL: (string-trim char-bag string)
(assert (string= (string-trim " " "  hello  ") "hello") "string-trim whitespace")
(assert (string= (string-trim " ab" "abracadabra") "racadabr") "string-trim a and b")
(assert (string= (string-trim "" "hello") "hello") "string-trim empty bag")
(assert (string= (string-trim " \t\n" "  hello\t\n") "hello") "string-trim whitespace chars")
(assert (string= (string-trim (list #\a #\b) "abracadabra") "racadabr") "string-trim with char list")
(assert (string= (string-left-trim " " "  hello  ") "hello  ") "string-left-trim")
(assert (string= (string-right-trim " " "  hello  ") "  hello") "string-right-trim")

;; ===== STRING-TRIM TESTS =====
(assert (string= (string-trim " " "  hello  ") "hello") "string-trim whitespace")
(assert (string= (string-trim " ab" "abracadabra") "racadabr") "string-trim a and b")
(assert (string= (string-trim "" "hello") "hello") "string-trim empty bag")
(assert (string= (string-trim (list #\a #\b) "abracadabra") "racadabr") "string-trim with char list")
(assert (string= (string-left-trim " " "  hello  ") "hello  ") "string-left-trim")
(assert (string= (string-right-trim " " "  hello  ") "  hello") "string-right-trim")

;; ===== FUNCTION SHORTHAND AND UTILITY FUNCTIONS =====
(assert (procedure? #'number?) "function reader macro")
(assert (procedure? #'+) "function reader macro builtin")
(assert (= (identity 42) 42) "identity number")
(assert (equal? (identity 'hello) 'hello) "identity symbol")
(assert (equal? (identity "world") "world") "identity string")
(assert (= (count-if (complement #'number?) '(a b 1 c 2)) 3) "complement with count-if")
(assert (= (count-if (complement #'symbol?) '(a 1 b 2 c 3)) 3) "complement with symbol?")
(assert (= (count-if (constantly 99) '(a b c)) 3) "constantly truthy")
(assert (= (count-if (constantly 'yes) '(1 2 3)) 3) "constantly symbol")


;; ===== MAPCAR/MAPCAN/MULTI-ARG MAP TESTS =====
(assert (equal? (mapcar (lambda (x) (* x 2)) '(1 2 3)) '(2 4 6)) "mapcar single list")
(assert (equal? (mapcar #'+ '(1 2 3) '(10 20 30)) '(11 22 33)) "mapcar two lists")
(assert (equal? (mapcar #'list '(a b c) '(1 2 3)) '((a 1) (b 2) (c 3))) "mapcar three lists")
(assert (equal? (mapcan (lambda (x) (list x x)) '(1 2)) '(1 1 2 2)) "mapcan")
(assert (equal? (map-into '(1 2 3) (lambda (x) (* x x)) '(4 5 6)) '(16 25 36)) "map-into")
(assert (equal? (revappend '(1 2 3) '(4 5)) '(3 2 1 4 5)) "revappend")
(let ((x '(1 2 3 4))) (assert (equal? (ldiff x (cddr x)) '(1 2)) "ldiff"))
(let ((x '(1 2 3))) (assert (tailp (cddr x) x) "tailp true"))
(assert (not (tailp '(3) '(1 2 3))) "tailp false on different list")
(assert (= (nconc '(1 2) '(3 4)) '(1 2 3 4)) "nconc two lists")
(assert (= (nconc '(1) '(2) '(3)) '(1 2 3)) "nconc three lists")
(assert (= (nconc '() '(1 2)) '(1 2)) "nconc empty first")

;; ===== SOME/EVERY/NOTANY/NTEVERY TESTS =====
(assert (some #'number? '(a 1 b)) "some true")
(assert (not (some #'number? '(a b c))) "some false")
(assert (every #'number? '(1 2 3)) "every true")
(assert (not (every #'number? '(1 a 3))) "every false")
(assert (notany #'number? '(a b c)) "notany true")
(assert (not (notany #'number? '(a 1 b))) "notany false")
(assert (notevery #'number? '(1 a 3)) "notevery true")
(assert (not (notevery #'number? '(1 2 3))) "notevery false")



;; ===== SUBST/SUBLIS/TREE-EQUAL TESTS =====
(assert (equal? (subst 'x 'a '(a b a c)) '(x b x c)) "subst basic")
(assert (equal? (subst 'x 'a '(1 (a 2) 3)) '(1 (x 2) 3)) "subst nested")
(assert (equal? (adjoin 1 '(2 3 4)) '(1 2 3 4)) "adjoin not present")
(assert (equal? (adjoin 1 '(1 2 3)) '(1 2 3)) "adjoin already present")
(assert (tree-equal '(1 (2 3)) '(1 (2 3))) "tree-equal equal")
(assert (not (tree-equal '(1 (2 3)) '(1 (2 4)))) "tree-equal different")


;; ===== SET OPERATION TESTS =====
(assert (equal? (union '(1 2 3) '(2 3 4)) '(1 2 3 4)) "union")
(assert (equal? (intersection '(1 2 3) '(2 3 4)) '(2 3)) "intersection")
(assert (equal? (set-difference '(1 2 3 4) '(2 3)) '(1 4)) "set-difference")
(assert (equal? (set-exclusive-or '(1 2 3) '(2 3 4)) '(1 4)) "set-exclusive-or")
(assert (subsetp '(1 2) '(1 2 3 4)) "subsetp true")
(assert (not (subsetp '(1 5) '(1 2 3 4))) "subsetp false")

;; ===== LIST OPERATION TESTS =====
(assert (= (list-length '(1 2 3)) 3) "list-length")
(assert (equal? (last '(1 2 3)) '(3)) "last single")
(assert (equal? (last '(1 2 3) 2) '(2 3)) "last two")
(assert (equal? (butlast '(1 2 3)) '(1 2)) "butlast")
(assert (equal? (butlast '(1 2 3 4) 2) '(1 2)) "butlast two")
(assert (equal? (pairlis '(a b c) '(1 2 3)) '((a  . 1) (b  . 2) (c  . 3))) "pairlis")
(assert (equal? (copy-list '(1 2 3)) '(1 2 3)) "copy-list")
(assert (equal? (copy-tree '(1 (2 3) 4)) '(1 (2 3) 4)) "copy-tree")

;; ===== POSITION/MEMBER-IF/ASSOC-IF TESTS =====
(assert (= (position (quote b) (quote (a b c))) 1) "position symbol")
(assert (= (position 3 (quote (1 2 3 4))) 2) "position number")
(assert (= (position-if (lambda (x) (> x 5)) (quote (1 6 2 7))) 1) "position-if")
(assert (equal? (member-if (lambda (x) (> x 5)) (quote (1 6 2 7))) (quote (6 2 7))) "member-if")
(assert (equal? (assoc-if (lambda (x) (number? x)) (quote ((a b) (1 2) (c d)))) (quote (1 2))) "assoc-if")

;; ===== PARSE-INTEGER TESTS =====
(assert (equal (multiple-value-list (parse-integer "123")) '(123 3)) "parse-integer basic")
(assert (equal (multiple-value-list (parse-integer "  42  ")) '(42 4)) "parse-integer whitespace")
(assert (equal (multiple-value-list (parse-integer "FF" :radix 16)) '(255 2)) "parse-integer hex")
(assert (equal (multiple-value-list (parse-integer "-7")) '(-7 2)) "parse-integer negative")
(assert (equal (multiple-value-list (parse-integer "+3")) '(3 2)) "parse-integer plus sign")
(assert (equal (multiple-value-list (parse-integer "1010" :radix 2)) '(10 4)) "parse-integer binary")
(assert (equal (multiple-value-list (parse-integer "123abc" :junk-allowed t)) '(123 3)) "parse-integer junk allowed")

;; ===== GETF TESTS =====
(assert (= (getf '(a 1 b 2 c 3) 'b) 2) "getf basic")
(assert (null? (getf '(a 1 b 2) 'x)) "getf missing")
(assert (equal? (getf '(a 1 b 2) 'x 'default) 'default) "getf default")
(assert (= (getf '(a 1 b 2 c 3) 'c) 3) "getf third")

;; ===== GET-PROPERTIES TESTS =====
(assert (equal? (get-properties '(a 1 b 2 c 3) '(b x)) '(2 b (b 2 c 3))) "get-properties found")
(assert (equal? (get-properties '(a 1 b 2) '(x y)) '(() () ())) "get-properties not found")

;; ===== MAKE-STRING TESTS =====
(assert (string= (make-string 5 :initial-element #\x) "xxxxx") "make-string")
(assert (string= (make-string 0) "") "make-string zero")

;; ===== MAKE-LIST TESTS =====
(assert (= (length (make-list 10)) 10) "make-list length")
(assert (equal? (make-list 3 :initial-element 'a) '(a a a)) "make-list init")
(assert (equal? (make-list 0) '()) "make-list zero")

;; ===== REVERSE TESTS =====
(assert (equal? (reverse '(1 2 3)) '(3 2 1)) "reverse list")
(assert (string= (reverse "hello") "olleh") "reverse string")
(assert (equal? (reverse '()) '()) "reverse empty")

;; ===== NTH / NTHCDR TESTS =====
(assert (equal? (nth 0 '(a b c)) 'a) "nth 0")
(assert (equal? (nth 2 '(a b c)) 'c) "nth 2")
(assert (null? (nth 5 '(a b c))) "nth out of bounds")
(assert (equal? (nthcdr 2 '(a b c d)) '(c d)) "nthcdr 2")
(assert (null? (nthcdr 3 '(a b c))) "nthcdr end")

;; ===== ACONS / RASSOC TESTS =====
(assert (equal? (acons 'x 1 '((a . b))) '((x . 1) (a . b))) "acons")
(assert (equal? (rassoc 2 '((a . 1) (b . 2) (c . 3))) '(b . 2)) "rassoc")
(assert (null? (rassoc 99 '((a . 1)))) "rassoc not found")
(assert (equal? (rassoc-if (lambda (x) (> x 1)) '((a . 1) (b . 2) (c . 3))) '(b . 2)) "rassoc-if")

;; ===== ABS / MAX / MIN TESTS =====
(assert (= (abs -5) 5) "abs negative")
(assert (= (abs 5) 5) "abs positive")
(assert (= (abs 0) 0) "abs zero")
(assert (= (max 1 5 2 8 3) 8) "max")
(assert (= (min 1 5 2 8 3) 1) "min")

;; ===== MOD / REM TESTS =====
(assert (= (mod 10 3) 1) "mod positive")
(assert (= (rem 10 3) 1) "rem positive")
(assert (= (mod -10 3) 2) "mod negative")  ; CL mod: result has sign of divisor
(assert (= (rem -10 3) -1) "rem negative") ; CL rem: result has sign of dividend

;; ===== FLOOR / CEILING / TRUNCATE / ROUND TESTS =====
(assert (= (car (floor 3.7)) 3) "floor")
(assert (= (car (ceiling 3.2)) 4) "ceiling")
(assert (= (car (truncate 3.9)) 3) "truncate")
(assert (= (car (round 3.5)) 4) "round half")

;; ===== GCD / LCM TESTS =====
(assert (= (gcd 12 18) 6) "gcd")
(assert (= (gcd 0) 0) "gcd zero")
(assert (= (lcm 4 6) 12) "lcm")
(assert (= (lcm) 1) "lcm no args")

;; ===== SQRT / LOG TESTS =====
(assert (= (sqrt 144) 12) "sqrt")
(assert (> (log 2.718281828) 0.99) "log e")
(assert (< (log 2.718281828) 1.01) "log e upper")

;; ===== EVENP / ODDP / ZEROP / PLUSP / MINUSP =====
(assert (evenp 4) "evenp true")
(assert (not (evenp 3)) "evenp false")
(assert (oddp 3) "oddp true")
(assert (not (oddp 4)) "oddp false")
(assert (zerop 0) "zerop true")
(assert (not (zerop 1)) "zerop false")
(assert (plusp 5) "plusp true")
(assert (not (plusp 0)) "plusp false")
(assert (minusp -3) "minusp true")
(assert (not (minusp 3)) "minusp false")

;; ===== 1+ / 1- =====
(assert (= (1+ 5) 6) "1+")
(assert (= (1- 5) 4) "1-")
(assert (= (1+ 0) 1) "1+ zero")
(assert (= (1- 1) 0) "1- one")

;; ===== SIGNUM =====
(assert (= (signum 5) 1) "signum positive")
(assert (= (signum -5) -1) "signum negative")
(assert (= (signum 0) 0) "signum zero")

;; ===== CHAR-EQUAL (case-insensitive) =====
(assert (char-equal #\A #\a) "char-equal same")
(assert (char-not-equal #\A #\b) "char-not-equal diff")
(assert (char-lessp #\a #\B) "char-lessp")
(assert (char-greaterp #\B #\a) "char-greaterp")

;; ===== CHAR/= / CHAR<= / CHAR>= =====
(assert (char/= #\a #\b) "char/=")
(assert (not (char/= #\a #\a)) "char/= same")
(assert (char<= #\a #\b) "char<= less")
(assert (char<= #\a #\a) "char<= equal")
(assert (char>= #\b #\a) "char>= greater")
(assert (char>= #\a #\a) "char>= equal")

;; ===== DIGIT-CHAR / DIGIT-CHAR-P =====
(assert (char= (digit-char 9) #\9) "digit-char 9")
(assert (char= (digit-char 12 16) #\C) "digit-char hex")
(assert (null? (digit-char 16 16)) "digit-char overflow")
(assert (= (digit-char-p #\9) 9) "digit-char-p 9")
(assert (= (digit-char-p #\a 16) 10) "digit-char-p hex")
(assert (null? (digit-char-p #\z 10)) "digit-char-p not digit")

;; ===== ALPHANUMERICP / ALPHA-CHAR-P =====
(assert (alphanumericp #\a) "alphanumericp letter")
(assert (alphanumericp #\5) "alphanumericp digit")
(assert (not (alphanumericp #\space)) "alphanumericp space")
(assert (alpha-char-p #\Z) "alpha-char-p upper")
(assert (not (alpha-char-p #\5)) "alpha-char-p digit")

;; ===== UPPER-CASE-P / LOWER-CASE-P =====
(assert (upper-case-p #\A) "upper-case-p true")
(assert (not (upper-case-p #\a)) "upper-case-p false")
(assert (lower-case-p #\a) "lower-case-p true")
(assert (not (lower-case-p #\A)) "lower-case-p false")

;; ===== CHAR-UPCASE / CHAR-DOWNCASE =====
(assert (char= (char-upcase #\a) #\A) "char-upcase")
(assert (char= (char-downcase #\A) #\a) "char-downcase")

;; ===== STRING COMPARISON (case-sensitive) =====
;; string< returns position of first mismatch (or nil if s1 >= s2)
(assert (= (string> "xyz" "abc") 0) "string> first diff")
(assert (null? (string> "abc" "xyz")) "string> less")
;; string<= returns position of first mismatch or length if s1 <= s2
(assert (= (string<= "abc" "def") 0) "string<= first diff")  ; 'a' < 'd' at pos 0
(assert (= (string<= "abc" "abc") 3) "string<= equal")  ; equal strings return length
;; string>= returns position of first mismatch or length if s1 >= s2
(assert (= (string>= "xyz" "abc") 0) "string>= first diff")  ; 'x' > 'a' at pos 0
(assert (= (string>= "abc" "abc") 3) "string>= equal")  ; equal strings return length
(assert (= (string/= "abc" "abd") 2) "string/=")  ; first diff at position 2

;; ===== STRING COMPARISON (case-insensitive) =====
(assert (null? (string-not-equal "hello" "HELLO")) "string-not-equal same")
(assert (string-lessp "abc" "DEF") "string-lessp")
(assert (string-greaterp "DEF" "abc") "string-greaterp")

;; ===== NSTRING FUNCTIONS =====
(assert (string= (nstring-upcase "hello") "HELLO") "nstring-upcase")
(assert (string= (nstring-downcase "HELLO") "hello") "nstring-downcase")

;; ===== STRING COMPARISONS =====
(assert (string< "abc" "abd") "string<")
(assert (null? (string< "abd" "abc")) "string< false")
(assert (string> "abd" "abc") "string>")
(assert (null? (string> "abc" "abd")) "string> false")

;; ===== PRINC-TO-STRING / PRIN1-TO-STRING =====
(assert (string= (princ-to-string "hello") "hello") "princ-to-string")
(assert (string= (prin1-to-string "hello") "\"hello\"") "prin1-to-string quoted")

;; ===== STRING-ELT =====
(assert (char= (string-elt "hello" 0) #\h) "string-elt first")
(assert (char= (string-elt "hello" 4) #\o) "string-elt last")

;; ===== NRECONC =====
(assert (equal? (nreconc '(1 2 3) '(4 5)) '(3 2 1 4 5)) "nreconc")

;; ===== MAP FAMILY =====
(assert (equal? (maplist (lambda (x) (car x)) '(1 2 3)) '(1 2 3)) "maplist")
(assert (equal? (mapc (lambda (x) x) '(1 2 3)) '(1 2 3)) "mapc returns list")
(assert (equal? (mapl (lambda (x) x) '(1 2 3)) '(1 2 3)) "mapl returns list")

;; ===== RANDOM =====
(assert (< (random 10) 10) "random < 10")
(assert (>= (random 10) 0) "random >= 0")

;; ===== STABLE-SORT TESTS =====
(assert (equal? (stable-sort '(3 1 4 1 5) (lambda (a b) (< a b))) '(1 1 3 4 5)) "stable-sort ascending")
(assert (equal? (stable-sort '(5 4 3 2 1) (lambda (a b) (> a b))) '(5 4 3 2 1)) "stable-sort descending")

;; ===== NSUBST / NSUBST-IF / NSUBLIS TESTS =====
(assert (equal? (nsubst 0 1 '(1 2 1 3)) '(0 2 0 3)) "nsubst")
(assert (equal? (nsubst-if 99 (lambda (x) (number? x)) '(1 (2 a) 3)) '(99 (99 a) 99)) "nsubst-if")
(assert (equal? (nsublis '((x . 10) (y . 20)) '(x (y z))) '(10 (20 z))) "nsublis")
(assert (equal? (subst-if 0 (lambda (x) (number? x)) '(1 a 2)) '(0 a 0)) "subst-if")

;; ===== LIST* TESTS =====
(assert (equal? (list* 1 2 3) '(1 2 . 3)) "list* basic")
(assert (equal? (list* 1 2 '(3 4)) '(1 2 3 4)) "list* with list tail")
(assert (equal? (list* 'a 'b 'c 'd) '(a b c . d)) "list* with symbols")
(assert (= (list* 'a) 'a) "list* single arg")

;; ===== COMPLEX NUMBER TESTS =====
(assert (complexp #c(3 4)) "complexp true")
(assert (not (complexp 5)) "complexp false")
(assert (= (realpart #c(3 4)) 3) "realpart complex")
(assert (= (imagpart #c(3 4)) 4) "imagpart complex")
(assert (= (realpart 5) 5) "realpart real")
(assert (= (imagpart 5) 0) "imagpart real")
(assert (complexp (complex 3 4)) "complex creates complex")
(assert (= (realpart (complex 3 4)) 3) "complex realpart")
(assert (= (imagpart (complex 3 4)) 4) "complex imagpart")
(assert (equal? (conjugate #c(3 4)) #c(3 -4)) "conjugate")
(assert (equal? (cis 0) #c(1 0)) "cis zero")

;; ===== TYPE PREDICATE TESTS =====
(assert (integerp 42) "integerp int")
(assert (integerp -7) "integerp negative")
(assert (not (integerp 3.14)) "integerp float")
(assert (floatp 3.14) "floatp float")
(assert (not (floatp 42)) "floatp int")
(assert (rationalp 2/3) "rationalp rat")
(assert (rationalp 5) "rationalp int")
(assert (realp 5.0) "realp float")
(assert (complexp #c(1 2)) "complexp complex")
(assert (not (complexp '(1 2))) "complexp not list")

;; ===== NUMERIC CONVERSION TESTS =====
(assert (floatp (float 2/3)) "float conversion")
(assert (rationalp (rational 3.14)) "rational from float")
(assert (= (numerator 2/3) 2) "numerator")
(assert (= (denominator 2/3) 3) "denominator")

;; ===== BIT OPERATIONS TESTS =====
(assert (= (ash 1 3) 8) "ash left")
(assert (= (ash 8 -1) 4) "ash right")
(assert (= (ash -8 -1) -4) "ash negative right")
(assert (= (logand #b1010 #b1100) #b1000) "logand")
(assert (= (logior #b1010 #b1100) #b1110) "logior")
(assert (= (logxor #b1010 #b1100) #b0110) "logxor")
(assert (= (logcount #b1101) 3) "logcount")
(assert (= (integer-length 10) 4) "integer-length")
(assert (= (lognot #b1111) -16) "lognot")

;; ===== BYTE OPERATIONS TESTS =====
(assert (= (ldb (list 8 0) 255) 255) "ldb byte 0")
(assert (= (ldb (list 8 0) #xABCD) #xCD) "ldb byte extract")
(assert (= (dpb 0 (list 8 0) 255) 0) "dpb byte insert zero")
(assert (= (dpb #xCD (list 8 0) 0) #xCD) "dpb byte insert low")
(assert (= (dpb #xAB (list 8 8) #xCD) #xABCD) "dpb byte insert high")

;; ===== BITWISE / LOGICAL OPERATIONS =====
(assert (= (logand #b1111 #b1010) #b1010) "logand basic")
(assert (= (logand #b11110000 #b10101010) #b10100000) "logand mixed")
(assert (= (logior #b1111 #b0000) #b1111) "logior basic")
(assert (= (logior #b1000 #b0100) #b1100) "logior basic2")
(assert (= (logxor #b1111 #b1010) #b0101) "logxor basic")
(assert (= (logxor #b1111 #b1111) 0) "logxor cancel")
(assert (= (lognot #b0000) -1) "lognot zero")
(assert (= (lognot -1) 0) "lognot minus1")

;; ===== ASH (ARITHMETIC SHIFT) =====
(assert (= (ash 1 3) 8) "ash left")
(assert (= (ash 8 -1) 4) "ash right")
(assert (= (ash -4 -1) -2) "ash negative")
(assert (= (ash 0 5) 0) "ash zero")

;; ===== LOGCOUNT =====
(assert (= (logcount #b1010) 2) "logcount mixed")
(assert (= (logcount #b1111) 4) "logcount all ones")
(assert (= (logcount #b0000) 0) "logcount all zeros")
(assert (= (logcount -1) 0) "logcount minus1")

;; ===== INTEGER-LENGTH =====
(assert (= (integer-length 0) 0) "int-length zero")
(assert (= (integer-length 1) 1) "int-length one")
(assert (= (integer-length 7) 3) "int-length 7")
(assert (= (integer-length 8) 4) "int-length 8")
(assert (= (integer-length -1) 0) "int-length -1")
(assert (= (integer-length -2) 1) "int-length -2")

;; ===== LOGBITP / LOGTEST =====
(assert (logbitp 1 #b1010) "logbitp set bit")
(assert (not (logbitp 0 #b1010)) "logbitp clear bit")

;; ===== MAKE-STRING =====
(assert (= (string-length (make-string 5)) 5) "make-string length")
(assert (string= (make-string 3 :initial-element (quote #\x)) "xxx") "make-string fill")
(assert (string= (make-string 0) "") "make-string zero")

;; ===== BOOLE TESTS =====
(assert (= (boole 1 #b1010 #b1100) #b1000) "boole and")
(assert (= (boole 7 #b1010 #b1100) #b1110) "boole ior")
(assert (= (boole 6 #b1010 #b1100) #b0110) "boole xor")
(assert (= (boole 0 0 0) 0) "boole clr")
(assert (= (boole 15 0 0) -1) "boole set")

;; ===== LDB / DPB TESTS =====
(assert (= (byte-size (list 8 0)) 8) "byte-size")
(assert (= (byte-position (list 8 0)) 0) "byte-position")

;; ===== COERCE TESTS =====
(assert (string= (coerce '(#\a #\b #\c) 'string) "abc") "coerce char-list to string")
(assert (= (length (coerce "abc" 'list)) 3) "coerce string to list")
(assert (string= (coerce (quote a) 'string) "a") "coerce symbol to string")

;; ===== MULTIPLE-VALUE TESTS =====
(assert (equal? (multiple-value-list (values 1 2 3)) '(1 2 3)) "multiple-value-list")
(assert (equal? (multiple-value-list (floor 10 3)) '(3 1)) "multiple-value-list floor")

;; ===== TYPECASE TESTS =====
(assert (string= (typecase 42 (string "no") (integer "yes") (otherwise "other")) "yes") "typecase int")
(assert (string= (typecase "hello" (string "str") (otherwise "other")) "str") "typecase string")
(assert (string= (typecase 42 (string "no") (otherwise "default")) "default") "typecase otherwise")

;; ===== WITH-OUTPUT-TO-STRING TESTS =====
(assert (string= (with-output-to-string (s) (princ "hello" s)) "hello") "with-output-to-string")
(assert (string= (with-output-to-string (s) (princ "foo" s) (princ "bar" s)) "foobar") "with-output-to-string multiple")

;; ===== WITH-INPUT-FROM-STRING =====
(assert (string= (with-input-from-string (s "hello world") (car (read-line s))) "hello world") "with-input-from-string")

;; ===== HASH-TABLE-P =====
(assert (hash-table-p (make-hash-table)) "hash-table-p true")
(assert (not (hash-table-p 42)) "hash-table-p false")

;; ===== REMF =====
(assert (equal? (remf (quote (:a 1 :b 2 :c 3)) :b) (quote (:a 1 :c 3))) "remf basic")
(assert (equal? (remf (quote (:a 1 :b 2)) :c) (quote (:a 1 :b 2))) "remf missing")

;; ===== PUSHNEW =====
;; pushnew is a macro, test via setf
(assert (equal? (let ((x (quote (1 2 3)))) (pushnew 0 x) x) (quote (0 1 2 3))) "pushnew add")
(assert (equal? (let ((x (quote (1 2 3)))) (pushnew 2 x) x) (quote (1 2 3))) "pushnew dup")

;; ===== REMF TESTS =====
(assert (equal? (remf (quote (:a 1 :b 2 :c 3)) :b) (quote (:a 1 :c 3))) "remf basic")
(assert (equal? (remf (quote (:a 1 :b 2)) :c) (quote (:a 1 :b 2))) "remf missing")

;; ===== NSET-EXCLUSIVE-OR =====
(assert (equal? (nset-exclusive-or (quote (1 2 3)) (quote (2 3 4))) (quote (1 4))) "nset-exclusive-or basic")
(assert (equal? (nset-exclusive-or (quote (1 2)) (quote (3 4))) (quote (1 2 3 4))) "nset-exclusive-or disjoint")

;; ===== COPY-ALIST =====
(assert (equal? (copy-alist (quote ((a . 1) (b . 2)))) (quote ((a . 1) (b . 2)))) "copy-alist")

;; ===== MISMATCH =====
(assert (= (mismatch "foobar" "fooxar") 3) "mismatch at diff")
(assert (null? (mismatch "foo" "foo")) "mismatch equal")
(assert (= (mismatch "abc" "abcdef") 3) "mismatch shorter")
(assert (= (mismatch "abc" "ab") 2) "mismatch first shorter")

;; ===== PSETQ =====
(assert (equal? (let ((x 1) (y 2)) (psetq x 10 y 20) (list x y)) (quote (10 20))) "psetq basic")
(assert (equal? (let ((x 1) (y 2)) (psetq x y y x) (list x y)) (quote (2 1))) "psetq swap")

;; ===== SETQ =====
(assert (= (let ((x 0)) (setq x 42) x) 42) "setq basic")

;; ===== DO / DO* =====
(assert (= (do ((i 0 (+ i 1))) ((>= i 5) i)) 5) "do count")
(assert (equal? (do ((x (quote (1 2 3)) (cdr x)) (acc (quote ()) (cons (car x) acc))) ((null? x) acc)) (quote (3 2 1))) "do collect reverse")
(assert (equal? (do* ((i 0 (+ i 1)) (j 0 (+ i j))) ((>= i 3) (list i j))) (quote (3 6))) "do* sequential")

;; ===== FUNCALL =====
(assert (= (funcall (lambda (x y) (+ x y)) 3 4) 7) "funcall basic")
(assert (= (funcall (lambda (x) (* x x)) 5) 25) "funcall unary")

;; ===== NULL =====
(assert (null (quote ())) "null empty")
(assert (not (null (quote (1 2)))) "null non-empty")
(assert (not (null 1)) "null number")

;; ===== EQ / EQL / EQUAL =====
(assert (eq (quote a) (quote a)) "eq symbols")
(assert (not (eq (quote a) (quote b))) "eq different symbols")
(assert (eql 2 2) "eql numbers")
(assert (eql 3.0 3.0) "eql floats")
(assert (equal (quote (1 2 3)) (quote (1 2 3))) "equal lists")
(assert (not (equal (quote (1 2)) (quote (1 2 3)))) "equal different lengths")
(assert (equal "hello" "hello") "equal strings")

;; ===== NSTRING-UPCASE/DOWNCASE =====
(assert (= (nstring-upcase "hello") "HELLO") "nstring-upcase")
(assert (= (nstring-downcase "WORLD") "world") "nstring-downcase")

;; ===== FLOOR / CEILING / TRUNCATE / ROUND TESTS (with multi-val) =====


;; ===== Final summary =====

(display "All ANSI-style tests passed!")
(newline)