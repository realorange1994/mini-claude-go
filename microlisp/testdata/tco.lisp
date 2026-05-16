;; ============================================================
;; Tail Recursion Optimization (TCO) Tests
;; ============================================================
;; These tests verify that tail-recursive functions don't blow
;; the stack. With TCO, each recursive call reuses the same
;; stack frame, allowing deep recursion.

(load "tests/framework.lisp")
(start-suite "Tail Recursion Optimization")

;; TCO: Countdown - tail recursive
(define (countdown n)
  (if (= n 0) 'done
    (countdown (- n 1))))

(assert-equal 'done (countdown 10000) "TCO: countdown 10000")
(assert-equal 'done (countdown 50000) "TCO: countdown 50000")

;; TCO: Sum to n (tail recursive with accumulator)
(define (sum-to n acc)
  (if (= n 0) acc
    (sum-to (- n 1) (+ n acc))))

(assert-equal 5050 (sum-to 100 0) "TCO: sum 1..100 = 5050")
(assert-equal 50005000 (sum-to 10000 0) "TCO: sum 1..10000 = 50005000")

;; TCO: List reverse (tail recursive)
(define (rev-helper lst acc)
  (if (null? lst) acc
    (rev-helper (cdr lst) (cons (car lst) acc))))

(define (rev lst) (rev-helper lst '()))

(assert-equal '(5 4 3 2 1) (rev '(1 2 3 4 5)) "TCO: reverse small list")
(define big-list '(1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20))
(assert-equal (reverse big-list) (rev big-list) "TCO: reverse matches builtin")

;; TCO: Build list (reverse order, then reverse)
(define (range-rev n acc)
  (if (= n 0) acc
    (range-rev (- n 1) (cons n acc))))

(define (range n) (range-rev n '()))

(assert-equal 1000 (length (range 1000)) "TCO: range 1000 elements")
(assert-equal 1 (car (range 1000)) "TCO: range car = 1")
(assert-equal 1000 (list-ref (range 1000) 999) "TCO: range last = 1000")

;; TCO: Fibonacci (tail recursive)
(define (fib n)
  (define (fib-iter a b count)
    (if (= count 0) a
      (fib-iter b (+ a b) (- count 1))))
  (fib-iter 0 1 n))

(assert-equal 0 (fib 0) "TCO: fib(0) = 0")
(assert-equal 1 (fib 1) "TCO: fib(1) = 1")
(assert-equal 1 (fib 2) "TCO: fib(2) = 1")
(assert-equal 2 (fib 3) "TCO: fib(3) = 2")
(assert-equal 5 (fib 5) "TCO: fib(5) = 5")
(assert-equal 55 (fib 10) "TCO: fib(10) = 55")
(assert-equal 6765 (fib 20) "TCO: fib(20) = 6765")

;; TCO: Tail recursive map
(define (trev-map f lst acc)
  (if (null? lst) (reverse acc)
    (trev-map f (cdr lst) (cons (f (car lst)) acc))))

(define (tmap f lst) (trev-map f lst '()))

(assert-equal '(2 4 6 8 10) (tmap (lambda (x) (* x 2)) '(1 2 3 4 5))
  "TCO: tail-recursive map")

;; TCO: Tail recursive filter
(define (trev-filter pred lst acc)
  (cond
    ((null? lst) (reverse acc))
    ((pred (car lst)) (trev-filter pred (cdr lst) (cons (car lst) acc)))
    (else (trev-filter pred (cdr lst) acc))))

(define (tfilter pred lst) (trev-filter pred lst '()))

(assert-equal '(2 4 6 8 10)
  (tfilter (lambda (x) (= 0 (modulo x 2))) '(1 2 3 4 5 6 7 8 9 10))
  "TCO: tail-recursive filter even numbers")

;; TCO: Mutual tail recursion (even/odd check)
(define (even? n)
  (if (= n 0) #t
    (odd? (- n 1))))

(define (odd? n)
  (if (= n 0) #f
    (even? (- n 1))))

;; Note: mutual recursion may not be TCO'd in all implementations
;; but this tests the evaluator's ability to handle it
(assert-true (even? 100) "mutual TCO: even? 100")
(assert-false (odd? 100) "mutual TCO: odd? 100")
(assert-false (even? 101) "mutual TCO: even? 101")
(assert-true (odd? 101) "mutual TCO: odd? 101")

;; TCO: Deep recursion with cond (not if)
(define (range-cond n acc)
  (cond
    ((= n 0) acc)
    (else (range-cond (- n 1) (cons n acc)))))

(assert-equal 10000 (length (range-cond 10000 '()))
  "TCO: cond-based recurstion 10000 elements")

;; TCO: Accumulator in lambda body
(define find-first
  (lambda (pred lst)
    (if (null? lst) #f
      (if (pred (car lst)) (car lst)
        (find-first pred (cdr lst))))))

(assert-equal 5 (find-first (lambda (x) (> x 4)) '(1 2 3 4 5 6))
  "TCO: find-first in list")
(assert-false (find-first (lambda (x) (> x 100)) '(1 2 3))
  "TCO: find-first not found")

;; Non-trivial TCO: ackermann (not tail recursive, but tests recursion depth)
(define (ack m n)
  (cond
    ((= m 0) (+ n 1))
    ((= n 0) (ack (- m 1) 1))
    (else (ack (- m 1) (ack m (- n 1))))))

(assert-equal 3 (ack 0 2) "ack: ack(0,2)=3")
(assert-equal 5 (ack 1 3) "ack: ack(1,3)=5")
(assert-equal 9 (ack 2 3) "ack: ack(2,3)=9")
(assert-equal 61 (ack 3 3) "ack: ack(3,3)=61")

(end-suite)

(start-suite "Large Data with TCO")

;; Build a large list using tail recursion
(define (large-list n)
  (define (loop i acc)
    (if (= i n) (reverse acc)
      (loop (+ i 1) (cons i acc))))
  (loop 0 '()))

(define ll (large-list 5000))
(assert-equal 5000 (length ll) "large-list: length 5000")
(assert-equal 0 (car ll) "large-list: first = 0")
(assert-equal 4999 (list-ref ll 4999) "large-list: last = 4999")

;; Process large list with tail-recursive map
(define ll-doubled (tmap (lambda (x) (* x 2)) ll))
(assert-equal 5000 (length ll-doubled) "large-map: length 5000")
(assert-equal 0 (car ll-doubled) "large-map: first = 0")
(assert-equal 9998 (list-ref ll-doubled 4999) "large-map: last = 9998")

(end-suite)

;; Test TCO in begin position (begin body tail position)
(start-suite "TCO in begin")

(define (begin-tail n)
  (if (= n 0) 'done
    (begin
      (display ".")
      (begin-tail (- n 1)))))

(assert-equal 'done (begin-tail 100) "TCO: begin body with recursion")

(end-suite)

(test-summary)
