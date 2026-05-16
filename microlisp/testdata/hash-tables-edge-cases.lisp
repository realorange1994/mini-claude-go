;; ============================================================
;; Hash Table Edge Cases Tests - derived from SBCL hash*.pure.lisp
;; ============================================================

(load "tests/framework.lisp")

;; ============================================================
;; Basic Hash Table Operations
;; ============================================================
(start-suite "hash-table-basics")

;; Create hash table and basic operations
(define ht (make-hash-table))
(assert-true (hash-table? ht) "make-hash-table returns hash-table")

;; setf/gethash
(setf (gethash 'a ht) 1)
(setf (gethash 'b ht) 2)
(setf (gethash 'c ht) 3)

(assert-equal 1 (gethash 'a ht) "gethash: finds 'a")
(assert-equal 2 (gethash 'b ht) "gethash: finds 'b")
(assert-nil (gethash 'd ht) "gethash: not found returns nil")

;; Update existing
(setf (gethash 'a ht) 100)
(assert-equal 100 (gethash 'a ht) "gethash: update value")

(end-suite)

;; ============================================================
;; Hash Table Count and Size
;; ============================================================
(start-suite "hash-table-count-size")

(define ht (make-hash-table))
(assert-equal 0 (hash-table-count ht) "hash-table-count: empty")

(setf (gethash 'a ht) 1)
(assert-equal 1 (hash-table-count ht) "hash-table-count: after 1 insert")

(setf (gethash 'b ht) 2)
(setf (gethash 'c ht) 3)
(assert-equal 3 (hash-table-count ht) "hash-table-count: after 3 inserts")

;; Remove decreases count
(remhash 'b ht)
(assert-equal 2 (hash-table-count ht) "hash-table-count: after remove")

;; Clear
(clrhash ht)
(assert-equal 0 (hash-table-count ht) "hash-table-count: after clear")

(end-suite)

;; ============================================================
;; REMHASH Edge Cases
;; ============================================================
(start-suite "remhash-edge")

(define ht (make-hash-table))
(setf (gethash 'a ht) 1)
(setf (gethash 'b ht) 2)

(assert-true (remhash 'a ht) "remhash: returns true when key exists")
(assert-nil (remhash 'a ht) "remhash: returns nil when key doesn't exist")
(assert-nil (gethash 'a ht) "remhash: key no longer present")

(end-suite)

;; ============================================================
;; CLRHASH Edge Cases
;; ============================================================
(start-suite "clrhash-edge")

(define ht (make-hash-table))
(setf (gethash 'a ht) 1)
(setf (gethash 'b ht) 2)

(clrhash ht)
(assert-equal 0 (hash-table-count ht) "clrhash: count is 0")
(assert-nil (gethash 'a ht) "clrhash: a no longer exists")
(assert-nil (gethash 'b ht) "clrhash: b no longer exists")

(end-suite)

;; ============================================================
;; WITH-HASH-TABLE-ITERATOR Edge Cases
;; ============================================================
(start-suite "hash-iteration-edge")

(define ht (make-hash-table))
(setf (gethash 'a ht) 1)
(setf (gethash 'b ht) 2)
(setf (gethash 'c ht) 3)

;; Simple iteration - collect all keys and values
(define collected '())
(maphash (lambda (k v)
           (set! collected (cons (list k v) collected)))
         ht)
(set! collected (reverse collected))

;; Check we got all pairs
(assert-equal 3 (length collected) "maphash: collected 3 pairs")

(end-suite)

;; ============================================================
;; Hash Table Rehash - simplified test
;; ============================================================
(start-suite "hash-rehash-simple")

;; Insert multiple entries
(define ht (make-hash-table :size 10))
(setf (gethash 'a ht) 1)
(setf (gethash 'b ht) 2)
(setf (gethash 'c ht) 3)

(assert-equal 3 (hash-table-count ht) "hash-rehash: 3 entries")

(end-suite)

;; ============================================================
;; SXHASH Tests
;; ============================================================
(start-suite "sxhash-basics")

;; sxhash basic - should return an integer
(assert-true (number? (sxhash 'a)) "sxhash: returns number")
(assert-true (number? (sxhash 42)) "sxhash: on number")
(assert-true (number? (sxhash "hello")) "sxhash: on string")

;; Equal things have equal hashes
(assert-equal (sxhash 'abc) (sxhash 'abc) "sxhash: same symbol same hash")

(end-suite)

;; ============================================================
;; Summary
;; ============================================================
(test-summary)
