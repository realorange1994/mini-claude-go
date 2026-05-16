package microlisp

import (
	"fmt"
	"testing"
)

// -------- Basic Evaluator Tests --------
// These tests cover the core evaluator functionality.

func TestMainBasicArithmetic(t *testing.T) {
	ResetGlobalEnv()

	tests := []struct {
		input    string
		expected string
	}{
		{"(+ 1 2)", "3"},
		{"(* 3 4)", "12"},
		{"(- 10 3)", "7"},
		{"(/ 15 3)", "5"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			r, err := EvalString(tt.input, globalEnv)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			got := ToString(r)
			if got != tt.expected {
				t.Fatalf("expected %s, got %s", tt.expected, got)
			}
		})
	}
}

func TestMainListOperations(t *testing.T) {
	ResetGlobalEnv()

	tests := []struct {
		input    string
		expected string
	}{
		{"(quote (1 2 3))", "(1 2 3)"},
		{"'()", "()"},
		{"(car '(1 2 3))", "1"},
		{"(cdr '(1 2 3))", "(2 3)"},
		{"(cons 1 '(2 3))", "(1 2 3)"},
		{"(list 1 2 3)", "(1 2 3)"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			r, err := EvalString(tt.input, globalEnv)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			got := ToString(r)
			if got != tt.expected {
				t.Fatalf("expected %s, got %s", tt.expected, got)
			}
		})
	}
}

func TestMainPredicates(t *testing.T) {
	ResetGlobalEnv()

	tests := []struct {
		input    string
		expected string
	}{
		{"(null? '())", "#t"},
		{"(null? '(1))", "#f"},
		{"(pair? '(1 2))", "#t"},
		{"(number? 42)", "#t"},
		{"(string? \"hi\")", "#t"},
		{"(symbol? 'x)", "#t"},
		{"(eq? 'a 'a)", "#t"},
		{"(equal? '(1 2) '(1 2))", "#t"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			r, err := EvalString(tt.input, globalEnv)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			got := ToString(r)
			if got != tt.expected {
				t.Fatalf("expected %s, got %s", tt.expected, got)
			}
		})
	}
}

func TestMainConditionals(t *testing.T) {
	ResetGlobalEnv()

	tests := []struct {
		input    string
		expected string
	}{
		{"(if #t 1 2)", "1"},
		{"(if #f 1 2)", "2"},
		{"(begin 1 2 3)", "3"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			r, err := EvalString(tt.input, globalEnv)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			got := ToString(r)
			if got != tt.expected {
				t.Fatalf("expected %s, got %s", tt.expected, got)
			}
		})
	}
}

func TestMainDefinitions(t *testing.T) {
	ResetGlobalEnv()

	tests := []struct {
		input    string
		expected string
	}{
		{"(define x 42) x", "42"},
		{"(define (sq x) (* x x)) (sq 5)", "25"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			r, err := EvalString(tt.input, globalEnv)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			got := ToString(r)
			if got != tt.expected {
				t.Fatalf("expected %s, got %s", tt.expected, got)
			}
		})
	}
}

func TestMainLet(t *testing.T) {
	ResetGlobalEnv()

	tests := []struct {
		input    string
		expected string
	}{
		{"(let ((x 2) (y 3)) (+ x y))", "5"},
		{"((lambda (x) (* x x)) 7)", "49"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			r, err := EvalString(tt.input, globalEnv)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			got := ToString(r)
			if got != tt.expected {
				t.Fatalf("expected %s, got %s", tt.expected, got)
			}
		})
	}
}

func TestMainStrings(t *testing.T) {
	ResetGlobalEnv()

	tests := []struct {
		input    string
		expected string
	}{
		{"(string-append \"a\" \"b\")", "\"ab\""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			r, err := EvalString(tt.input, globalEnv)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			got := ToString(r)
			if got != tt.expected {
				t.Fatalf("expected %s, got %s", tt.expected, got)
			}
		})
	}
}

func TestMainComparison(t *testing.T) {
	ResetGlobalEnv()

	tests := []struct {
		input    string
		expected string
	}{
		{"(> 3 2)", "#t"},
		{"(< 2 3)", "#t"},
		{"(= 5 5)", "#t"},
		{"(>= 5 3)", "#t"},
		{"(<= 3 5)", "#t"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			r, err := EvalString(tt.input, globalEnv)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			got := ToString(r)
			if got != tt.expected {
				t.Fatalf("expected %s, got %s", tt.expected, got)
			}
		})
	}
}

func TestMainTypeOf(t *testing.T) {
	ResetGlobalEnv()

	tests := []struct {
		input    string
		expected string
	}{
		{"(type-of 42)", "INTEGER"},
		{"(type-of 'x)", "SYMBOL"},
		{"(type-of \"hi\")", "STRING"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			r, err := EvalString(tt.input, globalEnv)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			got := ToString(r)
			if got != tt.expected {
				t.Fatalf("expected %s, got %s", tt.expected, got)
			}
		})
	}
}

// -------- Standard Library Tests --------

func TestMainStdlibListOps(t *testing.T) {
	ResetGlobalEnv()

	tests := []struct {
		input    string
		expected string
	}{
		{"(caar '((1 2)))", "1"},
		{"(cadr '(1 2 3))", "2"},
		{"(cddr '(1 2 3))", "(3)"},
		{"(caddr '(1 2 3))", "3"},
		{"(mapcar (lambda (x) (* x 2)) '(1 2 3))", "(2 4 6)"},
		{"(filter (lambda (x) (> x 2)) '(1 2 3 4))", "(3 4)"},
		{"(fold + 0 '(1 2 3))", "6"},
		{"(length '(1 2 3 4))", "4"},
		{"(append '(1 2) '(3 4))", "(1 2 3 4)"},
		{"(reverse '(1 2 3))", "(3 2 1)"},
		{"(range 5)", "(0 1 2 3 4)"},
		{"(take 2 '(1 2 3))", "(1 2)"},
		{"(drop 2 '(1 2 3))", "(3)"},
		{"(zip '(1 2) '(a b))", "((1 A) (2 B))"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			r, err := EvalString(tt.input, globalEnv)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			got := ToString(r)
			if got != tt.expected {
				t.Fatalf("expected %s, got %s", tt.expected, got)
			}
		})
	}
}

func TestMainStdlibPredicates(t *testing.T) {
	ResetGlobalEnv()

	tests := []struct {
		input    string
		expected string
	}{
		{"(not #t)", "#f"},
		{"(member? 2 '(1 2 3))", "#t"},
		{"(any number? '(a b 3))", "#t"},
		{"(all number? '(1 2 3))", "#t"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			r, err := EvalString(tt.input, globalEnv)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			got := ToString(r)
			if got != tt.expected {
				t.Fatalf("expected %s, got %s", tt.expected, got)
			}
		})
	}
}

func TestMainStdlibOther(t *testing.T) {
	ResetGlobalEnv()

	tests := []struct {
		input    string
		expected string
	}{
		{"(list-ref '(a b c) 1)", "B"},
		{"(abs -5)", "5"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			r, err := EvalString(tt.input, globalEnv)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			got := ToString(r)
			if got != tt.expected {
				t.Fatalf("expected %s, got %s", tt.expected, got)
			}
		})
	}
}

// -------- Advanced Tests --------

func TestMainMacro(t *testing.T) {
	ResetGlobalEnv()

	r, err := EvalString(`
		(define-macro (twice expr) (list (quote +) expr expr))
		(twice (+ 1 1))
	`, globalEnv)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	got := ToString(r)
	if got != "4" {
		t.Fatalf("expected 4, got %s", got)
	}
}

func TestMainClosure(t *testing.T) {
	ResetGlobalEnv()

	_, err := EvalString(`(define (make-counter)
		(let ((count 0))
			(lambda ()
				(set! count (+ count 1))
				count)))
	(define counter (make-counter))
	`, globalEnv)
	if err != nil {
		t.Fatalf("closure setup error: %v", err)
	}

	r1, err := EvalString(`(counter)`, globalEnv)
	if err != nil {
		t.Fatalf("counter call error: %v", err)
	}
	r2, _ := EvalString(`(counter)`, globalEnv)
	r3, _ := EvalString(`(counter)`, globalEnv)

	if ToString(r1) != "1" || ToString(r2) != "2" || ToString(r3) != "3" {
		t.Fatalf("expected 1 2 3, got %s %s %s", ToString(r1), ToString(r2), ToString(r3))
	}
}

func TestMainTailCallOptimization(t *testing.T) {
	ResetGlobalEnv()

	_, err := EvalString(`(define (range-tco n acc)
		(if (= n 0) acc
			(range-tco (- n 1) (cons n acc))))`, globalEnv)
	if err != nil {
		t.Fatalf("TCO setup error: %v", err)
	}

	r, err := EvalString(`(range-tco 1000 '())`, globalEnv)
	if err != nil {
		t.Fatalf("TCO error: %v", err)
	}

	l := length(r)
	if l != 1000 {
		t.Fatalf("expected length 1000, got %d", l)
	}
}

// -------- Subtests Group --------

func TestMainCoreEvaluator(t *testing.T) {
	t.Run("arithmetic", TestMainBasicArithmetic)
	t.Run("list", TestMainListOperations)
	t.Run("predicates", TestMainPredicates)
	t.Run("conditionals", TestMainConditionals)
	t.Run("definitions", TestMainDefinitions)
	t.Run("let", TestMainLet)
	t.Run("strings", TestMainStrings)
	t.Run("comparison", TestMainComparison)
	t.Run("typeof", TestMainTypeOf)
}

func TestMainStdlib(t *testing.T) {
	t.Run("list ops", TestMainStdlibListOps)
	t.Run("predicates", TestMainStdlibPredicates)
	t.Run("other", TestMainStdlibOther)
}

func TestMainAdvanced(t *testing.T) {
	t.Run("macro", TestMainMacro)
	t.Run("closure", TestMainClosure)
	t.Run("TCO", TestMainTailCallOptimization)
}

// -------- Benchmarks --------

func BenchmarkMainFibonacci(b *testing.B) {
	ResetGlobalEnv()
	_, err := EvalString(`
		(define (fib n)
			(if (<= n 1) n
				(+ (fib (- n 1)) (fib (- n 2)))))
	`, globalEnv)
	if err != nil {
		b.Fatal(err)
	}
	for i := 0; i < b.N; i++ {
		_, err := EvalString(`(fib 15)`, globalEnv)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMainMapcar(b *testing.B) {
	ResetGlobalEnv()
	for i := 0; i < b.N; i++ {
		_, err := EvalString(`(mapcar (lambda (x) (* x 2)) '(1 2 3 4 5 6 7 8 9 10))`, globalEnv)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMainFactorial(b *testing.B) {
	ResetGlobalEnv()
	_, err := EvalString(`
		(define (factorial n)
			(if (<= n 1) 1
				(* n (factorial (- n 1)))))
	`, globalEnv)
	if err != nil {
		b.Fatal(err)
	}
	for i := 0; i < b.N; i++ {
		_, err := EvalString(`(factorial 20)`, globalEnv)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMainTCO(b *testing.B) {
	ResetGlobalEnv()
	_, err := EvalString(`
		(define (sum-to n acc)
			(if (= n 0) acc
				(sum-to (- n 1) (+ acc n))))
	`, globalEnv)
	if err != nil {
		b.Fatal(err)
	}
	for i := 0; i < b.N; i++ {
		_, err := EvalString(`(sum-to 10000 0)`, globalEnv)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// -------- Table-driven Test for All Basic Cases --------

func TestMainAllBasicCases(t *testing.T) {
	ResetGlobalEnv()

	type testCase struct {
		name     string
		input    string
		expected string
	}

	allTests := []testCase{
		// Arithmetic
		{"add", "(+ 1 2)", "3"},
		{"mul", "(* 3 4)", "12"},
		{"sub", "(- 10 3)", "7"},
		{"div", "(/ 15 3)", "5"},

		// Lists
		{"quote", "(quote (1 2 3))", "(1 2 3)"},
		{"empty quote", "'()", "()"},
		{"car", "(car '(1 2 3))", "1"},
		{"cdr", "(cdr '(1 2 3))", "(2 3)"},
		{"cons", "(cons 1 '(2 3))", "(1 2 3)"},
		{"list", "(list 1 2 3)", "(1 2 3)"},
		{"length", "(length '(1 2 3))", "3"},

		// Predicates
		{"null true", "(null? '())", "#t"},
		{"null false", "(null? '(1))", "#f"},
		{"pair", "(pair? '(1 2))", "#t"},
		{"number", "(number? 42)", "#t"},
		{"string", "(string? \"hi\")", "#t"},
		{"symbol", "(symbol? 'x)", "#t"},
		{"eq", "(eq? 'a 'a)", "#t"},
		{"equal", "(equal? '(1 2) '(1 2))", "#t"},

		// Conditionals
		{"if true", "(if #t 1 2)", "1"},
		{"if false", "(if #f 1 2)", "2"},
		{"begin", "(begin 1 2 3)", "3"},

		// Definitions
		{"define var", "(define x 42) x", "42"},
		{"define func", "(define (sq x) (* x x)) (sq 5)", "25"},

		// Let and lambda
		{"let", "(let ((x 2) (y 3)) (+ x y))", "5"},
		{"lambda", "((lambda (x) (* x x)) 7)", "49"},

		// Comparison
		{"gt", "(> 3 2)", "#t"},
		{"lt", "(< 2 3)", "#t"},
		{"eq-num", "(= 5 5)", "#t"},
		{"gte", "(>= 5 3)", "#t"},
		{"lte", "(<= 3 5)", "#t"},

		// Type introspection
		{"type-of number", "(type-of 42)", "INTEGER"},
		{"type-of symbol", "(type-of 'x)", "SYMBOL"},
		{"type-of string", "(type-of \"hi\")", "STRING"},
	}

	for _, tt := range allTests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := EvalString(tt.input, globalEnv)
			if err != nil {
				t.Fatalf("input: %q, error: %v", tt.input, err)
			}
			got := ToString(r)
			if got != tt.expected {
				t.Fatalf("input: %q, expected %s, got %s", tt.input, tt.expected, got)
			}
		})
	}
}

// -------- Fuzz Tests --------

func FuzzMainArithmetic(f *testing.F) {
	f.Add(1, 2)
	f.Add(0, 0)
	f.Add(100, 200)
	f.Add(-5, 3)
	f.Fuzz(func(t *testing.T, a, b int) {
		ResetGlobalEnv()
		r, err := EvalString(fmt.Sprintf("(+ %d %d)", a, b), globalEnv)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		got := ToString(r)
		expected := fmt.Sprintf("%d", a+b)
		if got != expected {
			t.Fatalf("(+ %d %d) expected %s, got %s", a, b, expected, got)
		}
	})
}
