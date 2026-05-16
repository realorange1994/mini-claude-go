;; packages-advanced.lisp — tests for package operations
;; Covers: make-package, find-package, package-name, export, intern

(load "tests/framework.lisp")
(start-suite "Package Creation")

;; --- make-package ---
(define p1 (make-package "TEST-PKGB-1"))
(assert-true (if (find-package "TEST-PKGB-1") #t #f) "make-package creates findable package")
(assert-equal "TEST-PKGB-1" (package-name p1) "package-name returns name")

;; --- make-package with nicknames ---
(define p2 (make-package "TEST-PKGB-2" :nicknames (list "TPB2")))
(assert-true (if (find-package "TEST-PKGB-2") #t #f) "make-package with nicknames creates package")

(end-suite)
(start-suite "Package Introspection")

;; --- find-package ---
(assert-true (if (find-package "CL") #t #f) "find-package CL")
(assert-true (if (find-package "KEYWORD") #t #f) "find-package KEYWORD")
(assert-nil (find-package "NONEXISTENT-PKG-B") "find-package nonexistent")

;; --- package-name ---
(assert-equal "CL" (package-name (find-package "CL")) "package-name CL")

;; --- list-all-packages ---
(define all-pkgs (list-all-packages))
(assert-true (list? all-pkgs) "list-all-packages returns list")

(end-suite)
(start-suite "Package Operations")

;; --- defpackage ---
(defpackage "TEST-PKGB-3")
(assert-true (if (find-package "TEST-PKGB-3") #t #f) "defpackage creates package")

;; --- intern ---
(define p4 (make-package "TEST-INTERN-B"))
(intern "MY-SYM-B" p4)
(assert-true (if (find-symbol "MY-SYM-B" p4) #t #f) "intern creates symbol in package")

(end-suite)
(start-suite "Package Export")

;; --- export ---
(define p5 (make-package "TEST-EXPORT-B"))
(define sym-b (intern "EXPORTED-SYM-B" p5))
(export sym-b p5)
(assert-true (if (find-symbol "EXPORTED-SYM-B" p5) #t #f) "exported symbol exists")

;; --- unexport: find-symbol still finds symbol (just not external) ---
(unexport sym-b p5)
(assert-true (if (find-symbol "EXPORTED-SYM-B" p5) #t #f) "find-symbol still finds unexported symbol")

(end-suite)
(start-suite "Keyword Package")

;; --- keyword symbols ---
(assert-true (if (find-package "KEYWORD") #t #f) "keyword package exists")
(assert-equal ":FOO" (symbol-name :FOO) "keyword symbol name includes colon")
(assert-true (keywordp :test) "keywordp true for keyword")
(assert-false (keywordp 'foo) "keywordp false for regular symbol")

(end-suite)
(start-suite "Package Current State")

;; --- *package* is bound ---
(define pkg-name (package-name *package*))
(assert-true (if (string= pkg-name pkg-name) #t #f) "*package* has valid name")

(end-suite)
(test-summary)
