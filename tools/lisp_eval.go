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
	return "Evaluate Common Lisp CODE / EXPRESSIONS — " +
		"NOT a command runner. This is a Lisp interpreter, NOT os/exec. " +
		"Do NOT use this to run system commands like go, python, npm, ls — use lisp_exec for that. " +
		"Use this to evaluate Lisp code only. State persists between calls. " +
		"Quick start: expression=\"(+ 1 2)\". " +
		"Use operation=\"define\" to see function signatures (params, return types). " +
		"Use operation=\"help\" for topic docs, \"examples\" for code samples, " +
		"\"skill\" for a complete usage guide. FFI: (ffi \"math.Sqrt\") calls Go stdlib."
}

func (*LispEvalTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"description": "Evaluates Lisp CODE / EXPRESSIONS via a Lisp interpreter. " +
			"NOT for running system commands (go, python, npm, shell) — use lisp_exec for that. " +
			"Do NOT use this to execute CLI tools — it is a Lisp evaluator only.",
		"properties": map[string]any{
			"expression": map[string]any{
				"type":        "string",
				"description": "Lisp expression to evaluate (e.g. (+ 1 2), (car '(1 2 3))). This is Lisp CODE, NOT a shell command. For system commands, use lisp_exec. For source/xref operations: use a PLAIN function name only (e.g. \"car\"). For help/skill: use a topic name like \"arithmetic\" or \"ffi\".",
			},
			"operation": map[string]any{
				"type":        "string",
				"enum":        []string{"eval", "reset", "help", "examples", "eval_file", "lint", "source", "source-list", "xref", "xref-list", "skill", "define"},
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
		// Run ResetGlobalEnv in a goroutine so we can respect context
		// cancellation. ResetGlobalEnv acquires evalMu, which may be held
		// by a prior eval that's still running. Without this, reset would
		// block indefinitely on evalMu, causing a 10-minute timeout.
		ch := make(chan evalResult, 1)
		go func() {
			defer func() {
				if r := recover(); r != nil {
					ch <- evalResult{"", fmt.Errorf("panic: %v", r)}
				}
			}()
			microlisp.ResetGlobalEnv()
			ch <- evalResult{"Lisp interpreter state has been reset. All user-defined variables, functions, macros, and classes have been cleared.", nil}
		}()
		select {
		case <-ctx.Done():
			return ToolResult{Output: "Error: lisp_eval timed out during reset (evalMu may be held by a prior evaluation)", IsError: true}
		case r := <-ch:
			if r.err != nil {
				return ToolResult{Output: fmt.Sprintf("Error: %v", r.err), IsError: true}
			}
			return ToolResult{Output: r.output}
		}

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
		if file != "" {
			ch := make(chan evalResult, 1)
			go func() {
				defer func() {
					if r := recover(); r != nil {
						ch <- evalResult{"", fmt.Errorf("panic: %v", r)}
					}
				}()
				err := microlisp.SafeLintFileWithLimits(file, limits)
				if err != nil {
					ch <- evalResult{"", err}
				} else {
					ch <- evalResult{"No syntax errors found.", nil}
				}
			}()
			select {
			case <-ctx.Done():
				close(cancelChan)
				return ToolResult{Output: "Error: lisp_eval timed out during lint", IsError: true}
			case r := <-ch:
				if r.err != nil {
					return ToolResult{Output: fmt.Sprintf("Lint error: %v", r.err), IsError: true}
				}
				return ToolResult{Output: r.output}
			}
		}
		if expr == "" {
			return ToolResult{Output: "Error: expression or file is required for operation=lint", IsError: true}
		}
		ch := make(chan evalResult, 1)
		go func() {
			defer func() {
				if r := recover(); r != nil {
					ch <- evalResult{"", fmt.Errorf("panic: %v", r)}
				}
			}()
			err := microlisp.SafeLintWithLimits(expr, limits)
			if err != nil {
				ch <- evalResult{"", err}
			} else {
				ch <- evalResult{"No syntax errors found.", nil}
			}
		}()
		select {
		case <-ctx.Done():
			close(cancelChan)
			return ToolResult{Output: "Error: lisp_eval timed out during lint", IsError: true}
		case r := <-ch:
			if r.err != nil {
				return ToolResult{Output: fmt.Sprintf("Lint error: %v", r.err), IsError: true}
			}
			return ToolResult{Output: r.output}
		}

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
			// Wait for the eval goroutine to actually finish. stepCheck()
			// checks CancelChan every 1024 steps; we must wait for evalMu
			// to be released, otherwise subsequent evals will deadlock.
			<-ch
			return ToolResult{Output: "Error: lisp_eval timed out evaluating expression", IsError: true}
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
  (+ a b c...)     Addition (arbitrary precision integers, rationals, floats, complex)
  (- a b)          Subtraction; (- a) negates
  (* a b c...)     Multiplication
  (/ a b)          Division (returns rational if no remainder, float if remainder)
  (mod n m)        Modulo
  (expt b e)       Exponentiation: b^e
  (sqrt n)         Square root
  (abs n)          Absolute value
  (sin/cos/tan n)  Trigonometric functions (asin, acos, atan, atan2 also available)
  (sinh/cosh/tanh) Hyperbolic functions (asinh, acosh, atanh also)
  (floor n)        Floor division
  (ceiling n)      Ceiling
  (round n)        Round to nearest integer
  (truncate n)     Truncate toward zero
  (ffloor/fceiling/ftruncate/fround)  Float variants
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

Bit Operations:
  (logand a b) / (logior a b) / (logxor a b) / (lognot a)
  (lognand/lognor/logandc1/logandc2/logorc1/logorc2)
  (logcount/logbitp/logtest/integer-length)
  (ash n count)  Arithmetic shift
  (byte size pos)  Byte specifier
  (ldb/ldb-test/mask-field/deposit-field)
  (boole op a b)  With 16 boole constants

Complex Numbers:
  (complex r i)  Create complex number
  (realpart c) / (imagpart c)  Extract parts
  (conjugate c) / (phase c) / (cis theta)

Constants: pi, most-positive-fixnum, most-negative-fixnum, char-code-limit,
  most-positive-{short/single/double/long}-float, most-negative-...,
  float-{epsilon/epsilon/negative-epsilon}/...
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
  (member-if fn l)  First element satisfying predicate
  (assoc x alist)  Association list lookup
  (rassoc x alist)  Reverse assoc (by cdr)
  (assoc-if fn alist) / (rassoc-if fn alist)
  (push x lst)     Push element onto list
  (pop lst)        Pop element from list
  (pushnew x lst)  Push if not already member
  (nreverse l)     Destructive reverse
  (nconc l1 l2)    Destructive append
  (list* a b c)    Improper list constructor
  (last l)         Last cons of a list
  (butlast l)      All but last element
  (ldiff list tail)  Copy list up to tail
  (subst/new/old l)  Substitute elements
  (sublis alist tree)  Alist substitution
  (tree-equal t1 t2)  Tree equality
  (union/intersection/set-difference/set-exclusive-or/subsetp)  Set operations`,

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
  (parse-integer s)  Parse integer with :start/:end/:radix/:junk-allowed
  (char s i)        Get character at index
  (char-setf s i ch) Set character at index
  (schar s i)       Character access (no bounds check)
  (make-string len &key initial-element element-type)
  (string-equal a b)  Case-insensitive string equality
  (string/= / string< / string> / string<= / string>=)  Ordering
  (string-not-equal / string-greaterp / string-lessp)  Inequality
  (string-not-greaterp / string-not-lessp)
  (string-upcase / string-downcase / string-capitalize)
  (nstring-upcase / nstring-downcase / nstring-capitalize)  Destructive
  (string-trim / string-left-trim / string-right-trim chars string)
  (string= a b)  Case-sensitive equality
  Character Predicates: alpha-char-p, upper-case-p, lower-case-p, both-case-p,
    digit-char-p, alphanumericp, graphic-char-p, standard-char-p`,

		"lambda": `Lambda and Closures:
  (lambda (args) body...)  Anonymous function
  ((lambda (x) (* x x)) 5)  => 25
  (define (name args) body...)  Define a function
  (defun name (args) body...)   Define a function (CL style)
  (funcall fn args...)    Call a function
  (apply fn list)         Apply function to list of args
  (let ((var val)...) body...)  Lexical binding
  (let* ((var val)...) body...) Sequential let
  (letrec ...)            Recursive let
  (labels ((name args body)...) body...)  Local functions
  (flet ((name args body)...) body...)    Local non-recursive functions
  (progn body...)      Execute body, return last result
  (begin body...)      Same as progn
  (if test then [else])  Conditional
  (cond (test1 body1) (test2 body2) ...)  Multi-way conditional
  (when test body...)   Execute if test is true
  (unless test body...) Execute if test is false
  (and a b c...)       Short-circuit AND
  (or a b c...)        Short-circuit OR
  (not x)              Logical NOT
  (case key (val body...) (otherwise body...))
  (ecase key (val body...))  Error if no match
  (typecase var (type body...) (otherwise body...))
  (etypecase / ctypecase)
  Multiple values: (values a b...), (multiple-value-bind (a b) form body...),
    (nth-value n form), (multiple-value-list form)
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
  (loop for x in '(1 2 3) collect (* x x))
  (loop with acc = 0 for i from 1 to 10 do (setf acc (+ acc i)) finally (return acc))
  (dotimes (i 10) (print i))
  (dolist (x '(1 2 3)) (print x))
  (do ((i 0 (1+ i))) ((>= i 10)) (print i))
  (do* ((i 0 (1+ i))) ((>= i 10)))
  (block name body...) / (return-from name value) / (return name value)
  (tagbody body... (go label) ... label ...)
  (loop-finish)  Exit from loop construct`,

		"format": `Format and Output:
  (format destination control-string args...)
  Destinations: nil (returns string), t (stdout), stream
  Control sequences:
    ~A  Aesthetic (print without quotes)
    ~S  Standard (print with quotes/escaping)
    ~D  Decimal integer
    ~B  Binary, ~O  Octal, ~X  Hexadecimal
    ~F  Fixed-point float
    ~E  Exponential float
    ~G  General float
    ~$  Dollars float (financial)
    ~%  Newline
    &~  Fresh line
    ~~  Literal tilde
    ~_  Conditional newline
    ~T  Tabulation
    ~{...~}  Iterate over list
    ~^  Escape from ~{~} iteration
    ~R  Roman numeral or radix
    ~P  Plural ("s" if arg != 1)
    ~@R  Roman numeral with sign
  Modifiers: ~<mincol,colinc,minpad,padchar>R, ~<width,digits> format floats
  Examples:
  (format nil "Hello, ~A!" "World")  => "Hello, World!"
  (format t "~D + ~D = ~D~%" 2 3 (+ 2 3))
  (format nil "~{~A, ~}" '(1 2 3))  => "1, 2, 3, "`,

		"hash": `Hash Tables:
  (make-hash-table &key test size rehash-size rehash-threshold)
  (hash-table-p x)
  (gethash key ht)   Get value (returns value, found-p)
  (gethash key ht default)  Get value with default
  (setf (gethash key ht) val)  Set value
  (remhash key ht)   Remove entry
  (clrhash ht)       Clear hash table
  (maphash fn ht)    Apply function to each key-value pair
  (hash-table-count ht)  Number of entries
  (hash-table-size ht)  Current size
  (hash-table-test ht)  Test function (eql, equal, eq)
  (hash-table-rehash-size ht)  Rehash size
  (hash-table-rehash-threshold ht)  Rehash threshold
  (hash-table-keys ht)  List of keys
  (hash-table-values ht)  List of values
  (hash-table-exists? key ht)  Check key exists
  (sxhash obj)       Hash code for object
  Tests: :eql (default), :equal, :eq`,

		"arrays": `Arrays:
  (make-array dims &key initial-element initial-contents
              element-type adjustable fill-pointer)
  (make-vector len)  Create 1D array
  (vector &rest items)  Create vector from items
  (aref array i...)  Access element
  (svref vector i)   Simple vector access
  (setf (aref array i...) val)  Set element
  (row-major-aref array idx)  Row-major access
  (arrayp x) / (vectorp x) / (bit-vector-p x)  Predicates
  (simple-vector-p / simple-bit-vector-p / simple-string-p)
  (array-rank array)  Number of dimensions
  (array-dimension array n)  Size of dimension n
  (array-dimensions array)  All dimensions as list
  (array-total-size array)  Total elements
  (array-element-type array)  Element type
  (array-has-fill-pointer-p array)
  (adjustable-array-p array)
  (array-in-bounds-p array indices...)  Bounds check
  (array-row-major-index array indices...)  Convert to row-major index
  (fill-pointer array) / (set-fill-pointer array n)
  (vector-push elem vector)  Push if fill-pointer < size
  (vector-push-extend elem vector)  Push with auto-extend
  (vector-pop vector)  Pop from fill-pointer
  (adjust-array array new-dims &key fill-pointer)  Resize
  (bit array indices...) / (sbit array indices...)  Bit access
  (upgraded-array-element-type type)`,

		"clos": `CLOS (Common Lisp Object System):
  (defclass name (superclasses) (slot-specs...) options...)
  (defclass person () ((name :initarg :name :accessor person-name)
                        (age :initarg :age :accessor person-age)))
  (make-instance 'class-name &key initargs...)
  (defgeneric fn-name (args...) &key method-combination)
  (defmethod fn-name qualifier ((arg class-name) ...) body...)
    Qualifiers: :before, :after, :around
    EQL specializers: (arg (eql value))
    Class specializers: (arg 'class-name)
  Method combinations: standard, progn, and, or, list, append, nconc, min, max, +
  (set-method-combination gf-name combination)
  (slot-value obj 'slot-name)  Access slot
  (setf (slot-value obj 'slot) val)  Set slot
  (slot-boundp obj 'slot-name)  Check if slot has value
  (slot-exists-p obj 'slot-name)  Check if slot exists
  (slot-makunbound obj 'slot)  Remove slot value
  (class-of obj)  Get class of object
  (class-name class)  Get class name
  (find-class name)  Find class by name
  (is-a? obj class-name)  Instance-of predicate
  (instance? obj)  Check if CLOS instance
  (class-slots class) / (class-slot-defs class)
  (find-method gf-name qualifiers specializers)
  (remove-method gf-name method)
  (compute-applicable-methods gf args)
  (method-qualifiers method)
  (generic-function-p obj)
  (call-next-method) / (next-method-p)  Inside methods
  C3 linearization for class precedence lists
  (copy-structure struct)`,

		"conditions": `Conditions and Error Handling:
  (define-condition name (superclass) (slot-specs...))
  (make-condition 'name &key slot-values...)
  (signal condition)  Signal a condition (non-fatal)
  (error "message")  Signal an error (fatal)
  (cerror "continue-format" "error-format" args...)  Continuable error
  (warn "message")   Signal a warning
  (break &optional fmt args...)  Enter debugger
  (handler-case form
    (condition-type (var) handler-body...)
    (:no-error (result) body...))
  (handler-bind ((condition-type handler)...) body...)
  (restart-case form
    (restart-name () handler-body...))
  (restart-bind ((name fn)...) body...)
  (invoke-restart restart-name &optional value)
  (compute-restarts &optional condition)
  (find-restart name &optional condition)
  (restart-name restart)
  Standard restarts: abort, continue, muffle-warning,
    store-value, use-value
  (ignore-errors form...)  Catch errors, return NIL+error
  Condition type hierarchy matching via class system`,

		"macros": `Macros:
  (define-macro (name args) body...)  Define a macro
  (defmacro name (args) body...)  CL-style macro
  (backquote/quasiquote) with , (unquote) and ,@ (splice)
  Macro expansion:
  (define-macro (unless test . body)
    (list 'if test (list 'progn) (cons 'begin body)))
  (macroexpand-1 form)  Expand macro once
  (macroexpand form)    Fully expand macro
  (macro-function symbol)  Get macro function
  (setf (macro-function symbol) fn)  Set macro function
  Compiler macros (define-compiler-macro) for optimization
  (compiler-macro-function symbol)
  (symbol-macrolet ((sym expansion)...) body...)
  (macrolet ((name args body)...) body...)  Local macros`,

		"types": `Type System:
  (type-of x)       Runtime type of value
  (typep x type)    Check if value is of type
  (subtypep a b)    Check if type A is subtype of B
  (deftype name (args) body...)  Define new type
  (constantp form)  Check if compile-time constant
  Type specifiers:
    number, integer, float, rational, complex
    string, symbol, character, list, cons
    array, hash-table, readtable, package
    stream, function, null, boolean, channel
    (integer 0 100)  Bounded integer type
    (array fixnum (*))  1D array of fixnums
    go-val  For opaque Go values`,

		"special": `Special Variables and Symbols:
  *print-base*      Output number base (default 10)
  *print-radix*     Show radix prefix (e.g., #x for hex)
  *print-case*     Symbol case: :UPCASE, :DOWNCASE, :CAPITALIZE
  *read-base*       Input number base
  *random-state*    Current random state
  (random-state-p x) Check if random state
  (make-random-state &optional state)  Create random state
  (copy-random-state state)  Copy random state
  *posix-argv*      Command line arguments
  (boundp 'symbol)   Check if symbol has a value
  (fboundp 'symbol)  Check if symbol has a function
  (gensym)           Generate a unique symbol
  (gentemp)          Generate a unique temp symbol
  (copy-symbol sym)  Copy symbol with property list
  (symbol-value s)   Get symbol's value
  (symbol-function s) Get symbol's function
  (symbol-plist s)   Get symbol's property list
  (symbol-package s) Get symbol's package
  (set 'sym val)     Set symbol's value
  (makunbound sym)   Remove symbol's value
  (fmakunbound sym)  Remove symbol's function
  (get sym prop) / (putprop sym val prop) / (remprop sym prop)
  (getf plist prop) / (remf plist prop)
  (special-operator-p sym)  Check if special operator
  (functionp obj) / (compiled-function-p obj)
  (identity x) / (complement fn) / (constantly x)`,

		"concurrency": `Concurrency (Channels, Threads, Locks, Atomics):
  Channels:
    (make-channel &optional buf-size)  Create channel
    (chan-send ch val) / (go:send ch val)  Blocking send
    (chan-recv ch) / (go:recv ch)  Blocking recv
    (chan-try-send ch val)  Non-blocking send -> (:ok)/:would-block/:closed
    (chan-try-recv ch)  Non-blocking recv -> (:ok val)/:would-block/:closed
    (chan-close ch)  Close channel
    (chan-p val)  Channel predicate
    (chan-info ch) -> (:buffer N :closed T/NIL)
    (go:select (:send ch val) (:recv ch) (:default))
    (chan-select-timeout ms (:recv ch) (:default))
  Threads:
    (go:spawn (lambda () body...))  Spawn thread
    (make-thread (lambda () body...))  Create thread
    (join-thread thread-id)  Wait for thread
    (thread? val)  Thread predicate
  Locks:
    (make-lock)  Create mutex
    (lock l) / (unlock l)  Acquire/release
    (lock? val)  Lock predicate
  Condition Variables:
    (make-condvar)  Create condition variable
    (condition-wait condvar lock)  Wait (releases lock)
    (condition-notify condvar)  Signal one waiter
    (condition-broadcast condvar)  Signal all waiters
    (condvar? val)  Condition predicate
  Atomics:
    (atomic-incf place)  Atomic increment
    (atomic-decf place)  Atomic decrement
    (atomic-get place) / (atomic-set place val)
  Special Forms:
    (go:recover body...)  Panic recovery
    (go:with-defer body...)  Go-style defer`,

		"ffi": `Go FFI Interop (go:* functions):
  (go:import "pkg.Func")  Import Go function/variable
  (ffi "pkg.Func")  Alias for go:import
  (go:list) / (go:list "pkg")  List packages/symbols
  (go:register "pkg.name" value)  Register custom Go value
  Structs:
    (go:new "pkg.Type")  Create zero-value struct
    (go:field obj "Field")  Read field
    (go:set-field obj "Field" val)  Set field
    (go:fields-of "pkg.Type")  List struct fields
    (go:methods-of obj)  List methods
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
    (go:uintptr obj)  Pointer address
    (go:search pattern)  Search symbols
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
    Signatures: "int32->bool", "int32->int32", "int->int",
      "int->bool", "int,int->bool", "int,int->", "string->string",
      "string->error", "int32->bool", "int,int->", "()->"
  VGoVal: Opaque Go values preserving type info for reflection.
    Automatically unwrapped when passed to compatible FFI functions.`,

		"sequences": `Sequences (CL-compatible):
  (map result-type fn seq...)  Map over sequences
  (mapcar fn lst...) / (mapcan fn lst...)
  (maplist fn lst) / (mapc fn lst) / (mapl fn lst) / (mapcon fn lst)
  (map-into result-seq fn seq...)  Destructive map
  (reduce fn seq &key initial-value from-end start end)
  (find item seq &key key test test-not start end from-end)
  (find-if pred seq &key start end from-end)
  (count item seq &key test start end)
  (count-if pred seq &key start end)
  (remove item seq &key test start end count)
  (remove-if pred seq &key start end count)
  (remove-duplicates seq &key test start end)
  (substitute new old seq &key test start end count)
  (substitute-if new pred seq &key start end count)
  (position item seq &key test start end from-end)
  (position-if pred seq &key start end from-end)
  (sort seq pred &key key) / (stable-sort seq pred &key key)
  (fill seq item &key start end)
  (replace seq1 seq2 &key start1 end1 start2 end2)
  (search seq1 seq2 &key test test-not key start1 end1 start2 end2)
  (mismatch seq1 seq2 &key test key from-end start1 end1 start2 end2)
  (copy-seq seq) / (subseq seq start &optional end)
  (concatenate result-type seq...)  Concatenate sequences
  (merge result-type seq1 seq2 pred &key key)
  (some pred seq...) / (every pred seq...)
  (notany pred seq...) / (notevery pred seq...)`,

		"packages": `Package System:
  (make-package name &key nicknames use)  Create package
  (in-package name)  Switch to package
  (find-package name)  Find package by name/nickname
  (intern name &optional package)  Intern symbol
  (export symbols &optional package)  Export symbols
  (unexport symbols &optional package)
  (import symbols &optional package)
  (shadow symbols &optional package)
  (shadowing-import symbols &optional package)
  (use-package packages &optional package)
  (unuse-package packages &optional package)
  (unintern symbol &optional package)
  (rename-package package new-name nicknames)
  (delete-package package)
  (find-symbol name &optional package)  -> symbol, status
  (keywordp x)  Check if keyword
  (symbol-package sym)  Get symbol's package
  (package-name pkg) / (package-nicknames pkg)
  (package-use-list pkg) / (package-used-by-list pkg)
  (list-all-packages)
  (package-symbols pkg) / (package-external-symbols pkg)
  (package-shadowing-import-list pkg)`,

		"io": `I/O and Streams:
  (open path &key direction if-exists if-does-not-exist element-type)
  (close stream &key abort)
  (read stream) / (read-line stream) / (read-char stream)
  (read-byte stream) / (read-char-no-hang stream)
  (read-from-string str)  Parse from string
  (peek-char &optional recursive-p stream)
  (unread-char char stream)
  (read-sequence seq stream)
  (read-preserving-whitespace stream)
  (write object &key stream) / (print object stream)
  (prin1 object stream) / (princ object stream)
  (write-char char stream) / (write-string str stream)
  (write-line str stream) / (write-byte byte stream)
  (write-sequence seq stream) / (terpri stream) / (fresh-line stream)
  (force-output stream) / (finish-output stream) / (clear-output stream)
  (read-delimited-list delim-char stream)
  (make-string-input-stream str) / (make-string-output-stream)
  (get-output-stream-string stream)
  (file-string-length stream) / (file-length stream)
  (file-position stream &optional position)
  Stream Predicates:
    (streamp x) / (input-stream-p x) / (output-stream-p x)
    (open-stream-p x) / (interactive-stream-p x)
    (string-stream-p x) / (echo-stream-p x) / (synonym-stream-p x)
    (broadcast-stream-p x) / (concatenated-stream-p x) / (two-way-stream-p x)
  Composite Streams:
    (make-synonym-stream symbol) / (make-broadcast-stream streams...)
    (make-concatenated-stream streams...) / (make-two-way-stream in out)
    (make-echo-stream in out)
  Composite Accessors:
    (echo-stream-input-stream s) / (echo-stream-output-stream s)
    (synonym-stream-symbol s) / (broadcast-stream-streams s)
    (concatenated-stream-streams s)
    (two-way-stream-input-stream s) / (two-way-stream-output-stream s)
  Stream Adapters:
    (reader-read-all reader)  Read all from io.Reader
    (io-copy-to-string reader)  Copy to string
    (io-copy-to-file reader path)  Copy to file
    (io-limit-string reader n)  Read N bytes
    (io-nop-closer reader)  Wrap as ReadCloser
    (listener-p stream) / (clear-input stream)
    (y-or-n-p &optional fmt) / (yes-or-no-p &optional fmt)`,

		"pathnames": `Pathnames:
  (make-pathname &key host device directory name type version)
  (pathname thing)  Coerce to pathname
  (pathname-host p) / (pathname-device p) / (pathname-directory p)
  (pathname-name p) / (pathname-type p) / (pathname-version p)
  (namestring p) / (file-namestring p) / (directory-namestring p)
  (host-namestring p) / (enough-namestring p)
  (parse-namestring thing &optional host defaults)
  (merge-pathnames path &optional defaults)
  (pathnamep x)  Pathname predicate
  (user-homedir-pathname)
  (probe-file path)  Check if file exists
  (directory pathspec)  List directory contents
  (file-author path) / (file-write-date path)
  (delete-file path) / (rename-file file new)
  (ensure-directories-exist path)
  (truename path)
  (wild-pathname-p p) / (pathname-match-p p wildcard)
  (logical-pathname p) / (translate-logical-pathname p)
  (logical-pathname-translations host)
  (translate-pathname path from-wildcard to-wildcard)
  (directory-pathname-p p)`,

		"readtable": "Readtable System:\n" +
			"  (make-readtable &key case base)  Create readtable\n" +
			"  (copy-readtable &optional from-readtable)\n" +
			"  (readtable-case rt) / (set-readtable-case rt case)\n" +
			"  (set-macro-character char fn &optional non-terminating-p rt)\n" +
			"  (get-macro-character char &optional rt)\n" +
			"  (set-syntax-from-char to-char from-char &optional rt)\n" +
			"  (make-dispatch-macro-character char &optional non-terminating-p rt)\n" +
			"  (set-dispatch-macro-character disp-char sub-char fn rt)\n" +
			"  (get-dispatch-macro-character disp-char sub-char rt)\n" +
			"  (readtablep x)  Readtable predicate\n" +
			"  *readtable*  Current readtable\n" +
			"  #\\\"  String reader\n" +
			"  #\\\\  Character reader\n" +
			"  #\\\\|  Single-line comment\n" +
			"  #\\\\#\\\\|  Multi-line comment\n" +
			"  #\\\\(  List reader\n" +
			"  #'  Function quote\n" +
			"  #\x60  Backquote\n" +
			"  #'x  (function x)\n" +
			"  #.x  (eval x) at read time",

		"time": `Time:
  (get-universal-time)  Seconds since 1900-01-01 00:00:00 UTC
  (decode-universal-time ut &optional zone)
    -> (second minute hour date month year day-of-week daylight-p zone)
  (encode-universal-time sec min hour date month year &optional zone)
  (get-internal-real-time)  Milliseconds since program start
  (get-internal-run-time)
  (sleep seconds)  Sleep for N seconds
  Constants: pi, internal-time-units-per-second`,
	}

	if topic == "" {
		return `Lisp Interpreter Manual — Available Topics:
  arithmetic   Math: +, -, *, /, mod, expt, sqrt, sin/cos/tan, complex, bit ops
  lists        Cons, car, cdr, list, append, member, assoc, set operations
  strings      String/char ops, comparison, case conversion, trimming
  lambda       Anonymous functions, define, let, labels, flet, case, typecase
  recursion    Factorial, fibonacci, loops (loop/dotimes/dolist/do), block/tagbody
  format       Format strings: ~A, ~D, ~F, ~%, ~{~}, ~R, ~P, ~@
  hash         Hash tables: make-hash-table, gethash, maphash, :eql/:equal/:eq
  arrays       Multi-D arrays, fill-pointer, vector-push/pop, adjust-array
  clos         Object system: defclass, defmethod, defgeneric, method combinations
  conditions   Error handling: handler-case, restart-case, signal, error, cerror
  macros       define-macro, defmacro, backquote, macrolet, symbol-macrolet
  types        type-of, typep, subtypep, deftype, bounded types
  special      *print-base*, *random-state*, gensym, boundp, property lists
  concurrency  Channels, threads, locks, condition variables, atomics
  ffi          Go FFI: go:import, go:new, go:call, go:make, go:field, go:callback
  sequences    mapcar, reduce, find, remove, sort, search, position, substitute
  packages     make-package, in-package, find-symbol, export, import, use-package
  io           Streams: open, read, write, make-string-input/output-stream
  pathnames    make-pathname, directory, probe-file, translate-pathname
  readtable    make-readtable, set-macro-character, set-dispatch-macro-character
  time         get-universal-time, decode-universal-time, encode-universal-time

Use lisp_eval with operation="help" and expression=<topic> for details.
Example: lisp_eval with operation="help", expression="arithmetic"`
	}

	for k, v := range topics {
		if k == topic {
			return v
		}
	}
	return fmt.Sprintf("Unknown topic: %s\n\nAvailable: arithmetic, lists, strings, lambda, recursion, format, hash, arrays, clos, conditions, macros, types, special, concurrency, ffi, sequences, packages, io, pathnames, readtable, time", topic)
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
  (zerop 0)                      => t
  (logand #b1010 #b1100)         => 8 (#b1000)
  (ash 1 4)                      => 16
  (complex 3 4)                  => #C(3 4)
  (realpart #C(3 4))             => 3`,

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
  (member 'b '(a b c))           => (b c)
  (push 'x lst)                  => (x ...)
  (pop lst)                      => x
  (union '(1 2 3) '(3 4 5))      => (1 2 3 4 5)`,

		"lambda": `Lambda and Function Examples:
  ((lambda (x) (* x x)) 5)       => 25
  (define (square x) (* x x))
  (square 7)                     => 49
  (funcall (lambda (a b) (+ a b)) 3 4)  => 7
  (let ((x 10) (y 20)) (+ x y))  => 30
  (let* ((x 5) (y (+ x 1))) (* x y)) => 30
  (if (> 5 3) "yes" "no")        => "yes"
  (cond ((> 5 3) "big") ((= 5 3) "equal") (t "small"))  => "big"
  (case 'b ((a) 1) ((b) 2) (t 3)) => 2`,

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
  (code-char 65)                   => #\A
  (string-trim '(#\Space #\Tab) "  hi  ")  => "hi"`,

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

		"ffi": `FFI Examples:
  ((ffi "math.Sqrt") 2.0)        => 1.414...
  ((ffi "math.Pow") 2 10)        => 1024.0
  ((ffi "strings.Contains") "hello" "ell") => t
  ((ffi "fmt.Sprintf") "Hello %s" "World") => "Hello World"
  (defvar sin-fn (ffi "math.Sin"))
  (funcall sin-fn 1.5708)        => ~1.0
  (go:list)                      => list all packages
  (go:list "math")               => list math symbols
  (defvar cert (go:new "crypto/x509.Certificate"))
  (go:set-field cert "IsCA" t)
  (go:field cert "IsCA")         => t
  (go:make "[]int" 5)            => zero-initialized slice of 5 ints
  (go:is-nil nil)                => t`,

		"concurrency": `Concurrency Examples:
  (defvar ch (make-channel 1))
  (chan-send ch 42)
  (chan-recv ch)                 => 42
  (chan-try-send ch "hello")     => (:ok)
  (chan-close ch)
  (chan-try-recv ch)             => (:closed)
  (go:spawn (lambda () (+ 1 2)))  => thread-id
  (join-thread thread-id)         => 3
  (defvar lock (make-lock))
  (lock lock) ... (unlock lock)
  (atomic-incf counter)`,

		"clos": `CLOS Examples:
  (defclass person () ((name :initarg :name :accessor person-name)
                        (age :initarg :age :accessor person-age)))
  (defvar p (make-instance 'person :name "Alice" :age 30))
  (person-name p)                => "Alice"
  (defgeneric greet (p))
  (defmethod greet ((p person))
    (format nil "Hello, ~A!" (person-name p)))
  (greet p)                      => "Hello, Alice!"`,

		"conditions": `Condition Examples:
  (handler-case (error "oops")
    (error (e) (format nil "Caught: ~A" e)))
  (restart-case (error "retry?")
    (retry () "retried")
    (abort () "aborted"))
  (define-condition my-error (error) ((code :initarg :code)))
  (handler-case (error "fail")
    (error (e) (format nil "Error: ~A" e)))`,

		"misc": `Miscellaneous Examples:
  (random 100)                   => random integer [0, 100)
  (type-of 42)                   => integer
  (type-of "hello")              => string
  (type-of '(1 2))               => cons
  (boundp 'nil)                  => t
  (gensym)                       => #:G0
  (loop for i from 1 to 5 collect (* i i))
    => (1 4 9 16 25)
  (dotimes (i 3) (print i))     => prints 0 1 2
  (sleep 1)                      => nil (1 second delay)`,
	}

	if category == "" {
		return `Lisp Examples — Categories:
  arithmetic    (+ 1 2), (expt 2 10), (sqrt 2), (logand #b1010 #b1100)
  lists         (cons 1 '(2 3)), (car '(1 2 3)), (append '(1 2) '(3 4))
  lambda        ((lambda (x) (* x x)) 5), (define (square x) (* x x))
  recursion     factorial, fibonacci, sum-list
  map           mapcar, reduce, remove-if
  strings       string-append, string->number, char-code
  format        (format nil "Hello, ~A!" "World")
  hash          make-hash-table, gethash, setf
  arrays        make-array, aref, array-dimensions
  ffi           (ffi "math.Sqrt"), go:new, go:field, go:make
  concurrency   make-channel, chan-send, go:spawn, join-thread
  clos          defclass, make-instance, defmethod
  conditions    handler-case, restart-case, define-condition
  misc          random, type-of, gensym, loop, dotimes

Use lisp_eval with operation="examples" and expression=<category> for details.`
	}

	for k, v := range examples {
		if k == category {
			return v
		}
	}
	return fmt.Sprintf("Unknown category: %s\n\nAvailable: arithmetic, lists, lambda, recursion, map, strings, format, hash, arrays, ffi, concurrency, clos, conditions, misc", category)
}

// lispSkill returns a comprehensive guide for learning how to use this Lisp interpreter.
// topic: "ffi" for FFI/Go interop guide, "ops" for operations guide, "xref" for cross-reference guide,
// "concurrency" for concurrency guide, empty for the full guide.
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
150+ packages are registered via FFI, including:

Core:
  fmt             — Sprintf, Printf, Println, Errorf, Sprint, etc.
  strings         — Contains, HasPrefix, HasSuffix, Split, Join, Replace, Trim, ToUpper, ToLower, etc.
  strconv         — Atoi, Itoa, ParseFloat, FormatInt, Quote, Unquote, etc.
  math            — Sin, Cos, Tan, Sqrt, Abs, Pow, Exp, Log, Floor, Ceil, Round, Max, Min, Pi, E, etc.
  math/big        — NewInt, NewFloat, NewRat, Int, Float, Rat methods
  math/rand       — Intn, Float64, Float32, Int63, Perm, Shuffle, Seed
  math/bits       — Len, OnesCount, TrailingZeros, Reverse, RotateLeft, Add, Sub, Mul, Div
  math/cmplx      — Complex number functions
  os              — Getenv, Setenv, Environ, Getwd, Mkdir, Remove, Stat, Open, Args, Exit, etc.
  io              — Copy, ReadFull, WriteString, Pipe, MultiReader, MultiWriter
  io/fs           — WalkDir, ReadDir, ReadFile, Glob, ValidPath
  bytes           — Contains, Equal, Split, Join, Replace, Trim, Compare, NewBuffer
  context         — Background, TODO, WithCancel, WithTimeout, WithDeadline, WithValue
  errors          — New, Is, As, Unwrap, Join
  sync            — NewCond (types: Mutex, RWMutex, WaitGroup, Once, Map, Pool)
  sync/atomic     — AddInt64, LoadInt64, StoreInt64, SwapInt64, CompareAndSwapInt64
  runtime         — GOOS, GOARCH, NumCPU, NumGoroutine, GC, Version, ReadMemStats
  runtime/debug   — FreeOSMemory, ReadBuildInfo, SetGCPercent, Stack

Encoding:
  encoding/json   — Marshal, Unmarshal, MarshalIndent, Valid, Compact, HTMLEscape
  encoding/xml    — Marshal, Unmarshal, MarshalIndent, EscapeText
  encoding/csv    — NewReader, NewWriter
  encoding/base64 — StdEncoding, URLEncoding, EncodeToString, DecodeToString
  encoding/hex    — EncodeToString, DecodeString, Dump
  encoding/binary — Read, Write, BigEndian, LittleEndian
  encoding/gob    — NewEncoder, NewDecoder, Register
  encoding/ascii85 — Encode, Decode
  encoding/base32 — StdEncoding, HexEncoding
  encoding/pem    — Encode, Decode, Block

Crypto:
  crypto/md5      — New, Sum, Size
  crypto/sha1     — New, Sum, Size
  crypto/sha256   — New, Sum256, Size
  crypto/sha512   — New, Sum512, Size
  crypto/hmac     — New, Equal
  crypto/aes      — NewCipher, NewGCM
  crypto/cipher   — NewGCM, NewCTR, NewCBCDecrypter, etc.
  crypto/des      — NewCipher, NewTripleDESCipher
  crypto/rsa      — GenerateKey, EncryptOAEP, DecryptOAEP, SignPKCS1v15
  crypto/ecdsa    — GenerateKey, Sign, Verify
  crypto/ed25519  — GenerateKey, Sign, Verify
  crypto/rand     — Read (Reader variable)
  crypto/tls      — Dial, LoadX509KeyPair, Listen, Config
  crypto/x509     — ParseCertificate, CreateCertificate, ParsePKIXPublicKey
  crypto/subtle   — ConstantTimeCompare, ConstantTimeByteEq

Network:
  net             — Dial, Listen, LookupHost, ParseIP, SplitHostPort
  net/http        — Get, Post, NewRequest, ListenAndServe, StatusText, NewServeMux
  net/url         — Parse, QueryEscape, PathEscape, JoinPath
  net/mail        — ParseAddress, ParseDate
  net/smtp        — SendMail, PlainAuth
  net/rpc         — NewServer, Register, ServeConn
  net/textproto   — Reader, Writer

Text:
  text/template   — New, Must, ParseFiles
  text/scanner    — ScanInts, ScanFloats, TokenString
  text/tabwriter  — NewWriter, FilterHTML

Data:
  archive/tar     — NewReader, NewWriter
  archive/zip     — NewReader, Open
  compress/gzip   — NewReader, NewWriter
  compress/flate  — NewReader, NewWriter
  compress/zlib   — NewReader, NewWriter
  compress/bzip2  — NewReader
  container/list  — New
  container/ring  — New
  database/sql    — Open

Go tooling:
  go/ast          — Inspect, Print, IsExported
  go/parser       — ParseFile, ParseExpr
  go/token        — NewFileSet
  go/format       — Source, Node
  go/printer      — Fprint, Config
  go/scanner      — Scanner
  go/build        — Context, Default

Debug/Testing:
  debug/dwarf     — New
  debug/elf       — NewFile
  debug/macho     — NewFile, NewFatFile
  debug/pe        — NewFile
  testing         — Short, Verbose, CoverMode, Main
  log             — Print, Printf, Println, Fatal, SetFlags, SetPrefix
  log/slog        — Default, Info, Warn, Error, New

Hash:
  hash/crc32      — ChecksumIEEE, NewIEEE, MakeTable
  hash/crc64      — Checksum, New, MakeTable
  hash/adler32    — Checksum, New
  hash/fnv        — New32, New64
  hash/maphash    — Hash, Seed

Image:
  image           — NewRGBA, NewPaletted, Pt, Rect
  image/color     — NRGBAModel, RGBAModel, GrayModel
  image/draw      — Draw, FloydSteinberg
  image/gif       — EncodeAll, DecodeAll
  image/jpeg      — Encode, Decode
  image/png       — Encode, Decode

Other:
  html            — EscapeString, UnescapeString
  html/template   — HTML, CSS, JS, URL
  mime            — TypeByExtension, ExtensionsByType, TypeByExtension
  mime/multipart  — NewReader, NewWriter
  mime/quotedprintable — NewReader, NewWriter
  path            — Base, Clean, Dir, Ext, IsAbs, Join, Match, Split
  path/filepath   — Abs, Base, Clean, Dir, Glob, Join, Walk, WalkDir, Ext, IsAbs, Rel
  reflect         — TypeOf, ValueOf, Zero, New, MakeSlice, MakeMap, MakeChan
  regexp          — MustCompile, Compile, QuoteMeta
  regexp/syntax   — Parse, Compile
  sort            — Ints, Float64s, Strings, Search, Slice, SliceStable
  time            — Now, Parse, Sleep, Since, Unix, Date, RFC3339, Hour, Minute, Second
  unicode         — IsLetter, IsDigit, IsLower, IsUpper, IsSpace, ToLower, ToUpper
  unicode/utf8    — DecodeRune, EncodeRune, RuneCount, Valid, RuneLen, FullRune
  unicode/utf16   — Decode, Encode, IsSurrogate
  os/exec         — Command, CommandContext, LookPath
  os/signal       — Notify, Stop, Reset
  os/user         — Current, Lookup, LookupId
  flag            — Parse, Bool, String, Int, Lookup, Args
  io/ioutil       — ReadFile, WriteFile, ReadAll, TempFile, TempDir, ReadDir, NopCloser
  syscall         — Getpid, Getppid, Getuid, Getgid

Plus custom microlisp packages:
  microlisp/io    — NewStringReader, NewBufferReader, NewFileReader,
                    NewStringWriter, NewBufferWriter, NewFileWriter,
                    ContextCancel, ContextDone
  microlisp/fmt   — FormatString
  microlisp/binary — BinaryReadUint32/64, BinaryReadInt32/64, BinaryWriteUint32/64
  microlisp/jsonx — JsonMarshalIndent

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

--- Go Type Introspection ---
  (go:type-of cert)               => "*x509.Certificate"
  (go:is-nil cert)                => NIL
  (go:is-zero cert)               => NIL
  (go:fields-of "crypto/x509.Certificate")  => list of field names
  (go:kind-of cert)               => "Ptr"
  (go:implements reader "io.Reader")  => T
  (go:convert "hello" "[]byte")   => []byte slice
  (go:type-parse "[]int")         => type metadata
  (go:search "Sqrt")              => find symbols matching "Sqrt"

--- Creating Go Values ---
  (go:make "[]int" 10)            => slice of 10 zero ints (default cap=8)
  (go:make "[]int" 10 20)         => slice of 10 ints, capacity 20
  (go:make "map[string]int")      => empty map
  (go:make "chan int" 5)          => buffered channel (cap=5)
  (go:make "chan int")            => unbuffered channel
  (go:make "[10]int")             => fixed array of 10 ints
  (go:make "io.Reader")           => interface pointer (nil)
  (go:register "mypkg.val" 42)    => register custom value

--- Go Callbacks ---
  (go:callback fn "int32->bool")  => Go func(int32) bool
  (go:callback fn "string->string")  => Go func(string) string
  (go:callback fn "()->")         => Go func()
  (go:callback fn "int,int->bool")  => Go func(int, int) bool
  (go:callback fn "string->error")  => Go func(string) error
  Supported signatures:
    "int32->bool", "int32->int32", "int->int", "int->bool",
    "int,int->bool", "int,int->", "string->string", "()->", "string->error"

--- Registering Custom Go Values ---
  (go:register "mypackage.myvalue" 42)   => register a number
  (go:register "mypackage.mystring" "hello") => register a string

--- Panic Recovery and Defer ---
  (go:recover body...)  Catches Go panics inside body
  (go:with-defer (defer-form) body...)  Go-style defer

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
- Use operation=source with expression="ffi" to view the FFI builtin source code

--- Stdlib Wrappers (Typeless API) ---
These Lisp functions wrap Go stdlib operations with automatic type handling.
No need to deal with Go types, error tuples, or VGoVal — just call and get Lisp values.

File I/O:
  (read-file path)                    => string
  (write-file path content)           => t
  (append-file path content)          => t
  (file-exists-p path)                => t/nil
  (directory-exists-p path)           => t/nil
  (file-size path)                    => number
  (delete-file path)                  => t
  (rename-file old new)               => t
  (directory path)                    => list of filenames
  (mkdir path &key parents)           => t
  (temp-file &key prefix suffix dir)  => path string

HTTP:
  (http-get url &key headers)         => string (body)
  (http-post url content &key content-type) => string
  (http-get-json url)                 => string (raw JSON)
  (http-post-json url data)           => string

JSON:
  (json-encode obj)                   => JSON string
  (json-valid-p str)                  => t/nil
  ;; obj can be: nil, t, string, number, list (array), alist (object)

Time:
  (sleep seconds)                     => nil
  (format-time fmt &optional unix)    => string
  (current-timestamp)                 => "2006-01-02 15:04:05"
  (parse-time fmt str)                => unix timestamp

Regex:
  (regex-match pattern str)           => t/nil
  (regex-find-all pattern str &optional count) => list
  (regex-replace pattern str replacement) => string
  (regex-split pattern str)           => list

Path:
  (path-absolute path)                => string
  (path-base path)                    => string
  (path-dir path)                     => string
  (path-ext path)                     => string
  (path-join &rest paths)             => string
  (path-clean path)                   => string
  (path-exists-p path)                => t/nil
  (path-is-absolute path)             => t/nil

Environment:
  (getenv key &optional default)      => string
  (setenv key value)                  => t
  (unsetenv key)                      => t
  (getenv-all)                        => alist of (key . value)
  (current-dir)                       => string
  (change-dir dir)                    => t

Encoding:
  (base64-encode str)                 => base64 string
  (base64-decode str)                 => decoded string
  (url-encode str)                    => URL-encoded string
  (url-decode str)                    => decoded string
  (url-parse url-str)                 => alist (:scheme . s) (:host . h) ...

Crypto:
  (md5 str)                           => hex digest string
  (sha1 str)                          => hex digest string
  (sha256 str)                        => hex digest string

Process:
  (run-command cmd &rest args)        => (output exit-code)
  (shell command-string)              => (output exit-code)
  (which command)                     => path or nil

String Utilities:
  (string-contains s substr)          => t/nil
  (string-starts-with s prefix)       => t/nil
  (string-ends-with s suffix)         => t/nil
  (string-split s sep &optional max)  => list
  (string-join lst sep)               => string
  (string-replace s old new &optional count) => string
  (string-trim s)                     => string
  (string-to-upper s)                 => string
  (string-to-lower s)                 => string
  (string-repeat s count)             => string

System Info:
  (go-os)                             => "windows"/"linux"/"darwin"
  (go-arch)                           => "amd64"/"arm64"
  (go-version)                        => Go version string
  (num-cpus)                          => number
  (pid)                               => process ID
  (hostname)                          => machine hostname
  (expand-env str)                    => expand $ENV_VAR in string`,

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
- Concurrency guide?  → operation=skill, expression="concurrency"
- View source code?   → operation=source, expression="car" (plain name)
- Explore call graph? → operation=xref, expression="filter" (plain name)
- List functions?     → operation=source-list, expression=""
- Full guide?         → operation=skill, expression=""

--- All Operations ---
  define — function signature: params, return types, usage
    expression: "car"         →  (car lst), returns first element
    expression: "math.Pow"    →  func math.Pow(float64, float64) → (float64)
    expression: "format"      →  (format dest fmt args...), returns formatted string
  eval (default) — evaluate Lisp expression; state persists
    expression: "(+ 1 2)"  →  3
    limits: "default" | "strict" | "unlimited"
  reset — clear all interpreter state
  help — topic reference; expression="" lists topics
    Topics: arithmetic, lists, strings, lambda, recursion, format,
            hash, arrays, clos, conditions, macros, types, special,
            concurrency, ffi, sequences, packages, io, pathnames, readtable, time
  examples — code examples; expression="" lists categories
    Categories: arithmetic, lists, lambda, recursion, map, strings,
                format, hash, arrays, ffi, concurrency, clos, conditions, misc
  skill — comprehensive guide; expression="ffi"/"ops"/"xref"/"concurrency"
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

		"concurrency": `=== Concurrency Guide ===

Microlisp provides CSP-style concurrency via Go channels, threads, and locks.

--- Channels ---
Channels allow goroutines (spawned threads) to communicate safely.

Create:
  (make-channel)              => unbuffered channel
  (make-channel 10)           => buffered channel (capacity 10)
  (go:channel 5)              => alias for make-channel

Send and Receive:
  (chan-send ch 42)           => blocking send (returns NIL)
  (chan-recv ch)              => blocking receive (returns value or NIL if closed)
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
  (join-thread thread-id)     => wait for thread, returns result or error
  (thread? val)               => T if val is a thread

--- Locks ---
  (make-lock)                 => create mutex
  (lock l)                    => acquire mutex (blocks if held)
  (unlock l)                  => release mutex
  (lock? val)                 => T if val is a lock

--- Condition Variables ---
  (make-condvar)              => create condition variable
  (condition-wait cv lock)    => wait for signal (atomically releases lock)
  (condition-notify cv)       => wake one waiter
  (condition-broadcast cv)    => wake all waiters
  (condvar? val)              => T if val is a condition variable

--- Atomic Operations ---
  (atomic-incf place)         => atomically increment
  (atomic-decf place)         => atomically decrement
  (atomic-get place)          => atomic read
  (atomic-set place val)      => atomic write

--- Special Forms ---
  (go:recover body...)        => catch Go panics inside body
  (go:with-defer (defer-form) body...)  => execute defer-form when body exits

--- Example: Producer-Consumer ---
  (let ((ch (make-channel 10)))
    (go:spawn
      (lambda ()
        (loop for i from 1 to 5 do
          (chan-send ch i))
        (chan-close ch)))
    (loop for val = (chan-recv ch)
          while val
          collect val))
  => (1 2 3 4 5)

--- CancelChan Integration ---
All blocking operations (chan-recv, chan-send, go:select) integrate with
the evaluator's CancelChan for resource-limited execution. When a
time/step limit is reached, blocking operations are interrupted.`,
	}

	if topic != "" {
		for k, v := range guides {
			if k == topic {
				return v
			}
		}
		return fmt.Sprintf("Unknown skill topic: %s\n\nAvailable: ffi, ops, xref, concurrency (or empty for full guide)", topic)
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
- Concurrency/channels?                     → operation=skill, expression="concurrency"
- View source of a function?                → operation=source, expression="car"
- Explore who calls what?                   → operation=xref, expression="filter"
- All operations explained?                 → operation=skill, expression="ops"

--- Core Features ---
- Arbitrary-precision integers, rationals, complex numbers
- Lists, strings, hash tables, arrays, structs
- Lambda, closures, macros, CLOS (object system)
- Conditions/restarts (error handling)
- Format directives (~A, ~D, ~F, ~%, ~{~}, ~R, ~P)
- State persists between calls (defvar, defun survive)
- Thread-safe: concurrent calls serialized
- CSP-style concurrency with channels, threads, locks
- 150+ Go stdlib packages via FFI
- Full CL sequence operations (mapcar, reduce, find, remove, sort, search)
- Package system (make-package, in-package, find-symbol, export, import)
- Pathnames and I/O streams with composite stream support
- Readtable customization with macro characters

--- FFI Quick Reference ---
Import:  (ffi "math.Sqrt") or (go:import "math.Sin")
Call:    ((ffi "math.Sqrt") 2.0) → 1.414...
Structs: (go:new "crypto/x509.Certificate")
Fields:  (go:field cert "Version"), (go:set-field cert "IsCA" t)
Create:  (go:make "[]int" 10), (go:make "map[string]int"), (go:make "chan int")
Callback: (go:callback fn "int->bool")
Recover: (go:recover body...)
Details: operation=skill, expression="ffi"

--- Concurrency Quick Reference ---
Channel: (make-channel 10), (chan-send ch val), (chan-recv ch)
Select:  (go:select (:recv ch1) (:send ch2 val) (:default))
Thread:  (go:spawn (lambda () body)), (join-thread id)
Lock:    (make-lock), (lock l), (unlock l)
Atomic:  (atomic-incf place), (atomic-get place)
Details: operation=skill, expression="concurrency"

--- Resource Limits ---
default:  1M steps, 30s, 256MB — normal use
strict:   100K steps, 10s, 64MB — untrusted code
unlimited: no limits — REPL mode only

--- All Skill Topics ---
  expression="ffi"         — FFI/Go interop guide (import, call, structs, VGoVal, callbacks)
  expression="concurrency" — channels, threads, locks, atomics, select
  expression="ops"         — all operations explained (eval, help, source, xref, etc.)
  expression="xref"        — cross-reference guide (call graph exploration)`
}
