package microlisp

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"sync"
)

// evalMu protects all microlisp evaluation from concurrent access.
// The interpreter has extensive global mutable state (globalEnv, evalDepth,
// handlerStack, etc.) and is NOT safe for concurrent use.
var evalMu sync.Mutex

// SafeEvalString evaluates a Lisp expression with thread safety.
// InitGlobalEnv() must be called once before first use.
func SafeEvalString(s string) (string, error) {
	evalMu.Lock()
	defer evalMu.Unlock()
	result, err := EvalString(s, globalEnv)
	if err != nil {
		return "", err
	}
	return ToString(result), nil
}

// captureStdout captures output written to os.Stdout (fmt.Print etc.) during fn.
// It returns the captured stdout text and the result from fn.
func captureStdout(fn func() error) (output string, err error) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		io.Copy(&buf, r)
		close(done)
	}()

	defer func() {
		w.Close()
		os.Stdout = oldStdout
		<-done
		output = buf.String()
		r.Close()
	}()

	err = fn()
	return
}

// SafeEvalStringCapture evaluates a Lisp expression and captures all stdout output
// produced during evaluation (from display/newline etc.).
// It returns (capturedStdout, returnValue, evalError).
func SafeEvalStringCapture(s string) (captured, returnValue string, err error) {
	evalMu.Lock()
	defer evalMu.Unlock()

	var result *Value
	captured, evalErr := captureStdout(func() error {
		var e error
		result, e = EvalString(s, globalEnv)
		return e
	})
	err = evalErr
	if err == nil && result != nil {
		returnValue = ToString(result)
	}
	return
}

// SafeLoadFile loads a Lisp source file with thread safety.
// It captures all stdout output produced during loading (from display/newline etc.)
// and returns it along with any error. InitGlobalEnv() must be called once before first use.
func SafeLoadFile(fname string) (output string, err error) {
	evalMu.Lock()
	defer evalMu.Unlock()

	output, loadErr := captureStdout(func() error {
		_, e := LoadFile(fname, globalEnv)
		return e
	})
	err = loadErr
	return
}

// ResetGlobalEnv reinitializes the interpreter to its clean state.
// This clears all user-defined variables, functions, macros, and classes,
// and restores the standard library and built-in functions.
func ResetGlobalEnv() {
	evalMu.Lock()
	defer evalMu.Unlock()
	// Reset all mutable global state. InitGlobalEnv will recreate
	// maps that it initializes (lispFeatures, lispModules, packages).
	globalEnv = NewEnv(nil)
	evalDepth = 0
	handlerStack = nil
	restartStack = nil
	innermostRestartFrame = -1
	classRegistry = make(map[string]*Value)
	compilerMacros = make(map[string]*Value)
	structPrintFns = make(map[string]*Value)
	symbolTable = make(map[string]*Value)
	docstrings = make(map[string]string)
	traceTable = make(map[string]bool)
	traceDepth = 0
	gensymCounter = 0
	nextThreadID = 0
	threadChannels = make(map[int64]chan threadResult)
	lockMutexMap = make(map[int64]*sync.Mutex)
	condVars = make(map[int64]*sync.Cond)
	logicalPathnameTranslations = map[string]*Value{}
	// These are set by InitGlobalEnv sub-calls (initFeatures/initPackages),
	// so we clear them to empty; InitGlobalEnv will repopulate.
	lispFeatures = make(map[string]bool)
	lispModules = make(map[string]bool)
	// packages is map[string]*Package — clear via reflection-free approach:
	// initPackages creates fresh packages, so just empty the map.
	for k := range packages {
		delete(packages, k)
	}
	// Re-initialize the global environment with builtins and stdlib
	InitGlobalEnv()
}

// LintString parses Lisp source without evaluating. Returns any syntax errors.
func LintString(s string) error {
	l := lex(s)
	p := &Parser{l: l, ptoks: make([]Tok, 0, 64), readtable: currentReadtable, env: globalEnv}
	p.advance()
	for p.tok.typ != TEOF {
		p.readtable = currentReadtable
		p.env = globalEnv
		v, err := p.readExpr()
		if err != nil {
			return err
		}
		if v == nil {
			return fmt.Errorf("syntax error: unmatched close parenthesis")
		}
	}
	return nil
}

// LintFile reads a Lisp source file and parses it without evaluating.
func LintFile(fname string) error {
	data, err := os.ReadFile(fname)
	if err != nil {
		return err
	}
	return LintString(string(data))
}
