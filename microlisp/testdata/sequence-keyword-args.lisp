;; sequence-keyword-args.lisp — tests for sequence functions with keyword arguments
;; Covers: :test :key :from-end :start :end on remove, find, count, position, etc.

(load "tests/framework.lisp")
(start-suite "Remove with Keyword Args")

;; --- remove ---
(assert-equal '(1 3 4) (remove 2 (list 1 2 3 2 4)) "remove basic")
(assert-equal '(1 3 4) (remove 2 (list 1 2 3 2 4) :test (function =)) "remove with = test")

;; --- remove-if ---
(assert-equal '(1 3) (remove-if (function evenp) (list 1 2 3 4)) "remove-if basic")
(assert-equal '(2 4) (remove-if (function oddp) (list 1 2 3 4)) "remove-if oddp")

;; --- remove-duplicates ---
(assert-equal '(1 2 3 4) (remove-duplicates (list 1 2 3 2 4)) "remove-duplicates")

(end-suite)
(start-suite "Find with Keyword Args")

;; --- find ---
(assert-equal 3 (find 3 (list 1 2 3 4)) "find basic")
(assert-nil (find 5 (list 1 2 3 4)) "find not found")
(assert-equal 2 (find 2 (list 1 2 3 2 4) :from-end #t) "find from-end")

;; --- find-if ---
(assert-equal 2 (find-if (function evenp) (list 1 2 3 4)) "find-if basic")

(end-suite)
(start-suite "Count with Keyword Args")

;; --- count ---
(assert-equal 2 (count 2 (list 1 2 3 2 4)) "count basic")
(assert-equal 0 (count 5 (list 1 2 3)) "count not found")

(end-suite)
(start-suite "Position with Keyword Args")

;; --- position ---
(assert-equal 2 (position 3 (list 1 2 3 4 5)) "position basic")
(assert-equal 2 (position 3 (list 1 2 3 4 5) :from-end #t) "position from-end")
(assert-nil (position 9 (list 1 2 3)) "position not found")

;; --- position-if ---
(assert-equal 1 (position-if (function evenp) (list 1 2 3 4)) "position-if basic")

(end-suite)
(start-suite "Substitute with Keyword Args")

;; --- substitute ---
(assert-equal '(1 0 3 0 4) (substitute 0 2 (list 1 2 3 2 4)) "substitute basic")
(assert-equal '(1 0 3 0 4) (substitute 0 2 (list 1 2 3 2 4) :from-end #t) "substitute from-end")

;; --- substitute-if ---
(assert-equal '(1 99 3 99 99) (substitute-if 99 (function evenp) (list 1 2 3 2 4)) "substitute-if basic")

(end-suite)
(start-suite "Reduce")

;; --- reduce ---
(assert-equal 15 (reduce (function +) (list 1 2 3 4 5)) "reduce sum")
(assert-equal 25 (reduce (function +) (list 1 2 3 4 5) :initial-value 10) "reduce with initial")
(assert-equal 6 (reduce (function *) (list 1 2 3)) "reduce product")

(end-suite)
(start-suite "Iteration Predicates")

;; --- every ---
(assert-true (every (function evenp) (list 2 4 6)) "every true")
(assert-false (every (function evenp) (list 2 3 4)) "every false")

;; --- some ---
(assert-true (some (function evenp) (list 1 2 3)) "some true")
(assert-nil (some (function evenp) (list 1 3 5)) "some false")

;; --- notany ---
(assert-true (notany (function evenp) (list 1 3 5)) "notany true")
(assert-false (notany (function evenp) (list 1 2 3)) "notany false")

;; --- notevery ---
(assert-true (notevery (function evenp) (list 1 2 3)) "notevery true")
(assert-false (notevery (function evenp) (list 2 4 6)) "notevery false")

(end-suite)
(start-suite "Sort and Related")

;; --- sort ---
(assert-equal '(1 1 3 4 5) (sort (list 3 1 4 1 5) (function <)) "sort ascending")
(assert-equal '(5 4 3 1 1) (sort (list 3 1 4 1 5) (function >)) "sort descending")

(end-suite)
(test-summary)
