;; ============================================================
;; Macro System Tests
;; ============================================================

(load "tests/framework.lisp")
(start-suite "Basic Macros")

;; Simple macro: infix
(define-macro (infix a op b)
  (list op a b))

(assert-equal 5 (infix 2 + 3) "macro: infix 2+3=5")
(assert-equal 15 (infix 3 * 5) "macro: infix 3*5=15")
(assert-equal 7 (infix 10 - 3) "macro: infix 10-3=7")

;; Macro: when (convenience wrapper around if)
(define-macro (when test . body)
  (list 'if test (cons 'begin body)))

(define when-x 0)
(when #t (set! when-x 1))
(assert-equal 1 when-x "macro: when #t executes body")

(when #f (set! when-x 2))
(assert-equal 1 when-x "macro: when #f does NOT execute body")

;; Macro: unless
(define-macro (unless test . body)
  (list 'if (list 'not test) (cons 'begin body)))

(define unless-x 0)
(unless #f (set! unless-x 1))
(assert-equal 1 unless-x "macro: unless #f executes body")

(unless #t (set! unless-x 2))
(assert-equal 1 unless-x "macro: unless #t does NOT execute body")

;; Macro: while loop
;; (a proper while macro using letrec):


(define-macro (while test . body)
  (list 'letrec (list (list 'loop (list 'lambda ()
    (list 'if test
      (cons 'begin (append body (list (list 'loop))))
      '#t))))
    '(loop)))

(define w-i 0)
(while (< w-i 5)
  (set! w-i (+ w-i 1)))
(assert-equal 5 w-i "macro: while loop runs 5 times")

(end-suite)

(start-suite "Macro Hygiene with gensym")

;; Without gensym: this macro captures 'x'
(define-macro (bad-inc var)
  (list 'set! var (list '+ var 1)))

(define bad-val 10)
(bad-inc bad-val)
(assert-equal 11 bad-val "macro: bad-inc works in simple case")

;; With gensym: hygienic macro
(define-macro (hygienic-let bindings . body)
  (let ((varnames (mapcar car bindings))
        (vals (mapcar cadr bindings)))
    `((lambda ,varnames ,@body) ,@vals)))

(assert-equal 15 (hygienic-let ((x 5) (y 10)) (+ x y))
  "macro: hygienic-let via lambda")

;; gensym produces unique symbols
(define g1 (gensym))
(define g2 (gensym))
(assert-false (eq? g1 g2) "gensym: produces unique symbols")

;; gensym with prefix
(define g3 (gensym "TEMP"))
(define g3-str (symbol->string g3))
(assert-true (> (string-length g3-str) 0) "gensym: returns symbol with prefix")

;; Macro using gensym for hygiene
(define-macro (defactor name val)
  (let ((g (gensym)))
    `(define ,name
       (let ((,g ,val))
         (lambda (x) (* x ,g))))))

(defactor times10 10)
(assert-equal 100 (times10 10) "hygienic macro: times10(10)=100")
(assert-equal 50 (times10 5) "hygienic macro: times10(5)=50")

;; The 'g' symbol used internally should not leak
(assert-false (defined? 'g) "hygienic macro: internal symbol g does not leak")

(end-suite)

(start-suite "Macro-Generated Code")

;; Macro that generates another macro
(define-macro (def-thunk name expr)
  `(define (,name) ,expr))

(def-thunk get-answer (+ 40 2))
(assert-equal 42 (get-answer) "macro: generated thunk returns 42")

;; Macro that generates function definitions
(define-macro (def-accessors struct fields)
  `(begin
     ,@(mapcar (lambda (f idx)
              `(define (,f ,struct)
                 (list-ref ,struct ,idx)))
            fields (range (length fields)))))

;; Recursive macro: or
(define-macro (my-or . args)
  (if (null? args) #f
    (if (null? (cdr args)) (car args)
      (let ((g (gensym)))
        `(let ((,g ,(car args)))
           (if ,g ,g (my-or ,@(cdr args))))))))

(assert-false (my-or) "macro: my-or with no args = #f")
(assert-equal 1 (my-or 1) "macro: my-or single arg = 1")
(assert-equal 1 (my-or #f 1) "macro: my-or returns first truthy")
(assert-equal 3 (my-or #f #f 3) "macro: my-or returns last")
(assert-false (my-or #f #f) "macro: my-or all false = #f")

;; Recursive macro: and
(define-macro (my-and . args)
  (if (null? args) #t
    (if (null? (cdr args)) (car args)
      `(if ,(car args) (my-and ,@(cdr args)) #f))))

(assert-true (my-and) "macro: my-and with no args = #t")
(assert-equal 1 (my-and 1) "macro: my-and single = 1")
(assert-false (my-and #f 1) "macro: my-and #f => #f")
(assert-equal 3 (my-and 1 2 3) "macro: my-and all truthy => last")
(assert-false (my-and 1 #f 3) "macro: my-and short-circuits")

;; Macro: cond expansion
(define-macro (my-cond . clauses)
  (if (null? clauses) #f
    (let ((cl (car clauses)))
      (if (eq? (car cl) 'else)
        `(begin ,@(cdr cl))
        `(if ,(car cl)
           (begin ,@(cdr cl))
           (my-cond ,@(cdr clauses)))))))

(assert-equal 2 (my-cond (#f 1) (#t 2)) "macro: my-cond second clause")
(assert-equal 1 (my-cond (#t 1) (else 2)) "macro: my-cond first + else")
(assert-equal 3 (my-cond (#f 1) (#f 2) (else 3)) "macro: my-cond with else")
(assert-equal #f (my-cond) "macro: my-cond empty = #f")
(assert-false (my-cond (#f 1)) "macro: my-cond no match = #f")

(end-suite)

(start-suite "macro-expand")

;; macro-expand should show the expansion
(define-macro (twice expr)
  (list '+ expr expr))

(define twice-expand (macro-expand '(twice (+ 1 2))))
;; The expansion should be '(+ (+ 1 2) (+ 1 2))
(assert-equal '(+ (+ 1 2) (+ 1 2)) twice-expand
  "macro-expand: twice expands correctly")

(assert-equal 6 (eval twice-expand)
  "macro-expand: expanded code evaluates correctly")

(assert-equal 6 (twice (+ 1 2))
  "macro: twice (+ 1 2) = 6")

(end-suite)

(test-summary)
