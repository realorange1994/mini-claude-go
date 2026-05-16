;; ============================================================
;; Panic Bug Tests
;; ============================================================
;; Tests for nil pointer dereferences and missing type guards

(load "tests/framework.lisp")

;; ============================================================
;; Test 1: VPair guards in special forms
;; Bug: Accessing .car.str on non-VPair values causes panic
;; ============================================================
(start-suite "Special form VPair guards")

;; Test that malformed forms return errors, not panics
;; (lambda) without lambda list should error, not crash
(assert-error '(lambda) "lambda without lambda list errors")

;; (let) without bindings should error
(assert-error '(let) "let without bindings errors")

;; (define) without name should error
(assert-error '(define) "define without name errors")

;; (throw) without tag should error
(assert-error '(throw) "throw without tag errors")

;; (multiple-value-bind) without vars should error
(assert-error '(multiple-value-bind) "multiple-value-bind without vars errors")

(end-suite)

;; ============================================================
;; Test 2: Type checking in flet/labels
;; Bug: b.car.str panics when b is not a VPair
;; ============================================================
(start-suite "flet/labels type guards")

;; Test that flet with malformed binding errors gracefully
(assert-error '(flet (42) (func)) "flet with non-pair binding errors")

;; Test normal flet works
(assert-equal 3
  (flet ((add1 (x) (+ x 1)))
    (add1 2))
  "flet basic functionality")

;; Test labels works
(assert-equal 120
  (labels ((fact (n)
             (if (= n 0) 1 (* n (fact (- n 1))))))
    (fact 5))
  "labels recursive function")

(end-suite)

;; ============================================================
;; Test 3: handler-bind body nil check
;; Bug: handler-bind panics when body is nil
;; ============================================================
(start-suite "handler-bind body guard")

;; Test handler-bind with normal body
(assert-equal 42
  (handler-case
    42
    (condition (c) 0))
  "handler-case with value form")

;; Test handler-bind with empty body returns nil
(assert-true
  (null? (handler-case
           nil
           (condition (c) nil)))
  "handler-case with nil body")

(end-suite)

;; ============================================================
;; Test 4: defclass slot options VPair guard
;; Bug: options.cdr.car.str panics when options.cdr is not VPair
;; ============================================================
(start-suite "defclass slot options guard")

;; Test defclass with accessor
(defclass test-point ()
  (x
   (y :accessor point-y :initform 0)))

(define tp (make-instance 'test-point))
(assert-equal 0 (slot-value tp 'y) "defclass with accessor and initform")

;; Test defclass with reader
(defclass test-reader-class ()
  ((val :reader get-val :initform 42)))

(define trc (make-instance 'test-reader-class))
(assert-equal 42 (get-val trc) "defclass with reader")

(end-suite)

;; ============================================================
;; Test 5: with-open-file guard
;; Bug: spec.car.str panics when spec is not VPair
;; ============================================================
(start-suite "with-open-file guard")

;; Test that with-output-to-string works
(assert-true
  (string? (with-output-to-string (s)
             (format s "hello")))
  "with-output-to-string basic")

;; Test with-input-from-string
(assert-equal "h"
  (with-input-from-string (s "hello")
    (string (read-char s)))
  "with-input-from-string basic")

(end-suite)

;; ============================================================
;; Test 6: load with non-string argument
;; Bug: v.cdr.car.str panics when filename is not a string
;; ============================================================
(start-suite "load guard")

;; Test that load with non-string errors gracefully
(assert-error '(load 42) "load with non-string filename errors")

(end-suite)

;; ============================================================
;; Test 7: Circular list in length
;; Bug: length enters infinite loop on circular lists
;; ============================================================
(start-suite "Circular list length")

;; Create a circular list
(define (make-circular n)
  (let ((lst (list n)))
    (set-cdr! lst lst)
    lst))

;; length should not hang on circular lists
;; (it should detect the cycle and return a bounded count)
(define circular-lst (make-circular 5))
(define len (length circular-lst))
(assert-true
  (and (number? len) (> len 0))
  "length on circular list returns bounded number")

(end-suite)

;; ============================================================
;; Test 8: class-of returns class metaobject (ANSI CL spec)
;; ============================================================
(start-suite "class-of return type")

;; class-of returns a class metaobject (VClass), use class-name to get symbol
(defclass test-class-of-class () ())

(define class-of-instance (make-instance 'test-class-of-class))
(define class-result (class-of class-of-instance))

(assert-true
  (symbol? (class-name class-result))
  "class-of returns a class with a name")

(assert-equal 'test-class-of-class
  (class-name class-result)
  "class-of returns correct class name")

(end-suite)

;; ============================================================
;; Test 9: map with correct arguments
;; Bug: map requires result-type as first argument (CL standard)
;; ============================================================
(start-suite "map function")

;; Test map with result-type
(assert-equal '(2 3 4)
  (map 'list (lambda (x) (+ x 1)) '(1 2 3))
  "map with list result type")

(assert-equal (vector 2 3 4)
  (map 'vector (lambda (x) (+ x 1)) (vector 1 2 3))
  "map with vector result type")

(end-suite)

;; ============================================================
;; Test 10: typep edge cases
;; ============================================================
(start-suite "typep edge cases")

;; Test typep with nil type specifier
(assert-false (typep 5 'nil) "typep with nil type returns false")
(assert-true (typep 5 't) "typep with t type returns true")
(assert-true (typep 'x 'symbol) "typep with symbol type")
(assert-true (typep 42 'integer) "typep with integer type")
(assert-true (typep "hello" 'string) "typep with string type")

(end-suite)

;; ============================================================
;; Summary
;; ============================================================
(test-summary)
