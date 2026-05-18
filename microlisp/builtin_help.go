package microlisp

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// builtinHelp provides Lisp-level help for builtins, FFI functions, and types.
// (help) — list all help categories
// (help "go:field") — help for a specific builtin/FFI
// (help "go:") — help for all go:* functions
// (help "crypto/x509") — help for an FFI package
func builtinHelp(args []*Value) (*Value, error) {
	if len(args) == 0 || isNil(args[0]) {
		return vstr(helpAll()), nil
	}

	name := args[0].str
	if name == "" {
		return vstr(helpAll()), nil
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
	"=":   "(= a b) — Numeric equality.",
	"/=":  "(/= a b) — Numeric inequality.",
	"<":   "(< a b) — Less than.",
	">":   "(> a b) — Greater than.",
	"<=":  "(<= a b) — Less than or equal.",
	">=":  "(>= a b) — Greater than or equal.",
	"eq":  "(eq a b) — Identity comparison (same object).",
	"eql": "(eql a b) — Value comparison (same value).",
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
	"ffi":        `(ffi "pkg.Func") — Import a Go function from stdlib.`,
	"ffi-register": `(ffi-register "name" value) — Register a custom Go value.`,

	// Type predicates
	"null":       "(null x) — Check if x is nil.",
	"listp":      "(listp x) — Check if x is a list.",
	"consp":      "(consp x) — Check if x is a cons cell.",
	"symbolp":    "(symbolp x) — Check if x is a symbol.",
	"stringp":    "(stringp x) — Check if x is a string.",
	"numberp":    "(numberp x) — Check if x is a number.",
	"characterp": "(characterp x) — Check if x is a character.",
	"hash-tablep": "(hash-tablep x) — Check if x is a hash table.",

	// Arrays
	"make-array": "(make-array dims) — Create an array. dims: int (1D) or list (multi-D).",
	"aref":       "(aref arr i...) — Access array element.",

	// CLOS
	"defclass":   "(defclass name (parents) (slots)) — Define a class.",
	"defgeneric": "(defgeneric name (args)) — Define a generic function.",
	"defmethod":  "(defmethod name ((arg type)) body...) — Define a method.",
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

	"go:send":    `(go:send channel value) — Send value to channel.`,

	"go:recv":    `(go:recv channel) — Receive value from channel.`,

	"go:close":   `(go:close channel) — Close a channel.`,

	"go:select":  `(go:select cases...) — Go-style select on channels.`,

	"go:chanp":   `(go:chanp x) — Check if x is a channel.`,

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
	sb.WriteString("Built-in functions (showing a sample):\n")
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
