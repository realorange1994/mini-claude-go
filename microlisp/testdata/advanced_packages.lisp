;; ============================================================
;; Advanced Test: Package System
;;
;; Tests:
;;   1. Package creation and basic properties
;;   2. Interning symbols and export
;;   3. Keyword package and keywordp
;;   4. find-symbol and symbol-package
;;   5. Import and unintern
;;   6. Shadow and shadowing-import
;; ============================================================

(load "tests/framework.lisp")

(start-suite "Package Creation")

;; make-package creates a new package and returns a package object
(define p (make-package "MY-PACKAGE"))
(assert-true (package? p) "make-package: returns package")
(assert-equal "MY-PACKAGE" (package-name p) "package-name: MY-PACKAGE")

;; find-package finds an existing package
(define p2 (find-package "MY-PACKAGE"))
(assert-true (package? p2) "find-package: returns package")
(assert-equal "MY-PACKAGE" (package-name p2) "find-package: found MY-PACKAGE")

;; find-package returns nil for nonexistent package
(assert-nil (find-package "NONEXISTENT") "find-package: nil for nonexistent")

;; make-package on existing name returns existing
(define p3 (make-package "MY-PACKAGE"))
(assert-equal "MY-PACKAGE" (package-name p3) "make-package: existing returns existing")

(end-suite)

(start-suite "Intern and Export")

;; in-package switches current package
(define pkg (make-package "TEST-PKG"))
(in-package "TEST-PKG")

;; intern a symbol
(define sym (intern "my-var"))
(assert-true (symbol? sym) "intern: returns symbol")
(assert-equal 'my-var sym "intern: my-var")

;; export a symbol
(export 'my-var)
(assert-true (symbol? (export 'my-var)) "export: returns symbol")

;; Switch back to USER package
(in-package "USER")

(end-suite)

(start-suite "Keywords")

;; keywordp
(assert-true (keywordp :foo) "keywordp: :foo is keyword")
(assert-true (keywordp :bar) "keywordp: :bar is keyword")
(assert-false (keywordp 'foo) "keywordp: foo is not keyword")
(assert-false (keywordp 42) "keywordp: number is not keyword")
(assert-false (keywordp "foo") "keywordp: string is not keyword")

;; Keywords are self-evaluating
(assert-equal :foo :foo ":foo evaluates to :foo")
(assert-equal :hello :hello ":hello evaluates to :hello")

;; Keywords can be compared with eq?
(assert-true (eq? :foo :foo) "eq?: :foo eq :foo")
(assert-true (eq? :bar :bar) "eq?: :bar eq :bar")
(assert-false (eq? :foo :bar) "eq?: :foo not eq :bar")

;; keywordp with keyword
(assert-true (keywordp :test-keyword) "keywordp: :test-keyword")

(end-suite)

(start-suite "find-symbol")

;; find-symbol in a specific package
(define test-pkg (find-package "TEST-PKG"))
(assert-true (package? test-pkg) "find-package: TEST-PKG found")

;; find-symbol for existing symbol (by string name, specific package)
(define found (find-symbol "my-var" test-pkg))
(assert-true (symbol? found) "find-symbol: my-var found in TEST-PKG")
(assert-equal 'my-var found "find-symbol: correct symbol")

;; find-symbol for nonexistent symbol returns nil
(assert-nil (find-symbol "nonexistent-var") "find-symbol: nil for nonexistent")

(end-suite)

(start-suite "Import")

;; Create a new package for import testing
(define import-pkg (make-package "IMPORT-TEST"))
(in-package "IMPORT-TEST")

;; Export a symbol
(export (intern "imported-sym"))

;; Import into USER
(in-package "USER")

;; import the symbol into current package
(import 'imported-sym)

;; find it
(assert-true (symbol? (find-symbol "imported-sym")) "import: symbol available in USER")

(end-suite)

(start-suite "Unintern")

(define unintern-pkg (make-package "UNINTERN-TEST"))
(in-package "UNINTERN-TEST")

(intern "to-be-removed")
(assert-true (symbol? (find-symbol "to-be-removed")) "unintern: symbol exists before removal")

;; unintern removes from package
(unintern 'to-be-removed)
(assert-nil (find-symbol "to-be-removed") "unintern: symbol removed")

(in-package "USER")

(end-suite)

(start-suite "list-all-packages")

(define all-pkgs (list-all-packages))
(assert-true (pair? all-pkgs) "list-all-packages: returns a list")

;; Should contain at least KEYWORD, USER, MY-PACKAGE, etc.
(define pkg-list (list-all-packages))

(define (list-contains-pkg? lst name)
  (cond
    ((null? lst) #f)
    ((and (package? (car lst)) (equal? (package-name (car lst)) name)) #t)
    (else (list-contains-pkg? (cdr lst) name))))

(assert-true (list-contains-pkg? pkg-list "KEYWORD") "list-all-packages: contains KEYWORD")
(assert-true (list-contains-pkg? pkg-list "USER") "list-all-packages: contains USER")
(assert-true (list-contains-pkg? pkg-list "MY-PACKAGE") "list-all-packages: contains MY-PACKAGE")
(assert-true (list-contains-pkg? pkg-list "TEST-PKG") "list-all-packages: contains TEST-PKG")

(end-suite)

(start-suite "Shadow")

(define shadow-pkg (make-package "SHADOW-TEST"))
(in-package "SHADOW-TEST")

;; Intern a symbol
(intern "original")

;; Shadow it - should succeed
(shadow 'original)
(assert-true (symbol? (find-symbol "original")) "shadow: symbol still present after shadow")

(in-package "USER")

(end-suite)

(start-suite "Shadowing-Import")

(define si-pkg (make-package "SI-TEST"))
(in-package "SI-TEST")

;; Export a symbol for shadowing-import
(export (intern "si-sym"))

;; Switch back and do shadowing-import
(in-package "USER")
(shadowing-import 'si-sym)
(assert-true (symbol? (find-symbol "si-sym")) "shadowing-import: symbol available")

(end-suite)

(start-suite "symbol-package")

;; Keywords are in KEYWORD package
(define kw-pkg (symbol-package :foo))
(assert-true (package? kw-pkg) "symbol-package: :foo returns package")
(assert-equal "KEYWORD" (package-name kw-pkg) "symbol-package: :foo in KEYWORD")

(define kw-pkg2 (symbol-package :bar))
(assert-true (package? kw-pkg2) "symbol-package: :bar returns package")

;; Non-keyword symbol may have a package
(define testp (find-package "TEST-PKG"))
(assert-true (package? testp) "find-package: TEST-PKG for symbol-package test")

(end-suite)

(start-suite "package-use-list")

;; USER should have CL in use-list (or at least return a list)
(define use-list (package-use-list (find-package "USER")))
(assert-true (or (null? use-list) (pair? use-list)) "package-use-list: returns list")

(end-suite)

(test-summary)
