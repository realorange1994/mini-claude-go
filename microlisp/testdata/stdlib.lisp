;; ============================================================
;; Standard Library Tests
;; ============================================================

(load "tests/framework.lisp")
(start-suite "cXr Combinations")

(assert-equal 1 (caar '((1 2) 3)) "caar")
(assert-equal 2 (cadr '(1 2 3)) "cadr")
(assert-equal '(3) (cddr '(1 2 3)) "cddr")
(assert-equal 3 (caddr '(1 2 3)) "caddr")
(assert-equal 1 (caaar '(((1 2)) 3)) "caaar")
(assert-equal 2 (caadr '((1) (2 3))) "caadr")
(assert-equal 2 (cadar '((1 2 3))) "cadar")
(assert-equal 3 (caddr '(1 2 3)) "caddr")

(end-suite)

(start-suite "mapcar / filter / fold")

;; mapcar
(assert-equal '(2 4 6) (mapcar (lambda (x) (* x 2)) '(1 2 3)) "mapcar: double")
(assert-equal '(1 4 9) (mapcar square '(1 2 3)) "mapcar: squares")
(assert-equal '() (mapcar square '()) "mapcar: empty list")

;; filter
(assert-equal '(4 5) (filter (lambda (x) (> x 3)) '(1 2 3 4 5)) "filter: >3")
(assert-equal '(1 3 5) (filter (lambda (x) (= 1 (modulo x 2))) '(1 2 3 4 5))
  "filter: odds")
(assert-equal '() (filter (lambda (x) #f) '(1 2 3)) "filter: remove all")
(assert-equal '(1 2 3) (filter (lambda (x) #t) '(1 2 3)) "filter: keep all")
(assert-equal '() (filter number? '()) "filter: empty list")

;; fold (left)
(assert-equal 0 (fold + 0 '()) "fold: empty = 0")
(assert-equal 6 (fold + 0 '(1 2 3)) "fold: sum = 6")
(assert-equal 24 (fold * 1 '(1 2 3 4)) "fold: product = 24")
(assert-equal 10 (fold (lambda (x acc) (+ x acc)) 0 '(1 2 3 4)) "fold: sum lambda")
(assert-equal "cba" (fold (lambda (x acc) (string-append x acc)) "" '("a" "b" "c"))
  "fold: string reverse")

;; fold-right
(assert-equal 0 (fold-right + 0 '()) "fold-right: empty = 0")
(assert-equal 6 (fold-right + 0 '(1 2 3)) "fold-right: sum = 6")
(assert-equal '((1 1) (2 2)) (fold-right (lambda (x acc)
  (cons (list x x) acc)) '() '(1 2))
  "fold-right: construct list")

(end-suite)

(start-suite "not / boolean")

(assert-false (not #t) "not #t = #f")
(assert-true (not #f) "not #f = #t")
(assert-false (not 0) "not 0 = #f (0 is truthy)")
(assert-true (not '()) "not () = #t (() is nil/falsy)")

(end-suite)

(start-suite "length / append / reverse")

(assert-equal 0 (length '()) "length: empty")
(assert-equal 5 (length '(1 2 3 4 5)) "length: 5")
(assert-equal 3 (length '((1) (2) (3))) "length: nested")

(assert-equal '(1 2 3 4) (append '(1 2) '(3 4)) "append: two lists")
(assert-equal '(1 2) (append '(1 2) '()) "append: with empty")
(assert-equal '(1 2) (append '() '(1 2)) "append: from empty")
(assert-equal '() (append '() '()) "append: both empty")
(assert-equal '(1 2 3 4) (append (append '(1) '(2)) (append '(3) '(4))) "append: four lists")

(assert-equal '() (reverse '()) "reverse: empty")
(assert-equal '(1) (reverse '(1)) "reverse: one")
(assert-equal '(3 2 1) (reverse '(1 2 3)) "reverse: three")
(assert-equal '(5 4 3 2 1) (reverse '(1 2 3 4 5)) "reverse: 5")
(assert-equal '((6 7) (4 5)) (reverse '((4 5) (6 7))) "reverse: nested")

(end-suite)

(start-suite "range / list-ref")

(assert-equal '() (range 0) "range: 0 = ()")
(assert-equal '(0) (range 1) "range: 1 = (0)")
(assert-equal '(0 1 2 3 4) (range 5) "range: 5 = (0 1 2 3 4)")
(assert-equal 0 (car (range 10)) "range: first = 0")
(assert-equal 9 (list-ref (range 10) 9) "range: last = n-1")

(assert-equal 'a (list-ref '(a b c) 0) "list-ref: 0")
(assert-equal 'c (list-ref '(a b c) 2) "list-ref: 2")
(assert-equal '(3 4) (list-ref '((1 2) (3 4)) 1) "list-ref: nested")

(end-suite)

(start-suite "member? / any / all")

(assert-false (member? 1 '()) "member?: empty list")
(assert-true (member? 'a '(b a c)) "member?: found")
(assert-false (member? 'z '(a b c)) "member?: not found")
(assert-true (member? '(1 2) '((3 4) (1 2) (5 6))) "member?: nested using equal?")

(assert-false (any number? '()) "any: empty = #f")
(assert-false (any number? '(a b c)) "any: no numbers")
(assert-true (any number? '(a 2 c)) "any: has number")
(assert-true (any number? '(1 b c)) "any: first is number")
(assert-false (any (lambda (x) (> x 10)) '(1 2 3)) "any: all < 10")

(assert-true (all number? '()) "all: empty = #t")
(assert-true (all number? '(1 2 3)) "all: all numbers")
(assert-false (all number? '(1 2 'a)) "all: one non-number")
(assert-false (all number? '('a 2 3)) "all: first non-number")
(assert-true (all (lambda (x) (> x 0)) '(1 2 3)) "all: all positive")

(end-suite)

(start-suite "assoc")

(assert-nil (assoc 'a '()) "assoc: empty alist")
(define al '((a . 1) (b . 2) (c . 3)))
(assert-equal '(a . 1) (assoc 'a al) "assoc: dotted pair alist")
(assert-equal '(c . 3) (assoc 'c al) "assoc: last element")
(assert-nil (assoc 'z al) "assoc: not found")

(define al2 '((name "Alice") (age 30) (city "NYC")))
(assert-equal '(name "Alice") (assoc 'name al2) "assoc: with string values")
(assert-equal '(age 30) (assoc 'age al2) "assoc: with number value")

(end-suite)

(start-suite "take / drop / zip")

(assert-equal '() (take 0 '(1 2 3)) "take: 0")
(assert-equal '(1) (take 1 '(1 2 3)) "take: 1")
(assert-equal '(1 2 3) (take 5 '(1 2 3)) "take: > length")
(assert-equal '() (take 5 '()) "take: from empty")

(assert-equal '(1 2 3) (drop 0 '(1 2 3)) "drop: 0")
(assert-equal '(2 3) (drop 1 '(1 2 3)) "drop: 1")
(assert-equal '() (drop 5 '(1 2 3)) "drop: > length")
(assert-equal '() (drop 5 '()) "drop: from empty")

(assert-equal '() (zip '() '()) "zip: both empty")
(assert-equal '((1 a) (2 b)) (zip '(1 2) '(a b)) "zip: two lists")
(assert-equal '() (zip '(1) '()) "zip: second empty")

(end-suite)

(start-suite "abs / min / max")

(assert-equal 5 (abs -5) "abs: -5 = 5")
(assert-equal 5 (abs 5) "abs: 5 = 5")
(assert-equal 0 (abs 0) "abs: 0 = 0")

(assert-equal 5 (max 3 5) "max: 3,5 = 5")
(assert-equal 3 (min 3 5) "min: 3,5 = 3")
(assert-equal -5 (max -5 -10) "max: negatives")
(assert-equal -10 (min -5 -10) "min: negatives")

(end-suite)

(start-suite "flatten")

(assert-equal '() (flatten '()) "flatten: empty")
(assert-equal '(1 2 3) (flatten '(1 2 3)) "flatten: already flat")
(assert-equal '(1 2 3 4) (flatten '((1) (2 3) (4))) "flatten: nested")
(assert-equal '(1 2 3 4 5)
  (flatten '((1 (2 3)) (4 (5)))) "flatten: deep nesting")
(assert-equal '(a b c) (flatten '(((a) b) c)) "flatten: symbols")

(end-suite)

(start-suite "for-each")

(define fe-items '())
(for-each (lambda (x) (set! fe-items (cons x fe-items))) '(1 2 3 4 5))
(assert-equal 5 (length fe-items) "for-each: called 5 times")
;; Note: for-each returns #t on success
(assert-true (for-each display '()) "for-each: empty list returns #t")

(end-suite)

(start-suite "when / unless macros")

(define wu-x 0)
(when #t (set! wu-x 1))
(assert-equal 1 wu-x "when: #t executes body")

(when #f (set! wu-x 2))
(assert-equal 1 wu-x "when: #f does not execute body")

(unless #f (set! wu-x 3))
(assert-equal 3 wu-x "unless: #f executes body")

(unless #t (set! wu-x 4))
(assert-equal 3 wu-x "unless: #t does not execute body")

;; Multiple body expressions
(define wu-multi 0)
(when #t
  (set! wu-multi 1)
  (set! wu-multi (+ wu-multi 1)))
(assert-equal 2 wu-multi "when: multiple body expressions")

(end-suite)

(test-summary)
