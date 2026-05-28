package microlisp

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// helpTopicMap stores rich documentation for Lisp help topics that the agent
// commonly requests via (help "topic"). This is distinct from builtinDocMap
// which documents individual functions.
var helpTopicMap = map[string]string{
	"ffi": `Go FFI Interop (go:* functions):
  Getting Go values:
    (go:import "pkg.Func")  Import Go function → returns PRIMITIVE, call to get go-val
    (ffi "pkg.Func")        Alias for go:import (same thing)
    ((ffi "time.Now"))      Call the primitive → returns go-val (or use funcall)
  Important: ffi/go:import return a PRIMITIVE, not a go-val directly.
    You MUST call them to get the actual go-val before using go:call.
  Listing:
    (go:list) / (go:list "pkg")  List packages/symbols
    (go:register "pkg.name" go-val)  Register custom Go value (go-val only)
  Structs:
    (go:new "pkg.Type")  Create zero-value struct → returns go-val
    (go:field obj "Field")  Read field (exported fields only)
    (go:set-field obj "Field" val)  Set field (exported fields only)
    (go:fields-of "pkg.Type")  List struct fields
    (go:methods-of obj)  List methods
  Calling methods on go-val:
    (go:call obj "MethodName" args...)  Call Go method on a go-val
    obj MUST be a go-val (from go:new, go:make, or calling an imported function)
  Examples:
    ;; Get current time
    (go:call ((ffi "time.Now")) "Format" "2006-01-02 15:04:05")
    ;; Same result
    (go:call ((go:import "time.Now")) "Format" "2006-01-02 15:04:05")
    ;; Create struct and call method
    (go:call (go:new "bytes.Buffer") "String")
  Reflection:
    (go:type-of obj)  Get Go type name
    (go:is-nil obj)  Check if Go nil
    (go:is-zero obj)  Check if zero value
    (go:assert-type obj "io.Reader")  Type assertion
    (go:implements obj "io.Reader")  Check interface
    (go:kind-of obj) / (go:kind-of "string")  reflect.Kind
    (go:elem-of type)  Element type
    (go:len-of obj) / (go:cap-of obj)  Length/capacity
    (go:func-of obj "Method")  Get method as callable
    (go:type-parse "[]int")  Parse type string
    (go:uintptr obj)  Pointer address (for ptr types)
    (go:convert val "[]byte")  Go type conversion
  Creation:
    (go:make "[]int" 10)  Create slice (default cap=8)
    (go:make "[]int" 10 20)  Create slice with size+cap
    (go:make "map[string]int")  Create map
    (go:make "chan int" 5)  Create buffered channel
    (go:make "[10]int")  Create array
  Callbacks:
    (go:callback fn "int32->bool")  Create Go callback
    (go:callback fn "string->string")  etc.
    Signatures: "int32->bool", "int32->int32", "int->int", "int->bool",
      "int,int->bool", "int,int->", "string->string", "string->error", "()->"
  VGoVal: Opaque Go values preserving type info for reflection.`,

	"ops": `=== lisp_eval Operations Guide ===

--- Getting Started ---
1. Evaluate:  expression="(+ 1 2)"
2. Define:    expression="(defun square (x) (* x x))"
3. Call:      expression="(square 7)"

--- Need Something Specific? ---
- Function signature?  → operation=define, expression="car"
- Function reference?  → (help "arithmetic") or similar topic
- Code examples?       → operation=examples, expression="lists"
- Go interop/FFI?      → (help "ffi")
- Concurrency guide?   → (help "concurrency")
- View source code?    → operation=source, expression="car"
- Explore call graph?  → operation=xref, expression="filter"

--- All Operations ---
  define — function signature: params, return types, usage
  eval (default) — evaluate Lisp expression; state persists
  reset — clear all interpreter state
  help — topic reference (use (help "topic") in Lisp)
  examples — code examples
  eval_file — run .lisp file
  lint — check syntax
  source — view function source
  source-list — browse indexed functions
  xref — callers/callees
  xref-list — browse all xrefs

--- Resource Limits ---
  "default"  — 1M steps, 30s, 256MB (normal use)
  "strict"   — 100K steps, 10s, 64MB (untrusted code)
  "unlimited" — no limits (REPL mode only)`,

	"xref": `=== Cross-Reference (xref) Guide ===

The xref feature lets you explore the call graph of the Lisp interpreter.
It answers two questions:
  1. Who calls X? (callers) — which functions reference or invoke function X
  2. What does X call? (callees) — which functions does X invoke or reference

--- How to Use ---
operation=xref with expression=<function-name> (plain name, not Lisp syntax)

Examples:
  operation=xref, expression="car"
    => Shows all functions that call car (callers), and what car calls (callees)

  operation=xref, expression="filter"
    => Shows callers of filter AND callees of filter

--- xref-list: Browse All Relationships ---
  operation=xref-list, expression=""
    => Show all caller->callee pairs (paginated)

--- Understanding the Output ---
Callers section: "filter (stdlib, stdlib.go:45)"
Callees section: "if (special, conditions.go:12-45)"
Call sites section shows exact source line where the call happens`,

	"concurrency": `=== Concurrency Guide ===

Microlisp provides CSP-style concurrency via Go channels, threads, and locks.

--- Channels ---
Create:
  (make-channel)              => unbuffered channel
  (make-channel 10)           => buffered channel (capacity 10)

Send and Receive:
  (chan-send ch 42)           => send (returns NIL, or (:would-block "...") if no receiver)
  (chan-recv ch)              => receive (returns value or NIL if closed, or (:would-block "..."))
  (chan-try-send ch "hi")     => non-blocking: (:ok), :would-block, or :closed
  (chan-try-recv ch)          => non-blocking: (:ok value), :would-block, or :closed

Close and Inspect:
  (chan-close ch)             => close channel (no more sends)
  (chan-p val)                => T if val is a channel
  (chan-info ch)              => (:buffer N :closed T/NIL)

Select — choose among multiple channel operations:
  (go:select
    (:recv ch1)               => returns (index value) if ch1 has data
    (:send ch2 val)           => returns (index NIL) if send succeeds
    (:default))               => returns immediately if no other case ready

Select with timeout:
  (chan-select-timeout 5000   => 5 second timeout
    (:recv ch))               => returns (0 value) or (:timeout)

--- Threads ---
  (go:spawn (lambda () body...))  => spawn thread, returns thread-id
  (make-thread (lambda () body...))  => create thread, returns thread-id
  (join-thread thread-id)     => wait for thread, returns result or (:would-block "...")
  (thread? val)               => T if val is a thread

--- Locks ---
  (make-lock)                 => create mutex
  (lock l)                    => acquire mutex (returns :would-block if held too long)
  (unlock l)                  => release mutex
  (lock? val)                 => T if val is a lock

--- Condition Variables ---
  (make-condvar)              => create condition variable
  (condition-wait cv lock)    => wait for signal (atomically releases lock)
  (condition-notify cv)       => wake one waiter
  (condition-broadcast cv)    => wake all waiters

--- Atomic Operations ---
  (atomic-incf place)         => atomically increment
  (atomic-decf place)         => atomically decrement
  (atomic-get place)          => atomic read
  (atomic-set place val)      => atomic write

--- Special Forms ---
  (go:recover body...)        => catch Go panics inside body
  (go:with-defer (defer-form) body...)  => execute defer-form when body exits`,

	"arithmetic": `Arithmetic Operations:
  (+ a b c...)     Addition (arbitrary precision integers, rationals, floats, complex)
  (- a b)          Subtraction; (- a) negates
  (* a b c...)     Multiplication
  (/ a b)          Division (returns rational if no remainder, float if remainder)
  (mod n m)        Modulo
  (expt b e)       Exponentiation: b^e
  (sqrt n)         Square root
  (abs n)          Absolute value
  (sin/cos/tan n)  Trigonometric functions
  (floor n)        Floor division
  (ceiling n)      Ceiling
  (round n)        Round to nearest integer
  (max a b...)     Maximum
  (min a b...)     Minimum
  (gcd a b...)     Greatest common divisor
  (lcm a b...)     Least common multiple
  (random n)       Random number [0, n)
  (= a b)          Numeric equality
  (/= a b)         Not equal
  (< a b) / (> a b)  Less than / Greater than

Bit Operations:
  (logand a b) / (logior a b) / (logxor a b) / (lognot a)
  (ash n count)  Arithmetic shift

Complex Numbers:
  (complex r i)  Create complex number
  (realpart c) / (imagpart c)  Extract parts`,

	"lists": `List Operations:
  (cons a b)       Create a pair/cons cell
  (car x)          First element of a cons/list
  (cdr x)          Rest of a cons/list
  (list a b c...)  Create a list from arguments
  (append l1 l2)   Concatenate lists
  (length l)       Length of list/string
  (reverse l)      Reverse a list
  (nth n l)        Nth element (0-indexed)
  (member x l)     Check if x is a member of l
  (assoc x alist)  Association list lookup
  (push x lst)     Push element onto list
  (pop lst)        Pop element from list
  (union/intersection/set-difference)  Set operations`,

	"strings": `String Operations:
  (string x)        Convert to string
  (string-append a b...)  Concatenate strings
  (string-length s)  Length of string
  (string-find s ch)  Find character in string
  (substring s start [end])  Extract substring
  (string->number s)  Parse string to number
  (number->string n)  Convert number to string
  (char s i)        Get character at index
  (string-equal a b)  Case-insensitive string equality
  (string-trim / string-left-trim / string-right-trim chars string)`,

	"hash": `Hash Tables:
  (make-hash-table &key test size)  Create hash table
  (gethash key ht)   Get value (returns value, found-p)
  (gethash key ht default)  Get value with default
  (setf (gethash key ht) val)  Set value
  (remhash key ht)   Remove entry
  (clrhash ht)       Clear hash table
  (maphash fn ht)    Apply function to each key-value pair
  Tests: :eql (default), :equal, :eq`,

	"arrays": `Arrays:
  (make-array dims &key initial-element)  Create array
  (aref array i...)  Access element
  (setf (aref array i...) val)  Set element
  (array-rank array)  Number of dimensions
  (array-dimension array n)  Size of dimension n
  (vector-push elem vector)  Push if fill-pointer < size
  (vector-push-extend elem vector)  Push with auto-extend
  (vector-pop vector)  Pop from fill-pointer`,

	"clos": `CLOS (Common Lisp Object System):
  (defclass name (superclasses) (slot-specs...) options...)
  (make-instance 'class-name &key initargs...)
  (defgeneric fn-name (args...) &key method-combination)
  (defmethod fn-name qualifier ((arg class-name) ...) body...)
    Qualifiers: :before, :after, :around
  (slot-value obj 'slot-name)  Access slot
  (setf (slot-value obj 'slot) val)  Set slot
  (class-of obj)  Get class of object`,

	"format": `Format and Output:
  (format destination control-string args...)
  Destinations: nil (returns string), t (stdout), stream
  Control sequences:
    ~A  Aesthetic (print without quotes)
    ~S  Standard (print with quotes/escaping)
    ~D  Decimal integer
    ~F  Fixed-point float
    ~%  Newline
    ~~  Literal tilde
    ~{...~}  Iterate over list
  Example: (format nil "Hello, ~A!" "World")  => "Hello, World!"`,

	"macros": `Macros:
  (define-macro (name args) body...)  Define a macro
  (defmacro name (args) body...)  CL-style macro
  (backquote/quasiquote) with , (unquote) and ,@ (splice)
  (macroexpand-1 form)  Expand macro once
  (macroexpand form)    Fully expand macro`,

	"lambda": `Lambda and Closures:
  (lambda (args) body...)  Anonymous function
  (define (name args) body...)  Define a function
  (defun name (args) body...)   Define a function (CL style)
  (funcall fn args...)    Call a function
  (apply fn list)         Apply function to list of args
  (let ((var val)...) body...)  Lexical binding
  (let* ((var val)...) body...) Sequential let
  (if test then [else])  Conditional
  (cond (test1 body1) (test2 body2) ...)  Multi-way conditional`,

	"packages": `Package System:
  (make-package name &key nicknames use)  Create package
  (in-package name)  Switch to package
  (find-package name)  Find package by name/nickname
  (intern name &optional package)  Intern symbol
  (export symbols &optional package)  Export symbols
  (import symbols &optional package)
  (find-symbol name &optional package)  -> symbol, status`,

	"io": `I/O and Streams:
  (open path &key direction if-exists if-does-not-exist)
  (close stream &key abort)
  (read stream) / (read-line stream) / (read-char stream)
  (write object &key stream) / (print object stream)
  (read-from-string str)  Parse from string
  (format destination fmt args...)  Formatted output`,

	"conditions": `Conditions and Error Handling:
  (define-condition name (superclass) (slot-specs...))
  (error "message")  Signal an error (fatal)
  (cerror "continue-format" "error-format")  Continuable error
  (warn "message")   Signal a warning
  (handler-case form (condition-type (var) handler-body...))
  (restart-case form (restart-name () handler-body...))
  (ignore-errors form...)  Catch errors, return NIL+error`,
}

// builtinHelp provides Lisp-level help for builtins, FFI functions, and types.
// (help) — list all help categories
// (help "go:field") — help for a specific builtin/FFI
// (help "go:") — help for all go:* functions
// (help "crypto/x509") — help for an FFI package
// (help "ffi") — rich FFI guide
// (help "concurrency") — rich concurrency guide
// (help "ops") — operations guide
// (help "xref") — cross-reference guide
func builtinHelp(args []*Value) (*Value, error) {
	if len(args) == 0 || isNil(args[0]) {
		return vstr(helpAll()), nil
	}

	name := args[0].str
	if name == "" {
		return vstr(helpAll()), nil
	}

	// Check help topic map first (rich topic docs)
	if doc, ok := helpTopicMap[name]; ok {
		return vstr(doc), nil
	}

	// Check if it's a go:* function
	if strings.HasPrefix(name, "go:") {
		if doc, ok := goDocMap[name]; ok {
			return vstr(doc), nil
		}
		// List all go:* functions if name is just "go:" or partial
		if name == "go:" {
			return vstr(helpGoGroup()), nil
		}
	}

	// Check if it's a builtin
	if doc, ok := builtinDocMap[name]; ok {
		return vstr(doc), nil
	}

	// Check if it's an FFI package
	if pkg, ok := GoPackageRegistry[name]; ok {
		return vstr(helpPackage(name, pkg)), nil
	}

	// Check if it's an FFI type
	if pkg, ok := GoTypeRegistry[name]; ok {
		return vstr(helpTypes(name, pkg)), nil
	}

	// Check if it's a "package.Function" FFI function
	if parts := strings.SplitN(name, ".", 2); len(parts) == 2 {
		if pkg, ok := GoPackageRegistry[parts[0]]; ok {
			if _, ok := pkg[parts[1]]; ok {
				return vstr(helpFFIFunction(parts[0], parts[1])), nil
			}
		}
	}

	return nil, fmt.Errorf("help: no documentation for %q", name)
}

// builtinDocMap stores documentation for Lisp builtins.
var builtinDocMap = map[string]string{
	// Core
	"car":     "(car lst) — Return the first element of a cons cell or list.",
	"cdr":     "(cdr lst) — Return the rest of a cons cell or list.",
	"cons":    "(cons a b) — Create a new cons cell with a as car and b as cdr.",
	"list":    "(list a b c...) — Create a list from arguments.",
	"append":  "(append l1 l2) — Concatenate two or more lists.",
	"length":  "(length lst) — Return the length of a list or string.",
	"reverse": "(reverse lst) — Return a new list with elements in reverse order.",
	"nth":     "(nth n lst) — Return the nth element of a list (0-indexed).",
	"member":  "(member x lst) — Check if x is a member of lst. Returns tail or nil.",

	// Control flow
	"if":     "(if test then else) — Conditional. Evaluates then if test is true, else otherwise.",
	"cond":   "(cond (test1 body1) (test2 body2) ...) — Multi-way conditional.",
	"when":   "(when test body...) — Execute body if test is true.",
	"unless": "(unless test body...) — Execute body if test is false.",
	"and":    "(and a b c...) — Short-circuit logical AND.",
	"or":     "(or a b c...) — Short-circuit logical OR.",
	"not":    "(not x) — Logical negation.",

	// Functions
	"define":  "(define (name args) body...) — Define a function.",
	"defun":   "(defun name (args) body...) — Define a function (CL style).",
	"lambda":  "(lambda (args) body...) — Create an anonymous function.",
	"funcall": "(funcall fn args...) — Call a function with arguments.",
	"apply":   "(apply fn list) — Apply a function to a list of arguments.",

	// Binding
	"let":    "(let ((var val)...) body...) — Parallel lexical binding.",
	"let*":   "(let* ((var val)...) body...) — Sequential lexical binding.",
	"letrec": "(letrec ((var val)...) body...) — Recursive lexical binding.",

	// Arithmetic
	"+":       "(+ a b c...) — Addition.",
	"-":       "(- a b) — Subtraction; (- a) negates.",
	"*":       "(* a b c...) — Multiplication.",
	"/":       "(/ a b) — Division (returns rational if exact, float otherwise).",
	"mod":     "(mod n m) — Modulo operation.",
	"expt":    "(expt b e) — Exponentiation: b raised to e.",
	"sqrt":    "(sqrt n) — Square root.",
	"abs":     "(abs n) — Absolute value.",
	"floor":   "(floor n) — Floor (largest integer <= n).",
	"ceiling": "(ceiling n) — Ceiling (smallest integer >= n).",
	"round":   "(round n) — Round to nearest integer.",
	"max":     "(max a b...) — Return the maximum value.",
	"min":     "(min a b...) — Return the minimum value.",
	"random":  "(random n) — Return a random number in [0, n).",

	// Comparison
	"=":     "(= a b) — Numeric equality.",
	"/=":    "(/= a b) — Numeric inequality.",
	"<":     "(< a b) — Less than.",
	">":     "(> a b) — Greater than.",
	"<=":    "(<= a b) — Less than or equal.",
	">=":    "(>= a b) — Greater than or equal.",
	"eq":    "(eq a b) — Identity comparison (same object).",
	"eql":   "(eql a b) — Value comparison (same value).",
	"equal": "(equal a b) — Structural equality.",

	// Strings
	"string-append":  "(string-append a b...) — Concatenate strings.",
	"string-length":  "(string-length s) — Length of string.",
	"string-find":    "(string-find s ch) — Find character position in string.",
	"substring":      "(substring s start end) — Extract substring.",
	"string->number": "(string->number s) — Parse string to number.",
	"number->string": "(number->string n) — Convert number to string.",

	// Hash tables
	"make-hash-table": "(make-hash-table) — Create a new hash table.",
	"gethash":         "(gethash key ht [default]) — Look up value in hash table.",
	"setf":            "(setf place value) — Set a place to a value.",
	"remhash":         "(remhash key ht) — Remove entry from hash table.",
	"clrhash":         "(clrhash ht) — Clear all entries from hash table.",
	"maphash":         "(maphash fn ht) — Apply fn to each key-value pair.",

	// Format
	"format": `(format dest fmt args...) — Format output.
  dest: nil (return string), t (stdout), or stream.
  directives: ~A (aesthetic), ~S (standard), ~D (decimal),
              ~F (float), ~% (newline), ~~ (tilde), ~{~} (loop).`,

	// FFI
	"ffi":          `(ffi "pkg.Func") — Import a Go function from stdlib.`,
	"ffi-register": `(ffi-register "name" value) — Register a custom Go value.`,

	// Type predicates
	"null":        "(null x) — Check if x is nil.",
	"listp":       "(listp x) — Check if x is a list.",
	"consp":       "(consp x) — Check if x is a cons cell.",
	"symbolp":     "(symbolp x) — Check if x is a symbol.",
	"stringp":     "(stringp x) — Check if x is a string.",
	"numberp":     "(numberp x) — Check if x is a number.",
	"characterp":  "(characterp x) — Check if x is a character.",
	"hash-tablep": "(hash-tablep x) — Check if x is a hash table.",

	// Arrays
	"make-array": "(make-array dims) — Create an array. dims: int (1D) or list (multi-D).",
	"aref":       "(aref arr i...) — Access array element.",

	// CLOS
	"defclass":      "(defclass name (parents) (slots)) — Define a class.",
	"defgeneric":    "(defgeneric name (args)) — Define a generic function.",
	"defmethod":     "(defmethod name ((arg type)) body...) — Define a method.",
	"make-instance": "(make-instance 'class key val...) — Create an instance.",

	// Conditions
	"define-condition": "(define-condition name (parents) (slots)) — Define a condition.",
	"handler-case":     "(handler-case expr (type (var) handler...)) — Handle conditions.",
	"restart-case":     "(restart-case expr (name () handler...)) — Define restarts.",
	"error":            "(error fmt args...) — Signal an error.",
	"warn":             "(warn fmt args...) — Signal a warning.",
	"signal":           "(signal condition) — Signal a condition.",
}

// goDocMap stores documentation for go:* builtins.
var goDocMap = map[string]string{
	"go:import": `(go:import "pkg.Func"|"pkg.Var") — Import a Go function or variable from stdlib.
  Returns a callable function or a VGoVal (for interface/pointer variables).
  Example: (go:import "math.Sin") => function
           (go:import "crypto/rand.Reader") => #<go-val *rand.reader>
           (go:import "time.Time.AddDate") => method expression`,

	"go:list": `(go:list ["package"]) — List all FFI packages or symbols in a package.
  Example: (go:list) => ("math" "strings" "fmt" ...)
           (go:list "math") => ("math.Abs" "math.Sin" ...)`,

	"go:register": `(go:register "name" value) — Register a custom Go value for FFI access.
  Supports numbers, strings, and booleans.`,

	"go:spawn": `(go:spawn form) — Launch form in a goroutine. Returns thread ID.`,

	"go:channel": `(go:channel [buf-size]) — Create a Go channel.`,

	"go:send": `(go:send channel value) — Send value to channel.`,

	"go:recv": `(go:recv channel) — Receive value from channel.`,

	"go:close": `(go:close channel) — Close a channel.`,

	"go:select": `(go:select cases...) — Go-style select on channels.`,

	"go:chanp": `(go:chanp x) — Check if x is a channel.`,

	"go:new": `(go:new "pkg.TypeName") — Create a zero-value instance of a Go struct type.
  The type must be registered in GoTypeRegistry (auto-registered for all stdlib types).
  Example: (go:new "crypto/x509.Certificate") => #<go-val x509.Certificate>`,

	"go:field": `(go:field obj "FieldName") — Read a struct field from a VGoVal.
  Automatically dereferences pointers to structs.
  Returns the field value: primitives as Lisp values, structs as VGoVal.
  Example: (go:field cert "Version") => 0`,

	"go:set-field": `(go:set-field obj "FieldName" value) — Set a struct field on a VGoVal.
  Supported field types: integers, booleans, strings, slices, *big.Int, nested structs.
  Pointer fields can be set to nil. Lisp lists convert to Go slices.
  Example: (go:set-field cert "IsCA" t) => nil`,

	"go:type-of": `(go:type-of obj) — Return the Go type name of a VGoVal as a string.
  Example: (go:type-of key) => "*rsa.PrivateKey"`,

	"go:call": `(go:call obj "MethodName" args...) — Call a Go method on a VGoVal.
  Automatically finds methods on both value and pointer receivers.
  Example: (go:call cert "Equal" otherCert) => t/nil`,
}

func helpAll() string {
	var sb strings.Builder
	sb.WriteString("=== Lisp Help ===\n\n")
	sb.WriteString("Usage: (help \"name\") to get documentation for a function.\n\n")
	sb.WriteString("Help Topics:\n")
	topicNames := make([]string, 0, len(helpTopicMap))
	for k := range helpTopicMap {
		topicNames = append(topicNames, k)
	}
	sort.Strings(topicNames)
	for _, name := range topicNames {
		sb.WriteString(fmt.Sprintf("  (help \"%s\")\n", name))
	}
	sb.WriteString("\nBuilt-in functions (showing a sample):\n")
	count := 0
	names := make([]string, 0, len(builtinDocMap))
	for k := range builtinDocMap {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, name := range names {
		if count >= 15 {
			sb.WriteString(fmt.Sprintf("  ... and %d more builtins\n", len(builtinDocMap)-15))
			break
		}
		doc := builtinDocMap[name]
		// Show just the signature part (before em-dash)
		if idx := strings.Index(doc, "—"); idx > 0 {
			sb.WriteString(fmt.Sprintf("  %s\n", strings.TrimSpace(doc[:idx])))
		} else {
			sb.WriteString(fmt.Sprintf("  %s\n", doc))
		}
		count++
	}
	sb.WriteString("\ngo:* functions:\n")
	goNames := make([]string, 0, len(goDocMap))
	for k := range goDocMap {
		goNames = append(goNames, k)
	}
	sort.Strings(goNames)
	for _, name := range goNames {
		doc := goDocMap[name]
		if idx := strings.Index(doc, "—"); idx > 0 {
			sb.WriteString(fmt.Sprintf("  %s\n", strings.TrimSpace(doc[:idx])))
		} else {
			sb.WriteString(fmt.Sprintf("  %s\n", doc))
		}
	}
	sb.WriteString(fmt.Sprintf("\nFFI packages: %d available. Try: (help \"math\")\n", len(GoPackageRegistry)))
	sb.WriteString("Special forms: quote if cond and or when unless let let* letrec define defun ")
	sb.WriteString("lambda set! begin do loop defmacro block return-from tagbody go catch throw ")
	sb.WriteString("handler-case restart-case defclass defgeneric defmethod define-condition ")
	sb.WriteString("destructuring-bind multiple-value-bind prog1 prog2 the progv\n")
	return sb.String()
}

func helpGoGroup() string {
	var sb strings.Builder
	sb.WriteString("=== go:* Functions ===\n\n")
	names := make([]string, 0, len(goDocMap))
	for k := range goDocMap {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, name := range names {
		sb.WriteString(goDocMap[name])
		sb.WriteString("\n\n")
	}
	return sb.String()
}

func helpPackage(name string, pkg map[string]reflect.Value) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("=== FFI Package: %s ===\n\n", name))
	sb.WriteString("Functions:\n")
	funcs := make([]string, 0, len(pkg))
	for k := range pkg {
		funcs = append(funcs, k)
	}
	sort.Strings(funcs)
	for _, sym := range funcs {
		rv := pkg[sym]
		if rv.Kind() == reflect.Func {
			sb.WriteString(fmt.Sprintf("  %s.%s (%d params)\n", name, sym, rv.Type().NumIn()))
		}
	}
	if types, ok := GoTypeRegistry[name]; ok {
		sb.WriteString("\nTypes:\n")
		typeNames := make([]string, 0, len(types))
		for k := range types {
			typeNames = append(typeNames, k)
		}
		sort.Strings(typeNames)
		for _, tn := range typeNames {
			t := types[tn]
			sb.WriteString(fmt.Sprintf("  %s.%s (use (go:new \"%s.%s\") to create)\n", name, tn, name, tn))
			_ = t
		}
	}
	return sb.String()
}

func helpTypes(name string, types map[string]reflect.Type) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("=== FFI Types: %s ===\n\n", name))
	typeNames := make([]string, 0, len(types))
	for k := range types {
		typeNames = append(typeNames, k)
	}
	sort.Strings(typeNames)
	for _, tn := range typeNames {
		t := types[tn]
		sb.WriteString(fmt.Sprintf("  %s — %s\n", tn, t.String()))
	}
	return sb.String()
}

func helpFFIFunction(pkg, sym string) string {
	rv := GoPackageRegistry[pkg][sym]
	if rv.Kind() != reflect.Func {
		return fmt.Sprintf("%s.%s is not a function", pkg, sym)
	}
	sig := rv.Type()
	params := make([]string, sig.NumIn())
	for i := 0; i < sig.NumIn(); i++ {
		params[i] = sig.In(i).String()
	}
	returns := make([]string, sig.NumOut())
	for i := 0; i < sig.NumOut(); i++ {
		returns[i] = sig.Out(i).String()
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s.%s\n", pkg, sym))
	sb.WriteString(fmt.Sprintf("  Params: (%s)\n", strings.Join(params, ", ")))
	sb.WriteString(fmt.Sprintf("  Returns: (%s)\n", strings.Join(returns, ", ")))
	return sb.String()
}
