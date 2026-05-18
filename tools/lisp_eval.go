package tools

import (
	"context"
	"fmt"

	"miniclaudecode-go/microlisp"
)

// panicResult converts a recovered panic into a ToolResult.
func panicResult(op string, r any) ToolResult {
	return ToolResult{Output: fmt.Sprintf("Error: lisp_eval panic during %s: %v", op, r), IsError: true}
}

// LispEvalTool evaluates Common Lisp expressions for arithmetic,
// data structures, logic, and computation.
type LispEvalTool struct{}

// evalResult holds the result of a Lisp evaluation.
type evalResult struct {
	output string
	err    error
}

func (*LispEvalTool) Name() string { return "lisp_eval" }

func (*LispEvalTool) Description() string {
	return "Evaluate Common Lisp expressions. State persists between calls. " +
		"Quick start: expression=\"(+ 1 2)\". " +
		"Use operation=\"define\" to see function signatures (params, return types). " +
		"Use operation=\"help\" for topic docs, \"examples\" for code samples, " +
		"\"skill\" for a complete usage guide. FFI: (ffi \"math.Sqrt\") calls Go stdlib."
}

func (*LispEvalTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"expression": map[string]any{
				"type":        "string",
				"description": "Lisp expression to evaluate, e.g. (+ 1 2), (car '(1 2 3)). For source/xref operations: use a PLAIN function name only (e.g. \"car\"). For help/skill: use a topic name like \"arithmetic\" or \"ffi\".",
			},
			"operation": map[string]any{
				"type": "string",
				"enum": []string{"eval", "reset", "help", "examples", "eval_file", "lint", "source", "source-list", "xref", "xref-list", "skill", "define"},
				"description": `Action: eval (default, evaluate expression), reset (clear state), help (topic docs), examples (code samples), skill (usage guide — expression="ffi"/"ops"/"xref" or empty for full), define (function signature — params, return types, usage), source (view function source — expression=plain name), source-list (browse indexed functions), xref (call graph — expression=plain name), xref-list (browse all xrefs), eval_file (run .lisp file), lint (check syntax)`,
			},
			"file": map[string]any{
				"type":        "string",
				"description": "File path for operation=eval_file or lint. Required for eval_file. Optional for lint (use either expression or file).",
			},
			"limits": map[string]any{
				"type":        "string",
				"enum":        []string{"default", "strict", "unlimited"},
				"description": "Resource limit profile for safety: 'default' (1M steps, 30s, 256MB heap) for normal use; 'strict' (100K steps, 10s, 64MB heap) for untrusted code; 'unlimited' disables all limits (REPL mode only). Defaults to 'default'.",
			},
				"offset": map[string]any{
					"type":        "integer",
					"description": "Zero-based line offset for source code display, or entry offset for source-list pagination (default: 0).",
				},
				"limit": map[string]any{
					"type":        "integer",
					"description": "Max lines to return for source code display, or max entries for source-list (default: 50).",
				},
		},
		"required": []string{},
	}
}

func (*LispEvalTool) CheckPermissions(params map[string]any) PermissionResult {
	return PermissionResultPassthrough()
}

func (t *LispEvalTool) ExecuteContext(ctx context.Context, params map[string]any) (result ToolResult) {
	defer func() {
		if r := recover(); r != nil {
			result = panicResult("execute", r)
		}
	}()

	// Check context early
	select {
	case <-ctx.Done():
		return ToolResult{Output: fmt.Sprintf("Error: lisp_eval timed out: %v", ctx.Err()), IsError: true}
	default:
	}

	op, _ := params["operation"].(string)
	expr, _ := params["expression"].(string)
	limitsProfile, _ := params["limits"].(string)

	switch op {
	case "reset":
		microlisp.ResetGlobalEnv()
		return ToolResult{Output: "Lisp interpreter state has been reset. All user-defined variables, functions, macros, and classes have been cleared."}

	case "help":
		return ToolResult{Output: lispHelp(expr)}

	case "skill":
		return ToolResult{Output: lispSkill(expr)}

	case "examples":
		return ToolResult{Output: lispExamples(expr)}

	case "eval_file":
		file, _ := params["file"].(string)
		if file == "" {
			return ToolResult{Output: "Error: file is required for operation=eval_file", IsError: true}
		}
		var limits microlisp.ResourceLimits
		switch limitsProfile {
		case "strict":
			limits = microlisp.StrictLimits()
		case "unlimited":
			limits = microlisp.UnlimitedLimits()
		default:
			limits = microlisp.DefaultLimits()
		}
		cancelChan := microlisp.NewCancelChannel()
		limits.CancelChan = cancelChan
		ch := make(chan evalResult, 1)
		go func() {
			defer func() {
				if r := recover(); r != nil {
					ch <- evalResult{"", fmt.Errorf("panic: %v", r)}
				}
			}()
			output, err := microlisp.SafeLoadFileWithLimits(file, limits)
			ch <- evalResult{output, err}
		}()
		select {
		case <-ctx.Done():
			close(cancelChan)
			return ToolResult{Output: "Error: lisp_eval timed out loading file", IsError: true}
		case r := <-ch:
			if r.err != nil {
				return ToolResult{Output: fmt.Sprintf("Error: %v", r.err), IsError: true}
			}
			result := r.output
			if result == "" {
				result = "NIL"
			}
			return ToolResult{Output: result}
		}

	case "lint":
		file, _ := params["file"].(string)
		if file != "" {
			var limits microlisp.ResourceLimits
			switch limitsProfile {
			case "strict":
				limits = microlisp.StrictLimits()
			case "unlimited":
				limits = microlisp.UnlimitedLimits()
			default:
				limits = microlisp.DefaultLimits()
			}
			err := microlisp.SafeLintFileWithLimits(file, limits)
			if err != nil {
				return ToolResult{Output: fmt.Sprintf("Lint error: %v", err), IsError: true}
			}
			return ToolResult{Output: "No syntax errors found."}
		}
		if expr == "" {
			return ToolResult{Output: "Error: expression or file is required for operation=lint", IsError: true}
		}
		var limits microlisp.ResourceLimits
		switch limitsProfile {
		case "strict":
			limits = microlisp.StrictLimits()
		case "unlimited":
			limits = microlisp.UnlimitedLimits()
		default:
			limits = microlisp.DefaultLimits()
		}
		err := microlisp.SafeLintWithLimits(expr, limits)
		if err != nil {
			return ToolResult{Output: fmt.Sprintf("Lint error: %v", err), IsError: true}
		}
		return ToolResult{Output: "No syntax errors found."}

	case "define":
		if expr == "" {
			return ToolResult{Output: "Error: expression is required for operation=define. Use a plain function name like \"car\" or \"math.Sin\" — NOT a Lisp expression.", IsError: true}
		}
		return ToolResult{Output: microlisp.GetDefine(expr)}

	case "source":
		if expr == "" {
			return ToolResult{Output: "Error: expression is required for operation=source. Use a plain function name like \"car\" or \"string-append\" — NOT a Lisp expression like (source 'car).", IsError: true}
		}
		srcOffset := 0
		srcLimit := 0
		if v, ok := params["offset"].(float64); ok {
			srcOffset = int(v)
		}
		if v, ok := params["limit"].(float64); ok {
			srcLimit = int(v)
		}
		return ToolResult{Output: microlisp.GetSource(expr, srcOffset, srcLimit)}

	case "source-list":
		offset := 0
		limit := 0
		if v, ok := params["offset"].(float64); ok {
			offset = int(v)
		}
		if v, ok := params["limit"].(float64); ok {
			limit = int(v)
		}
		return ToolResult{Output: microlisp.SourceList(expr, offset, limit)}

	case "xref":
		if expr == "" {
			return ToolResult{Output: "Error: expression is required for operation=xref. Use a plain function name like \"car\" or \"string-append\".", IsError: true}
		}
		contextLines := 2
		if v, ok := params["limit"].(float64); ok {
			contextLines = int(v)
		}
		return ToolResult{Output: microlisp.GetXRef(expr, contextLines)}

	case "xref-list":
		offset := 0
		limit := 0
		if v, ok := params["offset"].(float64); ok {
			offset = int(v)
		}
		if v, ok := params["limit"].(float64); ok {
			limit = int(v)
		}
		return ToolResult{Output: microlisp.XRefList(expr, offset, limit)}

	default: // eval (default when operation is empty or unknown)
		if expr == "" {
			return ToolResult{Output: "Error: expression is required. Examples: (+ 1 2) => 3, (car '(1 2 3)) => 1", IsError: true}
		}

		// Select resource limits based on profile
		var limits microlisp.ResourceLimits
		switch limitsProfile {
		case "strict":
			limits = microlisp.StrictLimits()
		case "unlimited":
			limits = microlisp.UnlimitedLimits()
		default:
			limits = microlisp.DefaultLimits()
		}

		// Wire context cancellation into the Lisp evaluator's CancelChan.
		// When ctx.Done() fires, closing cancelChan triggers stepCheck()
		// to abort the evaluation (checked every 1024 steps), releasing
		// evalMu. Without this, the goroutine holding evalMu would
		// continue running indefinitely after context cancellation,
		// causing permanent deadlock on any subsequent lisp_eval call.
		cancelChan := microlisp.NewCancelChannel()
		limits.CancelChan = cancelChan

		// Run eval in a goroutine so we can respect context cancellation.
		// The microlisp interpreter holds evalMu during execution.
		// CancelChan allows stepCheck() to abort mid-evaluation when
		// the context is cancelled, ensuring evalMu is released promptly.
		ch := make(chan evalResult, 1)
		go func() {
			defer func() {
				if r := recover(); r != nil {
					ch <- evalResult{"", fmt.Errorf("panic: %v", r)}
				}
			}()
			result, err := microlisp.SafeEvalWithLimits(expr, limits)
			ch <- evalResult{result, err}
		}()
		select {
		case <-ctx.Done():
			close(cancelChan)
			return ToolResult{Output: fmt.Sprintf("Error: lisp_eval timed out evaluating expression"), IsError: true}
		case r := <-ch:
			if r.err != nil {
				return ToolResult{Output: fmt.Sprintf("Error: %v", r.err), IsError: true}
			}
			result := r.output
			if result == "" {
				result = "NIL"
			}
			return ToolResult{Output: result}
		}
	}
}

func (t *LispEvalTool) Execute(params map[string]any) ToolResult {
	return t.ExecuteContext(context.Background(), params)
}

// lispHelp returns the usage manual. If topic is non-empty, shows that topic specifically.
func lispHelp(topic string) string {
	topics := map[string]string{
		"arithmetic": `Arithmetic Operations:
  (+ a b c...)     Addition (arbitrary precision integers, rationals, floats)
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
  (truncate n)     Truncate toward zero
  (max a b...)     Maximum
  (min a b...)     Minimum
  (gcd a b...)     Greatest common divisor
  (lcm a b...)     Least common multiple
  (1+ n) / (1- n)  Increment / Decrement
  (float n)        Convert to float
  (rational n)     Convert to rational
  (numerator r)    Numerator of rational
  (denominator r)  Denominator of rational
  (random n)       Random number [0, n)
  (= a b)          Numeric equality
  (/= a b)         Not equal
  (< a b) / (> a b)  Less than / Greater than
  (<= a b) / (>= a b)  Less-equal / Greater-equal
  (evenp n) / (oddp n)  Even / Odd check
  (zerop n)        Zero check
  (plusp n)        Positive check
  (minusp n)       Negative check
  (integerp n) / (floatp n) / (rationalp n)  Type predicates

Constants: pi, most-positive-fixnum, most-negative-fixnum
Booleans: t (true), nil (false)`,

		"lists": `List Operations:
  (cons a b)       Create a pair/cons cell
  (car x)          First element of a cons/list
  (cdr x)          Rest of a cons/list (all but first)
  (list a b c...)  Create a list from arguments
  (append l1 l2)   Concatenate lists
  (length l)       Length of list/string
  (reverse l)      Reverse a list
  (nth n l)        Nth element (0-indexed)
  (first l) / (second l) / (third l) ... (tenth l)
  (rest l)         Same as cdr
  (null x) / (null? x)  Check if nil/empty
  (consp x) / (pair? x)  Check if cons cell
  (listp x) / (LISTP x)  Check if list
  (atom x)         Check if not a cons cell
  (member x l)     Check if x is a member of l
  (assoc x alist)  Association list lookup
  (push x lst)     Push element onto list
  (pop lst)        Pop element from list
  (nreverse l)     Destructive reverse`,

		"strings": `String Operations:
  (string x)        Convert to string
  (string-append a b...)  Concatenate strings
  (string-length s)  Length of string
  (string-find s ch)  Find character in string
  (substring s start [end])  Extract substring
  (string->number s)  Parse string to number
  (number->string n)  Convert number to string
  (symbol->string sym)  Convert symbol to string
  (string->symbol s)  Convert string to symbol
  (char s i)        Get character at index
  (char-setf s i ch) Set character at index
  (char= a b) / (char< a b) / (char> a b)  Character comparison
  (code-char n)     Integer to character
  (char-code ch)    Character to integer code
  (char-name ch)    Get character name (e.g., "Space", "Newline")
  (name-char name)  Name to character
  (char-upcase ch) / (char-downcase ch)  Case conversion
  (alphanumericp ch)  Check if alphanumeric
  (digit-char-p ch)  Check if digit
  (characterp x)    Check if character type
  (stringp x)       Check if string type
  (string-equal a b)  Case-insensitive string equality`,

		"lambda": `Lambda and Closures:
  (lambda (args) body...)  Anonymous function
  ((lambda (x) (* x x)) 5)  => 25
  (define (name args) body...)  Define a function
  (defun name (args) body...)   Define a function (CL style)
  (funcall fn args...)    Call a function
  (apply fn list)         Apply function to list of args
  (let ((var val)...) body...)  Lexical binding
  (let* ((var val)...) body...) Sequential let
  (progn body...)      Execute body, return last result
  (if test then [else])  Conditional
  (cond (test1 body1) (test2 body2) ...)  Multi-way conditional
  (when test body...)   Execute if test is true
  (unless test body...) Execute if test is false
  (and a b c...)       Short-circuit AND
  (or a b c...)        Short-circuit OR
  (not x)              Logical NOT
  Closures capture lexical environment:
  (define (make-counter) (let ((count 0)) (lambda () (set! count (1+ count)))))`,

		"recursion": `Recursion and Iteration:
  (define (fibonacci n)
    (if (<= n 1) n (+ (fibonacci (- n 1)) (fibonacci (- n 2)))))
  (define (factorial n)
    (if (<= n 1) 1 (* n (factorial (- n 1)))))
  Tail-call optimization is supported.
  Loop constructs:
  (loop for i from 1 to 10 collect (* i i))
  (dotimes (i 10) (print i))
  (dolist (x '(1 2 3)) (print x))
  (do ((i 0 (1+ i))) ((>= i 10)) (print i))`,

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
    ~R  Roman numeral or radix
  Examples:
  (format nil "Hello, ~A!" "World")  => "Hello, World!"
  (format t "~D + ~D = ~D~%" 2 3 (+ 2 3))
  (format nil "~{~A, ~}" '(1 2 3))  => "1, 2, 3, "`,

		"hash": `Hash Tables:
  (make-hash-table)  Create a hash table
  (gethash key ht)   Get value (returns value, found-p)
  (gethash key ht default)  Get value with default
  (setf (gethash key ht) val)  Set value
  (remhash key ht)   Remove entry
  (clrhash ht)       Clear hash table
  (maphash fn ht)    Apply function to each key-value pair
  (hash-table-count ht)  Number of entries
  (hash-tablep x)    Check if hash table`,

		"arrays": `Arrays:
  (make-array dims)  Create array. dims: integer (1D) or list (multi-D)
  (aref array i...)  Access element
  (setf (aref array i...) val)  Set element
  (array-rank array)  Number of dimensions
  (array-dimension array n)  Size of dimension n
  (array-dimensions array)  All dimensions as list
  (array-total-size array)  Total elements
  (adjust-array array new-dims)  Resize array
  (copy-array array)  Copy array
  (arrayp x)  Check if array`,

		"clos": `CLOS (Common Lisp Object System):
  (defclass name (superclasses) (slots) options...)
  (defclass person () ((name :initarg :name :accessor person-name)
                        (age :initarg :age :accessor person-age)))
  (make-instance 'person :name "Alice" :age 30)
  (defgeneric fn-name (args))
  (defmethod fn-name ((arg class-name)) body...)
  Method combinations: :before, :after, :around
  (slot-value obj 'slot-name)  Access slot directly
  (slot-boundp obj 'slot-name) Check if slot has value`,

		"conditions": `Conditions and Error Handling:
  (define-condition name (superclass) (slots))
  (make-condition 'name :slot value)
  (signal condition)  Signal a condition
  (error "message")  Signal an error
  (warn "message")   Signal a warning
  (handler-case form (condition-type (var) handler-body...))
  (restart-case form (restart-name () handler-body...))
  (invoke-restart restart-name)  Invoke a restart`,

		"macros": `Macros:
  (define-macro (name args) body...)  Define a macro
  (defmacro name (args) body...)  CL-style macro
  (backquote/quasiquote) with , (unquote) and ,@ (splice)
  Macro expansion:
  (define-macro (unless test . body)
    (list 'if test (list 'progn) (cons 'begin body)))
  (macroexpand-1 form)  Expand macro once
  (macroexpand form)    Fully expand macro
  Compiler macros (define-compiler-macro) for optimization.`,

		"types": `Type System:
  (type-of x)       Runtime type of value
  (typep x type)    Check if value is of type
  (subtypep a b)    Check if type A is subtype of B
  (deftype name (args) body...)  Define new type
  Type specifiers:
    number, integer, float, rational, complex
    string, symbol, character, list, cons
    array, hash-table, readtable, package
    stream, function, null, boolean
    (integer 0 100)  Bounded integer type
    (array fixnum (*))  1D array of fixnums`,

		"special": `Special Variables and Symbols:
  *print-base*      Output number base (default 10)
  *print-radix*     Show radix prefix (e.g., #x for hex)
  *print-case*     Symbol case: :UPCASE, :DOWNCASE, :CAPITALIZE
  *read-base*       Input number base
  *random-state*    Current random state
  (random-state-p x) Check if random state
  *posix-argv*      Command line arguments
  (boundp 'symbol)   Check if symbol has a value
  (fboundp 'symbol)  Check if symbol has a function
  (gensym)           Generate a unique symbol
  (gentemp)          Generate a unique temp symbol
  (symbol-value s)   Get symbol's value
  (symbol-function s) Get symbol's function
  (set 'sym val)     Set symbol's value`,
	}

	if topic == "" {
		return `Lisp Interpreter Manual — Available Topics:
  arithmetic   Math: +, -, *, /, mod, expt, sqrt, sin/cos/tan
  lists        Cons, car, cdr, list, append, reverse, nth
  strings      String manipulation, character operations
  lambda       Anonymous functions, define, let, if, cond
  recursion    Factorial, fibonacci, loops (loop, dotimes, dolist)
  format       Format strings: ~A, ~D, ~%, ~{~}, ~R
  hash         Hash tables: make-hash-table, gethash, setf
  arrays       Multi-dimensional arrays: make-array, aref
  clos         Object system: defclass, defgeneric, defmethod
  conditions   Error handling: handler-case, restart-case
  macros       define-macro, defmacro, backquote
  types        type-of, typep, subtypep, deftype
  special      *print-base*, *random-state*, gensym, boundp

Use lisp_eval with operation="help" and expression=<topic> for details.
Example: lisp_eval with operation="help", expression="arithmetic"`
	}

	for k, v := range topics {
		if k == topic {
			return v
		}
	}
	return fmt.Sprintf("Unknown topic: %s\n\nAvailable: arithmetic, lists, strings, lambda, recursion, format, hash, arrays, clos, conditions, macros, types, special", topic)
}

// lispExamples returns example expressions.
func lispExamples(category string) string {
	examples := map[string]string{
		"arithmetic": `Arithmetic Examples:
  (+ 1 2 3)                      => 6
  (- 10 3)                       => 7
  (* 2 3 4)                      => 24
  (/ 10 3)                       => 10/3 (exact rational)
  (mod 17 5)                     => 2
  (expt 2 10)                    => 1024
  (sqrt 2)                       => 1.4142135623730951
  (abs -42)                      => 42
  (+ 1/3 1/6)                    => 1/2
  (floor 7 3)                    => 2
  (ceiling 7 3)                  => 3
  (round 2.7)                    => 3
  (max 1 5 3 9 2)               => 9
  (min 1 5 3 9 2)               => 1
  (gcd 48 18)                    => 6
  (lcm 4 6)                      => 12
  (1+ 99)                        => 100
  (evenp 42)                     => t
  (oddp 7)                       => t
  (zerop 0)                      => t`,

		"lists": `List Examples:
  (cons 1 '(2 3))                => (1 2 3)
  (car '(1 2 3))                 => 1
  (cdr '(1 2 3))                 => (2 3)
  (list 1 2 3)                   => (1 2 3)
  (append '(1 2) '(3 4))         => (1 2 3 4)
  (length '(a b c))              => 3
  (reverse '(1 2 3))             => (3 2 1)
  (nth 2 '(a b c d))             => c
  (first '(a b c))               => a
  (member 'b '(a b c))           => (b c)`,

		"lambda": `Lambda and Function Examples:
  ((lambda (x) (* x x)) 5)       => 25
  (define (square x) (* x x))
  (square 7)                     => 49
  (funcall (lambda (a b) (+ a b)) 3 4)  => 7
  (let ((x 10) (y 20)) (+ x y))  => 30
  (let* ((x 5) (y (+ x 1))) (* x y)) => 30
  (if (> 5 3) "yes" "no")        => "yes"
  (cond ((> 5 3) "big") ((= 5 3) "equal") (t "small"))  => "big"`,

		"recursion": `Recursion Examples:
  (define (factorial n)
    (if (<= n 1) 1 (* n (factorial (- n 1)))))
  (factorial 10)                 => 3628800

  (define (fibonacci n)
    (if (<= n 1) n (+ (fibonacci (- n 1)) (fibonacci (- n 2)))))
  (fibonacci 10)                 => 55

  (define (sum-list lst)
    (if (null lst) 0 (+ (car lst) (sum-list (cdr lst)))))
  (sum-list '(1 2 3 4 5))        => 15`,

		"map": `Map and Higher-Order Examples:
  (mapcar (lambda (x) (* x x)) '(1 2 3 4 5))
    => (1 4 9 16 25)
  (mapcar (lambda (x y) (+ x y)) '(1 2 3) '(4 5 6))
    => (5 7 9)
  (reduce (lambda (a b) (+ a b)) '(1 2 3 4 5))
    => 15
  (remove-if (lambda (x) (= (mod x 2) 0)) '(1 2 3 4 5 6))
    => (1 3 5)`,

		"strings": `String Examples:
  (string-append "Hello, " "World!")  => "Hello, World!"
  (string-length "Hello")          => 5
  (string->number "42")            => 42
  (number->string 3.14)            => "3.14"
  (substring "Hello World" 0 5)    => "Hello"
  (char "ABC" 1)                   => #\B
  (char-code #\A)                  => 65
  (code-char 65)                   => #\A`,

		"format": `Format Examples:
  (format nil "Hello, ~A!" "World")
    => "Hello, World!"
  (format nil "~D items" 42)
    => "42 items"
  (format nil "~S" "hello")
    => "\"hello\""
  (format nil "~{~A, ~}" '(apple banana cherry))
    => "apple, banana, cherry, "
  (format nil "~R" 2024)
    => "two thousand and twenty-four"`,

		"hash": `Hash Table Examples:
  (defvar ht (make-hash-table))
  (setf (gethash "name" ht) "Alice")
  (setf (gethash "age" ht) 30)
  (gethash "name" ht)            => "Alice", t
  (gethash "missing" ht "default")  => "default", nil
  (hash-table-count ht)           => 2`,

		"arrays": `Array Examples:
  (defvar vec (make-array 5))
  (setf (aref vec 0) 10)
  (aref vec 0)                   => 10
  (make-array '(2 3))            => 2x3 array
  (defvar mat (make-array '(2 3)))
  (setf (aref mat 0 1) 42)
  (aref mat 0 1)                 => 42
  (array-dimensions mat)         => (2 3)`,

		"misc": `Miscellaneous Examples:
  (random 100)                   => random integer [0, 100)
  (type-of 42)                   => integer
  (type-of "hello")              => string
  (type-of '(1 2))               => cons
  (boundp 'nil)                  => t
  (gensym)                       => #:G0
  (loop for i from 1 to 5 collect (* i i))
    => (1 4 9 16 25)
  (dotimes (i 3) (print i))     => prints 0 1 2`,
	}

	if category == "" {
		return `Lisp Examples — Categories:
  arithmetic    (+ 1 2), (expt 2 10), (sqrt 2)
  lists         (cons 1 '(2 3)), (car '(1 2 3)), (append '(1 2) '(3 4))
  lambda        ((lambda (x) (* x x)) 5), (define (square x) (* x x))
  recursion     factorial, fibonacci, sum-list
  map           mapcar, reduce, remove-if
  strings       string-append, string->number, char-code
  format        (format nil "Hello, ~A!" "World")
  hash          make-hash-table, gethash, setf
  arrays        make-array, aref, array-dimensions
  misc          random, type-of, gensym, loop, dotimes

Use lisp_eval with operation="examples" and expression=<category> for details.`
	}

	for k, v := range examples {
		if k == category {
			return v
		}
	}
	return fmt.Sprintf("Unknown category: %s\n\nAvailable: arithmetic, lists, lambda, recursion, map, strings, format, hash, arrays, misc", category)
}

// lispSkill returns a comprehensive guide for learning how to use this Lisp interpreter.
// topic: "ffi" for FFI/Go interop guide, "ops" for operations guide, "xref" for cross-reference guide,
// empty for the full guide.
func lispSkill(topic string) string {
	guides := map[string]string{
		"ffi": `=== FFI: Go Standard Library Interop Guide ===

This Lisp interpreter can call Go standard library functions via FFI (Foreign Function Interface).
Use the (ffi "package.Function") or (go:import "package.Function") form to import and call Go functions.

--- How to Import a Go Function ---
(ffi "package.FunctionName")  => returns a callable function object
(go:import "package.FunctionName")  => same, alias form

Example: Import math.Sin and call it:
  (ffi "math.Sin")        => returns a function
  ((ffi "math.Sin") 1.0)  => calls math.Sin(1.0) => 0.8414709848...

--- How to Import Go Variables ---
(go:import "package.Variable")  => returns the actual Go value as a VGoVal
(go:import "crypto/rand.Reader")  => #<go-val *rand.reader>

VGoVal is an opaque Go value type that preserves the original Go interface/struct/pointer.
When passed to another FFI function, the actual Go value is extracted automatically if the
target parameter type is compatible (assignable or implements the required interface).

Example: Pass crypto/rand.Reader to rsa.GenerateKey:
  (funcall (go:import "crypto/rsa.GenerateKey")
           (go:import "crypto/rand.Reader")
           2048)
  => #<go-val *rsa.PrivateKey>

The reader VGoVal (type *rand.reader) is automatically recognized as implementing io.Reader.

--- Available Go Packages ---
  fmt             — Sprintf, Printf, Println, Errorf, Sprint, etc.
  strings         — Contains, HasPrefix, HasSuffix, Split, Join, Replace, Trim, ToUpper, ToLower, etc.
  strconv         — Atoi, Itoa, ParseFloat, FormatInt, Quote, Unquote, etc.
  math            — Sin, Cos, Tan, Sqrt, Abs, Pow, Exp, Log, Floor, Ceil, Round, Max, Min, Pi, E, etc.
  math/big        — NewInt, NewFloat, NewRat
  math/rand       — Intn, Float64, Float32, Int63, Perm, Shuffle, Seed
  math/bits       — Len, OnesCount, TrailingZeros, Reverse, RotateLeft, Add, Sub, Mul, Div
  os              — Getenv, Setenv, Environ, Getwd, Mkdir, Remove, Stat, Open, Args, Exit, etc.
  io              — Copy, ReadFull, WriteString, Pipe, MultiReader, MultiWriter
  bytes           — Contains, Equal, Split, Join, Replace, Trim, Compare, NewBuffer
  regexp          — MustCompile, Compile, QuoteMeta
  sort            — Ints, Float64s, Strings, Search, Slice, SliceStable
  time            — Now, Parse, Sleep, Since, Unix, Date, RFC3339, Hour, Minute, Second
  net             — Dial, Listen, LookupHost, ParseIP, SplitHostPort
  net/http        — Get, Post, NewRequest, ListenAndServe, StatusText, NewServeMux
  net/url         — Parse, QueryEscape, PathEscape, JoinPath
  encoding/json   — Marshal, Unmarshal, MarshalIndent, Valid, Compact, HTMLEscape
  encoding/xml    — Marshal, Unmarshal, MarshalIndent, EscapeText
  encoding/csv    — NewReader, NewWriter
  encoding/base64 — StdEncoding, URLEncoding, EncodeToString, DecodeToString
  log             — Print, Printf, Println, Fatal, SetFlags, SetPrefix
  sync            — NewCond (types: Mutex, RWMutex, WaitGroup, Once, Map, Pool)
  runtime         — GOOS, GOARCH, NumCPU, NumGoroutine, GC, Version, ReadMemStats
  errors          — New, Is, As, Unwrap, Join
  flag            — Parse, Bool, String, Int, Lookup, Args
  path            — Base, Clean, Dir, Ext, IsAbs, Join, Match, Split
  path/filepath   — Abs, Base, Clean, Dir, Glob, Join, Walk, WalkDir, Ext, IsAbs, Rel
  context         — Background, TODO, WithCancel, WithTimeout, WithDeadline, WithValue
  os/exec         — Command, CommandContext, LookPath
  os/signal       — Notify, Stop, Reset
  unicode         — IsLetter, IsDigit, IsLower, IsUpper, IsSpace, ToLower, ToUpper
  unicode/utf8    — DecodeRune, EncodeRune, RuneCount, Valid, RuneLen, FullRune
  unicode/utf16   — Decode, Encode, IsSurrogate
  io/fs           — WalkDir, ReadDir, ReadFile, Glob, ValidPath
  hash/crc32      — ChecksumIEEE, NewIEEE, MakeTable
  hash/crc64      — Checksum, New, MakeTable
  crypto/md5      — New, Sum, Size
  crypto/sha1     — New, Sum, Size
  crypto/sha256   — New, Sum256, Size
  text/template   — New, Must, ParseFiles
  text/scanner    — ScanInts, ScanFloats, TokenString
  text/tabwriter  — NewWriter, FilterHTML
  syscall         — Getpid, Getppid, Getuid, Getgid
  runtime/debug   — FreeOSMemory, ReadBuildInfo, SetGCPercent, Stack

--- Listing Available Symbols ---
  (go:list)                => list all package names
  (go:list "math")         => list all symbols in math package
  (go:list "strings")      => list all symbols in strings package

--- FFI Call Patterns ---
1. Simple function call:
   ((ffi "math.Sqrt") 2.0)  => 1.4142135623...

2. Multi-argument function:
   ((ffi "strings.Contains") "hello world" "world")  => true
   ((ffi "math.Pow") 2.0 10.0)  => 1024.0
   ((ffi "fmt.Sprintf") "%d items" 42)  => "42 items"

3. Store imported function for reuse:
   (defvar sin-fn (ffi "math.Sin"))
   (funcall sin-fn 1.5708)  => ~1.0

4. Variadic functions (like fmt.Sprintf):
   ((ffi "fmt.Sprintf") "Hello, %s! You have %d messages." "Alice" 5)
   => "Hello, Alice! You have 5 messages."

5. Functions returning multiple values:
   ((ffi "math.Modf") 3.14)  => returns a list of values

6. Functions returning (value, error):
   ((ffi "strconv.Atoi") "42")  => 42 (error is nil)
   ((ffi "strconv.Atoi") "abc")  => Error: strconv.Atoi: parsing "abc": invalid syntax

7. Passing Go values between functions (VGoVal):
   (defvar reader (go:import "crypto/rand.Reader"))  => #<go-val *rand.reader>
   (type-of reader)  => GO-VAL
   ;; Pass VGoVal as argument — automatically extracts underlying Go value:
   (funcall (go:import "crypto/rsa.GenerateKey") reader 2048)  => #<go-val *rsa.PrivateKey>
   ;; Works for io.Reader, io.Writer, and other interfaces

--- Creating and Manipulating Go Structs ---
  (go:new "crypto/x509.Certificate")  => #<go-val x509.Certificate>
  (go:field cert "Version")           => 0
  (go:set-field cert "Version" 3)     => nil
  (go:set-field cert "IsCA" t)        => nil
  (go:field cert "Subject")           => #<go-val pkix.Name> (nested struct)

  Full example — create a self-signed certificate:
  (let* ((cert (go:new "crypto/x509.Certificate"))
         (priv (funcall (go:import "crypto/rsa.GenerateKey")
                        (go:import "crypto/rand.Reader")
                        2048)))
    (go:set-field cert "SerialNumber" 1)
    (go:set-field cert "IsCA" t)
    (go:set-field cert "BasicConstraintsValid" t)
    (go:import "crypto/x509.CreateCertificate")
    ;; pass cert and priv to CreateCertificate...
  )

  Supported field types: integers, booleans, strings, slices, *big.Int, nested structs.
  Pointer fields can be set to nil or to numeric/struct values.

--- Registering Custom Go Values ---
  (go:register "mypackage.myvalue" 42)   => register a number
  (go:register "mypackage.mystring" "hello") => register a string

--- Important Notes ---
- All numeric arguments are converted to the Go type the function expects (int, float64, etc.)
- String arguments pass directly to Go string parameters
- Boolean arguments: Lisp t/nil map to Go true/false
- List arguments convert to Go slices when the parameter expects a slice
- VGoVal values (opaque Go values) are automatically unwrapped when passed to FFI functions
  whose parameter type is compatible (assignable or implements the interface)
- If a Go function returns an error, it becomes a Lisp error (evaluation stops)
- If a Go function panics, it is caught and reported as an error
- Argument count must match exactly (non-variadic) or meet minimum (variadic)
- Use operation=source with expression="ffi" to view the FFI builtin source code`,

		"ops": `=== lisp_eval Operations Guide ===

--- Getting Started ---
1. Evaluate:  expression="(+ 1 2)"
2. Define:    expression="(defun square (x) (* x x))"
3. Call:      expression="(square 7)"

--- Need Something Specific? ---
- Function signature?  → operation=define, expression="car" (params, return types, usage)
- Function reference? → operation=help, expression="arithmetic" (or any topic)
- Code examples?      → operation=examples, expression="lists" (or any category)
- Go interop/FFI?     → operation=skill, expression="ffi"
- View source code?   → operation=source, expression="car" (plain name)
- Explore call graph? → operation=xref, expression="filter" (plain name)
- List functions?     → operation=source-list, expression=""
- Full guide?         → operation=skill, expression=""

--- All Operations ---
  define (NEW) — function signature: params, return types, usage
    expression: "car"         →  (car lst), returns first element
    expression: "math.Pow"    →  func math.Pow(float64, float64) → (float64)
    expression: "format"      →  (format dest fmt args...), returns formatted string
  eval (default) — evaluate Lisp expression; state persists
    expression: "(+ 1 2)"  →  3
    limits: "default" | "strict" | "unlimited"
  reset — clear all interpreter state
  help — topic reference; expression="" lists topics
    Topics: arithmetic, lists, strings, lambda, recursion, format,
            hash, arrays, clos, conditions, macros, types, special
  examples — code examples; expression="" lists categories
    Categories: arithmetic, lists, lambda, recursion, map, strings,
                format, hash, arrays, misc
  skill — comprehensive guide; expression="ffi"/"ops"/"xref"
  eval_file — run .lisp file; file="/path/to/file.lisp" (required)
  lint — check syntax; expression="(defun foo (x) x)" or file="/path.lisp"
  source — view function source; expression=PLAIN NAME (e.g. "car", NOT "(source 'car)")
    offset: 0 (line offset), limit: 50 (max lines)
  source-list — browse indexed functions; expression="string" filters
    offset: 0 (entry offset), limit: 50 (max entries)
  xref — callers/callees; expression=PLAIN NAME (e.g. "filter")
    limit: 2 (context lines around each call site)
  xref-list — browse all xrefs; expression="" lists all pairs
    offset: 0, limit: 50

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
    => Each reference shows: file, line number, and source snippet

  operation=xref, expression="filter"
    => Shows callers of filter AND callees of filter
    => Callees might include: if, null?, car, cons, cdr, etc.

  operation=xref, expression="string-append", limit=3
    => limit controls how many context lines to show around each call site

--- xref-list: Browse All Relationships ---
  operation=xref-list, expression=""
    => Show all caller->callee pairs (paginated)

  operation=xref-list, expression="string"
    => Filter to pairs involving "string" functions

  operation=xref-list, expression="car", offset=0, limit=20
    => First 20 pairs involving "car"

--- Understanding the Output ---
Callers section:
  "filter (stdlib, stdlib.go:45)" — filter is called at line 45 of stdlib

Callees section:
  "if (special, conditions.go:12-45)" — filter calls the if special form
  "car [called 3 times]" — filter calls car 3 times

Call sites section (with context):
  stdlib.go:45 → if
    (if (null? lst) nil (cons (func (car lst)) (filter func (cdr lst))))
  Shows the exact source line where the call happens

--- Tips ---
- Use operation=source to see full source code of any function
- Use operation=source-list to discover function names you can query with xref
- Cross-references cover both Go builtins and Lisp stdlib functions
- The index is built lazily on first xref query (may take a moment)`,
	}

	if topic != "" {
		for k, v := range guides {
			if k == topic {
				return v
			}
		}
		return fmt.Sprintf("Unknown skill topic: %s\n\nAvailable: ffi, ops, xref (or empty for full guide)", topic)
	}

	// Full guide
	return `=== Lisp Interpreter Skill: Complete Guide ===

--- Quick Start (3 steps) ---
1. Calculate:  expression="(+ 1 2)" → 3
2. Define:     expression="(defun square (x) (* x x))" → function
3. Call:       expression="(square 7)" → 49

--- Learn More ---
- How to call a function?                   → operation=define, expression="car"
- What params does a Go function take?      → operation=define, expression="math.Pow"
- Arithmetic, lists, strings, lambda, etc.? → operation=help, expression="topic"
- See code examples?                        → operation=examples, expression="category"
- Call Go stdlib functions?                 → operation=skill, expression="ffi"
- View source of a function?                → operation=source, expression="car"
- Explore who calls what?                   → operation=xref, expression="filter"
- All operations explained?                 → operation=skill, expression="ops"

--- Core Features ---
- Arbitrary-precision integers, rationals, complex numbers
- Lists, strings, hash tables, arrays, structs
- Lambda, closures, macros, CLOS (object system)
- Conditions/restarts (error handling)
- Format directives (~A, ~D, ~F, ~%, ~{~})
- State persists between calls (defvar, defun survive)
- Thread-safe: concurrent calls serialized

--- FFI Quick Reference ---
Import:  (ffi "math.Sqrt") or (go:import "math.Sin")
Call:    ((ffi "math.Sqrt") 2.0) → 1.414...
Structs: (go:new "crypto/x509.Certificate")
Fields:  (go:field cert "Version"), (go:set-field cert "IsCA" t)
Details: operation=skill, expression="ffi"

--- Resource Limits ---
default:  1M steps, 30s, 256MB — normal use
strict:   100K steps, 10s, 64MB — untrusted code
unlimited: no limits — REPL mode only

--- All Skill Topics ---
  expression="ffi"   — FFI/Go interop guide (import, call, structs, VGoVal)
  expression="ops"   — all operations explained (eval, help, source, xref, etc.)
  expression="xref"  — cross-reference guide (call graph exploration)`
}
