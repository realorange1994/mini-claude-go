;; ============================================================
;; Hash Table Edge Cases - derived from SBCL hash.pure.lisp
;; ============================================================

(load "tests/framework.lisp")

;; ============================================================
;; Hash Table Basic Operations
;; ============================================================
(start-suite "hash-table-basics")

(define ht (make-hash-table))
(assert-true (hash-table? ht) "make-hash-table: returns hash table")
(assert-equal 0 (hash-table-count ht) "hash-table-count: empty table")

;; setf/getf
(setf (gethash 'a ht) 1)
(setf (gethash 'b ht) 2)
(setf (gethash 'c ht) 3)
(assert-equal 3 (hash-table-count ht) "hash-table-count: after adds")
(assert-equal 1 (gethash 'a ht) "gethash: finds a")
(assert-equal 2 (gethash 'b ht) "gethash: finds b")
(assert-equal 3 (gethash 'c ht) "gethash: finds c")

(end-suite)

;; ============================================================
;; Hash Table :test Key
;; ============================================================
(start-suite "hash-table-test-key")

;; Default test is eql
(define ht-eq (make-hash-table))
(setf (gethash 1 ht-eq) 'one)
(setf (gethash 'a ht-eq) 'symbol)
(assert-equal 'one (gethash 1 ht-eq) "gethash: integer key with eql")
(assert-equal 'symbol (gethash 'a ht-eq) "gethash: symbol key with eql")

(end-suite)

;; ============================================================
;; Hash Table Missing Keys
;; ============================================================
(start-suite "hash-table-missing-keys")

(define ht (make-hash-table))
(setf (gethash 'a ht) 1)

;; Missing key returns nil (or default value)
(assert-nil (gethash 'nonexistent ht) "gethash: missing key returns nil")
(assert-equal 'default (gethash 'nonexistent ht 'default) "gethash: missing key with default")
(assert-equal 99 (gethash 'missing ht 99) "gethash: missing key numeric default")

;; Check if key exists
(assert-true (hash-table-exists? ht 'a) "hash-table-exists?: key exists")
(assert-false (hash-table-exists? ht 'z) "hash-table-exists?: key doesn't exist")

(end-suite)

;; ============================================================
;; Hash Table Update and Remove Operations
;; ============================================================
(start-suite "hash-table-update")

(define ht (make-hash-table))

;; Increment a counter
(setf (gethash 'counter ht) 0)
(setf (gethash 'counter ht) (+ (gethash 'counter ht) 1))
(assert-equal 1 (gethash 'counter ht) "gethash: increment counter")

;; Remove hash entry
(setf (gethash 'to-remove ht) 'value)
(assert-true (hash-table-exists? ht 'to-remove) "before remhash: exists")
(remhash 'to-remove ht)
(assert-false (hash-table-exists? ht 'to-remove) "after remhash: doesn't exist")

;; Clear hash table
(clrhash ht)
(assert-equal 0 (hash-table-count ht) "clrhash: clears all entries")

(end-suite)

;; ============================================================
;; Hash Table Iteration
;; ============================================================
(start-suite "hash-table-iteration")

(define ht (make-hash-table))
(setf (gethash 'a ht) 1)
(setf (gethash 'b ht) 2)
(setf (gethash 'c ht) 3)

;; maphash
(define results '())
(maphash (lambda (k v)
           (set! results (cons (list k v) results)))
        ht)
(assert-equal 3 (length results) "maphash: iterates all pairs")

(end-suite)

;; ============================================================
;; Hash Table Edge Cases
;; ============================================================
(start-suite "hash-table-edge-cases")

;; Empty hash table
(define empty-ht (make-hash-table))
(assert-equal 0 (hash-table-count empty-ht) "empty hash-table-count")
(assert-nil (gethash 'anything empty-ht) "empty gethash returns nil")

;; Hash table with many entries
(define big-ht (make-hash-table))
(define (add_entries n)
  (if (> n 0)
      (begin
        (setf (gethash n big-ht) (* n n))
        (add_entries (- n 1)))))
(add_entries 100)
(assert-equal 100 (hash-table-count big-ht) "big hash-table: count")
(assert-equal 10000 (gethash 100 big-ht) "big hash-table: last entry")
(assert-equal 1 (gethash 1 big-ht) "big hash-table: first entry")

;; Re-adding same key updates value
(setf (gethash 'same ht 'initial) 'first)
(setf (gethash 'same ht 'initial) 'second)
(assert-equal 'second (gethash 'same ht 'initial) "hash-table: updates existing key")

(end-suite)

;; ============================================================
;; Hash Table with Different Key Types
;; ============================================================
(start-suite "hash-table-key-types")

(define ht (make-hash-table))

;; Symbol keys
(setf (gethash 'foo ht) 'symbol-value)
(assert-equal 'symbol-value (gethash 'foo ht) "symbol key")

;; String keys (if test is string=)
(define ht-string (make-hash-table :test 'string=))
(setf (gethash "hello" ht-string) 'string-value)
(assert-equal 'string-value (gethash "hello" ht-string) "string key")

;; Integer keys
(setf (gethash 42 ht) 'int-value)
(assert-equal 'int-value (gethash 42 ht) "integer key")

(end-suite)

;; ============================================================
;; SXHASH Tests
;; ============================================================
(start-suite "sxhash-basics")

(assert-true (integerp (sxhash 'a)) "sxhash: returns integer")
(assert-true (integerp (sxhash "test")) "sxhash: string returns integer")
(assert-true (integerp (sxhash '(1 2 3))) "sxhash: list returns integer")

;; Same values should have same hash
(assert-equal (sxhash 'x) (sxhash 'x) "sxhash: same symbol same hash")
(assert-equal (sxhash "hello") (sxhash "hello") "sxhash: same string same hash")

;; Different values should (usually) have different hashes
(assert-true (/= (sxhash "foo") (sxhash "bar")) "sxhash: different strings different hash")

(end-suite)

;; ============================================================
;; Summary
;; ============================================================
(test-summary)
