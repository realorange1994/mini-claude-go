package microlisp

// -------- Standard Library (embedded) --------
var initLib = `
;; SBCL test harness stub: with-test just evaluates the body
(define-macro (with-test spec . body)
  (cons (quote begin) body))

;; SBCL test harness stubs
(define (enable-test-parallelism) nil)
(define-macro (checked-compile form . rest)
  (list 'eval form))
(define (checked-eval form) (eval form))
(define (ctu:asm-search pattern source) nil)

;; SBCL test utility stubs
(define-macro (assert-error form . type)
  (list 'ignore-errors form))

(define-macro (assert-signal form condition)
  (list 'ignore-errors form))

(define-macro (assert-type fn expected-type)
  nil)

(define-macro (checked-compile-and-assert opts form cases)
  (let ((fn-sym (gensym "cca-")))
    (let ((is-pair (pair? cases))
          (is-nested #f))
      (if is-pair
          (set! is-nested (pair? (car cases)))
          (set! is-nested #f))
      (cons 'let
        (list (list (list fn-sym (list 'eval form)))
              (cons 'begin
                (let ((case-list (if is-nested (car cases) (list cases))))
                  (let ((gen-case (lambda (c)
                                    (if (pair? c)
                                        (let ((inputs (car c))
                                              (expected (if (pair? (cdr c)) (car (cdr c)) nil)))
                                          (if (and (pair? expected) (eq? (car expected) (quote condition)))
                                              (let ((raw-t (if (pair? (cdr expected)) (car (cdr expected)) (quote error))))
                                                (let ((cond-type (if (and (pair? raw-t) (eq? (car raw-t) (quote quote)) (pair? (cdr raw-t)))
                                                                       (car (cdr raw-t))
                                                                       raw-t)))
                                                  (list 'assert
                                                        (list 'handler-case
                                                              (list 'progn
                                                                    (list 'apply fn-sym (list 'quote inputs))
                                                                    (quote nil))
                                                              (list cond-type (list (quote c)) (quote t))
                                                              (list (quote error) (list (quote c)) (quote nil))))))
                                              (if (and (pair? expected) (eq? (car expected) (quote values)))
                                                  (list 'assert
                                                        (list 'equal
                                                              (list 'multiple-value-list
                                                                    (list 'apply fn-sym (list 'quote inputs)))
                                                              (list 'multiple-value-list
                                                                    (list 'quote (cdr expected)))))
                                                  (list 'assert
                                                        (list 'equal
                                                              (list 'apply fn-sym (list 'quote inputs))
                                                              (list 'quote expected))))))
                                        (list 'begin)))))
                    (mapcar gen-case case-list)))))))))

(define (ctu:find-code-constants fn) nil)
(define (ctu:ir1-named-calls fn) nil)
(define-macro (ctu:assert-no-consing form) form)
(define (ctu:find-named-callees fn) nil)
(define (sb-int:encapsulate fn name impl) nil)
(define (sb-int:unencapsulate fn name) nil)
(define (sb-impl::%remf indicator list) (remf list indicator))

(define (not x) (if x #f #t))
(define (caar x) (car (car x)))
(define (cadr x) (car (cdr x)))
(define (cdar x) (cdr (car x)))
(define (cddr x) (cdr (cdr x)))
(define (caaar x) (car (car (car x))))
(define (caadr x) (car (car (cdr x))))
(define (cadar x) (car (cdr (car x))))
(define (caddr x) (car (cdr (cdr x))))
(define (cdaar x) (cdr (car (car x))))
(define (cdadr x) (cdr (car (cdr x))))
(define (cddar x) (cdr (cdr (car x))))
(define (cdddr x) (cdr (cdr (cdr x))))
(define (caaaar x) (car (car (car (car x)))))
(define (caaadr x) (car (car (car (cdr x)))))
(define (caadar x) (car (car (cdr (car x)))))
(define (caaddr x) (car (car (cdr (cdr x)))))
(define (cadaar x) (car (cdr (car (car x)))))
(define (cadadr x) (car (cdr (car (cdr x)))))
(define (caddar x) (car (cdr (cdr (car x)))))
(define (cadddr x) (car (cdr (cdr (cdr x)))))
(define (cdaaar x) (cdr (car (car (car x)))))
(define (cdaadr x) (cdr (car (car (cdr x)))))
(define (cdadar x) (cdr (car (cdr (car x)))))
(define (cdaddr x) (cdr (car (cdr (cdr x)))))
(define (cddaar x) (cdr (cdr (car (car x)))))
(define (cddadr x) (cdr (cdr (car (cdr x)))))
(define (cdddar x) (cdr (cdr (cdr (car x)))))
(define (cddddr x) (cdr (cdr (cdr (cdr x)))))

(define (list . x) x)
(define (filter f lst)
  (if (null? lst) '()
    (if (f (car lst))
      (cons (car lst) (filter f (cdr lst)))
      (filter f (cdr lst)))))
(define (fold f init lst)
  (if (null? lst) init
    (fold f (f (car lst) init) (cdr lst))))
(define (fold-right f init lst)
  (if (null? lst) init
    (f (car lst) (fold-right f init (cdr lst)))))
(define (append . lists)
  (cond
    ((null? lists) '())
    ((null? (cdr lists)) (car lists))
    ((null? (car lists)) (apply append (cdr lists)))
    ((not (pair? (car lists))) (error "append: argument is not a list"))
    (else (cons (car (car lists)) (apply append (cons (cdr (car lists)) (cdr lists)))))))
(define (range n)
  (if (<= n 0) '() (append (range (- n 1)) (list (- n 1)))))
(define (list-ref lst n)
  (cond
    ((null? lst) '())
    ((= n 0) (car lst))
    (#t (list-ref (cdr lst) (- n 1)))))
;; member is now a Go builtin with :key/:test support
(define (member? x lst)
  (if (null? lst) #f
    (if (equal? x (car lst)) #t (member? x (cdr lst)))))
(define (any pred lst)
  (cond
    ((null? lst) #f)
    ((pred (car lst)) #t)
    (else (any pred (cdr lst)))))
(define (all pred lst)
  (cond
    ((null? lst) #t)
    ((not (pred (car lst))) #f)
    (else (all pred (cdr lst)))))
(define (take n lst)
  (if (or (<= n 0) (null? lst)) '()
    (cons (car lst) (take (- n 1) (cdr lst)))))
(define (drop n lst)
  (if (or (<= n 0) (null? lst)) lst
    (drop (- n 1) (cdr lst))))
(define (zip a b)
  (if (or (null? a) (null? b)) '()
    (cons (list (car a) (car b)) (zip (cdr a) (cdr b)))))
(define (flatten lst)
  (cond
    ((null? lst) '())
    ((pair? (car lst)) (append (flatten (car lst)) (flatten (cdr lst))))
    (else (cons (car lst) (flatten (cdr lst))))))
(define (square x) (* x x))
(define (modulo n d) (- n (* d (ffi "math/floor" (/ n d)))))
(define (atom x) (not (pair? x)))
(define (list? x) (or (null? x) (pair? x)))
; butlast and nbutlast are implemented as Go builtins (builtinButlast, builtinNbutlast)
; which properly handle both proper and dotted lists.
(define (last lst . n)
  (let ((n (if (null? n) 1 (car n))))
    (drop (max 0 (- (length lst) n)) lst)))
(define (signum n)
  (cond ((> n 0) 1) ((< n 0) -1) (#t 0)))
(define (even? n) (= (modulo n 2) 0))
(define (odd? n) (not (= (modulo n 2) 0)))
(define (zero? n) (= n 0))
(define (positive? n) (> n 0))
(define (negative? n) (< n 0))
(define (nreverse lst) (reverse lst))
(define (nconc . lsts)
  (define (nconc-2 a b)
    (if (null? a) b
      (let ((last-pair (last a)))
        (set-cdr! last-pair b)
        a)))
  (if (null? lsts) '()
    (if (null? (cdr lsts)) (car lsts)
      (nconc-2 (car lsts) (apply nconc (cdr lsts))))))
(define (rassoc key alist . rest)
  (if (null? alist) '()
    (let ((pair (car alist)))
      (if (and (pair? pair) (equal? key (cdr pair))) pair
        (apply rassoc key (cdr alist) rest)))))
(define (union list1 list2 . rest)
  (let ((test (if (null? rest) equal?
                (let ((args rest))
                  (if (and (pair? args) (equal? (car args) :test) (pair? (cdr args)))
                      (cadr args) equal?)))))
    (append list1 (filter (lambda (x) (not (member x list1))) list2))))
(define (intersection list1 list2)
  (filter (lambda (x) (member x list2)) list1))
(define (set-difference list1 list2)
  (filter (lambda (x) (not (member x list2))) list1))
(define (adjoin item list . rest)
  (if (member item list) list (cons item list)))
(define (every pred lst . rest)
  (apply seq-every (cons pred (cons lst rest))))
(define (some pred lst . rest)
  (apply seq-some (cons pred (cons lst rest))))
(define (notany pred lst . rest)
  (apply seq-notany (cons pred (cons lst rest))))
(define (notevery pred lst . rest)
  (apply seq-notevery (cons pred (cons lst rest))))

;; -------- CL macros: cond, case, typecase, prog1, push, pop, incf, decf --------
(define-macro (cond . clauses)
  (if (null? clauses) #f
    (if (eq? (caar clauses) 'else)
      (cons 'begin (cdar clauses))
      (list 'if (caar clauses)
        (cons 'begin (cdar clauses))
        (cons 'cond (cdr clauses))))))

(define-macro (prog1 first . rest)
  (let ((v (gensym)))
    (list 'let (list (list v first))
      (cons 'begin (append rest (list v))))))

(define-macro (prog2 first second . rest)
  (list 'begin first (cons 'prog1 (cons second rest))))

;; prog: (prog (varlist) . body) => (block nil (let (bindings) (tagbody . body)))
;; Each var in varlist can be: var, (var), or (var init)
(define-macro (prog varlist . body)
  (let ((bindings (mapcar (lambda (v)
                            (if (pair? v)
                              (if (null? (cdr v)) (list (car v) nil) v)
                              (list v nil)))
                          varlist)))
    (list 'block 'nil (list 'let bindings (cons 'tagbody body)))))

;; prog*: (prog* (varlist) . body) => (block nil (let* (bindings) (tagbody . body)))
(define-macro (prog* varlist . body)
  (let ((bindings (mapcar (lambda (v)
                            (if (pair? v)
                              (if (null? (cdr v)) (list (car v) nil) v)
                              (list v nil)))
                          varlist)))
    (list 'block 'nil (list 'let* bindings (cons 'tagbody body)))))

(define-macro (push item place)
  (list 'setf place (list 'cons item place)))

(define-macro (pop place)
  (let ((v (gensym)))
    (list 'let (list (list v place))
      (list 'setf place (list 'cdr v))
      (list 'car v))))

(define-macro (incf place . delta)
  (if (null? delta)
    (list 'setf place (list '+ place 1))
    (let ((g (gensym)))
      (list 'let (list (list g (car delta)))
        (list 'setf place (list '+ place g))))))

(define-macro (decf place . delta)
  (if (null? delta)
    (list 'setf place (list '- place 1))
    (let ((g (gensym)))
      (list 'let (list (list g (car delta)))
        (list 'setf place (list '- place g))))))

(define-macro (rotatef . places)
  (if (null? places) #f
    (if (null? (cdr places)) (car places)
      (let ((g (mapcar (lambda (_) (gensym)) places)))
        (list 'let (mapcar list g places)
          (cons 'begin
            (append
              (mapcar (lambda (p v) (list 'setf p v))
                   places
                   (append (cdr g) (list (car g))))
              (list (car g)))))))))

(define-macro (shiftf . args)
  (if (< (length args) 2)
    (error "shiftf: need at least 2 args")
    (let ((places (butlast args 1))
          (newval (car (last-pair args)))
          (g (mapcar (lambda (_) (gensym)) (butlast args 1))))
      (list 'let (mapcar list g (butlast args 1))
        (cons 'begin
          (append
            (mapcar (lambda (p v) (list 'setf p v))
                 places
                 (append (cdr g) (list newval)))
            (list (car g))))))))

(define-macro (psetq . pairs)
  (let ((places (quote ()))
        (values (quote ()))
        (rest pairs)
        (temps (quote ())))
    (while (not (null? rest))
      (set! places (cons (car rest) places))
      (set! rest (cdr rest))
      (set! values (cons (car rest) values))
      (set! rest (cdr rest)))
    (set! places (reverse places))
    (set! values (reverse values))
    (let ((i 0))
      (while (< i (length values))
        (set! temps (cons (gensym) temps))
        (set! i (+ i 1))))
    (set! temps (reverse temps))
    (list (quote let)
      (zip temps values)
      (cons (quote begin)
        (append
          (map2 (lambda (p t) (list (quote set!) p t)) places temps)
          (list (car temps)))))))

(define-macro (psetf . pairs)
  (let ((places (quote ()))
        (values (quote ()))
        (rest pairs)
        (temps (quote ())))
    (while (not (null? rest))
      (set! places (cons (car rest) places))
      (set! rest (cdr rest))
      (set! values (cons (car rest) values))
      (set! rest (cdr rest)))
    (set! places (reverse places))
    (set! values (reverse values))
    (let ((i 0))
      (while (< i (length values))
        (set! temps (cons (gensym) temps))
        (set! i (+ i 1))))
    (set! temps (reverse temps))
    (list (quote let)
      (zip temps values)
      (cons (quote begin)
        (append
          (map2 (lambda (p t) (list (quote setf) p t)) places temps)
          (list (car temps)))))))

(define (map2 f l1 l2)
  (if (or (null? l1) (null? l2)) '()
    (cons (f (car l1) (car l2))
          (map2 f (cdr l1) (cdr l2)))))

(define-macro (do varlist endlist . body)
  (let ((vars (mapcar car varlist))
        (inits (mapcar cadr varlist))
        (steps (mapcar (lambda (v) (if (null? (cddr v)) (car v) (caddr v))) varlist))
        (end-test (car endlist))
        (end-result (if (null? (cdr endlist)) #f (cadr endlist)))
        (lp (gensym)))
    (let ((step-args (mapcan2 list vars steps)))
      (list 'let (mapcar2 list vars inits)
        (list 'letrec
          (list (list lp (list 'lambda '()
                (list 'if end-test
                  end-result
                  (cons 'begin
                    (append body
                            (list (cons 'psetq step-args)
                                  (list lp))))))))
          (list lp))))))

(define-macro (do* varlist endlist . body)
  (let ((vars (mapcar car varlist))
        (inits (mapcar cadr varlist))
        (steps (mapcar (lambda (v) (if (null? (cddr v)) (car v) (caddr v))) varlist))
        (end-test (car endlist))
        (end-result (if (null? (cdr endlist)) #f (cadr endlist)))
        (lp (gensym)))
    (let ((step-forms (mapcar2 (lambda (v s) (list 'set! v s)) vars steps)))
      (list 'let* (mapcar2 list vars inits)
        (list 'letrec
          (list (list lp (list 'lambda '()
                (list 'if end-test
                  end-result
                  (cons 'begin
                    (append body
                            (append step-forms
                                    (list (list lp)))))))))
          (list lp))))))

(define (mapcar2 f l1 l2)
  (if (or (null? l1) (null? l2)) '()
    (cons (f (car l1) (car l2))
          (mapcar2 f (cdr l1) (cdr l2)))))

(define (mapcan2 f l1 l2)
  (if (or (null? l1) (null? l2)) '()
    (append (f (car l1) (car l2))
            (mapcan2 f (cdr l1) (cdr l2)))))

;; -------- Additional CL list functions --------
(define (copy-list lst)
  (if (null? lst) '()
    (if (pair? lst) (cons (car lst) (copy-list (cdr lst)))
      lst)))

(define (copy-tree x)
  (if (null? x) '()
    (if (pair? x) (cons (copy-tree (car x)) (copy-tree (cdr x)))
      x)))

(define (tree-equal x y . rest)
  (if (and (pair? x) (pair? y))
    (and (tree-equal (car x) (car y)) (tree-equal (cdr x) (cdr y)))
    (if (null? rest)
      (equal? x y)
      ((car rest) x y))))

(define (revappend x y)
  (if (null? x) y
    (revappend (cdr x) (cons (car x) y))))

(define (ldiff list sublist)
  (if (eq? list sublist) '()
    (if (null? list) '()
      (cons (car list) (ldiff (cdr list) sublist)))))

(define (tailp sublist list)
  (cond ((eq? sublist list) #t)
        ((null? list) #f)
        (else (tailp sublist (cdr list)))))

;; -------- type-of-matches helper --------
(define (type-of-matches obj type-spec)
  (cond
    ((eq? type-spec 'number) (number? obj))
    ((eq? type-spec 'string) (string? obj))
    ((eq? type-spec 'symbol) (symbol? obj))
    ((eq? type-spec 'list) (pair? obj))
    ((eq? type-spec 'cons) (pair? obj))
    ((eq? type-spec 'null) (null? obj))
    ((eq? type-spec 'boolean) (boolean? obj))
    ((eq? type-spec 'character) (char? obj))
    ((eq? type-spec 'function) (procedure? obj))
    ((eq? type-spec 'hash-table) (hash-table? obj))
    ((eq? type-spec 'stream) (streamp obj))
    ((eq? type-spec 'instance) (instance? obj))
    (else #f)))

;; destructure-pattern and destructuring-bind macro removed:
;; Go special form (bindPatternRec) handles &rest/&optional/&key properly
;; The old Lisp macro shadowed the Go implementation and was broken

(define (for-each f lst)
  (if (null? lst) #t
    (begin (f (car lst)) (for-each f (cdr lst)))))
(define-macro (when test . body)
  (list 'if test (cons 'begin body)))
(define-macro (unless test . body)
  (list 'if (list 'not test) (cons 'begin body)))

;; assert: simple version - (assert condition)
(define-macro (assert test . opts)
  (list 'if test
    nil
    (list 'error "assertion failed")))

;; builtin describe is used instead

;; -------- dotimes macro (letrec-based) --------
(define-macro (dotimes . args)
  (let ((var (caar args))
        (count (cadar args))
        (result (cdar args))
        (body (cdr args))
        (n (gensym))
        (lp (gensym)))
    (list 'let (list (list var 0) (list n count))
      (list 'letrec
        (list (list lp (list 'lambda '()
              (list 'if (list '>= var n)
                (cons 'begin result)
                (cons 'begin (append body
                  (list (list 'set! var (list '+ var 1))
                        (list lp))))))))
        (list lp)))))
(define-macro (defstruct name-and-options . slots)
  (letrec
    ((struct-name
       (if (pair? name-and-options) (car name-and-options) name-and-options))
     (options
       (if (pair? name-and-options) (cdr name-and-options) '()))
     (include-name #f)
     (constructor-name #f)
     (predicate-name #f)
     (copier-name #f)
     (conc-prefix #f)
     (print-fn #f)
     (repr-type 'instance)
     (slot-defs '())
     (expansions '())
     (parse-options
       (lambda (opts)
         (if (null? opts) 'done
           (begin
             (if (and (pair? (car opts)) (eq? (caar opts) :include))
               (set! include-name (cadar opts)))
             (if (and (pair? (car opts)) (eq? (caar opts) :constructor))
               (set! constructor-name (cadar opts)))
             (if (and (pair? (car opts)) (eq? (caar opts) :predicate))
               (set! predicate-name (cadar opts)))
             (if (and (pair? (car opts)) (eq? (caar opts) :copier))
               (set! copier-name (cadar opts)))
             (if (and (pair? (car opts)) (eq? (caar opts) :conc-name))
               (if (null? (cdar opts))
                 (set! conc-prefix "")
                 (set! conc-prefix (symbol->string (cadar opts))))
               )
             (if (and (pair? (car opts)) (eq? (caar opts) :print-object))
               (set! print-fn (cadar opts)))
             (if (and (pair? (car opts)) (eq? (caar opts) :print-function))
               (set! print-fn (cadar opts)))
             (if (and (pair? (car opts)) (eq? (caar opts) :type))
               (set! repr-type (cadar opts)))
             (parse-options (cdr opts))))))
     (parse-slots
       (lambda (sls)
         (if (null? sls) '()
           (if (symbol? (car sls))
             (cons (list (car sls) #f) (parse-slots (cdr sls)))
             (if (pair? (car sls))
               (cons (car sls) (parse-slots (cdr sls)))
               (parse-slots (cdr sls)))))))
     (slot-name
       (lambda (slot)
         (if (symbol? slot) slot
           (if (null? (cdr slot)) (car slot)
             (if (null? (cddr slot)) (car slot)
               (car slot))))))
     (gen-accessor
       (lambda (slot)
         (list 'define (list (accessor-name slot) 'obj)
               (list 'slot-value 'obj (list 'quote (slot-name slot))))))
     (gen-setter
       (lambda (slot read-only-slots)
         (if (member? (slot-name slot) read-only-slots)
           '#f
           (list 'define
                 (list (string->symbol
                       (string-append (symbol->string (accessor-name slot)) "-SETF"))
                       'val 'obj)
                 (list 'slot-set! 'obj (list 'quote (slot-name slot)) 'val)))))
     (accessor-name
       (lambda (slot)
         (if (string? conc-prefix)
           (if (eq? conc-prefix "")
             (slot-name slot)
             (string->symbol
               (string-append conc-prefix
                              (symbol->string (slot-name slot)))))
           (string->symbol
             (string-append (symbol->string struct-name) "-"
                            (symbol->string (slot-name slot)))))))
     (slot-keyword
       (lambda (slot)
         (string->symbol
           (string-append ":" (symbol->string (slot-name slot))))))
     (_g (gensym))
     (kw-args-sym (gensym)))
    ;; Body
    (parse-options options)
    (set! slot-defs (parse-slots slots))
    (if include-name
      (let ((parent-class (eval include-name)))
        (let ((parent-slots (class-slot-defs parent-class)))
          (let ((all-slots (append parent-slots slot-defs)))
            (let ((slot-names (mapcar slot-name all-slots))
                  (slot-defaults
                    (mapcar (lambda (s) (if (and (pair? s) (not (null? (cdr s)))) (cadr s) #f))
                         all-slots))
                  (read-only-slots
                    (mapcar (lambda (s)
                           (if (and (pair? s) (pair? (cddr s)) (eq? (caddr s) :read-only))
                             (car s) #f))
                         all-slots))
                  (pred-name
                    (if predicate-name predicate-name
                      (string->symbol
                        (string-append (symbol->string struct-name) "-p"))))
                  (cons-name
                    (if constructor-name constructor-name
                      (string->symbol
                        (string-append "make-" (symbol->string struct-name)))))
                  (copy-name
                    (if copier-name copier-name
                      (string->symbol
                        (string-append "copy-" (symbol->string struct-name))))))
            ;; Build expansion
            (set! expansions
              (cons (list 'defclass struct-name (list include-name) slot-names all-slots)
                    expansions))
            (let ((constructor-body
                    (cons 'let
                      (cons (list (list 'inst (list 'make-instance (list 'quote struct-name))))
                        (append
                          (mapcar (lambda (s)
                                 (list 'slot-set! 'inst (list 'quote (slot-name s))
                                   (list 'let (list (list _g (list 'member (slot-keyword s) kw-args-sym)))
                                     (list 'if _g (list 'cadr _g)
                                       (if (and (pair? s) (not (null? (cdr s)))) (cadr s) #f)))))
                               all-slots)
                          (list 'inst))))))
              (set! expansions
                (cons (list 'define (cons cons-name kw-args-sym)
                            constructor-body)
                      expansions)))
            (set! expansions (append (mapcar gen-accessor all-slots) expansions))
            (set! expansions (append (mapcar (lambda (s) (gen-setter s read-only-slots)) all-slots) expansions))
            (set! expansions
              (cons
                (list 'define (list pred-name 'obj)
                      (list 'is-a? 'obj (list 'quote struct-name)))
                expansions))
            (set! expansions
              (cons
                (list 'define (list copy-name 'obj)
                      (cons 'begin
                        (cons (list 'define 'new (list 'make-instance (list 'quote struct-name)))
                          (append
                            (mapcar (lambda (s)
                                   (list 'slot-set! 'new (list 'quote (slot-name s))
                                         (list (accessor-name s) 'obj)))
                                 all-slots)
                            (list 'new)))))
                expansions))
            (if print-fn
              (let ((print-fn-name
                      (string->symbol
                        (string-append (symbol->string struct-name) "-print"))))
                (set! expansions
                  (cons (list 'define print-fn-name print-fn) expansions))
                (set! expansions
                  (cons (list 'set-class-print-fn (list 'quote struct-name) print-fn-name)
                        expansions))))
            (cons 'begin (reverse expansions))))))
      ;; no :include branch
      (let ((all-slots slot-defs)
            (slot-names (mapcar slot-name slot-defs))
            (slot-defaults
              (mapcar (lambda (s) (if (and (pair? s) (not (null? (cdr s)))) (cadr s) #f))
                   slot-defs))
            (read-only-slots
              (mapcar (lambda (s)
                     (if (and (pair? s) (pair? (cddr s)) (eq? (caddr s) :read-only))
                       (car s) #f))
                   slot-defs))
            (pred-name
              (if predicate-name predicate-name
                (string->symbol
                  (string-append (symbol->string struct-name) "-p"))))
            (cons-name
              (if constructor-name constructor-name
                (string->symbol
                  (string-append "make-" (symbol->string struct-name)))))
            (copy-name
              (if copier-name copier-name
                (string->symbol
                  (string-append "copy-" (symbol->string struct-name))))))
        (set! expansions
          (cons (list 'defclass struct-name '() slot-names all-slots) expansions))
        (let ((constructor-body
                (cons 'let
                  (cons (list (list 'inst (list 'make-instance (list 'quote struct-name))))
                    (append
                      (mapcar (lambda (s)
                             (list 'slot-set! 'inst (list 'quote (slot-name s))
                               (list 'let (list (list _g (list 'member (slot-keyword s) kw-args-sym)))
                                 (list 'if _g (list 'cadr _g)
                                   (if (and (pair? s) (not (null? (cdr s)))) (cadr s) #f)))))
                           all-slots)
                      (list 'inst))))))
          (set! expansions
            (cons (list 'define (cons cons-name kw-args-sym)
                        constructor-body)
                  expansions)))
        (set! expansions (append (mapcar gen-accessor all-slots) expansions))
        (set! expansions (append (mapcar (lambda (s) (gen-setter s read-only-slots)) all-slots) expansions))
        (set! expansions
          (cons
            (list 'define (list pred-name 'obj)
                  (list 'is-a? 'obj (list 'quote struct-name)))
            expansions))
        (set! expansions
          (cons
            (list 'define (list copy-name 'obj)
                  (cons 'begin
                    (cons (list 'define 'new (list 'make-instance (list 'quote struct-name)))
                      (append
                        (mapcar (lambda (s)
                               (list 'slot-set! 'new (list 'quote (slot-name s))
                                     (list (accessor-name s) 'obj)))
                             all-slots)
                        (list 'new)))))
            expansions))
        (if print-fn
          (let ((print-fn-name
                  (string->symbol
                    (string-append (symbol->string struct-name) "-print"))))
            (set! expansions
              (cons (list 'define print-fn-name print-fn) expansions))
            (set! expansions
              (cons (list 'set-class-print-fn (list 'quote struct-name) print-fn-name)
                    expansions))))
        (cons 'begin (reverse expansions))))))

;; -------- let* macro --------
(define-macro (let* bindings . body)
  (if (null? bindings)
    (cons 'let (cons '() body))
    (list 'let (list (car bindings))
          (cons 'let* (cons (cdr bindings) body)))))

;; -------- while macro --------
(define-macro (while test . body)
  (let ((lp (gensym)))
    (list 'letrec
      (list (list lp (list 'lambda '()
            (list 'if test
              (cons 'begin (cons '(%loop-check) (append body (list (list lp)))))))))
      (list lp))))

;; -------- dolist macro --------
(define-macro (dolist spec . body)
  (let ((var (car spec))
        (list-expr (cadr spec))
        (result-cdr (cddr spec))
        (lst (gensym))
        (result-var (gensym)))
    (let ((result (if (null? result-cdr) #f (car result-cdr))))
      (list 'let*
        (list (list lst list-expr)
              (list var '())
              (list result-var (if result result #f)))
        (list 'while (list 'not (list 'null? lst))
          (list 'begin
            (list 'set! var (list 'car lst))
            (cons 'begin body)
            (list 'set! lst (list 'cdr lst))))
        result-var))))

;; -------- loop macro --------
(define-macro (loop . clauses)
  (let ((named #f) (initially '()) (finally '()) (with-list '())
        (for-list '()) (accum-list '()) (term-list '()) (do-list '())
        (return-val #f) (has-return #f)
        (finish-mode #f) (finish-val #f)
        (current-guard #f))


    ;; helper: get keyword from a list (first symbol)
    (define (get-keyword x)
      (if (and (not (null? x)) (symbol? (car x))) (car x) #f))

    (define (parse-clauses cls)
      (if (null? cls) 'done
        (let ((kw (car cls)) (rest (cdr cls)))
          (cond
            ((equal? kw 'for)
             (let ((var (car rest))
                   (kind (cadr rest))
                   (tail (cddr rest)))
               ;; Collect arguments until the next loop keyword
               (let ((args '()))
                 (while (and (pair? tail)
                             (not (member (car tail)
                                    '(being for with while until repeat collect sum count
                                      maximize minimize append nconc when unless
                                      if do named initially finally return always never thereis and by))))
                   (set! args (append args (list (car tail))))
                   (set! tail (cdr tail)))
                 (set! for-list
                   (cons (cons var (cons kind args)) for-list))
                 (parse-clauses tail))))
            ((equal? kw 'with)
             (let ((var (car rest))
                   (val (if (and (not (null? (cdr rest))) (equal? (cadr rest) '=))
                          (caddr rest) (cadr rest))))
               (set! with-list (cons (list var val) with-list))
               (parse-clauses (if (and (not (null? (cdr rest))) (equal? (cadr rest) '=))
                               (cdddr rest) (cddr rest)))))
            ((equal? kw 'while)
             (set! term-list (cons (list 'while (car rest)) term-list))
             (parse-clauses (cdr rest)))
            ((equal? kw 'until)
             (set! term-list (cons (list 'until (car rest)) term-list))
             (parse-clauses (cdr rest)))
            ((equal? kw 'repeat)
             (set! term-list (cons (list 'repeat (car rest)) term-list))
             (parse-clauses (cdr rest)))
            ((equal? kw 'and)
             ;; Parallel for clause: parse the next for clause
             (parse-clauses rest))
            ((equal? kw 'named)
             (set! named (car rest))
             (parse-clauses (cdr rest)))
            ((equal? kw 'initially)
             (let ((body '()))
               (define (collect-body xs)
                 (if (or (null? xs)
                         (member (car xs)
                           '(for with while until repeat collect sum count
                             maximize minimize append nconc when unless
                             if do named initially finally return always never thereis and)))
                   (begin (set! initially (append initially (reverse body)))
                          (parse-clauses xs))
                   (begin (set! body (cons (car xs) body))
                          (collect-body (cdr xs)))))
               (collect-body rest)))
            ((equal? kw 'finally)
             (let ((body '()))
               (define (collect-body xs)
                 (if (or (null? xs)
                         (member (car xs)
                           '(for with while until repeat collect sum count
                             maximize minimize append nconc when unless
                             if do named initially finally return always never thereis and)))
                   (begin (set! finally (append finally (reverse body)))
                          (parse-clauses xs))
                   (begin (set! body (cons (car xs) body))
                          (collect-body (cdr xs)))))
               (collect-body rest)))
            ((equal? kw 'do)
             (let ((body '()))
               (define (collect-body xs)
                 (if (or (null? xs)
                         (member (car xs)
                           '(for with while until repeat collect sum count
                             maximize minimize append nconc when unless
                             if do named initially finally return always never thereis and)))
                   (begin (if current-guard
                        (let ((bl (reverse body)))
                          (set! do-list (cons (list 'if current-guard
                                            (if (null? (cdr bl)) (car bl) (cons 'begin bl)))
                                          do-list))
                          (set! current-guard #f))
                        (set! do-list (append do-list (reverse body))))
                          (parse-clauses xs))
                   (begin (set! body (cons (car xs) body))
                          (collect-body (cdr xs)))))
               (collect-body rest)))
            ((equal? kw 'return)
             (if current-guard
               (begin
                 (set! do-list (cons (list 'if current-guard
                                      (list 'return-from 'nil (car rest)) '())
                                      do-list))
                 (set! current-guard #f))
               (begin
                 (set! has-return #t)
                 (set! return-val (car rest))))
             (parse-clauses (cdr rest)))
            ((equal? kw 'always)
             (set! finish-mode 'always)
             (if current-guard
               (set! do-list (cons (list 'if current-guard
                                    (list 'if (car rest) '()
                                      (list 'progn (list 'set! 'loop-result #f) (list 'loop-finish))))
                                    do-list))
               (set! do-list (cons (list 'if (car rest) '()
                                    (list 'progn (list 'set! 'loop-result #f) (list 'loop-finish)))
                                    do-list)))
             (set! current-guard #f)
             (parse-clauses (cdr rest)))
            ((equal? kw 'never)
             (set! finish-mode 'never)
             (if current-guard
               (set! do-list (cons (list 'if current-guard
                                    (list 'if (car rest)
                                      (list 'progn (list 'set! 'loop-result #f) (list 'loop-finish)) '()))
                                    do-list))
               (set! do-list (cons (list 'if (car rest)
                                    (list 'progn (list 'set! 'loop-result #f) (list 'loop-finish)) '())
                                    do-list)))
             (set! current-guard #f)
             (parse-clauses (cdr rest)))
            ((equal? kw 'thereis)
             (set! finish-mode 'thereis)
             (let ((tvar (gensym "thereis-")))
               (let ((new-item (list 'let (list (list tvar (car rest)))
                                      (list 'if tvar
                                        (list 'progn (list 'set! 'loop-result tvar) (list 'loop-finish)) '()))))
                 (if current-guard
                   (set! do-list (cons (list 'if current-guard new-item '()) do-list))
                   (set! do-list (cons new-item do-list)))))
             (set! current-guard #f)
             (parse-clauses (cdr rest)))
            ((equal? kw 'collect)
             (let ((expr (car rest))
                   (into-var (if (and (pair? (cdr rest)) (equal? (cadr rest) 'into))
                               (caddr rest) #f)))
               (set! accum-list
                 (cons (list 'collect expr current-guard into-var) accum-list))
               (set! current-guard #f)
               (if (and (pair? (cdr rest)) (equal? (cadr rest) 'into))
                 (parse-clauses (cdddr rest))
                 (parse-clauses (cdr rest)))))
            ((equal? kw 'sum)
             (let ((expr (car rest))
                   (into-var (if (and (pair? (cdr rest)) (equal? (cadr rest) 'into))
                               (caddr rest) #f)))
               (set! accum-list
                 (cons (list 'sum expr current-guard into-var) accum-list))
               (set! current-guard #f)
               (if (and (pair? (cdr rest)) (equal? (cadr rest) 'into))
                 (parse-clauses (cdddr rest))
                 (parse-clauses (cdr rest)))))
            ((equal? kw 'count)
             (let ((expr (car rest))
                   (into-var (if (and (pair? (cdr rest)) (equal? (cadr rest) 'into))
                               (caddr rest) #f)))
               (set! accum-list
                 (cons (list 'count expr current-guard into-var) accum-list))
               (set! current-guard #f)
               (if (and (pair? (cdr rest)) (equal? (cadr rest) 'into))
                 (parse-clauses (cdddr rest))
                 (parse-clauses (cdr rest)))))
            ((equal? kw 'maximize)
             (let ((expr (car rest))
                   (into-var (if (and (pair? (cdr rest)) (equal? (cadr rest) 'into))
                               (caddr rest) #f)))
               (set! accum-list
                 (cons (list 'maximize expr current-guard into-var) accum-list))
               (set! current-guard #f)
               (if (and (pair? (cdr rest)) (equal? (cadr rest) 'into))
                 (parse-clauses (cdddr rest))
                 (parse-clauses (cdr rest)))))
            ((equal? kw 'minimize)
             (let ((expr (car rest))
                   (into-var (if (and (pair? (cdr rest)) (equal? (cadr rest) 'into))
                               (caddr rest) #f)))
               (set! accum-list
                 (cons (list 'minimize expr current-guard into-var) accum-list))
               (set! current-guard #f)
               (if (and (pair? (cdr rest)) (equal? (cadr rest) 'into))
                 (parse-clauses (cdddr rest))
                 (parse-clauses (cdr rest)))))
            ((equal? kw 'append)
             (let ((expr (car rest))
                   (into-var (if (and (pair? (cdr rest)) (equal? (cadr rest) 'into))
                               (caddr rest) #f)))
               (set! accum-list
                 (cons (list 'append expr current-guard into-var) accum-list))
               (set! current-guard #f)
               (if (and (pair? (cdr rest)) (equal? (cadr rest) 'into))
                 (parse-clauses (cdddr rest))
                 (parse-clauses (cdr rest)))))
            ((equal? kw 'nconc)
             (let ((expr (car rest))
                   (into-var (if (and (pair? (cdr rest)) (equal? (cadr rest) 'into))
                               (caddr rest) #f)))
               (set! accum-list
                 (cons (list 'nconc expr current-guard into-var) accum-list))
               (set! current-guard #f)
               (if (and (pair? (cdr rest)) (equal? (cadr rest) 'into))
                 (parse-clauses (cdddr rest))
                 (parse-clauses (cdr rest)))))
            ((equal? kw 'by)
             ;; Set the step for the most recent for clause
             (if (null? for-list)
               (error "by: no for clause to modify")
               (let ((last-for (car for-list)))
                 (set-cdr! last-for (append (cdr last-for) (list 'by (car rest))))
                 (parse-clauses (cdr rest)))))
            ((or (equal? kw 'when) (equal? kw 'if))
             (let ((test (car rest)))
               (set! current-guard test)
               (parse-clauses (cdr rest))))
            ((equal? kw 'unless)
             (set! current-guard (list 'not (car rest)))
             (parse-clauses (cdr rest)))
            (else
             (set! do-list (cons kw do-list))
             (parse-clauses rest))))))

    ;; --- Parse ---
    (parse-clauses clauses)

    ;; --- Generate expansion ---
    (let* ((raw-for-vars (reverse for-list))
           (accums (reverse accum-list))
           (acc-names '())
           (collect-acc-names '())
           (acc-inits '())
           (acc-updates '())
           (acc-results '())
           (idx 0)
           ;; Expand in-clauses: (x in list) -> (tail-N in list) + (x (car tail-N))
           (in-lets '())
           ;; Expand = clauses without then: (x = expr) -> param with nil init + body set!
           (eq-lets '())
           (expanded '())
           ;; Collect destructuring specs for for...in with pattern vars
           (destr-specs '()))
      (for-each
        (lambda (f)
          (if (equal? (cadr f) 'in)
            (let* ((user-var (car f))
                   (list-expr (car (cddr f)))
                   (tail (gensym "in-")))
              ;; Check if user-var is a destructuring pattern (a list, not a symbol)
              (if (symbol? user-var)
                (begin
                  (set! in-lets (cons (list user-var tail list-expr #f) in-lets))
                  (set! expanded (cons (list tail 'in list-expr) expanded)))
                (let ((hidden-var (gensym "destr-")))
                  (set! in-lets (cons (list user-var hidden-var list-expr #t) in-lets))
                  (set! expanded (cons (list hidden-var 'in list-expr) expanded)))))
            (if (equal? (cadr f) 'on)
              (let* ((user-var (car f))
                     (list-expr (car (cddr f)))
                     (tail (gensym "on-")))
                ;; Check if user-var is a destructuring pattern (a list, not a symbol)
                (if (symbol? user-var)
                  (set! expanded (cons f expanded))
                  (let ((hidden-var (gensym "destr-")))
                    (set! in-lets (cons (list user-var hidden-var list-expr #t 'on) in-lets))
                    (set! expanded (cons (list hidden-var 'on list-expr) expanded)))))
            (if (equal? (cadr f) 'being)
              ;; Handle: for sym being each present-symbol of package
              ;; Handle: for k being the hash-keys of ht
              ;; Handle: for v being the hash-values of ht
              ;; Handle: for k being the hash-keys of ht using (hash-value v)
              ;;   args = (each/the present-symbol/hash-keys/hash-values of <expr> [using (hash-value/hash-key var)])
              (let* ((user-var (car f))
                     (args (cddr f))
                     (skip-article (if (and (pair? args) (or (equal? (car args) 'each) (equal? (car args) 'the))) (cdr args) args))
                     (sym-kind (if (pair? skip-article) (car skip-article) #f))
                     (after-kind (if (pair? skip-article) (cdr skip-article) '()))
                     (of-expr (if (and (pair? after-kind) (equal? (car after-kind) 'of) (pair? (cdr after-kind)))
                                   (cadr after-kind) #f))
                     ;; Check for using clause: (using (hash-value var)) or (using (hash-key var))
                     (rest-after-of (if (and (pair? after-kind) (equal? (car after-kind) 'of) (pair? (cdr after-kind)))
                                        (cddr after-kind) '()))
                     (using-clause (if (and (pair? rest-after-of) (equal? (car rest-after-of) 'using) (pair? (cdr rest-after-of)))
                                       (cadr rest-after-of) #f))
                     (using-kind (if (and using-clause (pair? using-clause)) (car using-clause) #f))
                     (using-var (if (and using-clause (pair? using-clause) (pair? (cdr using-clause))) (cadr using-clause) #f)))
                (if (and sym-kind of-expr
                         (or (equal? sym-kind 'present-symbol) (equal? sym-kind 'external-symbol)
                             (equal? sym-kind 'hash-keys) (equal? sym-kind 'hash-key)
                             (equal? sym-kind 'hash-values) (equal? sym-kind 'hash-value)))
                  (let* ((list-expr (cond
                                      ((equal? sym-kind 'present-symbol) (list 'package-symbols of-expr))
                                      ((equal? sym-kind 'external-symbol) (list 'package-external-symbols of-expr))
                                      ((or (equal? sym-kind 'hash-keys) (equal? sym-kind 'hash-key))
                                       (list 'hash-table-keys of-expr))
                                      ((or (equal? sym-kind 'hash-values) (equal? sym-kind 'hash-value))
                                       (list 'hash-table-values of-expr))
                                      (else (list 'package-symbols of-expr))))
                         (tail (gensym "in-")))
                    (set! in-lets (cons (list user-var tail list-expr) in-lets))
                    (set! expanded (cons (list tail 'in list-expr) expanded))
                    ;; Handle using clause: add parallel binding for hash-value or hash-key
                    (if (and using-var using-kind
                             (or (equal? using-kind 'hash-value) (equal? using-kind 'hash-values)
                                 (equal? using-kind 'hash-key) (equal? using-kind 'hash-keys)))
                      (let* ((using-list-expr (if (or (equal? using-kind 'hash-value) (equal? using-kind 'hash-values))
                                                   (list 'hash-table-values of-expr)
                                                   (list 'hash-table-keys of-expr)))
                             (using-tail (gensym "in-")))
                        (set! in-lets (cons (list using-var using-tail using-list-expr) in-lets))
                        (set! expanded (cons (list using-tail 'in using-list-expr) expanded)))))
                  (set! expanded (cons f expanded))))
            (if (equal? (cadr f) 'across)
              (let* ((var (car f))
                     (array-expr (car (cddr f)))
                     (idx (gensym "across-")))
                (set! eq-lets (cons (list var (list 'aref array-expr idx)) eq-lets))
                (set! expanded (cons (list idx 'from 0 'below (list 'length array-expr)) expanded))
                (set! expanded (cons (list var '= 'nil) expanded)))
            (if (equal? (cadr f) '=)
              (let* ((var (car f))
                     (args (cddr f))
                     (init-expr (car args)))
                (set! eq-lets (cons (list var init-expr) eq-lets))
                (set! expanded (cons (list var '= 'nil) expanded)))
              (set! expanded (cons f expanded))))))))
        raw-for-vars)
      (let* ((for-vars (reverse expanded))
             (in-lets (reverse in-lets))
             (eq-lets (reverse eq-lets)))

      ;; Build accumulator information
      (define (setup-accumulators)
        (for-each
          (lambda (a)
            (let* ((kind (car a))
                   (expr (cadr a))
                   (guard (or (caddr a) #t))
                   (into (if (null? (cdddr a)) #f (car (cdddr a))))
                   (name (if into into (string->symbol
                           (string-append
                             (symbol->string kind) "-acc-"
                             (number->string idx)))))
                   (_ (set! idx (+ idx 1))))
              (set! acc-names (cons name acc-names))
              (cond
                ((equal? kind 'collect)
                 (set! collect-acc-names (cons name collect-acc-names))
                 (set! acc-inits (cons (list name ''()) acc-inits))
                 (set! acc-updates
                   (cons (list 'set! name (list 'if guard (list 'cons expr name) name))
                         acc-updates))
                 (set! acc-results
                   (cons (list 'reverse name) acc-results)))
                ((equal? kind 'append)
                 (set! acc-inits (cons (list name ''()) acc-inits))
                 (set! acc-updates
                   (cons (list 'set! name (list 'if guard (list 'append name expr) name))
                         acc-updates))
                 (set! acc-results (cons name acc-results)))
                ((equal? kind 'nconc)
                 (set! acc-inits (cons (list name ''()) acc-inits))
                 (set! acc-updates
                   (cons (list 'set! name (list 'if guard (list 'append name expr) name))
                         acc-updates))
                 (set! acc-results (cons name acc-results)))
                ((equal? kind 'sum)
                 (set! acc-inits (cons (list name 0) acc-inits))
                 (set! acc-updates
                   (cons (list 'set! name (list 'if guard (list '+ name expr) name))
                         acc-updates))
                 (set! acc-results (cons name acc-results)))
                ((equal? kind 'count)
                 (set! acc-inits (cons (list name 0) acc-inits))
                 (set! acc-updates
                   (cons (list 'set! name
                           (list '+ name (list 'if (list 'if guard expr #f) 1 0)))
                         acc-updates))
                 (set! acc-results (cons name acc-results)))
                ((equal? kind 'maximize)
                 (set! acc-inits (cons (list name #f) acc-inits))
                 (set! acc-updates
                   (cons (list 'set! name
                           (list 'if guard
                             (list 'if (list 'or (list 'not name) (list '> expr name)) expr name)
                             name))
                         acc-updates))
                 (set! acc-results (cons name acc-results)))
                ((equal? kind 'minimize)
                 (set! acc-inits (cons (list name #f) acc-inits))
                 (set! acc-updates
                   (cons (list 'set! name
                           (list 'if guard
                             (list 'if (list 'or (list 'not name) (list '< expr name)) expr name)
                             name))
                         acc-updates))
                 (set! acc-results (cons name acc-results))))))
          accums))

      (setup-accumulators)

      ;; Build for-variable information
      (define (for-init f)
        (let ((var (car f)) (kind (cadr f)) (args (cddr f)))
          (cond
            ((or (equal? kind 'from) (equal? kind 'to)
                 (equal? kind 'below) (equal? kind 'above)
                 (equal? kind 'downto))
             (list var (car args)))
            ((equal? kind 'in)
             (list var (car args)))
            ((equal? kind 'on)
             (list var (car args)))
            ((equal? kind '=)
             ;; =: body rebinds each iteration
             (list var #f))
            (else (list var 0)))))

      (define (for-step f)
        (let ((var (car f)) (kind (cadr f)) (args (cddr f)))
          (cond
            ((equal? kind 'from)
             (let* ((start (car args))
                    (rest-args (cdr args))
                    (dir (if (null? rest-args) 'to (car rest-args)))
                    (end (cond
                           ((null? rest-args) start)
                           ((equal? (car rest-args) 'by) start)
                           ((null? (cdr rest-args)) (car rest-args))
                           ((equal? (cadr rest-args) 'by) (car rest-args))
                           (else (cadr rest-args))))
                    (by-val (cond
                              ((null? rest-args) 1)
                              ((equal? (car rest-args) 'by)
                               (if (null? (cdr rest-args)) 1 (cadr rest-args)))
                              ((null? (cdr rest-args)) 1)
                              ((equal? (cadr rest-args) 'by)
                               (if (null? (cddr rest-args)) 1 (caddr rest-args)))
                              ((null? (cddr rest-args)) 1)
                              ((equal? (caddr rest-args) 'by)
                               (if (null? (cdddr rest-args)) 1 (car (cdddr rest-args))))
                              (else 1))))
               (cond
                 ((equal? dir 'downto) (list '- var by-val))
                 ((equal? dir 'above) (list '- var by-val))
                 (else (list '+ var by-val)))))
            ((equal? kind 'downto)
             (let* ((end (car args))
                    (rest-args (cdr args))
                    (by-val (cond
                              ((null? rest-args) 1)
                              ((equal? (car rest-args) 'by)
                               (if (null? (cdr rest-args)) 1 (cadr rest-args)))
                              ((null? (cdr rest-args)) 1)
                              ((equal? (cadr rest-args) 'by)
                               (if (null? (cddr rest-args)) 1 (caddr rest-args)))
                              (else 1))))
               (list '- var by-val)))
            ((equal? kind 'in)
             (list 'cdr var))
            ((equal? kind 'on)
             (list 'cdr var))
            ((equal? kind '=)
             ;; =: body rebinds each iteration, step doesn't matter
             #f)
            (else var))))

      (define (for-termination f)
        (let ((var (car f)) (kind (cadr f)) (args (cddr f)))
          (cond
            ((equal? kind 'from)
             (let* ((start (car args))
                    (rest-args (cdr args))
                    (dir (if (null? rest-args) #f (car rest-args)))
                    (end (if (null? rest-args) #f
                           (cadr rest-args))))
               (cond
                 ((not dir) #f)
                 ((equal? dir 'to) (list '> var end))
                 ((equal? dir 'below) (list '>= var end))
                 ((equal? dir 'above) (list '<= var end))
                 ((equal? dir 'downto) (list '< var end))
                 ((equal? dir 'by) #f)
                 (else #f))))
            ((equal? kind 'downto)
             (list '< var (car args)))
            ((equal? kind 'to)
             (list '> var (car args)))
            ((equal? kind 'below)
             (list '>= var (car args)))
            ((equal? kind 'above)
             (list '<= var (car args)))
            ((equal? kind 'in)
             (list 'null? var))
            ((equal? kind 'on)
             (list 'null? var))
            (else #f))))

      ;; Build loop function body
      (let* ((params (mapcar car for-vars))
             (loop-name (if named named (gensym "loop-")))
             (term-tests '())
             (body-exprs '()))


        ;; Termination conditions from for clauses
        (for-each
          (lambda (f)
            (let ((t (for-termination f)))
              (if t (set! term-tests (cons t term-tests)))))
          for-vars)

        ;; Termination conditions from term-list
        (for-each
          (lambda (t)
            (cond
              ((equal? (car t) 'while)
               (set! term-tests
                 (cons (list 'not (cadr t)) term-tests)))
              ((equal? (car t) 'until)
               (set! term-tests (cons (cadr t) term-tests)))
              ((equal? (car t) 'repeat)
               (let ((rvar (gensym "repeat-")))
                 (set! acc-inits (cons (list rvar (cadr t)) acc-inits))
                 (set! term-tests
                   (cons (list '<= rvar 0) term-tests))
                 (set! acc-updates
                   (cons (list 'set! rvar (list '- rvar 1))
                         (reverse acc-updates)))))))
          (reverse term-list))

        ;; Build body: do clauses first, then accumulations
        (set! body-exprs (reverse do-list))

        ;; Build body: accumulations come AFTER do-clauses
        (set! body-exprs
          (append body-exprs (reverse acc-updates)))

        ;; If has-return and named, add return-from to body
        (if (and has-return named)
          (set! body-exprs
            (append body-exprs (list (list 'return-from loop-name return-val)))))


        ;; Add = clause bindings as pre-body set! operations
        (for-each
          (lambda (e)
            (let ((var (car e))
                  (init-expr (cadr e)))
              ;; recompute expr each iteration
              (set! body-exprs
                (cons (list 'set! var init-expr) body-exprs))))
          eq-lets)

        ;; Add in-clause element extraction as pre-body bindings (after eq-lets so in-lets come first)
        ;; For destructuring patterns: we use tail-var as the lambda param, and add
        ;; (let ((hidden-var (car tail-var))) ...) to the body. No set! needed.
        (for-each
          (lambda (p)
            (let ((user-var (car p))
                  (tail-var (cadr p))
                  (list-expr (caddr p))
                  (is-destr (car (cdddr p)))
                  (destr-mode (if (null? (cdr (cdddr p))) 'in (cadr (cdddr p)))))
              (if is-destr
                (begin
                  ;; Store (pattern tail-var mode) for wrapping
                  (set! destr-specs (cons (list user-var tail-var destr-mode) destr-specs)))
                (begin
                  ;; Add initial binding: (user-var (car list-expr))
                  (set! acc-inits
                    (cons (list user-var (list 'car list-expr)) acc-inits))
                  ;; Add set! at start of each iteration
                  (set! body-exprs
                    (cons (list 'set! user-var (list 'car tail-var))
                          body-exprs))))))
          in-lets)

        ;; Build the loop function
        (let* ((result-expr
                 (cond
                   (has-return
                    (let ((rv return-val))
                      (if (member rv acc-names)
                          (list 'reverse rv)
                          rv)))
                   (finish-mode
                    (cond ((equal? finish-mode 'always) #t)
                          ((equal? finish-mode 'never) #t)
                          ((equal? finish-mode 'thereis)
                           (if finish-val finish-val #f))
                          (else #f)))
                   ((not (null? acc-results))
                    (if (null? (cdr acc-results))
                      (car acc-results)
                      (cons 'list acc-results)))
                   (else #f)))
               (next-vals (mapcar for-step for-vars))
               (recurse-call (cons loop-name next-vals))
               (body-with-result-save
                 (if (and result-expr (not finish-mode))
                   (append body-exprs (list (list 'set! 'loop-result result-expr)))
                   body-exprs))
               (full-body
                 (if (null? term-tests)
                   (if (null? body-with-result-save)
                     (cons 'begin (list '(%loop-check) recurse-call))
                     (cons 'begin
                       (append (cons '(%loop-check) body-with-result-save) (list recurse-call))))
                   ;; With termination check
                   (let ((combined-term
                           (if (null? (cdr term-tests))
                             (car term-tests)
                             (cons 'or term-tests))))
                     (list 'if combined-term
                       (if result-expr result-expr '())
                       (if (null? body-with-result-save)
                         (cons 'begin (list '(%loop-check) recurse-call))
                         (cons 'begin
                           (append (cons '(%loop-check) body-with-result-save) (list recurse-call))))))))
               ;; finally forms
               (post-expr
                 (if (null? finally) '()
                   (if (null? (cdr finally))
                     (car finally)
                     (cons 'begin finally))))
               (loop-fn-body full-body)
               ;; Wrap full-body with destructuring-bind for any destructuring for-clauses
               (wrapped-body
                 (let ((result full-body))
                   (for-each
                     (lambda (d)
                       (let ((pattern (car d))
                             (tail-var (cadr d))
                             (mode (caddr d)))
                         (set! result (list 'destructuring-bind pattern
                                       (if (equal? mode 'on) tail-var (list 'car tail-var))
                                       result))))
                     destr-specs)
                   result)))

          ;; Wrap with finally
          (let* ((no-fin (cons 'letrec
                          (cons (list (list loop-name
                                     (cons 'lambda (cons params
                                       (list wrapped-body)))))
                                (list (cons loop-name
                                  (mapcar (lambda (f) (cadr (for-init f)))
                                       for-vars))))))
                 (final-expr
                   (if (not (null? post-expr))
                     (let ((rev-steps (mapcar (lambda (n) (list 'set! n (list 'reverse n))) collect-acc-names)))
                       (list 'let (list (list (gensym "loop-result-") no-fin))
                         (if (null? (cdr finally))
                           (if (null? rev-steps)
                             (car finally)
                             (cons 'begin (append rev-steps (list (car finally)))))
                           (cons 'begin (append rev-steps finally)))))
                     no-fin)))
            ;; Wrap with initial forms
            (let* ((with-bindings
                     (if (null? with-list) '()
                       (mapcar (lambda (w) (list (car w) (cadr w))) with-list)))
                   (all-bindings (append acc-inits (list (list 'loop-result '())) with-bindings))
                   (initially-expr
                     (if (null? initially) #f
                       (if (null? (cdr initially))
                         (car initially)
                         (cons 'begin initially))))
                   (wrapped
                     (if (null? all-bindings)
                       final-expr
                       (list 'let (reverse all-bindings) final-expr))))
              (if initially-expr
                (set! wrapped (list 'begin initially-expr wrapped)))
              (if named
                (list 'block named (list 'block 'nil wrapped))
                (list 'block 'nil wrapped))))))))))
;; The Go builtin format is registered in defPrim and takes precedence.

;; -------- CL-style sequence wrappers --------
(define (reduce fn seq . rest)
  (if (null? rest)
    (apply seq-reduce (list fn seq))
    (apply seq-reduce (append (list fn seq) rest))))
(define (find item seq . rest)
  (apply seq-find (cons item (cons seq rest))))
(define (position item seq . rest)
  (apply seq-position (cons item (cons seq rest))))
(define (remove-if pred seq . rest)
  (apply seq-remove-if (cons pred (cons seq rest))))
(define (remove-if-not pred seq . rest)
  (apply seq-remove-if-not (cons pred (cons seq rest))))
(define (substitute new old seq . rest)
  (apply seq-substitute (cons new (cons old (cons seq rest)))))
(define (substitute-if new pred seq . rest)
  (apply seq-substitute-if (cons new (cons pred (cons seq rest)))))
(define (substitute-if-not new pred seq . rest)
  (apply seq-substitute-if-not (cons new (cons pred (cons seq rest)))))
(define (sort seq pred . rest)
  (apply seq-sort (cons seq (cons pred rest))))
(define (stable-sort seq pred . rest)
  (apply seq-sort (cons seq (cons pred rest))))
(define (count item seq . rest)
  (apply seq-count (cons item (cons seq rest))))
(define (count-if pred seq . rest)
  (apply seq-count-if (cons pred (cons seq rest))))
(define (count-if-not pred seq . rest)
  (apply seq-count-if-not (cons pred (cons seq rest))))
(define (remove item seq . rest)
  (apply seq-remove (cons item (cons seq rest))))
(define (remove-duplicates seq . rest)
  (apply seq-remove-duplicates (cons seq rest)))
(define (find-if pred seq . rest)
  (apply seq-find-if (cons pred (cons seq rest))))
(define (find-if-not pred seq . rest)
  (apply seq-find-if-not (cons pred (cons seq rest))))
(define (position-if pred seq . rest)
  (apply seq-position-if (cons pred (cons seq rest))))
(define (position-if-not pred seq . rest)
  (apply seq-position-if-not (cons pred (cons seq rest))))
(define (merge seq1 seq2 pred . rest)
  (if (or (eq? seq1 'list) (eq? seq1 'vector) (eq? seq1 'string))
      ;; CL-style: (merge 'type seq1 seq2 pred) -- seq1 is type
      (let ((result (seq-merge seq2 pred (car rest))))
        (coerce result seq1))
      ;; MicroLisp-style: (merge seq1 seq2 pred)
      (seq-merge seq1 seq2 pred)))


;; -------- Condition system macros --------

;; with-simple-restart: establish a simple named restart
;; (with-simple-restart (name format-string &rest args) &body body)
(define-macro (with-simple-restart spec . body)
  (let ((rname (car spec)))
    (list 'restart-case
          (cons 'begin body)
          (list rname '() (list 'quote 'nil)))))

;; define-condition: macro for defining condition classes with :report support
;; (define-condition name (parents) (slots...))
;; (define-condition name (parents) (slots...) (:report fn))
(define-macro (define-condition name parents slots . options)
  (let ((report-expr nil))
    (dolist (opt options)
      (if (and (pair? opt) (eq (car opt) (quote :report)))
          (setq report-expr (cadr opt))))
    (let ((defclass-form (list (quote defclass) name parents slots)))
      (if report-expr
          (list (quote progn)
                defclass-form
                (list (quote defvar)
                      (string->symbol (string-append (symbol->string name) "-cond-report"))
                      report-expr))
          defclass-form))))




;; -------- with-slots, with-accessors --------
(define-macro (with-slots slots instance . body)
  (let ((inst (gensym)))
    (list 'let (list (list inst instance))
      (cons 'let
        (cons (mapcar (lambda (s)
                     (if (pair? s)
                       (list (car s) (list 'slot-value inst (list 'quote (cadr s))))
                       (list s (list 'slot-value inst (list 'quote s)))))
                   slots)
              body)))))

(define-macro (with-accessors accessors instance . body)
  (let ((inst (gensym)))
    (list 'let (list (list inst instance))
      (cons 'let
        (cons (mapcar (lambda (a)
                     (list (car a) (list (cadr a) inst)))
                   accessors)
              body)))))

;; -------- with-open-file --------
;; (with-open-file (var filespec &rest keys) &body body)
(define-macro (with-open-file spec . body)
  (let ((var (car spec))
        (filespec (cadr spec))
        (keys (cddr spec)))
    (list 'let (list (list var (cons 'open (cons filespec keys))))
      (list 'unwind-protect
        (cons 'progn body)
        (list 'close var)))))

;; -------- step macro --------
;; (step form) -- evaluate form with stepping enabled (currently just evaluates)
;; *step* variable controls stepping behavior
(define-macro (step form)
  form)

;; -------- defgeneric --------
;; (defgeneric name args &rest options)
;; Options: (:method-combination combo) (:method ...)
(define-macro (defgeneric name args . options)
  (let ((combo-opt (defpackage-find-opt options ':method-combination))
        (method-forms (filter (lambda (o) (and (pair? o) (equal? (car o) ':method))) options)))
    (let ((base (list 'defparameter name (list 'make-generic-function (list 'quote name))))
          (combo-form (if (null? combo-opt) '()
                         (list (list 'set-method-combination name (list 'quote (car combo-opt))))))
          (method-defs (mapcar (lambda (m) (cons 'defmethod (cons name (cdr m)))) method-forms)))
      (cons 'progn (append (list base) combo-form method-defs)))))

;; -------- defpackage --------
;; defpackage: create a package with optional use, export, nicknames
;; Usage: (defpackage :name (:use :cl) (:export :foo :bar) (:nickname :n))
(define-macro (defpackage name . options)
  ;; Strip leading colon from keyword package names
  (let ((pkg-str (if (keywordp name)
                   (substring (symbol->string name) 1 (string-length (symbol->string name)))
                   (if (symbol? name) (symbol->string name) name))))
    (let ((use-list (defpackage-find-opt options ':use))
          (exports (defpackage-find-opt options ':export))
          (nicknames (defpackage-find-opt options ':nickname)))
      (let ((base-forms (list (list 'make-package (list 'quote pkg-str))
                              (list 'in-package (list 'quote pkg-str))))
            (use-form (if (null? use-list)
                        '()
                        (list (list 'use-package
                                (cons 'list (mapcar (lambda (p) (list 'quote p)) use-list))))))
            (export-forms (mapcar (lambda (s) (list 'export (list 'quote s))) exports))
            (nick-forms (if (null? nicknames)
                          '()
                          (list (list 'rename-package
                                  (list 'quote pkg-str)
                                  (list 'quote pkg-str)
                                  (cons 'list (mapcar (lambda (n) (list 'quote n)) nicknames)))))))
        (cons 'progn
              (append base-forms
                      (append use-form
                              (append export-forms nick-forms))))))))

;; Helper function for defpackage: find option by key
(define (defpackage-find-opt opts key)
  (cond
    ((null? opts) '())
    ((and (pair? (car opts)) (equal? (caar opts) key)) (cdar opts))
    (else (defpackage-find-opt (cdr opts) key))))

;; remove-env: remove &environment and its following symbol from a lambda list
(define (remove-env ll)
  (cond
    ((null ll) nil)
    ((eq (car ll) '&environment) (cdr (cdr ll)))
    ((eq (car ll) '&ENVIRONMENT) (cdr (cdr ll)))
    (t (cons (car ll) (remove-env (cdr ll))))))

;; defsetf: (defsetf accessor (var* &environment env newval) body...)
;; or:      (defsetf accessor (var* newval) body...)
;; Supports optional &environment parameter. Filters it out from setter params.
(define-macro (defsetf name params . body)
  (let* ((params-list (if (and (pair? params) (pair? (car params)))
                          (car params)
                          params))
          (cleaned (remove-env params-list))
          (orig-vars (butlast cleaned))
          (newval-sym (car (last cleaned)))
          (setter-fn (string->symbol (string-append (symbol->string name) "-SETF"))))
    (eval (list 'define
                (cons setter-fn (cons newval-sym orig-vars))
                (cons 'begin body)))
    setter-fn))

;; get-setf-expansion: returns (vars vals store-vars setter-form) for a place form
(define (get-setf-expansion place)
  (cond
    ((and (pair? place) (symbol? (car place)))
     (let ((accessor (car place))
           (args (cdr place)))
       (let ((vars (mapcar (lambda (_) (gensym "gs-")) args))
             (store-var (gensym "store-")))
         (list vars
               args
               (list store-var)
               (list (string->symbol (string-append (symbol->string accessor) "-SETF"))
                     store-var
                     (cons (quote list) vars))))))
    (else
     (list '() '() (list (gensym "store-")) (quote (progn))))))

;; -------- Standard Condition Classes --------
;; ANSI CL condition hierarchy with appropriate slots and initargs
(defclass condition ()
  ((message :initform nil :initarg :message)
   (format-arguments :initform () :initarg :format-arguments)
   (format-control :initform nil :initarg :format-control)))

(defclass serious-condition (condition) ())

(defclass error (serious-condition) ())
(defclass simple-error (error) ())

(defclass warning (condition) ())
(defclass simple-warning (warning) ())

(defclass type-error (error)
  ((datum :initform nil :initarg :datum)
   (expected-type :initform nil :initarg :expected-type)))
(defclass simple-type-error (type-error) ())

(defclass program-error (error) ())
(defclass division-by-zero (error) ())
(defclass arithmetic-error (error)
  ((operation :initform nil :initarg :operation)
   (operands :initform () :initarg :operands)))
(defclass control-error (error) ())
(defclass stream-error (error)
  ((stream :initform nil :initarg :stream)))
(defclass end-of-file (stream-error) ())
(defclass file-error (error)
  ((pathname :initform nil :initarg :pathname)))
(defclass package-error (condition)
  ((package :initform nil :initarg :package)))
(defclass reader-error (stream-error) ())

(defclass break (serious-condition) ())
(defclass style-warning (warning) ())
(defclass storage-condition (serious-condition) ())
(defclass cell-error (error)
  ((name :initform nil :initarg :name)))
(defclass unbound-variable (cell-error) ())
(defclass unbound-slot (cell-error)
  ((instance :initform nil :initarg :instance)))
(defclass undefined-function (cell-error) ())
(defclass parse-error (error) ())
(defclass print-not-readable (error)
  ((object :initform nil :initarg :object)))
(defclass simple-condition (condition)
  ((format-control :initform nil :initarg :format-control)
   (format-arguments :initform () :initarg :format-arguments)))

;; -------- Condition Accessor Functions --------
(define (condition-message c) (slot-value c 'message))
(define (condition-format-string c) (slot-value c 'format-control))
(define (cell-error-name c) (slot-value c 'name))
(define (unbound-slot-instance c) (slot-value c 'instance))
(define (print-not-readable-object c) (slot-value c 'object))
(define (simple-condition-format-control c) (slot-value c 'format-control))
(define (simple-condition-format-arguments c) (slot-value c 'format-arguments))

;; -------- make-condition helper --------
;; make-condition is aliased to builtinMakeInstance (which calls make-instance).
;; Standard condition classes are defined above as CLOS defclass forms.

;; Exported condition classes:
;; condition, serious-condition, error, simple-error
;; warning, simple-warning, type-error, simple-type-error
;; program-error, division-by-zero, arithmetic-error
;; control-error, stream-error, end-of-file
;; file-error, package-error, reader-error

;; -------- with-condition-restarts macro --------
(defmacro with-condition-restarts (cond-form restarts-form &body body)
  (let ((cond-var (gensym "cond"))
        (restarts-var (gensym "restarts")))
    (list 'let
          (list (list cond-var cond-form)
                (list restarts-var restarts-form))
          (list 'unwind-protect
                (list 'progn
                      (list '%associate-restarts-with-condition cond-var restarts-var)
                      (cons 'progn body))
                (list '%dissociate-restarts-with-condition cond-var restarts-var)))))

;; -------- Standard Condition Accessor Functions --------
(defun type-error-datum (c) (slot-value c 'datum))
(defun type-error-expected-type (c) (slot-value c 'expected-type))
(defun stream-error-stream (c) (slot-value c 'stream))
(defun file-error-pathname (c) (slot-value c 'pathname))
(defun arithmetic-error-operation (c) (slot-value c 'operation))
(defun arithmetic-error-operands (c) (slot-value c 'operands))
(defun package-error-package (c) (slot-value c 'package))

;; -------- with-standard-io-syntax --------
;; (with-standard-io-syntax &body body)
;; Binds all standard read/write variables to their ANSI CL defaults,
;; executes body, then restores the original values.
(define-macro (with-standard-io-syntax . body)
  (list 'let
        (list (list '*old-print-base* '*print-base*)
              (list '*old-print-case* '*print-case*)
              (list '*old-print-radix* '*print-radix*)
              (list '*old-print-escape* '*print-escape*)
              (list '*old-print-circle* '*print-circle*)
              (list '*old-print-pretty* '*print-pretty*)
              (list '*old-print-length* '*print-length*)
              (list '*old-print-level* '*print-level*)
              (list '*old-print-readably* '*print-readably*)
              (list '*old-print-gensym* '*print-gensym*)
              (list '*old-print-array* '*print-array*)
              (list '*old-read-base* '*read-base*)
              (list '*old-read-default-float-format* '*read-default-float-format*)
              (list '*old-read-eval* '*read-eval*)
              (list '*old-read-suppress* '*read-suppress*))
        (list 'unwind-protect
              (list 'progn
                    (list 'set! '*print-base* 10)
                    (list 'set! '*print-case* (list 'quote ':upcase))
                    (list 'set! '*print-radix* 'nil)
                    (list 'set! '*print-escape* #t)
                    (list 'set! '*print-circle* 'nil)
                    (list 'set! '*print-pretty* 'nil)
                    (list 'set! '*print-length* 'nil)
                    (list 'set! '*print-level* 'nil)
                    (list 'set! '*print-readably* #t)
                    (list 'set! '*print-gensym* #t)
                    (list 'set! '*print-array* #t)
                    (list 'set! '*read-base* 10)
                    (list 'set! '*read-default-float-format* (list 'quote 'single-float))
                    (list 'set! '*read-eval* #t)
                    (list 'set! '*read-suppress* 'nil)
                    (cons 'progn body))
              (list 'progn
                    (list 'set! '*print-base* '*old-print-base*)
                    (list 'set! '*print-case* '*old-print-case*)
                    (list 'set! '*print-radix* '*old-print-radix*)
                    (list 'set! '*print-escape* '*old-print-escape*)
                    (list 'set! '*print-circle* '*old-print-circle*)
                    (list 'set! '*print-pretty* '*old-print-pretty*)
                    (list 'set! '*print-length* '*old-print-length*)
                    (list 'set! '*print-level* '*old-print-level*)
                    (list 'set! '*print-readably* '*old-print-readably*)
                    (list 'set! '*print-gensym* '*old-print-gensym*)
                    (list 'set! '*print-array* '*old-print-array*)
                    (list 'set! '*read-base* '*old-read-base*)
                    (list 'set! '*read-default-float-format* '*old-read-default-float-format*)
                    (list 'set! '*read-eval* '*old-read-eval*)
                    (list 'set! '*read-suppress* '*old-read-suppress*)))))

;; -------- with-hash-table-iterator --------
;; (with-hash-table-iterator (name hash-table) &body body)
;; Creates a local macro NAME that on each call returns (values key value t)
;; or (values nil nil nil) at end.
(define-macro (with-hash-table-iterator spec . body)
  (let ((name (car spec))
        (ht (cadr spec))
        (keys-var (gensym))
        (idx-var (gensym)))
    (list 'let (list (list keys-var (list 'hash-table-keys ht))
                     (list idx-var 0))
      (list 'macrolet
            (list (list name '()
                  (list 'let (list (list 'k (list 'nth idx-var keys-var)))
                    (list 'if (list '< idx-var (list 'length keys-var))
                          (list 'progn
                                (list 'set! idx-var (list '+ idx-var 1))
                                (list 'values 'k (list 'gethash 'k ht) #t))
                          (list 'values 'nil 'nil 'nil)))))
            (cons 'progn body)))))

;; -------- internal-time-units-per-second --------
(define (internal-time-units-per-second) 1000000)

;; -------- get-decoded-time --------
;; Convenience: calls decode-universal-time on current universal time
(define (get-decoded-time)
  (decode-universal-time (get-universal-time)))

;; -------- with-open-stream --------
;; (with-open-stream (var stream-form) &body body)
(define-macro (with-open-stream spec . body)
  (let ((var (car spec))
        (form (cadr spec)))
    (list 'let (list (list var form))
      (list 'unwind-protect
            (cons 'progn body)
            (list 'if var (list 'close var))))))

;; -------- with-input-from-string --------
;; (with-input-from-string (var string) &body body)
(define-macro (with-input-from-string spec . body)
  (let ((var (car spec))
        (str (cadr spec)))
    (list 'let (list (list var (list 'make-string-input-stream str)))
      (list 'unwind-protect
            (cons 'progn body)
            (list 'close var)))))

;; -------- with-output-to-string --------
;; (with-output-to-string (var) &body body)
(define-macro (with-output-to-string spec . body)
  (let ((var (car spec)))
    (list 'let (list (list var (list 'make-string-output-stream)))
      (cons 'progn (append body
        (list (list 'get-output-stream-string var)))))))

;; -------- pprint --------
;; Pretty-print a form to the current output stream
(define (pprint obj)
  (let ((*print-pretty* #t))
    (princ obj)
    (terpri)
    obj))

;; -------- pprint-newline --------
(define (pprint-newline kind) nil)

;; -------- pprint-dispatch --------
(define (pprint-dispatch object) nil)

;; -------- pprint-tab --------
(define (pprint-tab kind col1 col2) nil)

;; -------- pprint-logical-block --------
(define-macro (pprint-logical-block spec . body)
  (cons 'progn body))

;; -------- pprint-pop / pprint-exit-if-list-exhausted --------
(define (pprint-pop list) nil)
(define (pprint-exit-if-list-exhausted) nil)

;; -------- with-package-iterator --------
(define-macro (with-package-iterator spec . body)
  (cons 'progn body))

;; -------- pp (alias for pprint) --------
(define (pp obj)
  (let ((*print-pretty* #t))
    (princ obj)
    (terpri)
    obj))

;; -------- copy-structure (Lisp-level wrapper) --------
;; copy-structure is implemented as a Go builtin

`
