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
	return "Evaluate Common Lisp expressions for precise computation, logic, and data manipulation. " +
		"Supports: arbitrary-precision integers, rationals, complex numbers, lists, strings, hash tables, " +
		"lambda, macros, CLOS, conditions/restarts, format directives, sequence operations, and Go FFI. " +
		"Use for: math calculations, string processing, list manipulation, algorithm prototyping, calling Go stdlib. " +
		"State persists between calls (defvar, defun survive). " +
		"Thread-safe: concurrent calls are serialized. " +
		"FFI: use (ffi \"package.Func\") or (go:import \"package.Func\") to call Go stdlib. " +
		"VGoVal: Go interface/pointer values are preserved when passed between FFI calls (e.g. crypto/rand.Reader as io.Reader). " +
		"Use operation='skill' to learn how to use this Lisp interpreter, FFI, and all tool operations. Use operation='source' with a plain function name (NOT Lisp syntax) to view source of any builtin/stdlib function, 'source-list' to browse all available functions. " +
		"Use operation='xref' to see callers/callees of a function, 'xref-list' to browse all cross-references."
}

func (*LispEvalTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"expression": map[string]any{
				"type":        "string",
				"description": "Lisp expression to evaluate, e.g. (+ 1 2), (car '(1 2 3)), (format nil \"Hello ~A\" \"World\"). IMPORTANT for operation=source and operation=source-list: use a PLAIN FUNCTION NAME only, NOT a Lisp expression. Correct: expression=\"car\", expression=\"string-append\", expression=\"defclass\". WRONG: expression=\"(source 'car)\", expression=\"(source \\\"car\\\")\", expression=\"'car\". For operation=help: use topic name like \"arithmetic\", \"lists\", \"lambda\". For operation=examples: use category name like \"arithmetic\", \"recursion\", \"format\". For operation=source-list: optional filter substring like \"car\" to find all functions containing that string.",
			},
			"operation": map[string]any{
				"type":        "string",
				"enum":        []string{"eval", "reset", "help", "examples", "eval_file", "lint", "source", "source-list", "xref", "xref-list", "skill"},
				"description": "Action to perform: 'eval' to evaluate a Lisp expression (default), 'reset' to clear interpreter state, 'help' to show usage manual, 'examples' to show code examples, 'eval_file' to load and execute a Lisp file, 'lint' to check syntax without executing, 'source' to view source of a builtin/stdlib function (use PLAIN function name in expression, NOT Lisp syntax), 'source-list' to list all indexed functions (use optional filter string in expression), 'xref' to show callers/callees of a function (use PLAIN function name, limit=context lines), 'xref-list' to browse all cross-reference relationships (use optional filter string in expression), 'skill' to load a comprehensive guide teaching how to use this Lisp interpreter (expression='ffi' for FFI guide, expression='ops' for operations guide, expression='xref' for cross-reference guide, or empty for full guide).",
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

The lisp_eval tool supports these operations (set via the "operation" parameter):

--- eval (default) ---
Evaluate a Lisp expression. State persists between calls.
  expression: "(+ 1 2)"          => 3
  expression: "(defun square (x) (* x x))"  => defines function
  expression: "(square 7)"       => 49
  limits: "default" | "strict" | "unlimited"

--- reset ---
Clear all interpreter state (variables, functions, macros, classes).
  No parameters needed.

--- help ---
Show usage manual for a topic.
  expression: ""        => list all topics
  expression: "arithmetic" => show arithmetic operations
  expression: "lambda"  => show lambda/closures guide
  Topics: arithmetic, lists, strings, lambda, recursion, format, hash, arrays, clos, conditions, macros, types, special

--- examples ---
Show example expressions for a category.
  expression: ""        => list all categories
  expression: "arithmetic" => show arithmetic examples
  Categories: arithmetic, lists, lambda, recursion, map, strings, format, hash, arrays, misc

--- eval_file ---
Load and execute a Lisp file.
  file: "/path/to/file.lisp" (required)
  limits: "default" | "strict" | "unlimited"

--- lint ---
Check syntax without executing. Use expression or file.
  expression: "(defun foo (x) x)"  => check syntax
  file: "/path/to/file.lisp"       => check file syntax
  limits: "default" | "strict" | "unlimited"

--- source ---
View source code of a builtin or stdlib function.
  expression: "car"            => show car source (PLAIN NAME, not Lisp syntax!)
  expression: "string-append"  => show string-append source
  offset: 0 (line offset for large functions)
  limit: 50 (max lines to show)
  IMPORTANT: Use a plain function name, NOT a Lisp expression.
  Correct: expression="car"  WRONG: expression="(source 'car)"

--- source-list ---
List all indexed functions, optionally filtered.
  expression: ""       => list all functions
  expression: "car"    => list functions containing "car"
  expression: "string" => list functions containing "string"
  offset: 0 (entry offset for pagination)
  limit: 50 (max entries per page)

--- xref ---
Show cross-reference: callers and callees of a function.
  expression: "car"            => who calls car, and what car calls
  expression: "string-append"  => callers/callees of string-append
  limit: 2 (context lines around each call site)
  IMPORTANT: Use a plain function name, NOT a Lisp expression.

--- xref-list ---
Browse all cross-reference relationships, optionally filtered.
  expression: ""       => list all caller->callee pairs
  expression: "string" => list pairs involving "string"
  offset: 0 (entry offset for pagination)
  limit: 50 (max entries per page)

--- skill ---
Load a comprehensive guide for learning this Lisp interpreter.
  expression: ""     => full guide (all topics)
  expression: "ffi"  => FFI/Go interop guide
  expression: "ops"  => operations guide
  expression: "xref" => cross-reference guide

--- Resource Limits ---
  "default"  — 1M steps, 30s timeout, 256MB heap (normal use)
  "strict"   — 100K steps, 10s timeout, 64MB heap (untrusted code)
  "unlimited" — no limits (REPL mode only, use carefully)`,

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

This guide teaches you everything you need to use this Lisp interpreter effectively.
Use operation=skill with expression=<topic> for detailed guides on specific topics.

--- Quick Start ---
1. Evaluate expressions:  operation=eval, expression="(+ 1 2)"
2. Define functions:      operation=eval, expression="(defun square (x) (* x x))"
3. Call Go functions:     operation=eval, expression="((ffi \"math.Sqrt\") 2.0)"
4. View source code:      operation=source, expression="car"
5. Find callers:          operation=xref, expression="car"
6. List all functions:    operation=source-list, expression=""
7. Get help on topics:    operation=help, expression="arithmetic"
8. See examples:          operation=examples, expression="lists"

--- Core Lisp Features ---
- Arbitrary-precision integers, rationals, complex numbers
- Lists, strings, hash tables, arrays, structs
- Lambda, closures, macros, CLOS (object system)
- Conditions/restarts (error handling)
- Format directives (~A, ~D, ~F, ~%, ~{~})
- State persists between calls (defvar, defun survive)

--- FFI: Calling Go Standard Library ---
Import: (ffi "package.Function") or (go:import "package.Function")
Call:   ((ffi "math.Sin") 1.0)
Vars:   (go:import "crypto/rand.Reader") => #<go-val *rand.reader>
List:   (go:list) or (go:list "math")
VGoVal: Go interface/pointer values are preserved — can be passed to other FFI functions
See:    operation=skill, expression="ffi" for full FFI guide

--- Tool Operations ---
eval, reset, help, examples, eval_file, lint, source, source-list, xref, xref-list, skill
See:    operation=skill, expression="ops" for full operations guide

--- Cross-Reference ---
Who calls X?   operation=xref, expression="car"
What does X call? operation=xref, expression="filter"
Browse all:     operation=xref-list, expression=""
See:            operation=skill, expression="xref" for full xref guide

--- Resource Limits ---
default:  1M steps, 30s, 256MB — normal use
strict:   100K steps, 10s, 64MB — untrusted code
unlimited: no limits — REPL only

--- Detailed Guides ---
  operation=skill, expression="ffi"   — FFI/Go interop guide
  operation=skill, expression="ops"   — operations guide
  operation=skill, expression="xref"  — cross-reference guide`
}
