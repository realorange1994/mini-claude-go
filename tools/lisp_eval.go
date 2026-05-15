package tools

import (
	"context"
	"fmt"

	"miniclaudecode-go/microlisp"
)

// LispEvalTool evaluates Common Lisp expressions for arithmetic,
// data structures, logic, and computation.
type LispEvalTool struct{}

func (*LispEvalTool) Name() string { return "lisp_eval" }

func (*LispEvalTool) Description() string {
	return `Evaluate Common Lisp expressions. Use for arithmetic (+ 2 3), list operations (car '(1 2 3)), string manipulation, hash tables, arrays, format strings, recursion, lambda functions, CLOS objects, and more. Also provides help/manual (operation="help"), code examples (operation="examples"), and state reset (operation="reset").`
}

func (*LispEvalTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"expression": map[string]any{
				"type":        "string",
				"description": "Lisp expression to evaluate, e.g. (+ 1 2), (car '(1 2 3)), (format nil \"Hello ~A\" \"World\"). For operation=help: use topic name like \"arithmetic\", \"lists\", \"lambda\". For operation=examples: use category name like \"arithmetic\", \"recursion\", \"format\".",
			},
			"operation": map[string]any{
				"type":        "string",
				"enum":        []string{"eval", "reset", "help", "examples"},
				"description": "Action to perform: 'eval' to evaluate a Lisp expression (default), 'reset' to clear interpreter state, 'help' to show usage manual, 'examples' to show code examples.",
			},
		},
		"required": []string{},
	}
}

func (*LispEvalTool) CheckPermissions(params map[string]any) PermissionResult {
	return PermissionResultPassthrough()
}

func (t *LispEvalTool) ExecuteContext(ctx context.Context, params map[string]any) ToolResult {
	// Check context early
	select {
	case <-ctx.Done():
		return ToolResult{Output: fmt.Sprintf("Error: lisp_eval timed out: %v", ctx.Err()), IsError: true}
	default:
	}

	op, _ := params["operation"].(string)
	expr, _ := params["expression"].(string)

	switch op {
	case "reset":
		microlisp.ResetGlobalEnv()
		return ToolResult{Output: "Lisp interpreter state has been reset. All user-defined variables, functions, macros, and classes have been cleared."}

	case "help":
		return ToolResult{Output: lispHelp(expr)}

	case "examples":
		return ToolResult{Output: lispExamples(expr)}

	default: // eval (default when operation is empty or unknown)
		if expr == "" {
			return ToolResult{Output: "Error: expression is required. Examples: (+ 1 2) => 3, (car '(1 2 3)) => 1", IsError: true}
		}
		// Run eval in a goroutine so we can respect context cancellation.
		// The microlisp interpreter holds evalMu during execution, so
		// we can't cancel mid-evaluation, but we can abort waiting for it.
		type evalResult struct {
			output string
			err    error
		}
		ch := make(chan evalResult, 1)
		go func() {
			result, err := microlisp.SafeEvalString(expr)
			ch <- evalResult{result, err}
		}()
		select {
		case <-ctx.Done():
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
