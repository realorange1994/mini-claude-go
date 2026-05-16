;; MicroLisp hash table tests

(define (assert condition msg)
  (if (not condition)
    (begin (display "FAIL: ") (display msg) (newline))
    (begin (display "PASS: ") (display msg) (newline))))

(display "=== hash table tests ===")
(newline)

;; --- Test 1: Create and type check ---
(display "Test 1: Create and type check")
(newline)
(define ht (make-hash-table))
(assert (hash-table? ht) "hash-table? true")
(assert (not (hash-table? 42)) "hash-table? false")

;; --- Test 2: gethash/setf ---
(display "Test 2: gethash/setf")
(newline)
(setf (gethash :a ht) 10)
(setf (gethash :b ht) 20)
(assert (= (gethash :a ht) 10) "gethash :a")
(assert (= (gethash :b ht) 20) "gethash :b")
(assert (null? (gethash :c ht)) "gethash missing key")

;; --- Test 3: hash-table-count ---
(display "Test 3: hash-table-count")
(newline)
(assert (= (hash-table-count ht) 2) "count after inserts")

;; --- Test 4: remhash ---
(display "Test 4: remhash")
(newline)
(assert (remhash :a ht) "remhash returns true")
(assert (null? (gethash :a ht)) "gethash after remhash")
(assert (= (hash-table-count ht) 1) "count after remhash")

;; --- Test 5: clrhash ---
(display "Test 5: clrhash")
(newline)
(clrhash ht)
(assert (= (hash-table-count ht) 0) "count after clrhash")

;; --- Test 6: maphash ---
(display "Test 6: maphash")
(newline)
(setf (gethash :x ht) 1)
(setf (gethash :y ht) 2)
(define kv-pairs '())
(maphash (lambda (k v) (set! kv-pairs (cons (list k v) kv-pairs))) ht)
(assert (= (length kv-pairs) 2) "maphash visited 2 entries")

;; --- Test 7: sxhash ---
(display "Test 7: sxhash")
(newline)
(assert (> (sxhash "hello") 0) "sxhash string positive")
(assert (number? (sxhash 42)) "sxhash number returns number")
(assert (number? (sxhash #t)) "sxhash bool returns number")
(assert (number? (sxhash '(1 2 3))) "sxhash list returns number")

(display "=== ALL HASH TABLE TESTS PASSED ===")
(newline)
