;; ============================================================
;; Advanced Test: CLOS (Common Lisp Object System)
;;
;; Tests:
;;   1. Basic class with slots and make-instance
;;   2. Slot access with slot-value and slot-set!
;;   3. Single method dispatch with specialized parameters
;;   4. Before, after, and primary methods
;;   5. call-next-method chaining
;;   6. Diamond inheritance with C3 linearization
;;   7. class-of and instance? predicates
;; ============================================================

(load "tests/framework.lisp")

(start-suite "Basic Class and Instance")

(defclass point () (x y))

(define p (make-instance 'point))
(assert-true (instance? p) "make-instance: creates instance")
(assert-equal 'point (class-name (class-of p)) "class-of: point")

;; Slot access - initially nil
(assert-nil (slot-value p 'x) "slot-value: x initially nil")
(assert-nil (slot-value p 'y) "slot-value: y initially nil")

;; Slot setting
(assert-equal 10 (slot-set! p 'x 10) "slot-set!: set x to 10")
(assert-equal 20 (slot-set! p 'y 20) "slot-set!: set y to 20")
(assert-equal 10 (slot-value p 'x) "slot-value: x = 10 after set")
(assert-equal 20 (slot-value p 'y) "slot-value: y = 20 after set")

(end-suite)

(start-suite "Single Method Dispatch")

(defclass animal () (name))

(define (make-animal name)
  (define a (make-instance 'animal))
  (slot-set! a 'name name)
  a)

;; Default unspecialized method
(defmethod speak (a)
  "Generic animal sound")

(define generic-animal (make-animal "Generic"))
(assert-equal "Generic animal sound" (speak generic-animal) "speak: default method")

(defclass dog () ())
;; Specialized method for dog - param syntax: ((d dog))
(defmethod speak ((d dog))
  "Woof!")

(define my-dog (make-instance 'dog))
(assert-equal "Woof!" (speak my-dog) "speak: dog says Woof!")

(defclass cat () ())
(defmethod speak ((c cat))
  "Meow!")

(define my-cat (make-instance 'cat))
(assert-equal "Meow!" (speak my-cat) "speak: cat says Meow!")

(end-suite)

(start-suite "Before and After Methods")

(defclass person () (name))

(define (make-person name)
  (define p (make-instance 'person))
  (slot-set! p 'name name)
  p)

(define greet-log '())

;; Primary method for person
(defmethod greet ((p person))
  (set! greet-log (cons 'hello greet-log))
  "Hello!")

(defmethod greet :before ((p person))
  (set! greet-log (cons 'before greet-log)))

(defmethod greet :after ((p person))
  (set! greet-log (cons 'after greet-log)))

(set! greet-log '())
(define result (greet (make-person "Alice")))
(assert-equal "Hello!" result "greet: primary method result")
(assert-true (member? 'before greet-log) "greet: before method ran")
(assert-true (member? 'after greet-log) "greet: after method ran")

(end-suite)

(start-suite "call-next-method")

(defclass base () ())

(defmethod compute ((b base))
  "base")

(defclass derived (base) ())
(defmethod compute ((d derived))
  (string-append (call-next-method) "-derived"))

(define base-obj (make-instance 'base))
(assert-equal "base" (compute base-obj) "compute: base method")

;; Test that derived calls base via call-next-method
(define derived-obj (make-instance 'derived))
(assert-equal "base-derived" (compute derived-obj) "compute: derived calls base")

(end-suite)

(start-suite "Diamond Inheritance")

(defclass top () ())
(defmethod priority ((t top)) "top")

(defclass left (top) ())
(defmethod priority ((l left)) "left")

(defclass right (top) ())
(defmethod priority ((r right)) "right")

(defclass bottom (left right) ())
(defmethod priority ((b bottom))
  (string-append "bottom-" (call-next-method)))

(define bobj (make-instance 'bottom))
(assert-equal "bottom-left" (priority bobj) "priority: bottom calls left (most specific)")

;; Verify instance creation works
(assert-true (instance? (make-instance 'top)) "diamond: top instance")
(assert-true (instance? (make-instance 'left)) "diamond: left instance")
(assert-true (instance? (make-instance 'right)) "diamond: right instance")

(end-suite)

(start-suite "Multiple Slots and State Isolation")

(defclass counter () (count))

(define (make-counter)
  (define c (make-instance 'counter))
  (slot-set! c 'count 0)
  c)

(defmethod inc ((c counter))
  (slot-set! c 'count (+ (slot-value c 'count) 1))
  (slot-value c 'count))

(define c1 (make-counter))
(define c2 (make-counter))

(assert-equal 1 (inc c1) "counter1: inc=1")
(assert-equal 1 (inc c2) "counter2: inc=1 (isolated)")
(assert-equal 2 (inc c1) "counter1: inc=2")
(assert-equal 2 (inc c2) "counter2: inc=2 (isolated)")
(assert-equal 3 (inc c1) "counter1: inc=3")

(end-suite)

(start-suite "class-of and instance? Edge Cases")

;; class-of on non-instances returns type symbol
(assert-equal 'integer (class-of 42) "class-of: number")
(assert-equal 'string (class-of "hello") "class-of: string")
(assert-equal 'symbol (class-of 'x) "class-of: symbol")
(assert-equal 'pair (class-of '(1 2)) "class-of: pair")
(assert-equal 'boolean (class-of #t) "class-of: boolean")

;; instance? on non-instances
(assert-false (instance? 42) "instance?: number")
(assert-false (instance? 'x) "instance?: symbol")
(assert-false (instance? '(1 2)) "instance?: pair")
(assert-false (instance? "hello") "instance?: string")

(end-suite)

(test-summary)
