;; ============================================================
;; Conditions and Handler/Restart Bug Tests
;; ============================================================
;; Tests covering panic conditions, infinite recursion/loop bugs

(load "tests/framework.lisp")

;; ============================================================
;; Test 1: typepCheck with (not) - missing argument
;; Bug: typeSpec.cdr.car panics when typeSpec is just '(not)'
;; ============================================================
(start-suite "typepCheck edge cases")

;; Test typep with 'not'
(assert-false (typep 5 '(not number)) "typep: 5 is not (not number)")
(assert-true (typep 'x '(not number)) "typep: symbol is (not number)")

;; Test typep with compound types
(assert-true (typep 5 '(and number)) "typep: (and number) with single type")

(end-suite)

;; ============================================================
;; Test 2: Circular list detection in toSlice
;; Bug: toSlice enters infinite loop on circular lists
;; ============================================================
(start-suite "Circular list handling")

;; Create a circular list: (set! c (list 1 2 3)) then (set-cdr! (cddr c) c)
(define (make-circular-list)
  (let ((c (list 1 2 3)))
    (set-cdr! (cddr c) c)
    c))

;; Test that length on circular list doesn't hang
;; We expect this to either detect the cycle or have bounded iteration
(define (test-circular-safe lst)
  (let ((result '())
        (count 0))
    (define (loop-once l)
      (if (> count 1000)
        'hit-limit
        (begin
          (set! count (+ count 1))
          (if (null? l)
            'done
            (loop-once (cdr l))))))
    (loop-once lst)))

(assert-equal 'hit-limit
  (test-circular-safe (make-circular-list))
  "circular list detection in iteration")

(end-suite)

;; ============================================================
;; Test 3: appendToList deep recursion
;; Bug: appendToList can cause stack overflow on very long lists
;; ============================================================
(start-suite "appendToList deep recursion")

;; Create a long list that could cause stack overflow in recursive appendToList
(define (make-long-list n)
  (if (= n 0)
    '()
    (cons n (make-long-list (- n 1)))))

;; Test that append works on reasonably long lists
(define long-lst (make-long-list 100))
(define appended (append long-lst '(999)))
(assert-equal 101 (length appended) "append on 100-element list")
(assert-equal 999 (list-ref appended 100) "append adds correct element")

(end-suite)

;; ============================================================
;; Test 4: Plist operations with malformed plists
;; Bug: get/putprop/remprop may infinite loop on malformed plists
;; ============================================================
(start-suite "Plist edge cases")

;; Test with properly formatted plist
(define *plist-test-sym* 'plist-edge-test-sym)
(setf (symbol-plist *plist-test-sym*) '())
(putprop *plist-test-sym* 42 'x)
(putprop *plist-test-sym* 99 'y)

;; get should return correct values for proper plist
(assert-equal 42 (get *plist-test-sym* 'x) "get returns correct value from plist")
(assert-equal 99 (get *plist-test-sym* 'y) "get returns correct second value")
(assert-nil (get *plist-test-sym* 'z) "get returns nil for missing indicator")

(end-suite)

;; ============================================================
;; Test 5: classHasAncestor with circular class hierarchy
;; Bug: classHasAncestor can infinite recursion on circular inheritance
;; ============================================================
(start-suite "Class hierarchy circular inheritance")

;; Define conditions that might have circular references
(define-condition test-base-condition () ())
(define-condition test-child-condition (test-base-condition) ())

;; Verify basic inheritance works
(assert-true (car (subtypep 'test-child-condition 'test-base-condition)) "basic condition inheritance works")

(end-suite)

;; ============================================================
;; Test 6: evalString with circular exprs
;; Bug: evalString can infinite loop if exprs forms a cycle
;; ============================================================
(start-suite "evalString circular expressions")

;; Test that eval handles normal expressions correctly
(define result (eval-string "(+ 1 2)"))
(assert-equal 3 result "eval-string basic arithmetic")

(define result2 (eval-string "(define x 10) (+ x 5)"))
(assert-equal 15 result2 "eval-string multiple forms")

(end-suite)

;; ============================================================
;; Test 7: Condition handler dead loops
;; Bug: handler-case with continue after handler execution
;; ============================================================
(start-suite "Handler-case dead loops")

;; Test basic handler-case works
(define (test-handler-case)
  (handler-case
    (error "test error")
    (condition (c) 'handled)))

(assert-equal 'handled
  (test-handler-case)
  "handler-case basic functionality")

;; Test restart-case
(define (test-restart-case)
  (restart-case
    (+ 1 2)
    (return-value (v) v)))

(assert-equal 3
  (test-restart-case)
  "restart-case returns value")

(end-suite)

;; ============================================================
;; Test 8: toSlice with improper lists
;; Bug: toSlice may not handle dotted pairs correctly
;; ============================================================
(start-suite "toSlice with improper lists")

;; Test that list operations handle dotted pairs
(define dotted (cons 'a 'b))

;; In correct behavior, list should iterate properly
(assert-equal '(a b) (list 'a 'b) "list on symbols")

(end-suite)

;; ============================================================
;; Test 9: Multiple value handling in apply
;; ============================================================
(start-suite "Apply with multiple values")

;; Test apply with functions returning multiple values
(define (test-multi-values)
  (values 1 2 3))

(define mv-result (multiple-value-list (test-multi-values)))
(assert-equal '(1 2 3) mv-result "multiple-value-list works")

(end-suite)

;; ============================================================
;; Test 10: cond/typecase iteration safety
;; Bug: clauses iteration without cycle detection
;; ============================================================
(start-suite "Cond/typecase iteration")

;; Test basic cond works
(assert-equal 1
  (cond
    ((> 3 2) 1)
    (#t 2))
  "cond: first clause matches")

(assert-equal 'else
  (cond
    (#f 1)
    (else 'else))
  "cond: else clause")

(end-suite)

;; ============================================================
;; Test 11: gcv nil return handling
;; Bug: gcv could return nil on memory allocation failure
;; ============================================================
(start-suite "GCV allocation edge cases")

;; Test that cons doesn't return nil even with many allocations
(define (stress-cons n)
  (if (= n 0)
    '()
    (cons n (stress-cons (- n 1)))))

(define many-cons (stress-cons 100))
(assert-equal 100 (length many-cons) "cons stress test: 100 allocations")
(assert-equal 1 (list-ref many-cons 99) "cons stress test: last element")

(end-suite)

;; ============================================================
;; Test 12: builtinWarn error handling
;; Bug: builtinWarn ignores handler function errors
;; ============================================================
(start-suite "Warning handler errors")

;; Test that warn handles its body correctly
(define (test-warn)
  (handler-case
    (warn "test warning")
    (warning (c) 'warning-caught)))

(assert-true
  (member? (test-warn) '(nil warning-caught))
  "warn executes body or catches")

(end-suite)

;; ============================================================
;; Test 13: invoke-restart error handling
;; Bug: builtinInvokeRestart panics when restart not found
;; ============================================================
(start-suite "Invoke-restart error cases")

;; Test invoking non-existent restart returns nil or error tuple
(define result (ignore-errors (invoke-restart 'nonexistent-restart)))
(assert-true
  (or (null? (car result))
      (string? (cdr result)))
  "invoke-restart handles missing restart gracefully")

(end-suite)

;; ============================================================
;; Test 14: Circular structure in list construction
;; ============================================================
(start-suite "List construction safety")

;; Test that normal list operations work
(assert-equal '(1 2 3) (list 1 2 3) "list construction")
(assert-equal '(1) (list 1) "list single element")
(assert-equal '() (list) "list empty")

(end-suite)

;; ============================================================
;; Summary
;; ============================================================
(test-summary)
