;; MicroLisp sequence operation tests

(define (assert condition msg)
  (if (not condition)
    (begin (display "FAIL: ") (display msg) (newline))
    (begin (display "PASS: ") (display msg) (newline))))

(display "=== sequence tests ===")
(newline)

(define nums '(3 1 4 1 5 9 2 6))

;; --- Test 1: seq-map ---
(display "Test 1: seq-map")
(newline)
(assert (equal? (seq-map (lambda (x) (* x 2)) '(1 2 3)) '(2 4 6)) "seq-map double")

;; --- Test 2: reduce ---
(display "Test 2: reduce")
(newline)
(assert (= (reduce + '(1 2 3 4 5)) 15) "reduce sum")
(assert (= (reduce + '(1 2 3) :initial-value 10) 16) "reduce with initial-value")

;; --- Test 3: sort ---
(display "Test 3: sort")
(newline)
(assert (equal? (sort '(3 1 2) <) '(1 2 3)) "sort ascending")
(assert (equal? (sort '(1 2 3) >) '(3 2 1)) "sort descending")

;; --- Test 4: find ---
(display "Test 4: find")
(newline)
(assert (= (find 4 nums) 4) "find 4")
(assert (null? (find 99 nums)) "find missing")

;; --- Test 5: position ---
(display "Test 5: position")
(newline)
;; nums = (3 1 4 1 5 9 2 6), 0-indexed: 0=3, 1=1, 2=4, 3=1, 4=5, 5=9, 6=2, 7=6
(assert (= (position 4 nums) 2) "position of 4 is 2")
(assert (null? (position 99 nums)) "position of missing")
(assert (= (position 1 nums) 1) "position of first 1 is 1")
(assert (= (position 1 nums :from-end #t) 3) "position from-end of last 1 is 3")

;; --- Test 6: remove-if ---
(display "Test 6: remove-if")
(newline)
(define (gt3 x) (> x 3))
(assert (equal? (remove-if gt3 '(1 2 3 4 5 6)) '(1 2 3)) "remove-if >3")
(assert (equal? (remove-if gt3 '(1 2 3 4 5 6) :count 1) '(1 2 3 5 6)) "remove-if :count 1")
(assert (equal? (remove-if gt3 '(1 2 3 4 5 6) :start 2) '(1 2 3)) "remove-if :start 2")
(assert (equal? (remove-if gt3 '(1 2 3 4 5 6) :end 4) '(1 2 3 5 6)) "remove-if :end 4")

;; --- Test 7: substitute ---
(display "Test 7: substitute")
(newline)
(assert (equal? (substitute :x 3 '(1 2 3 4 3 5)) '(1 2 :x 4 :x 5)) "substitute all")
(assert (equal? (substitute :x 3 '(1 2 3 4 3 5) :count 1) '(1 2 :x 4 3 5)) "substitute :count 1")
(assert (equal? (substitute :x 3 '(1 2 3 4 3 5) :start 3) '(1 2 3 4 :x 5)) "substitute :start 3")
(assert (equal? (substitute :x 3 '(1 2 3 4 3 5) :from-end #t :count 1) '(1 2 3 4 :x 5)) "substitute from-end :count 1")

;; --- Test 8: string-upcase/downcase ---
(display "Test 8: string-upcase/downcase")
(newline)
(assert (equal? (string-upcase "hello") "HELLO") "string-upcase")
(assert (equal? (string-downcase "HELLO") "hello") "string-downcase")

(display "=== ALL SEQUENCE TESTS PASSED ===")
(newline)
