;; ============================================================
;; Advanced Test: Mutual Recursion TCO & Dynamic Variables
;;
;; Tests:
;;   1. Mutual tail recursion: even?/odd? with cond branches
;;   2. Large depth (100000) verification of TCO
;;   3. Dynamic variable *depth* tracking across recursive calls
;;   4. labels-style local mutual recursion
;; ============================================================

(load "tests/framework.lisp")

(start-suite "Basic even?/odd? Mutual Recursion")

(define *depth* 0)

(define (even? n)
  (cond
    ((= n 0) #t)
    ((= n 1) #f)
    (else
      (set! *depth* (+ *depth* 1))
      (odd? (- n 1)))))

(define (odd? n)
  (cond
    ((= n 0) #f)
    ((= n 1) #t)
    (else
      (set! *depth* (+ *depth* 1))
      (even? (- n 1)))))

;; Basic correctness
(assert-true (even? 0) "even? 0 = #t")
(assert-false (even? 1) "even? 1 = #f")
(assert-true (even? 2) "even? 2 = #t")
(assert-false (even? 3) "even? 3 = #f")
(assert-true (even? 100) "even? 100 = #t")
(assert-false (even? 101) "even? 101 = #f")

(assert-false (odd? 0) "odd? 0 = #f")
(assert-true (odd? 1) "odd? 1 = #t")
(assert-false (odd? 2) "odd? 2 = #f")
(assert-true (odd? 101) "odd? 101 = #t")
(assert-false (odd? 100) "odd? 100 = #f")

(end-suite)

(start-suite "Depth Tracking")

;; Reset and test depth accumulation
(set! *depth* 0)
(assert-true (even? 100) "even? 100 = #t")
(assert-equal 99 *depth* "even? 100: *depth* = 99 (n-1 recursive steps)")

;; Depth continues accumulating on next call
(assert-true (odd? 99) "odd? 99 = #t")
(assert-equal 197 *depth* "odd? 99: *depth* = 197 (cumulative: 99 + 98)")

(end-suite)

(start-suite "Large Depth TCO")

;; Reset depth
(set! *depth* 0)

;; Test large depth (TCO)
(assert-true (even? 100000) "even? 100000 = #t (TCO)")
(assert-equal 99999 *depth* "even? 100000: *depth* = 99999")

;; Verify stack independence after large recursion
(assert-true (even? 0) "even? 0 after 100000 = #t")

(end-suite)

(start-suite "Local Mutual Recursion (via define inside define)")

;; Test that locally-defined mutually recursive functions also work
(define (make-even-odd)
  (define (local-even n)
    (if (= n 0) #t
      (local-odd (- n 1))))
  (define (local-odd n)
    (if (= n 0) #f
      (local-even (- n 1))))
  (list local-even local-odd))

(define pair (make-even-odd))
(define local-even (car pair))
(define local-odd (cadr pair))

(assert-true (local-even 100) "local: even? 100 = #t")
(assert-false (local-odd 100) "local: odd? 100 = #f")
(assert-false (local-even 101) "local: even? 101 = #f")
(assert-true (local-odd 101) "local: odd? 101 = #t")

;; Local functions with large depth (TCO)
(assert-true (local-even 100000) "local: even? 100000 = #t (TCO)")

(end-suite)

(start-suite "Single-Function Tail Recursion Deep Dive")

;; Classic tail-recursive sum with accumulator
(define (tail-sum n acc)
  (if (= n 0) acc
    (tail-sum (- n 1) (+ n acc))))

(assert-equal 50005000 (tail-sum 10000 0) "tail-sum: 1..10000 = 50005000")
(assert-equal 5000050000 (tail-sum 100000 0) "tail-sum: 1..100000 = 5000050000 (TCO)")

;; Tail-recursive list construction
(define (make-list n)
  (define (loop i acc)
    (if (< i 0) acc
      (loop (- i 1) (cons i acc))))
  (loop n '()))

(define big-list (make-list 10000))
(assert-equal 10001 (length big-list) "make-list: length 10001")
(assert-equal 0 (car big-list) "make-list: first = 0")
(assert-equal 10000 (list-ref big-list 10000) "make-list: last = 10000")

;; Large list with TCO
(define big-list2 (make-list 100000))
(assert-equal 100001 (length big-list2) "make-list: length 100001 (TCO)")

(end-suite)

(start-suite "Stack Depth Independence")

;; Verify that recursion depth doesn't change results
(define (deep-even? n)
  (if (= n 0) #t
    (deep-odd? (- n 1))))

(define (deep-odd? n)
  (if (= n 0) #f
    (deep-even? (- n 1))))

;; Test increasing depths to confirm linear stack behavior
(assert-true (deep-even? 10000) "deep: even? 10000")
(assert-true (deep-even? 50000) "deep: even? 50000")
(assert-true (deep-even? 100000) "deep: even? 100000")
(assert-false (deep-even? 100001) "deep: even? 100001 = #f")

(end-suite)

(test-summary)
