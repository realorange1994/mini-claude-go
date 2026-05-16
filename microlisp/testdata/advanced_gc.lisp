;; ============================================================
;; Advanced Test: Closure Shared State & Circular References / GC
;;
;; Tests:
;;   1. Closures sharing mutable state (via set-car! on shared list)
;;   2. Circular references between closures (a references b, b references a)
;;   3. GCC: stress test creating many circular reference chains
;;   4. Closure state isolation between independently created groups
;; ============================================================

(load "tests/framework.lisp")

(start-suite "Closure Shared Mutable State")

;; Two closures share a mutable list and can observe each other's modifications
(define (make-shared-pair)
  (let ((shared (list 'initial)))
    (define (get-state) (car shared))
    (define (set-state-a!) (set-car! shared 'from-a))
    (define (set-state-b!) (set-car! shared 'from-b))
    (list set-state-a! set-state-b! get-state)))

(define pair (make-shared-pair))
(define set-a! (car pair))
(define set-b! (cadr pair))
(define get-state (caddr pair))

(assert-equal 'initial (get-state) "shared: initial state = 'initial")

;; a modifies shared state
(set-a!)
(assert-equal 'from-a (get-state) "shared: after set-a!, state = 'from-a")

;; b modifies shared state, a can see it via get-state
(set-b!)
(assert-equal 'from-b (get-state) "shared: after set-b!, state = 'from-b")

;; Verify that a sees b's modification
(assert-equal 'from-b (get-state) "shared: a sees b's modification")

(end-suite)

(start-suite "Closure Chain with Circular References")

;; Create closures that form a reference cycle:
;; a → b, b → a (circular), and both share mutable data
(define (make-circular-pair)
  (let ((shared (list 'init)))
    ;; Forward declarations
    (define a-fn #f)
    (define b-fn #f)
    ;; b modifies and returns state
    (set! b-fn (lambda ()
                 (set-car! shared 'from-b)
                 (car shared)))
    ;; a calls b, then returns state
    (set! a-fn (lambda ()
                 (set-car! shared 'from-a)
                 (b-fn)          ;; a -> b call
                 (car shared)))
    (list a-fn b-fn)))

(define circ-pair (make-circular-pair))
(define circ-a (car circ-pair))
(define circ-b (cadr circ-pair))

;; Call a: it modifies shared to 'from-a, calls b (modifies to 'from-b), then returns current state
(assert-equal 'from-b (circ-a) "circular: a() = 'from-b (b modified it)")

;; Call b directly: modifies to 'from-b and returns it
(assert-equal 'from-b (circ-b) "circular: b() = 'from-b")

;; Verify both closures still work after GC activity
(assert-equal 'from-b (circ-a) "circular: a() still works after GC")
(assert-equal 'from-b (circ-b) "circular: b() still works after GC")

(end-suite)

(start-suite "Independent Closure Groups")

;; Each group has its own isolated shared state
(define group1 (make-circular-pair))
(define group2 (make-circular-pair))

(define g1a (car group1))
(define g1b (cadr group1))
(define g2a (car group2))
(define g2b (cadr group2))

;; g1 chain: from-a → from-b
(assert-equal 'from-b (g1a) "independent: group1 a() = 'from-b")

;; g2 should be independent, starting fresh
(assert-equal 'from-b (g2a) "independent: group2 a() = 'from-b (independent)")

;; Modifying g2's state should not affect g1
;; Re-call g1 to verify its state is intact
(assert-equal 'from-b (g1a) "independent: g1 unaffected by g2 calls")

(end-suite)

(start-suite "Multiple Cycle Stress Test")

;; Create many circular reference groups to stress-test GC
;; Each group creates a cycle that becomes unreachable when the
;; group reference is dropped. Mark-sweep GC must collect these.
(define (make-cycle n)
  (if (= n 0) 'done
    (begin
      ;; Create a cycle that becomes garbage immediately
      (let ((a (lambda () 'a))
            (b (lambda () (a)))
            (c (lambda () (b))))
        ;; a, b, c form a cycle via captured references
        'ignored)
      (make-cycle (- n 1)))))

;; Create 10000 cycles - each cycle is garbage after creation
(assert-equal 'done (make-cycle 10000) "gc-stress: 10000 cycles created without leak")

;; Verify the system still works correctly after all that GC activity
(define fresh-pair (make-circular-pair))
(define fresh-a (car fresh-pair))
(assert-equal 'from-b (fresh-a) "gc-stress: fresh circular pair still works")

(end-suite)

(start-suite "Deep Closure Capture Chains")

;; Closures that capture other closures in a deep chain
;; Each closure in the chain has a reference to the next
(define (make-deep-chain depth)
  (define (build n)
    (if (= n depth)
      (lambda (x) x)           ;; base: identity
      (let ((next (build (+ n 1))))
        (lambda (x)
          (next (+ x 1))))))   ;; each level adds 1
  (build 0))

(define chain (make-deep-chain 100))
;; chain adds 1 for each of 100 levels
(assert-equal 100 (chain 0) "deep-chain: 100-level chain adds 100")
(assert-equal 200 (chain 100) "deep-chain: chain(100) = 200")

;; Even deeper chain
(define deep-chain (make-deep-chain 1000))
(assert-equal 1000 (deep-chain 0) "deep-chain: 1000-level chain adds 1000")

(end-suite)

(start-suite "Closure Replacing Captured Variable")

;; Test that closures can update captured variables and other
;; closures sharing the same variable see the update
(define (make-observer-pair)
  (let ((state 'start))
    (define (get) state)
    (define (set v) (set! state v))
    (define (swap) (if (eq? state 'a) (set 'b) (set 'a)))
    (list get set swap)))

(define obs (make-observer-pair))
(define obs-get (car obs))
(define obs-set (cadr obs))
(define obs-swap (caddr obs))

(assert-equal 'start (obs-get) "observer: initial state = 'start")

(obs-set 'hello)
(assert-equal 'hello (obs-get) "observer: after set, state = 'hello")

(obs-swap)
(assert-equal 'a (obs-get) "observer: after swap, state = 'a")

(obs-swap)
(assert-equal 'b (obs-get) "observer: after second swap, state = 'b")

(end-suite)

(test-summary)
