package microlisp

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// -------- REPL --------
func repl() {
	reader := bufio.NewReader(os.Stdin)
	fmt.Println("MicroLisp v1.0 - Type (exit) to quit")
	for {
		fmt.Print("λ> ")
		var input string
		depth := 0
		inStr := false
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					fmt.Println()
					return
				}
				fmt.Printf("read error: %v\n", err)
				break
			}
			input += line
			for _, ch := range line {
				if inStr {
					if ch == '"' {
						inStr = false
					}
				} else {
					switch ch {
					case '"':
						inStr = true
					case '(':
						depth++
					case ')':
						depth--
					case ';':
						// skip comment - already part of line
					}
				}
			}
			if depth <= 0 && strings.TrimSpace(input) != "" {
				break
			}
			if strings.TrimSpace(input) != "" {
				fmt.Print("  > ")
			}
		}
		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}
		exprs, err := parseAll(input)
		if err != nil {
			fmt.Printf("parse error: %v\n", err)
			continue
		}
		for !isNil(exprs) {
			loopIterationCount = 0
			var result *Value
			var evalErr error
			func() {
				defer func() {
					if r := recover(); r != nil {
						// Handle tailCall panic
						if tc, ok := r.(*tailCall); ok {
							evalErr = fmt.Errorf("tail call error: %v", tc)
							return
						}
						evalErr = fmt.Errorf("%v", r)
					}
				}()
				result, evalErr = Eval(exprs.car, globalEnv)
			}()
			if evalErr != nil {
				fmt.Printf("error: %v\n", evalErr)
			} else {
				fmt.Println(writeToString(primaryValue(result)))
			}
			exprs = exprs.cdr
		}
	}
}

// -------- Main --------
func main() {
	InitGlobalEnv()
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "-e", "--eval":
			if len(os.Args) > 2 {
				var result *Value
				var err error
				func() {
					defer func() {
						if r := recover(); r != nil {
							err = fmt.Errorf("panic: %v", r)
						}
					}()
					result, err = EvalString(os.Args[2], globalEnv)
				}()
				if err != nil {
					fmt.Fprintf(os.Stderr, "error: %v\n", err)
					os.Exit(1)
				}
				fmt.Println(ToString(primaryValue(result)))
			}
		case "--test":
			runTests()
		default:
			_, err := LoadFile(os.Args[1], globalEnv)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
		}
		return
	}
	repl()
}

// -------- Tests --------
func runTests() {
	tests := []struct {
		input    string
		expected string
	}{
		{"(+ 1 2)", "3"},
		{"(* 3 4)", "12"},
		{"(- 10 3)", "7"},
		{"(/ 15 3)", "5"},
		{"(quote (1 2 3))", "(1 2 3)"},
		{"'()", "()"},
		{"(car '(1 2 3))", "1"},
		{"(cdr '(1 2 3))", "(2 3)"},
		{"(cons 1 '(2 3))", "(1 2 3)"},
		{"(null? '())", "#t"},
		{"(null? '(1))", "#f"},
		{"(pair? '(1 2))", "#t"},
		{"(number? 42)", "#t"},
		{"(string? \"hi\")", "#t"},
		{"(symbol? 'x)", "#t"},
		{"(eq? 'a 'a)", "#t"},
		{"(equal? '(1 2) '(1 2))", "#t"},
		{"(if #t 1 2)", "1"},
		{"(if #f 1 2)", "2"},
		{"(begin 1 2 3)", "3"},
		{"(define x 42) x", "42"},
		{"(define (sq x) (* x x)) (sq 5)", "25"},
		{"(let ((x 2) (y 3)) (+ x y))", "5"},
		{"((lambda (x) (* x x)) 7)", "49"},
		{"(string-append \"a\" \"b\")", "\"ab\""},
		{"(length '(1 2 3))", "3"},
		{"(list 1 2 3)", "(1 2 3)"},
		{"(not #t)", "#f"},
		{"(not #f)", "#t"},
		{"(null? '())", "#t"},
		{"(> 3 2)", "#t"},
		{"(< 2 3)", "#t"},
		{"(= 5 5)", "#t"},
		{"(>= 5 3)", "#t"},
		{"(<= 3 5)", "#t"},
		{"(type-of 42)", "number"},
		{"(type-of 'x)", "symbol"},
		{"(type-of \"hi\")", "string"},
	}
	passed := 0
	for _, tt := range tests {
		result, err := func() (r *Value, e error) {
			defer func() {
				if rp := recover(); rp != nil {
					e = fmt.Errorf("panic: %v", rp)
				}
			}()
			r, e = EvalString(tt.input, globalEnv)
			return
		}()
		if err != nil {
			fmt.Printf("FAIL: %s => error: %v\n", tt.input, err)
			continue
		}
		got := ToString(result)
		if got != tt.expected {
			fmt.Printf("FAIL: %s => got %s, expected %s\n", tt.input, got, tt.expected)
			continue
		}
		passed++
	}
	// Standard library tests
	libTests := []struct {
		input    string
		expected string
	}{
		{"(caar '((1 2)))", "1"},
		{"(cadr '(1 2 3))", "2"},
		{"(cddr '(1 2 3))", "(3)"},
		{"(caddr '(1 2 3))", "3"},
		{"(not #t)", "#f"},
		{"(mapcar (lambda (x) (* x 2)) '(1 2 3))", "(2 4 6)"},
		{"(filter (lambda (x) (> x 2)) '(1 2 3 4))", "(3 4)"},
		{"(fold + 0 '(1 2 3))", "6"},
		{"(length '(1 2 3 4))", "4"},
		{"(append '(1 2) '(3 4))", "(1 2 3 4)"},
		{"(reverse '(1 2 3))", "(3 2 1)"},
		{"(range 5)", "(0 1 2 3 4)"},
		{"(list-ref '(a b c) 1)", "B"},
		{"(member? 2 '(1 2 3))", "#t"},
		{"(any number? '(a b 3))", "#t"},
		{"(all number? '(1 2 3))", "#t"},
		{"(take 2 '(1 2 3))", "(1 2)"},
		{"(drop 2 '(1 2 3))", "(3)"},
		{"(zip '(1 2) '(a b))", "((1 A) (2 B))"},
		{"(abs -5)", "5"},
	}
	fmt.Println("\n--- Standard Library Tests ---")
	for _, tt := range libTests {
		result, err := EvalString(tt.input, globalEnv)
		if err != nil {
			fmt.Printf("FAIL: %s => error: %v\n", tt.input, err)
			continue
		}
		got := ToString(result)
		if got != tt.expected {
			fmt.Printf("FAIL: %s => got %s, expected %s\n", tt.input, got, tt.expected)
			continue
		}
		passed++
	}

	// Macro test
	// Macro test
	macroResult, err := EvalString(`
		(define-macro (twice expr) (list (quote +) expr expr))
		(twice (+ 1 1))
	`, globalEnv)
	if err != nil {
		fmt.Printf("FAIL macro: %v\n", err)
	} else {
		got := ToString(macroResult)
		if got != "4" {
			fmt.Printf("FAIL macro: expected 4, got %s\n", got)
		} else {
			passed++
			fmt.Printf("PASS macro test\n")
		}
	}

	// Closure test
	_, err = EvalString(`(define (make-counter)
		(let ((count 0))
			(lambda ()
				(set! count (+ count 1))
				count)))
	(define counter (make-counter))
	`, globalEnv)
	if err != nil {
		fmt.Printf("FAIL closure setup: %v\n", err)
	} else {
		r1, _ := EvalString(`(counter)`, globalEnv)
		r2, _ := EvalString(`(counter)`, globalEnv)
		r3, _ := EvalString(`(counter)`, globalEnv)
		if ToString(r1) == "1" && ToString(r2) == "2" && ToString(r3) == "3" {
			passed++
			fmt.Printf("PASS closure/counter test: %s %s %s\n", ToString(r1), ToString(r2), ToString(r3))
		} else {
			fmt.Printf("FAIL closure: got %s %s %s\n", ToString(r1), ToString(r2), ToString(r3))
		}
	}

	// TCO test (no stack overflow)
	_, err = EvalString(`(define (range-tco n acc)
		(if (= n 0) acc
			(range-tco (- n 1) (cons n acc))))`, globalEnv)
	if err != nil {
		fmt.Printf("FAIL TCO setup: %v\n", err)
	} else {
		r, err := EvalString(`(range-tco 1000 '())`, globalEnv)
		if err != nil {
			fmt.Printf("FAIL TCO: %v\n", err)
		} else {
			l := length(r)
			if l == 1000 {
				passed++
				fmt.Printf("PASS TCO test: length=%d\n", l)
			} else {
				fmt.Printf("FAIL TCO: expected length 1000, got %d\n", l)
			}
		}
	}

	if passed == len(tests)+len(libTests)+3 {
		fmt.Printf("\nAll %d tests PASSED\n", passed)
	} else {
		fmt.Printf("\n%d/%d passed\n", passed, len(tests)+len(libTests)+3)
	}
}
