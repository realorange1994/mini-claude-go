;; control-flow-advanced.lisp — tests for advanced control flow
;; Covers: block/return-from, catch/throw, progv, multiple-value-bind,
;;         multiple-value-setq, values, values-list, flet, labels

(load "tests/framework.lisp")
(start-suite "Block and Return-From")

;; --- block / return-from ---
(assert-equal 5 (block test (return-from test 5) 3) "block with immediate return")
(assert-equal 7 (block test 7) "block without return evaluates body")
(assert-equal 5 (block nil (return-from nil 5) 3) "nil block with return")
(assert-equal 10 (block test (+ 3 7)) "block evaluates last form")
(assert-equal 42 (block test (if #t (return-from test 42) 99)) "block return in if-true branch")

(end-suite)
(start-suite "Catch and Throw")

;; --- catch / throw ---
(assert-equal 99 (catch 'foo (throw 'foo 99) 100) "catch/throw basic")
(assert-equal 100 (catch 'foo 100) "catch without throw returns body")
(assert-equal 42 (catch 'bar (if #t 42 (throw 'bar 99))) "catch without throw in if")
(assert-equal 7 (catch 'a (catch 'b (throw 'a 7) 8)) "nested catch - outer catches")
(assert-equal 8 (catch 'a (catch 'b (throw 'b 8) 9)) "nested catch - inner catches")

(end-suite)
(start-suite "Progv")

;; --- progv ---
(assert-equal 99 (progv '(x) '(99) x) "progv basic binding")
(assert-equal 15 (progv '(a b) '(5 10) (+ a b)) "progv multiple bindings")

(end-suite)
(start-suite "Multiple Values")

;; --- values ---
(assert-equal 1 (values 1) "values with one arg")
(assert-equal 3 (values 3 4) "values primary value")

;; --- multiple-value-bind ---
(assert-equal 3 (multiple-value-bind (a b) (values 1 2) (+ a b)) "mvb basic")
(assert-equal 6 (multiple-value-bind (x y z) (values 1 2 3) (+ x y z)) "mvb three values")
(assert-equal 1 (multiple-value-bind (a) (values 1 2 3) a) "mvb takes first value only")

;; --- values-list ---
(assert-equal 1 (values-list '(1 2 3)) "values-list primary value")

(end-suite)
(start-suite "Flet and Labels")

;; --- flet ---
(assert-equal 7 (flet ((add (x y) (+ x y))) (add 3 4)) "flet basic")
(assert-equal 10 (flet ((double (x) (* x 2))) (double 5)) "flet double")
(assert-equal 25 (flet ((square (x) (* x x))) (square 5)) "flet square")
(assert-equal 5 (flet ((f (x) (+ x 1)) (g (x) (+ x 2))) (+ (f 1) (g 1))) "flet two functions")
(assert-equal 10 (flet ((f (x) (* x 2))) (flet ((f (x) (* x 5))) (f 2))) "flet shadows outer flet")

;; --- labels ---
(assert-equal 5 (labels ((count (n) (if (<= n 0) 0 (+ 1 (count (- n 1)))))) (count 5)) "labels recursive")
(assert-equal 120 (labels ((fact (n) (if (<= n 1) 1 (* n (fact (- n 1)))))) (fact 5)) "labels factorial")
(assert-equal 55 (labels ((fib (n) (if (<= n 1) n (+ (fib (- n 1)) (fib (- n 2)))))) (fib 10)) "labels fibonacci")
(assert-equal 2 (labels ((f (n) (if (= n 0) 1 (g (- n 1)))) (g (n) (if (= n 0) 2 (f (- n 1))))) (f 5)) "labels mutual recursion")

(end-suite)
(start-suite "Control Flow Combinations")

;; --- Combining control flow ---
(assert-equal 15 (block b (flet ((f (x) (if (> x 10) (return-from b x) x))) (f 5) (f 15))) "block return from flet")
(assert-equal 30 (catch 'done (labels ((search (lst) (if (null? lst) (throw 'done 0) (if (> (car lst) 25) (throw 'done (car lst)) (search (cdr lst)))))) (search '(1 5 10 20 30 40)))) "catch/throw in labels")

(end-suite)
(test-summary)
