;; ============================================================
;; Advanced Test: Nested Macro Hygiene & Variable Capture
;;
;; Tests:
;;   1. with-double macro - uses gensym to avoid variable capture,
;;      doubles x in the lexical context
;;   2. capture-guard macro - generates functions using with-double
;;   3. Nested macro composition - macros calling macros
;;   4. Hygiene verification - no gensym leakage to outer scope
;; ============================================================

(load "tests/framework.lisp")

(start-suite "Basic Macro Hygiene")

;; with-double: rebinds x to (* x 2) in the expression scope
;; Uses gensym for the intermediate binding to prevent capture
(define-macro (with-double expr)
  (let ((new-x (gensym)))
    `(let ((,new-x (* x 2)))
       (let ((x ,new-x))
         ,expr))))

;; capture-guard: generates a function that uses with-double internally
(define-macro (capture-guard fn-name)
  `(define (,fn-name x)
     (with-double (list x x))))

;; Generate test function via macro
(capture-guard test-func)

;; (test-func 10): x=10, doubled to 20, then (list x x) = (20 20)
(assert-equal '(20 20) (test-func 10) "with-double: (test-func 10) = (20 20)")
(assert-equal '(0 0) (test-func 0) "with-double: (test-func 0) = (0 0)")
(assert-equal '(100 100) (test-func 50) "with-double: (test-func 50) = (100 100)")

;; Verify no macro-introduced symbols leak to outer scope
(assert-false (defined? 'new-x) "hygiene: gensym symbol 'new-x does not leak")
(assert-false (defined? 'g) "hygiene: generic 'g does not leak")

(end-suite)

(start-suite "Nested Macro Composition")

;; A second macro that triples x, for testing nesting
(define-macro (with-triple expr)
  (let ((new-x (gensym)))
    `(let ((,new-x (* x 3)))
       (let ((x ,new-x))
         ,expr))))

;; Compose with-double then with-triple
;; Each macro gensyms its own temporary binding
(define-macro (double-then-triple fn-name)
  `(define (,fn-name x)
     (with-double (with-triple (list x x)))))

(double-then-triple test-func2)

;; (test-func2 5):
;;   x=5, with-double: x=5*2=10, with-triple: x=10*3=30, (list x x) = (30 30)
(assert-equal '(30 30) (test-func2 5) "nested: double then triple 5 = (30 30)")
(assert-equal '(60 60) (test-func2 10) "nested: double then triple 10 = (60 60)")

;; Reverse order: triple then double
(define-macro (triple-then-double fn-name)
  `(define (,fn-name x)
     (with-triple (with-double (list x x)))))

(triple-then-double test-func3)

;; (test-func3 5):
;;   x=5, with-triple: x=5*3=15, with-double: x=15*2=30, (list x x) = (30 30)
(assert-equal '(30 30) (test-func3 5) "nested: triple then double 5 = (30 30)")

(end-suite)

(start-suite "Multiple gensym Calls Are Unique")

;; Each macro invocation must generate fresh gensyms
(define-macro (capture-twice fn-name)
  `(define (,fn-name x)
     (list (with-double x) (with-double x))))

(capture-twice test-func4)

;; Both with-double calls should independently double x
;; The gensyms from the two macro-expansions must NOT collide
(assert-equal '(20 20) (test-func4 10) "macro-multi: two independent with-double calls")

(end-suite)

(start-suite "Non-Interference with Outer Bindings")

;; Verify that outer x is unchanged after calls
(define x 100)
(assert-equal '(20 20) (test-func 10) "non-interference: test-func works with outer x=100")
(assert-equal 100 x "non-interference: outer x still = 100 after macro call")

;; Multiple nested macro calls don't pollute each other
(assert-equal '(30 30) (test-func2 5) "non-interference: test-func2 after outer x=100")
(assert-equal 100 x "non-interference: outer x still = 100 after all calls")

(end-suite)

(start-suite "Macro Inside Macro (expansion-time)")

;; Macro that generates a macro - tests expansion-time hygiene
;; Uses explicit list construction instead of nested quasiquote
;; to avoid nested backquote complexity.
;; NOTE: expr-inner in the generated macro body is the macro's own parameter,
;; so we keep it literal (not unquoted).
(define-macro (def-hygienic-doubler name)
  (let ((g (gensym)))
    `(define-macro (,name expr-inner)
       (let ((,g (gensym)))
         (list 'let
               (list (list ,g (list '* 'x 2)))
               (list 'let (list (list 'x ,g))
                     expr-inner))))))

;; This generates a macro that internally uses gensym
;; The generated macro constructs its expansion with list
(def-hygienic-doubler my-double)

;; my-double is now a macro
(define (test-my-double x)
  (my-double (list x x)))

(assert-equal '(20 20) (test-my-double 10) "macro-gen: my-double 10 = (20 20)")
(assert-equal '(100 100) (test-my-double 50) "macro-gen: my-double 50 = (100 100)")

(end-suite)

(test-summary)
