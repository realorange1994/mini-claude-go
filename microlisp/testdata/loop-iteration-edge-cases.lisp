;; ============================================================
;; Loop and Iteration Edge Cases - simplified for MicroLisp
;; ============================================================

(load "tests/framework.lisp")

;; ============================================================
;; Basic Loop Forms
;; ============================================================
(start-suite "loop-basics")

;; Simple counting loop
(assert-equal 55 (loop for i from 1 to 10 sum i) "loop: basic summing")

;; Loop with collect
(assert-equal '(1 2 3 4 5) (loop for i from 1 to 5 collect i) "loop: collect")

;; Loop with list iteration
(assert-equal '(a b c) (loop for x in '(a b c) collect x) "loop: for-in")

(end-suite)

;; ============================================================
;; Loop Downto
;; ============================================================
(start-suite "loop-downto")

;; Downto
(assert-equal '(5 4 3 2 1) (loop for i from 5 downto 1 collect i) "loop: downto")

(end-suite)

;; ============================================================
;; Loop Multiple Variables
;; ============================================================
(start-suite "loop-multiple-vars")

(assert-equal '((1 a) (2 b) (3 c))
              (loop for i from 1 to 3
                    for letter in '(a b c)
                    collect (list i letter))
              "loop: multiple variables")

(end-suite)

;; ============================================================
;; Loop Conditions
;; ============================================================
(start-suite "loop-conditions")

;; While
(assert-equal '(1 2 3) (loop for i from 1 to 10 while (< i 4) collect i) "loop: while")

;; Until
(assert-equal '(1 2 3) (loop for i from 1 until (= i 4) collect i) "loop: until")

(end-suite)

;; ============================================================
;; Loop Aggregation
;; ============================================================
(start-suite "loop-aggregation")

;; Sum
(assert-equal 15 (loop for i from 1 to 5 sum i) "loop: sum")

;; Maximize/Minimize
(assert-equal 10 (loop for i in '(1 5 3 10 2) maximize i) "loop: maximize")
(assert-equal 1 (loop for i in '(1 5 3 10 2) minimize i) "loop: minimize")

(end-suite)

;; ============================================================
;; DOTIMES and DOLIST
;; ============================================================
(start-suite "dotimes-dolist")

(define sum 0)
(dotimes (i 5)
  (set! sum (+ sum i)))
(assert-equal 10 sum "dotimes: basic")

(define items '())
(dolist (x '(a b c))
  (set! items (cons x items)))
(assert-equal '(c b a) items "dolist: basic")

(end-suite)

;; ============================================================
;; DO Variations
;; ============================================================
(start-suite "do-variations")

;; Basic DO
(assert-equal 5 (do ((i 0 (+ i 1)))
                     ((= i 5) i)
                   ;; body
                   ) "do: basic")

(end-suite)

;; ============================================================
;; Iteration with Destructuring
;; ============================================================
(start-suite "iteration-destructuring")

;; Destructuring in dolist
(define pairs '((1 . a) (2 . b)))
(define result '())
(dolist (pair pairs)
  (let ((key (car pair))
        (val (cdr pair)))
    (set! result (cons (list key val) result))))
;; Reverse to get expected order
(assert-equal '((1 a) (2 b)) (reverse result) "destructuring: manual")

(end-suite)

;; ============================================================
;; Summary
;; ============================================================
(test-summary)
