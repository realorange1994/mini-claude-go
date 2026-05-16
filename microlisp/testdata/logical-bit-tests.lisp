;; logical-bit-tests.lisp — tests for bitwise/logical operations
;; Covers: logand, logior, logxor, lognot, logtest, logbitp, logcount, ash

(load "tests/framework.lisp")
(start-suite "Logical and Bit Operations")

;; --- logand ---
(assert-equal 7 (logand 15 7) "logand 15 7 = 7")
(assert-equal 0 (logand 15 16) "logand 15 16 = 0")
(assert-equal 6 (logand 14 7) "logand 14 7 = 6")
(assert-equal -1 (logand -1 -1) "logand -1 -1 = -1")
(assert-equal 0 (logand 0 255) "logand 0 255 = 0")
(assert-equal 255 (logand 255 -1) "logand 255 -1 = 255")
(assert-equal 5 (logand 13 7) "logand 13 7 = 5")
(assert-equal 0 (logand 8 7) "logand 8 7 = 0")
(assert-equal 255 (logand 255 255) "logand 255 255 = 255")

;; --- logior ---
(assert-equal 15 (logior 8 7) "logior 8 7 = 15")
(assert-equal 15 (logior 15 7) "logior 15 7 = 15")
(assert-equal 0 (logior 0 0) "logior 0 0 = 0")
(assert-equal 255 (logior 15 240) "logior 15 240 = 255")
(assert-equal 31 (logior 16 15) "logior 16 15 = 31")
(assert-equal -1 (logior -1 0) "logior -1 0 = -1")

;; --- logxor ---
(assert-equal 15 (logxor 8 7) "logxor 8 7 = 15")
(assert-equal 0 (logxor 7 7) "logxor 7 7 = 0")
(assert-equal 8 (logxor 15 7) "logxor 15 7 = 8")
(assert-equal 255 (logxor 0 255) "logxor 0 255 = 255")
(assert-equal 0 (logxor 255 255) "logxor 255 255 = 0")
(assert-equal 240 (logxor 255 15) "logxor 255 15 = 240")

;; --- lognot ---
(assert-equal -1 (lognot 0) "lognot 0 = -1")
(assert-equal 0 (lognot -1) "lognot -1 = 0")
(assert-equal -16 (lognot 15) "lognot 15 = -16")
(assert-equal -256 (lognot 255) "lognot 255 = -256")
(assert-equal 15 (lognot -16) "lognot -16 = 15")
(assert-equal -6 (lognot 5) "lognot 5 = -6")

;; --- logtest ---
(assert-true (logtest 7 15) "logtest 7 15 = true (bits in common)")
(assert-false (logtest 8 7) "logtest 8 7 = false (no bits in common)")
(assert-true (logtest 1 1) "logtest 1 1 = true")
(assert-false (logtest 0 0) "logtest 0 0 = false")
(assert-true (logtest -1 1) "logtest -1 1 = true")
(assert-true (logtest 255 128) "logtest 255 128 = true")
(assert-true (logtest 255 255) "logtest 255 255 = true")
(assert-false (logtest 1 0) "logtest 1 0 = false")

;; --- logbitp ---
(assert-true (logbitp 0 1) "logbitp 0 1 = true (bit 0 set)")
(assert-false (logbitp 1 1) "logbitp 1 1 = false (bit 1 not set)")
(assert-true (logbitp 2 4) "logbitp 2 4 = true (bit 2 set)")
(assert-true (logbitp 3 8) "logbitp 3 8 = true (bit 3 set)")
(assert-false (logbitp 3 7) "logbitp 3 7 = false (bit 3 not set)")
(assert-true (logbitp 7 255) "logbitp 7 255 = true")
(assert-false (logbitp 8 255) "logbitp 8 255 = false")

;; --- logcount ---
(assert-equal 0 (logcount 0) "logcount 0 = 0")
(assert-equal 1 (logcount 1) "logcount 1 = 1")
(assert-equal 1 (logcount 2) "logcount 2 = 1")
(assert-equal 2 (logcount 3) "logcount 3 = 2")
(assert-equal 8 (logcount 255) "logcount 255 = 8")
(assert-equal 4 (logcount 15) "logcount 15 = 4")
(assert-equal 1 (logcount 16) "logcount 16 = 1")
(assert-equal 2 (logcount 6) "logcount 6 = 2")

;; --- ash (arithmetic shift) ---
(assert-equal 4 (ash 1 2) "ash 1 2 = 4 (left shift)")
(assert-equal 8 (ash 2 2) "ash 2 2 = 8")
(assert-equal 1 (ash 4 -2) "ash 4 -2 = 1 (right shift)")
(assert-equal 0 (ash 1 -1) "ash 1 -1 = 0 (right shift truncates)")
(assert-equal 20 (ash 5 2) "ash 5 2 = 20")
(assert-equal 2 (ash 10 -2) "ash 10 -2 = 2")
(assert-equal 0 (ash 0 5) "ash 0 5 = 0")
(assert-equal 256 (ash 1 8) "ash 1 8 = 256")
(assert-equal 128 (ash 32 2) "ash 32 2 = 128")

(end-suite)
(test-summary)
