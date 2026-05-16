;; ============================================================
;; Closures & Higher-Order Functions Tests
;; ============================================================

(load "tests/framework.lisp")
(start-suite "Basic Closures")

;; Simple closure - captured variable
(define (make-adder n)
  (lambda (x) (+ x n)))

(define add5 (make-adder 5))
(define add10 (make-adder 10))

(assert-equal 15 (add5 10) "add5: 10 + 5 = 15")
(assert-equal 20 (add10 10) "add10: 10 + 10 = 20")
(assert-equal 100 (add5 95) "add5: 95 + 5 = 100")

;; Multiple independent closures
(define add1 (make-adder 1))
(define add2 (make-adder 2))
(assert-equal 6 (add1 5) "add1: 5+1=6")
(assert-equal 7 (add2 5) "add2: 5+2=7")

;; Closure that encapsulates state (with set!)
(define (make-counter)
  (let ((count 0))
    (lambda ()
      (set! count (+ count 1))
      count)))

(define counter1 (make-counter))
(define counter2 (make-counter))

(assert-equal 1 (counter1) "counter1: first call = 1")
(assert-equal 2 (counter1) "counter1: second call = 2")
(assert-equal 3 (counter1) "counter1: third call = 3")
(assert-equal 1 (counter2) "counter2: first call = 1 (independent)")
(assert-equal 2 (counter2) "counter2: second call = 2")
(assert-equal 4 (counter1) "counter1: fourth call = 4 (still counting)")

;; Closure with multiple captured variables
(define (make-point x y)
  (lambda (op)
    (cond
      ((= op 0) x)
      ((= op 1) y)
      (else (list x y)))))

(define pt (make-point 3 4))
(assert-equal 3 (pt 0) "point: get x = 3")
(assert-equal 4 (pt 1) "point: get y = 4")
(assert-equal '(3 4) (pt 2) "point: get both")

;; Closure capturing closure
(define (make-multiplier factor)
  (lambda (x)
    (* x factor)))

(define (make-processor base)
  (lambda (x)
    ((make-multiplier base) x)))

(define times5 (make-processor 5))
(assert-equal 25 (times5 5) "processor: 5*5=25")
(assert-equal 50 (times5 10) "processor: 5*10=50")

(end-suite)

(start-suite "Higher-Order Functions")

;; mapcar (CL standard - takes function and sequences)
(assert-equal '() (mapcar (lambda (x) x) '()) "mapcar: empty list")
(assert-equal '(2 4 6) (mapcar (lambda (x) (* x 2)) '(1 2 3)) "mapcar: double")
(assert-equal '(1 4 9) (mapcar square '(1 2 3)) "mapcar: squares")
(assert-equal '(#f #f #t) (mapcar number? '(a b 3)) "mapcar: type predicate")
(assert-equal '(#t #f #t) (mapcar (lambda (x) (> x 2)) '(3 1 4)) "mapcar: comparison")

;; filter
(assert-equal '() (filter (lambda (x) #t) '()) "filter: empty list")
(assert-equal '(1 2 3) (filter (lambda (x) #t) '(1 2 3)) "filter: keep all")
(assert-equal '() (filter (lambda (x) #f) '(1 2 3)) "filter: remove all")
(assert-equal '(3 4 5) (filter (lambda (x) (> x 2)) '(1 2 3 4 5)) "filter: > 2")
(assert-equal '(1 3 5) (filter (lambda (x) (= (modulo x 2) 1)) '(1 2 3 4 5)) "filter: odds")

;; fold (left fold)
(assert-equal 0 (fold + 0 '()) "fold: empty = init")
(assert-equal 6 (fold + 0 '(1 2 3)) "fold: sum = 6")
(assert-equal 24 (fold * 1 '(1 2 3 4)) "fold: product = 24")
(assert-equal 10 (fold (lambda (x acc) (+ x acc)) 0 '(1 2 3 4)) "fold: sum lambda")
(assert-equal '(4 3 2 1) (fold (lambda (x acc) (cons x acc)) '() '(1 2 3 4))
  "fold: reverse using cons")

;; fold-right
(assert-equal 6 (fold-right + 0 '(1 2 3)) "fold-right: sum = 6")
(assert-equal '(1 2 3 4) (fold-right (lambda (x acc) (cons x acc)) '() '(1 2 3 4))
  "fold-right: identity for lists")

;; for-each
(define fe-acc '())
(define (fe-collect x)
  (set! fe-acc (cons x fe-acc)))
(for-each fe-collect '(1 2 3 4))
(set! fe-acc (reverse fe-acc))
(assert-equal '(1 2 3 4) fe-acc "for-each: collects side effects")

(end-suite)

(start-suite "Function Composition")

;; compose two functions
(define (compose f g)
  (lambda (x) (f (g x))))

(define inc (lambda (x) (+ x 1)))
(define double (lambda (x) (* x 2)))
(define inc-then-double (compose double inc))
(define double-then-inc (compose inc double))

(assert-equal 10 (inc-then-double 4) "compose: (4+1)*2 = 10")
(assert-equal 9 (double-then-inc 4) "compose: (4*2)+1 = 9")

;; curry a two-argument function
(define (curry f)
  (lambda (a)
    (lambda (b)
      (f a b))))

(define curried-add (curry +))
(define add7 (curried-add 7))
(assert-equal 12 (add7 5) "curry: add7(5) = 12")
(assert-equal 20 (add7 13) "curry: add7(13) = 20")

(define curried-mul (curry *))
(define times3 (curried-mul 3))
(assert-equal 15 (times3 5) "curry: times3(5)=15")
(assert-equal 30 (times3 10) "curry: times3(10)=30")

;; partial application
(define (partial f . args)
  (lambda rest
    (apply f (append args rest))))

(define add-10 (partial + 10))
(assert-equal 15 (add-10 5) "partial: add-10(5) = 15")
(assert-equal 25 (add-10 15) "partial: add-10(15) = 25")

(end-suite)

(start-suite "Lambda Variadic / Rest Args")

;; Rest args with single capture
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

;; map with variadic
(define (my-map fn . lists)
  (if (null? (car lists)) '()
    (cons (apply fn (mapcar car lists))
          (apply my-map fn (mapcar cdr lists)))))

(assert-equal '((1 10) (2 20) (3 30))
  (my-map list '(1 2 3) '(10 20 30))
  "my-map: two lists")

(end-suite)

(start-suite "Lexical Scoping Rules")

;; Lexical (not dynamic) scoping
(define x 100)
(define (test-scope)
  (define x 200)
  (lambda () x))

(define get-x (test-scope))
(assert-equal 200 (get-x) "lexical scope: captured x=200, not global 100")

;; Shadowing in nested lambdas
(define (outer x)
  (define (inner x)
    (+ x 1))
  (inner (* x 10)))

(assert-equal 21 (outer 2) "shadowing: outer x=2, inner gets 20, returns 21")

;; Deep lexical nesting
(define (level1 x)
  (define (level2 y)
    (define (level3 z)
      (+ x y z))
    level3)
  level2)

(assert-equal 6 (((level1 1) 2) 3) "deep nesting: 1+2+3=6")
(assert-equal 30 (((level1 10) 10) 10) "deep nesting: 10+10+10=30")

;; Returning multiple closures sharing same captured state
(define (make-pair-counter)
  (let ((count 0))
    (list
      (lambda () (set! count (+ count 1)) count)
      (lambda () (set! count (- count 1)) count)
      (lambda () count))))

(define pair-count (make-pair-counter))
(define inc-pair (car pair-count))
(define dec-pair (cadr pair-count))
(define get-pair (caddr pair-count))

(assert-equal 1 (inc-pair) "pair-counter: inc to 1")
(assert-equal 2 (inc-pair) "pair-counter: inc to 2")
(assert-equal 1 (dec-pair) "pair-counter: dec to 1")
(assert-equal 1 (get-pair) "pair-counter: get = 1")

(end-suite)

(test-summary)
