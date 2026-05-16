;; ============================================================
;; Closure & Higher-Order Function Edge Cases
;; ============================================================

(load "tests/framework.lisp")

;; ============================================================
;; Closure Edge Cases
;; ============================================================
(start-suite "closure-edge-cases")

;; Closure with set! modifying captured variable
(define (make-counter-with-reset start)
  (let ((count start))
    (list
      (lambda () (set! count (+ count 1)) count)
      (lambda () (set! count (- count 1)) count)
      (lambda () (set! count start) 'reset)
      (lambda () count))))

(define counter (make-counter-with-reset 10))
(define inc! (car counter))
(define dec! (cadr counter))
(define reset! (caddr counter))
(define get-count (cadddr counter))

(assert-equal 11 (inc!) "closure: inc to 11")
(assert-equal 12 (inc!) "closure: inc to 12")
(assert-equal 11 (dec!) "closure: dec to 11")
(assert-equal 'reset (reset!) "closure: reset")
(assert-equal 10 (get-count) "closure: after reset")

;; Multiple closures sharing same captured state
(define (make-bank-account initial)
  (let ((balance initial))
    (list
      (lambda (amount) (set! balance (+ balance amount)) balance)
      (lambda (amount) (set! balance (- balance amount)) balance)
      (lambda () balance))))

(define account (make-bank-account 100))
(define deposit (car account))
(define withdraw (cadr account))
(define balance (caddr account))

(assert-equal 150 (deposit 50) "shared closure: deposit 50")
(assert-equal 120 (withdraw 30) "shared closure: withdraw 30")
(assert-equal 120 (balance) "shared closure: balance")

(end-suite)

;; ============================================================
;; Lambda Edge Cases
;; ============================================================
(start-suite "lambda-edge-cases")

;; Lambda returning lambda (currying)
(define curried-add
  (lambda (a)
    (lambda (b)
      (+ a b))))

(define add5 (curried-add 5))
(assert-equal 8 (add5 3) "curried: add5(3) = 8")
(assert-equal 15 (add5 10) "curried: add5(10) = 15")

;; Compose functions
(define (compose f g)
  (lambda (x) (f (g x))))

(define double (lambda (x) (* x 2)))
(define inc (lambda (x) (+ x 1)))
(define double-inc (compose double inc))
(define inc-double (compose inc double))

(assert-equal 10 (double-inc 4) "compose: (4+1)*2 = 10")
(assert-equal 9 (inc-double 4) "compose: (4*2)+1 = 9")

;; Partial application
(define (partial fn . args)
  (lambda rest
    (apply fn (append args rest))))

(define add100 (partial + 100))
(assert-equal 150 (add100 50) "partial: add100(50) = 150")

;; Identity function
(define identity (lambda (x) x))
(assert-equal 42 (identity 42) "identity: returns argument")

;; Constant function
(define always-5 (lambda (x) 5))
(assert-equal 5 (always-5 100) "constant: always 5")
(assert-equal 5 (always-5 'anything) "constant: ignores argument")

(end-suite)

;; ============================================================
;; Higher-Order Function Edge Cases
;; ============================================================
(start-suite "higher-order-edge")

;; fold (left fold)
(assert-equal 0 (fold + 0 '()) "fold: empty = init")
(assert-equal 6 (fold + 0 '(1 2 3)) "fold: sum")
(assert-equal 24 (fold * 1 '(1 2 3 4)) "fold: product")
(assert-equal '(4 3 2 1) (fold (lambda (x acc) (cons x acc)) '() '(1 2 3 4))
  "fold: reverse")

;; fold-right (right fold)
(assert-equal '(1 2 3 4) (fold-right (lambda (x acc) (cons x acc)) '() '(1 2 3 4))
  "fold-right: identity")
(assert-equal 10 (fold-right + 0 '(1 2 3 4)) "fold-right: sum")

;; mapcar
(assert-equal '() (mapcar (lambda (x) x) '()) "mapcar: empty")
(assert-equal '(2 4 6) (mapcar (lambda (x) (* x 2)) '(1 2 3)) "mapcar: double")
(assert-equal '(1 4 9) (mapcar square '(1 2 3)) "mapcar: squares")

;; filter
(assert-equal '() (filter (lambda (x) #t) '()) "filter: empty")
(assert-equal '(1 2 3) (filter (lambda (x) #t) '(1 2 3)) "filter: keep all")
(assert-equal '() (filter (lambda (x) #f) '(1 2 3)) "filter: remove all")
(assert-equal '(3 4 5) (filter (lambda (x) (> x 2)) '(1 2 3 4 5)) "filter: > 2")

;; apply
(assert-equal 6 (apply + '(1 2 3)) "apply: + to list")
(assert-equal 10 (apply + 1 2 '(3 4)) "apply: with extra args")

;; funcall
(assert-equal 6 (funcall + 1 2 3) "funcall: basic")

(end-suite)

;; ============================================================
;; Lexical Scoping Edge Cases
;; ============================================================
(start-suite "lexical-scoping-edge")

;; Shadowing
(define x 100)
(define (outer x)
  (define inner (lambda () x))
  (inner))

(assert-equal 5 (outer 5) "shadowing: outer param shadows global")
(assert-equal 200 (outer 200) "shadowing: different outer value")

;; Nested closures
(define (make-adder-factory base)
  (lambda (addend)
    (+ base addend)))

(define add-to-10 (make-adder-factory 10))
(define add-to-100 (make-adder-factory 100))

(assert-equal 15 (add-to-10 5) "nested closure: add to 10")
(assert-equal 105 (add-to-100 5) "nested closure: add to 100")

;; Closure escaping
(define (make-list-appender lst)
  (lambda (x)
    (append lst (list x))))

(define appender (make-list-appender '(1 2 3)))
(assert-equal '(1 2 3 4) (appender 4) "escaping closure: appends correctly")

(end-suite)

;; ============================================================
;; Variadic/Rest Args Edge Cases
;; ============================================================
(start-suite "variadic-edge")

;; Rest args
(define (variadic-demo . args)
  (length args))

(assert-equal 0 (variadic-demo) "variadic: no args")
(assert-equal 1 (variadic-demo 1) "variadic: one arg")
(assert-equal 5 (variadic-demo 1 2 3 4 5) "variadic: five args")

;; Rest args with named params
(define (variadic-with-fixed a b . rest)
  (list a b rest))

(assert-equal '(1 2 ()) (variadic-with-fixed 1 2) "fixed+rest: no rest")
(assert-equal '(1 2 (3 4 5)) (variadic-with-fixed 1 2 3 4 5)
  "fixed+rest: with rest")

;; Using apply with variadic
(assert-equal 15 (apply + (list 1 2 3 4 5)) "apply variadic")

(end-suite)

;; ============================================================
;; Summary
;; ============================================================
(test-summary)
