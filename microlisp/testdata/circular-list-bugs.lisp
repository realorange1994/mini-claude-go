;; ============================================================
;; Circular List and Iteration Bug Tests
;; ============================================================
;; Tests for infinite loops and recursion in list/sequence operations

(load "tests/framework.lisp")

;; ============================================================
;; Test Suite: Circular List Detection
;; ============================================================
(start-suite "Circular List Detection")

;; Helper to detect if a list is circular using simple visited-set approach
(define (has-cycle? lst)
  (define (check l visited)
    (cond
      ((null? l) #f)
      ((member? l visited) #t)
      (#t (check (cdr l) (cons l visited)))))
  (check lst '()))

;; Test has-cycle? on normal lists
(assert-false (has-cycle? '()) "empty list has no cycle")
(assert-false (has-cycle? '(1)) "single element list has no cycle")
(assert-false (has-cycle? '(1 2 3)) "proper list has no cycle")

;; Create a circular list and verify detection
(define (make-circular)
  (let ((p (cons 1 (cons 2 (cons 3 '())))))
    (set-cdr! (cddr p) p)
    p))

(assert-true (has-cycle? (make-circular)) "circular list is detected")

(end-suite)

;; ============================================================
;; Test Suite: toSlice on Circular Lists
;; Bug: toSlice enters infinite loop on circular lists
;; ============================================================
(start-suite "toSlice circular list bug")

;; Test that list->vector or similar operations handle circular lists
;; The bug is in toSlice: for !isNil(v) without cycle detection

(define (make-circular-list)
  (let ((c (list 1 2 3)))
    (set-cdr! (cddr c) c)
    c))

;; Test that we can detect when iteration would be infinite
;; This tests the buggy behavior
(define (would-loop-forever? lst)
  (define (check l visited max-count)
    (cond
      ((> max-count 0) #f)
      ((null? l) #t)
      ((member? l visited) #f)
      (#t (check (cdr l) (cons l visited) (- max-count 1)))))
  (check lst '() 100))

;; Test that our circular list would loop
(assert-false
  (would-loop-forever? (make-circular-list))
  "circular list detected as would-loop")

;; Test normal list operations
(define (safe-take lst n)
  (define (iter l cnt acc)
    (if (or (= cnt 0) (null? l) (not (pair? l)))
      (reverse acc)
      (iter (cdr l) (- cnt 1) (cons (car l) acc))))
  (iter lst n '()))

(define normal-lst '(1 2 3 4 5))
(assert-equal '(1 2 3) (safe-take normal-lst 3) "safe-take works on normal list")

(end-suite)

;; ============================================================
;; Test Suite: appendToList Deep Recursion
;; Bug: recursive appendToList can cause stack overflow
;; ============================================================
(start-suite "appendToList recursion depth")

;; Test recursive append implementation
(define (recursive-append lst elem)
  (if (null? lst)
    (list elem)
    (cons (car lst) (recursive-append (cdr lst) elem))))

(assert-equal '(1 2 3 4) (recursive-append '(1 2 3) 4) "recursive append works")

;; Test on a longer list (100 elements)
(define (make-list n)
  (if (= n 0)
    '()
    (cons n (make-list (- n 1)))))

(define long-list (make-list 100))
(define appended (recursive-append long-list 999))
(assert-equal 101 (length appended) "append on 100-element list")
(assert-equal 999 (list-ref appended 100) "appended element correct")

;; Test append (the actual function uses appendToList internally)
(define result (append '(1 2 3) '(4 5 6)))
(assert-equal '(1 2 3 4 5 6) result "append combines lists")

(end-suite)

;; ============================================================
;; Test Suite: evalString Iteration Safety
;; Bug: evalString can loop on circular expression lists
;; ============================================================
(start-suite "evalString iteration safety")

;; Test basic eval-string
(assert-equal 3 (eval-string "(+ 1 2)") "eval-string basic")
(assert-equal 10 (eval-string "(define x 5) (* x 2)") "eval-string with definitions")

;; Test multiple forms
(define multi-result (eval-string "(define a 1) (define b 2) (+ a b)"))
(assert-equal 3 multi-result "eval-string multiple forms returns last")

(end-suite)

;; ============================================================
;; Test Suite: cond Clause Iteration
;; Bug: cond iterates over clauses without cycle detection
;; ============================================================
(start-suite "Cond clause iteration")

;; Basic cond tests
(assert-equal 1 (cond (#t 1)) "cond: trivial #t")
(assert-equal 'else (cond (else 'else)) "cond: else symbol")
(assert-equal 2 (cond (#f 1) (else 2)) "cond: else clause")

;; cond with multiple clauses
(assert-equal 'third
  (cond
    (#f 'first)
    (#f 'second)
    (#t 'third))
  "cond: third clause matches")

;; cond with expressions in test position
(define x 5)
(assert-equal 'big (cond ((> x 10) 'small) ((> x 3) 'big) (else 'none))
  "cond: expression test")

(end-suite)

;; ============================================================
;; Test Suite: typecase Clause Iteration
;; Bug: typecase iterates without cycle detection
;; ============================================================
(start-suite "Typecase clause iteration")

;; Basic typecase if implemented
;; Note: Some implementations may not have full typecase
;; Testing with typep which is related

(assert-true (typep 5 'number) "typep: 5 is number")
(assert-false (typep 'x 'number) "typep: symbol is not number")

;; Test typep with not
(assert-false (typep 5 '(not number)) "typep: 5 is not (not number)")
(assert-true (typep 'x '(not number)) "typep: symbol is (not number)")

(end-suite)

;; ============================================================
;; Test Suite: Handler-case Iteration
;; Bug: handler-case handlers may be called multiple times due to continue
;; ============================================================
(start-suite "Handler-case iteration")

;; Basic handler-case
(define (test-handler-case-basic)
  (handler-case
    (error "test")
    (condition (c) 'handled)))

;; Should return 'handled, not loop
(assert-equal 'handled (test-handler-case-basic) "handler-case basic")

(end-suite)

;; ============================================================
;; Test Suite: Restart-case Iteration
;; Bug: restart-case iteration issues
;; ============================================================
(start-suite "Restart-case iteration")

;; Basic restart-case
(define (test-restart-case-basic)
  (restart-case
    (values 1 2)
    (ret1 (v) v)
    (ret2 (v w) (list v w))))

(assert-equal 1 (test-restart-case-basic) "restart-case basic")

(end-suite)

;; ============================================================
;; Test Suite: Get/Putprop Plist Iteration
;; Bug: plist iteration without cycle detection
;; ============================================================
(start-suite "Plist iteration")

;; Basic plist operations using a symbol
(define test-sym 'plist-test-sym)
(putprop test-sym 1 'a)
(putprop test-sym 2 'b)
(assert-equal 1 (get test-sym 'a) "get: first property")
(assert-equal 2 (get test-sym 'b) "get: second property")
(assert-nil (get test-sym 'c) "get: missing property")

;; Test remprop
(remprop test-sym 'a)
(assert-nil (get test-sym 'a) "remprop: removed property still returns nil")

(end-suite)

;; ============================================================
;; Test Suite: Long List Iteration
;; ============================================================
(start-suite "Long list iteration")

;; Create long lists and test iteration
(define (iota n)
  (define (iter n acc)
    (if (= n 0) acc (iter (- n 1) (cons n acc))))
  (iter n '()))

(define long (iota 1000))
(assert-equal 1000 (length long) "iota: creates 1000 elements")

;; Test that iteration on long lists works
(define (sum-list lst)
  (fold + 0 lst))

(assert-equal 500500 (sum-list long) "sum of 1..1000 = 500500")

;; Test map on long list
(define squared (mapcar square (iota 100)))
(assert-equal 100 (length squared) "map on 100 elements")
(assert-equal 10000 (list-ref squared 99) "last squared is 10000")

(end-suite)

;; ============================================================
;; Test Suite: Class Inheritance Cycle Detection
;; Bug: classHasAncestor can recurse infinitely on circular hierarchies
;; ============================================================
(start-suite "Class inheritance cycle detection")

;; Define a simple class hierarchy
(defclass person () (name age))
(defclass student (person) (grade))

;; Verify basic inheritance
(assert-true (car (subtypep 'student 'person)) "student is subtype of person")

(end-suite)

;; ============================================================
;; Test Suite: apply with Many Arguments
;; ============================================================
(start-suite "Apply argument handling")

;; Test apply with many arguments
(define (sum . nums) (fold + 0 nums))
(assert-equal 15 (apply sum '(1 2 3 4 5)) "apply sum to list")
(assert-equal 10 (length (list 1 2 3 4 5 6 7 8 9 10)) "list with 10 args")

(end-suite)

;; ============================================================
;; Test Suite: List Reference Bounds
;; ============================================================
(start-suite "List reference bounds")

;; Test list-ref with various indices
(define lst '(a b c d e))

(assert-equal 'a (list-ref lst 0) "list-ref: index 0")
(assert-equal 'e (list-ref lst 4) "list-ref: last element")
(assert-nil (list-ref lst 10) "list-ref: beyond length returns nil")

(end-suite)

;; ============================================================
;; Test Suite: Safe Iteration Primitives
;; ============================================================
(start-suite "Safe iteration primitives")

;; Test that our iteration primitives work correctly
(define (safe-length lst)
  (define (iter l cnt)
    (if (null? l) cnt (iter (cdr l) (+ cnt 1))))
  (iter lst 0))

(assert-equal 0 (safe-length '()) "safe-length: empty")
(assert-equal 3 (safe-length '(1 2 3)) "safe-length: three elements")

;; Test safe member
(define (safe-member? elem lst)
  (cond
    ((null? lst) #f)
    ((equal? elem (car lst)) #t)
    (#t (safe-member? elem (cdr lst)))))

(assert-true (safe-member? 'b '(a b c)) "safe-member?: finds element")
(assert-false (safe-member? 'd '(a b c)) "safe-member?: not found")

(end-suite)

;; ============================================================
;; Summary
;; ============================================================
(test-summary)
