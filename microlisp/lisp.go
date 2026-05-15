package microlisp

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"math"
	"math/big"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"
	"unsafe"
)

// -------- Types --------
type ValType int

const (
	VNil ValType = iota
	VNum
	VRat
	VComplex
	VStr
	VSym
	VBool
	VPair
	VPrim
	VFunc
	VMacro
	VSymMacro
	VRestart
	VClass
	VGeneric
	VInstance
	VVHash
	VThread
	VLock
	VCondition
	VChar
	VStream
	VMultiVal
	VArray
	VBigInt
	VPathname
	VPackage
	VReadtable
	VRandomState
	VMethod
)

// LispStream represents an I/O stream
type LispStream struct {
	file      *os.File
	reader    *bufio.Reader
	writer    *bufio.Writer
	isFile    bool
	isInput   bool
	isOutput  bool
	isClosed  bool
	isString  bool
	strBuf    *bytes.Buffer
	strReader *strings.Reader
	path      string
	// Composite stream types
	isSynonym        bool
	synSym           string // symbol name to resolve for synonym-stream
	isBroadcast      bool
	broadcastTargets []*Value // list of streams to broadcast to
	isConcatenated   bool
	concatStreams    []*Value // ordered list of input streams
	concatIndex      int      // current stream index
	isTwoWay         bool
	twoWayInput      *Value // input stream
	twoWayOutput     *Value // output stream
	isEcho           bool   // echo-stream: echo reads to output
	// Peek/unread support
	peekedChar rune   // cached peeked character
	hasPeeked  bool   // whether peekedChar is valid
	unreadBuf  []rune // stack of characters pushed back by unread-char
}

// LispPathname represents a Common Lisp pathname
type LispPathname struct {
	host      string // host (e.g., "localhost", or "" for unspecified)
	device    string // device (empty for Unix-style)
	directory *Value // list like (:absolute "usr" "local") or nil
	name      string // file name
	ftype     string // type/extension
	version   string // "newest", "", or version number
}

// Logical pathname translation table: host -> list of (from-pattern to-pattern)
var logicalPathnameTranslations = map[string]*Value{}

// LispArray represents a multi-dimensional array
type LispArray struct {
	dims       []int    // dimensions
	elements   []*Value // flat storage (row-major)
	fillPtr    int      // fill-pointer for vectors (-1 if no fill-pointer)
	adjustable bool     // whether array is adjustable
	elemType   string   // element type (e.g., "T", "CHARACTER", "BIT", "SINGLE-FLOAT")
}

type Value struct {
	typ    ValType
	mark   byte
	gen    int
	num    float64
	isFloat bool // true when value was explicitly created as a floating-point number
	ch     rune
	irat   int64
	iden   int64
	imag   float64
	str    string
	name   string // function name for trace
	car    *Value
	cdr    *Value
	params []string
	rest   string
	whole  string // &whole binding
	optDefaults []*Value // default expressions for &optional params (nil = default to NIL)
	keySpecs    []*Value // (key-name param-name default) triples for &key params
	body   *Value
	env    *Env
	fn     NativeFunc

	// CLOS fields
	className    string
	classSlots   []string
	classParents []*Value
	cpl          []*Value
	genMethods   []genMethod
	methodCombo  string         // method combination type: "standard", "progn", "and", "or", "list", "append", "nconc", "min", "max", "+"
	instClass    *Value
	instSlots    map[string]*Value
	hashTab      *HashTable
	stream       *LispStream
	array        *LispArray
	bigInt       *big.Int
	plist        *Value        // symbol property list
	pathname     *LispPathname // pathname components
	pkg          *Package      // for VPackage type
	readtable    *Readtable    // for VReadtable type
	randState   *rand.Rand   // for VRandomState type
	methodGF    *Value       // generic function for VMethod type
	methodIdx   int          // method index in genMethods for VMethod type
}

type genMethod struct {
	qualifier    string
	params       []string
	specializers []string
	body         *Value
	env          *Env
}

type HashTable struct {
	testFn          *Value
	hashFn          *Value
	table           map[uint64][]*hashEntry
	count           int
	rehashSize      float64
	rehashThreshold float64
}

type hashEntry struct {
	key   *Value
	value *Value
}

type NativeFunc func([]*Value) (*Value, error)

type blockReturn struct {
	name         string
	value        *Value
	isLoopFinish bool
}

func (br *blockReturn) Error() string { return "<block-return>" }

type goTag struct {
	tag string
}

func (gt *goTag) Error() string { return "<go-tag>" }

type throwValue struct {
	tag   string
	value *Value
}

func (tv *throwValue) Error() string { return "<throw>" }

// tailCall is used to implement proper tail-call optimization (TCO).
// Instead of making a recursive Go call in tail position (which would
// grow the Go stack), we return a tailCall error. The eval loop
// catches it and continues with the new form/environment.
type tailCall struct {
	form *Value
	env  *Env
}

func (tc *tailCall) Error() string { return "<tail-call>" }

type Env struct {
	bindings map[string]*Value
	parent   *Env
}

func NewEnv(parent *Env) *Env {
	return &Env{bindings: make(map[string]*Value), parent: parent}
}

func (e *Env) Get(s string) (*Value, error) {
	sUp := strings.ToUpper(s)
	sLo := strings.ToLower(s)
	for scope := e; scope != nil; scope = scope.parent {
		if v, ok := scope.bindings[s]; ok {
			return v, nil
		}
		// Case-insensitive lookup for CL compatibility (:UPCASE readtable)
		if s != sUp {
			if v, ok := scope.bindings[sUp]; ok {
				return v, nil
			}
		}
		if s != sLo {
			if v, ok := scope.bindings[sLo]; ok {
				return v, nil
			}
		}
	}
	return nil, fmt.Errorf("undefined: %s", s)
}

func (e *Env) Set(s string, v *Value) { e.bindings[s] = v }

func (e *Env) Extend(s string, v *Value) *Env {
	child := NewEnv(e)
	child.bindings[s] = v
	return child
}

// Global class registry — separates class namespace from function namespace
var classRegistry = make(map[string]*Value)

var globalEnv = NewEnv(nil)

// Features for reader conditionals (#+ / #-)
var lispFeatures map[string]bool

// Modules table for provide/require
var lispModules map[string]bool

// Compiler macro registry — stores define-compiler-macro definitions
var compilerMacros = make(map[string]*Value)

// Struct print functions — stores :print-function definitions for defstruct
var structPrintFns = make(map[string]*Value)

// Threading globals
var nextThreadID int64

type threadResult struct {
	value *Value
	err   error
}

var threadChannels = make(map[int64]chan threadResult)
var threadChannelsMu sync.Mutex

// Stream globals
var stdoutStream *Value
var stdinStream *Value
var stderrStream *Value

// Condition system globals
type handlerEntry struct {
	typeSymbol string
	handlerFn  *Value
	env        *Env
}
type restartEntry struct {
	name      string
	handlerFn *Value
	condition *Value
	env       *Env
	id        int // unique ID for restart object identity
}

var handlerStack []handlerEntry
var restartStack []restartEntry
var nextRestartID int
var signaledCondition *Value // tracks the currently-being-handled condition
var innermostRestartFrame int = -1 // start index of innermost restart-case's restarts

// Recursion depth limit to prevent infinite recursion / stack overflow
const maxEvalDepth = 3000

var evalDepth int

// Loop iteration safety limit
const maxLoopIterations = 10000000

var loopIterationCount int

// handledError is used internally by error/signal to carry a handled condition result.
// restartInvoke is used by invoke-restart to transfer control to restart-case.
type restartInvoke struct {
	name string
	args *Value
	id   int // restart ID for precise matching
}

func (r *restartInvoke) Error() string { return "restart-invoke" }

type handledError struct {
	condition *Value
	result    *Value
	typeSym   string // the type symbol of the matching handler
}

func (h *handledError) Error() string { return "handled" }

// conditionError wraps a Go error with the condition that was signaled,
// so restart-case can auto-associate its restarts with the condition.
type conditionError struct {
	condition *Value
	err       error
}

func (c *conditionError) Error() string { return c.err.Error() }
func (c *conditionError) Unwrap() error { return c.err }

func initFeatures() {
	lispFeatures = make(map[string]bool)
	features := []string{
		":microlisp", ":common-lisp", ":ansi-cl",
		":windows", ":x86-64",
		":threading", ":sb-unicode",
	}
	for _, f := range features {
		lispFeatures[f] = true
	}
	// Create *features* Lisp list
	var featList *Value
	for i := len(features) - 1; i >= 0; i-- {
		featList = cons(vsym(features[i]), featList)
	}
	if featList == nil {
		featList = vnil()
	}
	globalEnv.Set("*features*", featList)
}

// featureSatisfied checks a feature spec against lispFeatures
func featureSatisfied(spec *Value) bool {
	return featureSatisfiedEnv(spec, globalEnv, make(map[*Value]bool))
}

func featureSatisfiedEnv(spec *Value, env *Env, seen map[*Value]bool) bool {
	if spec.typ == VSym && len(spec.str) > 0 && spec.str[0] == ':' {
		return lispFeatures[strings.ToLower(spec.str)]
	}
	if spec.typ == VSym {
		return lispFeatures[strings.ToLower(":"+spec.str)] || lispFeatures[strings.ToLower(spec.str)]
	}
	if spec.typ == VPair {
		if seen[spec] {
			return false
		}
		seen[spec] = true
		// Compound feature spec: (and ...), (or ...), (not feature)
		car := spec.car
		if car.typ == VSym {
			switch car.str {
			case "and":
				args := spec.cdr
				seenArgs := make(map[*Value]bool)
				for !isNil(args) {
					if seenArgs[args] {
						break
					}
					seenArgs[args] = true
					if !featureSatisfiedEnv(args.car, env, seen) {
						return false
					}
					args = args.cdr
				}
				return true
			case "or":
				args := spec.cdr
				seenOrArgs := make(map[*Value]bool)
				for !isNil(args) {
					if seenOrArgs[args] {
						break
					}
					seenOrArgs[args] = true
					if featureSatisfiedEnv(args.car, env, seen) {
						return true
					}
					args = args.cdr
				}
				return false
			case "not":
				if !isNil(spec.cdr) {
					return !featureSatisfiedEnv(spec.cdr.car, env, seen)
				}
				return true
			}
		}
	}
	return false
}

func initStdStreams() {
	stdoutStream = newFileStream(os.Stdout, false, true, "")
	stdinStream = newFileStream(os.Stdin, true, false, "")
	stderrStream = newFileStream(os.Stderr, false, true, "")
	globalEnv.Set("*standard-output*", stdoutStream)
	globalEnv.Set("*standard-input*", stdinStream)
	globalEnv.Set("*error-output*", stderrStream)
	globalEnv.Set("*query-io*", stdinStream) // Default query-io uses stdin/stdout
	globalEnv.Set("*terminal-io*", stdoutStream) // Default terminal-io
}

func InitGlobalEnv() {
	initFeatures()
	initStdStreams()
	lispModules = make(map[string]bool)
	initPackages()
	// Initialize standard readtable BEFORE EvalString(initLib) so that
	// the lexer has a valid readtable during initLib evaluation.
	initStandardReadtable()
	globalEnv.Set("nil", &Value{typ: VNil})
	globalEnv.Set("#t", &Value{typ: VBool})
	globalEnv.Set("#f", &Value{typ: VBool})
	// Make 't' also evaluate to #t (CL compatibility: both 't' and '#t' are canonical true)
	tSym := &Value{typ: VSym, str: "t"}
	symbolTable["t"] = tSym
	globalEnv.Set("t", globalEnv.bindings["#t"])
	for _, b := range builtins {
		globalEnv.Set(b.name, &Value{typ: VPrim, fn: b.fn})
	}
	globalEnv.Set("*modules*", vnil())
	// *package* is set by initPackages via makePackage
	globalEnv.Set("*debugger-hook*", vnil())
	globalEnv.Set("*step*", vnil())
	globalEnv.Set("*break-on-signals*", vnil())
	globalEnv.Set("*print-circle*", vbool(false))
	globalEnv.Set("*print-case*", vsym(":UPCASE"))
	globalEnv.Set("*print-escape*", vbool(true))
	globalEnv.Set("*print-length*", vnil())
	globalEnv.Set("*print-level*", vnil())
	globalEnv.Set("*print-pretty*", vbool(false))
	globalEnv.Set("*print-base*", vnum(10))
	globalEnv.Set("*print-radix*", vbool(false))
	globalEnv.Set("*print-readably*", vbool(true))
	globalEnv.Set("*print-gensym*", vbool(true))
	globalEnv.Set("*print-array*", vbool(true))
	globalEnv.Set("*read-base*", vnum(10))
	globalEnv.Set("*read-default-float-format*", vsym("SINGLE-FLOAT"))
	globalEnv.Set("*read-eval*", vbool(true))
	globalEnv.Set("*read-suppress*", vbool(false))
	// Standard load-time special variables (bound dynamically during load)
	globalEnv.Set("*load-pathname*", vnil())
	globalEnv.Set("*load-truename*", vnil())
	// Initialize *random-state* per CL spec
	rs, _ := builtinMakeRandomState(nil)
	globalEnv.Set("*random-state*", rs)
	// Initialize *posix-argv* (SBCL extension): list of command-line argument strings
	{
		argvVals := make([]*Value, len(os.Args))
		for i, a := range os.Args {
			argvVals[i] = vstr(a)
		}
		globalEnv.Set("*posix-argv*", listFromSlice(argvVals))
	}
	if initLib != "" {
		_, err := EvalString(initLib, globalEnv)
		if err != nil {
			fmt.Fprintf(os.Stderr, "initLib error: %v\n", err)
		}
	}
	// Sync CL package with USER package (after all builtins/initLib are registered)
	syncCLPackage()
	// Set *readtable* variable (readtable is already initialized above)
	globalEnv.Set("*readtable*", vrt(standardReadtable))
	// CL constant: exclusive upper bound on character codes (Unicode has 1114112 code points)
	globalEnv.Set("char-code-limit", vnum(1114112))
		// CL standard constants
		globalEnv.Set("pi", vnum(math.Pi))
		globalEnv.Set("most-positive-fixnum", vnum(4611686018427387903))
		globalEnv.Set("most-negative-fixnum", vnum(-4611686018427387904))
		// CL floating-point constants (float64 values)
		globalEnv.Set("most-positive-single-float", vfloat(math.MaxFloat32))
		globalEnv.Set("most-positive-double-float", vfloat(math.MaxFloat64))
		globalEnv.Set("most-positive-long-float", vfloat(math.MaxFloat64))
		globalEnv.Set("most-positive-short-float", vfloat(math.MaxFloat32))
		globalEnv.Set("least-positive-single-float", vfloat(math.SmallestNonzeroFloat32))
		globalEnv.Set("least-positive-double-float", vfloat(math.SmallestNonzeroFloat64))
		globalEnv.Set("least-positive-long-float", vfloat(math.SmallestNonzeroFloat64))
		globalEnv.Set("least-positive-short-float", vfloat(math.SmallestNonzeroFloat32))
		globalEnv.Set("least-positive-normalized-single-float", vfloat(math.Float64frombits(0x00800000)))
		globalEnv.Set("least-positive-normalized-double-float", vfloat(math.Float64frombits(0x0010000000000000)))
		globalEnv.Set("most-negative-single-float", vfloat(-math.MaxFloat32))
		globalEnv.Set("most-negative-double-float", vfloat(-math.MaxFloat64))
		globalEnv.Set("most-negative-long-float", vfloat(-math.MaxFloat64))
		globalEnv.Set("most-negative-short-float", vfloat(-math.MaxFloat32))
		globalEnv.Set("least-negative-single-float", vfloat(-math.SmallestNonzeroFloat32))
		globalEnv.Set("least-negative-double-float", vfloat(-math.SmallestNonzeroFloat64))
		globalEnv.Set("least-negative-long-float", vfloat(-math.SmallestNonzeroFloat64))
		globalEnv.Set("least-negative-short-float", vfloat(-math.SmallestNonzeroFloat32))
		globalEnv.Set("least-negative-normalized-single-float", vfloat(-math.Float64frombits(0x00800000)))
		globalEnv.Set("least-negative-normalized-double-float", vfloat(-math.Float64frombits(0x0010000000000000)))
		globalEnv.Set("single-float-epsilon", vfloat(math.Float64frombits(0x3CB0000000000000)))
		globalEnv.Set("double-float-epsilon", vfloat(math.Float64frombits(0x3CB0000000000000)))
		globalEnv.Set("short-float-epsilon", vfloat(math.Float64frombits(0x3CB0000000000000)))
		globalEnv.Set("long-float-epsilon", vfloat(math.Float64frombits(0x3CB0000000000000)))
		globalEnv.Set("single-float-negative-epsilon", vfloat(math.Float64frombits(0x3C90000000000000)))
		globalEnv.Set("double-float-negative-epsilon", vfloat(math.Float64frombits(0x3C90000000000000)))
		// CL boole operation constants (ANSI CL standard)
		globalEnv.Set("boole-clr", vnum(0))
		globalEnv.Set("boole-and", vnum(1))
		globalEnv.Set("boole-andc1", vnum(2))
		globalEnv.Set("boole-1", vnum(3))
		globalEnv.Set("boole-andc2", vnum(4))
		globalEnv.Set("boole-2", vnum(5))
		globalEnv.Set("boole-xor", vnum(6))
		globalEnv.Set("boole-ior", vnum(7))
		globalEnv.Set("boole-nor", vnum(8))
		globalEnv.Set("boole-eqv", vnum(9))
		globalEnv.Set("boole-c2", vnum(10))
		globalEnv.Set("boole-orc2", vnum(11))
		globalEnv.Set("boole-c1", vnum(12))
		globalEnv.Set("boole-orc1", vnum(13))
		globalEnv.Set("boole-nand", vnum(14))
		globalEnv.Set("boole-set", vnum(15))
}

// -------- GC --------
var allValues []*Value
var minorCount int

const youngThreshold = 20000
const majorEvery = 5

func gcv() *Value {
	v := &Value{}
	allValues = append(allValues, v)
	if len(allValues) >= youngThreshold {
		gcollect()
	}
	return v
}

func gcollect() {
	minorCount++
	doMajor := minorCount%majorEvery == 0
	markRoots()
	n := 0
	for _, v := range allValues {
		if v.mark != 0 {
			v.mark = 0
			if v.gen < 3 {
				v.gen++
			}
			allValues[n] = v
			n++
		} else if !doMajor && v.gen > 0 {
			allValues[n] = v
			n++
		}
	}
	allValues = allValues[:n]
}

func markRoots() {
	markEnv(globalEnv)
	for _, sym := range symbolTable {
		markVal(sym)
	}
}

func markVal(v *Value) {
	if v == nil || v.mark != 0 {
		return
	}
	v.mark = 1
	switch v.typ {
	case VPair:
		markVal(v.car)
		markVal(v.cdr)
	case VFunc, VMacro:
		markVal(v.body)
		if v.env != nil {
			markEnv(v.env)
		}
	case VClass:
		for _, p := range v.classParents {
			markVal(p)
		}
		for _, c := range v.cpl {
			markVal(c)
		}
	case VGeneric:
		for _, m := range v.genMethods {
			markVal(m.body)
			if m.env != nil {
				markEnv(m.env)
			}
		}
	case VInstance:
		markVal(v.instClass)
		for _, sv := range v.instSlots {
			markVal(sv)
		}
	case VVHash:
		if v.hashTab != nil {
			for _, bucket := range v.hashTab.table {
				for _, entry := range bucket {
					markVal(entry.key)
					markVal(entry.value)
				}
			}
		}
	case VArray:
		if v.array != nil {
			for _, elem := range v.array.elements {
				markVal(elem)
			}
		}
	}
}

func markEnv(e *Env) {
	for s := e; s != nil; s = s.parent {
		for _, v := range s.bindings {
			markVal(v)
		}
	}
}

// -------- Value helpers --------
var symbolTable = make(map[string]*Value)

// -------- Package System --------
type Package struct {
	name          string
	symbols       map[string]*Value
	exports       map[string]bool
	used          []*Package
	nicknames     []string
	shadowingImps []string // symbols imported via shadowing-import
}

var packages = map[string]*Package{}

// -------- Readtable --------

// macroEntry stores a readtable macro character function.
// Either goFn or lispFn should be set, not both.
type macroEntry struct {
	goFn        func(*Parser, rune) (*Value, error) // Go-level macro function
	lispFn      *Value                              // Lisp-level VFunc or VPrim
	terminating bool                                // true = terminating macro char
}

// closeParenSentinel is the special VPrim function returned by
// get-macro-character for ')', recognized by set-macro-character
// to register the target character as a close-paren equivalent.
var closeParenSentinel *Value

// Readtable controls how the reader parses input.
type Readtable struct {
	macroFns map[rune]*macroEntry // character → macro function
	dispFns  map[rune]*macroEntry // # dispatch: sub-character → function
	caseMode string               // :UPCASE, :DOWNCASE, :PRESERVE, :INVERT
}

var standardReadtable *Readtable
var currentReadtable *Readtable

func initStandardReadtable() {
	standardReadtable = &Readtable{
		macroFns: make(map[rune]*macroEntry),
		dispFns:  make(map[rune]*macroEntry),
		caseMode: ":UPCASE",
	}
	currentReadtable = standardReadtable

	// Register standard macro characters with Go-level entries for introspection.
	// The lexer/parser handles these via hardcoded switch cases, but the entries
	// allow get-macro-character to return useful information.
	registerGoMacro := func(ch rune, term bool) {
		standardReadtable.macroFns[ch] = &macroEntry{
			goFn:        nil,
			lispFn:      nil,
			terminating: term,
		}
	}

	// Standard terminating macro characters
	registerGoMacro('(', true)
	registerGoMacro(')', true)
	registerGoMacro('"', true)
	registerGoMacro(';', true)

	// Standard non-terminating macro characters
	registerGoMacro('\'', false)
	registerGoMacro('`', false)
	registerGoMacro(',', false)
	registerGoMacro('#', false)

	// Initialize dispatch table
	standardReadtable.dispFns = make(map[rune]*macroEntry)
	// Note: actual dispatch handling is done in the parser's dispatch reader.
	// Dispatch entries are nil by default (handled by hardcoded parser logic).
	// They can be overridden with set-dispatch-macro-character.

	// Create the close-paren sentinel - a VPrim function that represents
	// the behavior of ')'. get-macro-character returns this for ')', and
	// set-macro-character recognizes it to register close-paren equivalence.
	closeParenSentinel = &Value{typ: VPrim, fn: func(args []*Value) (*Value, error) {
		return nil, fmt.Errorf("unmatched close parenthesis")
	}}
}

func findPackage(name string) *Package {
	up := strings.ToUpper(name)
	for _, p := range packages {
		if p.name == up {
			return p
		}
		for _, n := range p.nicknames {
			if strings.ToUpper(n) == up {
				return p
			}
		}
	}
	return nil
}

func makePackage(name string) *Package {
	if p := findPackage(name); p != nil {
		return p
	}
	p := &Package{
		name:    strings.ToUpper(name),
		symbols: make(map[string]*Value),
		exports: make(map[string]bool),
	}
	packages[p.name] = p
	// Set current package
	currentPackage = p
	// Update *package* special variable
	globalEnv.Set("*package*", vpkg(p))
	return p
}

var currentPackage *Package

func initPackages() {
	// Create KEYWORD package
	makePackage("KEYWORD")
	// Create initial user package (with CL-USER as standard nickname)
	userPkg := makePackage("USER")
	userPkg.nicknames = append(userPkg.nicknames, "CL-USER")
	// Create CL package (will be synced with USER after initLib)
	makePackage("CL")
}

// syncCLPackage copies all symbols from USER to CL so (defpackage ... (:use :cl)) works.
// It also adds special operators and all builtin function names to the CL package's
// exports so that cl:NAME syntax works for all standard CL names.
func syncCLPackage() {
	userPkg := findPackage("USER")
	clPkg := findPackage("CL")
	if userPkg == nil || clPkg == nil {
		return
	}
	for name, sym := range userPkg.symbols {
		clPkg.symbols[name] = sym
	}
	for name, exported := range userPkg.exports {
		if exported {
			clPkg.exports[name] = true
		}
	}
	// Add all special operators to CL package exports
	for _, name := range specialOpNames {
		sym := vsym(name)
		clPkg.symbols[name] = sym
		clPkg.exports[name] = true
	}
	// Add all builtin functions to CL package exports
	for _, b := range builtins {
		sym := vsym(b.name)
		clPkg.symbols[b.name] = sym
		clPkg.exports[b.name] = true
	}
}

// internSymbol interns a symbol name in a package, returning the symbol
func internSymbol(name string, pkg *Package) *Value {
	sym := vsym(name)
	pkg.symbols[name] = sym
	return sym
}

// isKeyword checks if a symbol name represents a keyword
func isKeyword(name string) bool {
	return len(name) > 0 && name[0] == ':'
}

// keywordName strips the leading colon from a keyword
func keywordName(name string) string {
	if len(name) > 0 && name[0] == ':' {
		return name[1:]
	}
	return name
}

// mkKeyword creates a keyword symbol from a mode string like ":UPCASE"
func mkKeyword(modeStr string) *Value {
	if len(modeStr) > 0 && modeStr[0] != ':' {
		return vsym(":" + modeStr)
	}
	return vsym(modeStr)
}

// isSpecialOp returns true if name is a special operator handled by eval.
func isSpecialOp(name string) bool {
	for _, s := range specialOpNames {
		if s == name {
			return true
		}
	}
	return false
}

// resolvePackageSymbol splits "pkg:sym" or "pkg::sym" and resolves the symbol.
// For single-colon forms (pkg:sym), it returns nil if the symbol is not exported.
// For double-colon forms (pkg::sym), it interns the symbol internally if not found.
func resolvePackageSymbol(s string) *Value {
	if strings.Contains(s, "::") {
		parts := strings.SplitN(s, "::", 2)
		pkg := findPackage(parts[0])
		if pkg == nil {
			return nil
		}
		symName := parts[1]
		if sym, ok := pkg.symbols[symName]; ok {
			return sym
		}
		// Intern if not found (internal access creates)
		return internSymbol(symName, pkg)
	}
	if strings.Contains(s, ":") {
		parts := strings.SplitN(s, ":", 2)
		pkg := findPackage(parts[0])
		if pkg == nil {
			return nil
		}
		symName := parts[1]
		if sym, ok := pkg.symbols[symName]; ok && pkg.exports[symName] {
			return sym
		}
		// For package-qualified special operators (e.g. cl:defparameter),
		// intern the symbol in the package even if not explicitly exported,
		// since special operators are part of the package's API.
		if isSpecialOp(symName) {
			sym := vsym(symName)
			pkg.symbols[symName] = sym
			pkg.exports[symName] = true
			return sym
		}
		// Fallback: check global symbolTable for canonical symbols like nil, t
		// This handles cl:nil, cl:t, etc.
		if sym, ok := symbolTable[symName]; ok {
			return sym
		}
		return nil // not found or not exported
	}
	return nil
}

// resolvePackageFromDesignator converts a package designator (string, symbol, or VPackage) to a *Package.
func resolvePackageFromDesignator(v *Value) *Package {
	if v == nil || isNil(v) {
		return currentPackage
	}
	if v.typ == VPackage {
		return v.pkg
	}
	if v.typ == VStr {
		return findPackage(v.str)
	}
	if v.typ == VSym {
		name := v.str
		if isKeyword(name) {
			name = keywordName(name)
		}
		return findPackage(name)
	}
	return nil
}

// internOrVsym creates a symbol, properly interning it into the current package.
// This should be used by the reader instead of bare vsym().
// CL requires the reader to upcase unescaped symbol names by default.
func internOrVsym(name string) *Value {
	// CL reader case handling per *readtable-case*
	if currentReadtable != nil {
		switch currentReadtable.caseMode {
		case ":UPCASE":
			name = strings.ToUpper(name)
		case ":DOWNCASE":
			name = strings.ToLower(name)
		case ":PRESERVE":
			// keep name as-is
		case ":INVERT":
			// Flip case: if all uppercase, make lowercase; if all lowercase, make uppercase
			if name == strings.ToUpper(name) {
				name = strings.ToLower(name)
			} else if name == strings.ToLower(name) {
				name = strings.ToUpper(name)
			}
		default:
			name = strings.ToUpper(name)
		}
	} else {
		// CL default: upcase
		name = strings.ToUpper(name)
	}
	// Handle qualified symbols: pkg:sym or pkg::sym
	if resolved := resolvePackageSymbol(name); resolved != nil {
		return resolved
	}
	// Handle keywords: auto-intern into KEYWORD package
	if isKeyword(name) {
		kPkg := findPackage("KEYWORD")
		if kPkg != nil {
			if sym, ok := kPkg.symbols[name]; ok {
				return sym
			}
			// Create a fresh keyword symbol with the ":" prefix in its name.
			// This ensures it won't conflict with regular symbols in symbolTable.
			sym := gcv()
			sym.typ = VSym
			sym.str = name // keep ":" prefix
			kPkg.symbols[name] = sym
			return sym
		}
	}
	// Intern in current package
	cp := currentPackage
	if cp != nil {
		if sym, ok := cp.symbols[name]; ok {
			return sym
		}
		return internSymbol(name, cp)
	}
	// Fallback: create uninterned symbol
	return vsym(name)
}

func vnum(f float64) *Value { v := gcv(); v.typ = VNum; v.num = f; return v }
func vfloat(f float64) *Value { v := gcv(); v.typ = VNum; v.num = f; v.isFloat = true; return v }

// anyFloat returns vfloat(f) if any arg has isFloat set, else vnum(f)
func numOrFloat(f float64, args []*Value) *Value {
	for _, a := range args {
		if a.typ == VNum && a.isFloat {
			return vfloat(f)
		}
	}
	return vnum(f)
}
func vrat(n, d int64) *Value {
	if d == 0 {
		return nil
	}
	if d < 0 {
		n = -n
		d = -d
	}
	g := gcd(n, d)
	if g < 0 {
		g = -g
	}
	n /= g
	d /= g
	if d == 1 {
		return vnum(float64(n))
	}
	v := gcv()
	v.typ = VRat
	v.irat = n
	v.iden = d
	return v
}

// varray creates a 1-D VArray from a slice of values.
func varray(elems []*Value) *Value {
	v := gcv()
	v.typ = VArray
	v.array = &LispArray{dims: []int{len(elems)}, elements: elems, fillPtr: -1}
	return v
}
func vcomplex(r, i float64) *Value {
	if i == 0 {
		return vnum(r)
	}
	v := gcv()
	v.typ = VComplex
	v.num = r
	v.imag = i
	return v
}

// vcomplexAlways creates a VComplex value even when imaginary part is zero.
// Used for (coerce x '(complex float)) where the result type must be complex.
// Sets isFloat=true so that printing preserves the float appearance (e.g., #c(1.0 0.0)).
func vcomplexAlways(r, i float64) *Value {
	v := gcv()
	v.typ = VComplex
	v.num = r
	v.imag = i
	v.isFloat = true
	return v
}

// formatComplexPart formats a float64 part of a complex number,
// ensuring that if isFloat is true, the result always has a decimal point.
func formatComplexPart(f float64, isFloat bool) string {
	s := strconv.FormatFloat(f, 'g', -1, 64)
	if isFloat && !strings.Contains(s, ".") && !strings.Contains(s, "e") && !strings.Contains(s, "E") && !strings.Contains(s, "inf") && !strings.Contains(s, "Inf") && !strings.Contains(s, "NaN") {
		s += ".0"
	}
	return s
}
func gcd(a, b int64) int64 {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

// parseFloatStr parses a string as a float64, handling integers, floats, and rationals (e.g. "1/2")
func parseFloatStr(s string) (float64, error) {
	// Normalize CL float exponent markers (d, f, s, l) to Go's 'e' for ParseFloat
	var s2 strings.Builder
	for _, ch := range s {
		if ch == 'd' || ch == 'D' || ch == 'f' || ch == 'F' || ch == 's' || ch == 'S' || ch == 'l' || ch == 'L' {
			s2.WriteRune('e')
		} else {
			s2.WriteRune(ch)
		}
	}
	s = s2.String()
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f, nil
	}
	// Try rational like "1/2"
	if idx := strings.IndexByte(s, '/'); idx > 0 {
		num, err1 := strconv.ParseFloat(s[:idx], 64)
		den, err2 := strconv.ParseFloat(s[idx+1:], 64)
		if err1 == nil && err2 == nil && den != 0 {
			return num / den, nil
		}
	}
	return 0, fmt.Errorf("not a number: %s", s)
}

func vbigint(i *big.Int) *Value {
	// Only auto-downgrade to float64 if value fits in float64 mantissa exactly.
	// float64 has 53 bits of mantissa precision, so only ±2^53 are exact.
	if i.IsInt64() {
		n := i.Int64()
		if n >= -9007199254740992 && n <= 9007199254740992 {
			return vnum(float64(n))
		}
	}
	v := gcv()
	v.typ = VBigInt
	v.bigInt = new(big.Int).Set(i)
	return v
}
func vbigInt(i *big.Int) *Value {
	if i.IsInt64() {
		n := i.Int64()
		if n >= -9007199254740992 && n <= 9007199254740992 {
			return vnum(float64(n))
		}
	}
	v := gcv()
	v.typ = VBigInt
	v.bigInt = new(big.Int).Set(i)
	return v
}
func vstr(s string) *Value { v := gcv(); v.typ = VStr; v.str = s; return v }
func vsym(s string) *Value {
	if sym, ok := symbolTable[s]; ok {
		return sym
	}
	v := gcv()
	v.typ = VSym
	v.str = s
	symbolTable[s] = v
	return v
}
func vbool(b bool) *Value {
	if b {
		return globalEnv.bindings["#t"]
	}
	return globalEnv.bindings["#f"]
}
func vchar(ch rune) *Value    { v := gcv(); v.typ = VChar; v.ch = ch; return v }
func vnil() *Value            { return globalEnv.bindings["nil"] }
func cons(a, b *Value) *Value { v := gcv(); v.typ = VPair; v.car = a; v.cdr = b; return v }
func list3(a, b, c *Value) *Value { return cons(a, cons(b, cons(c, vnil()))) }

func isNil(v *Value) bool { return v == nil || v.typ == VNil || (v.typ == VSym && strings.EqualFold(v.str, "nil")) }

// primaryValue extracts the primary value from a potentially multi-valued result.
func primaryValue(v *Value) *Value {
	if v != nil && v.typ == VMultiVal && v.car != nil {
		return v.car
	}
	return v
}

// multiVal creates a VMultiVal with the given values.
func multiVal(vals ...*Value) *Value {
	if len(vals) == 0 {
		v := gcv()
		v.typ = VMultiVal
		v.car = vnil()
		v.cdr = vnil()
		return v
	}
	v := gcv()
	v.typ = VMultiVal
	v.car = vals[0]
	v.cdr = list(vals[1:]...)
	return v
}

// multiValList extracts the list of values from a multi-valued result.
func multiValList(v *Value) *Value {
	if v != nil && v.typ == VMultiVal {
		return cons(v.car, v.cdr) // primary value + secondary values as full list
	}
	if isNil(v) {
		return vnil()
	}
	// It's already a proper list (VPair) - return as-is for floor/ceiling/etc. results
	return v
}
func isPair(v *Value) bool { return v != nil && v.typ == VPair }

func isTruthy(v *Value) bool {
	if v == nil {
		return false
	}
	return (v.typ != VBool || v == globalEnv.bindings["#t"]) && v.typ != VNil
}

func list(vv ...*Value) *Value {
	var r *Value = vnil()
	for i := len(vv) - 1; i >= 0; i-- {
		r = cons(vv[i], r)
	}
	return r
}

func listFromSlice(vv []*Value) *Value {
	var r *Value = vnil()
	for i := len(vv) - 1; i >= 0; i-- {
		r = cons(vv[i], r)
	}
	return r
}

func toSlice(v *Value) []*Value {
	var r []*Value
	seen := make(map[*Value]bool)
	for !isNil(v) && v.typ == VPair {
		if seen[v] {
			break // circular list detected
		}
		seen[v] = true
		r = append(r, v.car)
		v = v.cdr
	}
	return r
}

func length(v *Value) int {
	n := 0
	seen := make(map[*Value]bool)
	for !isNil(v) && v.typ == VPair {
		if seen[v] {
			break // circular list
		}
		seen[v] = true
		n++
		v = v.cdr
	}
	return n
}

// -------- Lexer --------
type TokType int

const (
	TErr TokType = iota
	TLParen
	TRParen
	TDot
	TQuote
	TQq
	TUnq
	TUnqS
	TNum
	TStr
	TSym
	TTrue
	TFalse
	TChar
	TEOF
	TFuncQuote
	TPathname
	TComplex
	TVector  // #( elements ) vector literal
	TMacro // readtable macro character
	TSharpDot   // #. read-time evaluation
	TSharpMacro // # dispatch macro character
)

type Tok struct {
	typ    TokType
	lit    string
	num    float64
	irat   int64   // rational numerator
	iden   int64   // rational denominator (0 = not rational)
	imag   float64 // complex imaginary part
	ch     rune
	pos    int
	bigInt *big.Int
	isBar  bool // true if symbol was read via |...| escapes (preserves case)
	isFlt  bool // true if number was parsed as a floating-point literal
}

type Lexer struct {
	src        []rune
	pos        int
	prevEndPos int // position right after the last token (before whitespace skip)
	tok        Tok
	err        error
	parser     *Parser
	bitVec     *Value
}

func lex(s string) *Lexer { return &Lexer{src: []rune(s)} }

func (l *Lexer) next() Tok {
	l.prevEndPos = l.pos // capture position before skipWS (right after previous token)
	l.skipWS()
	if l.pos >= len(l.src) {
		l.tok = Tok{typ: TEOF}
		return l.tok
	}
	p := l.pos
	ch := l.src[l.pos]
	l.pos++
	switch ch {
	case '(':
		l.tok = Tok{typ: TLParen, pos: p}
	case ')':
		l.tok = Tok{typ: TRParen, pos: p}
	case '.':
		l.tok = Tok{typ: TDot, pos: p}
	case '\'':
		l.tok = Tok{typ: TQuote, pos: p}
	case '`':
		l.tok = Tok{typ: TQq, pos: p}
	case ',':
		if l.pos < len(l.src) && l.src[l.pos] == '@' {
			l.pos++
			l.tok = Tok{typ: TUnqS, pos: p}
		} else {
			l.tok = Tok{typ: TUnq, pos: p}
		}
	case '"':
		return l.lexStr()
	case '|':
		return l.lexBarSym()
	case ';':
		for l.pos < len(l.src) && l.src[l.pos] != '\n' {
			l.pos++
		}
		return l.next()
	case '#':
		// #|...|# block comment — skip until closing |#
		if l.pos < len(l.src) && l.src[l.pos] == '|' {
			l.pos++ // skip |
			for l.pos+1 < len(l.src) {
				if l.src[l.pos] == '|' && l.src[l.pos+1] == '#' {
					l.pos += 2 // skip |#
					return l.next()
				}
				l.pos++
			}
			// Unterminated block comment — return next token to signal EOF
			return l.tok
		}
		if l.pos < len(l.src) && l.src[l.pos] == '\\' {
			return l.lexChar(p)
		}
		if l.pos < len(l.src) && l.src[l.pos] == '\x27' {
			// #' is function shorthand: #'name -> (function name)
			l.pos++ // skip quote
			l.tok = Tok{typ: TFuncQuote, pos: p}
			return l.tok
		}
		if l.pos < len(l.src) && l.src[l.pos] == '.' {
			// #. is read-time evaluation: #.expr reads and evaluates expr
			l.pos++ // skip .
			l.tok = Tok{typ: TSharpDot, pos: p}
			return l.tok
		}
		if l.pos < len(l.src) && (l.src[l.pos] == 'C' || l.src[l.pos] == 'c') {
			// #C(real imag) is complex number literal - let parser handle it
			l.pos++ // skip C
			if l.pos < len(l.src) && l.src[l.pos] == '(' {
				l.pos++ // skip (
				depth := 1
				start := l.pos
				for l.pos < len(l.src) && depth > 0 {
					if l.src[l.pos] == '(' {
						depth++
					} else if l.src[l.pos] == ')' {
						depth--
					}
					l.pos++
				}
				inner := strings.TrimSpace(string(l.src[start : l.pos-1]))
				l.tok = Tok{typ: TComplex, lit: inner, pos: p}
				return l.tok
			}
			// #C not followed by (, treat as symbol
			return l.lexSym()
		}
		if l.pos < len(l.src) && (l.src[l.pos] == 'P' || l.src[l.pos] == 'p') {
			// #P"..." is pathname syntax
			l.pos++ // skip P
			if l.pos < len(l.src) && l.src[l.pos] == '"' {
				l.pos++ // skip opening "
				var b strings.Builder
				for l.pos < len(l.src) {
					ch2 := l.src[l.pos]
					l.pos++
					if ch2 == '"' {
						l.tok = Tok{typ: TPathname, lit: b.String(), pos: p}
						return l.tok
					}
					if ch2 == '\\' && l.pos < len(l.src) {
						switch l.src[l.pos] {
						case 'n':
							b.WriteByte('\n')
						case 't':
							b.WriteByte('\t')
						case '"':
							b.WriteByte('"')
						case '\\':
							b.WriteByte('\\')
						default:
							b.WriteByte(byte(l.src[l.pos]))
						}
						l.pos++
						continue
					}
					b.WriteRune(ch2)
				}
				// Unterminated string
				l.tok = Tok{typ: TPathname, lit: b.String(), pos: p}
				return l.tok
			}
			// Not followed by string, treat #P as symbol
			return l.lexSym()
		}
		if l.pos < len(l.src) && l.src[l.pos] == '*' {
			// #*1010 is bit-vector literal
			l.pos++
			start := l.pos
			for l.pos < len(l.src) && (l.src[l.pos] == '0' || l.src[l.pos] == '1') {
				l.pos++
			}
			bits := string(l.src[start:l.pos])
			elements := make([]*Value, len(bits))
			for i := 0; i < len(bits); i++ {
				if bits[i] == '1' {
					elements[i] = vnum(1)
				} else {
					elements[i] = vnum(0)
				}
			}
			arr := &LispArray{dims: []int{len(elements)}, elements: elements, fillPtr: -1}
			v := gcv()
			v.typ = VArray
			v.array = arr
			l.tok = Tok{typ: TVector, lit: "/*bitvec*/", pos: p}
			l.bitVec = v
			return l.tok
		}
		if l.pos < len(l.src) && l.src[l.pos] == '(' {
			// #(...) is vector literal
			l.pos++ // skip (
			depth := 1
			start := l.pos
			for l.pos < len(l.src) && depth > 0 {
				if l.src[l.pos] == '(' {
					depth++
				} else if l.src[l.pos] == ')' {
					depth--
				}
				l.pos++
			}
			inner := string(l.src[start : l.pos-1])
			l.tok = Tok{typ: TVector, lit: inner, pos: p}
			return l.tok
		}
		// #+ and #- feature conditionals — return as symbol so parser can handle them
		if l.pos < len(l.src) && (l.src[l.pos] == '+' || l.src[l.pos] == '-') {
			l.pos++ // consume +/-
			return l.lexSymFrom(p)
		}
		// Check for user-registered dispatch macro characters
		if currentReadtable != nil && l.pos < len(l.src) {
			subCh := rune(l.src[l.pos])
			if entry, ok := currentReadtable.dispFns[subCh]; ok && entry != nil && entry.lispFn != nil {
				l.pos++ // consume sub-char
				l.tok = Tok{typ: TSharpMacro, ch: subCh, pos: p}
				return l.tok
			}
		}
		// Not #\, fall through to symbol/number handling
		return l.lexSym()
	default:
		// Check if this character is a readtable macro character with a Lisp-level function
		if currentReadtable != nil {
			if entry, ok := currentReadtable.macroFns[rune(ch)]; ok && entry != nil && entry.lispFn != nil {
				l.tok = Tok{typ: TMacro, ch: rune(ch), pos: p}
				return l.tok
			}
		}
		if unicode.IsDigit(ch) {
			// Check if this is a symbol like 1+ or 1- (CL arithmetic functions)
			if l.pos < len(l.src) && (l.src[l.pos] == '+' || l.src[l.pos] == '-') {
				// Peek ahead: if the char after +/- is a delimiter or EOF, it's a symbol
				nextPos := l.pos + 1
				if nextPos >= len(l.src) || l.src[nextPos] == ' ' || l.src[nextPos] == '\t' || l.src[nextPos] == ')' || l.src[nextPos] == '(' || l.src[nextPos] == '\n' || l.src[nextPos] == '\r' {
					return l.lexSymFrom(p)
				}
			}
			return l.lexNum()
		}
		if (ch == '-' || ch == '+') && l.pos < len(l.src) && unicode.IsDigit(l.src[l.pos]) {
			return l.lexNum()
		}
		return l.lexSym()
	}
	return l.tok
}

func (l *Lexer) skipWS() {
	for l.pos < len(l.src) && (l.src[l.pos] == ' ' || l.src[l.pos] == '\t' || l.src[l.pos] == '\n' || l.src[l.pos] == '\r') {
		l.pos++
	}
}

func (l *Lexer) lexStr() Tok {
	start := l.pos
	var b strings.Builder
	for l.pos < len(l.src) {
		ch := l.src[l.pos]
		l.pos++
		if ch == '"' {
			l.tok = Tok{typ: TStr, lit: b.String()}
			return l.tok
		}
		if ch == '\\' && l.pos < len(l.src) {
			switch l.src[l.pos] {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case '"':
				b.WriteByte('"')
			case '\\':
				b.WriteByte('\\')
			default:
				b.WriteRune(l.src[l.pos])
			}
			l.pos++
			continue
		}
		b.WriteRune(ch)
	}
	l.err = fmt.Errorf("unclosed string at %d", start)
	l.tok = Tok{typ: TErr, lit: "unclosed string"}
	return l.tok
}

func (l *Lexer) lexBarSym() Tok {
	start := l.pos
	var b strings.Builder
	for l.pos < len(l.src) {
		ch := l.src[l.pos]
		l.pos++
		if ch == '|' {
			l.tok = Tok{typ: TSym, lit: b.String(), pos: start, isBar: true}
			return l.tok
		}
		if ch == '\\' && l.pos < len(l.src) {
			ch2 := l.src[l.pos]
			if ch2 == '|' || ch2 == '\\' {
				b.WriteRune(ch2)
				l.pos++
				continue
			}
			// Backslash before other character: just include both literally
			b.WriteRune(ch)
			b.WriteRune(ch2)
			l.pos++
			continue
		}
		b.WriteRune(ch)
	}
	// Unterminated bar-escaped symbol — treat as empty
	l.tok = Tok{typ: TSym, lit: b.String(), pos: start, isBar: true}
	return l.tok
}

func (l *Lexer) lexNum() Tok {
	start := l.pos - 1
	if l.pos > 0 && (l.src[l.pos-1] == '-' || l.src[l.pos-1] == '+') {
		if l.pos >= len(l.src) || !unicode.IsDigit(l.src[l.pos]) {
			return l.lexSymFrom(start)
		}
	}
	// Read integer part (digits only)
	for l.pos < len(l.src) && unicode.IsDigit(l.src[l.pos]) {
		l.pos++
	}
	// Check for fraction syntax: integer/integer
	if l.pos < len(l.src) && l.src[l.pos] == '/' && l.pos+1 < len(l.src) && unicode.IsDigit(l.src[l.pos+1]) {
		l.pos++ // consume '/'
		denStart := l.pos
		for l.pos < len(l.src) && unicode.IsDigit(l.src[l.pos]) {
			l.pos++
		}
		numStr := string(l.src[start : denStart-1]) // numerator (before '/')
		denStr := string(l.src[denStart:l.pos])     // denominator (after '/')
		n, err1 := strconv.ParseInt(numStr, 10, 64)
		d, err2 := strconv.ParseInt(denStr, 10, 64)
		if err1 == nil && err2 == nil && d != 0 {
			l.tok = Tok{typ: TNum, irat: n, iden: d, pos: start}
			return l.tok
		}
		// Try big rational
		bn := new(big.Int)
		bd := new(big.Int)
		_, ok1 := bn.SetString(numStr, 10)
		_, ok2 := bd.SetString(denStr, 10)
		if ok1 && ok2 && bd.Sign() != 0 {
			br := new(big.Rat).SetFrac(bn, bd)
			f, _ := new(big.Float).SetRat(br).Float64()
			l.tok = Tok{typ: TNum, num: f, pos: start}
			return l.tok
		}
		// Invalid rational, fall through to symbol
		return l.lexSymFrom(start)
	}
	// Check for decimal point
	hasDecimal := false
	if l.pos < len(l.src) && l.src[l.pos] == '.' {
		hasDecimal = true
		l.pos++
		for l.pos < len(l.src) && unicode.IsDigit(l.src[l.pos]) {
			l.pos++
		}
	}
	// Check for scientific notation (exponent marker: e, f, d, s, l - all case variants)
	hasExponent := false
	if l.pos < len(l.src) {
		switch l.src[l.pos] {
		case 'e', 'E', 'f', 'F', 'd', 'D', 's', 'S', 'l', 'L':
			hasExponent = true
			l.pos++
			if l.pos < len(l.src) && (l.src[l.pos] == '-' || l.src[l.pos] == '+') {
				l.pos++
			}
			for l.pos < len(l.src) && unicode.IsDigit(l.src[l.pos]) {
				l.pos++
			}
		}
	}
	numStr := string(l.src[start:l.pos])
	// Normalize CL float exponent markers (d, f, s, l) to Go's 'e' for ParseFloat
	if hasExponent {
		var b strings.Builder
		foundExponent := false
		for _, ch := range numStr {
			if !foundExponent && (ch == 'd' || ch == 'D' || ch == 'f' || ch == 'F' || ch == 's' || ch == 'S' || ch == 'l' || ch == 'L') {
				b.WriteRune('e')
				foundExponent = true
			} else {
				b.WriteRune(ch)
			}
		}
		numStr = b.String()
	}
	// If pure integer (no decimal, no exponent), try big.Int first
	if !hasDecimal && !hasExponent {
		n, err := strconv.ParseInt(numStr, 10, 64)
		if err == nil {
			// Only use float64 if the integer fits in float64 mantissa exactly
			// (53 bits, ±2^53 = ±9007199254740992)
			if n >= -9007199254740992 && n <= 9007199254740992 {
				l.tok = Tok{typ: TNum, num: float64(n), pos: start}
				return l.tok
			}
			// Fits in int64 but not in float64 mantissa — use big.Int
			bi := big.NewInt(n)
			l.tok = Tok{typ: TNum, bigInt: bi, pos: start}
			return l.tok
		}
		// Overflow: try big.Int
		bi := new(big.Int)
		if _, ok := bi.SetString(numStr, 10); ok {
			l.tok = Tok{typ: TNum, bigInt: bi, pos: start}
			return l.tok
		}
	}
	f, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return l.lexSymFrom(start)
	}
	l.tok = Tok{typ: TNum, num: f, pos: start, isFlt: hasDecimal || hasExponent}
	return l.tok
}

// Examples: #\a, #\space, #\newline, #\tab, #\return, #\backspace, #\rubout, #\page
func (l *Lexer) lexChar(pos int) Tok {
	// Named characters mapping
	namedChars := map[string]rune{
		"space":     ' ',
		"newline":   '\n',
		"tab":       '\t',
		"return":    '\r',
		"backspace": '\x08',
		"rubout":    '\x7f',
		"page":      '\f',
		"null":      '\x00',
		"bell":      '\x07',
		"escape":    '\x1b',
		// Additional standard ASCII control character names
		"soh":       '\x01',
		"stx":       '\x02',
		"etx":       '\x03',
		"eot":       '\x04',
		"enq":       '\x05',
		"ack":       '\x06',
		"nak":       '\x15',
		"syn":       '\x16',
		"etb":       '\x17',
		"can":       '\x18',
		"em":        '\x19',
		"sub":       '\x1a',
		"fs":        '\x1c',
		"gs":        '\x1d',
		"rs":        '\x1e',
		"us":        '\x1f',
		"del":       '\x7f',
		// Common abbreviations
		"xoff":      '\x13',
		"xon":       '\x11',
	}

	// Skip the \ after #
	l.pos++ // consume \ after #

	// Read characters until whitespace or delimiter
	nameStart := l.pos
	for l.pos < len(l.src) && !unicode.IsSpace(l.src[l.pos]) && !strings.ContainsRune("()\"';'`,", l.src[l.pos]) {
		l.pos++
	}

	name := string(l.src[nameStart:l.pos])

	if len(name) == 0 {
		// #\ followed by whitespace - read one char
		if l.pos < len(l.src) {
			ch := l.src[l.pos]
			l.pos++
			l.tok = Tok{typ: TChar, ch: ch, pos: pos}
			return l.tok
		}
	}

	// Try named character (case-insensitive lookup)
	if ch, ok := namedChars[strings.ToLower(name)]; ok {
		l.tok = Tok{typ: TChar, ch: ch, pos: pos}
		return l.tok
	}

	// Single character (preserve original case)
	if len(name) == 1 {
		l.tok = Tok{typ: TChar, ch: rune(name[0]), pos: pos}
		return l.tok
	}

	// Multi-character that is not a named char - treat as symbol
	l.tok = Tok{typ: TSym, lit: "#" + "\\" + name, pos: pos}
	return l.tok
}

func (l *Lexer) lexSym() Tok {
	start := l.pos - 1
	return l.lexSymFrom(start)
}

func (l *Lexer) lexSymFrom(start int) Tok {
	for l.pos < len(l.src) && !unicode.IsSpace(l.src[l.pos]) && !strings.ContainsRune("()\";'`,", l.src[l.pos]) {
		// Check if this character is a readtable macro character with a Lisp-level function
		if currentReadtable != nil {
			if entry, ok := currentReadtable.macroFns[l.src[l.pos]]; ok && entry != nil && entry.lispFn != nil {
				break
			}
		}
		l.pos++
	}
	// If the first character of the symbol has a Lisp-level macro function,
	// emit it as a TMacro token instead of a symbol.
	if currentReadtable != nil && l.pos > start {
		firstCh := l.src[start]
		if entry, ok := currentReadtable.macroFns[firstCh]; ok && entry != nil && entry.lispFn != nil {
			l.tok = Tok{typ: TMacro, ch: firstCh, pos: start}
			l.pos = start + 1 // consume just the macro character
			return l.tok
		}
	}
	s := string(l.src[start:l.pos])
	switch s {
	case "#t":
		l.tok = Tok{typ: TTrue}
	case "#f":
		l.tok = Tok{typ: TFalse}
	case "#c":
		// #c(real imag) complex number syntax
		// skip whitespace
		for l.pos < len(l.src) && (l.src[l.pos] == ' ' || l.src[l.pos] == '\t' || l.src[l.pos] == '\n') {
			l.pos++
		}
		if l.pos < len(l.src) && l.src[l.pos] == '(' {
			l.pos++ // consume '('
			// read real part
			for l.pos < len(l.src) && (l.src[l.pos] == ' ' || l.src[l.pos] == '\t' || l.src[l.pos] == '\n') {
				l.pos++
			}
			realStart := l.pos
			for l.pos < len(l.src) && !unicode.IsSpace(l.src[l.pos]) && l.src[l.pos] != ')' {
				l.pos++
			}
			realVal, _ := strconv.ParseFloat(string(l.src[realStart:l.pos]), 64)
			// read imag part
			for l.pos < len(l.src) && (l.src[l.pos] == ' ' || l.src[l.pos] == '\t' || l.src[l.pos] == '\n') {
				l.pos++
			}
			imagStart := l.pos
			for l.pos < len(l.src) && !unicode.IsSpace(l.src[l.pos]) && l.src[l.pos] != ')' {
				l.pos++
			}
			imagVal, _ := strconv.ParseFloat(string(l.src[imagStart:l.pos]), 64)
			// consume ')'
			for l.pos < len(l.src) && (l.src[l.pos] == ' ' || l.src[l.pos] == '\t') {
				l.pos++
			}
			if l.pos < len(l.src) && l.src[l.pos] == ')' {
				l.pos++
			}
			l.tok = Tok{typ: TNum, num: realVal, imag: imagVal, pos: start}
			return l.tok
		}
		l.tok = Tok{typ: TSym, lit: s}
	default:
		// Check for radix reader macros: #b, #o, #x
		if len(s) >= 2 && s[0] == '#' {
			switch s[1] {
			case 'b', 'B':
				// Binary: #b1010 → 10
				if len(s) > 2 {
					n, err := strconv.ParseInt(s[2:], 2, 64)
					if err == nil {
						l.tok = Tok{typ: TNum, num: float64(n), pos: start}
						return l.tok
					}
				}
			case 'o', 'O':
				// Octal: #o777 → 511
				if len(s) > 2 {
					n, err := strconv.ParseInt(s[2:], 8, 64)
					if err == nil {
						l.tok = Tok{typ: TNum, num: float64(n), pos: start}
						return l.tok
					}
				}
			case 'x', 'X':
				// Hex: #xFF → 255
				if len(s) > 2 {
					n, err := strconv.ParseInt(s[2:], 16, 64)
					if err == nil {
						l.tok = Tok{typ: TNum, num: float64(n), pos: start}
						return l.tok
					}
				}
			}
		}
		l.tok = Tok{typ: TSym, lit: s}
	}
	return l.tok
}

// -------- Parser --------
type Parser struct {
	l         *Lexer
	tok       Tok
	ptoks     []Tok
	pi        int
	readtable *Readtable
	env       *Env // for calling Lisp-level macro functions
}

func (p *Parser) advance() {
	if p.pi < len(p.ptoks) {
		p.tok = p.ptoks[p.pi]
		p.pi++
	} else {
		p.l.next()
		p.tok = p.l.tok
		if p.l.err != nil {
			p.tok = Tok{typ: TErr, lit: p.l.err.Error()}
		}
		p.ptoks = append(p.ptoks, p.tok)
		p.pi = len(p.ptoks)
	}
}

func (p *Parser) read() (*Value, error) {
	switch p.tok.typ {
	case TLParen:
		return p.readList()
	case TNum:
		if p.tok.iden != 0 {
			v := vrat(p.tok.irat, p.tok.iden)
			if v == nil {
				return nil, fmt.Errorf("invalid rational: division by zero")
			}
			p.advance()
			return v, nil
		}
		if p.tok.imag != 0 {
			v := vcomplex(p.tok.num, p.tok.imag)
			p.advance()
			return v, nil
		}
		if p.tok.bigInt != nil {
			v := vbigint(p.tok.bigInt)
			p.advance()
			return v, nil
		}
		v := vnum(p.tok.num)
		if p.tok.isFlt {
			v = vfloat(p.tok.num)
		}
		p.advance()
		return v, nil
	case TStr:
		v := vstr(p.tok.lit)
		p.advance()
		return v, nil
	case TSym:
		lit := p.tok.lit
		if len(lit) >= 2 && lit[0] == '#' && (lit[1] == '+' || lit[1] == '-') {
			include := lit[1] == '+'
			var featureSpec *Value
			p.advance()
			if len(lit) == 2 {
				// #+feature form (space separated) — read feature spec
				featureSpec, _ = p.readExpr()
			} else {
				// #+feature (no space) — extract feature name from lit
				featName := lit[2:]
				if featName[0] != ':' {
					featName = ":" + featName
				}
				featureSpec = internOrVsym(featName)
			}
			// Read the conditional form
			form, err := p.readExpr()
			if err != nil {
				return nil, err
			}
			// Check features
			satisfied := featureSatisfied(featureSpec)
			if include == satisfied {
				return form, nil
			}
			// Feature not satisfied — skip and read next form
			return p.readExpr()
		}
		if p.tok.isBar {
			// Bar-escaped symbol: preserve case and handle package syntax
			name := p.tok.lit
			if idx := strings.Index(name, ":"); idx >= 0 {
				pkgName := name[:idx]
				symName := name[idx+1:]
				if pkg := findPackage(pkgName); pkg != nil {
					if sym, ok := pkg.symbols[symName]; ok {
						v := sym
						p.advance()
						return v, nil
					}
					v := internSymbol(symName, pkg)
					p.advance()
					return v, nil
				}
			}
			if strings.HasPrefix(name, ":") {
				kPkg := findPackage("KEYWORD")
				if kPkg != nil {
					if sym, ok := kPkg.symbols[name]; ok {
						v := sym
						p.advance()
						return v, nil
					}
					sym := gcv()
					sym.typ = VSym
					sym.str = name
					kPkg.symbols[name] = sym
					p.advance()
					return sym, nil
				}
			}
			v := vsym(name)
			p.advance()
			return v, nil
		}
		v := internOrVsym(p.tok.lit)
		p.advance()
		return v, nil
	case TTrue:
		p.advance()
		return vbool(true), nil
	case TFalse:
		p.advance()
		return vbool(false), nil
	case TChar:
		v := vchar(p.tok.ch)
		p.advance()
		return v, nil
	case TErr:
		return nil, fmt.Errorf("lex error: %s", p.tok.lit)
	case TEOF:
		return nil, fmt.Errorf("unexpected EOF")
	case TRParen:
		// Closing paren encountered — return nil as a signal to caller.
		// DO NOT advance; caller (e.g. readList) should check p.tok.typ.
		// Note: readList() now checks for TRParen before calling readExpr(),
		// so this is only hit when called from feature conditionals (#+/ #-)
		// or vector parsing where the closing paren terminates the parent list.
		return nil, nil
	case TDot:
		// Dot encountered outside a list context — this is a syntax error.
		// readList() checks for TDot before calling readExpr().
		return nil, fmt.Errorf("unexpected token: .")
	default:
		return nil, fmt.Errorf("unexpected token: %s", tokName(p.tok.typ))
	}
}

// isCloseParenMacro returns true if the current token is a TMacro whose
// registered lispFn is the closeParenSentinel, meaning it acts as ')'.
func (p *Parser) isCloseParenMacro() bool {
	if p.tok.typ != TMacro {
		return false
	}
	rt := p.readtable
	if rt == nil {
		rt = currentReadtable
	}
	entry, ok := rt.macroFns[p.tok.ch]
	if !ok || entry == nil || entry.lispFn == nil {
		return false
	}
	return entry.lispFn == closeParenSentinel
}

func (p *Parser) readList() (*Value, error) {
	p.advance() // skip (
	var head, tail *Value
	for p.tok.typ != TRParen && p.tok.typ != TEOF && !p.isCloseParenMacro() {
		if p.tok.typ == TDot {
			p.advance()
			v, err := p.readExpr()
			if err != nil {
				return nil, err
			}
			if p.tok.typ != TRParen && !p.isCloseParenMacro() {
				return nil, fmt.Errorf("expected ) after dot")
			}
			if tail == nil {
				return nil, fmt.Errorf("dot without preceding element")
			}
			tail.cdr = v
			p.advance()
			return head, nil
		}
		v, err := p.readExpr()
		if err != nil {
			return nil, err
		}
		if v == nil {
			// readExpr returned Go nil — this means it hit TRParen or TDot.
			// These are terminators, not values. Stop reading.
			break
		}
		pair := cons(v, vnil())
		if head == nil {
			head = pair
			tail = pair
		} else {
			tail.cdr = pair
			tail = pair
		}
	}
	if p.tok.typ == TEOF {
		return nil, fmt.Errorf("unclosed list")
	}
	p.advance() // skip ) or close-paren macro
	if head == nil {
		return vnil(), nil
	}
	return head, nil
}


func tokName(t TokType) string {
	switch t {
	case TLParen:
		return "("
	case TRParen:
		return ")"
	case TDot:
		return "."
	case TNum:
		return "number"
	case TStr:
		return "string"
	case TSym:
		return "symbol"
	case TChar:
		return "character"
	case TMacro:
		return "macro-char"
	case TEOF:
		return "EOF"
	default:
		return fmt.Sprintf("token(%d)", t)
	}
}

// multi-read: read one complete expression, handling quotes as reader syntax
func parseExpr(s string) (*Value, error) {
	v, _, err := parseExprWithPos(s)
	return v, err
}

// parseExprWithPos parses a single expression and returns the value and position after it.
func parseExprWithPos(s string) (*Value, int, error) {
	l := lex(s)
	p := &Parser{l: l, ptoks: make([]Tok, 0, 64), readtable: currentReadtable, env: globalEnv}
	p.advance()
	v, err := p.readExpr()
	if err != nil {
		return nil, 0, err
	}
	// After readExpr(), l.pos may have been advanced past whitespace by the last next() call.
	// l.prevEndPos is set at the start of each next() call to l.pos before skipWS(),
	// so it captures the position right after the previous token.
	// For compound expressions (lists etc.), multiple next() calls were made,
	// and prevEndPos captures the position right after the expression.
	// For simple expressions (numbers, symbols), only one next() was called,
	// so prevEndPos is still 0 and l.pos is already correct.
	if l.prevEndPos == 0 && l.pos > 0 {
		return v, l.pos, nil
	}
	return v, l.prevEndPos, nil
}

func (p *Parser) readExpr() (*Value, error) {
	switch p.tok.typ {
	case TQuote:
		p.advance()
		v, err := p.readExpr()
		if err != nil {
			return nil, err
		}
		return list(vsym("quote"), v), nil
	case TFuncQuote:
		p.advance()
		v, err := p.readExpr()
		if err != nil {
			return nil, err
		}
		return list(vsym("function"), v), nil
	case TSharpDot:
		p.advance()
		v, err := p.readExpr()
		if err != nil {
			return nil, err
		}
		result, err := Eval(v, globalEnv)
		if err != nil {
			return nil, fmt.Errorf("#. read-time evaluation error: %v", err)
		}
		return result, nil
	case TSharpMacro:
		// User-registered # dispatch macro character
		result, err := p.invokeDispatchMacro()
		if err != nil {
			return nil, err
		}
		return result, nil
	case TPathname:
		pathStr := p.tok.lit
		p.advance()
		return vpathname(parsePathnameString(pathStr)), nil
	case TQq:
		p.advance()
		v, err := p.readExpr()
		if err != nil {
			return nil, err
		}
		return list(vsym("QUASIQUOTE"), v), nil
	case TUnq:
		p.advance()
		v, err := p.readExpr()
		if err != nil {
			return nil, err
		}
		return list(vsym("UNQUOTE"), v), nil
	case TUnqS:
		p.advance()
		v, err := p.readExpr()
		if err != nil {
			return nil, err
		}
		return list(vsym("UNQUOTE-SPLICING"), v), nil
	case TComplex:
		// #C(real imag) - parse complex number literal
		inner := p.tok.lit
		cparts := strings.Fields(inner)
		if len(cparts) >= 2 {
			realStr := cparts[0]
			imagStr := cparts[1]
			realVal, err1 := parseFloatStr(realStr)
			imagVal, err2 := parseFloatStr(imagStr)
			if err1 == nil && err2 == nil {
				p.advance()
				hasFloat := strings.ContainsAny(realStr, ".eEdDfFsSlL") || strings.ContainsAny(imagStr, ".eEdDfFsSlL")
				v := vcomplex(realVal, imagVal)
				// Mark as float if either part was written as a float literal
				// (covers both VComplex and simplified VNum results)
				if hasFloat {
					v.isFloat = true
				}
				return v, nil
			}
		}
		return nil, fmt.Errorf("invalid complex number literal: #C(%s)", inner)
	case TVector:
		// #(elem1 elem2 ...) - parse vector literal
		inner := p.tok.lit
		// Check for bit-vector sentinel (#*1010 produces this)
		if inner == "/*bitvec*/" && p.l.bitVec != nil {
			bv := p.l.bitVec
			p.advance()
			return bv, nil
		}
		p.advance()
		// Parse inner contents as a list of expressions
		innerParser := &Parser{l: lex(inner), ptoks: make([]Tok, 0, 64), readtable: currentReadtable, env: globalEnv}
		innerParser.advance()
		var elements []*Value
		for innerParser.tok.typ != TEOF {
			elem, err := innerParser.readExpr()
			if err != nil {
				return nil, fmt.Errorf("vector literal parse error: %v", err)
			}
			elements = append(elements, elem)
		}
		arr := &LispArray{
			dims:     []int{len(elements)},
			elements: make([]*Value, len(elements)),
			fillPtr:  -1,
		}
		copy(arr.elements, elements)
		v := gcv()
		v.typ = VArray
		v.array = arr
		return v, nil
	case TMacro:
		// Readtable macro character: call the registered macro function
		result, err := p.invokeMacro(p.tok.ch)
		p.advance()
		return result, err
	default:
		return p.read()
	}
}

// invokeMacro calls a readtable macro function with the macro character.
func (p *Parser) invokeMacro(ch rune) (*Value, error) {
	rt := p.readtable
	if rt == nil {
		rt = currentReadtable
	}
	entry, ok := rt.macroFns[ch]
	if !ok || entry == nil || entry.lispFn == nil {
		// No Lisp-level macro function registered
		return nil, fmt.Errorf("invokeMacro: no macro function for %q", string(ch))
	}
	// Close-paren sentinel: signal error (like unmatched close paren)
	if entry.lispFn == closeParenSentinel {
		return nil, fmt.Errorf("unmatched close parenthesis")
	}
	// Call the macro function by constructing (fn ch) and evaluating it.
	chVal := &Value{typ: VChar, ch: ch}
	callForm := cons(entry.lispFn, cons(chVal, vnil()))
	env := p.env
	if env == nil {
		env = globalEnv
	}
	result, err := Eval(callForm, env)
	if err != nil {
		return nil, fmt.Errorf("macro function error: %v", err)
	}
	return result, nil
}

// invokeDispatchMacro calls a user-registered # dispatch macro function.
// The function is called as: (function stream sub-char num-params)
func (p *Parser) invokeDispatchMacro() (*Value, error) {
	rt := p.readtable
	if rt == nil {
		rt = currentReadtable
	}
	subCh := p.tok.ch
	entry, ok := rt.dispFns[subCh]
	if !ok || entry == nil || entry.lispFn == nil {
		return nil, fmt.Errorf("invokeDispatchMacro: no dispatch function for #%c", subCh)
	}
	// Build call: (function stream sub-char)
	// stream: pass as nil for now (reader stream)
	// sub-char: the character after #
	stream := vnil()
	chVal := &Value{typ: VChar, ch: subCh}
	callForm := list(entry.lispFn, stream, chVal)
	env := p.env
	if env == nil {
		env = globalEnv
	}
	result, err := Eval(callForm, env)
	if err != nil {
		return nil, fmt.Errorf("# dispatch macro error: %v", err)
	}
	p.advance()
	return result, nil
}

func parseAll(s string) (*Value, error) {
	l := lex(s)
	p := &Parser{l: l, ptoks: make([]Tok, 0, 64), readtable: currentReadtable, env: globalEnv}
	p.advance()
	var result *Value = vnil()
	for p.tok.typ != TEOF {
		v, err := p.readExpr()
		if err != nil {
			return nil, err
		}
		result = cons(v, result)
	}
	// reverse
	var rev *Value = vnil()
	for !isNil(result) {
		rev = cons(result.car, rev)
		result = result.cdr
	}
	return rev, nil
}

// parseExprList parses a string of zero or more expressions and returns them
// as a proper Lisp list. Used by read-delimited-list.
func parseExprList(s string) (*Value, error) {
	l := lex(s)
	p := &Parser{l: l, ptoks: make([]Tok, 0, 64), readtable: currentReadtable, env: globalEnv}
	p.advance()
	// Collect all parsed expressions in order
	var forms []*Value
	for p.tok.typ != TEOF {
		v, err := p.readExpr()
		if err != nil {
			return nil, err
		}
		if v != nil {
			forms = append(forms, v)
		}
	}
	// Build a proper Lisp list from the forms (in correct order)
	var head, tail *Value = nil, nil
	for _, form := range forms {
		pair := cons(form, vnil())
		if head == nil {
			head = pair
			tail = pair
		} else {
			tail.cdr = pair
			tail = pair
		}
	}
	return head, nil
}

// -------- Evaluator --------

func EvalString(s string, env *Env) (*Value, error) {
	// Use lazy parsing: parse and evaluate one expression at a time.
	// This ensures that set-macro-character calls take effect for
	// subsequent forms in the same source string.
	l := lex(s)
	p := &Parser{l: l, ptoks: make([]Tok, 0, 64), readtable: currentReadtable, env: globalEnv}
	p.advance()
	var result *Value = vnil()
	for p.tok.typ != TEOF {
		// Update readtable reference before each expression
		p.readtable = currentReadtable
		p.env = globalEnv
		v, err := p.readExpr()
		if err != nil {
			return nil, err
		}
		result, err = Eval(v, env)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func Eval(v *Value, env *Env) (result *Value, err error) {
	if v == nil {
		return vnil(), nil
	}
	evalDepth++
	if evalDepth > maxEvalDepth {
		evalDepth--
		return nil, fmt.Errorf("eval: maximum recursion depth (%d) exceeded — possible infinite recursion", maxEvalDepth)
	}
	defer func() { evalDepth-- }()
evalLoop:
	for {
		switch v.typ {
		case VNum, VStr, VBool, VNil, VPrim, VFunc, VRat, VComplex, VBigInt, VChar, VInstance, VClass, VMultiVal, VPathname, VPackage, VReadtable, VArray:
			return v, nil
		case VSym:
			// Keywords are self-evaluating (symbols in KEYWORD package)
			kPkg := findPackage("KEYWORD")
			if kPkg != nil {
				if _, ok := kPkg.symbols[v.str]; ok {
					return v, nil
				}
			}
			val, e := env.Get(v.str)
			if e != nil {
				return nil, e
			}
			if val == nil {
				return vnil(), nil
			}
			if val.typ == VSymMacro {
				v = val.car
				continue
			}
			return val, nil
		case VPair:
			if v.car == nil || v.car.typ != VSym {
				// Check if the car is quasiquote/backquote — expand and return
		if v.car != nil && v.car.typ == VPair {
			carCar := v.car.car
			if carCar != nil && carCar.typ == VSym {
				carSym := carCar.str
				if carSym == "QUASIQUOTE" || carSym == "BACKQUOTE" {
					expanded, qerr := evalQuasiquote(v.car, 0, env)
					if qerr != nil {
						return nil, qerr
					}
					// When eval receives ((quasiquote ...)), treat it as just
					// the quasiquote form — return the expanded data directly
					return expanded, nil
				}
			}
		}

		// application
				fn, e := Eval(v.car, env)
				if e != nil {
					return nil, e
				}
				r, e := Apply(fn, v.cdr, env)
				if e != nil {
					if tc, ok := e.(*tailCall); ok {
						v = tc.form
						env = tc.env
						continue evalLoop
					}
					return nil, e
				}
				return r, nil
			}
			// CL requires symbols to be case-insensitive: upcase for special form dispatch
			opName := strings.ToUpper(v.car.str)
			switch opName {
			case "FUNCALL":
				// funcall as special form to pass lexical env to callFnOnSeq
				if v.cdr == nil || v.cdr.typ != VPair {
					return nil, fmt.Errorf("funcall: malformed arguments")
				}
				fn, e := Eval(v.cdr.car, env)
				if e != nil {
					return nil, e
				}
				fn = primaryValue(fn)
				if fn.typ == VSym {
					resolved, err := env.Get(fn.str)
					if err == nil {
						fn = resolved
					}
				}
				callArgs := []*Value{}
				for cp := v.cdr.cdr; !isNil(cp) && cp.typ == VPair; cp = cp.cdr {
					arg, e := Eval(cp.car, env)
					if e != nil {
						return nil, e
					}
					callArgs = append(callArgs, arg)
				}
				r, e := callFnOnSeq(fn, callArgs, env)
				if e != nil {
					if tc, ok := e.(*tailCall); ok {
						v = tc.form
						env = tc.env
						continue evalLoop
					}
					return nil, e
				}
				return r, nil
			case "QUOTE":
				if v.cdr == nil || v.cdr.typ != VPair {
					return nil, fmt.Errorf("quote: malformed form")
				}
				return v.cdr.car, nil
			case "FUNCTION":
				if v.cdr == nil || v.cdr.typ != VPair {
					return nil, fmt.Errorf("function: malformed form")
				}
				arg := v.cdr.car
				if arg.typ == VSym {
					val, err := env.Get(arg.str)
					if err != nil {
						return nil, fmt.Errorf("function: undefined: %s", arg.str)
					}
					return val, nil
				}
				if arg.typ == VPair && arg.car != nil && arg.car.typ == VSym && arg.car.str == "LAMBDA" {
					return Eval(arg, env)
				}
				return Eval(arg, env)
			case "QUASIQUOTE":
				if v.cdr == nil || v.cdr.typ != VPair {
					return nil, fmt.Errorf("quasiquote: malformed form")
				}
				res, e := evalQuasiquote(v.cdr.car, 0, env)
				if e != nil {
					return nil, e
				}
				// Handle double backquote: when result is ((quasiquote X)),
				// return (quasiquote X) so the second eval processes it normally.
				if isPair(res) && isNil(res.cdr) && isPair(res.car) &&
					res.car.car != nil && res.car.car.typ == VSym &&
					res.car.car.str == "QUASIQUOTE" {
					return res.car, nil
				}
				return res, nil
			case "IF":
				if v.cdr == nil || v.cdr.typ != VPair {
					return nil, fmt.Errorf("if: malformed form")
				}
				cond, e := Eval(v.cdr.car, env)
				if e != nil {
					return nil, e
				}
				cond = primaryValue(cond)
				alt := v.cdr.cdr
				if isTruthy(cond) {
					if alt.typ != VPair {
						return nil, fmt.Errorf("if: malformed form")
					}
					v = alt.car
				} else if !isNil(alt) && alt.typ == VPair && !isNil(alt.cdr) {
					v = alt.cdr.car
				} else {
					return vnil(), nil
				}
				continue
			case "BEGIN":
				exprs := v.cdr
				if isNil(exprs) {
					return vnil(), nil
				}
				for exprs.typ == VPair && !isNil(exprs.cdr) {
					_, e := Eval(exprs.car, env)
					if e != nil {
						return nil, e
					}
					exprs = exprs.cdr
				}
				v = exprs.car
				continue
			case "LAMBDA":
				if v.cdr == nil || v.cdr.typ != VPair {
					return nil, fmt.Errorf("lambda: need lambda list")
				}
				fn := gcv()
				fn.typ = VFunc
				params, rest, optDefaults, keySpecs, e := parseParams(v.cdr.car)
				if e != nil {
					return nil, e
				}
				fn.params = params
				fn.rest = rest
				fn.optDefaults = optDefaults
				fn.keySpecs = keySpecs
				fn.body = v.cdr.cdr
				fn.env = env
				return fn, nil
			case "PROGN":
				exprs := v.cdr
				if isNil(exprs) {
					return vnil(), nil
				}
				for exprs.typ == VPair && !isNil(exprs.cdr) {
					_, e := Eval(exprs.car, env)
					if e != nil {
						return nil, e
					}
					exprs = exprs.cdr
				}
				v = exprs.car
				continue
			case "DECLARE":
				// Declarations are advisory, ignored in interpreter
				return vnil(), nil
			case "THE":
				// (the type-specifier form) — evaluate form, ignore type check
				if v.cdr == nil || v.cdr.typ != VPair || v.cdr.cdr == nil || v.cdr.cdr.typ != VPair {
					return nil, fmt.Errorf("the: need type and form")
				}
				v = v.cdr.cdr.car
				continue
			case "LOCALLY":
				// (locally declaration... body...) — skip declarations, eval body
				body := v.cdr
				for !isNil(body) && body.car != nil && body.car.typ == VPair && body.car.car != nil && body.car.car.typ == VSym && body.car.car.str == "DECLARE" {
					body = body.cdr
				}
				if isNil(body) {
					return vnil(), nil
				}
				for body.typ == VPair && !isNil(body.cdr) {
					_, e := Eval(body.car, env)
					if e != nil {
						return nil, e
					}
					body = body.cdr
				}
				if body.typ != VPair {
					return vnil(), nil
				}
				v = body.car
				continue
			case "PROCLAIM":
				// (proclaim decl-spec...) — advisory, ignored
				return vnil(), nil
			case "DECLAIM":
				// (declaim decl-spec...) — advisory, ignored
				return vnil(), nil
			case "PROGV":
				// (progv symbols-list values-list body...)
				if v.cdr == nil || v.cdr.typ != VPair {
					return nil, fmt.Errorf("progv: malformed form")
				}
				symsVal, e := Eval(v.cdr.car, env)
				if e != nil {
					return nil, e
				}
				if v.cdr.cdr == nil || v.cdr.cdr.typ != VPair {
					return nil, fmt.Errorf("progv: requires symbols-list and values-list")
				}
				valsVal, e := Eval(v.cdr.cdr.car, env)
				if e != nil {
					return nil, e
				}
				syms := seqToList(symsVal)
				vals := seqToList(valsVal)
				newEnv := NewEnv(env)
				for i, sym := range syms {
					if sym.typ != VSym {
						return nil, fmt.Errorf("progv: expected symbol at position %d", i)
					}
					var val *Value = vnil()
					if i < len(vals) {
						val = vals[i]
					}
					newEnv.Set(sym.str, val)
				}
				body := v.cdr.cdr.cdr
				if !isNil(body) {
					if isPair(body) && isPair(body.cdr) {
						for body.typ == VPair && !isNil(body.cdr) {
							_, err = Eval(body.car, newEnv)
							if err != nil {
								return nil, err
							}
							body = body.cdr
						}
						return Eval(body.car, newEnv)
					}
					if isPair(body) {
						return Eval(body.car, newEnv)
					}
					return Eval(body, newEnv)
				}
				return vnil(), nil

			case "FLET":
				if v.cdr == nil || v.cdr.typ != VPair {
					return nil, fmt.Errorf("flet: malformed form")
				}
				bindings := v.cdr.car
				body := v.cdr.cdr
				newEnv := NewEnv(env)
				for !isNil(bindings) {
					if bindings.typ != VPair {
						return nil, fmt.Errorf("flet: malformed bindings")
					}
					b := bindings.car
					if b.typ != VPair {
						return nil, fmt.Errorf("flet: malformed binding")
					}
					if b.car == nil || b.car.typ != VSym {
						return nil, fmt.Errorf("flet: binding name must be a symbol")
					}
					fname := b.car.str
					fn := gcv()
					fn.typ = VFunc
					fn.name = fname
					if b.cdr == nil || b.cdr.typ != VPair {
						return nil, fmt.Errorf("flet: malformed binding")
					}
					fparams := b.cdr.car
					fbody := b.cdr.cdr
					params, rest, optDefaults, keySpecs, e := parseParams(fparams)
					if e != nil {
						return nil, e
					}
					fn.params = params
					fn.rest = rest
					fn.optDefaults = optDefaults
					fn.keySpecs = keySpecs
					fn.body = fbody
					fn.env = newEnv
					newEnv.Set(fname, fn)
					bindings = bindings.cdr
				}
				env = newEnv
				for body.typ == VPair && !isNil(body.cdr) {
					_, err = Eval(body.car, env)
					if err != nil {
						return nil, err
					}
					body = body.cdr
				}
				if body.typ != VPair {
					return vnil(), nil
				}
				v = body.car
				continue
			case "LABELS":
				if v.cdr == nil || v.cdr.typ != VPair {
					return nil, fmt.Errorf("labels: malformed form")
				}
				bindings := v.cdr.car
				body := v.cdr.cdr
				newEnv := NewEnv(env)
				for !isNil(bindings) {
					if bindings.typ != VPair {
						return nil, fmt.Errorf("labels: malformed bindings")
					}
					if bindings.car == nil || bindings.car.typ != VPair {
						return nil, fmt.Errorf("labels: malformed binding")
					}
					if bindings.car.car == nil || bindings.car.car.typ != VSym {
						return nil, fmt.Errorf("labels: function name must be a symbol")
					}
					fname := bindings.car.car.str
					fn := gcv()
					fn.typ = VFunc
					fn.name = fname
					fn.env = newEnv
					newEnv.Set(fname, fn)
					bindings = bindings.cdr
				}
				bindings = v.cdr.car
				for !isNil(bindings) {
					if bindings.typ != VPair {
						return nil, fmt.Errorf("flet: malformed bindings")
					}
					b := bindings.car
					if b == nil || b.typ != VPair {
						return nil, fmt.Errorf("flet: malformed binding")
					}
					if b.car == nil || b.car.typ != VSym {
						return nil, fmt.Errorf("labels: binding name must be a symbol")
					}
					fname := b.car.str
					fn, _ := newEnv.Get(fname)
					if b.cdr == nil || b.cdr.typ != VPair {
						return nil, fmt.Errorf("labels: malformed binding")
					}
					fparams := b.cdr.car
					fbody := b.cdr.cdr
					params, rest, optDefaults, keySpecs, e := parseParams(fparams)
					if e != nil {
						return nil, e
					}
					fn.params = params
					fn.rest = rest
					fn.optDefaults = optDefaults
					fn.keySpecs = keySpecs
					fn.body = fbody
					bindings = bindings.cdr
				}
				env = newEnv
				for body.typ == VPair && !isNil(body.cdr) {
					_, err = Eval(body.car, env)
					if err != nil {
						return nil, err
					}
					body = body.cdr
				}
				if body.typ != VPair {
					return vnil(), nil
				}
				v = body.car
				continue

			case "DEFINE":
				if v.cdr == nil || v.cdr.typ != VPair {
					return nil, fmt.Errorf("define: malformed form")
				}
				head := v.cdr.car
				var name string
				var val *Value
				if head.typ == VSym {
					name = head.str
					if v.cdr.cdr == nil || isNil(v.cdr.cdr) {
						return nil, fmt.Errorf("define: missing value for %s", name)
					}
					val = v.cdr.cdr.car
				} else if isPair(head) {
					if head.car == nil || head.car.typ != VSym {
						return nil, fmt.Errorf("define: function name must be a symbol")
					}
					name = head.car.str
					params, rest, optDefaults, keySpecs, e := parseParams(head.cdr)
					if e != nil {
						return nil, e
					}
					fn := gcv()
					fn.typ = VFunc
					fn.name = name
					fn.params = params
					fn.rest = rest
					fn.optDefaults = optDefaults
					fn.keySpecs = keySpecs
					fn.body = v.cdr.cdr
					fn.env = env
					val = fn
				} else {
					return nil, fmt.Errorf("bad define syntax")
				}
				ev, e := Eval(val, env)
				if e != nil {
					return nil, e
				}
				env.Set(name, ev)
				return vsym(name), nil
			case "SET!", "SETQ":
				if v.cdr == nil || v.cdr.typ != VPair {
					return nil, fmt.Errorf("set!: malformed form")
				}
				if v.cdr.car == nil || v.cdr.car.typ != VSym {
					return nil, fmt.Errorf("set!: variable must be a symbol")
				}
				name := v.cdr.car.str
				if v.cdr.cdr == nil || v.cdr.cdr.typ != VPair {
					return nil, fmt.Errorf("set!: missing value form")
				}
				valv := v.cdr.cdr.car
				ev, e := Eval(valv, env)
				if e != nil {
					return nil, e
				}
				for scope := env; scope != nil; scope = scope.parent {
					if _, ok := scope.bindings[name]; ok {
						scope.bindings[name] = ev
						return ev, nil
					}
				}
				// Also check globalEnv for already-defined global variables
				if _, err := globalEnv.Get(name); err == nil {
					globalEnv.Set(name, ev)
					return ev, nil
				}
				return nil, fmt.Errorf("set!: undefined %s", name)
			case "DEFVAR":
				if v.cdr == nil || v.cdr.typ != VPair {
					return nil, fmt.Errorf("defvar: malformed form")
				}
				if v.cdr.car == nil || v.cdr.car.typ != VSym {
					return nil, fmt.Errorf("defvar: first argument must be a symbol")
				}
				name := v.cdr.car.str
				if v.cdr.cdr != nil && v.cdr.cdr.typ == VPair && !isNil(v.cdr.cdr) {
					if _, err := globalEnv.Get(name); err != nil {
						ev, e := Eval(v.cdr.cdr.car, env)
						if e != nil {
							return nil, e
						}
						globalEnv.Set(name, ev)
					}
				}
				// defvar without initial value: leave unbound (no binding created)
				return vsym(name), nil
			case "DEFPARAMETER":
				if v.cdr == nil || v.cdr.typ != VPair {
					return nil, fmt.Errorf("defparameter: malformed form")
				}
				if v.cdr.car == nil || v.cdr.car.typ != VSym {
					return nil, fmt.Errorf("defparameter: first argument must be a symbol")
				}
				name := v.cdr.car.str
				if v.cdr.cdr == nil || v.cdr.cdr.typ != VPair {
					return nil, fmt.Errorf("defparameter: requires name and initform")
				}
				ev, e := Eval(v.cdr.cdr.car, env)
				if e != nil {
					return nil, e
				}
				globalEnv.Set(name, ev)
				return vsym(name), nil
			case "DEFCONSTANT":
				// (defconstant name initial-value-form &optional documentation)
				if v.cdr == nil || v.cdr.typ != VPair {
					return nil, fmt.Errorf("defconstant: malformed form")
				}
				if v.cdr.car == nil || v.cdr.car.typ != VSym {
					return nil, fmt.Errorf("defconstant: name must be a symbol")
				}
				name := v.cdr.car.str
				if v.cdr.cdr == nil || v.cdr.cdr.typ != VPair {
					return nil, fmt.Errorf("defconstant: requires name and initform")
				}
				ev, e := Eval(v.cdr.cdr.car, env)
				if e != nil {
					return nil, e
				}
				// In CL, defconstant signals a style-warning if the variable is already bound
				// to a different value. MicroLisp allows redefinition.
				globalEnv.Set(name, ev)
				return vsym(name), nil
			case "DEFMACRO":
				// (defmacro name lambda-list . body)
				// CL standard syntax: name is a symbol, lambda-list follows
				if v.cdr == nil || v.cdr.typ != VPair {
					return nil, fmt.Errorf("defmacro: malformed form")
				}
				head := v.cdr.car
				if head.typ != VSym {
					return nil, fmt.Errorf("defmacro: name must be a symbol")
				}
				name := head.str
				if v.cdr.cdr == nil || isNil(v.cdr.cdr) {
					return nil, fmt.Errorf("defmacro: missing lambda list")
				}
				lambdaList := v.cdr.cdr.car
				macroBody := v.cdr.cdr.cdr
				params, rest, whole, envSym, optDefaults, keySpecs, e := parseMacroParams(lambdaList)
				if e != nil {
					return nil, fmt.Errorf("defmacro: %v", e)
				}
				m := gcv()
				m.typ = VMacro
				m.str = name
				m.params = params
				m.rest = rest
				m.whole = whole
				m.body = macroBody
				m.env = env
				m.optDefaults = optDefaults
				m.keySpecs = keySpecs
				_ = envSym
				env.Set(name, m)
				return vsym(name), nil
			case "DEFUN":
				// (defun name lambda-list . body)
				// Also supports: (defun (setf name) lambda-list . body)
				if v.cdr == nil || v.cdr.typ != VPair {
					return nil, fmt.Errorf("defun: malformed form")
				}
				head := v.cdr.car
				var name string
				if head.typ == VSym {
					name = head.str
				} else if head.typ == VPair {
					// (setf foo) syntax
					if head.car == nil || head.car.typ != VSym || head.car.str != "SETF" {
						return nil, fmt.Errorf("defun: unsupported compound name: %s", ToString(head))
					}
					if isNil(head.cdr) || head.cdr.car.typ != VSym || !isNil(head.cdr.cdr) {
						return nil, fmt.Errorf("defun: unsupported compound name: %s", ToString(head))
					}
					symName := head.cdr.car.str
					name = symName + "-setf"
				} else {
					return nil, fmt.Errorf("defun: name must be a symbol")
				}
				if v.cdr.cdr == nil || isNil(v.cdr.cdr) {
					return nil, fmt.Errorf("defun: missing lambda list")
				}
				lambdaList := v.cdr.cdr.car
				params, rest, optDefaults, keySpecs, e := parseParams(lambdaList)
				if e != nil {
					return nil, fmt.Errorf("defun: %v", e)
				}
				body := v.cdr.cdr.cdr
				fn := gcv()
				fn.typ = VFunc
				fn.name = name
				fn.params = params
				fn.rest = rest
				fn.optDefaults = optDefaults
				fn.keySpecs = keySpecs
				fn.body = body
				fn.env = NewEnv(globalEnv)
				globalEnv.Set(name, fn)
				return vsym(name), nil
			case "LET":
				if v.cdr == nil || v.cdr.typ != VPair {
					return nil, fmt.Errorf("let: malformed form")
				}
				bindings := v.cdr.car
				body := v.cdr.cdr
				names := make([]string, 0, 8)
				vals := make([]*Value, 0, 8)
				for !isNil(bindings) {
					if bindings.typ != VPair {
						return nil, fmt.Errorf("let: malformed bindings")
					}
					b := bindings.car
					if b.typ != VPair {
						return nil, fmt.Errorf("let: malformed binding")
					}
					if b.car == nil || b.car.typ != VSym {
						return nil, fmt.Errorf("let: binding name must be a symbol")
					}
					names = append(names, b.car.str)
					if !isNil(b.cdr) && b.cdr.typ == VPair {
						vals = append(vals, b.cdr.car)
					} else {
						vals = append(vals, vnil())
					}
					bindings = bindings.cdr
				}
				// build ((lambda (names...) body...) vals...)
				params := vnil()
				args := vnil()
				for i := len(names) - 1; i >= 0; i-- {
					params = cons(vsym(names[i]), params)
					args = cons(vals[i], args)
				}
				lam := list(vsym("LAMBDA"), params)
				// append body to lambda
				t := lam.cdr
				for !isNil(body) {
					t.cdr = cons(body.car, vnil())
					t = t.cdr
					body = body.cdr
				}
				v = cons(lam, args)
				continue
			case "LETREC":
				if v.cdr == nil || v.cdr.typ != VPair {
					return nil, fmt.Errorf("letrec: malformed form")
				}
				bindings := v.cdr.car
				body := v.cdr.cdr
				names := make([]string, 0, 8)
				vals := make([]*Value, 0, 8)
				for !isNil(bindings) {
					if bindings.typ != VPair {
						return nil, fmt.Errorf("let: malformed bindings")
					}
					b := bindings.car
					if b.typ != VPair {
						return nil, fmt.Errorf("letrec: malformed binding")
					}
					if b.car == nil || b.car.typ != VSym {
						return nil, fmt.Errorf("letrec: binding name must be a symbol")
					}
					names = append(names, b.car.str)
					if !isNil(b.cdr) && b.cdr.typ == VPair {
						vals = append(vals, b.cdr.car)
					} else {
						vals = append(vals, vnil())
					}
					bindings = bindings.cdr
				}
				newEnv := &Env{parent: env, bindings: make(map[string]*Value)}
				for _, name := range names {
					newEnv.bindings[name] = vbool(false)
				}
				evals := make([]*Value, len(vals))
				for i, val := range vals {
					evald, e := Eval(val, newEnv)
					if e != nil {
						return nil, e
					}
					evals[i] = evald
				}
				for i, name := range names {
					newEnv.bindings[name] = evals[i]
				}
				var result *Value = vnil()
				for !isNil(body) {
					result, err = Eval(body.car, newEnv)
					if err != nil {
						return nil, err
					}
					body = body.cdr
				}
				return result, nil
			case "DEFINE-MACRO":
				if v.cdr == nil || v.cdr.typ != VPair {
					return nil, fmt.Errorf("define-macro: malformed form")
				}
				head := v.cdr.car
				if head.typ == VSym {
					name := head.str
					m := gcv()
					m.typ = VMacro
					m.str = name
					m.params = nil
					m.rest = ""
					m.whole = ""
					m.body = v.cdr.cdr
					m.env = env
					env.Set(name, m)
					return vsym(name), nil
				}
				if head.typ != VPair {
					return nil, fmt.Errorf("define-macro: need a name and lambda list")
				}
				if head.car == nil || head.car.typ != VSym {
					return nil, fmt.Errorf("define-macro: name must be a symbol")
				}
				name := head.car.str
				params, rest, whole, envSym, optDefaults, keySpecs, e := parseMacroParams(head.cdr)
				if e != nil {
					return nil, e
				}
				m := gcv()
				m.typ = VMacro
				m.str = name // store macro name for &whole reconstruction
				m.params = params
				m.rest = rest
				m.whole = whole
				m.body = v.cdr.cdr
				m.env = env
				m.optDefaults = optDefaults
				m.keySpecs = keySpecs
				_ = envSym // stored in envSym field via macro env
				env.Set(name, m)
				return vsym(name), nil
			case "DEFINE-SYMBOL-MACRO":
				// (define-symbol-macro name expansion)
				if v.cdr == nil || v.cdr.typ != VPair {
					return nil, fmt.Errorf("define-symbol-macro: malformed form")
				}
				symName := v.cdr.car
				if symName == nil || symName.typ != VSym {
					return nil, fmt.Errorf("define-symbol-macro: name must be a symbol")
				}
				if v.cdr.cdr == nil || v.cdr.cdr.typ != VPair {
					return nil, fmt.Errorf("define-symbol-macro: missing expansion")
				}
				expansion := v.cdr.cdr.car
				sv := gcv()
				sv.typ = VSymMacro
				sv.car = expansion
				globalEnv.Set(symName.str, sv)
				return symName, nil
			case "DEFINE-COMPILER-MACRO":
				// (define-compiler-macro name (args...) body...)
				// Stores the macro in the compilerMacros table for compiler-macro-function lookup
				if v.cdr == nil || v.cdr.typ != VPair {
					return nil, fmt.Errorf("define-compiler-macro: malformed form")
				}
				cmName := v.cdr.car
				if cmName == nil || cmName.typ != VSym {
					return nil, fmt.Errorf("define-compiler-macro: name must be a symbol")
				}
				name := cmName.str
				m := gcv()
				m.typ = VMacro
				m.str = name
				m.params = nil
				m.rest = ""
				m.whole = ""
				m.body = v.cdr.cdr
				m.env = env
				compilerMacros[name] = m
				return cmName, nil
			case "MACRO-EXPAND":
				if v.cdr == nil || v.cdr.typ != VPair {
					return nil, fmt.Errorf("macro-expand: malformed form")
				}
				expr, e2 := Eval(v.cdr.car, env)
				if e2 != nil {
					return nil, e2
				}
				if expr.typ != VPair {
					return nil, fmt.Errorf("macro-expand: need a list")
				}
				fnSym := expr.car
				if fnSym.typ != VSym {
					return nil, fmt.Errorf("macro-expand: first element must be a symbol")
				}
				fn, err := env.Get(fnSym.str)
				if err != nil || fn.typ != VMacro {
					return nil, fmt.Errorf("macro-expand: not a macro: %s", fnSym.str)
				}
				return expandMacro(fn, expr.cdr, env)
			case "STEP":
				// (step expr) — evaluate expr with stepping info
				body := v.cdr
				if isNil(body) || body.typ != VPair {
					return nil, fmt.Errorf("step: need an expression")
				}
				fmt.Fprintf(os.Stderr, "; Step: evaluating %s\n", writeToString(body.car))
				result, err := Eval(body.car, env)
				if err != nil {
					fmt.Fprintf(os.Stderr, "; Step: error: %s\n", err)
					return nil, err
				}
				fmt.Fprintf(os.Stderr, "; Step: => %s\n", writeToString(result))
				return result, nil
			case "TIME":
				// (time expr) — evaluate expr and print timing info
				body := v.cdr
				if isNil(body) || body.typ != VPair {
					return nil, fmt.Errorf("time: need an expression")
				}
				start := time.Now()
				result, err := Eval(body.car, env)
				elapsed := time.Since(start)
				fmt.Fprintf(os.Stderr, "; Evaluation took:\n;   %s\n;   (%v real time)\n", elapsed, elapsed)
				if err != nil {
					return nil, err
				}
				return result, nil
			case "IGNORE-ERRORS":
				// (ignore-errors body...) — returns (values result nil) on success,
				// or (values nil condition) on error
				body := v.cdr
				var result *Value
				for !isNil(body) {
					result, err = Eval(body.car, env)
					if err != nil {
						// On error, return (values nil condition)
						cond := goErrorToCondition(err)
						return multiVal(vnil(), cond), nil
					}
					body = body.cdr
				}
				return multiVal(result, vnil()), nil
			case "UNWIND-PROTECT":
				// (unwind-protect protected-form cleanup-form...)
				if v.cdr == nil || v.cdr.typ != VPair {
					return nil, fmt.Errorf("unwind-protect: malformed form")
				}
				protected := v.cdr.car
				cleanup := v.cdr.cdr
				var result *Value
				// Execute protected form
				func() {
					defer func() {
						// Always execute cleanup forms
						for !isNil(cleanup) {
							Eval(cleanup.car, env)
							cleanup = cleanup.cdr
						}
					}()
					result, err = Eval(protected, env)
				}()
				return result, err
			case "AND":
				args := v.cdr
				if isNil(args) {
					return vbool(true), nil
				}
				for args.typ == VPair && !isNil(args.cdr) {
					r, e := Eval(args.car, env)
					if e != nil {
						return nil, e
					}
					if !isTruthy(r) {
						return r, nil
					}
					args = args.cdr
				}
				if args.typ == VPair {
					v = args.car
					continue
				}
				return vnil(), nil
			case "OR":
				args := v.cdr
				if isNil(args) {
					return vbool(false), nil
				}
				for args.typ == VPair && !isNil(args.cdr) {
					r, e := Eval(args.car, env)
					if e != nil {
						return nil, e
					}
					if isTruthy(r) {
						return r, nil
					}
					args = args.cdr
				}
				if args.typ == VPair {
					v = args.car
					continue
				}
				return vnil(), nil
			case "BLOCK":
				if v.cdr == nil || v.cdr.typ != VPair {
					return nil, fmt.Errorf("block: malformed form")
				}
				if v.cdr.car == nil {
					return nil, fmt.Errorf("block: name must be a symbol")
				}
				// Accept VNil as a valid block name for (block nil ...)
				var blockName string
				if v.cdr.car.typ == VSym {
					blockName = v.cdr.car.str
				} else if v.cdr.car.typ == VNil {
					blockName = "NIL"
				} else {
					return nil, fmt.Errorf("block: name must be a symbol")
				}
				body := v.cdr.cdr
				var result *Value
				for !isNil(body) {
					result, err = Eval(body.car, env)
					if err != nil {
						if br, ok := err.(*blockReturn); ok && br.name == blockName {
							if br.isLoopFinish {
								// loop-finish: return loop-result if set, otherwise result
								if lr, lerr := env.Get("LOOP-RESULT"); lerr == nil && lr != nil && lr != vnil() {
									return lr, nil
								}
								return result, nil
							}
							return br.value, nil
						}
						return nil, err
					}
					body = body.cdr
				}
				return result, nil
			case "RETURN-FROM":
				if v.cdr == nil || v.cdr.typ != VPair {
					return nil, fmt.Errorf("return-from: malformed form")
				}
				if v.cdr.car == nil {
					return nil, fmt.Errorf("return-from: block name must be a symbol")
				}
				// Accept VNil as a valid block name for (return-from nil ...)
				var name string
				if v.cdr.car.typ == VSym {
					name = v.cdr.car.str
				} else if v.cdr.car.typ == VNil {
					name = "NIL"
				} else {
					return nil, fmt.Errorf("return-from: block name must be a symbol")
				}
				if v.cdr.cdr == nil || v.cdr.cdr.typ != VPair {
					return nil, fmt.Errorf("return-from: missing return value")
				}
				val, err := Eval(v.cdr.cdr.car, env)
				if err != nil {
					return nil, err
				}
				return nil, &blockReturn{name: name, value: val}
			case "LOOP-FINISH":
				// Check if loop-result is bound and return its value
				if v, lerr := env.Get("LOOP-RESULT"); lerr == nil && v != nil && v != vnil() {
					return nil, &blockReturn{name: "NIL", value: v}
				}
				return nil, &blockReturn{name: "NIL", value: vnil(), isLoopFinish: true}
			case "RETURN":
				rval := vnil()
				if v.cdr != nil && v.cdr.typ == VPair && !isNil(v.cdr) {
					var e2 error
					rval, e2 = Eval(v.cdr.car, env)
					if e2 != nil {
						return nil, e2
					}
				}
				return nil, &blockReturn{name: "NIL", value: rval}

			case "TAGBODY":
				body := v.cdr
				if body.typ != VPair && !isNil(body) {
					return nil, fmt.Errorf("tagbody: malformed form")
				}
				// Build tag -> position map
				tagPos := make(map[string]int)
				pos := 0
				for !isNil(body) {
					stmt := body.car
					if stmt.typ == VSym {
						tagPos[stmt.str] = pos
					} else if stmt.typ == VNum {
						tagPos[strconv.FormatFloat(stmt.num, 'f', 0, 64)] = pos
					}
					body = body.cdr
					pos++
				}
				// Execute statements
				body = v.cdr
				for !isNil(body) {
					stmt := body.car
					if stmt.typ != VSym && stmt.typ != VNum {
						_, err = Eval(stmt, env)
						if err != nil {
							if _, ok := err.(*blockReturn); ok {
								return nil, err
							}
							if gt, ok := err.(*goTag); ok {
								targetPos, ok := tagPos[gt.tag]
								if !ok {
									return nil, fmt.Errorf("go: tag %s not found", gt.tag)
								}
								body = v.cdr
								for i := 0; i < targetPos; i++ {
									body = body.cdr
								}
								// Skip past the tag itself to the next statement
								body = body.cdr
								continue
							}
							return nil, err
						}
					}
					body = body.cdr
				}
				return vnil(), nil
			case "GO":
				if v.cdr == nil || v.cdr.typ != VPair {
					return nil, fmt.Errorf("go: malformed form")
				}
				tag := v.cdr.car
				if tag.typ == VSym {
					return nil, &goTag{tag: tag.str}
				} else if tag.typ == VNum {
					return nil, &goTag{tag: strconv.FormatFloat(tag.num, 'f', 0, 64)}
				}
				return nil, fmt.Errorf("go: tag must be symbol or number")
			case "CATCH":
				if v.cdr == nil || v.cdr.typ != VPair {
					return nil, fmt.Errorf("catch: malformed form")
				}
				tag, err := Eval(v.cdr.car, env)
				if err != nil {
					return nil, fmt.Errorf("catch: tag evaluation error: %v", err)
				}
				var tagName string
				if tag.typ == VSym {
					tagName = tag.str
				} else if tag.typ == VStr {
					tagName = tag.str
				} else {
					return nil, fmt.Errorf("catch: tag must be a symbol or string")
				}
				body := v.cdr.cdr
				var result *Value = vnil()
				for !isNil(body) {
					result, err = Eval(body.car, env)
					if err != nil {
						if tv, ok := err.(*throwValue); ok && tv.tag == tagName {
							return tv.value, nil
						}
						// Propagate non-matching throws and other errors
						return nil, err
					}
					body = body.cdr
				}
				return result, nil
			case "THROW":
				if v.cdr == nil || v.cdr.typ != VPair {
					return nil, fmt.Errorf("throw: malformed form")
				}
				tag, err := Eval(v.cdr.car, env)
				if err != nil {
					return nil, fmt.Errorf("throw: tag evaluation error: %v", err)
				}
				var tagName string
				if tag.typ == VSym {
					tagName = tag.str
				} else if tag.typ == VStr {
					tagName = tag.str
				} else {
					return nil, fmt.Errorf("throw: tag must be a symbol or string")
				}
				if v.cdr.cdr == nil || v.cdr.cdr.typ != VPair {
					return nil, fmt.Errorf("throw: need a value form")
				}
				val := v.cdr.cdr.car
				val, err = Eval(val, env)
				if err != nil {
					return nil, err
				}
				return nil, &throwValue{tag: tagName, value: val}
			case "MULTIPLE-VALUE-BIND":
				// (multiple-value-bind (var1 var2 ...) values-form body...)
				if v.cdr == nil || v.cdr.typ != VPair {
					return nil, fmt.Errorf("multiple-value-bind: malformed form")
				}
				vars := v.cdr.car
				if v.cdr.cdr == nil || v.cdr.cdr.typ != VPair {
					return nil, fmt.Errorf("multiple-value-bind: malformed form")
				}
				valForm := v.cdr.cdr.car
				body := v.cdr.cdr.cdr
				valResult, e := Eval(valForm, env)
				if e != nil {
					return nil, e
				}
				// Get the list of values
				vals := multiValList(valResult)
				newEnv := NewEnv(env)
				vp := vars
				vl := vals
				for !isNil(vp) && !isNil(vl) {
					if vp.typ != VPair || vp.car == nil || vp.car.typ != VSym {
						return nil, fmt.Errorf("multiple-value-bind: vars must be symbols")
					}
					newEnv.Set(vp.car.str, vl.car)
					vp = vp.cdr
					vl = vl.cdr
				}
				// Remaining vars get nil
				for !isNil(vp) {
					if vp.typ != VPair || vp.car == nil || vp.car.typ != VSym {
						return nil, fmt.Errorf("multiple-value-bind: vars must be symbols")
					}
					newEnv.Set(vp.car.str, vnil())
					vp = vp.cdr
				}
				env = newEnv
				for body.typ == VPair && !isNil(body.cdr) {
					_, err = Eval(body.car, env)
					if err != nil {
						return nil, err
					}
					body = body.cdr
				}
				if body.typ != VPair {
					return vnil(), nil
				}
				v = body.car
				continue
			case "MULTIPLE-VALUE-LIST":
				// (multiple-value-list form)
				if v.cdr == nil || v.cdr.typ != VPair {
					return nil, fmt.Errorf("multiple-value-list: need a form")
				}
				form := v.cdr.car
				result, e := Eval(form, env)
				if e != nil {
					return nil, e
				}
				return multiValList(result), nil
			case "MULTIPLE-VALUE-SETQ":
				// (multiple-value-setq (var1 var2 ...) form)
				if v.cdr == nil || v.cdr.typ != VPair {
					return nil, fmt.Errorf("multiple-value-setq: malformed form")
				}
				vars := v.cdr.car
				form := v.cdr.cdr.car
				result, e := Eval(form, env)
				if e != nil {
					return nil, e
				}
				vals := multiValList(result)
				vp := vars
				vl := vals
				for !isNil(vp) && !isNil(vl) {
					if vp.typ != VPair || vp.car == nil || vp.car.typ != VSym {
						return nil, fmt.Errorf("multiple-value-setq: vars must be symbols")
					}
					env.Set(vp.car.str, vl.car)
					vp = vp.cdr
					vl = vl.cdr
				}
				v = result
				continue
			case "MULTIPLE-VALUE-PROG1":
				// (multiple-value-prog1 form1 form2 ...)
				// Evaluates all forms, returns values of the first form
				if isNil(v.cdr) {
					return nil, fmt.Errorf("multiple-value-prog1: need at least one form")
				}
				firstForm := v.cdr.car
				result, e := Eval(firstForm, env)
				if e != nil {
					return nil, e
				}
				// Evaluate remaining forms for side effects
				for form := v.cdr.cdr; !isNil(form); form = form.cdr {
					_, err = Eval(form.car, env)
					if err != nil {
						return nil, err
					}
				}
				return result, nil
			case "MULTIPLE-VALUE-CALL":
				// (multiple-value-call fn form1 form2 ...)
				if v.cdr == nil || v.cdr.typ != VPair {
					return nil, fmt.Errorf("multiple-value-call: malformed form")
				}
				fnForm := v.cdr.car
				fnVal, e := Eval(fnForm, env)
				if e != nil {
					return nil, e
				}
				// Collect all values from each form
				var allArgs *Value = vnil()
				tail := allArgs
				forms := v.cdr.cdr
				for !isNil(forms) {
					valResult, e := Eval(forms.car, env)
					if e != nil {
						return nil, e
					}
					vals := multiValList(valResult)
					// Append vals to allArgs
					for !isNil(vals) {
						cell := cons(vals.car, vnil())
						if isNil(allArgs) {
							allArgs = cell
							tail = cell
						} else {
							tail.cdr = cell
							tail = cell
						}
						vals = vals.cdr
					}
					forms = forms.cdr
				}
				return Apply(fnVal, allArgs, env)
			case "NTH-VALUE":
				// (nth-value n form) => nth value (0-based)
				if v.cdr == nil || v.cdr.typ != VPair {
					return nil, fmt.Errorf("nth-value: malformed form")
				}
				nVal, e := Eval(v.cdr.car, env)
				if e != nil {
					return nil, e
				}
				n := int(primaryValue(nVal).num)
				valResult, e := Eval(v.cdr.cdr.car, env)
				if e != nil {
					return nil, e
				}
				vals := multiValList(valResult)
				for i := 0; i < n && !isNil(vals); i++ {
					vals = vals.cdr
				}
				if isNil(vals) {
					return vnil(), nil
				}
				return vals.car, nil

			case "CASE":
				// (case keyform ((key1 key2 ...) body...) ... (else body...))
				// Also supports single key: (case keyform (key body...) ...)
				if v.cdr == nil || v.cdr.typ != VPair {
					return nil, fmt.Errorf("case: malformed form")
				}
				keyVal, e := Eval(v.cdr.car, env)
				if e != nil {
					return nil, e
				}
				keyVal = primaryValue(keyVal)
				clauses := v.cdr.cdr
				caseSeen := make(map[*Value]bool)
				for !isNil(clauses) {
					if caseSeen[clauses] {
						break
					}
					caseSeen[clauses] = true
					clause := clauses.car
					if clause == nil || clause.typ != VPair {
						clauses = clauses.cdr
						continue
					}
					keys := clause.car
					// Check for else clause (also t and otherwise per CL spec)
					if keys.typ == VSym && (keys.str == "ELSE" || keys.str == "T" || keys.str == "OTHERWISE") {
						body := clause.cdr
						for body.typ == VPair && !isNil(body.cdr) {
							_, err = Eval(body.car, env)
							if err != nil {
								return nil, err
							}
							body = body.cdr
						}
						if body.typ != VPair {
							return vnil(), nil
						}
						v = body.car
						continue evalLoop
					}
					// Normalize: if key is not a list, treat as single-element
					match := false
					if keys.typ != VPair {
						if eqVal(keys, keyVal) {
							match = true
						}
					} else {
						for !isNil(keys) {
							if eqVal(keys.car, keyVal) {
								match = true
								break
							}
							keys = keys.cdr
						}
					}
					if match {
						body := clause.cdr
						for body.typ == VPair && !isNil(body.cdr) {
							_, err = Eval(body.car, env)
							if err != nil {
								return nil, err
							}
							body = body.cdr
						}
						if body.typ != VPair {
							return vnil(), nil
						}
						v = body.car
						continue evalLoop
					}
					clauses = clauses.cdr
				}
				return vnil(), nil

			case "TYPECASE":
				// (typecase keyform ((type) body...) ... (else body...))
				if v.cdr == nil || v.cdr.typ != VPair {
					return nil, fmt.Errorf("typecase: malformed form")
				}
				keyVal, e := Eval(v.cdr.car, env)
				if e != nil {
					return nil, e
				}
				keyVal = primaryValue(keyVal)
				clauses := v.cdr.cdr
				typecaseSeen := make(map[*Value]bool)
				for !isNil(clauses) && clauses.typ == VPair {
					if typecaseSeen[clauses] {
						break
					}
					typecaseSeen[clauses] = true
					clause := clauses.car
					if clause == nil || clause.typ != VPair {
						clauses = clauses.cdr
						continue
					}
					typeSpec := clause.car
					// Check for else clause
					if typeSpec != nil && typeSpec.typ == VSym && (typeSpec.str == "ELSE" || typeSpec.str == "OTHERWISE" || typeSpec.str == "T") {
						body := clause.cdr
						for body.typ == VPair && !isNil(body.cdr) {
							_, err = Eval(body.car, env)
							if err != nil {
								return nil, err
							}
							body = body.cdr
						}
						if body.typ != VPair {
							return vnil(), nil
						}
						v = body.car
						continue evalLoop
					}
					if typepCheck(keyVal, typeSpec, env) {
						body := clause.cdr
						for body.typ == VPair && !isNil(body.cdr) {
							_, err = Eval(body.car, env)
							if err != nil {
								return nil, err
							}
							body = body.cdr
						}
						if body.typ != VPair {
							return vnil(), nil
						}
						v = body.car
						continue evalLoop
					}
					clauses = clauses.cdr
				}
				return vnil(), nil
			case "ECASE":
				// (ecase keyform (key body...) ...)
				if v.cdr == nil || v.cdr.typ != VPair {
					return nil, fmt.Errorf("ecase: malformed form")
				}
				keyVal, e := Eval(v.cdr.car, env)
				if e != nil {
					return nil, e
				}
				keyVal = primaryValue(keyVal)
				clauses := v.cdr.cdr
				ecaseSeen := make(map[*Value]bool)
				for !isNil(clauses) {
					if ecaseSeen[clauses] {
						break
					}
					ecaseSeen[clauses] = true
					clause := clauses.car
					if clause == nil || clause.typ != VPair {
						clauses = clauses.cdr
						continue
					}
					keys := clause.car
					match := false
					if keys.typ != VPair {
						if eqVal(keys, keyVal) {
							match = true
						}
					} else {
						for !isNil(keys) {
							if eqVal(keys.car, keyVal) {
								match = true
								break
							}
							keys = keys.cdr
						}
					}
					if match {
						body := clause.cdr
						for body.typ == VPair && !isNil(body.cdr) {
							_, err = Eval(body.car, env)
							if err != nil {
								return nil, err
							}
							body = body.cdr
						}
						if body.typ != VPair {
							return vnil(), nil
						}
						v = body.car
						continue evalLoop
					}
					clauses = clauses.cdr
				}
				return nil, fmt.Errorf("ecase: no match for %s", ToString(keyVal))
			case "ETYPECASE":
				// (etypecase keyform (type body...) ...)
				if v.cdr == nil || v.cdr.typ != VPair {
					return nil, fmt.Errorf("etypecase: malformed form")
				}
				keyVal, e := Eval(v.cdr.car, env)
				if e != nil {
					return nil, e
				}
				keyVal = primaryValue(keyVal)
				clauses := v.cdr.cdr
				for !isNil(clauses) {
					clause := clauses.car
					if clause == nil || clause.typ != VPair {
						clauses = clauses.cdr
						continue
					}
					typeSpec := clause.car
					if typepCheck(keyVal, typeSpec, env) {
						body := clause.cdr
						for body.typ == VPair && !isNil(body.cdr) {
							_, err = Eval(body.car, env)
							if err != nil {
								return nil, err
							}
							body = body.cdr
						}
						if body.typ != VPair {
							return vnil(), nil
						}
						v = body.car
						continue evalLoop
					}
					clauses = clauses.cdr
				}
				return nil, fmt.Errorf("etypecase: no match for %s", ToString(keyVal))

			case "CTYPECASE":
				// (ctypecase keyplace (type body...) ...)
				// Like etypecase but keyplace is a place (setf-able)
				if v.cdr == nil || v.cdr.typ != VPair {
					return nil, fmt.Errorf("ctypecase: malformed form")
				}
				keyVal, e := Eval(v.cdr.car, env)
				if e != nil {
					return nil, e
				}
				keyVal = primaryValue(keyVal)
				clauses := v.cdr.cdr
				for !isNil(clauses) {
					clause := clauses.car
					if clause == nil || clause.typ != VPair {
						clauses = clauses.cdr
						continue
					}
					typeSpec := clause.car
					if typepCheck(keyVal, typeSpec, env) {
						body := clause.cdr
						for body.typ == VPair && !isNil(body.cdr) {
							_, err = Eval(body.car, env)
							if err != nil {
								return nil, err
							}
							body = body.cdr
						}
						if body.typ != VPair {
							return vnil(), nil
						}
						v = body.car
						continue evalLoop
					}
					clauses = clauses.cdr
				}
				return nil, fmt.Errorf("ctypecase: no match for %s", ToString(keyVal))

			case "DESTRUCTURING-BIND":
				// (destructuring-bind (pattern) expr body...)
				if v.cdr == nil || v.cdr.typ != VPair {
					return nil, fmt.Errorf("destructuring-bind: malformed form")
				}
				pattern := v.cdr.car
				expr := v.cdr.cdr.car
				body := v.cdr.cdr.cdr
				val, e := Eval(expr, env)
				if e != nil {
					return nil, e
				}
				val = primaryValue(val)
				newEnv := NewEnv(env)
				e = bindPattern(pattern, val, newEnv)
				if e != nil {
					return nil, e
				}
				env = newEnv
				for body.typ == VPair && !isNil(body.cdr) {
					_, err = Eval(body.car, env)
					if err != nil {
						return nil, err
					}
					body = body.cdr
				}
				if body.typ != VPair {
					return vnil(), nil
				}
				v = body.car
				continue

			case "HANDLER-CASE":
				// (handler-case expr (type (var) body...) ... (:no-error (vars...) body...))
				if v.cdr == nil || v.cdr.typ != VPair {
					return nil, fmt.Errorf("handler-case: malformed form")
				}
				valForm := v.cdr.car
				clauses := v.cdr.cdr
				// Scan for :no-error clause first
				var noErrorClause *Value
				var noErrorVars []string
				hcSeen := make(map[*Value]bool)
				scanClauses := clauses
				for !isNil(scanClauses) && scanClauses.typ == VPair {
					if hcSeen[scanClauses] {
						break
					}
					hcSeen[scanClauses] = true
					clause := scanClauses.car
					if clause.typ == VPair && clause.car != nil && clause.car.typ == VSym && clause.car.str == ":NO-ERROR" {
						noErrorClause = clause
						// Parse variable list
						varsForm := clause.cdr.car
						for !isNil(varsForm) && varsForm.typ == VPair {
							if varsForm.car != nil && varsForm.car.typ == VSym {
								noErrorVars = append(noErrorVars, varsForm.car.str)
							}
							varsForm = varsForm.cdr
						}
						break
					}
					scanClauses = scanClauses.cdr
				}
				// Push handler entries for each clause type (skip :no-error)
				savedLen := len(handlerStack)
				hcSeen2 := make(map[*Value]bool)
				for !isNil(clauses) && clauses.typ == VPair {
					if hcSeen2[clauses] {
						break
					}
					hcSeen2[clauses] = true
					clause := clauses.car
					if clause == nil || clause.typ != VPair {
						clauses = clauses.cdr
						continue
					}
					if clause.typ != VPair {
						return nil, fmt.Errorf("handler-case: malformed clause")
					}
					if clause.car == nil || clause.car.typ != VSym {
						return nil, fmt.Errorf("handler-case: clause must start with a type symbol")
					}
					typeSym := clause.car.str
					if typeSym == ":NO-ERROR" {
						clauses = clauses.cdr
						continue // skip :no-error in handler setup
					}
					// Use a sentinel VPrim that panics with handledError
					capturedType := typeSym
					handlerFn := &Value{typ: VPrim, fn: func(args []*Value) (*Value, error) {
						cond := args[0]
						panic(&handledError{condition: cond, result: nil, typeSym: capturedType})
					}}
					handlerStack = append(handlerStack, handlerEntry{
						typeSymbol: typeSym,
						handlerFn:  handlerFn,
						env:        env,
					})
					clauses = clauses.cdr
				}
				// Evaluate valForm with panic recovery
				var hcResult *Value
				var hcErr error
				hcHandled := false
				func() {
					defer func() {
						if r := recover(); r != nil {
							handlerStack = handlerStack[:savedLen]
							if he, ok := r.(*handledError); ok {
								// Find matching clause and evaluate body
								cl2 := v.cdr.cdr
								cl2Seen := make(map[*Value]bool)
								for !isNil(cl2) && cl2.typ == VPair {
									if cl2Seen[cl2] {
										break
									}
									cl2Seen[cl2] = true
									clause := cl2.car
									cond := he.condition
									if clause.typ != VPair {
										cl2 = cl2.cdr
										continue
									}
									if clause.car == nil || clause.car.typ != VSym {
										cl2 = cl2.cdr
										continue
									}
									clauseTypeSym := clause.car.str
									if classMatchesCondition(clauseTypeSym, cond) {
										hcHandled = true
										body := clause.cdr
										varName := ""
										if isPair(body) && isPair(body.car) && body.car.car != nil && body.car.car.typ == VSym {
											varName = body.car.car.str
										} else if isPair(body) && body.car != nil && body.car.typ == VSym {
											varName = body.car.str
										}
										newEnv := NewEnv(env)
										if varName != "" {
											newEnv.Set(varName, cond)
										}
										body = body.cdr
										hcResult = vnil()
										for !isNil(body) {
											hcResult, hcErr = Eval(body.car, newEnv)
											if hcErr != nil {
												return
											}
											body = body.cdr
										}
										return
									}
									cl2 = cl2.cdr
								}
								// No handler matched — convert to error instead of re-panicking
								if he.condition != nil {
									hcErr = fmt.Errorf("unhandled condition: %s", ToString(he.condition))
								} else {
									hcErr = fmt.Errorf("unhandled condition")
								}
							} else {
								// Not a handledError — re-panic (genuine panic)
								panic(r)
							}
						}
						handlerStack = handlerStack[:savedLen]
					}()
					hcResult, hcErr = Eval(valForm, env)
				}()
				// If eval returned a Go error, convert it to a Lisp condition
				// and walk clauses to find a matching handler.
				if hcErr != nil && !hcHandled {
					cond := makeSimpleCondition("simple-error", hcErr.Error())
					checkBreakOnSignals(cond)
					cl3 := v.cdr.cdr
					cl3Seen := make(map[*Value]bool)
					for !isNil(cl3) && cl3.typ == VPair {
						if cl3Seen[cl3] {
							break
						}
						cl3Seen[cl3] = true
						clause := cl3.car
						if clause == nil || clause.typ != VPair {
							cl3 = cl3.cdr
							continue
						}
						if clause.car == nil || clause.car.typ != VSym {
							cl3 = cl3.cdr
							continue
						}
						clauseTypeSym := clause.car.str
						if clauseTypeSym == ":NO-ERROR" {
							cl3 = cl3.cdr
							continue
						}
						if classMatchesCondition(clauseTypeSym, cond) {
							hcHandled = true
							hcErr = nil
							body := clause.cdr
							varName := ""
							if isPair(body) && isPair(body.car) && body.car.car != nil && body.car.car.typ == VSym {
								varName = body.car.car.str
							} else if isPair(body) && body.car != nil && body.car.typ == VSym {
								varName = body.car.str
							}
							newEnv := NewEnv(env)
							if varName != "" {
								newEnv.Set(varName, cond)
							}
							body = body.cdr
							hcResult = vnil()
							for !isNil(body) {
								hcResult, hcErr = Eval(body.car, newEnv)
								if hcErr != nil {
									break
								}
								body = body.cdr
							}
							break
						}
						cl3 = cl3.cdr
					}
				}
				if hcErr != nil {
					return nil, hcErr
				}
				// Evaluate :no-error clause if present
				if !hcHandled && noErrorClause != nil {
					noErrorBody := noErrorClause.cdr.cdr // skip :no-error and var list
					if len(noErrorVars) > 0 {
						noErrorEnv := NewEnv(env)
						for i, vname := range noErrorVars {
							if i == 0 {
								noErrorEnv.Set(vname, hcResult)
							}
						}
						hcResult = vnil()
						for !isNil(noErrorBody) {
							hcResult, hcErr = Eval(noErrorBody.car, noErrorEnv)
							if hcErr != nil {
								return nil, hcErr
							}
							noErrorBody = noErrorBody.cdr
						}
					}
				}
				return hcResult, nil

			case "HANDLER-BIND":
				// (handler-bind ((type handler-fn) ...) body...)
				if v.cdr == nil || v.cdr.typ != VPair {
					return nil, fmt.Errorf("handler-bind: malformed form")
				}
				bindings := v.cdr.car
				body := v.cdr.cdr
				savedLen := len(handlerStack)
				hbSeen := make(map[*Value]bool)
				for !isNil(bindings) && bindings.typ == VPair {
					if hbSeen[bindings] {
						break
					}
					hbSeen[bindings] = true
					binding := bindings.car
					if binding.typ != VPair {
						return nil, fmt.Errorf("handler-bind: malformed binding")
					}
					if binding.car == nil || binding.car.typ != VSym {
						return nil, fmt.Errorf("handler-bind: type specifier must be a symbol")
					}
					typeSym := binding.car.str
					if binding.cdr == nil || binding.cdr.typ != VPair {
						return nil, fmt.Errorf("handler-bind: handler must be a form")
					}
					handlerFn := binding.cdr.car
					// Evaluate the handler expression (e.g. (lambda (c) ...))
					evaldFn, e := Eval(handlerFn, env)
					if e != nil {
						return nil, e
					}
					handlerStack = append(handlerStack, handlerEntry{
						typeSymbol: typeSym,
						handlerFn:  evaldFn,
						env:        env,
					})
					bindings = bindings.cdr
				}
				defer func() {
					handlerStack = handlerStack[:savedLen]
				}()
				var hbResult *Value
				var hbErr error
				func() {
					defer func() {
						if r := recover(); r != nil {
							// Re-panic handledError so handler-case can catch it
							// Let other panics (restartInvoke, tailCall) propagate naturally
							if _, ok := r.(*handledError); ok {
								panic(r)
							}
							// For all other panics, convert to error return
							hbErr = fmt.Errorf("handler-bind caught panic: %v", r)
						}
					}()
					for body.typ == VPair && !isNil(body.cdr) {
						hbResult, hbErr = Eval(body.car, env)
						if hbErr != nil {
							return
						}
						body = body.cdr
					}
					if body.typ == VPair {
						hbResult, hbErr = Eval(body.car, env)
					} else if !isNil(body) {
						hbResult, hbErr = Eval(body, env)
					} else {
						hbResult = vnil()
					}
				}()
				return hbResult, hbErr

			case "RESTART-CASE":
				// (restart-case expr (name (arg) body...) ...)
				if v.cdr == nil || v.cdr.typ != VPair {
					return nil, fmt.Errorf("restart-case: malformed form")
				}
				valForm := v.cdr.car
				clauses := v.cdr.cdr
				savedLen := len(restartStack)
				rcSeen := make(map[*Value]bool)
				for !isNil(clauses) && clauses.typ == VPair {
					if rcSeen[clauses] {
						break
					}
					rcSeen[clauses] = true
					clause := clauses.car
					if clause == nil || clause.typ != VPair {
						clauses = clauses.cdr
						continue
					}
					if clause.typ != VPair {
						return nil, fmt.Errorf("restart-case: malformed clause")
					}
					if clause.car == nil || clause.car.typ != VSym {
						return nil, fmt.Errorf("restart-case: clause must start with a name")
					}
					name := clause.car.str
					body := clause.cdr
					// Extract varName from lambda list
					varName := ""
					if isPair(body) && isPair(body.car) && body.car.car != nil && body.car.car.typ == VSym {
						varName = body.car.car.str
					}
					body = body.cdr // skip lambda list
					restartStack = append(restartStack, restartEntry{
						name:      name,
						handlerFn: nil, // body is evaluated on invoke
						condition: nil,
						env:       env,
						id:        nextRestartID,
					})
					nextRestartID++
					_ = varName
					_ = body
					clauses = clauses.cdr
				}
				// Evaluate valForm with restart handling
				var rcResult *Value
				var rcErr error
				func() {
					defer func() {
						if r := recover(); r != nil {
							// Save restart stack before truncating
							stk := restartStack
							// Check for condition in panic to auto-associate restarts
							var panicCond *Value
							if he, ok := r.(*handledError); ok {
								panicCond = he.condition
							}
							if panicCond != nil {
								for i := savedLen; i < len(stk); i++ {
									if stk[i].condition == nil {
										stk[i].condition = panicCond
									}
								}
							}
							restartStack = restartStack[:savedLen]
							if ri, ok := r.(*restartInvoke); ok {
								// Find matching restart by ID in our range [savedLen, len(stk))
								targetIdx := -1
								for i := savedLen; i < len(stk); i++ {
									if stk[i].id == ri.id {
										targetIdx = i
										break
									}
								}
								if targetIdx >= 0 {
									// Map to clause position: targetIdx - savedLen
									clausePos := targetIdx - savedLen
									cl2 := v.cdr.cdr
									cl2Seen := make(map[*Value]bool)
									for !isNil(cl2) && cl2.typ == VPair {
										if cl2Seen[cl2] {
											break
										}
										cl2Seen[cl2] = true
										if cl2.typ != VPair {
											break
										}
										cl2c := cl2.car
										if cl2c == nil || cl2c.typ != VPair || cl2c.car == nil || cl2c.car.typ != VSym {
											cl2 = cl2.cdr
											continue
										}
										if clausePos == 0 {
											bd := cl2c.cdr
											// Parse lambda list: extract var names
											varNames := []*Value{}
											var restVar *Value = nil
											ll := bd
											if isPair(ll) && isPair(ll.car) {
												ll = ll.car
												for !isNil(ll) {
													if ll.car != nil && ll.car.typ == VSym {
														s := ll.car.str
														if s == "&rest" || s == "&body" {
															if ll.cdr != nil && ll.cdr.typ == VPair && ll.cdr.car != nil && ll.cdr.car.typ == VSym {
																restVar = ll.cdr.car
															}
															break
														} else if s == "&optional" || s == "&key" || s == "&allow-other-keys" || s == "&aux" {
														ll = ll.cdr
														continue
														}
														varNames = append(varNames, ll.car)
													}
													ll = ll.cdr
												}
											}
											bd = bd.cdr
											newEnv := NewEnv(env)
											argVals := ri.args
											for j := 0; j < len(varNames) && !isNil(argVals); j++ {
												newEnv.Set(varNames[j].str, argVals.car)
												argVals = argVals.cdr
											}
											if restVar != nil {
												newEnv.Set(restVar.str, argVals)
											}
											rcResult = vnil()
											for !isNil(bd) {
												rcResult, rcErr = Eval(bd.car, newEnv)
												if rcErr != nil {
													return
												}
												bd = bd.cdr
											}
											return
										}
										clausePos--
										cl2 = cl2.cdr
									}
								}
							}
							restartStack = restartStack[:savedLen]
							panic(r)
						}
						// Normal return: check if error carries a condition
						// and auto-associate our restart entries with it.
						if ce, ok := rcErr.(*conditionError); ok {
							for i := savedLen; i < len(restartStack); i++ {
								if restartStack[i].condition == nil {
									restartStack[i].condition = ce.condition
								}
							}
						}
						restartStack = restartStack[:savedLen]
					}()
					// Set innermost restart frame so builtinError knows
					// which restarts to auto-associate with the signaled condition
					oldFrame := innermostRestartFrame
					innermostRestartFrame = savedLen
					defer func() { innermostRestartFrame = oldFrame }()
					rcResult, rcErr = Eval(valForm, env)
				}()
				if rcErr != nil {
					return nil, rcErr
				}
				return rcResult, nil

			case "RESTART-BIND":
				// (restart-bind ((name fn) ...) body...)
				if v.cdr == nil || v.cdr.typ != VPair {
					return nil, fmt.Errorf("restart-bind: malformed form")
				}
				rbindings := v.cdr.car
				rbody := v.cdr.cdr
				rsavedLen := len(restartStack)
				rbSeen := make(map[*Value]bool)
				for !isNil(rbindings) && rbindings.typ == VPair {
					if rbSeen[rbindings] {
						break
					}
					rbinding := rbindings.car
					if rbinding.typ != VPair {
						return nil, fmt.Errorf("restart-case: malformed restart binding")
					}
					if rbinding.car == nil || rbinding.car.typ != VSym {
						return nil, fmt.Errorf("restart-bind: binding name must be a symbol")
					}
					rname := rbinding.car.str
					if rbinding.cdr == nil || isNil(rbinding.cdr) {
						return nil, fmt.Errorf("restart-bind: malformed restart binding")
					}
					if rbinding.cdr.typ != VPair {
						return nil, fmt.Errorf("restart-bind: malformed restart binding")
					}
					rfn := rbinding.cdr.car
					evaldRfn, e := Eval(rfn, env)
					if e != nil {
						return nil, e
					}
					restartStack = append(restartStack, restartEntry{
						name:      rname,
						handlerFn: evaldRfn,
						condition: nil,
						env:       env,
					})
					rbindings = rbindings.cdr
				}
				defer func() {
					restartStack = restartStack[:rsavedLen]
				}()
				for rbody.typ == VPair && !isNil(rbody.cdr) {
					_, err = Eval(rbody.car, env)
					if err != nil {
						return nil, err
					}
					rbody = rbody.cdr
				}
				v = rbody.car
				continue

			case "MACROLET":
				// (macrolet ((name (args...) body...) ...) expr...)
				if v.cdr == nil || v.cdr.typ != VPair {
					return nil, fmt.Errorf("macrolet: malformed form")
				}
				bindings := v.cdr.car
				body := v.cdr.cdr
				newEnv := NewEnv(env)
				for !isNil(bindings) {
					if bindings.typ != VPair {
						return nil, fmt.Errorf("restart-case: malformed bindings")
					}
					binding := bindings.car
					if binding.typ != VPair {
						return nil, fmt.Errorf("macrolet: malformed binding")
					}
					if binding.car == nil || binding.car.typ != VSym {
						return nil, fmt.Errorf("macrolet: macro name must be a symbol")
					}
					mname := binding.car.str
					macroParams := binding.cdr.car
					macroBody := binding.cdr.cdr
					params, rest, _, _, optDefaults, keySpecs, e := parseMacroParams(macroParams)
					if e != nil {
						return nil, e
					}
					m := gcv()
					m.typ = VMacro
					m.params = params
					m.rest = rest
					m.body = macroBody
					m.env = newEnv
					m.optDefaults = optDefaults
					m.keySpecs = keySpecs
					newEnv.Set(mname, m)
					bindings = bindings.cdr
				}
				env = newEnv
				for body.typ == VPair && !isNil(body.cdr) {
					_, err = Eval(body.car, env)
					if err != nil {
						return nil, err
					}
					body = body.cdr
				}
				if body.typ != VPair {
					return vnil(), nil
				}
				v = body.car
				continue
			case "SYMBOL-MACROLET":
				// (symbol-macrolet ((sym expansion) ...) expr...)
				if v.cdr == nil || v.cdr.typ != VPair {
					return nil, fmt.Errorf("symbol-macrolet: malformed form")
				}
				symBindings := v.cdr.car
				symBody := v.cdr.cdr
				symEnv := NewEnv(env)
				for !isNil(symBindings) {
					if symBindings.typ != VPair {
						return nil, fmt.Errorf("symbol-macrolet: malformed bindings")
					}
					b := symBindings.car
					if b.typ != VPair {
						return nil, fmt.Errorf("symbol-macrolet: malformed binding")
					}
					if b.car == nil || b.car.typ != VSym {
						return nil, fmt.Errorf("symbol-macrolet: binding name must be a symbol")
					}
					sname := b.car.str
					if b.cdr == nil || b.cdr.typ != VPair {
						return nil, fmt.Errorf("symbol-macrolet: malformed binding")
					}
					expansion := b.cdr.car
					sv := gcv()
					sv.typ = VSymMacro
					sv.car = expansion
					symEnv.Set(sname, sv)
					symBindings = symBindings.cdr
				}
				env = symEnv
				for symBody.typ == VPair && !isNil(symBody.cdr) {
					_ = symBody.car
					symBody = symBody.cdr
					continue
				}
				v = symBody.car
				continue
			case "EVAL-WHEN":
				// (eval-when (situations) body...)
				if v.cdr == nil || v.cdr.typ != VPair {
					return nil, fmt.Errorf("eval-when: malformed form")
				}
				situations := v.cdr.car
				ewBody := v.cdr.cdr
				execute := false
				for !isNil(situations) {
					s := situations.car
					if s.typ == VSym {
						switch s.str {
						case ":EXECUTE", "EXECUTE", ":EVAL", "EVAL",
							":LOAD-TOPLEVEL", "LOAD-TOPLEVEL", ":LOAD", "LOAD":
							execute = true
						}
					}
					situations = situations.cdr
				}
				if execute {
					for ewBody.typ == VPair && !isNil(ewBody.cdr) {
						_, err = Eval(ewBody.car, env)
						if err != nil {
							return nil, err
						}
						ewBody = ewBody.cdr
					}
					v = ewBody.car
					continue
				}
				return vnil(), nil
			case "SETF":
				// (setf var newval) or (setf (accessor args...) newval)
				if v.cdr == nil || v.cdr.typ != VPair || v.cdr.cdr == nil || v.cdr.cdr.typ != VPair {
					return nil, fmt.Errorf("setf: malformed form")
				}
				target := v.cdr.car
				newValExpr := v.cdr.cdr.car
				if target.typ == VSym {
					// Simple variable: (setf x val) -> like (set! x val)
					name := target.str
					ev, e := Eval(newValExpr, env)
					if e != nil {
						return nil, e
					}
					for scope := env; scope != nil; scope = scope.parent {
						if _, ok := scope.bindings[name]; ok {
							scope.bindings[name] = ev
							return ev, nil
						}
					}
					// Not found in any scope: create in globalEnv (CL setf semantics)
					globalEnv.Set(name, ev)
					return ev, nil
				}
				// (setf (accessor args...) newval) or (setf (values ...) newval)
				if target.typ != VPair {
					return nil, fmt.Errorf("setf: target must be a list or symbol")
				}
				if target.car == nil { return nil, fmt.Errorf("setf: empty accessor") }
				// Handle (setf (values place1 place2 ...) newval)
				if target.car.typ == VSym && target.car.str == "VALUES" {
					// Evaluate the newval expression to get all values
					vals, err := Eval(newValExpr, env)
					if err != nil {
						return nil, err
					}
					// Convert to list of values
					valList := multiValList(vals)
					// Collect all places from (values place1 place2 ...)
					places := target.cdr
					var result *Value
					placeIdx := 0
					for ; !isNil(places); places = places.cdr {
						val := vnil()
							// Walk valList to find the Nth element
							cur := valList
							for i := 0; cur != nil && cur.typ == VPair && i < placeIdx; i++ {
								cur = cur.cdr
							}
							if cur != nil && cur.typ == VPair && cur.car != nil {
								val = cur.car
						}
						// Build (setf place val) AST and evaluate recursively
						setfCall := &Value{typ: VPair,
							car: &Value{typ: VSym, str: "SETF"},
							cdr: &Value{typ: VPair, car: places.car,
								cdr: &Value{typ: VPair, car: val, cdr: vnil()}}}
						r, err := Eval(setfCall, env)
						if err != nil {
							return nil, err
						}
						if placeIdx == 0 {
							result = r
						}
						placeIdx++
					}
					if result == nil {
						result = vnil()
					}
					return result, nil
				}
				accessorName := target.car.str
				args := target.cdr
				// Look up <accessor>-setf function
				setter, err := env.Get(accessorName + "-setf")
				if err != nil {
					return nil, fmt.Errorf("setf: no setter for %s", accessorName)
				}
				// Build: (setter newval args...)
				newValNode := &Value{typ: VPair, car: newValExpr, cdr: vnil()}
				var callArgs *Value
				if isNil(args) {
					callArgs = newValNode
				} else {
					// First arg: newVal
					callArgs = &Value{typ: VPair, car: newValExpr, cdr: vnil()}
					tail := callArgs
					// Then original args
					for ; !isNil(args); args = args.cdr {
						tail.cdr = &Value{typ: VPair, car: args.car, cdr: vnil()}
						tail = tail.cdr
					}
				}
				v = &Value{typ: VPair, car: setter, cdr: callArgs}
				continue
			case "COND":
				clauses := v.cdr
				seen := make(map[*Value]bool)
				for !isNil(clauses) && clauses.typ == VPair {
					if seen[clauses] {
						break
					}
					seen[clauses] = true
					cl := clauses.car
					if cl.typ != VPair {
						clauses = clauses.cdr
						continue
					}
					if cl.car != nil && cl.car.typ == VSym && cl.car.str == "ELSE" {
						body := cl.cdr
						if isNil(body) {
							return vnil(), nil
						}
						for body.typ == VPair && !isNil(body.cdr) {
							_, e := Eval(body.car, env)
							if e != nil {
								return nil, e
							}
							body = body.cdr
						}
						if body.typ != VPair {
							return vnil(), nil
						}
						v = body.car
						continue evalLoop
					}
					cond, e := Eval(cl.car, env)
					if e != nil {
						return nil, e
					}
					if isTruthy(cond) {
						body := cl.cdr
						if isNil(body) {
							return cond, nil
						}
						for body.typ == VPair && !isNil(body.cdr) {
							_, e := Eval(body.car, env)
							if e != nil {
								return nil, e
							}
							body = body.cdr
						}
						if body.typ != VPair {
							return vnil(), nil
						}
						v = body.car
						continue evalLoop
					}
					clauses = clauses.cdr
				}
				return vnil(), nil

			case "DEFCLASS":
				if v.cdr == nil || v.cdr.typ != VPair {
					return nil, fmt.Errorf("defclass: malformed form")
				}
				if v.cdr.car == nil || v.cdr.car.typ != VSym {
					return nil, fmt.Errorf("defclass: name must be a symbol")
				}
				className := v.cdr.car.str
				if v.cdr.cdr == nil || v.cdr.cdr.typ != VPair {
					return nil, fmt.Errorf("defclass: malformed form")
				}
				parentsVal := v.cdr.cdr.car
				if v.cdr.cdr.cdr == nil || v.cdr.cdr.cdr.typ != VPair {
					return nil, fmt.Errorf("defclass: missing slots")
				}
				slotsVal := v.cdr.cdr.cdr.car
				// Strip (quote ...) wrapper if present
				if !isNil(slotsVal) && slotsVal.typ == VPair && slotsVal.car != nil && slotsVal.car.typ == VSym && slotsVal.car.str == "QUOTE" {
					if !isNil(slotsVal.cdr) && slotsVal.cdr.typ == VPair {
						slotsVal = slotsVal.cdr.car
					} else {
						slotsVal = vnil()
					}
				}
				// Slot specifications (with options) come from the 3rd arg of defclass
				// If a 4th arg is provided, use that for full slot specs (used by defstruct)
				slotDefsVal := slotsVal
				if v.cdr.cdr.cdr.cdr != nil && v.cdr.cdr.cdr.cdr.typ == VPair && !isNil(v.cdr.cdr.cdr.cdr.car) {
					fullSpecs := v.cdr.cdr.cdr.cdr.car
					if fullSpecs.typ == VPair && fullSpecs.car != nil && fullSpecs.car.typ == VSym && fullSpecs.car.str == "QUOTE" {
						if !isNil(fullSpecs.cdr) && fullSpecs.cdr.typ == VPair {
							fullSpecs = fullSpecs.cdr.car
						}
					}
					slotDefsVal = fullSpecs
				}

				cl := gcv()
				cl.typ = VClass
				cl.str = className

				// Parse parent classes
				var parents []*Value
				for !isNil(parentsVal) {
					p := parentsVal.car
					// Look up parent in class registry (not eval — separates class ns from function ns)
					parentName := ""
					if p.typ == VSym {
						parentName = p.str
					} else {
						return nil, fmt.Errorf("defclass: parent must be a symbol")
					}
					pClass := findClass(parentName)
					if pClass == nil || pClass.typ != VClass {
						return nil, fmt.Errorf("defclass: %s is not a class", parentName)
					}
					parents = append(parents, pClass)
					parentsVal = parentsVal.cdr
				}
				cl.classParents = parents

				// Parse slot names (support both bare symbols and lists with options)
				var slots []string
				// Parse slot names and store slot specs in class body
				cl.body = slotDefsVal
				for !isNil(slotsVal) {
					slot := slotsVal.car
					if slot.typ == VSym {
						slots = append(slots, slot.str)
					} else if slot.typ == VPair && slot.car != nil && slot.car.typ == VSym {
						slots = append(slots, slot.car.str)
					}
					slotsVal = slotsVal.cdr
				}
				cl.classSlots = slots

				// Compute CPL
				cl.cpl = c3Linearize(cl, parents)

				// Store in class registry (separate namespace from functions)
				classRegistry[className] = cl
				// Also set in env, but don't shadow callable bindings
				existing, _ := env.Get(className)
				if existing == nil || existing.typ == VNil || existing.typ == VClass {
					env.Set(className, cl)
				}

				// Generate accessor functions for each slot
				for _, slotName := range slots {
					sn := slotName // capture in new variable for closure safety
					// slot-name reader
					readerName := sn
					readerFn := func(args []*Value) (*Value, error) {
						if len(args) != 1 || args[0].typ != VInstance {
							return nil, fmt.Errorf("%s: requires an instance", readerName)
						}
						inst := args[0]
						val, ok := inst.instSlots[sn]
						if !ok {
							return nil, fmt.Errorf("%s: slot %s is unbound", readerName, sn)
						}
						return val, nil
					}
					env.Set(readerName, &Value{typ: VPrim, fn: readerFn})

					// (setf slot-name) writer
					setfName := sn + "-setf"
					setfFn := func(args []*Value) (*Value, error) {
						if len(args) != 2 || args[0].typ != VInstance {
							return nil, fmt.Errorf("%s: requires instance and value", setfName)
						}
						inst := args[0]
						val := args[1]
						inst.instSlots[sn] = val
						return val, nil
					}
					env.Set(setfName, &Value{typ: VPrim, fn: setfFn})
				}

				// Generate generic function accessors from slot definitions
				// Parse slot definitions for :accessor, :reader, :writer options
				for slotDef := slotDefsVal; !isNil(slotDef); slotDef = slotDef.cdr {
					slotName := ""
					// First element is slot name symbol
					if slotDef.car != nil && slotDef.car.typ == VPair && slotDef.car.car != nil && slotDef.car.car.typ == VSym {
						slotName = slotDef.car.car.str
					} else if slotDef.car != nil && slotDef.car.typ == VSym {
						slotName = slotDef.car.str
					}
					if slotName == "" {
						continue
					}
					// Parse slot options
					var accessorName string
					var readerName string
					var writerName string
					options := slotDef.car.cdr // skip slot name
					for !isNil(options) {
						opt := options.car
						if opt != nil && opt.typ == VSym && opt.str == ":ACCESSOR" {
							if options.cdr != nil && options.cdr.typ == VPair {
								accessorName = options.cdr.car.str
							}
						} else if opt != nil && opt.typ == VSym && opt.str == ":READER" {
							if options.cdr != nil && options.cdr.typ == VPair {
								readerName = options.cdr.car.str
							}
						} else if opt != nil && opt.typ == VSym && opt.str == ":WRITER" {
							if options.cdr != nil && options.cdr.typ == VPair {
								writerName = options.cdr.car.str
							}
						}
						options = options.cdr
					}
					// Create generic function for :accessor or :reader
					if accessorName != "" {
						gf := gcv()
						gf.typ = VGeneric
						gf.str = accessorName
						m := genMethod{
							qualifier:    "",
							params:       []string{"INST"},
							specializers: []string{className},
							body:         list(list(vsym("SLOT-VALUE"), vsym("INST"), list(vsym("QUOTE"), vsym(slotName)))),
							env:          env,
						}
						gf.genMethods = append(gf.genMethods, m)
						env.Set(accessorName, gf)
					} else if readerName != "" {
						gf := gcv()
						gf.typ = VGeneric
						gf.str = readerName
						m := genMethod{
							qualifier:    "",
							params:       []string{"INST"},
							specializers: []string{className},
							body:         list(list(vsym("SLOT-VALUE"), vsym("INST"), list(vsym("QUOTE"), vsym(slotName)))),
							env:          env,
						}
						gf.genMethods = append(gf.genMethods, m)
						env.Set(readerName, gf)
					}
					// Create generic function for :writer
					if writerName != "" {
						gf := gcv()
						gf.typ = VGeneric
						gf.str = writerName
						m := genMethod{
							qualifier:    "",
							params:       []string{"VAL", "INST"},
							specializers: []string{"", className},
							body:         list(list(vsym("SETF"), list(vsym("SLOT-VALUE"), vsym("INST"), list(vsym("QUOTE"), vsym(slotName))), vsym("VAL"))),
							env:          env,
						}
						gf.genMethods = append(gf.genMethods, m)
						env.Set(writerName, gf)
					}
				}

				return vsym(className), nil

			case "DEFMETHOD":
				if v.cdr == nil || v.cdr.typ != VPair {
					return nil, fmt.Errorf("defmethod: malformed form")
				}
				qualifier := ""
				var gfName string
				rest := v.cdr

				// Parse function name and optional qualifier
				if rest.car != nil && rest.car.typ == VSym {
					gfName = rest.car.str
					rest = rest.cdr
					// Check for keyword qualifier: (defmethod greet :before ...)
					if rest.typ == VPair && rest.car != nil && rest.car.typ == VSym && isKeyword(rest.car.str) {
						qualifier = rest.car.str
						rest = rest.cdr
						// Check for auxiliary qualifier: (defmethod greet :around 1 ...)
						if rest.typ == VPair && rest.car != nil && rest.car.typ != VPair {
							qualifier += " " + ToString(rest.car)
							rest = rest.cdr
						}
					}
				} else if rest.car != nil && rest.car.typ == VPair {
					// (defmethod (greet :before) ...)
					head := rest.car
					if head.car == nil || head.car.typ != VSym {
						return nil, fmt.Errorf("defmethod: generic function name must be a symbol")
					}
					gfName = head.car.str
					rest = rest.cdr
					if head.cdr != nil && head.cdr.typ == VPair && head.cdr.car != nil && head.cdr.car.typ == VSym && isKeyword(head.cdr.car.str) {
						qualifier = head.cdr.car.str
						// Check for auxiliary qualifier: (defmethod (greet :around 1) ...)
						if head.cdr.cdr != nil && head.cdr.cdr.typ == VPair && head.cdr.cdr.car != nil && head.cdr.cdr.car.typ != VPair {
							qualifier += " " + ToString(head.cdr.cdr.car)
						}
					}
				} else {
					return nil, fmt.Errorf("defmethod: invalid form")
				}

				// Get or create generic function
				gf, err := env.Get(gfName)
				if err != nil {
					gf = gcv()
					gf.typ = VGeneric
					gf.str = gfName
					env.Set(gfName, gf)
				} else if gf.typ != VGeneric {
					gf = gcv()
					gf.typ = VGeneric
					gf.str = gfName
					env.Set(gfName, gf)
				}

				// Parse method parameters and body
				params := rest.car
				body := rest.cdr

				var paramNames []string
				var specializers []string
				for !isNil(params) {
					if params.car != nil && params.car.typ == VSym {
						// Simple parameter: (d)
						paramNames = append(paramNames, params.car.str)
						specializers = append(specializers, "")
					} else if params.car != nil && params.car.typ == VPair && params.car.car != nil && params.car.car.typ == VSym {
						// Specialized parameter: ((d dog)) or ((x (eql val)))
						paramNames = append(paramNames, params.car.car.str)
						sp := params.car.cdr
						if sp.typ == VPair && sp.car != nil {
							if sp.car.typ == VSym {
								// Class specializer: ((d dog))
								specializers = append(specializers, sp.car.str)
							} else if sp.car.typ == VPair && sp.car.car != nil && sp.car.car.typ == VSym && sp.car.car.str == "EQL" {
								// EQL specializer: ((x (eql val)))
								eqlVal := sp.car.cdr
								if eqlVal.typ == VPair && eqlVal.car != nil {
									specializers = append(specializers, "#EQL:"+valueToEQLKey(eqlVal.car))
								} else {
									specializers = append(specializers, "")
								}
							} else {
								specializers = append(specializers, "")
							}
						} else {
							specializers = append(specializers, "")
						}
					}
					params = params.cdr
				}

				m := genMethod{
					qualifier:    qualifier,
					params:       paramNames,
					specializers: specializers,
					body:         body,
					env:          env,
				}
				gf.genMethods = append(gf.genMethods, m)
				return vsym(gfName), nil

			case "CALL-NEXT-METHOD":
				// Check if call-next-method is bound locally (inside a method)
				if cnmFn, cnmErr := env.Get("CALL-NEXT-METHOD"); cnmErr == nil {
					return Apply(cnmFn, v.cdr, env)
				}
				return nil, fmt.Errorf("call-next-method: not inside a method")

				case "LOAD":
				if v.cdr == nil || v.cdr.typ != VPair {
					return nil, fmt.Errorf("load: need a filename")
				}
				fnameVal := primaryValue(v.cdr.car)
				if fnameVal.typ != VStr {
					return nil, fmt.Errorf("load: filename must be a string")
				}
				fname := fnameVal.str
				// Parse keyword arguments for :if-does-not-exist
				ifDoesNotExist := true // default per CL spec
				rest := v.cdr.cdr
				for !isNil(rest) {
					if rest.typ == VPair && rest.car.typ == VSym {
						sname := rest.car.str
						if sname == ":IF-DOES-NOT-EXIST" {
							if rest.cdr.typ == VPair {
								// Evaluate the keyword argument value
								val := rest.cdr.car
								if val.typ == VSym {
									// Evaluate symbol to get its value (nil -> VNil, t -> VBool for #t, etc.)
									ev, _ := Eval(val, env)
									if ev != nil {
										val = primaryValue(ev)
									}
								}
								if isNil(val) || val == globalEnv.bindings["#f"] {
									ifDoesNotExist = false
								}
								rest = rest.cdr.cdr
								continue
							}
						} else if sname == ":IF-EXISTS" {
							if rest.cdr.typ == VPair {
								rest = rest.cdr.cdr
								continue
							}
						}
					}
					if rest.typ == VPair {
						rest = rest.cdr
					} else {
						break
					}
				}
				// Check if file exists
				info, statErr := os.Stat(fname)
				if statErr != nil || info == nil {
					if !ifDoesNotExist {
						return vnil(), nil
					}
					return nil, fmt.Errorf("load: open %s: %v", fname, statErr)
				}
				return loadFile(fname, env)
			case "WITH-OPEN-FILE":
				// (with-open-file (var pathname &key direction ...) body...)
				if v.cdr == nil || v.cdr.typ != VPair {
					return nil, fmt.Errorf("with-open-file: malformed form")
				}
				spec := v.cdr.car
				if spec.typ != VPair {
					return nil, fmt.Errorf("with-open-file: need a binding spec")
				}
				if spec.car == nil || spec.car.typ != VSym {
					return nil, fmt.Errorf("with-open-file: var name must be a symbol")
				}
				varName := spec.car.str
				if spec.cdr.typ != VPair {
					return nil, fmt.Errorf("with-open-file: need a pathname")
				}
				body := v.cdr.cdr
				openCallArgs := []*Value{vsym("OPEN"), spec.cdr.car}
				rest := spec.cdr.cdr
				for !isNil(rest) {
					openCallArgs = append(openCallArgs, rest.car)
					rest = rest.cdr
				}
				stream, e := Eval(listFromSlice(openCallArgs), env)
				if e != nil {
					return nil, e
				}
				newEnv := env.Extend(varName, stream)
				var result *Value
				func() {
					defer func() {
						if stream.typ == VStream && !stream.stream.isClosed {
							stream.stream.close()
						}
					}()
					for body.typ == VPair && !isNil(body.cdr) {
						if body.typ == VPair {
							_, err = Eval(body.car, newEnv)
						} else if !isNil(body) {
							_, err = Eval(body, newEnv)
						}
						if err != nil {
							return
						}
						body = body.cdr
					}
					if body.typ == VPair {
						result, err = Eval(body.car, newEnv)
					} else if !isNil(body) {
						result, err = Eval(body, newEnv)
					} else {
						result = vnil()
					}
				}()
				return result, err
			case "WITH-OUTPUT-TO-STRING":
				// (with-output-to-string (var) body...)
				if v.cdr == nil || v.cdr.typ != VPair {
					return nil, fmt.Errorf("with-output-to-string: malformed form")
				}
				spec := v.cdr.car
				if spec.typ != VPair {
					return nil, fmt.Errorf("with-output-to-string: need binding spec")
				}
				if spec.car == nil || spec.car.typ != VSym {
					return nil, fmt.Errorf("with-output-to-string: var name must be a symbol")
				}
				body := v.cdr.cdr
				varName := spec.car.str
				stream := newStringOutput()
				newEnv := env.Extend(varName, stream)
				for body.typ == VPair && !isNil(body.cdr) {
					_, err = Eval(body.car, newEnv)
					if err != nil {
						return nil, err
					}
					body = body.cdr
				}
				if body.typ == VPair {
					_, err = Eval(body.car, newEnv)
					if err != nil {
						return nil, err
					}
				} else if !isNil(body) {
					_, err = Eval(body, newEnv)
					if err != nil {
						return nil, err
					}
				}
				return vstr(stream.stream.getStringOutput()), nil
			case "WITH-INPUT-FROM-STRING":
				// (with-input-from-string (var string) body...)
				if v.cdr == nil || v.cdr.typ != VPair {
					return nil, fmt.Errorf("with-input-from-string: malformed form")
				}
				spec := v.cdr.car
				if spec.typ != VPair {
					return nil, fmt.Errorf("with-input-from-string: need binding spec")
				}
				if spec.car == nil || spec.car.typ != VSym {
					return nil, fmt.Errorf("with-input-from-string: var name must be a symbol")
				}
				body := v.cdr.cdr
				varName := spec.car.str
				if spec.cdr.typ != VPair {
					return nil, fmt.Errorf("with-input-from-string: need a string form")
				}
				strVal, e := Eval(spec.cdr.car, env)
				if e != nil {
					return nil, e
				}
				stream := newStringInputStream(princToString(strVal))
				newEnv := env.Extend(varName, stream)
				var result *Value
				func() {
					defer func() {
						if stream.typ == VStream && !stream.stream.isClosed {
							stream.stream.close()
						}
					}()
					for body.typ == VPair && !isNil(body.cdr) {
						_, err = Eval(body.car, newEnv)
						if err != nil {
							return
						}
						body = body.cdr
					}
					if body.typ == VPair {
						result, err = Eval(body.car, newEnv)
					} else if !isNil(body) {
						result, err = Eval(body, newEnv)
					} else {
						result = vnil()
					}
				}()
				return result, err
			default:
				// regular application
				fn, e := Eval(v.car, env)
				if e != nil {
					return nil, e
				}
				r, e := Apply(fn, v.cdr, env)
				if e != nil {
					if tc, ok := e.(*tailCall); ok {
						v = tc.form
						env = tc.env
						continue evalLoop
					}
					return nil, e
				}
				return r, nil
			}
		}
	}
}

func Apply(fn *Value, args *Value, env *Env) (result *Value, err error) {
	switch fn.typ {
	case VPrim:
		argSlice := toSlice(args)
		evArgs := make([]*Value, len(argSlice))
		for i, a := range argSlice {
			v, e := Eval(a, env)
			if e != nil {
				return nil, e
			}
			evArgs[i] = primaryValue(v)
		}
		return fn.fn(evArgs)
	case VFunc:
		if fn.name != "" && traceTable[fn.name] {
			indent := strings.Repeat("  ", traceDepth)
			argSlice := toSlice(args)
			argStrs := make([]string, len(argSlice))
			for i, a := range argSlice {
				argStrs[i] = ToString(primaryValue(a))
			}
			fmt.Printf("%s%d: (%s %s)\n", indent, traceDepth, fn.name, strings.Join(argStrs, " "))
			traceDepth++
			defer func() {
				traceDepth--
				indent2 := strings.Repeat("  ", traceDepth)
				fmt.Printf("%s%d: <= %s\n", indent2, traceDepth, ToString(result))
			}()
		}
		argSlice := toSlice(args)
		evArgs := make([]*Value, len(argSlice))
		for i, a := range argSlice {
			v, e := Eval(a, env)
			if e != nil {
				return nil, e
			}
			evArgs[i] = primaryValue(v)
		}
				newEnv := NewEnv(fn.env)
		numRequired := len(fn.params) - len(fn.optDefaults) - len(fn.keySpecs)
		if numRequired < 0 {
			numRequired = 0
		}

		// Extract keyword args if function has key specs
		keyVals := make(map[string]*Value)
		positionalArgs := evArgs
		if len(fn.keySpecs) > 0 {
			var nonKeyword []*Value
			i := 0
			for i < len(evArgs) {
				if evArgs[i] != nil && evArgs[i].typ == VSym && len(evArgs[i].str) > 0 && evArgs[i].str[0] == ':' {
					keyName := evArgs[i].str[1:]
					if i+1 < len(evArgs) {
						keyVals[keyName] = evArgs[i+1]
						i += 2
					} else {
						nonKeyword = append(nonKeyword, evArgs[i])
						i++
					}
				} else {
					nonKeyword = append(nonKeyword, evArgs[i])
					i++
				}
			}
			positionalArgs = nonKeyword
		}

		// Bind required params
		for i := 0; i < numRequired; i++ {
			if i < len(positionalArgs) {
				newEnv.Set(fn.params[i], positionalArgs[i])
			} else {
				return nil, fmt.Errorf("apply: missing required argument")
			}
		}

		// Evaluate optional defaults and bind optional params
		paramIdx := numRequired
		for _, defaultExpr := range fn.optDefaults {
			if paramIdx < len(positionalArgs) {
				newEnv.Set(fn.params[paramIdx], positionalArgs[paramIdx])
			} else if !isNil(defaultExpr) {
				defVal, err := Eval(defaultExpr, newEnv)
				if err != nil {
					return nil, err
				}
				newEnv.Set(fn.params[paramIdx], defVal)
			} else {
				newEnv.Set(fn.params[paramIdx], vnil())
			}
			paramIdx++
		}

		// Bind key params
		paramIdx = numRequired + len(fn.optDefaults)
		for _, spec := range fn.keySpecs {
			if spec == nil || spec.typ != VPair || spec.car == nil || spec.cdr == nil || spec.cdr.typ != VPair || spec.cdr.cdr == nil || spec.cdr.cdr.typ != VPair {
				paramIdx++
				continue
			}
			keyName := spec.car.str
			if len(keyName) > 0 && keyName[0] == ':' {
				keyName = keyName[1:]
			}
			paramName := spec.cdr.car.str
			defaultExpr := spec.cdr.cdr.car
			if val, ok := keyVals[keyName]; ok {
				newEnv.Set(paramName, val)
			} else if !isNil(defaultExpr) {
				defVal, err := Eval(defaultExpr, newEnv)
				if err != nil {
					return nil, err
				}
				newEnv.Set(paramName, defVal)
			} else {
				newEnv.Set(paramName, vnil())
			}
			paramIdx++
		}

		// Bind rest param if present
		if fn.rest != "" {
			var restElems []*Value
			if len(fn.keySpecs) > 0 {
				restElems = positionalArgs[paramIdx:]
			} else if fn.optDefaults != nil {
				restElems = positionalArgs[len(fn.params)-len(fn.optDefaults):]
			} else {
				restElems = positionalArgs[len(fn.params):]
			}
			newEnv.Set(fn.rest, listFromSlice(restElems))
		}
		body := fn.body
		if isNil(body) {
			return vnil(), nil
		}
		for body.typ == VPair && !isNil(body.cdr) {
			_, e := Eval(body.car, newEnv)
			if e != nil {
				if _, ok := e.(*tailCall); ok {
					return nil, e
				}
				return nil, e
			}
			body = body.cdr
		}
		// Tail call optimization: instead of recursively calling eval,
		// return a tailCall instruction. The eval loop will catch it
		// and continue with the new form/environment without growing
		// the Go stack.
		return nil, &tailCall{form: body.car, env: newEnv}
	case VGeneric:
		argSlice := toSlice(args)
		evArgs := make([]*Value, len(argSlice))
		for i, a := range argSlice {
			v, e := Eval(a, env)
			if e != nil {
				return nil, e
			}
			evArgs[i] = v
		}

		// Filter applicable methods by specializers
		var applicable []genMethod
		for _, m := range fn.genMethods {
			if methodApplicable(m, evArgs) {
				applicable = append(applicable, m)
			}
		}

		// Separate methods by qualifier
		combo := fn.methodCombo
		if combo == "" {
			combo = "standard"
		}

		if combo != "standard" {
			// Non-standard method combination
			var before, after, around, comboPrimary []genMethod
			comboQual := ":" + strings.ToUpper(combo)
			for _, m := range applicable {
				switch m.qualifier {
				case ":BEFORE":
					before = append(before, m)
				case ":AFTER":
					after = append(after, m)
				case ":AROUND":
					around = append(around, m)
				case comboQual:
					comboPrimary = append(comboPrimary, m)
				default:
					if m.qualifier == "" {
						comboPrimary = append(comboPrimary, m)
					}
				}
			}

			// Sort methods by specificity
			for i := 0; i < len(comboPrimary); i++ {
				for j := i + 1; j < len(comboPrimary); j++ {
					if methodSpecificity(comboPrimary[j], evArgs) < methodSpecificity(comboPrimary[i], evArgs) {
						comboPrimary[i], comboPrimary[j] = comboPrimary[j], comboPrimary[i]
					}
				}
			}
			for i := 0; i < len(before); i++ {
				for j := i + 1; j < len(before); j++ {
					if methodSpecificity(before[j], evArgs) < methodSpecificity(before[i], evArgs) {
						before[i], before[j] = before[j], before[i]
					}
				}
			}
			for i := 0; i < len(after); i++ {
				for j := i + 1; j < len(after); j++ {
					if methodSpecificity(after[j], evArgs) > methodSpecificity(after[i], evArgs) {
						after[i], after[j] = after[j], after[i]
					}
				}
			}
			for i := 0; i < len(around); i++ {
				for j := i + 1; j < len(around); j++ {
					if methodSpecificity(around[j], evArgs) < methodSpecificity(around[i], evArgs) {
						around[i], around[j] = around[j], around[i]
					}
				}
			}

			execCombo := func() (*Value, error) {
				// Execute :before methods
				for _, m := range before {
					menv := NewEnv(m.env)
					for j, p := range m.params {
						if j < len(evArgs) {
							menv.Set(p, evArgs[j])
						}
					}
					if m.body != nil {
						bodyList := m.body
						for !isNil(bodyList) {
							_, e := Eval(bodyList.car, menv)
							if e != nil {
								return nil, e
							}
							bodyList = bodyList.cdr
						}
					}
				}

				var result *Value = vnil()
				switch combo {
				case "progn":
					for _, m := range comboPrimary {
						pEnv := NewEnv(m.env)
						for j, p := range m.params {
							if j < len(evArgs) {
								pEnv.Set(p, evArgs[j])
							}
						}
						if m.body != nil {
							bodyList := m.body
							for !isNil(bodyList) {
								r, e := Eval(bodyList.car, pEnv)
								if e != nil {
									return nil, e
								}
								result = r
								bodyList = bodyList.cdr
							}
						}
					}
				case "and":
					result = vsym("T")
					for _, m := range comboPrimary {
						pEnv := NewEnv(m.env)
						for j, p := range m.params {
							if j < len(evArgs) {
								pEnv.Set(p, evArgs[j])
							}
						}
						if m.body != nil {
							bodyList := m.body
							for !isNil(bodyList) {
								r, e := Eval(bodyList.car, pEnv)
								if e != nil {
									return nil, e
								}
								result = r
								bodyList = bodyList.cdr
							}
						}
						if isNil(result) {
							return vnil(), nil
						}
					}
				case "or":
					for _, m := range comboPrimary {
						pEnv := NewEnv(m.env)
						for j, p := range m.params {
							if j < len(evArgs) {
								pEnv.Set(p, evArgs[j])
							}
						}
						if m.body != nil {
							bodyList := m.body
							for !isNil(bodyList) {
								r, e := Eval(bodyList.car, pEnv)
								if e != nil {
									return nil, e
								}
								result = r
								bodyList = bodyList.cdr
							}
						}
						if !isNil(result) {
							return result, nil
						}
					}
					return vnil(), nil
				case "list":
					var results []*Value
					for _, m := range comboPrimary {
						pEnv := NewEnv(m.env)
						for j, p := range m.params {
							if j < len(evArgs) {
								pEnv.Set(p, evArgs[j])
							}
						}
						if m.body != nil {
							bodyList := m.body
							for !isNil(bodyList) {
								r, e := Eval(bodyList.car, pEnv)
								if e != nil {
									return nil, e
								}
								result = r
								bodyList = bodyList.cdr
							}
						}
						results = append(results, result)
					}
					return listFromSlice(results), nil
				case "append", "nconc":
					var elems []*Value
					for _, m := range comboPrimary {
						pEnv := NewEnv(m.env)
						for j, p := range m.params {
							if j < len(evArgs) {
								pEnv.Set(p, evArgs[j])
							}
						}
						if m.body != nil {
							bodyList := m.body
							for !isNil(bodyList) {
								r, e := Eval(bodyList.car, pEnv)
								if e != nil {
									return nil, e
								}
								result = r
								bodyList = bodyList.cdr
							}
						}
						if !isNil(result) && result.typ == VPair {
							for p := result; p != nil && p.typ == VPair; p = p.cdr {
								elems = append(elems, p.car)
							}
						} else if !isNil(result) {
							elems = append(elems, result)
						}
					}
					return listFromSlice(elems), nil
				case "min":
					if len(comboPrimary) == 0 {
						return vnil(), nil
					}
					var minVal *Value
					for _, m := range comboPrimary {
						pEnv := NewEnv(m.env)
						for j, p := range m.params {
							if j < len(evArgs) {
								pEnv.Set(p, evArgs[j])
							}
						}
						if m.body != nil {
							bodyList := m.body
							for !isNil(bodyList) {
								r, e := Eval(bodyList.car, pEnv)
								if e != nil {
									return nil, e
								}
								result = r
								bodyList = bodyList.cdr
							}
						}
						if minVal == nil || compareNumeric(result, minVal) < 0 {
							minVal = result
						}
					}
					return minVal, nil
				case "max":
					if len(comboPrimary) == 0 {
						return vnil(), nil
					}
					var maxVal *Value
					for _, m := range comboPrimary {
						pEnv := NewEnv(m.env)
						for j, p := range m.params {
							if j < len(evArgs) {
								pEnv.Set(p, evArgs[j])
							}
						}
						if m.body != nil {
							bodyList := m.body
							for !isNil(bodyList) {
								r, e := Eval(bodyList.car, pEnv)
								if e != nil {
									return nil, e
								}
								result = r
								bodyList = bodyList.cdr
							}
						}
						if maxVal == nil || compareNumeric(result, maxVal) > 0 {
							maxVal = result
						}
					}
					return maxVal, nil
				case "+":
					sum := 0.0
					hasFloat := false
					for _, m := range comboPrimary {
						pEnv := NewEnv(m.env)
						for j, p := range m.params {
							if j < len(evArgs) {
								pEnv.Set(p, evArgs[j])
							}
						}
						if m.body != nil {
							bodyList := m.body
							for !isNil(bodyList) {
								r, e := Eval(bodyList.car, pEnv)
								if e != nil {
									return nil, e
								}
								result = r
								bodyList = bodyList.cdr
							}
						}
						if result.isFloat {
							hasFloat = true
						}
						sum += toNum(result)
					}
					if hasFloat {
						return vfloat(sum), nil
					}
					return vnum(sum), nil
				default:
					if len(comboPrimary) > 0 {
						pm := comboPrimary[0]
						pEnv := NewEnv(pm.env)
						for j, p := range pm.params {
							if j < len(evArgs) {
								pEnv.Set(p, evArgs[j])
							}
						}
						if pm.body != nil {
							bodyList := pm.body
							for !isNil(bodyList) {
								r, e := Eval(bodyList.car, pEnv)
								if e != nil {
									return nil, e
								}
								result = r
								bodyList = bodyList.cdr
							}
						}
					}
				}

				// Execute :after methods
				for _, m := range after {
					aEnv := NewEnv(m.env)
					for j, p := range m.params {
						if j < len(evArgs) {
							aEnv.Set(p, evArgs[j])
						}
					}
					if m.body != nil {
						bodyList := m.body
						for !isNil(bodyList) {
							_, e := Eval(bodyList.car, aEnv)
							if e != nil {
								return nil, e
							}
							bodyList = bodyList.cdr
						}
					}
				}

				return result, nil
			}

			// Wrap around methods
			nextMethodFn := execCombo
			for i := len(around) - 1; i >= 0; i-- {
				am := around[i]
				prevNext := nextMethodFn
				nextMethodFn = func() (*Value, error) {
					aEnv := NewEnv(am.env)
					for j, p := range am.params {
						if j < len(evArgs) {
							aEnv.Set(p, evArgs[j])
						}
					}
					aEnv.Set("call-next-method", &Value{
						typ: VPrim,
						fn: func(args2 []*Value) (*Value, error) {
							return prevNext()
						},
					})
					aEnv.Set("next-method-p", &Value{
						typ: VPrim,
						fn: func(ignored []*Value) (*Value, error) {
							return vbool(true), nil
						},
					})
					var result *Value = vnil()
					if am.body != nil {
						bodyList := am.body
						for !isNil(bodyList) {
							r, e := Eval(bodyList.car, aEnv)
							if e != nil {
								return nil, e
							}
							result = r
							bodyList = bodyList.cdr
						}
					}
					return result, nil
				}
			}

			return nextMethodFn()
		}

		// Standard method combination (before/primary/after + around)
		// Separate methods by qualifier
		var primary, before, after, around []genMethod
		for _, m := range applicable {
			switch m.qualifier {
			case ":BEFORE":
				before = append(before, m)
			case ":AFTER":
				after = append(after, m)
			case ":AROUND":
				around = append(around, m)
			default:
				primary = append(primary, m)
			}
		}

		// Sort primary methods by specificity (most specific first)
		for i := 0; i < len(primary); i++ {
			for j := i + 1; j < len(primary); j++ {
				if methodSpecificity(primary[j], evArgs) < methodSpecificity(primary[i], evArgs) {
					primary[i], primary[j] = primary[j], primary[i]
				}
			}
		}

		// Sort before methods by specificity (most specific first)
		for i := 0; i < len(before); i++ {
			for j := i + 1; j < len(before); j++ {
				if methodSpecificity(before[j], evArgs) < methodSpecificity(before[i], evArgs) {
					before[i], before[j] = before[j], before[i]
				}
			}
		}

		// Sort after methods by specificity (least specific first for after)
		for i := 0; i < len(after); i++ {
			for j := i + 1; j < len(after); j++ {
				if methodSpecificity(after[j], evArgs) > methodSpecificity(after[i], evArgs) {
					after[i], after[j] = after[j], after[i]
				}
			}
		}

		// Sort around methods by specificity (most specific first)
		for i := 0; i < len(around); i++ {
			for j := i + 1; j < len(around); j++ {
				if methodSpecificity(around[j], evArgs) < methodSpecificity(around[i], evArgs) {
					around[i], around[j] = around[j], around[i]
				}
			}
		}

		// Build effective method: around methods wrapping before/primary/after chain
		execBeforePrimaryAfter := func() (*Value, error) {
			// Execute :before methods (most-specific first)
			for _, m := range before {
				menv := NewEnv(m.env)
				for j, p := range m.params {
					if j < len(evArgs) {
						menv.Set(p, evArgs[j])
					}
				}
				if m.body != nil {
					bodyList := m.body
					for !isNil(bodyList) {
						_, e := Eval(bodyList.car, menv)
						if e != nil {
							return nil, e
						}
						bodyList = bodyList.cdr
					}
				}
			}

			// Execute primary method
			var result *Value = vnil()
			if len(primary) > 0 {
				pm := primary[0]
				pEnv := NewEnv(pm.env)
				for j, p := range pm.params {
					if j < len(evArgs) {
						pEnv.Set(p, evArgs[j])
					}
				}
				// Bind call-next-method as a closure over remaining methods
				if len(primary) > 1 {
					remaining := primary[1:]
					cnmFn := &Value{
						typ: VPrim,
						fn: func(ignored []*Value) (*Value, error) {
							return callNextMethodChain(remaining, evArgs)
						},
					}
					pEnv.Set("call-next-method", cnmFn)
					pEnv.Set("next-method-p", &Value{
						typ: VPrim,
						fn: func(ignored []*Value) (*Value, error) {
							return vbool(true), nil
						},
					})
				} else {
					pEnv.Set("call-next-method", &Value{
						typ: VPrim,
						fn: func(args2 []*Value) (*Value, error) {
							return nil, fmt.Errorf("call-next-method: no next method")
						},
					})
					pEnv.Set("next-method-p", &Value{
						typ: VPrim,
						fn: func(ignored []*Value) (*Value, error) {
							return vbool(false), nil
						},
					})
				}
				if pm.body != nil {
					bodyList := pm.body
					for !isNil(bodyList) {
						r, e := Eval(bodyList.car, pEnv)
						if e != nil {
							return nil, e
						}
						result = r
						bodyList = bodyList.cdr
					}
				}
			}

			// Execute :after methods (least-specific first)
			for _, m := range after {
				aEnv := NewEnv(m.env)
				for j, p := range m.params {
					if j < len(evArgs) {
						aEnv.Set(p, evArgs[j])
					}
				}
				if m.body != nil {
					bodyList := m.body
					for !isNil(bodyList) {
						_, e := Eval(bodyList.car, aEnv)
						if e != nil {
							return nil, e
						}
						bodyList = bodyList.cdr
					}
				}
			}

			return result, nil
		}

		// Start with the before/primary/after chain as the innermost "next"
		nextMethodFn := execBeforePrimaryAfter

		// Wrap around methods from least-specific to most-specific
		for i := len(around) - 1; i >= 0; i-- {
			am := around[i]
			prevNext := nextMethodFn
			nextMethodFn = func() (*Value, error) {
				aEnv := NewEnv(am.env)
				for j, p := range am.params {
					if j < len(evArgs) {
						aEnv.Set(p, evArgs[j])
					}
				}
				aEnv.Set("call-next-method", &Value{
					typ: VPrim,
					fn: func(args2 []*Value) (*Value, error) {
						return prevNext()
					},
				})
				aEnv.Set("next-method-p", &Value{
					typ: VPrim,
					fn: func(ignored []*Value) (*Value, error) {
						return vbool(true), nil
					},
				})
				var result *Value = vnil()
				if am.body != nil {
					bodyList := am.body
					for !isNil(bodyList) {
						r, e := Eval(bodyList.car, aEnv)
						if e != nil {
							return nil, e
						}
						result = r
						bodyList = bodyList.cdr
					}
				}
				return result, nil
			}
		}

		// Execute the outermost effective method
		return nextMethodFn()
	case VMacro:
		expanded, e := expandMacro(fn, args, env)
		if e != nil {
			return nil, e
		}
		return Eval(expanded, env)
	default:
		return nil, fmt.Errorf("not a procedure: %s", typeStr(fn))
	}
}

func expandMacro(m *Value, args *Value, env *Env) (*Value, error) {
	newEnv := NewEnv(m.env)
	argSlice := toSlice(args)
	// Bind &whole if present
	if m.whole != "" {
		// Reconstruct the whole form: (macro-name . args)
		wholeForm := cons(vsym(m.str), args)
		newEnv.Set(m.whole, wholeForm)
	}

	// Calculate number of required params (exclude optional and key)
	numRequired := len(m.params) - len(m.optDefaults) - len(m.keySpecs)
	if numRequired < 0 {
		numRequired = 0
	}

	// Extract keyword args if macro has key specs
	keyVals := make(map[string]*Value)
	positionalArgs := argSlice
	if len(m.keySpecs) > 0 {
		var nonKeyword []*Value
		i := 0
		for i < len(argSlice) {
			if argSlice[i] != nil && argSlice[i].typ == VSym && len(argSlice[i].str) > 0 && argSlice[i].str[0] == ':' {
				keyName := argSlice[i].str[1:]
				if i+1 < len(argSlice) {
					keyVals[keyName] = argSlice[i+1]
					i += 2
				} else {
					nonKeyword = append(nonKeyword, argSlice[i])
					i++
				}
			} else {
				nonKeyword = append(nonKeyword, argSlice[i])
				i++
			}
		}
		positionalArgs = nonKeyword
	}

	if m.rest != "" {
		// Bind required params
		for i := 0; i < numRequired && i < len(positionalArgs); i++ {
			newEnv.Set(m.params[i], positionalArgs[i])
		}
		// Bind optional params with defaults
		for i := 0; i < len(m.optDefaults); i++ {
			p := m.params[numRequired+i]
			if numRequired+i < len(positionalArgs) {
				newEnv.Set(p, positionalArgs[numRequired+i])
			} else if m.optDefaults[i] != nil {
				defVal, err := Eval(m.optDefaults[i], newEnv)
				if err != nil {
					return nil, err
				}
				newEnv.Set(p, defVal)
			} else {
				newEnv.Set(p, vnil())
			}
		}
		// Bind key params
		paramIdx := numRequired + len(m.optDefaults)
		for _, spec := range m.keySpecs {
			if spec == nil || spec.typ != VPair || spec.car == nil || spec.cdr == nil || spec.cdr.typ != VPair || spec.cdr.cdr == nil || spec.cdr.cdr.typ != VPair {
				paramIdx++
				continue
			}
			keyName := spec.car.str
			if len(keyName) > 0 && keyName[0] == ':' {
				keyName = keyName[1:]
			}
			paramName := spec.cdr.car.str
			defaultExpr := spec.cdr.cdr.car
			if val, ok := keyVals[keyName]; ok {
				newEnv.Set(paramName, val)
			} else if defaultExpr != nil && !isNil(defaultExpr) {
				defVal, err := Eval(defaultExpr, newEnv)
				if err != nil {
					return nil, err
				}
				newEnv.Set(paramName, defVal)
			} else {
				newEnv.Set(paramName, vnil())
			}
			paramIdx++
		}
		// Bind rest param
		var restElems []*Value
		restIdx := numRequired + len(m.optDefaults)
		if len(m.keySpecs) > 0 {
			// rest gets everything after required+optional that isn't keywords
			if paramIdx < len(positionalArgs) {
				restElems = positionalArgs[paramIdx:]
			}
		} else if restIdx < len(positionalArgs) {
			restElems = positionalArgs[restIdx:]
		}
		newEnv.Set(m.rest, listFromSlice(restElems))
	} else {
		// No rest param - bind required params
		for i := 0; i < numRequired; i++ {
			if i < len(positionalArgs) {
				newEnv.Set(m.params[i], positionalArgs[i])
			} else {
				newEnv.Set(m.params[i], vnil())
			}
		}
		// Bind optional params with defaults
		for i := 0; i < len(m.optDefaults); i++ {
			p := m.params[numRequired+i]
			if numRequired+i < len(positionalArgs) {
				newEnv.Set(p, positionalArgs[numRequired+i])
			} else if m.optDefaults[i] != nil {
				defVal, err := Eval(m.optDefaults[i], newEnv)
				if err != nil {
					return nil, err
				}
				newEnv.Set(p, defVal)
			} else {
				newEnv.Set(p, vnil())
			}
		}
		// Bind key params
		paramIdx := numRequired + len(m.optDefaults)
		for _, spec := range m.keySpecs {
			if spec == nil || spec.typ != VPair || spec.car == nil || spec.cdr == nil || spec.cdr.typ != VPair || spec.cdr.cdr == nil || spec.cdr.cdr.typ != VPair {
				paramIdx++
				continue
			}
			keyName := spec.car.str
			if len(keyName) > 0 && keyName[0] == ':' {
				keyName = keyName[1:]
			}
			paramName := spec.cdr.car.str
			defaultExpr := spec.cdr.cdr.car
			if val, ok := keyVals[keyName]; ok {
				newEnv.Set(paramName, val)
			} else if defaultExpr != nil && !isNil(defaultExpr) {
				defVal, err := Eval(defaultExpr, newEnv)
				if err != nil {
					return nil, err
				}
				newEnv.Set(paramName, defVal)
			} else {
				newEnv.Set(paramName, vnil())
			}
			paramIdx++
		}
	}

	body := m.body
	if isNil(body) {
		return vnil(), nil
	}
	result := vnil()
	for !isNil(body) {
		head := body.car
		// If body.car is a VFunc/VPrim and the rest of body is an argument list,
		// call the function with evaluated arguments (this handles macros set
		// via (setf (macro-function ...) fn)).
		if head != nil && (head.typ == VFunc || head.typ == VPrim || head.typ == VGeneric) {
			// Collect and evaluate remaining items in body as args
			var callArgs []*Value
			for p := body.cdr; p != nil && p.typ == VPair; p = p.cdr {
				if p.car != nil {
					var ev *Value
					var err error
					// Special: #ENV resolves to the current lexical environment (passed as nil
					// for macro-function-setf macros, since the interpreter doesn't expose
					// environment objects to Lisp code; the macro function's closure captures
					// its original environment for free-variable resolution).
					if p.car.typ == VSym && p.car.str == "#ENV" {
						ev = vnil()
					} else {
						ev, err = Eval(p.car, newEnv)
						if err != nil {
							return nil, err
						}
					}
					callArgs = append(callArgs, primaryValue(ev))
				}
			}
			v, err := callFnOnSeq(head, callArgs, newEnv)
			if err != nil {
				return nil, err
			}
			result = v
			break // function call is the entire macro body
		}
		v, e := Eval(body.car, newEnv)
		if e != nil {
			return nil, e
		}
		result = v
		body = body.cdr
	}
	return result, nil
}

func evalQuasiquote(v *Value, depth int, env *Env) (*Value, error) {
	if !isPair(v) {
		return v, nil
	}
	// Determine symbol name for comparison (reader may produce lowercase)
	symName := ""
	if v.car != nil && v.car.typ == VSym {
		symName = v.car.str
	}
	// (unquote expr) at depth 0
	if strings.EqualFold(symName, "UNQUOTE") && depth == 0 {
		if v.cdr == nil || v.cdr.typ != VPair {
			return nil, fmt.Errorf("unquote: malformed form")
		}
		return Eval(v.cdr.car, env)
	}
	// (unquote-splicing expr) at depth > 0 - recursively process
	if strings.EqualFold(symName, "UNQUOTE-SPLICING") && depth > 0 {
		if v.cdr == nil || v.cdr.typ != VPair {
			return nil, fmt.Errorf("unquote-splicing: malformed form")
		}
		if depth == 1 {
			// The unquote-splicing belongs to the nearest quasiquote, evaluate the argument
			inner, e := Eval(v.cdr.car, env)
			if e != nil {
				return nil, e
			}
			return list(vsym("UNQUOTE-SPLICING"), inner), nil
		}
		// depth > 1: preserve the comma but process recursively
		inner, e := evalQuasiquote(v.cdr.car, depth-1, env)
		if e != nil {
			return nil, e
		}
		return list(vsym("UNQUOTE-SPLICING"), inner), nil
	}
	// (unquote-splicing expr) at depth 0 - not valid here
	if strings.EqualFold(symName, "UNQUOTE-SPLICING") && depth == 0 {
		return nil, fmt.Errorf("unquote-splicing outside list")
	}
	// (quasiquote expr)
	if strings.EqualFold(symName, "QUASIQUOTE") {
		if v.cdr == nil || v.cdr.typ != VPair {
			return nil, fmt.Errorf("quasiquote: malformed form")
		}
		inner, e := evalQuasiquote(v.cdr.car, depth+1, env)
		if e != nil {
			return nil, e
		}
		return list(vsym("QUASIQUOTE"), inner), nil
	}
	if strings.EqualFold(symName, "UNQUOTE") && depth > 0 {
		if v.cdr == nil || v.cdr.typ != VPair {
			return nil, fmt.Errorf("unquote: malformed form")
		}
		// depth > 0: process argument at depth-1 and preserve the unquote wrapper
		inner, e := evalQuasiquote(v.cdr.car, depth-1, env)
		if e != nil {
			return nil, e
		}
		return list(vsym("UNQUOTE"), inner), nil
	}
	// Build list
	var result *Value = vnil()
	var tail *Value
	iter := v
	seen := make(map[*Value]bool)
	for isPair(iter) {
		if seen[iter] {
			return nil, fmt.Errorf("quasiquote: circular list detected")
		}
		seen[iter] = true
		elem := iter.car
		if isPair(elem) && elem.car != nil && elem.car.typ == VSym && strings.EqualFold(elem.car.str, "UNQUOTE-SPLICING") && depth == 0 {
			if elem.cdr == nil || elem.cdr.typ != VPair {
				return nil, fmt.Errorf("unquote-splicing: malformed form")
			}
			splice, e := Eval(elem.cdr.car, env)
			if e != nil {
				return nil, e
			}
			// splice the list with cycle detection
			spliceSeen := make(map[*Value]bool)
			for !isNil(splice) {
				if spliceSeen[splice] {
					return nil, fmt.Errorf("quasiquote: circular list in spliced value")
				}
				spliceSeen[splice] = true
				pair := cons(splice.car, vnil())
				if result.typ == VNil {
					result = pair
					tail = pair
				} else {
					tail.cdr = pair
					tail = pair
				}
				splice = splice.cdr
			}
		} else {
			ev, e := evalQuasiquote(elem, depth, env)
			if e != nil {
				return nil, e
			}
			pair := cons(ev, vnil())
			if result.typ == VNil {
				result = pair
				tail = pair
			} else {
				tail.cdr = pair
				tail = pair
			}
		}
		iter = iter.cdr
	}
	// dotted tail
	if !isNil(iter) {
		if tail != nil {
			tail.cdr = iter
		} else {
			result = iter
		}
	}
	return result, nil
}

func parseParams(v *Value) ([]string, string, []*Value, []*Value, error) {
	if v.typ == VSym {
		return nil, v.str, nil, nil, nil
	}
	var params []string
	var optDefaults []*Value // default exprs for optional params (nil = use NIL)
	var keySpecs []*Value    // (keyword param-name default) for &key params
	seen := make(map[*Value]bool)
	inOptional := false
	inKey := false
	for !isNil(v) {
			// Handle dotted tail: when v becomes a VSym (e.g. (a b . rest)),
			// treat it as &rest parameter
			if v.typ == VSym {
				return params, v.str, optDefaults, keySpecs, nil
			}
		if seen[v] {
			return nil, "", nil, nil, fmt.Errorf("bad lambda parameter list: circular")
		}
		seen[v] = true
		elem := v.car
		// Check for keyword symbols first
		if elem != nil && elem.typ == VSym {
			s := elem.str
			if s == "&rest" || s == "&REST" || s == "&body" || s == "&BODY" {
				if v.cdr == nil || v.cdr.typ != VPair || v.cdr.car == nil || v.cdr.car.typ != VSym {
					return nil, "", nil, nil, fmt.Errorf("bad lambda parameter list: %s requires a symbol", s)
				}
				return params, v.cdr.car.str, optDefaults, keySpecs, nil
			}
			if s == "&optional" || s == "&OPTIONAL" {
				inOptional = true
				inKey = false
				v = v.cdr
				continue
			}
			if s == "&key" || s == "&KEY" {
				inOptional = false
				inKey = true
				v = v.cdr
				continue
			}
			if s == "&allow-other-keys" || s == "&AUX" || s == "&aux" || s == "&whole" || s == "&WHOLE" || s == "&environment" || s == "&ENVIRONMENT" {
				v = v.cdr
				continue
			}
			// Not a keyword, fall through to param handling
		}
		if inKey {
			// &key param
			var keyword, paramName string
			var defaultExpr *Value = vnil()
			if elem != nil && elem.typ == VSym {
				keyword = ":" + elem.str
				paramName = elem.str
			} else if elem != nil && elem.typ == VPair && elem.car != nil && elem.car.typ == VSym {
				// Check if this is ((keyword param) default) or (param default)
				if elem.car.typ == VSym && elem.car.str != "" && elem.car.str[0] == ':' {
					// This is ((:keyword param) default) format
					keyword = elem.car.str // already starts with ':'
					if elem.cdr != nil && elem.cdr.typ == VPair && elem.cdr.car != nil && elem.cdr.car.typ == VSym {
						paramName = elem.cdr.car.str
						if elem.cdr.cdr != nil && elem.cdr.cdr.typ == VPair && elem.cdr.cdr.car != nil {
							defaultExpr = elem.cdr.cdr.car
						}
					}
				} else if elem.car.typ == VPair && elem.car.car != nil && elem.car.car.typ == VSym {
					// This is ((keyword param) default) format where keyword may not start with ':'
					keyword = ":" + elem.car.car.str
					if elem.car.cdr != nil && elem.car.cdr.typ == VPair && elem.car.cdr.car != nil && elem.car.cdr.car.typ == VSym {
						paramName = elem.car.cdr.car.str
					}
					if elem.cdr != nil && elem.cdr.typ == VPair && elem.cdr.car != nil {
						defaultExpr = elem.cdr.car
					}
				} else {
					// This is (param default) format - keyword is :PARAM
					keyword = ":" + elem.car.str
					paramName = elem.car.str
					if elem.cdr != nil && elem.cdr.typ == VPair && elem.cdr.car != nil {
						defaultExpr = elem.cdr.car
					}
				}
			}
			if paramName != "" {
				params = append(params, paramName)
				keySpecs = append(keySpecs, list3(vsym(keyword), vsym(paramName), defaultExpr))
			} else {
			}
			v = v.cdr
			continue
		}
		if inOptional {
			// Optional param: can be symbol or (symbol default)
			if elem != nil && elem.typ == VSym {
				params = append(params, elem.str)
				optDefaults = append(optDefaults, nil)
			} else if elem != nil && elem.typ == VPair && elem.car != nil && elem.car.typ == VSym {
				params = append(params, elem.car.str)
				var defaultExpr *Value = vnil()
				if elem.cdr != nil && elem.cdr.typ == VPair && elem.cdr.car != nil {
					defaultExpr = elem.cdr.car
				}
				optDefaults = append(optDefaults, defaultExpr)
			}
			v = v.cdr
			continue
		}
		// Regular required param
		if elem != nil && elem.typ == VSym {
			params = append(params, elem.str)
		}
		v = v.cdr
	}
	if !isNil(v) && v.typ == VSym {
		return params, v.str, optDefaults, keySpecs, nil
	}
	return params, "", optDefaults, keySpecs, nil
}


func parseMacroParams(v *Value) (params []string, rest string, whole string, envSym string, optDefaults []*Value, keySpecs []*Value, err error) {
	// Check for &whole at the beginning
	if v.typ == VPair && v.car != nil && v.car.typ == VSym && (v.car.str == "&whole" || v.car.str == "&WHOLE") {
		restV := v.cdr
		if !isNil(restV) && restV.typ == VPair && restV.car != nil && restV.car.typ == VSym {
			whole = restV.car.str
			v = restV.cdr
		} else if !isNil(restV) && restV.typ == VSym {
			whole = restV.str
			v = vnil()
		}
	}
	inOptional := false
	inKey := false
	seen := make(map[*Value]bool)
	for !isNil(v) {
		if seen[v] {
			return nil, "", whole, envSym, nil, nil, fmt.Errorf("bad macro parameter list: circular")
		}
		seen[v] = true
		if v.car != nil && v.car.typ == VSym {
			s := v.car.str
			if s == "&rest" || s == "&body" || s == "&REST" || s == "&BODY" {
				restV := v.cdr
				if !isNil(restV) && restV.typ == VPair && restV.car != nil && restV.car.typ == VSym {
					rest = restV.car.str
					v = restV.cdr
				} else if !isNil(restV) && restV.typ == VSym {
					rest = restV.str
					v = vnil()
				} else {
					return nil, "", whole, envSym, nil, nil, fmt.Errorf("macro: need name after %s", s)
				}
				continue
			}
			if s == "&environment" || s == "&ENVIRONMENT" {
				envV := v.cdr
				if !isNil(envV) && envV.typ == VPair && envV.car != nil && envV.car.typ == VSym {
					envSym = envV.car.str
					v = envV.cdr
				} else if !isNil(envV) && envV.typ == VSym {
					envSym = envV.str
					v = vnil()
				}
				continue
			}
			if s == "&optional" || s == "&OPTIONAL" {
				inOptional = true
				inKey = false
				v = v.cdr
				continue
			}
			if s == "&key" || s == "&KEY" {
				inOptional = false
				inKey = true
				v = v.cdr
				continue
			}
			if s == "&allow-other-keys" || s == "&AUX" || s == "&aux" || s == "&whole" || s == "&WHOLE" {
				v = v.cdr
				continue
			}
			if inKey {
				keyword := ":" + s
				params = append(params, s)
				keySpecs = append(keySpecs, list3(vsym(keyword), vsym(s), vnil()))
				v = v.cdr
				continue
			}
			if inOptional {
				params = append(params, s)
				optDefaults = append(optDefaults, nil)
				v = v.cdr
				continue
			}
			params = append(params, s)
		} else if v.car != nil && v.car.typ == VPair {
			car := v.car.car
			if car != nil && car.typ == VSym {
				if inKey {
					var keyword, paramName string
					var defaultExpr *Value = vnil()
					elem := v.car
					if elem.car.typ == VPair && elem.car.car != nil && elem.car.car.typ == VSym {
						keyword = ":" + elem.car.car.str
						if elem.car.cdr != nil && elem.car.cdr.typ == VPair && elem.car.cdr.car != nil && elem.car.cdr.car.typ == VSym {
							paramName = elem.car.cdr.car.str
						}
						if elem.cdr != nil && elem.cdr.typ == VPair && elem.cdr.car != nil {
							defaultExpr = elem.cdr.car
						}
					} else {
						keyword = ":" + elem.car.str
						paramName = elem.car.str
						if elem.cdr != nil && elem.cdr.typ == VPair && elem.cdr.car != nil {
							defaultExpr = elem.cdr.car
						}
					}
					if paramName != "" {
						params = append(params, paramName)
						keySpecs = append(keySpecs, list3(vsym(keyword), vsym(paramName), defaultExpr))
					}
					v = v.cdr
					continue
				}
				if inOptional {
					params = append(params, car.str)
					var defaultExpr *Value = vnil()
					if v.car.cdr != nil && v.car.cdr.typ == VPair && v.car.cdr.car != nil {
						defaultExpr = v.car.cdr.car
					}
					optDefaults = append(optDefaults, defaultExpr)
					v = v.cdr
					continue
				}
				params = append(params, car.str)
			} else {
				break
			}
		} else {
			break
		}
		v = v.cdr
	}
	if !isNil(v) && v.typ == VSym {
		return params, v.str, whole, envSym, optDefaults, keySpecs, nil
	}
	if !isNil(v) {
		return nil, "", whole, envSym, nil, nil, fmt.Errorf("bad macro parameter list")
	}
	return params, rest, whole, envSym, optDefaults, keySpecs, nil
}

func typeStr(v *Value) string {
	switch v.typ {
	case VNil:
		return "NULL"
	case VNum:
		if v.isFloat {
			return "SINGLE-FLOAT"
		}
		return "INTEGER"
	case VRat:
		return "RATIONAL"
	case VComplex:
		return "COMPLEX"
	case VStr:
		return "STRING"
	case VSym:
		return "SYMBOL"
	case VBool:
		return "BOOLEAN"
	case VPair:
		return "PAIR"
	case VPrim:
		return "PROCEDURE"
	case VFunc:
		return "PROCEDURE"
	case VMacro:
		return "MACRO"
	case VClass:
		return "CLASS"
	case VRestart:
		return "RESTART"
	case VGeneric:
		return "GENERIC"
	case VInstance:
		return "INSTANCE"
	case VVHash:
		return "HASH-TABLE"
	case VThread:
		return "THREAD"
	case VLock:
		return "LOCK"
	case VChar:
		return "CHARACTER"
	case VStream:
		return "STREAM"
	case VArray:
		return "ARRAY"
	case VMultiVal:
		return "MULTI-VALUE"
	case VBigInt:
		return "INTEGER"
	case VPackage:
		return "PACKAGE"
	case VReadtable:
		return "READTABLE"
	case VPathname:
		return "PATHNAME"
	case VRandomState:
		return "RANDOM-STATE"
	case VMethod:
		return "METHOD"
	default:
		return "UNKNOWN"
	}
}

// -------- Builtins --------
type builtinDef struct {
	name string
	fn   NativeFunc
}

// specialOpNames is the list of all special operator names handled by eval.
var specialOpNames = []string{
	"quote", "function", "if", "progn", "let", "letrec", "labels",
	"flet", "set!", "setq", "defvar", "defparameter", "defconstant",
	"defmacro", "defun", "declare", "the", "block", "return-from",
	"return", "tagbody", "go", "catch", "throw", "unwind-protect",
	"multiple-value-bind", "multiple-value-list", "multiple-value-setq",
	"multiple-value-prog1", "multiple-value-call", "proclaim", "declaim",
	"progv", "funcall", "macrolet", "symbol-macrolet", "eval-when",
	"locally", "define", "define-macro", "define-symbol-macro", "define-compiler-macro", "lambda", "step", "time",
	"ignore-errors", "loop-finish", "and", "or", "not", "begin",
	"case", "typecase", "ecase", "etypecase", "ctypcase", "ctypecase",
	"destructuring-bind", "handler-case", "handler-bind",
	"restart-case", "restart-bind", "setf", "cond", "defclass",
	"defmethod", "call-next-method", "load", "with-open-file",
	"with-output-to-string", "with-input-from-string", "quasiquote",
	"macro-expand", "nth-value",
}

var builtins = []builtinDef{
	{"+", builtinAdd},
	{"-", builtinSub},
	{"*", builtinMul},
	{"/", builtinDiv},
	{"=", builtinEq},
	{"/=", builtinNe},
	{"<", builtinLt},
	{">", builtinGt},
	{"<=", builtinLe},
	{">=", builtinGe},
	{"cons", builtinCons},
	{"car", builtinCar},
	{"cdr", builtinCdr},
	{"set-car!", builtinSetCar},
	{"set", builtinSet},
	{"set-cdr!", builtinSetCdr},
	{"car-setf", builtinSetCarAsSetter},
	{"cdr-setf", builtinSetCdrAsSetter},
	{"list", builtinList},
	{"rest", builtinRest},
	{"first", builtinFirst},
	{"second", builtinSecond},
	{"third", builtinThird},
	{"fourth", builtinFourth},
	{"fifth", builtinFifth},
	{"sixth", builtinSixth},
	{"seventh", builtinSeventh},
	{"eighth", builtinEighth},
	{"ninth", builtinNinth},
	{"tenth", builtinTenth},
	{"null?", builtinNullP},
	{"pair?", builtinPairP},
	{"consp", builtinPairP},
	{"LISTP", builtinListP},
	{"number?", builtinNumP},
	{"string?", builtinStrP},
	{"package?", builtinPackageP},
	{"readtablep", builtinReadtableP},
	{"make-readtable", builtinMakeReadtable},
	{"copy-readtable", builtinCopyReadtable},
	{"readtable-case", builtinReadtableCase},
	{"set-readtable-case", builtinSetReadtableCase},
	{"set-macro-character", builtinSetMacroCharacter},
	{"get-macro-character", builtinGetMacroCharacter},
	{"make-dispatch-macro-character", builtinMakeDispatchMacroCharacter},
	{"set-dispatch-macro-character", builtinSetDispatchMacroCharacter},
	{"get-dispatch-macro-character", builtinGetDispatchMacroCharacter},
	{"symbol?", builtinSymP},
	{"bool?", builtinBoolP},
	{"procedure?", builtinProcP},
	{"characterp", builtinCharP},
	{"char", builtinChar},
	{"char-setf", builtinCharSetf},
	{"char=", builtinCharEq},
	{"char<", builtinCharLt},
	{"char>", builtinCharGt},
	{"char<=", builtinCharLe},
	{"char>=", builtinCharGe},
	{"code-char", builtinCodeChar},
	{"char-code", builtinCharCode},
	{"char-int", builtinCharInt},
	{"name-char", builtinNameChar},
	{"char-name", builtinCharName},
	{"eq?", builtinEqvP},
	{"eqv?", builtinEqvP},
	{"equal?", builtinEqualP},
	{"length", builtinLength},
	{"display", builtinDisplay},
	{"newline", builtinNewline},
	{"print", builtinPrint},
	{"prin1", builtinPrin1},
	{"princ", builtinPrincl},
	{"write", builtinWrite},
	{"terpri", builtinTerpri},
	{"fresh-line", builtinFreshLine},
	{"write-to-string", builtinWriteToString},
	{"read", builtinRead},
	{"read-from-string", builtinReadFromString},
	{"read-line", builtinReadLine},
	{"read-char", builtinReadChar},
	{"peek-char", builtinPeekChar},
	{"unread-char", builtinUnreadChar},
	{"write-char", builtinWriteChar},
	{"write-string", builtinWriteString},
	{"write-line", builtinWriteLine},
	{"read-byte", builtinReadByte},
	{"read-delimited-list", builtinReadDelimitedList},
	{"write-byte", builtinWriteByte},
	{"read-sequence", builtinReadSequence},
	{"write-sequence", builtinWriteSequence},
	{"open", builtinOpen},
	{"close", builtinClose},
	{"open-input-file", builtinOpenInputStream},
	{"open-output-file", builtinOpenOutputStream},
	{"make-string-input-stream", builtinMakeStringInputStream},
	{"make-string-output-stream", builtinMakeStringOutputStream},
	{"get-output-stream-string", builtinGetStringOutput},
	{"streamp", builtinStreamP},
	{"stream-input-p", builtinStreamInputP},
	{"input-stream-p", builtinStreamInputP},
	{"stream-output-p", builtinStreamOutputP},
	{"output-stream-p", builtinStreamOutputP},
	{"open-stream-p", builtinOpenStreamP},
	{"stream-element-type", builtinStreamElementType},
	{"read-char-no-hang", builtinReadCharNoHang},
	{"read-preserving-whitespace", builtinReadPreservingWhitespace},
	{"set-syntax-from-char", builtinSetSyntaxFromChar},
	{"file-string-length", builtinFileStringLength},
	{"interactive-stream-p", builtinInteractiveStreamP},
	{"listen", builtinListen},
	{"clear-input", builtinClearInput},
	{"force-output", builtinForceOutput},
	{"clear-output", builtinClearOutput},
	{"finish-output", builtinFinishOutput},
	{"y-or-n-p", builtinYOrNP},
	{"yes-or-no-p", builtinYesOrNoP},
	{"make-synonym-stream", builtinMakeSynonymStream},
	{"make-broadcast-stream", builtinMakeBroadcastStream},
	{"make-concatenated-stream", builtinMakeConcatenatedStream},
	{"make-two-way-stream", builtinMakeTwoWayStream},
	{"make-echo-stream", builtinMakeEchoStream},
	{"synonym-stream-p", builtinSynonymStreamP},
	{"broadcast-stream-p", builtinBroadcastStreamP},
	{"concatenated-stream-p", builtinConcatenatedStreamP},
	{"two-way-stream-p", builtinTwoWayStreamP},
	{"echo-stream-p", builtinEchoStreamP},
	{"string-stream-p", builtinStringStreamP},
	{"echo-stream-input-stream", builtinEchoStreamInputStream},
	{"echo-stream-output-stream", builtinEchoStreamOutputStream},
	{"synonym-stream-symbol", builtinSynonymStreamSymbol},
	{"broadcast-stream-streams", builtinBroadcastStreamStreams},
	{"concatenated-stream-streams", builtinConcatenatedStreamStreams},
	{"two-way-stream-input-stream", builtinTwoWayStreamInputStream},
	{"two-way-stream-output-stream", builtinTwoWayStreamOutputStream},
	{"string", builtinStr},
	{"string-append", builtinStrAppend},
	{"string-length", builtinStrLen},
	{"number->string", builtinNumStr},
	{"string-find", builtinStrFind},
	{"substring", builtinSubstring},
	{"string->number", builtinStrNum},
	{"symbol->string", builtinSymStr},
	{"symbol-name", builtinSymStr},
	{"symbol-value", builtinSymbolValue},
	{"symbol-function", builtinSymbolFunction},
	{"macro-function", builtinMacroFunction},
	{"macro-function-setf", builtinMacroFunctionSetf},
	{"compiler-macro-function", builtinCompilerMacroFunction},
	{"compiler-macro-function-setf", builtinCompilerMacroFunctionSetf},
	{"set-class-print-fn", builtinSetClassPrintFn},
	{"removeClassPrintFn", builtinRemoveClassPrintFn},
	{"symbol-plist", builtinSymbolPlist},
	{"symbol-plist-setf", builtinSymbolPlistSetf},
	{"boundp", builtinBoundp},
	{"fboundp", builtinFboundp},
	{"special-operator-p", builtinSpecialOperatorP},
	{"functionp", builtinFunctionP},
	{"make-generic-function", builtinMakeGenericFunction},
	{"makunbound", builtinMakunbound},
	{"fmakunbound", builtinFmakunbound},
	{"string->symbol", builtinStrSym},
	{"make-symbol", builtinStrSym},
	{"error", builtinErr},
	{"exit", builtinExit},
	{"gensym", builtinGensym},
	{"gentemp", builtinGentemp},
	{"%loop-check", builtinLoopCheck},
	{"type-of", builtinTypeOf},
	{"describe", builtinDescribe},
	{"room", builtinRoom},
	{"make-load-form", builtinMakeLoadForm},
	{"make-load-form-saving-slots", builtinMakeLoadFormSavingSlots},
	{"documentation", builtinDocumentation},
	{"apropos", builtinApropos},
	{"apropos-list", builtinAproposList},
	{"compile", builtinCompile},
	{"compile-file", builtinCompileFile},
	{"compile-file-pathname", builtinCompileFilePathname},
	{"fdefinition", builtinFdefinition},
	{"disassemble", builtinDisassemble},
	{"parse-integer", builtinParseInteger},
	{"digit-char-p", builtinDigitCharP},
	{"alphanumericp", builtinAlphanumericP},
	{"char-upcase", builtinCharUpcase},
	{"char-downcase", builtinCharDowncase},
	{"subtypep", builtinSubtypep},
	{"typep", builtinTypep},
	{"trace", builtinTrace},
	{"untrace", builtinUntrace},
	{"break", builtinBreak},
	{"eval", builtinEval},
	{"handler-eval", builtinHandlerEval},
	{"eval-string", builtinEvalString},
	{"funcall", builtinFuncall},
	{"eq", builtinEqIdentity},
	{"eql", builtinEql},
	{"equal", builtinEqual},
	{"equalp", builtinEqualp},
	{"make-array", builtinMakeArray},
	{"make-vector", builtinVector},
	{"vector", builtinVector},
	{"aref", builtinAref},
	{"svref", builtinAref},
	{"aref-setf", builtinSetAref},
	{"svref-setf", builtinSetAref},
	{"nth-setf", builtinNthSetf},
	{"symbol-value-setf", builtinSymbolValueSetf},
	{"elt-setf", builtinEltSetf},
	{"arrayp", builtinArrayP},
	{"vectorp", builtinVectorP},
	{"bit-vector-p", builtinBitVectorP},
	{"simple-vector-p", builtinSimpleVectorP},
	{"simple-bit-vector-p", builtinSimpleBitVectorP},
	{"simple-string-p", builtinSimpleStringP},
	{"array-dimensions", builtinArrayDimensions},
	{"array-dimension", builtinArrayDimension},
	{"array-total-size", builtinArrayTotalSize},
	{"array-rank", builtinArrayRank},
	{"array-element-type", builtinArrayElementType},
	{"upgraded-array-element-type", builtinUpgradedArrayElementType},
	{"upgraded-complex-part-type", builtinUpgradedComplexPartType},
	{"fill-pointer", builtinFillPointer},
	{"fill-pointer-setf", builtinFillPointerSetf},
	{"set-fill-pointer", builtinSetFillPointer},
	{"vector-push", builtinVectorPush},
	{"vector-pop", builtinVectorPop},
	{"adjust-array", builtinAdjustArray},
	{"array-has-fill-pointer-p", builtinArrayHasFillPointerP},
	{"adjustable-array-p", builtinAdjustableArrayP},
	{"array-displacement", builtinArrayDisplacement},
	{"array-in-bounds-p", builtinArrayInBoundsP},
	{"array-row-major-index", builtinArrayRowMajorIndex},
	{"null", builtinNull},
	{"apply", builtinApply},
	{"defined?", builtinDefinedP},
	{"ffi", builtinFFI},
	{"ffi-register", builtinFFIRegister},
	{"make-package", builtinMakePackage},
	{"in-package", builtinInPackage},
	{"find-package", builtinFindPackage},
	{"intern", builtinIntern},
	{"export", builtinExport},
	{"find-symbol", builtinFindSymbol},
	{"find-all-symbols", builtinFindAllSymbols},
	{"keywordp", builtinKeywordP},
	{"symbolp", builtinSymP},
	{"stringp", builtinStringP},
	{"copy-symbol", builtinCopySymbol},
	{"get", builtinGet},
	{"putprop", builtinPutprop},
	{"remprop", builtinRemprop},
	{"get-setf", builtinGetSetf},
	{"symbol-package", builtinSymbolPackage},
	{"package-name", builtinPackageName},
	{"list-all-packages", builtinListAllPackages},
	{"macroexpand", builtinMacroexpand},
	{"macroexpand-1", builtinMacroexpand1},
	{"provide", builtinProvide},
	{"require", builtinRequire},
	{"package-use-list", builtinPackageUseList},
	{"package-used-by-list", builtinPackageUsedByList},
	{"package-shadowing-import-list", builtinPackageShadowingImportList},
	{"import", builtinImport},
	{"use-package", builtinUsePackage},
	{"unuse-package", builtinUnusePackage},
	{"shadow", builtinShadow},
	{"unintern", builtinUnintern},
	{"shadowing-import", builtinShadowingImport},
	{"rename-package", builtinRenamePackage},
	{"delete-package", builtinDeletePackage},
	{"package-nicknames", builtinPackageNicknames},
	{"package-symbols", builtinPackageSymbols},
	{"package-external-symbols", builtinPackageExternalSymbols},
	{"unexport", builtinUnexport},
	{"make-instance", builtinMakeInstance},
	{"make-condition", builtinMakeCondition},
	{"slot-value", builtinSlotValue},
	{"slot-value-setf", builtinSlotValueSetf},
	{"slot-set!", builtinSlotSet},
	{"slot-boundp", builtinSlotBoundp},
	{"slot-exists-p", builtinSlotExistsP},
	{"slot-makunbound", builtinSlotMakunbound},
	{"class-of", builtinClassOf},
	{"class-name", builtinClassName},
	{"class-name-setf", builtinClassNameSetf},
	{"find-class", builtinFindClass},
	{"find-class-setf", builtinFindClassSetf},
	{"is-a?", builtinIsA},
	{"class-slots", builtinClassSlots},
	{"class-slot-defs", builtinClassSlotDefs},
	{"instance?", builtinInstanceP},
	{"find-method", builtinFindMethod},
	{"remove-method", builtinRemoveMethod},
	{"compute-applicable-methods", builtinComputeApplicableMethods},
	{"method-qualifiers", builtinMethodQualifiers},
	{"generic-function-p", builtinGenericFunctionP},
	{"ensure-generic-function", builtinEnsureGenericFunction},
	{"set-method-combination", builtinSetMethodCombination},
	{"make-hash-table", builtinMakeHashTable},
	{"hash-table-p", builtinHashTableP},
	{"packagep", builtinPackageP},
	{"compiled-function-p", builtinCompiledFunctionP},
	{"gethash", builtinGethash},
	{"gethash-setf", builtinSetGethash},
	{"remhash", builtinRemhash},
	{"clrhash", builtinClrhash},
	{"hash-table-count", builtinHashTableCount},
	{"maphash", builtinMaphash},
	{"hash-table?", builtinHashTableP},
	{"hash-table-size", builtinHashTableSize},
	{"hash-table-rehash-size", builtinHashTableRehashSize},
	{"hash-table-rehash-threshold", builtinHashTableRehashThreshold},
	{"hash-table-test", builtinHashTableTest},
	{"hash-table-keys", builtinHashTableKeys},
	{"hash-table-values", builtinHashTableValues},
	{"sxhash", builtinSxhash},
	{"hash-table-exists?", builtinHashTableExists},
	// CL standard aliases for seq-* functions
	{"reduce", builtinSeqReduce},
	{"find", builtinSeqFind},
	{"find-if", builtinSeqFindIf},
	{"find-if-not", builtinSeqFindIfNot},
	{"count", builtinSeqCount},
	{"count-if", builtinSeqCountIf},
	{"count-if-not", builtinSeqCountIfNot},
	{"remove", builtinSeqRemove},
	{"remove-if", builtinSeqRemoveIf},
	{"remove-if-not", builtinSeqRemoveIfNot},
	{"remove-duplicates", builtinSeqRemoveDuplicates},
	{"substitute", builtinSeqSubstitute},
	{"substitute-if", builtinSeqSubstituteIf},
	{"substitute-if-not", builtinSeqSubstituteIfNot},
	{"delete", builtinDelete},
	{"delete-if", builtinDeleteIf},
	{"delete-if-not", builtinDeleteIfNot},
	{"delete-duplicates", builtinDeleteDuplicates},
	{"nsubstitute", builtinNsubstitute},
	{"nsubstitute-if", builtinNsubstituteIf},
	{"nsubstitute-if-not", builtinNsubstituteIfNot},
	{"position-if", builtinSeqPositionIf},
	{"position-if-not", builtinSeqPositionIfNot},
	{"member-if-not", builtinMemberIfNot},
	{"assoc-if-not", builtinAssocIfNot},
	{"rassoc-if-not", builtinRassocIfNot},
	// Push/pushnew
	{"push", builtinPush},
	{"pushnew", builtinPushnew},
	// Vector push extend
	{"vector-push-extend", builtinVectorPushExtend},
	// Row-major aref
	{"row-major-aref", builtinRowMajorAref},
	{"bit", builtinBitAref},
	{"sbit", builtinSbitAref},
	// Random state
	{"make-random-state", builtinMakeRandomState},
	{"random-state-p", builtinRandomStateP},
	{"copy-random-state", builtinCopyRandomState},
	// Universal time
	{"get-universal-time", builtinGetUniversalTime},
	{"decode-universal-time", builtinDecodeUniversalTime},
	{"encode-universal-time", builtinEncodeUniversalTime},
	{"get-internal-real-time", builtinGetInternalRealTime},
	{"get-internal-run-time", builtinGetInternalRunTime},
	{"sleep", builtinSleep},
	// Environment functions
	{"lisp-implementation-type", builtinLispImplementationType},
	{"lisp-implementation-version", builtinLispImplementationVersion},
	{"machine-type", builtinMachineType},
	{"machine-version", builtinMachineVersion},
	{"machine-instance", builtinMachineInstance},
	{"software-type", builtinSoftwareType},
	{"software-version", builtinSoftwareVersion},
	{"short-site-name", builtinShortSiteName},
	{"long-site-name", builtinLongSiteName},
	// CL bit aliases
	{"bit-and", builtinBitAnd},
	{"bit-ior", builtinBitIor},
	{"bit-xor", builtinBitXor},
	{"bit-not", builtinBitNot},
	{"bit-eqv", builtinBitEqv},
	{"bit-nand", builtinBitNand},
	{"bit-nor", builtinBitNor},
	{"bit-orc1", builtinBitOrc1},
	{"bit-orc2", builtinBitOrc2},
	{"bit-andc1", builtinBitAndc1},
	{"bit-andc2", builtinBitAndc2},
	{"seq-map", builtinSeqMap},
	{"seq-reduce", builtinSeqReduce},
	{"seq-sort", builtinSeqSort},
	{"seq-remove-if", builtinSeqRemoveIf},
	{"seq-remove-if-not", builtinSeqRemoveIfNot},
	{"seq-count", builtinSeqCount},
	{"seq-count-if", builtinSeqCountIf},
	{"seq-count-if-not", builtinSeqCountIfNot},
	{"seq-find", builtinSeqFind},
	{"seq-position", builtinSeqPosition},
	{"seq-find-if", builtinSeqFindIf},
	{"seq-find-if-not", builtinSeqFindIfNot},
	{"seq-position-if", builtinSeqPositionIf},
	{"seq-position-if-not", builtinSeqPositionIfNot},
	{"seq-substitute", builtinSeqSubstitute},
	{"seq-substitute-if", builtinSeqSubstituteIf},
	{"seq-substitute-if-not", builtinSeqSubstituteIfNot},
	{"seq-remove", builtinSeqRemove},
	{"seq-remove-duplicates", builtinSeqRemoveDuplicates},
	{"seq-merge", builtinSeqMerge},
	{"fill", builtinSeqFill},
	{"search", builtinSeqSearch},
	{"mismatch", builtinMismatch},
	{"copy-seq", builtinSeqCopySeq},
	{"nreverse", builtinSeqNReverse},
	{"string-trim", builtinStringTrim},
	{"string-left-trim", builtinStringLeftTrim},
	{"string-right-trim", builtinStringRightTrim},
	{"replace", builtinSeqReplace},
	{"subseq", builtinSeqSubseq},
	{"subseq-setf", builtinSubseqSetf},
	{"concatenate", builtinSeqConcatenate},
	{"mapcan", builtinMapcan},
	{"mapcar", builtinMapcar},
	{"some", builtinSome},
	{"seq-some", builtinSome},
	{"every", builtinEvery},
	{"seq-every", builtinEvery},
	{"notany", builtinNotany},
	{"seq-notany", builtinNotany},
	{"notevery", builtinNotevery},
	{"seq-notevery", builtinNotevery},
	{"nconc", builtinNconc},
	{"adjoin", builtinAdjoin},
	{"subst", builtinSubst},
	{"sublis", builtinSublis},
	{"subst-if", builtinSubstIf},
	{"subst-if-not", builtinSubstIfNot},
	{"nsubst", builtinNsubst},
	{"nsubst-if", builtinNsubstIf},
	{"nsubst-if-not", builtinNsubstIfNot},
	{"nsublis", builtinNsublis},
	{"tree-equal", builtinTreeEqual},
	{"map-into", builtinMapInto},
	{"map", builtinMap},
	{"stable-sort", builtinStableSort},
	{"union", builtinUnion},
	{"intersection", builtinIntersection},
	{"set-difference", builtinSetDifference},
	{"set-exclusive-or", builtinSetExclusiveOr},
	{"nset-exclusive-or", builtinNsetExclusiveOr},
	{"nunion", builtinUnion},
	{"nintersection", builtinIntersection},
	{"nset-difference", builtinSetDifference},
	{"subsetp", builtinSubsetp},
	{"copy-list", builtinCopyList},
	{"copy-structure", builtinCopyStructure},
	{"copy-alist", builtinCopyAlist},
	{"copy-tree", builtinCopyTree},
	{"list-length", builtinListLength},
	{"last", builtinLast},
	{"last-pair", builtinLastPair},
	{"butlast", builtinButlast},
	{"nbutlast", builtinNbutlast},
	{"pairlis", builtinPairlis},
	{"assoc-if", builtinAssocIf},
	{"member-if", builtinMemberIf},
	{"member", builtinMember},
	{"position", builtinPosition},
	{"position-if", builtinPositionIf},
	{"revappend", builtinRevappend},
	{"ldiff", builtinLdiff},
	{"tailp", builtinTailp},
	{"nth-value", builtinNthValue},
	{"string-upcase", builtinStrUpcase},
	{"string-downcase", builtinStrDowncase},
	{"string-capitalize", builtinStrCapitalize},
	{"string=", builtinStrEqual},
	{"string-equal", builtinStrEqualCI},
	{"string<", builtinStrLess},
	{"isqrt", builtinIsqrt},
	{"decode-float", builtinDecodeFloat},
	{"integer-decode-float", builtinIntegerDecodeFloat},
	{"scale-float", builtinScaleFloat},
	{"float-radix", builtinFloatRadix},
	{"float-digits", builtinFloatDigits},
	{"float-precision", builtinFloatPrecision},
	{"format", builtinFormat},
	{"make-thread", builtinMakeThread},
	{"join-thread", builtinJoinThread},
	{"make-lock", builtinMakeLock},
	{"lock", builtinLock},
	{"unlock", builtinUnlock},
	{"condition-wait", builtinConditionWait},
	{"condition-notify", builtinConditionNotify},
	{"condition-broadcast", builtinConditionBroadcast},
	{"atomic-incf", builtinAtomicIncf},
	{"atomic-decf", builtinAtomicDecf},
	{"atomic-get", builtinAtomicGet},
	{"atomic-set", builtinAtomicSet},
	{"sleep", builtinSleep},
	{"values", builtinValues},
	{"values-list", builtinValuesList},
	{"error", builtinError},
	{"cerror", builtinCError},
	{"warn", builtinWarn},
	{"signal", builtinSignal},
	{"invoke-restart", builtinInvokeRestart},
	{"%associate-restarts-with-condition", builtinAssociateRestarts},
	{"%dissociate-restarts-with-condition", builtinDissociateRestarts},
	{"abort", builtinAbort},
	{"continue", builtinContinue},
	{"muffle-warning", builtinMuffleWarning},
	{"store-value", builtinStoreValue},
	{"use-value", builtinUseValue},
	{"compute-restarts", builtinComputeRestarts},
	{"find-restart", builtinFindRestart},
	{"restart-name", builtinRestartName},
	{"make-condvar", builtinMakeCondVar},
	{"thread?", builtinThreadP},
	{"lock?", builtinLockP},
	{"condvar?", builtinCondVarP},
	{"identity", builtinIdentity},
	{"complement", builtinComplement},
	{"constantly", builtinConstantly},
	{"parse-integer", builtinParseInteger},
	{"getf", builtinGetf},
	{"remf", builtinRemf},
	{"get-properties", builtinGetProperties},
	{"make-string", builtinMakeString},
	{"make-list", builtinMakeList},
	{"make-sequence", builtinMakeSequence},
	{"random", builtinRandom},
	{"nstring-upcase", builtinNStringUpcase},
	{"nstring-downcase", builtinNStringDowncase},
	{"nstring-capitalize", builtinNStringCapitalize},
	{"string-not-equal", builtinStringNotEqual},
	{"string-greaterp", builtinStringGreaterp},
	{"string-lessp", builtinStringLessp},
	{"string-not-greaterp", builtinStringNotGreaterp},
	{"string-not-lessp", builtinStringNotLessp},
	{"write-to-string", builtinWriteToString},
	{"prin1-to-string", builtinPrin1ToString},
	{"princ-to-string", builtinPrincToString},
	{"string-elt", builtinStringElt},
	{"reverse", builtinReverse},
	{"assoc", builtinAssoc},
	{"acons", builtinAcons},
	{"rassoc", builtinRassoc},
	{"rassoc-if", builtinRassocIf},
	{"nth", builtinNth},
	{"nthcdr", builtinNthCdr},
	{"string/=", builtinStringNotEq},
	{"string>", builtinStringGreater},
	{"string<=", builtinStringLe},
	{"string>=", builtinStringGe},
	{"abs", builtinAbs},
	{"max", builtinMax},
	{"min", builtinMin},
	{"mod", builtinMod},
	{"rem", builtinRem},
	{"floor", builtinFloor},
	{"ceiling", builtinCeiling},
	{"truncate", builtinTruncate},
	{"round", builtinRound},
	{"ffloor", builtinFfloor},
	{"fceiling", builtinFceiling},
	{"ftruncate", builtinFtruncate},
	{"fround", builtinFround},
	{"signum", builtinSignum},
	{"gcd", builtinGCD},
	{"lcm", builtinLCM},
	{"log", builtinLog},
	{"sqrt", builtinSqrt},
	{"expt", builtinExpt},
	{"sin", builtinSin},
	{"cos", builtinCos},
	{"tan", builtinTan},
	{"atan", builtinAtan},
	{"atan2", builtinAtan2},
	{"asin", builtinAsin},
	{"acos", builtinAcos},
	{"rationalize", builtinRationalize},
	{"exp", builtinExp},
	{"sinh", builtinSinh},
	{"cosh", builtinCosh},
	{"tanh", builtinTanh},
	{"asinh", builtinAsinh},
	{"acosh", builtinAcosh},
	{"atanh", builtinAtanh},
	{"evenp", builtinEvenp},
	{"oddp", builtinOddp},
	{"plusp", builtinPlusp},
	{"minusp", builtinMinusp},
	{"zerop", builtinZerop},
	{"1+", builtinOnePlus},
	{"1-", builtinOneMinus},
	{"digit-char", builtinDigitChar},
	{"alphanumericp", builtinAlphanumericp},
	{"alpha-char-p", builtinAlphaCharP},
	{"graphic-char-p", builtinGraphicCharP},
	{"standard-char-p", builtinStandardCharP},
	{"upper-case-p", builtinUpperCaseP},
	{"lower-case-p", builtinLowerCaseP},
	{"both-case-p", builtinBothCaseP},
	{"char-equal", builtinCharEqual},
	{"char-not-equal", builtinCharNotEqual},
	{"char-lessp", builtinCharLessp},
	{"char-greaterp", builtinCharGreaterp},
	{"char-not-lessp", builtinCharNotLessp},
	{"char-not-greaterp", builtinCharNotGreaterp},
	{"char/=", builtinCharNotEq},
	{"char<=", builtinCharLe},
	{"char>=", builtinCharGe},
	{"nreconc", builtinNreconc},
	{"maplist", builtinMaplist},
	{"mapc", builtinMapc},
	{"mapl", builtinMapl},
	{"mapcon", builtinMapcon},
	{"list*", builtinListStar},
	{"realpart", builtinRealpart},
	{"imagpart", builtinImagpart},
	{"conjugate", builtinConjugate},
	{"phase", builtinPhase},
	{"cis", builtinCis},
	{"complex", builtinComplex},
	{"integerp", builtinIntegerp},
	{"floatp", builtinFloatp},
	{"rationalp", builtinRationalp},
	{"realp", builtinRealp},
	{"complexp", builtinComplexp},
	{"float", builtinFloat},
	{"rational", builtinRational},
	{"numberp", builtinNumberP},
	{"numerator", builtinNumerator},
	{"denominator", builtinDenominator},
	{"ash", builtinAsh},
	{"logand", builtinLogand},
	{"logior", builtinLogior},
	{"logxor", builtinLogxor},
	{"lognot", builtinLognot},
	{"lognand", builtinLognand},
	{"lognor", builtinLognor},
	{"logandc1", builtinLogandc1},
	{"logandc2", builtinLogandc2},
	{"logorc1", builtinLogorc1},
	{"logorc2", builtinLogorc2},
	{"logcount", builtinLogcount},
	{"logbitp", builtinLogbitp},
	{"logtest", builtinLogtest},
	{"integer-length", builtinIntegerLength},
	{"byte", builtinByte},
	{"byte-size", builtinByteSize},
	{"byte-position", builtinBytePosition},
	{"ldb", builtinLdb},
	{"dpb", builtinDpb},
	{"ldb-test", builtinLdbTest},
	{"mask-field", builtinMaskField},
	{"deposit-field", builtinDepositField},
	{"boole", builtinBoole},
	{"coerce", builtinCoerce},
	// Pathname operations
	{"make-pathname", builtinMakePathname},
	{"pathname", builtinPathname},
	{"pathname-host", builtinPathnameHost},
	{"pathname-device", builtinPathnameDevice},
	{"pathname-directory", builtinPathnameDirectory},
	{"pathname-name", builtinPathnameName},
	{"pathname-type", builtinPathnameType},
	{"pathname-version", builtinPathnameVersion},
	{"namestring", builtinNamestring},
	{"parse-namestring", builtinParseNamestring},
	{"file-namestring", builtinFileNamestring},
	{"directory-namestring", builtinDirectoryNamestring},
	{"host-namestring", builtinHostNamestring},
	{"enough-namestring", builtinEnoughNamestring},
	{"merge-pathnames", builtinMergePathnames},
	{"pathnamep", builtinPathnamep},
	{"user-homedir-pathname", builtinUserHomedirPathname},
	{"probe-file", builtinProbeFile},
	{"delete-file", builtinDeleteFile},
	{"rename-file", builtinRenameFile},
	{"file-author", builtinFileAuthor},
	{"file-write-date", builtinFileWriteDate},
	{"ensure-directories-exist", builtinEnsureDirectoriesExist},
	{"directory-pathname-p", builtinDirectoryPathnameP},
	{"wild-pathname-p", builtinWildPathnameP},
	{"pathname-match-p", builtinPathnameMatchP},
	{"logical-pathname", builtinLogicalPathname},
	{"translate-pathname", builtinTranslatePathname},
	{"translate-logical-pathname", builtinTranslateLogicalPathname},
	{"logical-pathname-translations", builtinLogicalPathnameTranslations},
	{"logical-pathname-translations-setf", builtinLogicalPathnameTranslationsSetf},
	{"directory", builtinDirectory},
	{"file-length", builtinFileLength},
	{"file-position", builtinFilePosition},
	{"truename", builtinTruename},
	{"character", builtinCharacter},
	{"constantp", builtinConstantp},
	{"variable-information", builtinVariableInformation},
	{"function-information", builtinFunctionInformation},
	{"declaration-information", builtinDeclarationInformation},
}

// -------- Pathname helpers --------

func vpathname(p *LispPathname) *Value {
	v := gcv()
	v.typ = VPathname
	v.pathname = p
	return v
}


func vpkg(p *Package) *Value {
	v := gcv()
	v.typ = VPackage
	v.pkg = p
	return v
}

func isPackage(v *Value) bool {
	return v != nil && v.typ == VPackage
}

func builtinPackageP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	return vbool(isPackage(args[0])), nil
}

func builtinCompiledFunctionP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	v := args[0]
	// In microlisp, all built-in and defined functions are "compiled"
	return vbool(v.typ == VPrim || v.typ == VFunc || v.typ == VGeneric), nil
}

func vrt(rt *Readtable) *Value {
	v := gcv()
	v.typ = VReadtable
	v.readtable = rt
	return v
}

func isReadtable(v *Value) bool {
	return v != nil && v.typ == VReadtable
}

func getPathname(v *Value) *LispPathname {
	if v.typ == VPathname {
		return v.pathname
	}
	if v.typ == VStr {
		return parsePathnameString(v.str)
	}
	return nil
}

func builtinParseNamestring(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("parse-namestring: need a namestring")
	}
	v := args[0]
	if v.typ == VPathname {
		// Already a pathname
		return multiVal(v, vnum(0)), nil
	}
	if v.typ == VStr {
		pn := parsePathnameString(v.str)
		return multiVal(vpathname(pn), vnum(float64(len(v.str)))), nil
	}
	return nil, fmt.Errorf("parse-namestring: not a valid namestring")
}

func parsePathnameString(s string) *LispPathname {
	p := &LispPathname{version: "newest"}
	// Handle logical pathname (e.g., "SYS:FOO;BAR.LISP")
	// A logical host has multiple alphabetic characters before a colon,
	// unlike a Windows drive letter which is a single character.
	if colonIdx := strings.Index(s, ":"); colonIdx > 1 {
		// Multi-char host: treat as logical pathname
		p.host = strings.ToUpper(s[:colonIdx])
		s = s[colonIdx+1:]
		// Logical pathnames use semicolons as directory separators
		if len(s) > 0 && s[0] == ';' {
			p.directory = list(vsym(":absolute"))
			s = s[1:]
		} else {
			p.directory = list(vsym(":relative"))
		}
		// Split on semicolons (logical pathname directory separator)
		parts := []string{}
		start := 0
		for i := 0; i < len(s); i++ {
			if s[i] == ';' {
				if i > start {
					parts = append(parts, s[start:i])
				}
				start = i + 1
			}
		}
		// Last part may contain name.type
		last := s[start:]
		if len(parts) > 0 || len(last) > 0 {
			// Check if last part has a dot (extension separator)
			dotIdx := -1
			for j := len(last) - 1; j >= 0; j-- {
				if last[j] == '.' {
					dotIdx = j
					break
				}
			}
			if dotIdx > 0 {
				p.name = strings.ToUpper(last[:dotIdx])
				p.ftype = strings.ToUpper(last[dotIdx+1:])
			} else if dotIdx == 0 {
				p.ftype = strings.ToUpper(last[1:])
			} else {
				p.name = strings.ToUpper(last)
			}
		}
		// Build directory list
		dirList := p.directory
		for _, part := range parts {
			dirList = appendToList(dirList, vsym(strings.ToUpper(part)))
		}
		p.directory = dirList
		return p
	}
	// Handle Windows paths (e.g., C:/)
	if len(s) >= 2 && s[1] == ':' {
		p.device = s[:2]
		s = s[2:]
	}
	// Determine absolute vs relative
	if len(s) > 0 && (s[0] == '/' || s[0] == '\\') {
		p.directory = list(vsym(":absolute"))
		// Skip leading separators
		for len(s) > 0 && (s[0] == '/' || s[0] == '\\') {
			s = s[1:]
		}
	} else {
		p.directory = list(vsym(":relative"))
	}
	// Split directory components
	parts := []string{}
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '/' || s[i] == '\\' {
			if i > start {
				parts = append(parts, s[start:i])
			}
			start = i + 1
		}
	}
	// Last part may contain name.type
	last := s[start:]
	if len(parts) > 0 || len(last) > 0 {
		// Check if last part has a dot (extension separator)
		dotIdx := -1
		for j := len(last) - 1; j >= 0; j-- {
			if last[j] == '.' {
				dotIdx = j
				break
			}
		}
		if dotIdx > 0 {
			p.name = last[:dotIdx]
			p.ftype = last[dotIdx+1:]
		} else if dotIdx == 0 {
			p.ftype = last[1:]
		} else {
			p.name = last
		}
	}
	// Build directory list
	dirList := p.directory
	for _, part := range parts {
		dirList = appendToList(dirList, vsym(part))
	}
	p.directory = dirList
	return p
}

func pathnameToString(p *LispPathname) string {
	var b strings.Builder
	if p.device != "" {
		b.WriteString(p.device)
	}
	// Directory
	if p.directory != nil && !isNil(p.directory) {
		dir := p.directory
		if !isNil(dir) && dir.car != nil && dir.car.typ == VSym {
			if dir.car.str == ":ABSOLUTE" {
				b.WriteString("/")
			}
			dir = dir.cdr
		}
		for !isNil(dir) && dir.typ == VPair {
			if dir.car != nil && dir.car.typ == VSym {
				b.WriteString(dir.car.str)
				b.WriteString("/")
			} else if dir.car != nil && dir.car.typ == VStr {
				b.WriteString(dir.car.str)
				b.WriteString("/")
			}
			dir = dir.cdr
		}
	}
	// Name
	b.WriteString(p.name)
	// Type
	if p.ftype != "" {
		b.WriteString(".")
		b.WriteString(p.ftype)
	}
	return b.String()
}

func appendToList(lst *Value, elem *Value) *Value {
	if isNil(lst) {
		return cons(elem, vnil())
	}
	// Iterate to find last cons cell
	cur := lst
	for cur.typ == VPair && !isNil(cur.cdr) && cur.cdr.typ == VPair {
		cur = cur.cdr
	}
	cur.cdr = cons(elem, vnil())
	return lst
}

func builtinMakePathname(args []*Value) (*Value, error) {
	p := &LispPathname{version: "newest"}
	for i := 0; i+1 < len(args); i += 2 {
		key := args[i]
		val := args[i+1]
		if key.typ != VSym || len(key.str) == 0 || key.str[0] != ':' {
			continue
		}
		switch key.str[1:] {
		case "host":
			if val.typ == VStr {
				p.host = val.str
			}
		case "device":
			if val.typ == VStr {
				p.device = val.str
			}
		case "directory":
			p.directory = val
		case "name":
			if val.typ == VStr {
				p.name = val.str
			} else if val.typ == VSym {
				p.name = val.str
			}
		case "type":
			if val.typ == VStr {
				p.ftype = val.str
			} else if val.typ == VSym {
				p.ftype = val.str
			}
		case "version":
			if val.typ == VSym {
				p.version = val.str
			} else if val.typ == VStr {
				p.version = val.str
			}
		}
	}
	return vpathname(p), nil
}

func builtinPathname(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("pathname: need a pathname designator")
	}
	v := args[0]
	if v.typ == VPathname {
		return v, nil
	}
	if v.typ == VStr {
		return vpathname(parsePathnameString(v.str)), nil
	}
	if v.typ == VStream && v.stream != nil && v.stream.path != "" {
		return vpathname(parsePathnameString(v.stream.path)), nil
	}
	return nil, fmt.Errorf("pathname: cannot convert to pathname")
}

func builtinPathnameHost(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	p := getPathname(args[0])
	if p == nil || p.host == "" {
		return vnil(), nil
	}
	return vstr(p.host), nil
}

func builtinPathnameDevice(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	p := getPathname(args[0])
	if p == nil || p.device == "" {
		return vnil(), nil
	}
	return vstr(p.device), nil
}

func builtinPathnameDirectory(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	p := getPathname(args[0])
	if p == nil || p.directory == nil {
		return vnil(), nil
	}
	return p.directory, nil
}

func builtinPathnameName(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	p := getPathname(args[0])
	if p == nil || p.name == "" {
		return vnil(), nil
	}
	return vstr(p.name), nil
}

func builtinPathnameType(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	p := getPathname(args[0])
	if p == nil || p.ftype == "" {
		return vnil(), nil
	}
	return vstr(p.ftype), nil
}

func builtinPathnameVersion(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	p := getPathname(args[0])
	if p == nil {
		return vsym(":newest"), nil
	}
	return vsym(p.version), nil
}

func builtinNamestring(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vstr(""), nil
	}
	p := getPathname(args[0])
	if p == nil {
		return vstr(""), nil
	}
	return vstr(pathnameToString(p)), nil
}

func builtinFileNamestring(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vstr(""), nil
	}
	p := getPathname(args[0])
	if p == nil {
		return vstr(""), nil
	}
	var b strings.Builder
	b.WriteString(p.name)
	if p.ftype != "" {
		b.WriteString(".")
		b.WriteString(p.ftype)
	}
	return vstr(b.String()), nil
}

func builtinDirectoryNamestring(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vstr(""), nil
	}
	p := getPathname(args[0])
	if p == nil {
		return vstr(""), nil
	}
	var b strings.Builder
	if p.device != "" {
		b.WriteString(p.device)
	}
	if p.directory != nil && !isNil(p.directory) {
		dir := p.directory
		if !isNil(dir) && dir.car != nil && dir.car.typ == VSym {
			if dir.car.str == ":ABSOLUTE" {
				b.WriteString("/")
			}
			dir = dir.cdr
		}
		for !isNil(dir) && dir.typ == VPair {
			if dir.car != nil && dir.car.typ == VSym {
				b.WriteString(dir.car.str)
				b.WriteString("/")
			} else if dir.car != nil && dir.car.typ == VStr {
				b.WriteString(dir.car.str)
				b.WriteString("/")
			}
			dir = dir.cdr
		}
	}
	return vstr(b.String()), nil
}

func builtinHostNamestring(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vstr(""), nil
	}
	p := getPathname(args[0])
	if p == nil || p.host == "" {
		return vstr(""), nil
	}
	return vstr(p.host), nil
}

func builtinEnoughNamestring(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vstr(""), nil
	}
	p := getPathname(args[0])
	if p == nil {
		return vstr(""), nil
	}
	var b strings.Builder
	b.WriteString(p.name)
	if p.ftype != "" {
		b.WriteString(".")
		b.WriteString(p.ftype)
	}
	return vstr(b.String()), nil
}

func builtinMergePathnames(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("merge-pathnames: need a pathname")
	}
	p := getPathname(args[0])
	if p == nil {
		p = &LispPathname{version: "newest"}
	}
	var dp *LispPathname
	if len(args) >= 2 {
		dp = getPathname(args[1])
	}
	if dp == nil {
		dp = &LispPathname{version: "newest"}
	}
	result := &LispPathname{version: "newest"}
	result.host = p.host
	if result.host == "" {
		result.host = dp.host
	}
	result.device = p.device
	if result.device == "" {
		result.device = dp.device
	}
	result.directory = p.directory
	if result.directory == nil || isNil(result.directory) {
		result.directory = dp.directory
	}
	result.name = p.name
	if result.name == "" {
		result.name = dp.name
	}
	result.ftype = p.ftype
	if result.ftype == "" {
		result.ftype = dp.ftype
	}
	result.version = p.version
	if result.version == "" {
		result.version = dp.version
	}
	return vpathname(result), nil
}

func builtinUserHomedirPathname(args []*Value) (*Value, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "/"
	}
	// Split home path into directory components
	parts := strings.Split(strings.TrimRight(home, "/\\"), string(os.PathSeparator))
	dirList := vnil()
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] != "" {
			dirList = cons(vstr(parts[i]), dirList)
		}
	}
	dirList = cons(vsym(":ABSOLUTE"), dirList)
	p := &LispPathname{
		directory: dirList,
		name:      "",
		ftype:     "",
	}
	return vpathname(p), nil
}

func builtinPathnamep(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	return vbool(args[0].typ == VPathname), nil
}

func builtinProbeFile(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("probe-file: need a pathname")
	}
	p := getPathname(args[0])
	if p == nil {
		return vnil(), nil
	}
	path := pathnameToString(p)
	if _, err := os.Stat(path); err == nil {
		return vpathname(p), nil
	}
	return vnil(), nil
}

func builtinDirectory(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	p := getPathname(args[0])
	if p == nil {
		return vnil(), nil
	}
	path := pathnameToString(p)
	matches, err := filepath.Glob(path)
	if err != nil {
		return vnil(), nil
	}
	result := vnil()
	for i := len(matches) - 1; i >= 0; i-- {
		pp := parsePathnameString(matches[i])
		result = cons(vpathname(pp), result)
	}
	return result, nil
}

func builtinFileLength(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("file-length: need a stream")
	}
	v := args[0]
	if v.typ == VStream && v.stream != nil && v.stream.file != nil {
		fi, err := v.stream.file.Stat()
		if err != nil {
			return vnil(), nil
		}
		return vnum(float64(fi.Size())), nil
	}
	return vnil(), nil
}

func builtinFilePosition(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("file-position: need a stream")
	}
	v := args[0]
	if v.typ != VStream || v.stream == nil {
		return vnil(), nil
	}
	if len(args) < 2 {
		if v.stream.file != nil {
			pos, err := v.stream.file.Seek(0, 1)
			if err != nil {
				return vnil(), nil
			}
			return vnum(float64(pos)), nil
		}
		return vnil(), nil
	}
	pos := int64(toNum(args[1]))
	if v.stream.file != nil {
		_, err := v.stream.file.Seek(pos, 0)
		if err != nil {
			return vnil(), nil
		}
		return vnum(float64(pos)), nil
	}
	return vnil(), nil
}

func builtinTruename(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("truename: need a pathname")
	}
	p := getPathname(args[0])
	if p == nil {
		return vnil(), nil
	}
	path := pathnameToString(p)
	abs, err := filepath.Abs(path)
	if err != nil {
		return vnil(), nil
	}
	return vpathname(parsePathnameString(abs)), nil
}

// -------- Additional pathname functions --------

// directory-pathname-p returns true if pathname has no name/type/version
func builtinDirectoryPathnameP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	p := getPathname(args[0])
	if p == nil {
		return vbool(false), nil
	}
	return vbool(p.name == "" && p.ftype == "" && p.version == ""), nil
}

// wild-pathname-p checks if pathname contains wildcard components
func builtinWildPathnameP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	p := getPathname(args[0])
	if p == nil {
		return vbool(false), nil
	}
	// Check for :wild in components, or * in name/type
	if p.name == "*" || p.ftype == "*" || p.version == "*" {
		return vbool(true), nil
	}
	if p.directory != nil && !isNil(p.directory) {
		for d := p.directory; !isNil(d) && d.typ == VPair; d = d.cdr {
			if d.car != nil && d.car.typ == VSym && d.car.str == ":WILD" {
				return vbool(true), nil
			}
			if d.car != nil && d.car.typ == VStr && d.car.str == "*" {
				return vbool(true), nil
			}
		}
	}
	return vbool(false), nil
}

// pathname-match-p checks if pathname matches a wildcard pattern
func builtinPathnameMatchP(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return vbool(false), nil
	}
	pathP := getPathname(args[0])
	patternP := getPathname(args[1])
	if pathP == nil || patternP == nil {
		return vbool(false), nil
	}
	// Simple matching: * matches anything, otherwise exact match
	if patternP.name == "*" || patternP.name == pathP.name {
		if patternP.ftype == "*" || patternP.ftype == pathP.ftype {
			return vbool(true), nil
		}
	}
	return vbool(false), nil
}

// file-author returns the author of a file
func builtinFileAuthor(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("file-author: need a pathname")
	}
	p := getPathname(args[0])
	if p == nil {
		return vnil(), nil
	}
	path := pathnameToString(p)
	_, err := os.Stat(path)
	if err != nil {
		return vnil(), nil
	}
	// Go doesn't natively support file author on all platforms; return nil
	return vnil(), nil
}

// file-write-date returns the modification time as universal time
func builtinFileWriteDate(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("file-write-date: need a pathname")
	}
	p := getPathname(args[0])
	if p == nil {
		return vnil(), nil
	}
	path := pathnameToString(p)
	info, err := os.Stat(path)
	if err != nil {
		return vnil(), nil
	}
	// Convert to Unix timestamp (Lisp universal time is seconds since 1900-01-01)
	modTime := info.ModTime().Unix()
	// Universal time epoch: 1900-01-01 00:00:00 UTC
	unixEpoch := int64(2208988800) // seconds from 1900 to 1970
	return vnum(float64(modTime + unixEpoch)), nil
}

// builtinRenameFile renames a file
func builtinRenameFile(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("rename-file: need old and new pathname")
	}
	oldP := getPathname(args[0])
	newP := getPathname(args[1])
	if oldP == nil || newP == nil {
		return vnil(), nil
	}
	oldPath := pathnameToString(oldP)
	newPath := pathnameToString(newP)
	err := os.Rename(oldPath, newPath)
	if err != nil {
		return vnil(), nil
	}
	oldAbs, _ := filepath.Abs(oldPath)
	newAbs, _ := filepath.Abs(newPath)
	return list(vpathname(parsePathnameString(newAbs)), vpathname(parsePathnameString(newAbs)), vpathname(parsePathnameString(oldAbs))), nil
}

// builtinDeleteFile deletes a file
func builtinDeleteFile(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("delete-file: need a pathname")
	}
	p := getPathname(args[0])
	if p == nil {
		return vbool(false), nil
	}
	path := pathnameToString(p)
	err := os.Remove(path)
	if err != nil {
		return vbool(false), nil
	}
	return vbool(true), nil
}

// ensure-directories-exist ensures parent directories exist for a pathname
func builtinEnsureDirectoriesExist(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("ensure-directories-exist: need a pathname")
	}
	p := getPathname(args[0])
	if p == nil {
		return vnil(), nil
	}
	path := pathnameToString(p)
	dir := filepath.Dir(path)
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return vbool(false), nil
	}
	return vpathname(p), nil
}

// logical-pathname translates a logical pathname to physical
func builtinLogicalPathname(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("logical-pathname: need a pathname")
	}
	v := args[0]
	var pathStr string
	if v.typ == VStr {
		pathStr = v.str
	} else if v.typ == VPathname {
		pathStr = pathnameToString(v.pathname)
	}
	// Treat as physical and return as-is for now
	return vpathname(parsePathnameString(pathStr)), nil
}

func builtinTranslatePathname(args []*Value) (*Value, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("translate-pathname: need source from-wildcard to-wildcard")
	}
	source := getPathname(args[0])
	fromP := getPathname(args[1])
	toP := getPathname(args[2])
	if source == nil || fromP == nil || toP == nil {
		return nil, fmt.Errorf("translate-pathname: all arguments must be pathnames")
	}
	// Translate each component: substitute from-wildcard matches into to-wildcard template
	result := &LispPathname{version: "newest"}
	result.host = translateComponent(source.host, fromP.host, toP.host)
	result.device = translateComponent(source.device, fromP.device, toP.device)
	result.name = translateComponent(source.name, fromP.name, toP.name)
	result.ftype = translateComponent(source.ftype, fromP.ftype, toP.ftype)
	// For directory, just copy from source if from matches
	result.directory = source.directory
	return vpathname(result), nil
}

func translateComponent(source, from, to string) string {
	if from == "*" || from == "" {
		if to == "*" {
			return source
		}
		return to
	}
	if source == from {
		return to
	}
	// Simple wildcard matching: if from is "*", replace with source in to
	if strings.Contains(from, "*") && to != "" {
		// Replace first wildcard in 'to' with the matched portion of source
		parts := strings.SplitN(from, "*", 2)
		if strings.HasPrefix(source, parts[0]) {
			matched := strings.TrimPrefix(source, parts[0])
			if len(parts) > 1 && parts[1] != "" {
				matched = strings.TrimSuffix(matched, parts[1])
			}
			return strings.Replace(to, "*", matched, 1)
		}
	}
	if to == "" {
		return source
	}
	return to
}

func builtinTranslateLogicalPathname(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("translate-logical-pathname: need a pathname")
	}
	p := getPathname(args[0])
	if p == nil {
		return nil, fmt.Errorf("translate-logical-pathname: need a pathname")
	}
	// If the pathname has a logical host, look up translations
	if p.host != "" {
		translations := logicalPathnameTranslations[strings.ToUpper(p.host)]
		if translations != nil && !isNil(translations) {
			// Apply each translation rule
			cur := translations
			for !isNil(cur) && cur.typ == VPair {
				rule := cur.car
				if rule.typ == VPair && !isNil(rule) {
					fromP := getPathname(rule.car)
					toP := getPathname(rule.cdr.car)
					if fromP != nil && toP != nil {
						// Check if source matches from pattern
						if pathnameMatchesPattern(p, fromP) {
							return builtinTranslatePathname([]*Value{vpathname(p), vpathname(fromP), vpathname(toP)})
						}
					}
				}
				cur = cur.cdr
			}
		}
	}
	// Not a logical pathname or no translation found; return as-is
	return vpathname(p), nil
}

func pathnameMatchesPattern(source, pattern *LispPathname) bool {
	if pattern.host != "" && pattern.host != "*" && pattern.host != source.host {
		return false
	}
	if pattern.name != "" && pattern.name != "*" && pattern.name != source.name {
		return false
	}
	if pattern.ftype != "" && pattern.ftype != "*" && pattern.ftype != source.ftype {
		return false
	}
	return true
}

func builtinLogicalPathnameTranslations(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("logical-pathname-translations: need a host")
	}
	host := strings.ToUpper(ToString(args[0]))
	if len(args) >= 2 {
		// SETF form: set the translations
		logicalPathnameTranslations[host] = args[1]
		return args[1], nil
	}
	// Get the translations
	t, ok := logicalPathnameTranslations[host]
	if !ok {
		return vnil(), nil
	}
	return t, nil
}

func builtinLogicalPathnameTranslationsSetf(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("logical-pathname-translations-setf: need new-value and host")
	}
	host := strings.ToUpper(ToString(args[1]))
	logicalPathnameTranslations[host] = args[0]
	return args[0], nil
}

func toNum(v *Value) float64 {
	switch v.typ {
	case VNum:
		return v.num
	case VRat:
		return float64(v.irat) / float64(v.iden)
	case VComplex:
		return v.num
	case VBigInt:
		f, _ := new(big.Float).SetInt(v.bigInt).Float64()
		return f
	}
	return 0
}

// isNumeric returns true if v is a numeric type
func isNumeric(v *Value) bool {
	return v.typ == VNum || v.typ == VRat || v.typ == VComplex || v.typ == VBigInt
}

// toRatParts extracts rational form. isInt indicates VNum with integer value.
func toRatParts(v *Value) (n, d int64, isInt bool) {
	switch v.typ {
	case VRat:
		return v.irat, v.iden, true
	case VNum:
		if v.num == math.Trunc(v.num) && !math.IsInf(v.num, 0) && v.num >= -9e15 && v.num <= 9e15 {
			return int64(v.num), 1, true
		}
	case VBigInt:
		if v.bigInt.IsInt64() {
			return v.bigInt.Int64(), 1, true
		}
	}
	return 0, 0, false
}

// toComplexParts extracts real and imaginary parts from any numeric value.
func toComplexParts(v *Value) (r, i float64) {
	switch v.typ {
	case VComplex:
		return v.num, v.imag
	case VRat:
		return float64(v.irat) / float64(v.iden), 0
	default:
		return toNum(v), 0
	}
}

// needComplex checks if any arg is a complex number.
func needComplex(args []*Value) bool {
	for _, a := range args {
		if a.typ == VComplex {
			return true
		}
	}
	return false
}

// needRat checks if any arg is a rational (and none are complex).
func needRat(args []*Value) bool {
	for _, a := range args {
		if a.typ == VRat || a.typ == VBigInt {
			return true
		}
	}
	return false
}

// isBigIntInt checks if any arg is a VBigInt.
func isBigIntInt(args []*Value) bool {
	for _, a := range args {
		if a.typ == VBigInt {
			return true
		}
	}
	return false
}

// toBigInt converts a Value to *big.Int (0 if not integer).
func toBigInt(v *Value) *big.Int {
	switch v.typ {
	case VNum:
		if v.num == math.Trunc(v.num) && !math.IsInf(v.num, 0) {
			return big.NewInt(int64(v.num))
		}
	case VRat:
		if v.iden == 1 {
			return big.NewInt(v.irat)
		}
	case VBigInt:
		return new(big.Int).Set(v.bigInt)
	}
	return nil
}

// toBigIntExact converts a Value to *big.Int if it is an exact integer.
// Returns nil for non-integer types (float, rational, complex).
func toBigIntExact(v *Value) *big.Int {
	switch v.typ {
	case VNum:
		if !v.isFloat && v.num == math.Trunc(v.num) && !math.IsInf(v.num, 0) {
			return big.NewInt(int64(v.num))
		}
	case VRat:
		if v.iden == 1 {
			return big.NewInt(v.irat)
		}
	case VBigInt:
		return new(big.Int).Set(v.bigInt)
	}
	return nil
}

// toBigRat converts a Value to big.Rat for exact rational comparison.
func toBigRat(v *Value) big.Rat {
	switch v.typ {
	case VNum:
		if v.isFloat {
			return *new(big.Rat).SetFloat64(v.num)
		}
		return *big.NewRat(int64(v.num), 1)
	case VRat:
		return *big.NewRat(v.irat, v.iden)
	case VBigInt:
		r := new(big.Rat).SetInt(v.bigInt)
		return *r
	}
	return *big.NewRat(0, 1)
}

// compareNumeric returns -1 if a < b, 0 if a == b, 1 if a > b.
func compareNumeric(a, b *Value) int {
	// Handle complex numbers specially - need to compare both parts
	if a.typ == VComplex || b.typ == VComplex {
		aReal, aImag := toComplexParts(a)
		bReal, bImag := toComplexParts(b)
		if aReal < bReal {
			return -1
		}
		if aReal > bReal {
			return 1
		}
		if aImag < bImag {
			return -1
		}
		if aImag > bImag {
			return 1
		}
		return 0
	}
	// Use big.Int for exact comparison when either operand is VBigInt
	aBi := toBigIntExact(a)
	bBi := toBigIntExact(b)
	if aBi != nil && bBi != nil {
		return aBi.Cmp(bBi)
	}
	// Use big.Rat for exact comparison when either operand is VRat
	if a.typ == VRat || b.typ == VRat {
		aRat := toBigRat(a)
		bRat := toBigRat(b)
		return aRat.Cmp(&bRat)
	}
	// Fall back to float64 comparison
	aReal, _ := toComplexParts(a)
	bReal, _ := toComplexParts(b)
	if aReal < bReal {
		return -1
	}
	if aReal > bReal {
		return 1
	}
	return 0
}

func builtinAdd(args []*Value) (*Value, error) {
	if len(args) == 0 {
		return vnum(0), nil
	}
	// Type check: all args must be numeric
	for _, a := range args {
		if !isNumeric(a) {
			return nil, fmt.Errorf("+: not a number: %s", ToString(a))
		}
	}
	if needComplex(args) {
		r, i := 0.0, 0.0
		for _, a := range args {
			ar, ai := toComplexParts(a)
			r += ar
			i += ai
		}
		return vcomplex(r, i), nil
	}
	// Try big.Int if any arg is VBigInt or if rational arithmetic might overflow
	if isBigIntInt(args) {
		result := new(big.Int)
		for _, a := range args {
			bi := toBigInt(a)
			if bi != nil {
				result.Add(result, bi)
				continue
			}
			// Not an exact integer - fall back to float
			f, _ := new(big.Float).SetInt(result).Float64()
			for _, a2 := range args {
				f += toNum(a2)
			}
			return vfloat(f), nil
		}
		return vbigint(result), nil
	}
	if needRat(args) {
		// Track as rational
		n, d := int64(0), int64(1)
		hasFloat := false
		for _, a := range args {
			an, ad, isInt := toRatParts(a)
			if !isInt {
				hasFloat = true
				break
			}
			n = n*ad + an*d
			d = d * ad
			g := gcd(n, d)
			if g < 0 {
				g = -g
			}
			n /= g
			d /= g
		}
		if hasFloat {
			r := 0.0
			for _, a := range args {
				r += toNum(a)
			}
			return vfloat(r), nil
		}
		return vrat(n, d), nil
	}
	r := 0.0
	for _, a := range args {
		r += toNum(a)
	}
	return numOrFloat(r, args), nil
}

func builtinSub(args []*Value) (*Value, error) {
	if len(args) == 0 {
		return vnum(0), nil
	}
	for _, a := range args {
		if !isNumeric(a) {
			return nil, fmt.Errorf("-: not a number: %s", ToString(a))
		}
	}
	if needComplex(args) {
		ar, ai := toComplexParts(args[0])
		if len(args) == 1 {
			return vcomplex(-ar, -ai), nil
		}
		for _, a := range args[1:] {
			br, bi := toComplexParts(a)
			ar -= br
			ai -= bi
		}
		return vcomplex(ar, ai), nil
	}
	if isBigIntInt(args) {
		result := toBigInt(args[0])
		if result == nil {
			f := toNum(args[0])
			for _, a := range args[1:] {
				f -= toNum(a)
			}
			return vfloat(f), nil
		}
		for _, a := range args[1:] {
			bi := toBigInt(a)
			if bi != nil {
				result.Sub(result, bi)
			} else {
				f, _ := new(big.Float).SetInt(result).Float64()
				for _, a2 := range args {
					f -= toNum(a2)
				}
				return vfloat(f), nil
			}
		}
		return vbigint(result), nil
	}
	if len(args) == 1 {
		if args[0].typ == VRat {
			return vrat(-args[0].irat, args[0].iden), nil
		}
		return vnum(-toNum(args[0])), nil
	}
	if needRat(args) {
		n0, d0, isInt0 := toRatParts(args[0])
		hasFloat := !isInt0
		for _, a := range args[1:] {
			an, ad, isInt := toRatParts(a)
			if !isInt {
				hasFloat = true
				break
			}
			n0 = n0*ad - an*d0
			d0 = d0 * ad
			g := gcd(n0, d0)
			if g < 0 {
				g = -g
			}
			n0 /= g
			d0 /= g
		}
		if hasFloat {
			r := toNum(args[0])
			for _, a := range args[1:] {
				r -= toNum(a)
			}
			return vfloat(r), nil
		}
		return vrat(n0, d0), nil
	}
	r := toNum(args[0])
	for _, a := range args[1:] {
		r -= toNum(a)
	}
	return numOrFloat(r, args), nil
}

func builtinMul(args []*Value) (*Value, error) {
	if len(args) == 0 {
		return vnum(1), nil
	}
	for _, a := range args {
		if !isNumeric(a) {
			return nil, fmt.Errorf("*: not a number: %s", ToString(a))
		}
	}
	if needComplex(args) {
		r, i := 1.0, 0.0 // start with 1+0i
		for _, a := range args {
			ar, ai := toComplexParts(a)
			// (r + i*i) * (ar + ai*i) = (r*ar - i*ai) + (r*ai + i*ar)*i
			newR := r*ar - i*ai
			newI := r*ai + i*ar
			r, i = newR, newI
		}
		return vcomplex(r, i), nil
	}
	if isBigIntInt(args) {
		result := big.NewInt(1)
		for _, a := range args {
			bi := toBigInt(a)
			if bi != nil {
				result.Mul(result, bi)
				continue
			}
			f, _ := new(big.Float).SetInt(result).Float64()
			for _, a2 := range args {
				f *= toNum(a2)
			}
			return vfloat(f), nil
		}
		return vbigint(result), nil
	}
	if needRat(args) {
		n, d := int64(1), int64(1)
		hasFloat := false
		for _, a := range args {
			an, ad, isInt := toRatParts(a)
			if !isInt {
				hasFloat = true
				break
			}
			n *= an
			d *= ad
			g := gcd(n, d)
			if g < 0 {
				g = -g
			}
			n /= g
			d /= g
		}
		if hasFloat {
			r := 1.0
			for _, a := range args {
				r *= toNum(a)
			}
			return vfloat(r), nil
		}
		return vrat(n, d), nil
	}
	// All args are VNum — check if they are all integer-valued
	// If so, use big.Int to avoid overflow
	allInt := true
	for _, a := range args {
		if a.typ != VNum || a.num != math.Trunc(a.num) || math.IsInf(a.num, 0) {
			allInt = false
			break
		}
	}
	if allInt {
		result := big.NewInt(1)
		for _, a := range args {
			bi := big.NewInt(int64(a.num))
			result.Mul(result, bi)
		}
		return vbigint(result), nil
	}
	r := 1.0
	for _, a := range args {
		r *= toNum(a)
	}
	return numOrFloat(r, args), nil
}

func builtinDiv(args []*Value) (*Value, error) {
	if len(args) == 0 {
		return vnum(1), nil
	}
	for _, a := range args {
		if !isNumeric(a) {
			return nil, fmt.Errorf("/: not a number: %s", ToString(a))
		}
	}
	if needComplex(args) {
		ar, ai := toComplexParts(args[0])
		if len(args) == 1 {
			// 1 / (ar + ai*i) = ar/(ar²+ai²) - ai/(ar²+ai²)*i
			den := ar*ar + ai*ai
			if den == 0 {
				return nil, signalDivisionByZero()
			}
			return vcomplex(ar/den, -ai/den), nil
		}
		for _, a := range args[1:] {
			br, bi := toComplexParts(a)
			den := br*br + bi*bi
			if den == 0 {
				return nil, signalDivisionByZero()
			}
			// (ar + ai*i) / (br + bi*i) = (ar*br + ai*bi)/den + (ai*br - ar*bi)/den * i
			newR := (ar*br + ai*bi) / den
			newI := (ai*br - ar*bi) / den
			ar, ai = newR, newI
		}
		return vcomplex(ar, ai), nil
	}
	if len(args) == 1 {
		if args[0].typ == VBigInt {
			if args[0].bigInt.Sign() == 0 {
				return nil, signalDivisionByZero()
			}
			return vnum(1.0 / toNum(args[0])), nil
		}
		if args[0].typ == VRat {
			if args[0].irat == 0 {
				return nil, signalDivisionByZero()
			}
			// 1 / (a/b) = b/a
			n := args[0].iden
			d := args[0].irat
			if d < 0 {
				n = -n
				d = -d
			}
			return vrat(n, d), nil
		}
		if toNum(args[0]) == 0 {
			return nil, signalDivisionByZero()
		}
		return vnum(1.0 / toNum(args[0])), nil
	}
	if isBigIntInt(args) {
		num := toBigInt(args[0])
		den := big.NewInt(1)
		if num == nil {
			r := toNum(args[0])
			for _, a := range args[1:] {
				if toNum(a) == 0 {
					return nil, signalDivisionByZero()
				}
				r /= toNum(a)
			}
			return vfloat(r), nil
		}
		for _, a := range args[1:] {
			bi := toBigInt(a)
			if bi == nil {
				r := toNum(args[0])
				for _, a2 := range args[1:] {
					if toNum(a2) == 0 {
						return nil, signalDivisionByZero()
					}
					r /= toNum(a2)
				}
				return vfloat(r), nil
			}
			if bi.Sign() == 0 {
				return nil, signalDivisionByZero()
			}
			den.Mul(den, bi)
		}
		g := new(big.Int).GCD(nil, nil, num, den)
		if g.Sign() != 0 {
			num.Quo(num, g)
			den.Quo(den, g)
		}
		if den.Sign() < 0 {
			num.Neg(num)
			den.Neg(den)
		}
		if den.IsInt64() && den.Int64() == 1 {
			return vbigint(num), nil
		}
		// Result is not an integer: try to reduce to int64 rational
		if num.IsInt64() && den.IsInt64() {
			return vrat(num.Int64(), den.Int64()), nil
		}
		// Fallback: return as float
		f, _ := new(big.Float).Quo(
			new(big.Float).SetInt(num),
			new(big.Float).SetInt(den),
		).Float64()
		return vfloat(f), nil
	}
	if needRat(args) {
		n0, d0, isInt0 := toRatParts(args[0])
		hasFloat := !isInt0
		for _, a := range args[1:] {
			an, ad, isInt := toRatParts(a)
			if !isInt {
				hasFloat = true
				break
			}
			if an == 0 {
				return nil, signalDivisionByZero()
			}
			// (n0/d0) / (an/ad) = n0*ad / (d0*an)
			n0 *= ad
			d0 *= an
			if d0 < 0 {
				n0 = -n0
				d0 = -d0
			}
			g := gcd(n0, d0)
			if g < 0 {
				g = -g
			}
			n0 /= g
			d0 /= g
		}
		if hasFloat {
			r := toNum(args[0])
			for _, a := range args[1:] {
				if toNum(a) == 0 {
					return nil, signalDivisionByZero()
				}
				r /= toNum(a)
			}
			return vfloat(r), nil
		}
		return vrat(n0, d0), nil
	}
	// If all args are integers (not floats), use rational division
	allInt := true
	for _, a := range args {
		if a.typ == VNum && a.isFloat {
			allInt = false
			break
		}
	}
	if allInt {
		n0, d0 := int64(toNum(args[0])), int64(1)
		for _, a := range args[1:] {
			an := int64(toNum(a))
			if an == 0 {
				return nil, signalDivisionByZero()
			}
			n0 *= an
			d0 *= 1
		}
		// Simplify: n0/d0 where d0 is the product of all denominators
		// Actually we need n0 / (product of all args[1:])
		// Redo: numerator = args[0], denominator = product of args[1:]
		num := int64(toNum(args[0]))
		den := int64(1)
		for _, a := range args[1:] {
			den *= int64(toNum(a))
		}
		if den == 0 {
			return nil, signalDivisionByZero()
		}
		return vrat(num, den), nil
	}

	r := toNum(args[0])
	for _, a := range args[1:] {
		if toNum(a) == 0 {
			return nil, signalDivisionByZero()
		}
		r /= toNum(a)
	}
	return numOrFloat(r, args), nil
}

func builtinEq(args []*Value) (*Value, error) {
	if len(args) == 0 {
		return vbool(true), nil
	}
	for _, a := range args {
		if !isNumber(a) {
			return signalTypeError(a)
		}
	}
	if len(args) == 1 {
		return vbool(true), nil
	}
	for i := 1; i < len(args); i++ {
		if compareNumeric(args[i-1], args[i]) != 0 {
			return vbool(false), nil
		}
	}
	return vbool(true), nil
}

func builtinNe(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return vbool(false), nil
	}
	for i := 0; i < len(args); i++ {
		for j := i + 1; j < len(args); j++ {
			if compareNumeric(args[i], args[j]) == 0 {
				return vbool(false), nil
			}
		}
	}
	return vbool(true), nil
}

func builtinLt(args []*Value) (*Value, error) {
	for _, a := range args {
		if !isReal(a) {
			return signalTypeError(a)
		}
	}
	if len(args) < 2 {
		return vbool(true), nil
	}
	for i := 1; i < len(args); i++ {
		if compareNumeric(args[i-1], args[i]) >= 0 {
			return vbool(false), nil
		}
	}
	return vbool(true), nil
}

func builtinGt(args []*Value) (*Value, error) {
	for _, a := range args {
		if !isReal(a) {
			return signalTypeError(a)
		}
	}
	if len(args) < 2 {
		return vbool(true), nil
	}
	for i := 1; i < len(args); i++ {
		if compareNumeric(args[i-1], args[i]) <= 0 {
			return vbool(false), nil
		}
	}
	return vbool(true), nil
}

func builtinLe(args []*Value) (*Value, error) {
	for _, a := range args {
		if !isReal(a) {
			return signalTypeError(a)
		}
	}
	if len(args) < 2 {
		return vbool(true), nil
	}
	for i := 1; i < len(args); i++ {
		if compareNumeric(args[i-1], args[i]) > 0 {
			return vbool(false), nil
		}
	}
	return vbool(true), nil
}

func builtinGe(args []*Value) (*Value, error) {
	for _, a := range args {
		if !isReal(a) {
			return signalTypeError(a)
		}
	}
	if len(args) < 2 {
		return vbool(true), nil
	}
	for i := 1; i < len(args); i++ {
		if compareNumeric(args[i-1], args[i]) < 0 {
			return vbool(false), nil
		}
	}
	return vbool(true), nil
}

func builtinCons(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("cons: need 2 arguments")
	}
	return cons(args[0], args[1]), nil
}

func builtinCar(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("car: need 1 argument")
	}
	v := args[0]
	if v != nil && v.typ == VMultiVal {
		return primaryValue(v), nil
	}
	if isNil(v) {
		return vnil(), nil
	}
	if !isPair(v) {
		return nil, fmt.Errorf("car: not a pair")
	}
	return v.car, nil
}

func builtinCdr(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("cdr: need 1 argument")
	}
	v := args[0]
	if v != nil && v.typ == VMultiVal {
		v = primaryValue(v)
	}
	if isNil(v) {
		return vnil(), nil
	}
	if !isPair(v) {
		return nil, fmt.Errorf("cdr: not a pair")
	}
	return v.cdr, nil
}

func builtinSetCar(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("set-car!: need pair and value")
	}
	if !isPair(args[0]) {
		return nil, fmt.Errorf("set-car!: not a pair")
	}
	args[0].car = args[1]
	return args[1], nil
}

// builtinSet implements CL's (set symbol value) - sets the dynamic value of a symbol
func builtinSet(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("set: need symbol and value")
	}
	sym := args[0]
	if sym.typ != VSym {
		return nil, fmt.Errorf("set: first argument must be a symbol")
	}
	val := args[1]
	globalEnv.Set(sym.str, val)
	return val, nil
}

func builtinSetCdr(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("set-cdr!: need pair and value")
	}
	if !isPair(args[0]) {
		return nil, fmt.Errorf("set-cdr!: not a pair")
	}
	args[0].cdr = args[1]
	return args[1], nil
}

// builtinSetCar is used for (setf (car x) val) -> (car-setf val x)
func builtinSetCarAsSetter(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("setf (car): need 2 arguments")
	}
	val := args[0]
	cons := args[1]
	if !isPair(cons) {
		return nil, fmt.Errorf("setf (car): not a pair")
	}
	cons.car = val
	return val, nil
}

// builtinSetCdrAsSetter is used for (setf (cdr x) val) -> (cdr-setf val x)
func builtinSetCdrAsSetter(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("setf (cdr): need 2 arguments")
	}
	val := args[0]
	cons := args[1]
	if !isPair(cons) {
		return nil, fmt.Errorf("setf (cdr): not a pair")
	}
	cons.cdr = val
	return val, nil
}

func builtinList(args []*Value) (*Value, error) {
	return listFromSlice(args), nil
}

// -------- List accessors --------
func builtinRest(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	if args[0] == nil || args[0].typ != VPair {
		return vnil(), nil
	}
	return args[0].cdr, nil
}
func builtinFirst(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	v := args[0]
	if v.typ == VPair {
		return v.car, nil
	}
	return vnil(), nil
}
func builtinSecond(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	v := args[0]
	for i := 0; i < 1; i++ {
		if v == nil || v.typ != VPair {
			return vnil(), nil
		}
		v = v.cdr
	}
	if v != nil && v.typ == VPair {
		return v.car, nil
	}
	return vnil(), nil
}
func builtinThird(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	v := args[0]
	for i := 0; i < 2; i++ {
		if v == nil || v.typ != VPair {
			return vnil(), nil
		}
		v = v.cdr
	}
	if v != nil && v.typ == VPair {
		return v.car, nil
	}
	return vnil(), nil
}
func builtinFourth(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	v := args[0]
	for i := 0; i < 3; i++ {
		if v == nil || v.typ != VPair {
			return vnil(), nil
		}
		v = v.cdr
	}
	if v != nil && v.typ == VPair {
		return v.car, nil
	}
	return vnil(), nil
}
func builtinFifth(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	v := args[0]
	for i := 0; i < 4; i++ {
		if v == nil || v.typ != VPair {
			return vnil(), nil
		}
		v = v.cdr
	}
	if v != nil && v.typ == VPair {
		return v.car, nil
	}
	return vnil(), nil
}
func builtinSixth(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	v := args[0]
	for i := 0; i < 5; i++ {
		if v == nil || v.typ != VPair {
			return vnil(), nil
		}
		v = v.cdr
	}
	if v != nil && v.typ == VPair {
		return v.car, nil
	}
	return vnil(), nil
}
func builtinSeventh(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	v := args[0]
	for i := 0; i < 6; i++ {
		if v == nil || v.typ != VPair {
			return vnil(), nil
		}
		v = v.cdr
	}
	if v != nil && v.typ == VPair {
		return v.car, nil
	}
	return vnil(), nil
}
func builtinEighth(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	v := args[0]
	for i := 0; i < 7; i++ {
		if v == nil || v.typ != VPair {
			return vnil(), nil
		}
		v = v.cdr
	}
	if v != nil && v.typ == VPair {
		return v.car, nil
	}
	return vnil(), nil
}
func builtinNinth(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	v := args[0]
	for i := 0; i < 8; i++ {
		if v == nil || v.typ != VPair {
			return vnil(), nil
		}
		v = v.cdr
	}
	if v != nil && v.typ == VPair {
		return v.car, nil
	}
	return vnil(), nil
}
func builtinTenth(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	v := args[0]
	for i := 0; i < 9; i++ {
		if v == nil || v.typ != VPair {
			return vnil(), nil
		}
		v = v.cdr
	}
	if v != nil && v.typ == VPair {
		return v.car, nil
	}
	return vnil(), nil
}

func builtinNullP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("null?: need 1 argument")
	}
	return vbool(isNil(args[0])), nil
}

func builtinPairP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("pair?: need 1 argument")
	}
	return vbool(isPair(args[0])), nil
}

func builtinListP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("listp: need 1 argument")
	}
	v := args[0]
	return vbool(isNil(v) || v.typ == VPair), nil
}

func builtinNumP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("number?: need 1 argument")
	}
	return vbool(args[0].typ == VNum || args[0].typ == VRat || args[0].typ == VComplex), nil
}

func builtinStrP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("string?: need 1 argument")
	}
	return vbool(args[0].typ == VStr), nil
}

func builtinSymP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("symbol?: need 1 argument")
	}
	return vbool(args[0].typ == VSym), nil
}

func builtinStringP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("stringp: need 1 argument")
	}
	return vbool(args[0].typ == VStr), nil
}

func builtinBoolP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("boolean?: need 1 argument")
	}
	return vbool(args[0].typ == VBool), nil
}

func builtinProcP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("procedure?: need 1 argument")
	}
	return vbool(args[0].typ == VPrim || args[0].typ == VFunc), nil
}

func builtinCharP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("character?: need 1 argument")
	}
	return vbool(args[0].typ == VChar), nil
}

func builtinNumberP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("numberp: need 1 argument")
	}
	return vbool(args[0].typ == VNum), nil
}

func builtinChar(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("char: need string and index")
	}
	if args[0].typ != VStr {
		return nil, fmt.Errorf("char: expected a string")
	}
	if args[1].typ != VNum {
		return nil, fmt.Errorf("char: expected an integer")
	}
	idx := int(args[1].num)
	s := args[0].str
	runes := []rune(s)
	if idx < 0 || idx >= len(runes) {
		return nil, fmt.Errorf("char: index %d out of range", idx)
	}
	return vchar(runes[idx]), nil
}

func builtinCharSetf(args []*Value) (*Value, error) {
	// (char-setf newchar string index)
	if len(args) < 3 {
		return nil, fmt.Errorf("char-setf: need newchar, string, and index")
	}
	newChar := args[0]
	s := args[1]
	idx := args[2]
	if newChar.typ != VChar {
		return nil, fmt.Errorf("char-setf: expected a character for new value")
	}
	if s.typ != VStr {
		return nil, fmt.Errorf("char-setf: expected a string")
	}
	if idx.typ != VNum {
		return nil, fmt.Errorf("char-setf: expected an integer index")
	}
	i := int(idx.num)
	runes := []rune(s.str)
	if i < 0 || i >= len(runes) {
		return nil, fmt.Errorf("char-setf: index %d out of range", i)
	}
	runes[i] = newChar.ch
	s.str = string(runes)
	return newChar, nil
}

func charCompare(op string, args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("char%s: expected at least 2 characters", op)
	}
	runes := make([]rune, len(args))
	for i, a := range args {
		if a.typ != VChar {
			return nil, fmt.Errorf("char%s: expected a character", op)
		}
		runes[i] = a.ch
	}
	for i := 0; i < len(runes)-1; i++ {
		a, b := runes[i], runes[i+1]
		switch op {
		case "=":
			if a != b {
				return vbool(false), nil
			}
		case "<":
			if !(a < b) {
				return vbool(false), nil
			}
		case ">":
			if !(a > b) {
				return vbool(false), nil
			}
		case "<=":
			if !(a <= b) {
				return vbool(false), nil
			}
		case ">=":
			if !(a >= b) {
				return vbool(false), nil
			}
		}
	}
	return vbool(true), nil
}

func builtinCharEq(args []*Value) (*Value, error) { return charCompare("=", args) }
func builtinCharLt(args []*Value) (*Value, error) { return charCompare("<", args) }
func builtinCharGt(args []*Value) (*Value, error) { return charCompare(">", args) }
func builtinCharLe(args []*Value) (*Value, error) { return charCompare("<=", args) }
func builtinCharGe(args []*Value) (*Value, error) { return charCompare(">=", args) }

func builtinCodeChar(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("code-char: need an integer")
	}
	if args[0].typ != VNum {
		return nil, fmt.Errorf("code-char: expected an integer")
	}
	n := int(args[0].num)
	if n < 0 || n > 0x10FFFF {
		return vnil(), nil
	}
	return vchar(rune(n)), nil
}

func builtinCharCode(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("char-code: need a character")
	}
	if args[0].typ != VChar {
		return nil, fmt.Errorf("char-code: expected a character")
	}
	return vnum(float64(args[0].ch)), nil
}

func builtinBitAref(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("bit: need bit-vector and index")
	}
	arr := args[0]
	if arr.typ != VArray {
		return nil, fmt.Errorf("bit: not a bit-vector")
	}
	if !isBitVector(arr.array) {
		return nil, fmt.Errorf("bit: not a bit-vector")
	}
	idx := int(toNum(args[1]))
	if idx < 0 || idx >= arr.array.dims[0] {
		return nil, fmt.Errorf("bit: index %d out of range", idx)
	}
	return arr.array.elements[idx], nil
}

func builtinSbitAref(args []*Value) (*Value, error) {
	// sbit is like bit but for simple bit-vectors (same as bit in microlisp)
	return builtinBitAref(args)
}

func builtinCharInt(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("char-int: need a character")
	}
	if args[0].typ != VChar {
		return nil, fmt.Errorf("char-int: expected a character")
	}
	// In most implementations, char-int is the same as char-code
	return vnum(float64(args[0].ch)), nil
}

// Standard character names
var charNameMap = map[string]rune{
	"space":     ' ',
	"newline":   '\n',
	"tab":       '\t',
	"return":    '\r',
	"backspace": '\x08',
	"bell":      '\x07',
	"page":      '\f',
	"escape":    '\x1b',
	"rubout":    '\x7f',
	"null":      '\x00',
	// Additional standard ASCII control character names
	"soh":       '\x01',
	"stx":       '\x02',
	"etx":       '\x03',
	"eot":       '\x04',
	"enq":       '\x05',
	"ack":       '\x06',
	"nak":       '\x15',
	"syn":       '\x16',
	"etb":       '\x17',
	"can":       '\x18',
	"em":        '\x19',
	"sub":       '\x1a',
	"fs":        '\x1c',
	"gs":        '\x1d',
	"rs":        '\x1e',
	"us":        '\x1f',
	"del":       '\x7f',
	// Common abbreviations
	"xoff":      '\x13',
	"xon":       '\x11',
}

func builtinNameChar(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("name-char: need a string")
	}
	name := args[0]
	var nameStr string
	switch name.typ {
	case VStr:
		nameStr = name.str
	case VSym:
		nameStr = name.str
	case VChar:
		// If already a char, return it
		return name, nil
	default:
		return vnil(), nil
	}
	nameStr = strings.ToLower(nameStr)
	if ch, ok := charNameMap[nameStr]; ok {
		return vchar(ch), nil
	}
	return vnil(), nil
}

func builtinCharName(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VChar {
		return nil, fmt.Errorf("char-name: expected a character")
	}
	ch := args[0].ch
	// CL spec: code 127 is named "Rubout" (not "Del" which is an implementation-dependent synonym)
	if ch == 127 {
		return vstr("Rubout"), nil
	}
	// Check named characters (return capitalized name per CL spec)
	for name, r := range charNameMap {
		if r == ch {
			// Capitalize first letter
			return vstr(strings.ToUpper(name[:1]) + name[1:]), nil
		}
	}
	// For control characters (C0: 0-31) that don't have specific names
	if ch < 32 && ch >= 0 {
		// Check if already in charNameMap (specific name like Newline)
		if _, ok := charNameMapRev[ch]; ok {
			// handled above
		} else {
			return vstr(fmt.Sprintf("C%d", ch)), nil
		}
	}
	// C1 control characters (128-159): return "C128", "C129", etc.
	if ch >= 128 && ch < 160 {
		return vstr(fmt.Sprintf("C%d", ch)), nil
	}
	// Other non-printing characters (e.g. NBSP, soft hyphen)
	if !unicode.IsPrint(ch) {
		return vstr("control"), nil
	}
	if ch == 127 {
		return vstr("rubout"), nil
	}
	return vnil(), nil
}

// charNameMapRev is the reverse of charNameMap (rune -> string name)
var charNameMapRev = map[rune]string{
	' ':     "space",
	'\n':    "newline",
	'\t':    "tab",
	'\r':    "return",
	'\x08':  "backspace",
	'\x7f':  "rubout",
	'\f':    "page",
	'\x00':  "null",
	'\x07':  "bell",
	'\x1b':  "escape",
	'\x01':  "soh",
	'\x02':  "stx",
	'\x03':  "etx",
	'\x04':  "eot",
	'\x05':  "enq",
	'\x06':  "ack",
	'\x15':  "nak",
	'\x16':  "syn",
	'\x17':  "etb",
	'\x18':  "can",
	'\x19':  "em",
	'\x1a':  "sub",
	'\x1c':  "fs",
	'\x1d':  "gs",
	'\x1e':  "rs",
	'\x1f':  "us",
	'\x13':  "xoff",
	'\x11':  "xon",
}

func eqVal(a, b *Value) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	// In CL, nil (symbol) and () (empty list/VNil) are equal
	if (a.typ == VNil && b.typ == VNil) {
		return true
	}
	if a.typ == VNil && b.typ == VSym && strings.EqualFold(b.str, "nil") ||
		b.typ == VNil && a.typ == VSym && strings.EqualFold(a.str, "nil") {
		return true
	}
	if a.typ != b.typ {
		return false
	}
	return eqValSeen(a, b, make(map[[2]uintptr]bool))
}

func eqValSeen(a, b *Value, seen map[[2]uintptr]bool) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if a.typ != b.typ {
		return false
	}
	switch a.typ {
	case VNum:
		return a.num == b.num
	case VRat:
		return a.irat == b.irat && a.iden == b.iden
	case VComplex:
		return a.num == b.num && a.imag == b.imag
	case VStr:
		return a.str == b.str
	case VSym:
		return a.str == b.str
	case VChar:
		return a.ch == b.ch
	case VPackage:
		return a.pkg == b.pkg
	case VReadtable:
		return a.readtable == b.readtable
	case VBool, VNil, VVHash:
		return a == b
	case VPair:
		ka := [2]uintptr{uintptr(unsafe.Pointer(a)), uintptr(unsafe.Pointer(b))}
		if seen[ka] {
			return true
		}
		seen[ka] = true
		return eqValSeen(a.car, b.car, seen) && eqValSeen(a.cdr, b.cdr, seen)
	case VArray:
		if a.array == nil || b.array == nil {
			return a.array == b.array
		}
		if len(a.array.elements) != len(b.array.elements) {
			return false
		}
		for i := range a.array.elements {
			if !eqValSeen(a.array.elements[i], b.array.elements[i], seen) {
				return false
			}
		}
		return true
	}
	return false
}



// bindPattern binds a destructuring pattern to a value in the given env.
func bindPattern(pattern *Value, val *Value, env *Env) error {
	return bindPatternRec(pattern, val, env, make(map[*Value]bool))
}

func bindPatternRec(pattern *Value, val *Value, env *Env, seen map[*Value]bool) error {
	if pattern.typ == VSym {
		// Simple variable: bind the whole value
		env.Set(pattern.str, val)
		return nil
	}
	if pattern.typ != VPair {
		return fmt.Errorf("destructuring-bind: invalid pattern")
	}
	// List pattern: bind each element
	// Handle lambda-list keywords: &rest, &optional, &key
	vp := pattern
	vv := val
	localSeen := make(map[*Value]bool)
	for !isNil(vp) {
		if localSeen[vp] {
			return fmt.Errorf("destructuring-bind: circular pattern")
		}
		localSeen[vp] = true
		// Check for dotted pair (rest parameter) - must come before &rest handling
		if !isNil(vp) && vp.typ == VSym {
			// Dotted pair: (a b . rest)
			env.Set(vp.str, vv)
			return nil
		}
		if vp.typ != VPair {
			break
		}
		head := vp.car
		// Skip &rest, &optional, &key keywords and handle them specially
		if head != nil && head.typ == VSym {
			symName := strings.ToUpper(head.str)
			if symName == "&REST" || symName == "&BODY" {
				// &rest var: bind var to the remaining value list
				restVar := vp.cdr
				if restVar == nil || restVar.typ != VPair || restVar.car == nil || restVar.car.typ != VSym {
					return fmt.Errorf("destructuring-bind: malformed &rest pattern")
				}
				env.Set(restVar.car.str, vv)
				return nil
			}
			if symName == "&OPTIONAL" || symName == "&KEY" {
				// Process &optional vars: bind from remaining values
				if symName == "&OPTIONAL" {
					vp = vp.cdr
					for !isNil(vp) {
						if vp.typ != VPair {
							break
						}
						elem := vp.car
						// Check if we've hit another lambda-list keyword
						if elem != nil && elem.typ == VSym {
							elemUpper := strings.ToUpper(elem.str)
							if elemUpper == "&REST" || elemUpper == "&BODY" || elemUpper == "&KEY" || elemUpper == "&AUX" || elemUpper == "&ALLOW-OTHER-KEYS" || elemUpper == "&ENVIRONMENT" || elemUpper == "&WHOLE" {
								break // let outer loop handle it
							}
						}
						if elem == nil || elem.typ == VNil {
							// nil element, skip
						} else if elem.typ == VSym {
							// Simple optional var: bind to value or nil
							if !isNil(vv) {
								env.Set(elem.str, vv.car)
								if !isNil(vv) {
									vv = vv.cdr
								}
							} else {
								env.Set(elem.str, vnil())
							}
						} else if elem.typ == VPair {
							// (var default-value supplied-p) or (var default-value) or (var)
							varName := elem.car
							if varName != nil && varName.typ == VSym {
								var optSuppliedPSym *Value
								if elem.cdr != nil && elem.cdr.typ == VPair && elem.cdr.cdr != nil && elem.cdr.cdr.typ == VPair && elem.cdr.cdr.car != nil && elem.cdr.cdr.car.typ == VSym {
									optSuppliedPSym = elem.cdr.cdr.car
								}
								if !isNil(vv) {
									env.Set(varName.str, vv.car)
									if optSuppliedPSym != nil {
										env.Set(optSuppliedPSym.str, vbool(true))
									}
									if !isNil(vv) {
										vv = vv.cdr
									}
									} else {
									var optDefault *Value
									if elem.cdr != nil && elem.cdr.typ == VPair && elem.cdr.car != nil {
										optDefault = elem.cdr.car
									}
									if optDefault != nil {
										evalDef, _ := Eval(optDefault, env)
										if evalDef == nil {
											evalDef = vnil()
										}
										env.Set(varName.str, evalDef)
									} else {
										env.Set(varName.str, vnil())
									}
									if optSuppliedPSym != nil {
										env.Set(optSuppliedPSym.str, vbool(false))
									}
									}
							}
						} else {
							// Non-symbol/non-list element (e.g., number literal in default),
							// not a valid pattern variable - skip
						}
						vp = vp.cdr
					}
					// Don't return - continue outer loop to handle &rest/&key/&aux
					continue
				}
				// Process &key vars: keyword-based binding
				// Build a keyword-to-index map from the value list
				// Keywords are symbols starting with ':'; bind var matching after ':'
				vp = vp.cdr
				// Collect keyword-value pairs from the value list
				keyValMap := make(map[string]*Value)
				for !isNil(vv) && vv.typ == VPair {
					key := vv.car
					val := vnil()
					if !isNil(vv.cdr) && vv.cdr.typ == VPair {
						val = vv.cdr.car
					}
					if key != nil && key.typ == VSym && len(key.str) > 0 && key.str[0] == ':' {
						keywordName := key.str[1:] // strip leading ':'
						keyValMap[keywordName] = val
					}
					vv = vv.cdr
					if !isNil(vv) && vv.typ == VPair {
						vv = vv.cdr
					} else {
						break
					}
				}
				// Bind each key pattern variable
				for !isNil(vp) {
					if vp.typ != VPair {
						break
					}
					elem := vp.car
					if elem == nil || elem.typ == VNil {
						// nil element, skip
					} else if elem.typ == VSym {
						// Simple key var: bind matching keyword or nil
						if val, ok := keyValMap[elem.str]; ok {
							env.Set(elem.str, val)
						} else {
							env.Set(elem.str, vnil())
						}
					} else if elem.typ == VPair {
						// (var default-value) or ((:keyword var) default-value) or (:keyword var) or (:keyword var default-value)
						varName := elem.car
						if varName != nil && varName.typ == VSym {
							var keySuppliedPSym *Value
							if elem.cdr != nil && elem.cdr.typ == VPair && elem.cdr.cdr != nil && elem.cdr.cdr.typ == VPair && elem.cdr.cdr.car != nil && elem.cdr.cdr.car.typ == VSym {
								keySuppliedPSym = elem.cdr.cdr.car
							}
							if val, ok := keyValMap[varName.str]; ok {
								env.Set(varName.str, val)
								if keySuppliedPSym != nil {
									env.Set(keySuppliedPSym.str, vbool(true))
								}
							} else if elem.cdr != nil && elem.cdr.typ == VPair && elem.cdr.car != nil {
								evalDef, _ := Eval(elem.cdr.car, env)
								if evalDef == nil {
									evalDef = vnil()
								}
								env.Set(varName.str, evalDef)
								if keySuppliedPSym != nil {
									env.Set(keySuppliedPSym.str, vbool(false))
								}
							} else {
								env.Set(varName.str, vnil())
								if keySuppliedPSym != nil {
									env.Set(keySuppliedPSym.str, vbool(false))
								}
							}
						} else if varName != nil && varName.typ == VPair && varName.car != nil && varName.car.typ == VSym {
							// (:keyword var) form
							keywordName := varName.car.str
							if keywordName[0] == ':' {
								keywordName = keywordName[1:]
							}
							subVar := varName.cdr
							if subVar != nil && subVar.typ == VSym {
								// (:keyword var) simple form - bind var from keyword map or nil
								if val, ok := keyValMap[keywordName]; ok {
									env.Set(subVar.str, val)
								} else {
									env.Set(subVar.str, vnil())
								}
								// Handle (:keyword var default) and (:keyword var default supplied-p)
								if elem.cdr != nil && elem.cdr.typ == VPair {
									var kwDefault *Value
									var kwSuppliedP *Value
									if elem.cdr.car != nil {
										kwDefault = elem.cdr.car
									}
									if elem.cdr.cdr != nil && elem.cdr.cdr.typ == VPair && elem.cdr.cdr.car != nil && elem.cdr.cdr.car.typ == VSym {
										kwSuppliedP = elem.cdr.cdr.car
									}
									if _, ok := keyValMap[keywordName]; !ok {
										if kwDefault != nil {
											evalDef, _ := Eval(kwDefault, env)
											if evalDef == nil {
												evalDef = vnil()
											}
											env.Set(subVar.str, evalDef)
										}
										if kwSuppliedP != nil {
											env.Set(kwSuppliedP.str, vbool(false))
										}
									} else if kwSuppliedP != nil {
										env.Set(kwSuppliedP.str, vbool(true))
									}
								}
							} else if subVar != nil && subVar.typ == VPair && subVar.car != nil && subVar.car.typ == VSym {
								// subVar is a list like (VAR) - extract car
								subVar = subVar.car
								var kwSuppliedPSym *Value
								if elem.cdr != nil && elem.cdr.typ == VPair && elem.cdr.cdr != nil && elem.cdr.cdr.typ == VPair && elem.cdr.cdr.car != nil && elem.cdr.cdr.car.typ == VSym {
									kwSuppliedPSym = elem.cdr.cdr.car
								}
								if val, ok := keyValMap[keywordName]; ok {
									env.Set(subVar.str, val)
									if kwSuppliedPSym != nil {
										env.Set(kwSuppliedPSym.str, vbool(true))
									}
								} else if elem.cdr != nil && elem.cdr.typ == VPair && elem.cdr.car != nil {
									evalDef2, _ := Eval(elem.cdr.car, env)
									if evalDef2 == nil {
										evalDef2 = vnil()
									}
									env.Set(subVar.str, evalDef2)
									if kwSuppliedPSym != nil {
										env.Set(kwSuppliedPSym.str, vbool(false))
									}
								} else {
									env.Set(subVar.str, vnil())
									if kwSuppliedPSym != nil {
										env.Set(kwSuppliedPSym.str, vbool(false))
									}
								}
							}
						}
					}
					vp = vp.cdr
				}
				return nil
			}
		}
		if isNil(vv) {
			// Not enough values — bind variable to nil
			if head == nil || head.typ != VSym {
				return fmt.Errorf("destructuring-bind: malformed var pattern")
			}
			env.Set(head.str, vnil())
		} else if vv.typ != VPair {
			return fmt.Errorf("destructuring-bind: expected a list, got %s", typeStr(vv))
		} else {
			if err := bindPatternRec(head, vv.car, env, seen); err != nil {
				return err
			}
		}
		vp = vp.cdr
		if !isNil(vp) && vp.typ == VSym {
			// Dotted pair: (a b . rest)
			if isNil(vv) {
				env.Set(vp.str, vnil())
			} else {
				env.Set(vp.str, vv.cdr)
			}
			return nil
		}
		if !isNil(vv) {
			vv = vv.cdr
		}
	}
	return nil
}

func builtinEqvP(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("eqv?: need 2 arguments")
	}
	a, b := args[0], args[1]
	if a.typ != b.typ {
		return vbool(false), nil
	}
	switch a.typ {
	case VNum:
		return vbool(a.num == b.num), nil
	case VRat:
		return vbool(a.str == b.str), nil
	case VComplex:
		return vbool(a.str == b.str), nil
	case VStr:
		return vbool(a.str == b.str), nil
	case VChar:
		return vbool(a.ch == b.ch), nil
	case VPackage:
		return vbool(a.pkg == b.pkg), nil
	case VReadtable:
		return vbool(a.readtable == b.readtable), nil
	default:
		return vbool(a == b), nil
	}
}

func builtinEqualP(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("equal?: need 2 arguments")
	}
	return vbool(eqVal(args[0], args[1])), nil
}

func builtinLength(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("length: need argument")
	}
	v := primaryValue(args[0])
	// In CL, nil is a valid sequence (empty list), length = 0
	if isNil(v) {
		return vnum(0), nil
	}
	if v.typ != VPair && v.typ != VStr && v.typ != VArray {
		return nil, fmt.Errorf("length: %s is not a sequence", ToString(v))
	}
	return vnum(float64(lengthSafe(v))), nil
}

func lengthSafe(v *Value) int64 {
	if v.typ == VStr {
		return int64(utf8.RuneCountInString(v.str))
	}
	if v.typ == VArray && v.array != nil {
		if v.array.fillPtr >= 0 {
			return int64(v.array.fillPtr)
		}
		return int64(len(v.array.elements))
	}
	n := int64(0)
	visited := make(map[*Value]bool)
	for !isNil(v) && v.typ == VPair {
		if visited[v] {
			break // circular list
		}
		visited[v] = true
		n++
		v = v.cdr
	}
	return n
}

func builtinDisplay(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("display: need 1 argument")
	}
	fmt.Print(ToString(args[0]))
	return args[0], nil
}

func builtinNewline(args []*Value) (*Value, error) {
	fmt.Println()
	return vnil(), nil
}

func builtinWriteToString(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("write-to-string: need argument")
	}
	return vstr(writeToString(primaryValue(args[0]))), nil
}

func builtinRead(args []*Value) (*Value, error) {
	if len(args) > 0 {
		// stream argument - return nil for now
		return vnil(), nil
	}
	r := bufio.NewReader(os.Stdin)
	line, err := r.ReadString('\n')
	if err != nil {
		return vnil(), nil
	}
	return parseExpr(strings.TrimSpace(line))
}

func builtinStr(args []*Value) (*Value, error) {
	var b strings.Builder
	for _, a := range args {
		if a.typ == VStr {
			b.WriteString(a.str)
		} else {
			b.WriteString(ToString(a))
		}
	}
	return vstr(b.String()), nil
}

func builtinStrAppend(args []*Value) (*Value, error) {
	var b strings.Builder
	for _, a := range args {
		if a.typ != VStr {
			return nil, fmt.Errorf("string-append: expected string")
		}
		b.WriteString(a.str)
	}
	return vstr(b.String()), nil
}

func builtinStrLen(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("string-length: need string")
	}
	if args[0].typ != VStr {
		return nil, fmt.Errorf("string-length: expected string")
	}
	return vnum(float64(len(args[0].str))), nil
}

func builtinStrFind(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("string-find: need char/string and string")
	}
	if args[1].typ != VStr {
		return nil, fmt.Errorf("string-find: second arg must be a string")
	}
	var searchStr string
	if args[0].typ == VChar {
		searchStr = string(args[0].ch)
	} else if args[0].typ == VStr {
		searchStr = args[0].str
	} else {
		return nil, fmt.Errorf("string-find: first arg must be string or character")
	}
	idx := strings.Index(args[1].str, searchStr)
	if idx < 0 {
		return vnil(), nil
	}
	return vnum(float64(idx)), nil
}

func builtinSubstring(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("substring: need string and start index")
	}
	if args[0].typ != VStr {
		return nil, fmt.Errorf("substring: expected string")
	}
	s := args[0].str
	runes := []rune(s)
	start, err := safeToNum(args[1], "substring")
	if err != nil {
		return nil, err
	}
	startInt := int(start)
	var endInt int
	if len(args) >= 3 {
		en, err := safeToNum(args[2], "substring")
		if err != nil {
			return nil, err
		}
		endInt = int(en)
	} else {
		endInt = len(runes)
	}
	// Bounds checking
	if startInt < 0 {
		return nil, fmt.Errorf("substring: start index %d out of range [0..%d]", startInt, len(runes))
	}
	if endInt < 0 {
		return nil, fmt.Errorf("substring: end index %d out of range [0..%d]", endInt, len(runes))
	}
	if endInt > len(runes) {
		return nil, fmt.Errorf("substring: end index %d out of range [0..%d]", endInt, len(runes))
	}
	if startInt > endInt {
		return nil, fmt.Errorf("substring: start index %d greater than end index %d", startInt, endInt)
	}
	return vstr(string(runes[startInt:endInt])), nil
}

func builtinNumStr(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("number->string: need a number")
	}
	s := strconv.FormatFloat(toNum(args[0]), 'g', -1, 64)
	return vstr(s), nil
}

// coerceStringDesignator converts a string designator (string, symbol, character, or array)
// to a Go string, per the CL spec.
func coerceStringDesignator(v *Value) (string, error) {
	switch v.typ {
	case VStr:
		return v.str, nil
	case VSym:
		return v.str, nil
	case VChar:
		return string(v.ch), nil
	case VArray:
		return vecToString(v), nil
	default:
		return "", fmt.Errorf("expected a string designator (string, symbol, or character)")
	}
}

func builtinStrUpcase(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("string-upcase: expected a string designator")
	}
	s, err := coerceStringDesignator(args[0])
	if err != nil {
		return nil, fmt.Errorf("string-upcase: %v", err)
	}
	runes := []rune(s)
	start, end := 0, len(runes)
	for i := 1; i < len(args); i++ {
		if args[i].typ == VSym {
			switch args[i].str {
			case ":START":
				if i+1 < len(args) {
					i++
					n, err := safeToNum(args[i], "string-upcase :start")
					if err != nil {
						return nil, err
					}
					start = int(n)
				}
			case ":END":
				if i+1 < len(args) {
					i++
					n, err := safeToNum(args[i], "string-upcase :end")
					if err != nil {
						return nil, err
					}
					end = int(n)
				}
			}
		}
	}
	if end > len(runes) {
		end = len(runes)
	}
	for i := start; i < end; i++ {
		runes[i] = unicode.ToUpper(runes[i])
	}
	return vstr(string(runes)), nil
}

func builtinStrDowncase(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("string-downcase: expected a string designator")
	}
	s, err := coerceStringDesignator(args[0])
	if err != nil {
		return nil, fmt.Errorf("string-downcase: %v", err)
	}
	runes := []rune(s)
	start, end := 0, len(runes)
	for i := 1; i < len(args); i++ {
		if args[i].typ == VSym {
			switch args[i].str {
			case ":START":
				if i+1 < len(args) {
					i++
					n, err := safeToNum(args[i], "string-downcase :start")
					if err != nil {
						return nil, err
					}
					start = int(n)
				}
			case ":END":
				if i+1 < len(args) {
					i++
					n, err := safeToNum(args[i], "string-downcase :end")
					if err != nil {
						return nil, err
					}
					end = int(n)
				}
			}
		}
	}
	if end > len(runes) {
		end = len(runes)
	}
	for i := start; i < end; i++ {
		runes[i] = unicode.ToLower(runes[i])
	}
	return vstr(string(runes)), nil
}

func builtinStrNum(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("string->number: need a string argument")
	}
	if args[0].typ != VStr {
		return nil, fmt.Errorf("string->number: expected a string")
	}
	f, err := strconv.ParseFloat(args[0].str, 64)
	if err != nil {
		return vnil(), nil
	}
	return vnum(f), nil
}

func builtinSymStr(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("symbol->string: need a symbol")
	}
	if args[0].typ != VSym {
		return nil, fmt.Errorf("symbol->string: expected symbol")
	}
	return vstr(args[0].str), nil
}

func builtinStrSym(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("string->symbol: need a string")
	}
	if args[0].typ != VStr {
		return nil, fmt.Errorf("string->symbol: expected string")
	}
	return vsym(strings.ToUpper(args[0].str)), nil
}

func builtinErr(args []*Value) (*Value, error) {
	var msg string
	for _, a := range args {
		msg += ToString(a)
	}
	return nil, fmt.Errorf("%s", msg)
}

func builtinExit(args []*Value) (*Value, error) {
	code := 0
	if len(args) > 0 {
		code = int(toNum(args[0]))
	}
	os.Exit(code)
	return vnil(), nil
}

var gensymCounter int64 = 0

func builtinGensym(args []*Value) (*Value, error) {
	gensymCounter++
	prefix := "G"
	if len(args) > 0 {
		prefix = args[0].str
	}
	return vsym(fmt.Sprintf("%s_%d", prefix, gensymCounter)), nil
}

func builtinGentemp(args []*Value) (*Value, error) {
	gensymCounter++
	prefix := "T"
	if len(args) > 0 {
		prefix = args[0].str
	}
	name := fmt.Sprintf("%s%d", prefix, gensymCounter)
	return internSymbol(name, currentPackage), nil
}

func builtinLoopCheck(args []*Value) (*Value, error) {
	loopIterationCount++
	if loopIterationCount > maxLoopIterations {
		loopIterationCount = 0
		return nil, fmt.Errorf("loop iteration limit exceeded (%d)", maxLoopIterations)
	}
	return vnil(), nil
}

// -------- Symbol introspection --------
func builtinSymbolValue(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("symbol-value: need a symbol")
	}
	sym := args[0]
	if sym.typ != VSym {
		return nil, fmt.Errorf("symbol-value: not a symbol")
	}
	val, err := globalEnv.Get(sym.str)
	if err != nil {
		return vnil(), nil
	}
	return val, nil
}

// builtinMacroFunction handles both:
// (macro-function sym) - returns the macro function or nil
// (macro-function (macro-function sym) new-fn) - setter form for setf
func builtinMacroFunction(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("macro-function: need a symbol or setter form")
	}
	// Setter form: (macro-function (macro-function sym) new-fn)
	if args[0].typ == VPair && args[0].car != nil && args[0].car.typ == VSym && args[0].car.str == "MACRO-FUNCTION" {
		// args[0] = (MACRO-FUNCTION sym), args[1] = new-fn
		accessorArgs := args[0].cdr
		if accessorArgs == nil || accessorArgs.typ != VPair {
			return nil, fmt.Errorf("macro-function setf: malformed")
		}
		sym := accessorArgs.car
		// Evaluate (quote sym) form if present
		if sym.typ == VPair && sym.car != nil && sym.car.typ == VSym && sym.car.str == "QUOTE" {
			sym = sym.cdr.car
		}
		if sym.typ != VSym {
			return nil, fmt.Errorf("macro-function setf: symbol required")
		}
		if len(args) < 2 {
			return nil, fmt.Errorf("macro-function setf: need new value")
		}
		return builtinMacroFunctionSetf([]*Value{args[1], sym})
	}
	// Getter: (macro-function sym)
	sym := args[0]
	// Evaluate (quote sym) form if present
	if sym.typ == VPair && sym.car != nil && sym.car.typ == VSym && sym.car.str == "QUOTE" {
		sym = sym.cdr.car
	}
	if sym.typ != VSym {
		return nil, fmt.Errorf("macro-function: not a symbol")
	}
	val, err := globalEnv.Get(sym.str)
	if err != nil || val == nil || val.typ != VMacro {
		return vnil(), nil
	}
	return val, nil
}

// builtinMacroFunctionSetf implements (setf (macro-function sym) fn).
// fn must be a function of two arguments: (form environment) -> expansion.
func builtinMacroFunctionSetf(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("setf (macro-function): need value and symbol")
	}
	newFn := args[0]
	sym := args[1]
	if sym.typ != VSym {
		return nil, fmt.Errorf("setf (macro-function): symbol required")
	}
	// Use expandMacro's &whole mechanism to pass the whole form (macro-name . args).
	// Set m.whole = "#FORM" so expandMacro binds the whole form to #FORM.
	// Then the body is (fn-ref #FORM #ENV) where #FORM = whole form and #ENV = nil.
	var closureForm *Value
	if newFn.typ == VSym {
		closureForm = list(newFn, vsym("#FORM"), vsym("#ENV"))
	} else {
		closureForm = list(newFn, vsym("#FORM"), vsym("#ENV"))
	}
	m := gcv()
	m.typ = VMacro
	m.str = sym.str
	m.params = nil      // no macro lambda-list params
	m.rest = ""
	m.whole = "#FORM"   // expandMacro binds whole form to #FORM
	m.body = closureForm
	if newFn.typ == VFunc || newFn.typ == VMacro {
		m.env = newFn.env
	} else {
		m.env = globalEnv
	}
	globalEnv.Set(sym.str, m)
	return newFn, nil
}

// builtinCompilerMacroFunction returns the compiler macro function for a symbol, or nil.
func builtinCompilerMacroFunction(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("compiler-macro-function: need a symbol")
	}
	sym := args[0]
	if sym.typ != VSym {
		return nil, fmt.Errorf("compiler-macro-function: not a symbol")
	}
	cm, ok := compilerMacros[sym.str]
	if !ok || cm == nil {
		return vnil(), nil
	}
	return cm, nil
}

// builtinCompilerMacroFunctionSetf implements (setf (compiler-macro-function sym) fn).
func builtinCompilerMacroFunctionSetf(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("setf (compiler-macro-function): need value and symbol")
	}
	newFn := args[0]
	sym := args[1]
	if sym.typ != VSym {
		return nil, fmt.Errorf("setf (compiler-macro-function): symbol required")
	}
	if isNil(newFn) {
		delete(compilerMacros, sym.str)
	} else {
		m := gcv()
		m.typ = VMacro
		m.str = sym.str
		m.params = nil
		m.rest = ""
		m.whole = ""
		m.body = list(newFn, vsym("#FORM"), vsym("#ENV"))
		m.env = globalEnv
		compilerMacros[sym.str] = m
	}
	return newFn, nil
}

// builtinSetClassPrintFn stores a print function for a defstruct class (used by :print-function)
func builtinSetClassPrintFn(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("set-class-print-fn: need class-name and function")
	}
	name := args[0]
	printFn := args[1]
	if name.typ != VSym {
		return nil, fmt.Errorf("set-class-print-fn: class-name must be a symbol")
	}
	if printFn.typ != VFunc && printFn.typ != VPrim {
		return nil, fmt.Errorf("set-class-print-fn: function must be callable")
	}
	structPrintFns[strings.ToUpper(name.str)] = printFn
	return vnil(), nil
}

// builtinRemoveClassPrintFn removes a print function for a defstruct class
func builtinRemoveClassPrintFn(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("remove-class-print-fn: need class-name")
	}
	name := args[0]
	if name.typ != VSym {
		return nil, fmt.Errorf("remove-class-print-fn: class-name must be a symbol")
	}
	delete(structPrintFns, strings.ToUpper(name.str))
	return vnil(), nil
}

func builtinSymbolFunction(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("symbol-function: need a symbol")
	}
	sym := args[0]
	if sym.typ != VSym {
		return nil, fmt.Errorf("symbol-function: not a symbol")
	}
	val, err := globalEnv.Get(sym.str)
	if err != nil {
		return nil, fmt.Errorf("symbol-function: %s has no function", sym.str)
	}
	if val.typ == VPrim || val.typ == VFunc || val.typ == VGeneric || val.typ == VMacro {
		return val, nil
	}
	return nil, fmt.Errorf("symbol-function: %s is not a function", sym.str)
}

func builtinSymbolPlist(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("symbol-plist: need a symbol")
	}
	sym := args[0]
	if sym.typ != VSym {
		return nil, fmt.Errorf("symbol-plist: not a symbol")
	}
	if sym.plist == nil {
		return vnil(), nil
	}
	return sym.plist, nil
}

func builtinSymbolPlistSetf(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("symbol-plist-setf: need value and symbol")
	}
	newVal := args[0]
	sym := args[1]
	if sym.typ != VSym {
		return nil, fmt.Errorf("symbol-plist-setf: need a symbol")
	}
	sym.plist = newVal
	return newVal, nil
}

func builtinBoundp(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("boundp: need a symbol")
	}
	sym := args[0]
	if sym.typ != VSym {
		return nil, fmt.Errorf("boundp: not a symbol")
	}
	_, err := globalEnv.Get(sym.str)
	return vbool(err == nil), nil
}

func builtinFboundp(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("fboundp: need a symbol")
	}
	sym := args[0]
	if sym.typ != VSym {
		return nil, fmt.Errorf("fboundp: not a symbol")
	}
	val, err := globalEnv.Get(sym.str)
	if err != nil {
		return vbool(false), nil
	}
	return vbool(val.typ == VPrim || val.typ == VFunc || val.typ == VGeneric || val.typ == VMacro), nil
}

// specialOperators is the set of CL special operators (not functions).
var specialOperators = map[string]bool{
	"QUOTE":       true,
	"IF":          true,
	"LAMBDA":      true,
	"PROGN":       true,
	"PROGV":       true,
	"THE":         true,
	"FLET":        true,
	"LABELS":      true,
	"LET":         true,
	"LET*":        true,
	"BLOCK":       true,
	"RETURN-FROM": true,
	"TAGBODY":     true,
	"GO":          true,
	"CATCH":       true,
	"THROW":       true,
	"MACROLET":    true,
	"MACRO-FUNCTION": true,
	"SETF":        true,
	"SETQ":        true,
	"FUNCTION":    true,
	"MULTIPLE-VALUE-CALL": true,
}

func builtinSpecialOperatorP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("special-operator-p: need a symbol")
	}
	sym := args[0]
	if sym.typ != VSym {
		return nil, fmt.Errorf("special-operator-p: not a symbol")
	}
	return vbool(specialOperators[sym.str]), nil
}

func builtinFunctionP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("functionp: need an argument")
	}
	return vbool(args[0].typ == VPrim || args[0].typ == VFunc || args[0].typ == VGeneric), nil
}

func builtinMakeGenericFunction(args []*Value) (*Value, error) {
	gf := gcv()
	gf.typ = VGeneric
	if len(args) > 0 && args[0].typ == VSym {
		gf.str = args[0].str
	} else if len(args) > 0 && args[0].typ == VStr {
		gf.str = args[0].str
	}
	return gf, nil
}

// builtinEnsureGenericFunction - ANSI CL ensure-generic-function
// If a generic function with the given name already exists, return it;
// otherwise create a new one and register it.
func builtinEnsureGenericFunction(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("ensure-generic-function: need a function name")
	}
	name := args[0]
	var nameStr string
	if name.typ == VSym {
		nameStr = name.str
	} else if name.typ == VStr {
		nameStr = name.str
	} else {
		return nil, fmt.Errorf("ensure-generic-function: function name must be a symbol or string")
	}
	// Check if it already exists as a generic function
	existing, err := globalEnv.Get(nameStr)
	if err == nil && existing != nil && existing.typ == VGeneric {
		return existing, nil
	}
	// Create a new generic function
	gf := gcv()
	gf.typ = VGeneric
	gf.str = nameStr
	// Parse optional keyword arguments
	for i := 1; i < len(args); i++ {
		if args[i].typ == VSym && args[i].str == ":LAMBDA-LIST" && i+1 < len(args) {
			// Store lambda-list info (simplified: just skip)
			i++
		} else if args[i].typ == VSym && args[i].str == ":DOCUMENTATION" && i+1 < len(args) {
			i++
		} else if args[i].typ == VSym && args[i].str == ":METHOD-COMBINATION" && i+1 < len(args) {
			if args[i+1].typ == VSym {
				gf.methodCombo = args[i+1].str
			}
			i++
		}
	}
	globalEnv.Set(nameStr, gf)
	return gf, nil
}

func builtinMakunbound(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("makunbound: need a symbol")
	}
	sym := args[0]
	if sym.typ != VSym {
		return nil, fmt.Errorf("makunbound: not a symbol")
	}
	delete(globalEnv.bindings, sym.str)
	return sym, nil
}

func builtinFmakunbound(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("fmakunbound: need a symbol")
	}
	sym := args[0]
	if sym.typ != VSym {
		return nil, fmt.Errorf("fmakunbound: not a symbol")
	}
	delete(globalEnv.bindings, sym.str)
	return sym, nil
}

func builtinTypeOf(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("type-of: need 1 argument")
	}
	return vsym(typeStr(args[0])), nil
}

func builtinDescribe(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("describe: need 1 argument")
	}
	obj := args[0]
	var sb strings.Builder
	sb.WriteString(ToString(obj))
	sb.WriteString(" is of type ")
	sb.WriteString(typeStr(obj))
	sb.WriteString("\n")
	switch obj.typ {
	case VNil:
		sb.WriteString("It is the canonical false value (nil).\n")
	case VBool:
		if obj == globalEnv.bindings["#t"] {
			sb.WriteString("It is the canonical true value (t).\n")
		} else {
			sb.WriteString("It is the canonical false value (nil).\n")
		}
	case VNum, VRat, VComplex:
		sb.WriteString("Value: ")
		sb.WriteString(ToString(obj))
		sb.WriteString("\nType: ")
		sb.WriteString(typeStr(obj))
		sb.WriteString("\n")
	case VStr:
		sb.WriteString("Value: ")
		sb.WriteString(ToString(obj))
		sb.WriteString("\nLength: ")
		sb.WriteString(strconv.Itoa(len(obj.str)))
		sb.WriteString("\n")
	case VChar:
		sb.WriteString("Character: ")
		sb.WriteString(ToString(obj))
		sb.WriteString("\nCode: ")
		sb.WriteString(strconv.Itoa(int(obj.ch)))
		sb.WriteString("\n")
	case VSym:
		sb.WriteString("Name: ")
		sb.WriteString(obj.str)
		sb.WriteString("\n")
		if obj.fn != nil {
			sb.WriteString("It has a function binding.\n")
		}
		val, err := globalEnv.Get(obj.str)
		if err == nil {
			sb.WriteString("Value: ")
			sb.WriteString(ToString(val))
			sb.WriteString("\n")
		}
		if val != nil && val.typ == VMacro {
			sb.WriteString("It is a macro.\n")
		}
	case VPair:
		length := 0
		cur := obj
		for cur != nil && cur.typ == VPair {
			length++
			cur = cur.cdr
		}
		sb.WriteString("It is a cons of length ")
		sb.WriteString(strconv.Itoa(length))
		sb.WriteString(".\nCar: ")
		sb.WriteString(ToString(obj.car))
		sb.WriteString("\nCdr: ")
		sb.WriteString(ToString(obj.cdr))
		sb.WriteString("\n")
	case VArray:
		if len(obj.array.dims) == 1 && obj.array.fillPtr < 0 {
			sb.WriteString("It is a vector.\n")
		} else {
			sb.WriteString("It is an array.\n")
		}
		sb.WriteString("Dimensions: ")
		for i, d := range obj.array.dims {
			if i > 0 {
				sb.WriteString(" x ")
			}
			sb.WriteString(strconv.Itoa(d))
		}
		sb.WriteString("\n")
	case VInstance:
		if obj.instClass != nil {
			sb.WriteString("It is an instance of class ")
			sb.WriteString(obj.instClass.str)
			sb.WriteString(".\nSlots:\n")
			for name, val := range obj.instSlots {
				sb.WriteString("  ")
				sb.WriteString(name)
				sb.WriteString(" = ")
				sb.WriteString(ToString(val))
				sb.WriteString("\n")
			}
		}
	case VClass:
		if obj.str != "" {
			sb.WriteString("It is a class named ")
			sb.WriteString(obj.str)
			sb.WriteString(".\n")
		}
		if obj.classParents != nil {
			parents := make([]string, len(obj.classParents))
			for i, p := range obj.classParents {
				if p != nil && p.str != "" {
					parents[i] = p.str
				} else {
					parents[i] = "(unknown)"
				}
			}
			sb.WriteString("Superclasses: ")
			for i, p := range parents {
				if i > 0 {
					sb.WriteString(" ")
				}
				sb.WriteString(p)
			}
			sb.WriteString("\n")
		}
	case VFunc:
		sb.WriteString("It is a function.\n")
		if obj.name != "" {
			sb.WriteString("Name: ")
			sb.WriteString(obj.name)
			sb.WriteString("\n")
		}
		sb.WriteString("Arity: ")
		sb.WriteString(strconv.Itoa(len(obj.params)))
		sb.WriteString("\n")
	case VPrim:
		sb.WriteString("It is a built-in function.\n")
	case VMacro:
		sb.WriteString("It is a macro.\n")
	case VStream:
		sb.WriteString("It is a stream.\n")
		if obj.stream.isInput {
			sb.WriteString("Direction: input\n")
		}
		if obj.stream.isOutput {
			sb.WriteString("Direction: output\n")
		}
	case VVHash:
		sb.WriteString("It is a hash-table.\n")
		sb.WriteString("Size: ")
		sb.WriteString(strconv.Itoa(obj.hashTab.count))
		sb.WriteString("\n")
	}
	return vstr(sb.String()), nil
}

// -------- room --------
func builtinRoom(args []*Value) (*Value, error) {
	var verbose bool
	if len(args) > 0 {
		verbose = !isNil(args[0])
	}
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	fmt.Fprintf(os.Stderr, "Dynamic Space Usage: %d bytes allocated, %d bytes total\n", m.Alloc, m.TotalAlloc)
	if verbose {
		fmt.Fprintf(os.Stderr, "  HeapSys: %d bytes\n", m.HeapSys)
		fmt.Fprintf(os.Stderr, "  HeapAlloc: %d bytes\n", m.HeapAlloc)
		fmt.Fprintf(os.Stderr, "  HeapIdle: %d bytes\n", m.HeapIdle)
		fmt.Fprintf(os.Stderr, "  HeapReleased: %d bytes\n", m.HeapReleased)
		fmt.Fprintf(os.Stderr, "  NumGC: %d\n", m.NumGC)
	}
	return vnil(), nil
}

// -------- make-load-form --------
func builtinMakeLoadForm(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("make-load-form: need an object")
	}
	// Default: return (make-load-form-saving-slots object) if the object is a structure
	if args[0].typ == VInstance {
		return vsym("MAKE-LOAD-FORM-SAVING-SLOTS"), nil
	}
	return vnil(), fmt.Errorf("make-load-form: cannot make load form for %s", typeStr(args[0]))
}

// -------- make-load-form-saving-slots --------
func builtinMakeLoadFormSavingSlots(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("make-load-form-saving-slots: need an object")
	}
	if args[0].typ != VInstance {
		return nil, fmt.Errorf("make-load-form-saving-slots: expected a structure")
	}
	inst := args[0]
	slotNames := listFromSlice([]*Value{})
	if inst.instClass != nil {
		// Try to get slot names from the class
		class := globalEnv.bindings[inst.instClass.str]
		if class != nil && class.typ == VClass {
			for _, sn := range class.classSlots {
				slotNames = cons(vstr(sn), slotNames)
			}
		}
	}
	// Return: (make-load-form-saving-slots-helper object slot-names)
	// For simplicity, return a list form
	return listFromSlice([]*Value{vsym("MAKE-LOAD-FORM-SAVING-SLOTS-HELPERS"), inst, slotNames}), nil
}

// docstrings stores documentation strings: map[symbolName_docType] -> docstring
var docstrings = make(map[string]string)

func builtinDocumentation(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("documentation: need symbol and doc-type")
	}
	sym := args[0]
	if sym.typ != VSym {
		return vnil(), nil
	}
	docType := ""
	if args[1].typ == VSym {
		docType = args[1].str
	}
	key := sym.str + "_" + docType
	if doc, ok := docstrings[key]; ok {
		return vstr(doc), nil
	}
	return vnil(), nil
}

func builtinApropos(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("apropos: need a string")
	}
	searchStr := ""
	if args[0].typ == VStr {
		searchStr = args[0].str
	} else if args[0].typ == VSym {
		searchStr = args[0].str
	} else {
		searchStr = ToString(args[0])
	}
	var results []*Value
	for name := range globalEnv.bindings {
		if strings.Contains(strings.ToLower(name), strings.ToLower(searchStr)) {
			results = append(results, vsym(name))
		}
	}
	// Sort results
	sort.Slice(results, func(i, j int) bool {
		return results[i].str < results[j].str
	})
	return listFromSlice(results), nil
}

func builtinAproposList(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("apropos-list: need a string")
	}
	searchStr := ""
	if args[0].typ == VStr {
		searchStr = args[0].str
	} else if args[0].typ == VSym {
		searchStr = args[0].str
	} else {
		searchStr = ToString(args[0])
	}
	var results []*Value
	for name := range globalEnv.bindings {
		if strings.Contains(strings.ToLower(name), strings.ToLower(searchStr)) {
			results = append(results, vsym(name))
		}
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].str < results[j].str
	})
	return listFromSlice(results), nil
}

func builtinCompile(args []*Value) (*Value, error) {
	// (compile name &optional definition)
	// In SBCL, compile returns (values compiled-fn warnings-p failure-p)
	// For MicroLisp, we return the function as-is (it's already interpreted)
	var name *Value
	var def *Value
	if len(args) >= 1 {
		name = args[0]
	}
	if len(args) >= 2 {
		def = args[1]
	}
	if name != nil && name.typ == VNil {
		name = nil
	}
	if name != nil && name.typ == VSym && def != nil {
		// Compile a lambda definition
		if def.typ == VPair && def.car != nil && def.car.typ == VSym && def.car.str == "LAMBDA" {
			// It's a lambda, eval it and return
			result, err := Eval(def, globalEnv)
			if err != nil {
				return vnil(), nil
			}
			// Set the function binding
			if name.typ == VSym {
				globalEnv.Set(name.str, result)
			}
			// Return (values result nil nil)
			return multiVal(result, vnil(), vnil()), nil
		}
	}
	if name != nil && name.typ == VSym {
		// Look up existing function
		val, err := globalEnv.Get(name.str)
		if err == nil {
			// Return (values val nil nil)
			return multiVal(val, vnil(), vnil()), nil
		}
	}
	if def != nil && def.typ == VPair && def.car != nil && def.car.typ == VSym && def.car.str == "LAMBDA" {
		result, err := Eval(def, globalEnv)
		if err != nil {
			return vnil(), nil
		}
		return multiVal(result, vnil(), vnil()), nil
	}
	return vnil(), nil
}

func builtinDisassemble(args []*Value) (*Value, error) {
	// (disassemble fn) -- returns a string describing the function
	if len(args) < 1 {
		return nil, fmt.Errorf("disassemble: need a function")
	}
	fn := primaryValue(args[0])
	var sb strings.Builder
	switch fn.typ {
	case VNil:
		sb.WriteString("#<NULL>\n")
	case VPrim:
		sb.WriteString("; This is a built-in function.\n")
		sb.WriteString("; Disassembly not available for primitives.\n")
	case VFunc:
		sb.WriteString("; Lambda Expression:\n")
		if fn.name != "" {
			sb.WriteString("; Name: ")
			sb.WriteString(fn.name)
			sb.WriteString("\n")
		}
		sb.WriteString("; Parameters: (")
		for i, p := range fn.params {
			if i > 0 {
				sb.WriteString(" ")
			}
			sb.WriteString(p)
		}
		if fn.rest != "" {
			if len(fn.params) > 0 {
				sb.WriteString(" &rest ")
			} else {
				sb.WriteString("&rest ")
			}
			sb.WriteString(fn.rest)
		}
		sb.WriteString(")\n")
		sb.WriteString("; Environment: (closure)\n")
		sb.WriteString("; Compiled: No (interpreted)\n")
	case VMacro:
		sb.WriteString("; This is a macro.\n")
		sb.WriteString("; Name: ")
		sb.WriteString(fn.name)
		sb.WriteString("\n")
	case VInstance:
		if fn.instClass != nil {
			sb.WriteString("; Instance of class: ")
			sb.WriteString(fn.instClass.str)
			sb.WriteString("\n")
		}
	default:
		sb.WriteString("; Not a function.\n")
	}
	fmt.Print(sb.String())
	return vnil(), nil
}

func builtinReplace(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("replace: need at least two sequences")
	}
	target := args[0]
	source := args[1]
	start1 := 0
	end1 := -1
	start2 := 0
	end2 := -1
	for i := 2; i+1 < len(args); i += 2 {
		key := primaryValue(args[i])
		if key.typ == VSym {
			switch key.str {
			case ":START1":
				start1 = int(primaryValue(args[i+1]).num)
			case ":END1":
				end1 = int(primaryValue(args[i+1]).num)
			case ":START2":
				start2 = int(primaryValue(args[i+1]).num)
			case ":END2":
				end2 = int(primaryValue(args[i+1]).num)
			}
		}
	}
	switch target.typ {
	case VStr:
		ts := []rune(target.str)
		tLen := len(ts)
		if end1 < 0 || end1 > tLen {
			end1 = tLen
		}
		if start1 < 0 {
			start1 = 0
		}
		ss := []rune(source.str)
		sLen := len(ss)
		if end2 < 0 || end2 > sLen {
			end2 = sLen
		}
		if start2 < 0 {
			start2 = 0
		}
		j := start2
		for i := start1; i < end1 && j < end2; i++ {
			ts[i] = ss[j]
			j++
		}
		target.str = string(ts)
		return target, nil
	case VArray:
		te := target.array.elements
		tLen := len(te)
		if end1 < 0 || end1 > tLen {
			end1 = tLen
		}
		if start1 < 0 {
			start1 = 0
		}
		se := seqToList(source)
		sLen := len(se)
		if end2 < 0 || end2 > sLen {
			end2 = sLen
		}
		if start2 < 0 {
			start2 = 0
		}
		j := start2
		for i := start1; i < end1 && j < end2; i++ {
			te[i] = se[j]
			j++
		}
		return target, nil
	case VPair:
		// For lists, rebuild with replaced portion
		if end1 < 0 {
			end1 = 999999
		}
		result := vnil()
		var tail **Value = &result
		idx := 0
		srcList := seqToList(source)
		sLen := len(srcList)
		if end2 < 0 || end2 > sLen {
			end2 = sLen
		}
		if start2 < 0 {
			start2 = 0
		}
		for i := 0; i < start2; i++ {
			// Copy source elements before replacement
		}
		cur := target
		idx = 0
		for !isNil(cur) && idx < start1 {
			*tail = cons(cur.car, vnil())
			tail = &((*tail).cdr)
			cur = cur.cdr
			idx++
		}
		// Skip target elements in range
		for !isNil(cur) && idx < end1 {
			cur = cur.cdr
			idx++
		}
		// Insert source elements
		for j := start2; j < end2 && j < sLen; j++ {
			*tail = cons(srcList[j], vnil())
			tail = &((*tail).cdr)
		}
		// Append remaining target
		for !isNil(cur) {
			*tail = cons(cur.car, vnil())
			tail = &((*tail).cdr)
			cur = cur.cdr
		}
		*tail = vnil()
		return result, nil
	default:
		return nil, fmt.Errorf("replace: not a sequence")
	}
}

func builtinParseInteger(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("parse-integer: need a string")
	}
	strVal := primaryValue(args[0])
	var str string
	if strVal.typ == VStr {
		str = strVal.str
	} else {
		str = ToString(strVal)
	}
	start := 0
	end := -1
	radix := 10
	junkAllowed := false
	for i := 1; i+1 < len(args); i += 2 {
		key := primaryValue(args[i])
		if key.typ == VSym {
			switch key.str {
			case ":START":
				start = int(primaryValue(args[i+1]).num)
			case ":END":
				end = int(primaryValue(args[i+1]).num)
			case ":RADIX":
				radix = int(primaryValue(args[i+1]).num)
			case ":JUNK-ALLOWED":
				junkAllowed = !isNil(args[i+1])
			}
		}
	}
	if end < 0 {
		end = len(str)
	}
	s := strings.TrimSpace(str[start:end])
	s = strings.ReplaceAll(s, "_", "")
	if s == "" || s == "-" || s == "+" {
		if junkAllowed {
			return vnil(), nil
		}
		return nil, fmt.Errorf("parse-integer: no integer at position %d", start)
	}
	// Position: count from start, skip whitespace then count sign+digits
	pos := start
	for pos < len(str) && (str[pos] == ' ' || str[pos] == '\t' || str[pos] == '\n' || str[pos] == '\r') {
		pos++
	}
	// s contains sign+digits (or just digits); len(s) counts all
	pos += len(s)

	n, err := strconv.ParseInt(s, radix, 64)
	if err != nil {
		// Try to parse as much as possible for junk-allowed
		if junkAllowed {
			// Find longest valid prefix of s
			for l := len(s); l > 0; l-- {
				partial, e2 := strconv.ParseInt(s[:l], radix, 64)
				if e2 == nil {
					p := start
					for p < len(str) && (str[p] == ' ' || str[p] == '\t' || str[p] == '\n' || str[p] == '\r') {
						p++
					}
					p += l
					return multiVal(vnum(float64(partial)), vnum(float64(p))), nil
				}
			}
			return vnil(), nil
		}
		return nil, fmt.Errorf("parse-integer: not an integer")
	}
	return multiVal(vnum(float64(n)), vnum(float64(pos))), nil
}

func builtinDigitCharP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	radix := 10
	if len(args) > 1 {
		radix = int(toNum(args[1]))
	}
	c := primaryValue(args[0])
	if c.typ != VChar {
		return vnil(), nil
	}
	d := unicode.ToLower(c.ch)
	baseDigit := '0'
	radixChar := rune('a' + radix - 10)
	if d >= baseDigit && d < baseDigit+rune(radix) {
		return vnum(float64(int(d - baseDigit))), nil
	}
	if d >= 'a' && d < radixChar {
		return vnum(float64(int(d - 'a' + 10))), nil
	}
	return vnil(), nil
}

func builtinAlphanumericP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	c := primaryValue(args[0])
	if c.typ == VChar {
		return vbool(unicode.IsLetter(c.ch) || unicode.IsDigit(c.ch)), nil
	}
	return vbool(false), nil
}

func builtinCharUpcase(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("char-upcase: need a character")
	}
	c := primaryValue(args[0])
	if c.typ != VChar {
		return nil, fmt.Errorf("char-upcase: not a character")
	}
	return vchar(unicode.ToUpper(c.ch)), nil
}

func builtinCharDowncase(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("char-downcase: need a character")
	}
	c := primaryValue(args[0])
	if c.typ != VChar {
		return nil, fmt.Errorf("char-downcase: not a character")
	}
	return vchar(unicode.ToLower(c.ch)), nil
}

func builtinReadSequence(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("read-sequence: need sequence and stream")
	}
	seq := args[0]
	stream := args[1]
	if stream.typ != VStream {
		return nil, fmt.Errorf("read-sequence: second argument must be a stream")
	}
	start := 0
	end := -1
	for i := 2; i+1 < len(args); i += 2 {
		key := primaryValue(args[i])
		if key.typ == VSym {
			switch key.str {
			case ":START":
				start = int(primaryValue(args[i+1]).num)
			case ":END":
				end = int(primaryValue(args[i+1]).num)
			}
		}
	}
	switch seq.typ {
	case VStr:
		s := seq.str
		runes := []rune(s)
		total := len(runes)
		if end < 0 || end > total {
			end = total
		}
		if start < 0 {
			start = 0
		}
		count := 0
		for i := start; i < end; i++ {
			r, err := stream.stream.readChar()
			if err != nil {
				break
			}
			runes[i] = r
			count++
		}
		seq.str = string(runes)
		return vnum(float64(start + count)), nil
	case VArray:
		arr := seq.array
		total := len(arr.elements)
		if end < 0 || end > total {
			end = total
		}
		if start < 0 {
			start = 0
		}
		count := 0
		for i := start; i < end; i++ {
			r, err := stream.stream.readChar()
			if err != nil {
				break
			}
			arr.elements[i] = vstr(string(r))
			count++
		}
		return vnum(float64(start + count)), nil
	case VPair:
		lst := seq
		seen := make(map[*Value]bool)
		idx := 0
		count := 0
		for !isNil(lst) {
			if seen[lst] {
				break
			}
			seen[lst] = true
			if idx >= start && (end < 0 || idx < end) {
				r, err := stream.stream.readChar()
				if err != nil {
					break
				}
				lst.car = vstr(string(r))
				count++
			}
			lst = lst.cdr
			idx++
		}
		return vnum(float64(start + count)), nil
	default:
		return nil, fmt.Errorf("read-sequence: not a sequence")
	}
}

func builtinWriteSequence(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("write-sequence: need a sequence")
	}
	seq := args[0]
	stream := stdoutStream
	if len(args) > 1 {
		stream = args[1]
		if stream.typ != VStream {
			return nil, fmt.Errorf("write-sequence: not a stream")
		}
	}
	start := 0
	end := -1
	for i := 2; i+1 < len(args); i += 2 {
		key := primaryValue(args[i])
		if key.typ == VSym {
			switch key.str {
			case ":START":
				start = int(primaryValue(args[i+1]).num)
			case ":END":
				end = int(primaryValue(args[i+1]).num)
			}
		}
	}
	switch seq.typ {
	case VStr:
		s := seq.str
		if end < 0 || end > len(s) {
			end = len(s)
		}
		if start < 0 {
			start = 0
		}
		if err := stream.stream.writeString(s[start:end]); err != nil {
			return nil, err
		}
		stream.stream.flush()
		return seq, nil
	case VArray:
		arr := seq.array
		total := len(arr.elements)
		if end < 0 || end > total {
			end = total
		}
		if start < 0 {
			start = 0
		}
		for i := start; i < end; i++ {
			if err := stream.stream.writeString(ToString(primaryValue(arr.elements[i]))); err != nil {
				return nil, err
			}
		}
		stream.stream.flush()
		return seq, nil
	case VPair:
		lst := seq
		seen := make(map[*Value]bool)
		idx := 0
		for !isNil(lst) {
			if seen[lst] {
				break
			}
			seen[lst] = true
			if idx >= start && (end < 0 || idx < end) {
				if err := stream.stream.writeString(ToString(primaryValue(lst.car))); err != nil {
					return nil, err
				}
			}
			lst = lst.cdr
			idx++
		}
		stream.stream.flush()
		return seq, nil
	default:
		return nil, fmt.Errorf("write-sequence: not a sequence")
	}
}

// subtypepChecks recursively checks subtype relationship between two type specifier Values.
// It returns (isKnown, isSubtype).
func subtypepChecks(v1, v2 *Value) (bool, bool) {
	typeName := func(v *Value) string {
		if v.typ == VSym {
			return v.str
		}
		if v.typ == VPair && v.car != nil && v.car.typ == VSym {
			return v.car.str
		}
		return ""
	}

	simpleSubtype := func(t1, t2 string) bool {
		if t1 == t2 {
			return true
		}
		types := map[string][]string{"INTEGER": {"RATIONAL", "REAL", "NUMBER", "ATOM", "T"}, "FLOAT": {"REAL", "NUMBER", "ATOM", "T"}, "RATIONAL": {"REAL", "NUMBER", "ATOM", "T"}, "COMPLEX": {"NUMBER", "ATOM", "T"}, "REAL": {"NUMBER", "ATOM", "T"}, "RATIO": {"RATIONAL", "REAL", "NUMBER", "ATOM", "T"}, "FIXNUM": {"INTEGER", "RATIONAL", "REAL", "NUMBER", "ATOM", "T"}, "BIGNUM": {"INTEGER", "RATIONAL", "REAL", "NUMBER", "ATOM", "T"}, "BIT": {"INTEGER", "RATIONAL", "REAL", "NUMBER", "ATOM", "T"}, "SHORT-FLOAT": {"FLOAT", "REAL", "NUMBER", "ATOM", "T"}, "SINGLE-FLOAT": {"FLOAT", "REAL", "NUMBER", "ATOM", "T"}, "DOUBLE-FLOAT": {"FLOAT", "REAL", "NUMBER", "ATOM", "T"}, "LONG-FLOAT": {"FLOAT", "REAL", "NUMBER", "ATOM", "T"}, "STRING": {"ARRAY", "VECTOR", "SEQUENCE", "ATOM", "T"}, "SIMPLE-STRING": {"STRING", "ARRAY", "VECTOR", "SEQUENCE", "ATOM", "T"}, "CHARACTER": {"ATOM", "T"}, "BASE-CHAR": {"CHARACTER", "ATOM", "T"}, "STANDARD-CHAR": {"BASE-CHAR", "CHARACTER", "ATOM", "T"}, "EXTENDED-CHAR": {"CHARACTER", "ATOM", "T"}, "SYMBOL": {"ATOM", "T"}, "KEYWORD": {"SYMBOL", "ATOM", "T"}, "NULL": {"SYMBOL", "LIST", "SEQUENCE", "ATOM", "T"}, "CONS": {"LIST", "SEQUENCE", "T"}, "PAIR": {"CONS", "LIST", "SEQUENCE", "T"}, "LIST": {"SEQUENCE", "T"}, "SEQUENCE": {"T"}, "VECTOR": {"ARRAY", "SEQUENCE", "T"}, "SIMPLE-VECTOR": {"VECTOR", "ARRAY", "SEQUENCE", "T"}, "ARRAY": {"T"}, "FUNCTION": {"T"}, "COMPILED-FUNCTION": {"FUNCTION", "T"}, "HASH-TABLE": {"T"}, "STREAM": {"T"}, "PACKAGE": {"T"}, "PATHNAME": {"T"}, "RANDOM-STATE": {"T"}, "READTABLE": {"T"}, "INSTANCE": {"T"}, "STRUCTURE": {"INSTANCE", "T"}, "METHOD": {"T"}, "BOOLEAN": {"ATOM", "T"}}
		if subtypes, ok := types[t1]; ok {
			for _, s := range subtypes {
				if s == t2 {
					return true
				}
			}
		}
		return false
	}

	n1 := strings.ToUpper(typeName(v1))
	n2 := strings.ToUpper(typeName(v2))

	// If both are simple type names
	if v1.typ == VSym && v2.typ == VSym {
		if n1 == n2 {
			return true, true
		}
		// * is universal type
		if n1 == "*" {
			if n2 == "*" || n2 == "T" {
				return true, true
			}
			return true, false
		}
		if n2 == "*" || n2 == "T" {
			return true, true
		}
		n1u := strings.ToUpper(n1)
		n2u := strings.ToUpper(n2)
		if simpleSubtype(n1u, n2u) {
			return true, true
		}
		cls1 := findClass(n1)
		cls2 := findClass(n2)
		if cls1 != nil && cls2 != nil {
			if classHasAncestor(cls1, n2) {
				return true, true
			}
		}
		return true, false
	}

	// Handle compound (cons ...) types
	if n1 == "CONS" || n2 == "CONS" {
		extractConsParts := func(v *Value) (carType *Value, cdrType *Value) {
			cdrType = vsym("*")
			if v.typ == VSym {
				carType = vsym("*")
				return
			}
			if v.typ == VPair && v.car != nil && v.car.typ == VSym && v.car.str == "CONS" {
				rest := v.cdr
				if rest != nil && !isNil(rest) && rest.typ == VPair {
					carType = rest.car
					rest = rest.cdr
					if rest != nil && !isNil(rest) && rest.typ == VPair {
						cdrType = rest.car
					}
				} else {
					carType = vsym("*")
				}
			}
			return
		}

		ct1, cd1 := extractConsParts(v1)
		ct2, cd2 := extractConsParts(v2)

		if ct1 != nil && ct2 != nil {
			isCarKnown, isCarSub := subtypepChecks(ct1, ct2)
			isCdrKnown, isCdrSub := subtypepChecks(cd1, cd2)
			if !isCarKnown || !isCdrKnown {
				return false, false
			}
			if isCarSub && isCdrSub {
				return true, true
			}
			return true, false
		}
		return false, false
	}

	// If t2 is t or *, anything is a subtype
	if n2 == "T" || n2 == "*" {
		return true, true
	}

	return false, false
}

func builtinSubtypep(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return multiVal(vbool(false), vbool(false)), nil
	}
	t1v := primaryValue(args[0])
	t2v := primaryValue(args[1])

	isSub, result := subtypepChecks(t1v, t2v)
	if isSub && result {
		return multiVal(vbool(true), vbool(true)), nil
	}
	if isSub && !result {
		return multiVal(vbool(false), vbool(true)), nil
	}
	return multiVal(vbool(false), vbool(false)), nil
}

var traceTable = make(map[string]bool)
var traceDepth = 0

func builtinTrace(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("trace: need function name")
	}
	var result []*Value
	for _, arg := range args {
		v := primaryValue(arg)
		name := ""
		switch v.typ {
		case VFunc, VPrim:
			name = v.name
		case VSym:
			name = v.str
		}
		if name != "" {
			traceTable[name] = true
			result = append(result, vsym(name))
		}
	}
	return listFromSlice(result), nil
}

func builtinUntrace(args []*Value) (*Value, error) {
	if len(args) < 1 {
		names := make([]string, 0, len(traceTable))
		for name := range traceTable {
			names = append(names, name)
		}
		traceTable = make(map[string]bool)
		result := make([]*Value, len(names))
		for i, n := range names {
			result[i] = vsym(n)
		}
		return listFromSlice(result), nil
	}
	var result []*Value
	for _, arg := range args {
		v := primaryValue(arg)
		name := ""
		switch v.typ {
		case VFunc, VPrim:
			name = v.name
		case VSym:
			name = v.str
		}
		if name != "" {
			delete(traceTable, name)
		}
		result = append(result, vsym(name))
	}
	return listFromSlice(result), nil
}

func builtinMakeCondVar(args []*Value) (*Value, error) {
	cid := atomic.AddInt64(&nextCondID, 1)
	condMu.Lock()
	// Create a condition variable associated with no specific lock initially
	// When wait is called with a lock, we associate the cond with that lock
	condVars[cid] = sync.NewCond(&sync.Mutex{})
	condMu.Unlock()
	return &Value{typ: VCondition, num: float64(cid)}, nil
}

func builtinConditionWait(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("condition-wait: need condition and lock")
	}
	if args[0].typ != VCondition || args[1].typ != VLock {
		return nil, fmt.Errorf("condition-wait: need condition and lock objects")
	}
	cid := int64(args[0].num)
	lid := int64(args[1].num)
	// Release the user lock, wait on condition, then reacquire
	condMu.Lock()
	cv, ok := condVars[cid]
	condMu.Unlock()
	if !ok {
		return nil, fmt.Errorf("condition-wait: invalid condition")
	}
	// The condition variable uses its own internal mutex for signaling
	cv.L.Lock()
	// Signal that we're about to wait (release user lock)
	lockMapMu.Lock()
	userMu, ok2 := lockMutexMap[lid]
	lockMapMu.Unlock()
	if !ok2 {
		return nil, fmt.Errorf("condition-wait: invalid lock")
	}
	userMu.Unlock() // Release the user lock
	cv.Wait()       // Wait on condition
	userMu.Lock()   // Reacquire the user lock
	return vnil(), nil
}

func builtinConditionNotify(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VCondition {
		return nil, fmt.Errorf("condition-notify: need a condition object")
	}
	cid := int64(args[0].num)
	condMu.Lock()
	cv, ok := condVars[cid]
	condMu.Unlock()
	if !ok {
		return nil, fmt.Errorf("condition-notify: invalid condition")
	}
	cv.Signal()
	return vnil(), nil
}

func builtinConditionBroadcast(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VCondition {
		return nil, fmt.Errorf("condition-broadcast: need a condition object")
	}
	cid := int64(args[0].num)
	condMu.Lock()
	cv, ok := condVars[cid]
	condMu.Unlock()
	if !ok {
		return nil, fmt.Errorf("condition-broadcast: invalid condition")
	}
	cv.Broadcast()
	return vnil(), nil
}

// Thread/Lock/Condition predicates
func builtinThreadP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	return vbool(args[0].typ == VThread), nil
}

func builtinLockP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	return vbool(args[0].typ == VLock), nil
}

func builtinCondVarP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	return vbool(args[0].typ == VCondition), nil
}

// Atomic operations
func builtinAtomicIncf(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("atomic-incf: need a reference")
	}
	delta := int64(1)
	if len(args) >= 2 {
		delta = int64(primaryValue(args[1]).num)
	}
	newVal := atomic.AddInt64(&atomicCounter, delta)
	return vnum(float64(newVal)), nil
}

func builtinAtomicDecf(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("atomic-decf: need a reference")
	}
	delta := int64(1)
	if len(args) >= 2 {
		delta = int64(primaryValue(args[1]).num)
	}
	newVal := atomic.AddInt64(&atomicCounter, -delta)
	return vnum(float64(newVal)), nil
}

func builtinAtomicGet(args []*Value) (*Value, error) {
	return vnum(float64(atomic.LoadInt64(&atomicCounter))), nil
}

func builtinAtomicSet(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("atomic-set: need a value")
	}
	val := int64(primaryValue(args[0]).num)
	atomic.StoreInt64(&atomicCounter, val)
	return vnum(float64(val)), nil
}

func builtinEval(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("eval: need expression")
	}
	return Eval(args[0], globalEnv)
}

func builtinEvalString(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("eval-string: need a string")
	}
	s := primaryValue(args[0])
	if s.typ != VStr {
		return nil, fmt.Errorf("eval-string: need a string, got %s", ToString(s))
	}
	return EvalString(s.str, globalEnv)
}

func builtinHandlerEval(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("handler-eval: need expression")
	}
	result, err := Eval(args[0], globalEnv)
	if err != nil {
		// Signal as a condition so handler-case can catch it
		return builtinError([]*Value{vstr(err.Error())})
	}
	return result, nil
}

func builtinFuncall(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("funcall: need function")
	}
	fn := args[0]
	callArgs := args[1:]
	return callFnOnSeq(fn, callArgs, globalEnv)
}

// eql: like eq, but numbers and characters with same value are equal
func builtinEql(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return vbool(false), nil
	}
	return vbool(eqVal(args[0], args[1])), nil
}

// eq: identity equality (pointer/symbol)
func builtinEqIdentity(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return vbool(false), nil
	}
	a, b := args[0], args[1]
	if a == b {
		return vbool(true), nil
	}
	// In CL, nil (symbol) and () (empty list/VNil) are eq
	if isNil(a) || isNil(b) {
		if (a.typ == VSym && strings.EqualFold(a.str, "nil") && b.typ == VNil) ||
			(b.typ == VSym && strings.EqualFold(b.str, "nil") && a.typ == VNil) {
			return vbool(true), nil
		}
	}
	return vbool(false), nil
}

// equal: structural equality (recursive)
func builtinEqual(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return vbool(false), nil
	}
	return vbool(eqVal(args[0], args[1])), nil
}

// equalp: case-insensitive string/char comparison, numeric equality, recursive
func builtinEqualp(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return vbool(false), nil
	}
	return vbool(equalpVal(args[0], args[1])), nil
}

func equalpVal(a, b *Value) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	// Numbers: same mathematical value
	if isNumeric(a) && isNumeric(b) {
		return equalpNumeric(a, b)
	}
	// Characters: case-insensitive
	if a.typ == VChar && b.typ == VChar {
		ac, bc := unicode.ToLower(a.ch), unicode.ToLower(b.ch)
		return ac == bc
	}
	// Strings: case-insensitive
	if a.typ == VStr && b.typ == VStr {
		return strings.EqualFold(a.str, b.str)
	}
	// Lists: recursively compare
	if a.typ == VPair && b.typ == VPair {
		return equalpVal(a.car, b.car) && equalpVal(a.cdr, b.cdr)
	}
	// Symbols: case-insensitive
	if a.typ == VSym && b.typ == VSym {
		return strings.EqualFold(a.str, b.str)
	}
	// Arrays: element-wise comparison
	if a.typ == VArray && b.typ == VArray {
		return equalpArray(a, b)
	}
	// Otherwise use eql
	return eqVal(a, b)
}

func equalpNumeric(a, b *Value) bool {
	switch a.typ {
	case VNum:
		switch b.typ {
		case VNum:
			return a.num == b.num
		case VRat:
			return float64(a.num) == float64(b.irat)/float64(b.iden)
		}
	case VRat:
		switch b.typ {
		case VNum:
			return float64(a.irat)/float64(a.iden) == b.num
		case VRat:
			return a.irat == b.irat && a.iden == b.iden
		}
	case VComplex:
		if b.typ == VComplex {
			return a.num == b.num && a.imag == b.imag
		}
	}

	return false
}

// -------- Array helpers --------

func equalpArray(a, b *Value) bool {
	if len(a.array.dims) != len(b.array.dims) {
		return false
	}
	for i := range a.array.dims {
		if a.array.dims[i] != b.array.dims[i] {
			return false
		}
	}
	if len(a.array.elements) != len(b.array.elements) {
		return false
	}
	for i := range a.array.elements {
		if !equalpVal(a.array.elements[i], b.array.elements[i]) {
			return false
		}
	}
	return true
}

func arrayToString(v *Value) string {
	if v.array == nil {
		return "#<array nil>"
	}
	arr := v.array
	if len(arr.dims) == 1 && arr.dims[0] == len(arr.elements) {
		// 1-D array (vector)
		parts := []string{"#("}
		for i, elem := range arr.elements {
			if i > 0 {
				parts = append(parts, " ")
			}
			if elem == nil || elem.typ == VNil {
				parts = append(parts, "NIL")
			} else if elem.typ == VStr {
				parts = append(parts, "\""+elem.str+"\"")
			} else {
				parts = append(parts, ToString(elem))
			}
		}
		parts = append(parts, ")")
		return strings.Join(parts, "")
	}
	// Multi-dimensional array
	dimStr := make([]string, len(arr.dims))
	for i, d := range arr.dims {
		dimStr[i] = strconv.Itoa(d)
	}
	return "#<" + strings.Join(dimStr, "x") + "-array>"
}

// Row-major index for given subscripts
func arrayRowMajorIndex(arr *LispArray, indices []int) (int, error) {
	if len(indices) != len(arr.dims) {
		return 0, fmt.Errorf("array access: expected %d subscripts, got %d", len(arr.dims), len(indices))
	}
	idx := 0
	stride := 1
	for i := len(arr.dims) - 1; i >= 0; i-- {
		if indices[i] < 0 || indices[i] >= arr.dims[i] {
			return 0, fmt.Errorf("array index %d out of bounds [0..%d] for dimension %d", indices[i], arr.dims[i]-1, i)
		}
		idx += indices[i] * stride
		stride *= arr.dims[i]
	}
	return idx, nil
}

// Total size of array
func arrayTotalSize(dims []int) int {
	size := 1
	for _, d := range dims {
		size *= d
	}
	return size
}

// null:

// -------- Array builtins --------

// arrayFillRecursive flattens nested lists into the flat elements array in row-major order.
func arrayFillRecursive(contents *Value, dims []int, elements []*Value, idx *int, visited map[*Value]bool) {
	if len(dims) == 1 {
		// Leaf level: fill elements directly
		// Support both VPair (list) and VArray (vector) as contents
		if contents.typ == VArray {
			arr := contents.array
			end := len(arr.elements)
			if arr.fillPtr >= 0 && arr.fillPtr < end {
				end = arr.fillPtr
			}
			for i := 0; i < end; i++ {
				if *idx >= len(elements) {
					return
				}
				elem := arr.elements[i]
				if elem == nil {
					elem = vnil()
				}
				elements[*idx] = elem
				*idx++
			}
		} else {
			for !isNil(contents) {
				if *idx >= len(elements) {
					return
				}
				if contents.typ == VPair {
					if visited[contents] {
						return // cycle detected
					}
					visited[contents] = true
				}
				elements[*idx] = contents.car
				*idx++
				contents = contents.cdr
			}
		}
	} else {
		// Nested: recurse into sublists
		// Support both VPair (list) and VArray (vector) as contents
		if contents.typ == VArray {
			arr := contents.array
			end := len(arr.elements)
			if arr.fillPtr >= 0 && arr.fillPtr < end {
				end = arr.fillPtr
			}
			for i := 0; i < end; i++ {
				subDims := dims[1:]
				elem := arr.elements[i]
				if elem == nil {
					elem = vnil()
				}
				arrayFillRecursive(elem, subDims, elements, idx, visited)
			}
		} else {
			for !isNil(contents) {
				if contents.typ == VPair {
					if visited[contents] {
						return // cycle detected
					}
					visited[contents] = true
				}
				subDims := dims[1:]
				arrayFillRecursive(contents.car, subDims, elements, idx, visited)
				contents = contents.cdr
			}
		}
	}
}

func builtinMakeArray(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("make-array: need dimensions")
	}
	dimArg := args[0]
	var dims []int
	if dimArg.typ == VNum {
		dims = []int{int(dimArg.num)}
	} else if dimArg.typ == VPair {
		for !isNil(dimArg) {
			if dimArg.car == nil || dimArg.car.typ != VNum {
				return nil, fmt.Errorf("make-array: dimension must be integer")
			}
			dims = append(dims, int(dimArg.car.num))
			dimArg = dimArg.cdr
		}
	} else {
		return nil, fmt.Errorf("make-array: dimensions must be integer or list")
	}
	if len(dims) == 0 {
		return nil, fmt.Errorf("make-array: need at least one dimension")
	}
	for i, d := range dims {
		if d < 0 {
			return nil, fmt.Errorf("make-array: dimension %d is negative: %d", i, d)
		}
	}
	// Parse keyword arguments
	initialElement := vnil()
	var initialContents *Value = nil
	fillPointer := -1
	adjustable := false
	elemType := "T"
	for i := 1; i < len(args); i++ {
		if args[i].typ == VSym {
			switch args[i].str {
			case ":INITIAL-ELEMENT":
				if i+1 < len(args) {
					i++
					initialElement = args[i]
				}
			case ":INITIAL-CONTENTS":
				if i+1 < len(args) {
					i++
					initialContents = args[i]
				}
			case ":FILL-POINTER":
				if i+1 < len(args) {
					i++
					if args[i] == globalEnv.bindings["#t"] {
						fillPointer = 0
					} else if args[i].typ == VNum {
						fillPointer = int(args[i].num)
					}
				}
			case ":ADJUSTABLE":
				if i+1 < len(args) {
					i++
					adjustable = isTruthy(args[i])
				}
			case ":ELEMENT-TYPE":
				if i+1 < len(args) {
					i++
					etName := strings.ToUpper(ToString(args[i]))
					switch etName {
					case "CHARACTER", "BASE-CHAR", "STANDARD-CHAR":
						elemType = "CHARACTER"
					case "BIT":
						elemType = "BIT"
					case "SINGLE-FLOAT", "DOUBLE-FLOAT", "FLOAT":
						elemType = "SINGLE-FLOAT"
					default:
						elemType = "T"
					}
				}
			}
		}
	}
	size := arrayTotalSize(dims)
	elements := make([]*Value, size)
	if initialContents != nil {
		idx := 0
		arrayFillRecursive(initialContents, dims, elements, &idx, make(map[*Value]bool))
		if idx < size {
			// Fill remaining with nil
			for i := idx; i < size; i++ {
				elements[i] = vnil()
			}
		}
	} else {
		for i := range elements {
			elements[i] = initialElement
		}
	}
	arr := &LispArray{
		dims:       dims,
		elements:   elements,
		fillPtr:    fillPointer,
		adjustable: adjustable,
		elemType:   elemType,
	}
	v := gcv()
	v.typ = VArray
	v.array = arr
	return v, nil
}

func builtinAref(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("aref: need array and subscripts")
	}
	arr := args[0]
	// Strings are also arrays in CL; (aref "hello" 0) returns a character
	if arr.typ == VStr {
		idx := int(args[1].num)
		if idx < 0 || idx >= len(arr.str) {
			return nil, fmt.Errorf("aref: index %d out of bounds for string of length %d", idx, len(arr.str))
		}
		ch := []rune(arr.str)[idx]
		return vchar(ch), nil
	}
	if arr.typ != VArray || arr.array == nil {
		return nil, fmt.Errorf("aref: first argument must be an array")
	}
	indices := make([]int, len(args)-1)
	for i := 1; i < len(args); i++ {
		if args[i].typ != VNum {
			return nil, fmt.Errorf("aref: subscript must be integer")
		}
		indices[i-1] = int(args[i].num)
	}
	idx, err := arrayRowMajorIndex(arr.array, indices)
	if err != nil {
		return nil, err
	}
	return arr.array.elements[idx], nil
}

func builtinSetAref(args []*Value) (*Value, error) {
	// (set-aref value array subscripts...)
	if len(args) < 3 {
		return nil, fmt.Errorf("set-aref: need value, array, and subscripts")
	}
	val := args[0]
	arr := args[1]
	if arr.typ != VArray || arr.array == nil {
		return nil, fmt.Errorf("set-aref: second argument must be an array")
	}
	indices := make([]int, len(args)-2)
	for i := 2; i < len(args); i++ {
		if args[i].typ != VNum {
			return nil, fmt.Errorf("set-aref: subscript must be integer")
		}
		indices[i-2] = int(args[i].num)
	}
	idx, err := arrayRowMajorIndex(arr.array, indices)
	if err != nil {
		return nil, err
	}
	arr.array.elements[idx] = val
	return val, nil
}

// builtinNthSetf is used for (setf (nth n list) val) -> (nth-setf val n list)
func builtinNthSetf(args []*Value) (*Value, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("setf (nth): need value, n, and list")
	}
	val := args[0]
	n := args[1]
	lst := args[2]
	if n.typ != VNum {
		return nil, fmt.Errorf("setf (nth): index must be a number")
	}
	idx := int(n.num)
	if idx < 0 {
		return nil, fmt.Errorf("setf (nth): index must be non-negative")
	}
	// Walk down to the nth element
	target := lst
	for i := 0; i < idx; i++ {
		if !isPair(target) {
			return nil, fmt.Errorf("setf (nth): index %d out of range", idx)
		}
		target = target.cdr
	}
	if !isPair(target) {
		return nil, fmt.Errorf("setf (nth): index %d out of range", idx)
	}
	target.car = val
	return val, nil
}

// builtinSymbolValueSetf is used for (setf (symbol-value sym) val)
func builtinSymbolValueSetf(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("setf (symbol-value): need value and symbol")
	}
	val := args[0]
	sym := args[1]
	if sym.typ != VSym {
		return nil, fmt.Errorf("setf (symbol-value): second argument must be a symbol")
	}
	if _, err := globalEnv.Get(sym.str); err != nil {
		return nil, fmt.Errorf("setf (symbol-value): symbol %s is unbound", sym.str)
	}
	globalEnv.Set(sym.str, val)
	return val, nil
}

// builtinEltSetf is used for (setf (elt seq n) val)
func builtinEltSetf(args []*Value) (*Value, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("setf (elt): need value, sequence, and index")
	}
	val := args[0]
	seq := args[1]
	idx := args[2]
	if idx.typ != VNum {
		return nil, fmt.Errorf("setf (elt): index must be a number")
	}
	i := int(idx.num)
	// Handle list sequences
	if isPair(seq) || isNil(seq) {
		target := seq
		for j := 0; j < i; j++ {
			if !isPair(target) {
				return nil, fmt.Errorf("setf (elt): index %d out of range", i)
			}
			target = target.cdr
		}
		if !isPair(target) {
			return nil, fmt.Errorf("setf (elt): index %d out of range", i)
		}
		target.car = val
		return val, nil
	}
	// Handle string sequences
	if seq.typ == VStr {
		if i < 0 || i >= len(seq.str) {
			return nil, fmt.Errorf("setf (elt): index %d out of range", i)
		}
		// Can't mutate strings in Go (immutable), return value anyway
		return val, nil
	}
	return nil, fmt.Errorf("setf (elt): not a sequence")
}

func builtinVector(args []*Value) (*Value, error) {
	arr := &LispArray{
		dims:     []int{len(args)},
		elements: make([]*Value, len(args)),
		fillPtr:  -1,
	}
	copy(arr.elements, args)
	v := gcv()
	v.typ = VArray
	v.array = arr
	return v, nil
}

func builtinArrayP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	return vbool(args[0].typ == VArray), nil
}

func builtinVectorP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	return vbool(args[0].typ == VArray && len(args[0].array.dims) == 1), nil
}

// isBitVector checks if a VArray contains only 0/1 integers.
func isBitVector(arr *LispArray) bool {
	if arr == nil {
		return false
	}
	for _, el := range arr.elements {
		if el == nil || el.typ == VNil {
			return false
		}
		if el.typ != VNum {
			return false
		}
		v := toNum(vnum(el.num))
		if v != 0 && v != 1 {
			return false
		}
	}
	return true
}

func builtinBitVectorP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	v := args[0]
	return vbool(v.typ == VArray && len(v.array.dims) == 1 && isBitVector(v.array)), nil
}

func builtinSimpleVectorP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	v := args[0]
	if v.typ != VArray || v.array == nil {
		return vbool(false), nil
	}
	// A simple vector is one-dimensional with no fill pointer
	return vbool(len(v.array.dims) == 1 && v.array.fillPtr < 0), nil
}

func builtinSimpleBitVectorP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	v := args[0]
	if v.typ != VArray || v.array == nil {
		return vbool(false), nil
	}
	if len(v.array.dims) != 1 || v.array.fillPtr >= 0 {
		return vbool(false), nil
	}
	return vbool(isBitVector(v.array)), nil
}

func builtinSimpleStringP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	v := args[0]
	// In CL, all strings are simple strings by default
	return vbool(v.typ == VStr), nil
}

func builtinArrayDimensions(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VArray || args[0].array == nil {
		return nil, fmt.Errorf("array-dimensions: need an array")
	}
	result := vnil()
	for i := len(args[0].array.dims) - 1; i >= 0; i-- {
		result = cons(vnum(float64(args[0].array.dims[i])), result)
	}
	return result, nil
}

func builtinArrayDimension(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("array-dimension: need array and axis-number")
	}
	v := args[0]
	if v.typ != VArray {
		return nil, fmt.Errorf("array-dimension: not an array")
	}
	axis := int(toNum(args[1]))
	if axis < 0 || axis >= len(v.array.dims) {
		return nil, fmt.Errorf("array-dimension: axis %d out of range", axis)
	}
	return vnum(float64(v.array.dims[axis])), nil
}

func builtinArrayTotalSize(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VArray || args[0].array == nil {
		return nil, fmt.Errorf("array-total-size: need an array")
	}
	return vnum(float64(len(args[0].array.elements))), nil
}

func builtinArrayRank(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VArray || args[0].array == nil {
		return nil, fmt.Errorf("array-rank: need an array")
	}
	return vnum(float64(len(args[0].array.dims))), nil
}

func builtinFillPointer(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VArray || args[0].array == nil {
		return nil, fmt.Errorf("fill-pointer: need a vector with fill-pointer")
	}
	arr := args[0].array
	if arr.fillPtr < 0 {
		return nil, fmt.Errorf("fill-pointer: array has no fill-pointer")
	}
	return vnum(float64(arr.fillPtr)), nil
}

func builtinSetFillPointer(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("set-fill-pointer: need array and value")
	}
	arr := args[0]
	if arr.typ != VArray || arr.array == nil {
		return nil, fmt.Errorf("set-fill-pointer: first argument must be an array")
	}
	if arr.array.fillPtr < 0 {
		return nil, fmt.Errorf("set-fill-pointer: array has no fill-pointer")
	}
	newVal := int(args[1].num)
	if newVal < 0 || newVal > len(arr.array.elements) {
		return nil, fmt.Errorf("set-fill-pointer: value %d out of range [0..%d]", newVal, len(arr.array.elements))
	}
	arr.array.fillPtr = newVal
	return args[1], nil
}

// fill-pointer-setf: for (setf (fill-pointer arr) val) -> (fill-pointer-setf val arr)
func builtinFillPointerSetf(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("(setf fill-pointer): need array and value")
	}
	newVal := int(args[0].num)
	arr := args[1]
	if arr.typ != VArray || arr.array == nil {
		return nil, fmt.Errorf("(setf fill-pointer): first arg must be an array")
	}
	if arr.array.fillPtr < 0 {
		return nil, fmt.Errorf("(setf fill-pointer): array has no fill-pointer")
	}
	if newVal < 0 || newVal > len(arr.array.elements) {
		return nil, fmt.Errorf("(setf fill-pointer): value %d out of range [0..%d]", newVal, len(arr.array.elements))
	}
	arr.array.fillPtr = newVal
	return args[0], nil
}

func builtinVectorPush(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("vector-push: need element and vector")
	}
	newEl := args[0]
	vec := args[1]
	if vec.typ != VArray || vec.array == nil {
		return nil, fmt.Errorf("vector-push: second argument must be a vector")
	}
	arr := vec.array
	if arr.fillPtr < 0 {
		return nil, fmt.Errorf("vector-push: vector has no fill-pointer")
	}
	if arr.fillPtr >= len(arr.elements) {
		return vnil(), nil // no room
	}
	arr.elements[arr.fillPtr] = newEl
	fp := arr.fillPtr
	arr.fillPtr++
	return vnum(float64(fp)), nil
}

func builtinVectorPop(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VArray || args[0].array == nil {
		return nil, fmt.Errorf("vector-pop: need a vector")
	}
	arr := args[0].array
	if arr.fillPtr <= 0 {
		return nil, fmt.Errorf("vector-pop: fill-pointer is 0")
	}
	arr.fillPtr--
	return arr.elements[arr.fillPtr], nil
}

func builtinArrayElementType(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("array-element-type: need an array")
	}
	arr := args[0]
	if arr.typ == VStr {
		// Strings are vectors of CHARACTER
		return vsym("CHARACTER"), nil
	}
	if arr.typ != VArray || arr.array == nil {
		return nil, fmt.Errorf("array-element-type: argument must be an array")
	}
	et := arr.array.elemType
	if et == "" {
		et = "T"
	}
	return vsym(et), nil
}

func builtinUpgradedArrayElementType(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("upgraded-array-element-type: need a type specifier")
	}
	typeArg := args[0]
	typeName := strings.ToUpper(ToString(typeArg))
	// Map known element types to their upgraded types
	switch typeName {
	case "CHARACTER", "BASE-CHAR", "STANDARD-CHAR":
		return vsym("CHARACTER"), nil
	case "BIT":
		return vsym("BIT"), nil
	case "SINGLE-FLOAT", "DOUBLE-FLOAT", "FLOAT", "SHORT-FLOAT", "LONG-FLOAT":
		return vsym("SINGLE-FLOAT"), nil
	case "FIXNUM", "BIGNUM", "INTEGER", "RATIONAL", "RATIO", "REAL", "NUMBER":
		return vsym("T"), nil
	default:
		return vsym("T"), nil
	}
}

// builtinUpgradedComplexPartType implements CL: UPGRADED-COMPLEX-PART-TYPE
// (upgraded-complex-part-type typespec)
// Returns the element type of the parts of a complex number created with the given typespec.
// In our implementation, float types upgrade to SINGLE-FLOAT; others remain as-is.
func builtinUpgradedComplexPartType(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("upgraded-complex-part-type: need a type specifier")
	}
	typeArg := args[0]
	typeName := strings.ToUpper(ToString(typeArg))
	switch typeName {
	case "SINGLE-FLOAT", "DOUBLE-FLOAT", "SHORT-FLOAT", "LONG-FLOAT", "FLOAT":
		return vsym("SINGLE-FLOAT"), nil
	case "RATIONAL", "RATIO", "INTEGER", "FIXNUM", "BIGNUM":
		return vsym("RATIONAL"), nil
	case "REAL", "NUMBER":
		return vsym("REAL"), nil
	default:
		return vsym(typeName), nil
	}
}

func builtinAdjustArray(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("adjust-array: need array and new dimensions")
	}
	arr := args[0]
	if arr.typ != VArray || arr.array == nil {
		return nil, fmt.Errorf("adjust-array: first argument must be an array")
	}
	dimArg := args[1]
	var newDims []int
	if dimArg.typ == VNum {
		newDims = []int{int(dimArg.num)}
	} else if dimArg.typ == VPair {
		for !isNil(dimArg) {
			if dimArg.car == nil || dimArg.car.typ != VNum {
				return nil, fmt.Errorf("adjust-array: dimension must be integer")
			}
			newDims = append(newDims, int(dimArg.car.num))
			dimArg = dimArg.cdr
		}
	}
	initialElement := vnil()
	for i := 2; i < len(args); i++ {
		if args[i].typ == VSym && args[i].str == ":INITIAL-ELEMENT" && i+1 < len(args) {
			i++
			initialElement = args[i]
		}
	}
	newSize := arrayTotalSize(newDims)
	newElements := make([]*Value, newSize)
	copy(newElements, arr.array.elements)
	for i := len(arr.array.elements); i < newSize; i++ {
		newElements[i] = initialElement
	}
	arr.array.dims = newDims
	arr.array.elements = newElements
	return arr, nil
}

func builtinArrayHasFillPointerP(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VArray || args[0].array == nil {
		return vbool(false), nil
	}
	return vbool(args[0].array.fillPtr >= 0), nil
}

func builtinAdjustableArrayP(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VArray || args[0].array == nil {
		return vbool(false), nil
	}
	return vbool(args[0].array.adjustable), nil
}

func builtinArrayDisplacement(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VArray || args[0].array == nil {
		return multiVal(vnil(), vbool(false)), nil
	}
	// microlisp does not support displaced arrays
	return multiVal(vnil(), vbool(false)), nil
}

func builtinArrayInBoundsP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("array-in-bounds-p: need an array")
	}
	v := args[0]
	if v.typ != VArray || v.array == nil {
		return nil, fmt.Errorf("array-in-bounds-p: not an array")
	}
	if len(args)-1 != len(v.array.dims) {
		return vbool(false), nil
	}
	for i := 0; i < len(v.array.dims); i++ {
		idx := int(toNum(args[i+1]))
		if idx < 0 || idx >= v.array.dims[i] {
			return vbool(false), nil
		}
	}
	return vbool(true), nil
}

func builtinArrayRowMajorIndex(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("array-row-major-index: need an array")
	}
	v := args[0]
	if v.typ != VArray || v.array == nil {
		return nil, fmt.Errorf("array-row-major-index: not an array")
	}
	if len(args)-1 != len(v.array.dims) {
		return nil, fmt.Errorf("array-row-major-index: wrong number of subscripts")
	}
	idx := 0
	for i := 0; i < len(v.array.dims); i++ {
		sub := int(toNum(args[i+1]))
		if sub < 0 || sub >= v.array.dims[i] {
			return nil, fmt.Errorf("array-row-major-index: subscript %d out of range", i)
		}
		idx = idx*v.array.dims[i] + sub
	}
	return vnum(float64(idx)), nil
}

func builtinNull(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(true), nil
	}
	return vbool(isNil(args[0])), nil
}

func builtinApply(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("apply: need function and list")
	}
	fn := args[0]
	// Resolve symbol to its function binding
	if fn.typ == VSym {
		val, err := globalEnv.Get(fn.str)
		if err != nil {
			return nil, fmt.Errorf("apply: %s has no function binding", fn.str)
		}
		fn = val
	}
	if fn.typ != VPrim && fn.typ != VFunc && fn.typ != VMacro && fn.typ != VGeneric {
		return nil, fmt.Errorf("apply: first arg must be a procedure, got %s", typeStr(fn))
	}
	// Build argument list from args[1..]
	// (apply proc arg1 ... argn list): last arg is the list, preceding are individual args
	var argList *Value
	if len(args) == 2 {
		argList = args[1]
	} else {
		argList = args[len(args)-1] // last arg IS the list of remaining args
		for i := len(args) - 2; i >= 1; i-- {
			argList = cons(args[i], argList)
		}
	}
	if argList.typ != VPair && argList.typ != VNil {
		return nil, fmt.Errorf("apply: last arg must be a list")
	}
	// Extract already-evaluated arg slice
	evalArgs := toSlice(argList)
	switch fn.typ {
	case VPrim:
		return fn.fn(evalArgs)
	case VFunc:
				newEnv := NewEnv(fn.env)
		numRequired := len(fn.params) - len(fn.optDefaults) - len(fn.keySpecs)
		if numRequired < 0 {
			numRequired = 0
		}

		keyVals := make(map[string]*Value)
		positionalArgs := evalArgs
		if len(fn.keySpecs) > 0 {
			var nonKeyword []*Value
			i := 0
			for i < len(evalArgs) {
				if evalArgs[i] != nil && evalArgs[i].typ == VSym && len(evalArgs[i].str) > 0 && evalArgs[i].str[0] == ':' {
					keyName := evalArgs[i].str[1:]
					if i+1 < len(evalArgs) {
						keyVals[keyName] = evalArgs[i+1]
						i += 2
					} else {
						nonKeyword = append(nonKeyword, evalArgs[i])
						i++
					}
				} else {
					nonKeyword = append(nonKeyword, evalArgs[i])
					i++
				}
			}
			positionalArgs = nonKeyword
		}

		for i := 0; i < numRequired; i++ {
			if i < len(positionalArgs) {
				newEnv.Set(fn.params[i], positionalArgs[i])
			} else {
				return nil, fmt.Errorf("apply: missing required argument")
			}
		}

		paramIdx := numRequired
		for _, defaultExpr := range fn.optDefaults {
			if paramIdx < len(positionalArgs) {
				newEnv.Set(fn.params[paramIdx], positionalArgs[paramIdx])
			} else if !isNil(defaultExpr) {
				defVal, err := Eval(defaultExpr, newEnv)
				if err != nil {
					return nil, err
				}
				newEnv.Set(fn.params[paramIdx], defVal)
			} else {
				newEnv.Set(fn.params[paramIdx], vnil())
			}
			paramIdx++
		}

		paramIdx = numRequired + len(fn.optDefaults)
		for _, spec := range fn.keySpecs {
			if spec == nil || spec.typ != VPair || spec.car == nil || spec.cdr == nil || spec.cdr.typ != VPair || spec.cdr.cdr == nil || spec.cdr.cdr.typ != VPair {
				paramIdx++
				continue
			}
			keyName := spec.car.str
			if len(keyName) > 0 && keyName[0] == ':' {
				keyName = keyName[1:]
			}
			paramName := spec.cdr.car.str
			defaultExpr := spec.cdr.cdr.car
			if val, ok := keyVals[keyName]; ok {
				newEnv.Set(paramName, val)
			} else if !isNil(defaultExpr) {
				defVal, err := Eval(defaultExpr, newEnv)
				if err != nil {
					return nil, err
				}
				newEnv.Set(paramName, defVal)
			} else {
				newEnv.Set(paramName, vnil())
			}
			paramIdx++
		}

		if fn.rest != "" {
			var restElems []*Value
			if len(fn.keySpecs) > 0 {
				restElems = positionalArgs[paramIdx:]
			} else if fn.optDefaults != nil {
				restElems = positionalArgs[len(fn.params)-len(fn.optDefaults):]
			} else {
				restElems = positionalArgs[len(fn.params):]
			}
			newEnv.Set(fn.rest, listFromSlice(restElems))
		}
		body := fn.body
		if isNil(body) {
			return vnil(), nil
		}
		for body.typ == VPair && !isNil(body.cdr) {
			_, e := Eval(body.car, newEnv)
			if e != nil {
				return nil, e
			}
			body = body.cdr
		}
		return Eval(body.car, newEnv)
	case VMacro:
		expanded, e := expandMacro(fn, argList, globalEnv)
		if e != nil {
			return nil, e
		}
		return Eval(expanded, globalEnv)
	}
	return nil, fmt.Errorf("apply: unsupported procedure type")
}

func builtinDefinedP(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VSym {
		return vbool(false), nil
	}
	_, err := globalEnv.Get(args[0].str)
	return vbool(err == nil), nil
}

func builtinFdefinition(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("fdefinition: need a function name")
	}
	name := args[0]
	if name.typ == VSym {
		fn := globalEnvGetFunction(name.str)
		if fn != nil {
			return fn, nil
		}
		return nil, fmt.Errorf("fdefinition: %s is not a function", name.str)
	}
	return nil, fmt.Errorf("fdefinition: expected a symbol")
}

func builtinCompileFile(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("compile-file: need a pathname")
	}
	path := args[0]
	var fileStr string
	if path.typ == VStr {
		fileStr = path.str
	} else if path.typ == VPathname {
		fileStr = pathnameToString(path.pathname)
	} else {
		return nil, fmt.Errorf("compile-file: need a pathname or string")
	}
	// Read and parse the file (MicroLisp has no native code compiler, so we just parse)
	data, err := os.ReadFile(fileStr)
	if err != nil {
		return nil, fmt.Errorf("compile-file: could not read %s: %v", fileStr, err)
	}
	forms, perr := parseAll(string(data))
	if perr != nil {
		return nil, fmt.Errorf("compile-file: parse error in %s: %v", fileStr, perr)
	}
	for !isNil(forms) {
		_, err := Eval(forms.car, globalEnv)
		if err != nil {
			return nil, fmt.Errorf("compile-file: error in %s: %v", fileStr, err)
		}
		forms = forms.cdr
	}
	// Return output pathname (append .fas to input name)
	outputPath := fileStr
	if !strings.HasSuffix(outputPath, ".lisp") && !strings.HasSuffix(outputPath, ".lsp") {
		outputPath += ".fas"
	} else if strings.HasSuffix(outputPath, ".lisp") {
		outputPath = outputPath[:len(outputPath)-5] + ".fas"
	} else {
		outputPath = outputPath[:len(outputPath)-4] + ".fas"
	}
	p := parsePathnameString(outputPath)
	v := gcv()
	v.typ = VPathname
	v.pathname = p
	return v, nil
}

func builtinCompileFilePathname(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("compile-file-pathname: need a pathname")
	}
	path := args[0]
	var fileStr string
	if path.typ == VStr {
		fileStr = path.str
	} else if path.typ == VPathname {
		fileStr = pathnameToString(path.pathname)
	} else {
		return nil, fmt.Errorf("compile-file-pathname: need a pathname or string")
	}
	outputPath := fileStr
	if strings.HasSuffix(outputPath, ".lisp") {
		outputPath = outputPath[:len(outputPath)-5] + ".fas"
	} else if strings.HasSuffix(outputPath, ".lsp") {
		outputPath = outputPath[:len(outputPath)-4] + ".fas"
	} else if !strings.HasSuffix(outputPath, ".fas") {
		outputPath += ".fas"
	}
	p := parsePathnameString(outputPath)
	v := gcv()
	v.typ = VPathname
	v.pathname = p
	return v, nil
}


func globalEnvGetFunction(name string) *Value {
	fn, _ := globalEnv.Get(name)
	if fn != nil && (fn.typ == VFunc || fn.typ == VPrim || fn.typ == VMacro) {
		return fn
	}
	return nil
}

// -------- FFI --------
var ffiRegistry = map[string]interface{}{
	"math/sin":   math.Sin,
	"math/cos":   math.Cos,
	"math/tan":   math.Tan,
	"math/sqrt":  math.Sqrt,
	"math/abs":   math.Abs,
	"math/floor": math.Floor,
	"math/ceil":  math.Ceil,
	"math/round": math.Round,
	"math/exp":   math.Exp,
	"math/log":   math.Log,
	"math/pow":   math.Pow,
	"os/getenv":  os.Getenv,
	"os/getpid":  os.Getpid,
}

func builtinFFI(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VStr {
		return nil, fmt.Errorf("ffi: need string function name")
	}
	name := args[0].str
	fn, ok := ffiRegistry[name]
	if !ok {
		return nil, fmt.Errorf("ffi: unknown function: %s", name)
	}
	fnVal := reflect.ValueOf(fn)
	// Handle non-function registered values
	if fnVal.Kind() != reflect.Func {
		return reflectToLisp(fnVal), nil
	}
	fnType := fnVal.Type()
	numIn := fnType.NumIn()
	// if variadic...
	callArgs := make([]reflect.Value, 0, len(args)-1)
	for i := 1; i < len(args); i++ {
		callArgs = append(callArgs, lispToReflect(args[i], fnType.In(min(i-1, numIn-1))))
	}
	// Handle variadic
	if fnType.IsVariadic() {
		fixedArgs := callArgs
		if len(fixedArgs) > numIn-1 {
			fixedArgs = callArgs[:numIn-1]
			varArgs := callArgs[numIn-1:]
			fixedArgs = append(fixedArgs, varArgs...)
			callArgs = fixedArgs
		}
	}
	results := fnVal.Call(callArgs)
	if len(results) == 0 {
		return vnil(), nil
	}
	return reflectToLisp(results[0]), nil
}

func builtinFFIRegister(args []*Value) (*Value, error) {
	if len(args) < 2 || args[0].typ != VStr {
		return nil, fmt.Errorf("ffi-register: need string name and value")
	}
	name := args[0].str
	// We can only register basic types here from Lisp
	// For Go functions, use the Go side
	if args[1].typ == VNum {
		ffiRegistry[name] = float64(toNum(args[1]))
	} else if args[1].typ == VStr {
		ffiRegistry[name] = args[1].str
	} else {
		return nil, fmt.Errorf("ffi-register: unsupported value type")
	}
	return vsym(name), nil
}

func builtinMacroexpand(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("macroexpand: need form")
	}
	form := args[0]
	depth := 0
	const maxMacroExpandDepth = 1000

	// Handle quasiquote specially since it's not a VMacro
	if form.typ == VPair && form.car != nil && form.car.typ == VSym && strings.EqualFold(form.car.str, "QUASIQUOTE") {
		if len(args) >= 1 && args[0].cdr != nil && args[0].cdr.typ == VPair && args[0].cdr.car != nil {
			expanded, e := evalQuasiquote(args[0].cdr.car, 0, globalEnv)
			if e != nil {
				return nil, fmt.Errorf("macroexpand: %s", e)
			}
			return list(vsym("quote"), expanded), nil
		}
		return form, nil
	}

	for form.typ == VPair && form.car != nil && form.car.typ == VSym {
		fn, err := globalEnv.Get(form.car.str)
		if err != nil || fn.typ != VMacro {
			break
		}
		depth++
		if depth > maxMacroExpandDepth {
			return nil, fmt.Errorf("macroexpand: expansion depth exceeded (%d)", maxMacroExpandDepth)
		}
		expanded, err := expandMacro(fn, form.cdr, globalEnv)
		if err != nil {
			return nil, fmt.Errorf("macroexpand: %s", err)
		}
		form = expanded
	}
	return form, nil
}

func builtinMacroexpand1(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("macroexpand-1: need form")
	}
	form := args[0]
	if form.typ == VPair && form.car != nil && form.car.typ == VSym {
		fn, err := globalEnv.Get(form.car.str)
		if err == nil && fn.typ == VMacro {
			expanded, err := expandMacro(fn, form.cdr, globalEnv)
			if err != nil {
				return nil, fmt.Errorf("macroexpand-1: %s", err)
			}
			return expanded, nil
		}
	}
	return form, nil
}

func builtinMakePackage(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("make-package: need package name")
	}
	name := primaryValue(args[0]).str
	pkg := makePackage(name)
	return vpkg(pkg), nil
}

func builtinInPackage(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("in-package: need package name")
	}
	pkg := resolvePackageFromDesignator(primaryValue(args[0]))
	if pkg == nil {
		return nil, fmt.Errorf("in-package: package not found")
	}
	currentPackage = pkg
	globalEnv.Set("*package*", vpkg(pkg))
	return vpkg(pkg), nil
}

func builtinFindPackage(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("find-package: need package name")
	}
	pkg := resolvePackageFromDesignator(primaryValue(args[0]))
	if pkg == nil {
		return vnil(), nil
	}
	return vpkg(pkg), nil
}

func builtinIntern(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("intern: need symbol name")
	}
	name := strings.ToUpper(primaryValue(args[0]).str)
	pkg := currentPackage
	if len(args) >= 2 && !isNil(args[1]) {
		pkg = resolvePackageFromDesignator(args[1])
		if pkg == nil {
			return nil, fmt.Errorf("intern: package not found")
		}
	}
	return internSymbol(name, pkg), nil
}

func builtinExport(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("export: need symbol or list of symbols")
	}
	pkg := currentPackage
	if len(args) >= 2 {
		pkg2 := resolvePackageFromDesignator(primaryValue(args[1]))
		if pkg2 != nil {
			pkg = pkg2
		}
	}
	// CL spec: first arg can be a symbol, string, or list of symbols/strings
	var syms []*Value
	first := primaryValue(args[0])
	if first.typ == VPair {
		syms = toSlice(first)
	} else {
		syms = []*Value{first}
	}
	var lastSym *Value
	for _, s := range syms {
		sym := primaryValue(s)
		var symName string
		if sym.typ == VSym {
			symName = sym.str
			// Strip keyword prefix
			if isKeyword(symName) {
				symName = keywordName(symName)
			}
		} else if sym.typ == VStr {
			// CL spec: strings are interned as symbols in the package
			symName = sym.str
			// Intern the string as a symbol in the package
			if existing, ok := pkg.symbols[symName]; ok {
				sym = existing
			} else {
				sym = internSymbol(symName, pkg)
				pkg.symbols[symName] = sym
			}
		} else {
			return nil, fmt.Errorf("export: need symbol or string, got %v", sym.typ)
		}
		pkg.exports[symName] = true
		// Also intern the symbol in the package
		if _, ok := pkg.symbols[symName]; !ok {
			pkg.symbols[symName] = sym
		}
		lastSym = sym
	}
	if len(syms) == 1 {
		return lastSym, nil
	}
	return vbool(true), nil
}

func builtinFindSymbol(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("find-symbol: need string name")
	}
	name := strings.ToUpper(primaryValue(args[0]).str)
	pkg := currentPackage
	if len(args) >= 2 && !isNil(args[1]) {
		pkg = resolvePackageFromDesignator(args[1])
		if pkg == nil {
			// Designator is not a valid package
			return multiVal(vnil(), vnil()), nil
		}
	}
	// Check internal symbols (including exported = external)
	if sym, ok := pkg.symbols[name]; ok {
		if pkg.exports[name] {
			return multiVal(sym, vsym("EXTERNAL")), nil
		}
		return multiVal(sym, vsym("INTERNAL")), nil
	}
	// Check inherited via use-list
	for _, used := range pkg.used {
		if sym, ok := used.symbols[name]; ok && used.exports[name] {
			return multiVal(sym, vsym("INHERITED")), nil
		}
	}
	return multiVal(vnil(), vnil()), nil
}

func builtinFindAllSymbols(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("find-all-symbols: need string name")
	}
	name := strings.ToUpper(primaryValue(args[0]).str)
	var result *Value
	for _, pkg := range packages {
		if sym, ok := pkg.symbols[name]; ok {
			result = cons(sym, result)
		}
	}
	if result == nil {
		return vnil(), nil
	}
	return result, nil
}

func builtinKeywordP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	if args[0].typ == VSym {
		kPkg := findPackage("KEYWORD")
		if kPkg != nil {
			if _, ok := kPkg.symbols[args[0].str]; ok {
				return vbool(true), nil
			}
		}
	}
	return vbool(false), nil
}

func builtinReadtableP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	return vbool(isReadtable(args[0])), nil
}

func builtinMakeReadtable(args []*Value) (*Value, error) {
	rt := &Readtable{
		macroFns: make(map[rune]*macroEntry),
		dispFns:  make(map[rune]*macroEntry),
		caseMode: ":UPCASE",
	}
	return vrt(rt), nil
}

func builtinCopyReadtable(args []*Value) (*Value, error) {
	src := currentReadtable
	if len(args) >= 1 && isReadtable(args[0]) {
		src = args[0].readtable
	}
	rt := &Readtable{
		macroFns: make(map[rune]*macroEntry),
		dispFns:  make(map[rune]*macroEntry),
		caseMode: src.caseMode,
	}
	for k, v := range src.macroFns {
		entry := *v
		rt.macroFns[k] = &entry
	}
	for k, v := range src.dispFns {
		entry := *v
		rt.dispFns[k] = &entry
	}
	return vrt(rt), nil
}

func builtinReadtableCase(args []*Value) (*Value, error) {
	if len(args) < 1 || !isReadtable(args[0]) {
		return nil, fmt.Errorf("readtable-case: expected a readtable")
	}
	return mkKeyword(args[0].readtable.caseMode), nil
}

func builtinSetReadtableCase(args []*Value) (*Value, error) {
	if len(args) < 2 || !isReadtable(args[0]) {
		return nil, fmt.Errorf("set-readtable-case: expected readtable and case mode")
	}
	mode := args[1]
	var modeStr string
	if mode.typ == VSym && isKeyword(mode.str) {
		modeStr = mode.str
	} else if mode.typ == VStr {
		modeStr = ":" + strings.ToUpper(mode.str)
	} else {
		return nil, fmt.Errorf("set-readtable-case: invalid case mode %v", mode)
	}
	switch modeStr {
	case ":UPCASE", ":DOWNCASE", ":PRESERVE", ":INVERT":
		args[0].readtable.caseMode = modeStr
	default:
		return nil, fmt.Errorf("set-readtable-case: invalid case mode %s (expected :UPCASE, :DOWNCASE, :PRESERVE, or :INVERT)", modeStr)
	}
	return mkKeyword(modeStr), nil
}

func builtinSetMacroCharacter(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("set-macro-character: need char and function")
	}
	var ch rune
	if args[0].typ == VChar {
		ch = args[0].ch
	} else {
		return nil, fmt.Errorf("set-macro-character: first argument must be a character")
	}
	fn := args[1]
	// CL spec: nil means remove the macro character
	if fn.typ == VNil {
		delete(currentReadtable.macroFns, ch)
		return vbool(true), nil
	}
	if fn.typ != VFunc && fn.typ != VPrim {
		return nil, fmt.Errorf("set-macro-character: second argument must be a function")
	}
	nonTerm := false
	if len(args) >= 3 {
		nonTerm = isTruthy(args[2])
	}
	rt := currentReadtable
	if len(args) >= 4 && isReadtable(args[3]) {
		rt = args[3].readtable
	}
	// Recognize close-paren sentinel: register as terminating macro character
	// (overrides the non-terminating flag if passed)
	isCloseParen := (fn == closeParenSentinel)
	rt.macroFns[ch] = &macroEntry{
		lispFn:      fn,
		terminating: isCloseParen || !nonTerm,
	}
	return vbool(true), nil
}

func builtinGetMacroCharacter(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("get-macro-character: need a character")
	}
	var ch rune
	if args[0].typ == VChar {
		ch = args[0].ch
	} else {
		return nil, fmt.Errorf("get-macro-character: argument must be a character")
	}
	rt := currentReadtable
	if len(args) >= 2 && isReadtable(args[1]) {
		rt = args[1].readtable
	}
	entry, ok := rt.macroFns[ch]
	if !ok || entry == nil {
		return vnil(), nil
	}
	if entry.goFn != nil {
		// Return nil for Go-level macro functions (not introspectable as Lisp fns)
		return vnil(), nil
	}
	if entry.lispFn != nil {
		return entry.lispFn, nil
	}
	// Standard macro character with no Lisp-level or Go-level function.
	// For close-paren ')', return the sentinel so set-macro-character can
	// recognize it and register the target character as close-paren equivalent.
	if ch == ')' {
		return closeParenSentinel, nil
	}
	// For other standard macro chars, return a wrapper that signals an error
	// when called as a reader macro (since the real handling is in the lexer).
	return &Value{typ: VPrim, fn: func(args []*Value) (*Value, error) {
		return nil, fmt.Errorf("standard macro character %q cannot be invoked as a reader macro function", string(ch))
	}}, nil
}

func builtinMakeDispatchMacroCharacter(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("make-dispatch-macro-character: need a character")
	}
	var ch rune
	if args[0].typ == VChar {
		ch = args[0].ch
	} else {
		return nil, fmt.Errorf("make-dispatch-macro-character: first argument must be a character")
	}
	nonTerm := false
	if len(args) >= 2 {
		nonTerm = isTruthy(args[1])
	}
	rt := currentReadtable
	if len(args) >= 3 && isReadtable(args[2]) {
		rt = args[2].readtable
	}
	// Register the dispatch character itself as a macro
	rt.macroFns[ch] = &macroEntry{
		goFn:        nil, // dispatch handled in parser
		terminating: !nonTerm,
	}
	// Initialize dispatch table if needed
	if rt.dispFns == nil {
		rt.dispFns = make(map[rune]*macroEntry)
	}
	return vbool(true), nil
}

func builtinSetDispatchMacroCharacter(args []*Value) (*Value, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("set-dispatch-macro-character: need disp-char, sub-char, and function")
	}
	var dispCh rune
	if args[0].typ == VChar {
		dispCh = args[0].ch
	} else {
		return nil, fmt.Errorf("set-dispatch-macro-character: first argument must be a character")
	}
	var subCh rune
	if args[1].typ == VChar {
		subCh = args[1].ch
	} else {
		return nil, fmt.Errorf("set-dispatch-macro-character: second argument must be a character")
	}
	fn := args[2]
	if fn.typ != VFunc && fn.typ != VPrim {
		return nil, fmt.Errorf("set-dispatch-macro-character: third argument must be a function")
	}
	rt := currentReadtable
	if len(args) >= 4 && isReadtable(args[3]) {
		rt = args[3].readtable
	}
	_ = dispCh // dispCh validated above (ensures it's a character)
	if rt.dispFns == nil {
		rt.dispFns = make(map[rune]*macroEntry)
	}
	rt.dispFns[subCh] = &macroEntry{
		lispFn:      fn,
		terminating: false,
	}
	return vbool(true), nil
}

func builtinGetDispatchMacroCharacter(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("get-dispatch-macro-character: need disp-char and sub-char")
	}
	var dispCh rune
	if args[0].typ == VChar {
		dispCh = args[0].ch
	} else {
		return nil, fmt.Errorf("get-dispatch-macro-character: first argument must be a character")
	}
	var subCh rune
	if args[1].typ == VChar {
		subCh = args[1].ch
	} else {
		return nil, fmt.Errorf("get-dispatch-macro-character: second argument must be a character")
	}
	rt := currentReadtable
	if len(args) >= 3 && isReadtable(args[2]) {
		rt = args[2].readtable
	}
	_ = dispCh // dispCh validated above
	if rt.dispFns == nil {
		return vnil(), nil
	}
	entry, ok := rt.dispFns[subCh]
	if !ok || entry == nil {
		return vnil(), nil
	}
	if entry.lispFn != nil {
		return entry.lispFn, nil
	}
	return vnil(), nil
}

func builtinSymbolPackage(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VSym {
		return vnil(), nil
	}
	name := args[0].str
	if isKeyword(name) {
		kPkg := findPackage("KEYWORD")
		if kPkg != nil {
			return vpkg(kPkg), nil
		}
		return vnil(), nil
	}
	// Find which package this symbol belongs to
	for _, pkg := range packages {
		if _, ok := pkg.symbols[name]; ok {
			return vpkg(pkg), nil
		}
	}
	return vnil(), nil
}

func builtinPackageName(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("package-name: need package designator")
	}
	pkg := resolvePackageFromDesignator(primaryValue(args[0]))
	if pkg == nil {
		return nil, fmt.Errorf("package-name: package not found")
	}
	return vstr(pkg.name), nil
}

func builtinListAllPackages(args []*Value) (*Value, error) {
	result := vnil()
	for _, pkg := range packages {
		result = cons(vpkg(pkg), result)
	}
	return result, nil
}

func builtinRenamePackage(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("rename-package: need package and new-name")
	}
	pkg := resolvePackageFromDesignator(primaryValue(args[0]))
	if pkg == nil {
		return nil, fmt.Errorf("rename-package: package not found")
	}
	newName := strings.ToUpper(primaryValue(args[1]).str)

	// Collect nicknames from options
	var nicknames []string
	for i := 2; i < len(args); i++ {
		n := primaryValue(args[i]).str
		if len(n) > 0 {
			nicknames = append(nicknames, n)
		}
	}

	// Remove old name from packages map
	delete(packages, pkg.name)
	for _, n := range pkg.nicknames {
		delete(packages, n)
	}

	// Update package
	pkg.name = newName
	pkg.nicknames = nicknames
	packages[newName] = pkg
	for _, n := range nicknames {
		packages[strings.ToUpper(n)] = pkg
	}

	return vpkg(pkg), nil
}

func builtinDeletePackage(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("delete-package: need package designator")
	}
	pkg := resolvePackageFromDesignator(primaryValue(args[0]))
	if pkg == nil {
		return vbool(false), nil
	}
	// Remove from packages map
	delete(packages, pkg.name)
	for _, n := range pkg.nicknames {
		delete(packages, n)
	}
	// Can't delete if it has external symbols, but for simplicity just delete
	return vbool(true), nil
}

func builtinPackageNicknames(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("package-nicknames: need package designator")
	}
	pkg := resolvePackageFromDesignator(primaryValue(args[0]))
	if pkg == nil {
		return nil, fmt.Errorf("package-nicknames: package not found")
	}
	result := vnil()
	for i := len(pkg.nicknames) - 1; i >= 0; i-- {
		result = cons(vsym(pkg.nicknames[i]), result)
	}
	return result, nil
}

func builtinPackageSymbols(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("package-symbols: need package designator")
	}
	pkg := resolvePackageFromDesignator(primaryValue(args[0]))
	if pkg == nil {
		return vnil(), nil
	}
	result := vnil()
	for _, sym := range pkg.symbols {
		result = cons(sym, result)
	}
	return result, nil
}

func builtinPackageExternalSymbols(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("package-external-symbols: need package designator")
	}
	pkg := resolvePackageFromDesignator(primaryValue(args[0]))
	if pkg == nil {
		return vnil(), nil
	}
	result := vnil()
	for name, isExported := range pkg.exports {
		if isExported {
			if sym, ok := pkg.symbols[name]; ok {
				result = cons(sym, result)
			}
		}
	}
	return result, nil
}

func builtinUnexport(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("unexport: need symbols")
	}
	pkg := currentPackage
	syms := args
	if len(args) >= 2 {
		pkg2 := resolvePackageFromDesignator(primaryValue(args[0]))
		if pkg2 != nil {
			pkg = pkg2
			syms = args[1:]
		}
	}
	for _, s := range syms {
		symName := primaryValue(s).str
		delete(pkg.exports, symName)
	}
	return vbool(true), nil
}

func builtinPackageUseList(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("package-use-list: need package designator")
	}
	pkg := resolvePackageFromDesignator(primaryValue(args[0]))
	if pkg == nil {
		return vnil(), nil
	}
	result := vnil()
	for _, used := range pkg.used {
		result = cons(vpkg(used), result)
	}
	return result, nil
}

func builtinPackageUsedByList(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("package-used-by-list: need package designator")
	}
	pkg := resolvePackageFromDesignator(primaryValue(args[0]))
	if pkg == nil {
		return vnil(), nil
	}
	var usedBy []*Package
	for _, p := range packages {
		for _, u := range p.used {
			if u == pkg {
				usedBy = append(usedBy, p)
				break
			}
		}
	}
	result := vnil()
	for _, p := range usedBy {
		result = cons(vpkg(p), result)
	}
	return result, nil
}

func builtinPackageShadowingImportList(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("package-shadowing-import-list: need package designator")
	}
	pkg := resolvePackageFromDesignator(primaryValue(args[0]))
	if pkg == nil {
		return vnil(), nil
	}
	result := vnil()
	for _, symName := range pkg.shadowingImps {
		result = cons(vsym(symName), result)
	}
	return result, nil
}

func builtinProvide(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VSym {
		return nil, fmt.Errorf("provide: need a module name (symbol)")
	}
	name := args[0].str
	lispModules[name] = true

	// Update *modules* list in global env
	var list *Value
	for m := range lispModules {
		list = cons(vsym(m), list)
	}
	globalEnv.Set("*modules*", list)
	return vsym(name), nil
}

func builtinRequire(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VSym {
		return nil, fmt.Errorf("require: need a module name (symbol)")
	}
	name := args[0].str
	if lispModules[name] {
		return vsym(name), nil // already loaded
	}

	// Try to load <name>.lisp
	filename := name + ".lisp"
	_, err := loadFile(filename, globalEnv)
	if err != nil {
		return nil, fmt.Errorf("require: cannot load %s: %v", filename, err)
	}

	// Mark as loaded (loadFile may have called provide, but be safe)
	if !lispModules[name] {
		lispModules[name] = true
		var list *Value
		for m := range lispModules {
			list = cons(vsym(m), list)
		}
		globalEnv.Set("*modules*", list)
	}
	return vsym(name), nil
}

func builtinImport(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VSym {
		return nil, fmt.Errorf("import: need symbol")
	}
	pkg := currentPackage
	if len(args) >= 2 {
		pkg2 := resolvePackageFromDesignator(primaryValue(args[1]))
		if pkg2 != nil {
			pkg = pkg2
		}
	}
	symName := primaryValue(args[0]).str
	pkg.symbols[symName] = args[0]
	return args[0], nil
}

func builtinUsePackage(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("use-package: need package name(s)")
	}
	pkgs := args[0]
	// Accept a list of package names or a single symbol
	if pkgs.typ == VPair {
		for !isNil(pkgs) {
			if err := useOnePackage(pkgs.car); err != nil {
				return nil, err
			}
			pkgs = pkgs.cdr
		}
		return vbool(true), nil
	}
	if err := useOnePackage(pkgs); err != nil {
		return nil, err
	}
	return vbool(true), nil
}

func useOnePackage(v *Value) error {
	srcPkg := resolvePackageFromDesignator(primaryValue(v))
	if srcPkg == nil {
		return fmt.Errorf("use-package: package not found")
	}
	pkg := currentPackage
	for symName, exported := range srcPkg.exports {
		if exported {
			if _, ok := pkg.symbols[symName]; !ok {
				if sym, ok := srcPkg.symbols[symName]; ok {
					pkg.symbols[symName] = sym
				}
			}
		}
	}
	pkg.used = append(pkg.used, srcPkg)
	return nil
}

func builtinUnusePackage(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("unuse-package: need package name(s)")
	}
	srcPkg := resolvePackageFromDesignator(primaryValue(args[0]))
	if srcPkg == nil {
		return nil, fmt.Errorf("unuse-package: package not found")
	}
	pkg := currentPackage
	if len(args) >= 2 {
		pkg2 := resolvePackageFromDesignator(primaryValue(args[1]))
		if pkg2 != nil {
			pkg = pkg2
		}
	}
	// Remove srcPkg from used list
	newUsed := make([]*Package, 0, len(pkg.used))
	for _, p := range pkg.used {
		if p != srcPkg {
			newUsed = append(newUsed, p)
		}
	}
	pkg.used = newUsed
	return vbool(true), nil
}

func builtinShadow(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VSym {
		return nil, fmt.Errorf("shadow: need symbol")
	}
	pkg := currentPackage
	if len(args) >= 2 {
		pkg2 := resolvePackageFromDesignator(primaryValue(args[1]))
		if pkg2 != nil {
			pkg = pkg2
		}
	}
	symName := primaryValue(args[0]).str
	pkg.symbols[symName] = args[0]
	pkg.exports[symName] = true
	return args[0], nil
}

func builtinUnintern(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VSym {
		return nil, fmt.Errorf("unintern: need symbol")
	}
	pkg := currentPackage
	if len(args) >= 2 && args[1].typ == VSym {
		pkg = findPackage(args[1].str)
		if pkg == nil {
			return nil, fmt.Errorf("unintern: package not found")
		}
	}
	symName := args[0].str
	delete(pkg.symbols, symName)
	delete(pkg.exports, symName)
	return args[0], nil
}

func builtinShadowingImport(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("shadowing-import: need symbol or list")
	}
	pkg := currentPackage
	if len(args) >= 2 && args[len(args)-1].typ == VSym {
		lastArg := args[len(args)-1]
		if p := findPackage(lastArg.str); p != nil {
			pkg = p
			args = args[:len(args)-1]
		}
	}
	symbols := args
	if len(args) == 1 && args[0].typ == VPair {
		seen := make(map[*Value]bool)
		for cur := args[0]; !isNil(cur) && cur.typ == VPair; cur = cur.cdr {
			if seen[cur] { break }
			seen[cur] = true
			symbols = append(symbols, cur.car)
		}
		symbols = symbols[1:]
	}
	for _, sym := range symbols {
		if sym.typ != VSym {
			return nil, fmt.Errorf("shadowing-import: need symbol, got %s", typeStr(sym))
		}
		symName := sym.str
		pkg.symbols[symName] = sym
		pkg.exports[symName] = true
		// Track shadowing imports
		found := false
		for _, s := range pkg.shadowingImps {
			if s == symName {
				found = true
				break
			}
		}
		if !found {
			pkg.shadowingImps = append(pkg.shadowingImps, symName)
		}
	}
	return vsym("t"), nil
}

// -------- CLOS Builtins --------

func builtinMakeInstance(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VSym {
		return nil, fmt.Errorf("make-instance: need class name")
	}
	className := args[0].str
	cv := findClass(className)
	if cv == nil || cv.typ != VClass {
		return nil, fmt.Errorf("make-instance: %s is not a class", className)
	}
	inst := gcv()
	inst.typ = VInstance
	inst.instClass = cv
	inst.instSlots = make(map[string]*Value)

	// Collect initarg mappings and initforms from slot definitions
	// Walk the entire CPL so inherited slots are included
	slotInitforms := make(map[string]*Value)
	slotInitargs := make(map[string]string) // initarg keyword -> slotName
	// Process each class in CPL (most-specific first)
	for _, c := range cv.cpl {
		if c.typ != VClass {
			continue
		}
		for slotDef := c.body; !isNil(slotDef); slotDef = slotDef.cdr {
			slotName := ""
			if slotDef.car != nil && slotDef.car.typ == VSym {
				slotName = slotDef.car.str
				if _, ok := inst.instSlots[slotName]; !ok {
					inst.instSlots[slotName] = nil // unbound sentinel
				}
			} else if slotDef.car != nil && slotDef.car.typ == VPair && slotDef.car.car != nil && slotDef.car.car.typ == VSym {
				slotName = slotDef.car.car.str
				if _, ok := inst.instSlots[slotName]; !ok {
					inst.instSlots[slotName] = nil // unbound sentinel
				}
				// Parse slot options: :initform, :initarg
				opts := slotDef.car.cdr
				for !isNil(opts) && !isNil(opts.cdr) {
					if opts.car != nil && opts.car.typ == VSym {
						switch opts.car.str {
						case ":INITFORM":
								if _, exists := slotInitforms[slotName]; !exists {
									if opts.cdr == nil || opts.cdr.typ != VPair {
										opts = opts.cdr.cdr
										continue
									}
									if opts.cdr.car != nil {
										initVal, e := Eval(opts.cdr.car, globalEnv)
										if e == nil {
											slotInitforms[slotName] = initVal
										}
									}
									opts = opts.cdr.cdr
									continue
								}
						case ":INITARG":
							if opts.cdr != nil && opts.cdr.typ == VPair && opts.cdr.car != nil && opts.cdr.car.typ == VSym {
								if _, exists := slotInitargs[opts.cdr.car.str]; !exists {
									slotInitargs[opts.cdr.car.str] = slotName
								}
							}
							opts = opts.cdr.cdr
							continue
						}
					}
					opts = opts.cdr
				}
			}
		}
		// Also add bare slot names from classSlots that weren't in body
		for _, slot := range c.classSlots {
			if _, ok := inst.instSlots[slot]; !ok {
				inst.instSlots[slot] = nil // unbound sentinel
			}
		}
	}
	// Process keyword arguments (:initarg -> slot, or direct slot name for condition classes)
	for i := 1; i < len(args)-1; i += 2 {
		if args[i].typ == VSym {
			key := args[i].str
			if i+1 >= len(args) {
				continue
			}
			if slotName, ok := slotInitargs[key]; ok {
				inst.instSlots[slotName] = args[i+1]
			} else if len(key) > 1 && key[0] == ':' {
				// Direct slot name mapping (e.g., :datum -> datum)
				slotName := key[1:]
				if _, exists := inst.instSlots[slotName]; exists || args[0].typ == VSym {
					inst.instSlots[slotName] = args[i+1]
				}
			}
		}
	}
	// Apply initforms for slots not yet set (nil = unbound sentinel)
	for slotName, initVal := range slotInitforms {
		if inst.instSlots[slotName] == nil {
			inst.instSlots[slotName] = initVal
		}
	}
	return inst, nil
}

func builtinSlotValue(args []*Value) (*Value, error) {
	if len(args) < 2 || args[0].typ != VInstance || args[1].typ != VSym {
		return nil, fmt.Errorf("slot-value: need instance and slot name")
	}
	inst := args[0]
	slotName := args[1].str
	v, ok := inst.instSlots[slotName]
	if !ok {
		// Check parent class slots
		for _, c := range inst.instClass.cpl {
			if c.typ == VClass {
				for _, s := range c.classSlots {
					if s == slotName {
						v, ok = inst.instSlots[s]
						break
					}
				}
			}
		}
	}
	if !ok {
		return nil, fmt.Errorf("slot-value: slot %s not found in %s", slotName, inst.instClass.str)
	}
	if v == nil {
		return vnil(), nil // unbound slot returns nil for backward compat
	}
	return v, nil
}

func builtinSlotValueSetf(args []*Value) (*Value, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("slot-value-setf: need value, instance, and slot name")
	}
	val := args[0]
	inst := args[1]
	slot := args[2]
	if inst.typ != VInstance {
		return nil, fmt.Errorf("slot-value-setf: second argument must be an instance")
	}
	if slot.typ != VSym {
		return nil, fmt.Errorf("slot-value-setf: third argument must be a symbol")
	}
	inst.instSlots[slot.str] = val
	return val, nil
}

func builtinSlotSet(args []*Value) (*Value, error) {
	if len(args) < 3 || args[0].typ != VInstance || args[1].typ != VSym {
		return nil, fmt.Errorf("slot-set!: need instance, slot name, and value")
	}
	inst := args[0]
	slotName := args[1].str
	inst.instSlots[slotName] = args[2]
	return args[2], nil
}

func builtinSlotBoundp(args []*Value) (*Value, error) {
	if len(args) < 2 || args[0].typ != VInstance || args[1].typ != VSym {
		return nil, fmt.Errorf("slot-boundp: need instance and slot name")
	}
	inst := args[0]
	slotName := args[1].str
	v, ok := inst.instSlots[slotName]
	if ok && v != nil {
		return vbool(true), nil // slot exists in map and is not unbound sentinel
	}
	// Check CPL for inherited slot declarations
	for _, c := range inst.instClass.cpl {
		if c.typ == VClass {
			for _, s := range c.classSlots {
				if s == slotName {
					v, ok = inst.instSlots[s]
					return vbool(ok && v != nil), nil
				}
			}
		}
	}
	return vbool(false), nil
}

func builtinSlotExistsP(args []*Value) (*Value, error) {
	if len(args) < 2 || args[0].typ != VInstance || args[1].typ != VSym {
		return nil, fmt.Errorf("slot-exists-p: need instance and slot name")
	}
	inst := args[0]
	slotName := args[1].str
	for _, c := range inst.instClass.cpl {
		if c.typ == VClass {
			for _, s := range c.classSlots {
				if s == slotName {
					return vbool(true), nil
				}
			}
		}
	}
	return vbool(false), nil
}

func builtinSlotMakunbound(args []*Value) (*Value, error) {
	if len(args) < 2 || args[0].typ != VInstance || args[1].typ != VSym {
		return nil, fmt.Errorf("slot-makunbound: need instance and slot name")
	}
	inst := args[0]
	slotName := args[1].str
	inst.instSlots[slotName] = nil // unbound sentinel
	return args[0], nil
}

func builtinClassName(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("class-name: need a class")
	}
	c := args[0]
	if c.typ == VClass {
		return vsym(c.str), nil
	}
	if c.typ == VSym {
		if cls, ok := classRegistry[c.str]; ok {
			return vsym(cls.str), nil
		}
		return nil, fmt.Errorf("class-name: %s is not a class", c.str)
	}
	return nil, fmt.Errorf("class-name: need a class or symbol")
}

// builtinClassNameSetf - (setf (class-name class) new-name)
func builtinClassNameSetf(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("(setf class-name): need class and new-name")
	}
	// setf convention: args[0] = new-value, args[1] = place-argument
	newName := args[0]
	cls := args[1]
	if cls.typ != VClass {
		return nil, fmt.Errorf("(setf class-name): second argument must be a class")
	}
	if newName.typ != VSym {
		return nil, fmt.Errorf("(setf class-name): new name must be a symbol")
	}
	// Remove from old name, add under new name
	delete(classRegistry, cls.str)
	cls.str = newName.str
	classRegistry[cls.str] = cls
	return cls, nil
}

func builtinFindClass(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("find-class: need a symbol")
	}
	sym := args[0]
	if sym.typ != VSym {
		return nil, fmt.Errorf("find-class: need a symbol")
	}
	if cls, ok := classRegistry[sym.str]; ok {
		return cls, nil
	}
	return vnil(), nil
}

// builtinFindClassSetf - (setf (find-class symbol) class)
func builtinFindClassSetf(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("(setf find-class): need class and symbol")
	}
	newClass := args[0]
	sym := args[1]
	if sym.typ != VSym {
		return nil, fmt.Errorf("(setf find-class): second argument must be a symbol")
	}
	if newClass.typ != VClass {
		return nil, fmt.Errorf("(setf find-class): first argument must be a class")
	}
	classRegistry[sym.str] = newClass
	return newClass, nil
}

func builtinClassOf(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("class-of: need argument")
	}
	v := args[0]
	// For instances, return the class object directly
	if v.typ == VInstance && v.instClass != nil {
		return v.instClass, nil
	}
	// For built-in types, look up the class in classRegistry by type name
	typeName := strings.ToUpper(typeStr(v))
	if cls, ok := classRegistry[typeName]; ok {
		return cls, nil
	}
	// If no class registered for this type, return the built-in-class symbol as fallback
	return vsym(typeName), nil
}

func builtinIsA(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("is-a?: need obj and class-name")
	}
	obj := args[0]
	if obj.typ != VInstance {
		return vnil(), nil
	}
	var cls *Value
	if args[1].typ == VSym {
		var err error
		cls, err = globalEnv.Get(args[1].str)
		if err != nil || cls.typ != VClass {
			return vnil(), nil
		}
	} else if args[1].typ == VClass {
		cls = args[1]
	} else {
		return vnil(), nil
	}
	for _, c := range obj.instClass.cpl {
		if c == cls {
			return vbool(true), nil
		}
	}
	return vnil(), nil
}

func builtinClassSlots(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("class-slots: need class name")
	}
	var cls *Value = args[0]
	// Support both VClass and VSym (symbol naming a class)
	if cls.typ == VSym {
		cls, _ = globalEnv.Get(cls.str)
	}
	if cls == nil || cls.typ != VClass {
		return nil, fmt.Errorf("class-slots: not a class")
	}
	// Build a list of slot name symbols
	var result *Value = vnil()
	for i := len(cls.classSlots) - 1; i >= 0; i-- {
		result = &Value{typ: VPair, car: vsym(cls.classSlots[i]), cdr: result}
	}
	return result, nil
}

func builtinClassSlotDefs(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("class-slot-defs: need class name")
	}
	var cls *Value = args[0]
	if cls.typ == VSym {
		cls, _ = globalEnv.Get(cls.str)
	}
	if cls == nil || cls.typ != VClass {
		return nil, fmt.Errorf("class-slot-defs: not a class")
	}
	if cls.body != nil {
		return cls.body, nil
	}
	return vnil(), nil
}

// valueToEQLKey produces a stable string key for an EQL specializer value
func valueToEQLKey(v *Value) string {
	v = primaryValue(v)
	switch v.typ {
	case VSym:
		return "S:" + v.str
	case VNum:
		if v.imag != 0 {
			return fmt.Sprintf("C:%v+%vi", v.num, v.imag)
		}
		return fmt.Sprintf("N:%v", v.num)
	case VStr:
		return "STR:" + v.str
	case VChar:
		return fmt.Sprintf("CH:%d", v.ch)
	case VNil:
		return "NIL"
	case VBool:
		if v.num != 0 {
			return "T"
		}
		return "NIL"
	default:
		return fmt.Sprintf("V:%s", ToString(v))
	}
}

// valueMatchesEQLKey checks if a runtime value matches an EQL specializer key
func valueMatchesEQLKey(arg *Value, key string) bool {
	arg = primaryValue(arg)
	switch {
	case key == "NIL":
		return isNil(arg)
	case key == "T":
		return arg.typ == VBool && arg.num != 0
	case strings.HasPrefix(key, "S:"):
		name := key[2:]
		if arg.typ == VSym && arg.str == name {
			return true
		}
		// Special case: (eql nil) / (eql t) — the parsed form is a symbol,
		// but the evaluated argument may be VNil/VBool
		if name == "nil" && isNil(arg) {
			return true
		}
		if name == "t" && arg.typ == VBool && arg.num != 0 {
			return true
		}
		return false
	case strings.HasPrefix(key, "N:"):
		if arg.typ != VNum {
			return false
		}
		n, _ := strconv.ParseFloat(key[2:], 64)
		return arg.num == n
	case strings.HasPrefix(key, "STR:"):
		return arg.typ == VStr && arg.str == key[4:]
	case strings.HasPrefix(key, "CH:"):
		if arg.typ != VChar {
			return false
		}
		n, _ := strconv.ParseInt(key[3:], 10, 32)
		return arg.ch == rune(n)
	case strings.HasPrefix(key, "C:"):
		if arg.typ != VNum {
			return false
		}
		parts := strings.SplitN(key[2:], "+", 2)
		if len(parts) == 2 {
			re, _ := strconv.ParseFloat(parts[0], 64)
			imStr := strings.TrimSuffix(parts[1], "i")
			im, _ := strconv.ParseFloat(imStr, 64)
			return arg.num == re && arg.imag == im
		}
		return false
	}
	return false
}

// isTypeSpecializerMatch checks if arg matches a built-in type specializer
func isTypeSpecializerMatch(arg *Value, typeName string) bool {
	arg = primaryValue(arg)
	switch typeName {
	case "t", "T":
		return true
	case "null", "NULL":
		return isNil(arg)
	case "list", "LIST":
		return isNil(arg) || arg.typ == VPair
	case "cons", "CONS":
		return arg.typ == VPair
	case "symbol", "SYMBOL":
		return arg.typ == VSym || isNil(arg)
	case "string", "STRING":
		return arg.typ == VStr
	case "number", "NUMBER":
		return arg.typ == VNum
	case "integer", "INTEGER":
		return arg.typ == VNum && !arg.isFloat && arg.num == float64(int64(arg.num))
	case "float", "single-float", "double-float", "FLOAT", "SINGLE-FLOAT", "DOUBLE-FLOAT":
		return arg.typ == VNum && arg.isFloat
	case "character", "CHARACTER":
		return arg.typ == VChar
	case "vector", "VECTOR":
		return arg.typ == VArray && arg.array != nil && len(arg.array.dims) == 1
	case "array", "ARRAY":
		return arg.typ == VArray
	case "function", "FUNCTION":
		return arg.typ == VFunc || arg.typ == VPrim || arg.typ == VGeneric
	case "hash-table", "HASH-TABLE":
		return arg.typ == VVHash
	case "stream", "STREAM":
		return arg.typ == VStream
	}
	return false
}

// methodApplicable checks if a method is applicable to the given evaluated arguments
func methodApplicable(m genMethod, evArgs []*Value) bool {
	if len(m.specializers) == 0 {
		return true // unspecialized method applies to all
	}
	for i, sp := range m.specializers {
		if sp == "" {
			continue // no specializer for this param
		}
		if i >= len(evArgs) {
			return false
		}
		arg := evArgs[i]
		// Handle EQL specializer: "#EQL:<key>"
		if strings.HasPrefix(sp, "#EQL:") {
			if valueMatchesEQLKey(arg, sp[5:]) {
				continue
			}
			return false
		}
		// Handle built-in type specializers (t, null, list, cons, etc.)
		if isTypeSpecializerMatch(arg, sp) {
			continue
		}
		// Handle class specializers for instances
		if arg.typ == VInstance {
			found := false
			for _, c := range arg.instClass.cpl {
				if c.typ == VClass && c.str == sp {
					found = true
					break
				}
			}
			if found {
				continue
			}
		}
		return false
	}
	return true
}
// methodSpecificity returns a score for method specificity (lower = more specific)
// Only meaningful for methods with specializers on the first argument
func methodSpecificity(m genMethod, evArgs []*Value) int {
	baseScore := 999
	if len(m.specializers) > 0 && m.specializers[0] != "" && len(evArgs) > 0 {
		sp := m.specializers[0]
		// EQL specializers are most specific (score 0)
		if strings.HasPrefix(sp, "#EQL:") {
			baseScore = 0
		} else if evArgs[0].typ == VInstance {
			for i, c := range evArgs[0].instClass.cpl {
				if c.typ == VClass && c.str == sp {
					baseScore = i
					break
				}
			}
		} else if isTypeSpecializerMatch(evArgs[0], sp) {
			spUp := strings.ToUpper(sp)
			// Score hierarchy: lower = more specific
			switch spUp {
			case "NULL":
				baseScore = 1
			case "CONS":
				baseScore = 2
			case "LIST":
				baseScore = 3
			case "SYMBOL":
				baseScore = 10
			case "STRING":
				baseScore = 11
			case "SIMPLE-STRING":
				baseScore = 12
			case "CHARACTER":
				baseScore = 13
			case "BASE-CHAR":
				baseScore = 14
			case "STANDARD-CHAR":
				baseScore = 15
			case "KEYWORD":
				baseScore = 16
			case "NUMBER":
				baseScore = 200
			case "REAL":
				baseScore = 150
			case "RATIONAL":
				baseScore = 140
			case "RATIO":
				baseScore = 135
			case "INTEGER":
				baseScore = 130
			case "FIXNUM":
				baseScore = 120
			case "BIGNUM":
				baseScore = 115
			case "BIT":
				baseScore = 110
			case "FLOAT":
				baseScore = 145
			case "SINGLE-FLOAT":
				baseScore = 125
			case "DOUBLE-FLOAT":
				baseScore = 118
			case "SHORT-FLOAT":
				baseScore = 122
			case "LONG-FLOAT":
				baseScore = 112
			case "COMPLEX":
				baseScore = 160
			case "SEQUENCE":
				baseScore = 180
			case "VECTOR":
				baseScore = 165
			case "ARRAY":
				baseScore = 175
			case "SIMPLE-VECTOR":
				baseScore = 155
			case "HASH-TABLE":
				baseScore = 300
			case "PACKAGE":
				baseScore = 301
			case "PATHNAME":
				baseScore = 302
			case "RANDOM-STATE":
				baseScore = 303
			case "READTABLE":
				baseScore = 304
			case "STREAM":
				baseScore = 305
			case "CLASS":
				baseScore = 306
			case "FUNCTION":
				baseScore = 400
			case "COMPILED-FUNCTION":
				baseScore = 390
			case "METHOD":
				baseScore = 395
			case "T":
				baseScore = 999
			default:
				// For user-defined types, check subtype relationship with known types.
				// Start with default score, then refine based on subtype relationships.
				score := 500
				// If sp is a subtype of a known type, give it a more specific score
				for other, otherScore := range map[string]int{
					"NUMBER": 200, "REAL": 150, "RATIONAL": 140, "INTEGER": 130,
					"FIXNUM": 120, "BIGNUM": 115, "BIT": 110, "RATIO": 135,
					"FLOAT": 145, "SINGLE-FLOAT": 125, "DOUBLE-FLOAT": 118,
					"SHORT-FLOAT": 122, "LONG-FLOAT": 112, "COMPLEX": 160,
					"SEQUENCE": 180, "LIST": 3, "VECTOR": 165, "ARRAY": 175,
					"SIMPLE-VECTOR": 155, "STRING": 11, "CHARACTER": 13,
					"SYMBOL": 10, "HASH-TABLE": 300, "STREAM": 305,
					"PACKAGE": 301, "PATHNAME": 302, "RANDOM-STATE": 303,
					"READTABLE": 304, "FUNCTION": 400, "CONS": 2, "NULL": 1,
				} {
					if other == spUp {
						continue
					}
					if typeIsSubtype(spUp, other) {
						if otherScore > 0 && otherScore <= score {
							score = otherScore - 1
						}
					}
				}
				baseScore = score
			}
		}
	}
	// Parse numeric auxiliary qualifier (e.g., ":around 2" -> 2)
	// Higher number = more specific = lower score
	if m.qualifier != "" {
		parts := strings.Split(m.qualifier, " ")
		if len(parts) == 2 {
			if n, err := strconv.Atoi(parts[1]); err == nil {
				baseScore -= n * 1000
			}
		}
	}
	return baseScore
}

// subtypepCheck checks if typeA is a subtype of typeB using the built-in subtypep logic
func subtypepCheck(typeA, typeB string, env *Env) bool {
	return typeIsSubtype(strings.ToUpper(typeA), strings.ToUpper(typeB))
}

// typeIsSubtype checks if t1 is a subtype of t2 using the same hierarchy as subtypepChecks
func typeIsSubtype(t1, t2 string) bool {
	if t1 == t2 {
		return true
	}
	types := map[string][]string{"INTEGER": {"RATIONAL", "REAL", "NUMBER", "ATOM", "T"}, "FLOAT": {"REAL", "NUMBER", "ATOM", "T"}, "RATIONAL": {"REAL", "NUMBER", "ATOM", "T"}, "COMPLEX": {"NUMBER", "ATOM", "T"}, "REAL": {"NUMBER", "ATOM", "T"}, "RATIO": {"RATIONAL", "REAL", "NUMBER", "ATOM", "T"}, "FIXNUM": {"INTEGER", "RATIONAL", "REAL", "NUMBER", "ATOM", "T"}, "BIGNUM": {"INTEGER", "RATIONAL", "REAL", "NUMBER", "ATOM", "T"}, "BIT": {"INTEGER", "RATIONAL", "REAL", "NUMBER", "ATOM", "T"}, "SHORT-FLOAT": {"FLOAT", "REAL", "NUMBER", "ATOM", "T"}, "SINGLE-FLOAT": {"FLOAT", "REAL", "NUMBER", "ATOM", "T"}, "DOUBLE-FLOAT": {"FLOAT", "REAL", "NUMBER", "ATOM", "T"}, "LONG-FLOAT": {"FLOAT", "REAL", "NUMBER", "ATOM", "T"}, "STRING": {"ARRAY", "VECTOR", "SEQUENCE", "ATOM", "T"}, "SIMPLE-STRING": {"STRING", "ARRAY", "VECTOR", "SEQUENCE", "ATOM", "T"}, "CHARACTER": {"ATOM", "T"}, "BASE-CHAR": {"CHARACTER", "ATOM", "T"}, "STANDARD-CHAR": {"BASE-CHAR", "CHARACTER", "ATOM", "T"}, "EXTENDED-CHAR": {"CHARACTER", "ATOM", "T"}, "SYMBOL": {"ATOM", "T"}, "KEYWORD": {"SYMBOL", "ATOM", "T"}, "NULL": {"SYMBOL", "LIST", "SEQUENCE", "ATOM", "T"}, "CONS": {"LIST", "SEQUENCE", "T"}, "PAIR": {"CONS", "LIST", "SEQUENCE", "T"}, "LIST": {"SEQUENCE", "T"}, "SEQUENCE": {"T"}, "VECTOR": {"ARRAY", "SEQUENCE", "T"}, "SIMPLE-VECTOR": {"VECTOR", "ARRAY", "SEQUENCE", "T"}, "ARRAY": {"T"}, "FUNCTION": {"T"}, "COMPILED-FUNCTION": {"FUNCTION", "T"}, "HASH-TABLE": {"T"}, "STREAM": {"T"}, "PACKAGE": {"T"}, "PATHNAME": {"T"}, "RANDOM-STATE": {"T"}, "READTABLE": {"T"}, "INSTANCE": {"T"}, "STRUCTURE": {"INSTANCE", "T"}, "METHOD": {"T"}, "BOOLEAN": {"ATOM", "T"}}
	if subtypes, ok := types[t1]; ok {
		for _, s := range subtypes {
			if s == t2 {
				return true
			}
		}
	}
	return false
}
// callNextMethodChain applies the next method(s) in a chain with the original arguments
func callNextMethodChain(methods []genMethod, evArgs []*Value) (*Value, error) {
	if len(methods) == 0 {
		return nil, fmt.Errorf("call-next-method: no next method")
	}
	pm := methods[0]
	pEnv := NewEnv(pm.env)
	for j, p := range pm.params {
		if j < len(evArgs) {
			pEnv.Set(p, evArgs[j])
		}
	}
	// Bind call-next-method for further chaining
	if len(methods) > 1 {
		remaining := methods[1:]
		pEnv.Set("call-next-method", &Value{
			typ: VPrim,
			fn: func(ignored []*Value) (*Value, error) {
				return callNextMethodChain(remaining, evArgs)
			},
		})
		pEnv.Set("next-method-p", &Value{
			typ: VPrim,
			fn: func(ignored []*Value) (*Value, error) {
				return vbool(true), nil
			},
		})
	} else {
		pEnv.Set("call-next-method", &Value{
			typ: VPrim,
			fn: func(args2 []*Value) (*Value, error) {
				return nil, fmt.Errorf("call-next-method: no next method")
			},
		})
		pEnv.Set("next-method-p", &Value{
			typ: VPrim,
			fn: func(ignored []*Value) (*Value, error) {
				return vbool(false), nil
			},
		})
	}
	// Evaluate body (list of expressions)
	var result *Value = vnil()
	if pm.body != nil {
		bodyList := pm.body
		for !isNil(bodyList) {
			r, e := Eval(bodyList.car, pEnv)
			if e != nil {
				return nil, e
			}
			result = r
			bodyList = bodyList.cdr
		}
	}
	return result, nil
}

func builtinInstanceP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	return vbool(args[0].typ == VInstance), nil
}

// builtinFindMethod implements CL: FIND-METHOD
// (find-method generic-function qualifier specializer-list &optional errorp)
func builtinFindMethod(args []*Value) (*Value, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("find-method: need generic-function, qualifier, and specializer-list")
	}
	gf := primaryValue(args[0])
	if gf.typ != VGeneric {
		return nil, fmt.Errorf("find-method: first argument must be a generic function")
	}
	qualifier := ""
	qv := primaryValue(args[1])
	if qv.typ == VSym && isKeyword(qv.str) {
		qualifier = qv.str
	} else if qv.typ == VNil || isNil(qv) {
		qualifier = ""
	} else if qv.typ == VStr || qv.typ == VSym {
		qualifier = strings.ToUpper(qv.str)
	} else {
		return nil, fmt.Errorf("find-method: qualifier must be a keyword, string, symbol, or nil")
	}
	specList := primaryValue(args[2])
	errorp := true
	if len(args) >= 4 {
		ep := primaryValue(args[3])
		if isNil(ep) || !isTruthy(ep) {
			errorp = false
		}
	}
	// Convert specializer-list to string slice
	var specStrings []string
	for !isNil(specList) && specList.typ == VPair {
		sp := specList.car
		if sp == nil || isNil(sp) {
			specStrings = append(specStrings, "")
		} else if sp.typ == VSym {
			if sp.str == "T" {
				specStrings = append(specStrings, "")
			} else {
				specStrings = append(specStrings, strings.ToUpper(sp.str))
			}
		} else if sp.typ == VPair && sp.car != nil && sp.car.typ == VSym && sp.car.str == "EQL" {
			// (eql value) specializer
			eqlVal := sp.cdr
			if eqlVal != nil && eqlVal.typ == VPair && eqlVal.car != nil {
				key := eqlVal.car
				if key.typ == VSym && len(key.str) > 0 && key.str[0] == ':' {
					specStrings = append(specStrings, "#EQL:"+key.str)
				} else {
					specStrings = append(specStrings, "#EQL:"+ToString(key))
				}
			} else {
				specStrings = append(specStrings, "#EQL:")
			}
		} else if sp.typ == VClass {
			classStr := strings.ToUpper(sp.str)
			if classStr == "T" {
				specStrings = append(specStrings, "") // t = unspecialized
			} else {
				specStrings = append(specStrings, classStr)
			}
		} else {
			specStrings = append(specStrings, "")
		}
		specList = specList.cdr
	}
	// Search for matching method
	for i, m := range gf.genMethods {
		if m.qualifier != qualifier {
			continue
		}
		if len(m.specializers) != len(specStrings) {
			continue
		}
		match := true
		for j, ms := range m.specializers {
			req := specStrings[j]
			if ms == req {
				continue
			}
			// Check if both are class names that match up to case
			if strings.EqualFold(ms, req) {
				continue
			}
			// t/T/"" all mean "unspecified" type
			if (ms == "" || strings.EqualFold(ms, "T")) && (req == "" || strings.EqualFold(req, "T")) {
				continue
			}
			match = false
			break
		}
		if match {
			mv := gcv()
			mv.typ = VMethod
			mv.str = gf.str
			mv.methodGF = gf
			mv.methodIdx = i
			return mv, nil
		}
	}
	if errorp {
		return nil, fmt.Errorf("find-method: no method matches qualifier %s and specializer-list", qualifier)
	}
	return vnil(), nil
}

// builtinRemoveMethod implements CL: REMOVE-METHOD
// (remove-method generic-function method)
func builtinRemoveMethod(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("remove-method: need generic-function and method")
	}
	gf := primaryValue(args[0])
	if gf.typ != VGeneric {
		return nil, fmt.Errorf("remove-method: first argument must be a generic function")
	}
	method := primaryValue(args[1])
	if method.typ == VMethod && method.methodGF == gf {
		idx := method.methodIdx
		if idx >= 0 && idx < len(gf.genMethods) {
			gf.genMethods = append(gf.genMethods[:idx], gf.genMethods[idx+1:]...)
		}
		return gf, nil
	}
	// Also allow passing a method found by find-method on a different GF reference
	// (same generic function but different Value pointer)
	if method.typ == VMethod {
		// Search by matching qualifier+specializers
		mQual := ""
		mSpecs := []string{}
		if method.methodGF != nil && method.methodIdx >= 0 && method.methodIdx < len(method.methodGF.genMethods) {
			srcM := method.methodGF.genMethods[method.methodIdx]
			mQual = srcM.qualifier
			mSpecs = srcM.specializers
		}
		for i, m := range gf.genMethods {
			if m.qualifier != mQual || len(m.specializers) != len(mSpecs) {
				continue
			}
			match := true
			for j, ms := range m.specializers {
				if ms != mSpecs[j] && !strings.EqualFold(ms, mSpecs[j]) {
					match = false
					break
				}
			}
			if match {
				gf.genMethods = append(gf.genMethods[:i], gf.genMethods[i+1:]...)
				return gf, nil
			}
		}
	}
	return nil, fmt.Errorf("remove-method: method not found in generic function")
}

// builtinComputeApplicableMethods implements CL: COMPUTE-APPLICABLE-METHODS
// (compute-applicable-methods generic-function arguments-list)
// Returns a list of applicable methods in order of decreasing precedence.
func builtinComputeApplicableMethods(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("compute-applicable-methods: need generic-function and arguments-list")
	}
	gf := primaryValue(args[0])
	if gf.typ != VGeneric {
		return nil, fmt.Errorf("compute-applicable-methods: first argument must be a generic function")
	}
	argList := args[1]
	evArgs := toSlice(argList)
	// Filter applicable methods
	var applicable []genMethod
	for _, m := range gf.genMethods {
		if methodApplicable(m, evArgs) {
			applicable = append(applicable, m)
		}
	}
	// Sort by specificity (lower score = more specific = comes first)
	sort.SliceStable(applicable, func(i, j int) bool {
		return methodSpecificity(applicable[i], evArgs) < methodSpecificity(applicable[j], evArgs)
	})
	// Convert to VMethod list
	result := vnil()
	for i := len(applicable) - 1; i >= 0; i-- {
		m := &applicable[i]
		mv := gcv()
		mv.typ = VMethod
		mv.str = m.qualifier
		mv.methodGF = gf
		mv.methodIdx = -1
		result = cons(mv, result)
	}
	return result, nil
}

// builtinMethodQualifiers implements CL: METHOD-QUALIFIERS
// (method-qualifiers method)
// Returns a list of qualifiers for the method.
func builtinMethodQualifiers(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("method-qualifiers: need a method")
	}
	m := primaryValue(args[0])
	if m.typ != VMethod {
		return nil, fmt.Errorf("method-qualifiers: argument must be a method")
	}
	if m.methodGF != nil && m.methodIdx >= 0 && m.methodIdx < len(m.methodGF.genMethods) {
		qual := m.methodGF.genMethods[m.methodIdx].qualifier
		if qual == "" {
			return vnil(), nil
		}
		return cons(vsym(qual), vnil()), nil
	}
	// Fallback: use the stored qualifier string
	if m.str == "" {
		return vnil(), nil
	}
	return cons(vsym(m.str), vnil()), nil
}

// generic-function-p is a type predicate
func builtinGenericFunctionP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("generic-function-p: need an object")
	}
	v := primaryValue(args[0])
	return vbool(v.typ == VGeneric), nil
}

func builtinSetMethodCombination(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("set-method-combination: need generic-function and combination-type")
	}
	gf := primaryValue(args[0])
	if gf.typ != VGeneric {
		return nil, fmt.Errorf("set-method-combination: first argument must be a generic function")
	}
	combo := primaryValue(args[1])
	comboStr := ""
	if combo.typ == VSym {
		comboStr = strings.ToUpper(combo.str)
	} else if combo.typ == VStr {
		comboStr = strings.ToUpper(combo.str)
	} else if combo.typ == VPair && combo.car != nil && combo.car.typ == VSym {
		// (progn) or (progn most-specific-first) etc.
		comboStr = strings.ToUpper(combo.car.str)
	}
	validCombos := map[string]bool{
		"STANDARD": true, "PROGN": true, "AND": true, "OR": true,
		"LIST": true, "APPEND": true, "NCONC": true,
		"MIN": true, "MAX": true, "+": true,
	}
	if !validCombos[comboStr] {
		return nil, fmt.Errorf("set-method-combination: unknown combination type %s", comboStr)
	}
	gf.methodCombo = strings.ToLower(comboStr)
	return gf, nil
}

// -------- Hash Table support --------

func sxhashVal(v *Value) uint64 {
	return sxhashSeen(v, make(map[*Value]bool))
}

func sxhashSeen(v *Value, seen map[*Value]bool) uint64 {
	if seen[v] {
		return 0
	}
	seen[v] = true
	switch v.typ {
	case VNil:
		return 0
	case VBool:
		if v == globalEnv.bindings["#t"] {
			return 1
		}
		return 2
	case VNum:
		return uint64(math.Float64bits(v.num))
	case VRat:
		return uint64(v.irat)*31 + uint64(v.iden)
	case VComplex:
		return uint64(math.Float64bits(v.num))*31 + uint64(math.Float64bits(v.imag))
	case VStr, VSym:
		h := uint64(0)
		for i := 0; i < len(v.str); i++ {
			h = h*31 + uint64(v.str[i])
		}
		return h
	case VPair:
		h := uint64(0x9e3779b97f4a7c15) // golden ratio constant for mixing
		h ^= sxhashSeen(v.car, seen)
		h *= 31
		h += sxhashSeen(v.cdr, seen)
		h ^= (h >> 33)
		return h
	case VArray:
		if v.array == nil {
			return 0
		}
		h := uint64(0x517cc1b727220a95) // different seed for arrays
		for _, elem := range v.array.elements {
			h = h*31 + sxhashSeen(elem, seen)
		}
		h ^= (h >> 33)
		return h
	case VVHash:
		return 3 // pointer-based
	default:
		return 3
	}
}

func hashTableKeyEqual(ht *HashTable, a, b *Value) bool {
	if ht.testFn == nil || ht.testFn.typ == VSym {
		// Default: use eqVal (like eql)
		return eqVal(a, b)
	}
	// Call the test function with pre-evaluated arguments
	result, err := callWithValueArgs(ht.testFn, []*Value{a, b})
	if err != nil {
		return false
	}
	return !isNil(result)
}

// callWithValueArgs calls a function with already-evaluated argument values
func callWithValueArgs(fn *Value, args []*Value) (*Value, error) {
	switch fn.typ {
	case VPrim:
		return fn.fn(args)
	case VFunc:
				newEnv := NewEnv(fn.env)
		numRequired := len(fn.params) - len(fn.optDefaults) - len(fn.keySpecs)
		if numRequired < 0 {
			numRequired = 0
		}

		keyVals := make(map[string]*Value)
		positionalArgs := args
		if len(fn.keySpecs) > 0 {
			var nonKeyword []*Value
			i := 0
			for i < len(args) {
				if args[i] != nil && args[i].typ == VSym && len(args[i].str) > 0 && args[i].str[0] == ':' {
					keyName := args[i].str[1:]
					if i+1 < len(args) {
						keyVals[keyName] = args[i+1]
						i += 2
					} else {
						nonKeyword = append(nonKeyword, args[i])
						i++
					}
				} else {
					nonKeyword = append(nonKeyword, args[i])
					i++
				}
			}
			positionalArgs = nonKeyword
		}

		for i := 0; i < numRequired; i++ {
			if i < len(positionalArgs) {
				newEnv.Set(fn.params[i], positionalArgs[i])
			} else {
				return nil, fmt.Errorf("call: missing required argument")
			}
		}

		paramIdx := numRequired
		for _, defaultExpr := range fn.optDefaults {
			if paramIdx < len(positionalArgs) {
				newEnv.Set(fn.params[paramIdx], positionalArgs[paramIdx])
			} else if !isNil(defaultExpr) {
				defVal, err := Eval(defaultExpr, newEnv)
				if err != nil {
					return nil, err
				}
				newEnv.Set(fn.params[paramIdx], defVal)
			} else {
				newEnv.Set(fn.params[paramIdx], vnil())
			}
			paramIdx++
		}

		paramIdx = numRequired + len(fn.optDefaults)
		for _, spec := range fn.keySpecs {
			if spec == nil || spec.typ != VPair || spec.car == nil || spec.cdr == nil || spec.cdr.typ != VPair || spec.cdr.cdr == nil || spec.cdr.cdr.typ != VPair {
				paramIdx++
				continue
			}
			keyName := spec.car.str
			if len(keyName) > 0 && keyName[0] == ':' {
				keyName = keyName[1:]
			}
			paramName := spec.cdr.car.str
			defaultExpr := spec.cdr.cdr.car
			if val, ok := keyVals[keyName]; ok {
				newEnv.Set(paramName, val)
			} else if !isNil(defaultExpr) {
				defVal, err := Eval(defaultExpr, newEnv)
				if err != nil {
					return nil, err
				}
				newEnv.Set(paramName, defVal)
			} else {
				newEnv.Set(paramName, vnil())
			}
			paramIdx++
		}

		if fn.rest != "" {
			var restElems []*Value
			if len(fn.keySpecs) > 0 {
				restElems = positionalArgs[paramIdx:]
			} else if fn.optDefaults != nil {
				restElems = positionalArgs[len(fn.params)-len(fn.optDefaults):]
			} else {
				restElems = positionalArgs[len(fn.params):]
			}
			newEnv.Set(fn.rest, listFromSlice(restElems))
		}
		body := fn.body
		if isNil(body) {
			return vnil(), nil
		}
		for body.typ == VPair && !isNil(body.cdr) {
			_, e := Eval(body.car, newEnv)
			if e != nil {
				return nil, e
			}
			body = body.cdr
		}
		return Eval(body.car, newEnv)
	default:
		return nil, fmt.Errorf("call: not a function")
	}
}

func builtinSxhash(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("sxhash: need argument")
	}
	return vnum(float64(sxhashVal(args[0]))), nil
}

func builtinMakeHashTable(args []*Value) (*Value, error) {
	ht := &HashTable{
		table:           make(map[uint64][]*hashEntry),
		rehashSize:      1.5,
		rehashThreshold: 1.0,
	}
	// Parse keyword args
	for i := 0; i < len(args); i++ {
		if args[i].typ == VSym && strings.HasPrefix(args[i].str, ":") {
			switch args[i].str {
			case ":TEST":
				if i+1 < len(args) {
					i++
					tv := args[i]
					if tv.typ != VSym && tv.typ != VPrim && tv.typ != VFunc && tv.typ != VGeneric {
						return nil, fmt.Errorf("make-hash-table: :test must be a function or symbol")
					}
					ht.testFn = args[i]
				}
			case ":HASH-FUNCTION":
				if i+1 < len(args) {
					i++
					ht.hashFn = args[i]
				}
			case ":REHASH-SIZE":
				if i+1 < len(args) {
					i++
					rsz := toNum(primaryValue(args[i]))
					if rsz > 0 {
						ht.rehashSize = rsz
					}
				}
			case ":REHASH-THRESHOLD":
				if i+1 < len(args) {
					i++
					rt := toNum(primaryValue(args[i]))
					if rt > 0 {
						ht.rehashThreshold = rt
					}
				}
			}
		}
	}
	v := gcv()
	v.typ = VVHash
	v.hashTab = ht
	return v, nil
}

func builtinHashTableP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	return vbool(args[0].typ == VVHash), nil
}

func builtinGethash(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("gethash: need key and hash-table")
	}
	key := args[0]
	ht := args[1]
	defaultVal := vnil()
	if len(args) > 2 {
		defaultVal = args[2]
	}
	if ht.typ != VVHash || ht.hashTab == nil {
		return multiVal(defaultVal, vnil()), nil
	}
	h := sxhashVal(key)
	bucket := ht.hashTab.table[h]
	for _, entry := range bucket {
		if hashTableKeyEqual(ht.hashTab, entry.key, key) {
			return multiVal(entry.value, vbool(true)), nil
		}
	}
	return multiVal(defaultVal, vnil()), nil
}

func builtinSetGethash(args []*Value) (*Value, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("gethash-setf: need value, key, and hash-table")
	}
	val := args[0]
	key := args[1]
	ht := args[2]
	if ht.typ != VVHash || ht.hashTab == nil {
		return nil, fmt.Errorf("gethash-setf: not a hash table")
	}
	h := sxhashVal(key)
	bucket := ht.hashTab.table[h]
	for i, entry := range bucket {
		if hashTableKeyEqual(ht.hashTab, entry.key, key) {
			bucket[i].value = val
			ht.hashTab.table[h] = bucket
			return val, nil
		}
	}
	// New entry
	entry := &hashEntry{key: key, value: val}
	ht.hashTab.table[h] = append(bucket, entry)
	ht.hashTab.count++
	return val, nil
}

func builtinRemhash(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("remhash: need key and hash-table")
	}
	key := args[0]
	ht := args[1]
	if ht.typ != VVHash || ht.hashTab == nil {
		return vnil(), nil
	}
	h := sxhashVal(key)
	bucket := ht.hashTab.table[h]
	for i, entry := range bucket {
		if hashTableKeyEqual(ht.hashTab, entry.key, key) {
			ht.hashTab.table[h] = append(bucket[:i], bucket[i+1:]...)
			ht.hashTab.count--
			return vbool(true), nil
		}
	}
	return vnil(), nil
}

func builtinHashTableExists(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("hash-table-exists?: need hash-table and key")
	}
	ht := args[0]
	key := args[1]
	if ht.typ != VVHash || ht.hashTab == nil {
		return vbool(false), nil
	}
	h := sxhashVal(key)
	bucket := ht.hashTab.table[h]
	for _, entry := range bucket {
		if hashTableKeyEqual(ht.hashTab, entry.key, key) {
			return vbool(true), nil
		}
	}
	return vbool(false), nil
}

func builtinClrhash(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("clrhash: need hash-table")
	}
	ht := args[0]
	if ht.typ != VVHash || ht.hashTab == nil {
		return nil, fmt.Errorf("clrhash: not a hash table")
	}
	ht.hashTab.table = make(map[uint64][]*hashEntry)
	ht.hashTab.count = 0
	return args[0], nil
}

func builtinHashTableCount(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("hash-table-count: need hash-table")
	}
	ht := args[0]
	if ht.typ != VVHash || ht.hashTab == nil {
		return nil, fmt.Errorf("hash-table-count: not a hash table")
	}
	return vnum(float64(ht.hashTab.count)), nil
}

func builtinMaphash(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("maphash: need function and hash-table")
	}
	fn := args[0]
	if fn.typ != VPrim && fn.typ != VFunc && fn.typ != VGeneric {
		return nil, fmt.Errorf("maphash: first argument must be a function, got %s", typeStr(fn))
	}
	ht := args[1]
	if ht.typ != VVHash || ht.hashTab == nil {
		return nil, fmt.Errorf("maphash: not a hash table")
	}
	for _, bucket := range ht.hashTab.table {
		for _, entry := range bucket {
			callArgs := []*Value{entry.key, entry.value}
			_, err := callWithValueArgs(fn, callArgs)
			if err != nil {
				return nil, err
			}
		}
	}
	return vnil(), nil
}

func builtinHashTableSize(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("hash-table-size: need hash-table")
	}
	ht := args[0]
	if ht.typ != VVHash || ht.hashTab == nil {
		return nil, fmt.Errorf("hash-table-size: not a hash table")
	}
	return vnum(float64(len(ht.hashTab.table))), nil
}

func builtinHashTableTest(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("hash-table-test: need hash-table")
	}
	ht := args[0]
	if ht.typ != VVHash || ht.hashTab == nil {
		return nil, fmt.Errorf("hash-table-test: not a hash table")
	}
	if ht.hashTab.testFn != nil && !isNil(ht.hashTab.testFn) {
		return ht.hashTab.testFn, nil
	}
	return vsym("eql"), nil
}

func builtinHashTableKeys(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("hash-table-keys: need hash-table")
	}
	ht := args[0]
	if ht.typ != VVHash || ht.hashTab == nil {
		return nil, fmt.Errorf("hash-table-keys: not a hash table")
	}
	var result *Value
	for _, bucket := range ht.hashTab.table {
		for _, entry := range bucket {
			result = cons(entry.key, result)
		}
	}
	return result, nil
}

func builtinHashTableValues(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("hash-table-values: need hash-table")
	}
	ht := args[0]
	if ht.typ != VVHash || ht.hashTab == nil {
		return nil, fmt.Errorf("hash-table-values: not a hash table")
	}
	var result *Value
	for _, bucket := range ht.hashTab.table {
		for _, entry := range bucket {
			result = cons(entry.value, result)
		}
	}
	return result, nil
}

func builtinHashTableRehashSize(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("hash-table-rehash-size: need hash-table")
	}
	ht := args[0]
	if ht.typ != VVHash || ht.hashTab == nil {
		return nil, fmt.Errorf("hash-table-rehash-size: not a hash table")
	}
	return vnum(args[0].hashTab.rehashSize), nil
}

func builtinHashTableRehashThreshold(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("hash-table-rehash-threshold: need hash-table")
	}
	ht := args[0]
	if ht.typ != VVHash || ht.hashTab == nil {
		return nil, fmt.Errorf("hash-table-rehash-threshold: not a hash table")
	}
	return vnum(ht.hashTab.rehashThreshold), nil
}

func builtinPush(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("push: need item and place")
	}
	item := args[0]
	place := args[1]
	if place.typ == VSym {
		currentVal, err := globalEnv.Get(place.str)
		if err != nil {
			currentVal = vnil()
		}
		newVal := cons(item, currentVal)
		globalEnv.Set(place.str, newVal)
		return newVal, nil
	}
	return nil, fmt.Errorf("push: second argument must be a symbol")
}

func builtinPushnew(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("pushnew: need item and place")
	}
	item := args[0]
	place := args[1]
	testFn := vnil()
	for i := 2; i < len(args); i++ {
		if args[i].typ == VSym && args[i].str == ":TEST" && i+1 < len(args) {
			testFn = args[i+1]
			i++
		}
	}
	if place.typ == VSym {
		currentVal, err := globalEnv.Get(place.str)
		if err != nil {
			currentVal = vnil()
		}
		// Check if item already in list
		for lst := currentVal; lst != nil && lst.typ == VPair; lst = lst.cdr {
			if !isNil(testFn) {
				res, err := callFnOnSeq(testFn, []*Value{item, lst.car}, globalEnv)
				if err != nil {
					return nil, err
				}
				if isTruthy(res) {
					return currentVal, nil
				}
			} else {
				if eqVal(item, lst.car) {
					return currentVal, nil
				}
			}
		}
		newVal := cons(item, currentVal)
		globalEnv.Set(place.str, newVal)
		return newVal, nil
	}
	return nil, fmt.Errorf("pushnew: second argument must be a symbol")
}

func builtinVectorPushExtend(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("vector-push-extend: need element and vector")
	}
	newEl := args[0]
	vec := args[1]
	extension := -1
	for i := 2; i < len(args); i++ {
		if args[i].typ == VSym && args[i].str == ":EXTENSION" && i+1 < len(args) {
			n, err := safeToNum(args[i+1], "vector-push-extend")
			if err != nil {
				return nil, err
			}
			extension = int(n)
			i++
		}
	}
	if vec.typ != VArray || vec.array == nil {
		return nil, fmt.Errorf("vector-push-extend: second argument must be a vector")
	}
	arr := vec.array
	if arr.fillPtr < 0 {
		return nil, fmt.Errorf("vector-push-extend: vector has no fill-pointer")
	}
	if arr.fillPtr >= len(arr.elements) {
		// Extend the vector
		ext := extension
		if ext <= 0 {
			ext = len(arr.elements)
			if ext < 16 {
				ext = 16
			}
		}
		newElems := make([]*Value, len(arr.elements)+ext)
		copy(newElems, arr.elements)
		for i := len(arr.elements); i < len(newElems); i++ {
			newElems[i] = vnil()
		}
		arr.elements = newElems
	}
	arr.elements[arr.fillPtr] = newEl
	fp := arr.fillPtr
	arr.fillPtr++
	return vnum(float64(fp)), nil
}

func builtinRowMajorAref(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("row-major-aref: need array and index")
	}
	arr := args[0]
	idx := args[1]
	if arr.typ != VArray || arr.array == nil {
		return nil, fmt.Errorf("row-major-aref: first argument must be an array")
	}
	n, err := safeToNum(idx, "row-major-aref")
	if err != nil {
		return nil, err
	}
	i := int(n)
	if i < 0 || i >= len(arr.array.elements) {
		return nil, fmt.Errorf("row-major-aref: index %d out of bounds", i)
	}
	return arr.array.elements[i], nil
}

func builtinMakeRandomState(args []*Value) (*Value, error) {
	v := gcv()
	v.typ = VRandomState
	if len(args) > 0 && !isNil(args[0]) {
		// Copy from existing state
		if args[0].typ == VRandomState && args[0].randState != nil {
			v.randState = rand.New(rand.NewSource(args[0].randState.Int63()))
		} else {
			v.randState = rand.New(rand.NewSource(time.Now().UnixNano()))
		}
	} else {
		v.randState = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	return v, nil
}

func builtinRandomStateP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	return vbool(args[0].typ == VRandomState), nil
}

func builtinCopyRandomState(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VRandomState {
		return builtinMakeRandomState(args)
	}
	v := gcv()
	v.typ = VRandomState
	v.randState = rand.New(rand.NewSource(args[0].randState.Int63()))
	return v, nil
}

func builtinGetUniversalTime(args []*Value) (*Value, error) {
	// Universal time: seconds since 1900-01-01 00:00:00 GMT
	t := time.Now()
	unixSec := t.Unix()
	// Offset from 1900 to 1970: 70 years worth of seconds
	offset := int64(2208988800)
	return vnum(float64(unixSec + offset)), nil
}

func builtinGetInternalRealTime(args []*Value) (*Value, error) {
	return vnum(float64(time.Now().UnixNano() / int64(time.Millisecond))), nil
}

func builtinDecodeUniversalTime(args []*Value) (*Value, error) {
	var ut float64
	if len(args) > 0 {
		ut = toNum(primaryValue(args[0]))
	} else {
		ut = float64(time.Now().Unix() + 2208988800)
	}
	t := time.Unix(int64(ut)-2208988800, 0)
	seconds := vnum(float64(t.Second()))
	minutes := vnum(float64(t.Minute()))
	hours := vnum(float64(t.Hour()))
	day := vnum(float64(t.Day()))
	month := vnum(float64(t.Month()))
	year := vnum(float64(t.Year()))
	dayOfWeek := vnum(float64(t.Weekday()))
	daylightSavingsP := vbool(false)
	timezone := vnum(0)
	return multiVal(seconds, minutes, hours, day, month, year, dayOfWeek, daylightSavingsP, timezone), nil
}

func builtinEncodeUniversalTime(args []*Value) (*Value, error) {
	if len(args) < 6 {
		return nil, fmt.Errorf("encode-universal-time: need second minute hour day month year")
	}
	sec := int(toNum(primaryValue(args[0])))
	min := int(toNum(primaryValue(args[1])))
	hour := int(toNum(primaryValue(args[2])))
	day := int(toNum(primaryValue(args[3])))
	month := int(toNum(primaryValue(args[4])))
	year := int(toNum(primaryValue(args[5])))
	t := time.Date(year, time.Month(month), day, hour, min, sec, 0, time.UTC)
	return vnum(float64(t.Unix() + 2208988800)), nil
}

func builtinLispImplementationType(args []*Value) (*Value, error) {
	return vstr("MicroLisp"), nil
}

func builtinLispImplementationVersion(args []*Value) (*Value, error) {
	return vstr("0.1.0"), nil
}

func builtinMachineType(args []*Value) (*Value, error) {
	return vstr(runtime.GOARCH), nil
}

func builtinMachineVersion(args []*Value) (*Value, error) {
	return vstr(""), nil
}

func builtinMachineInstance(args []*Value) (*Value, error) {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}
	return vstr(hostname), nil
}

func builtinSoftwareType(args []*Value) (*Value, error) {
	return vstr(runtime.GOOS), nil
}

func builtinSoftwareVersion(args []*Value) (*Value, error) {
	return vstr(""), nil
}

func builtinShortSiteName(args []*Value) (*Value, error) {
	return vstr(""), nil
}

func builtinLongSiteName(args []*Value) (*Value, error) {
	return vstr(""), nil
}

var processStartTime = time.Now()

func builtinGetInternalRunTime(args []*Value) (*Value, error) {
	return vnum(float64(time.Since(processStartTime).Milliseconds())), nil
}

// -------- Bit array operations --------

func bitArrayOp(args []*Value, op string) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("bit-%s: need two bit arrays", op)
	}
	a := args[0]
	b := args[1]
	if a.typ != VArray || a.array == nil {
		return nil, fmt.Errorf("bit-%s: first argument must be a bit array", op)
	}
	if b.typ != VArray || b.array == nil {
		return nil, fmt.Errorf("bit-%s: second argument must be a bit array", op)
	}
	aElems := a.array.elements
	bElems := b.array.elements
	n := len(aElems)
	if len(bElems) < n {
		n = len(bElems)
	}
	result := make([]*Value, n)
	for i := 0; i < n; i++ {
		av := toNum(primaryValue(aElems[i]))
		bv := toNum(primaryValue(bElems[i]))
		var rv float64
		switch op {
		case "and":
			if av != 0 && bv != 0 {
				rv = 1
			}
		case "ior":
			if av != 0 || bv != 0 {
				rv = 1
			}
		case "xor":
			if (av != 0) != (bv != 0) {
				rv = 1
			}
		case "eqv":
			if (av != 0) == (bv != 0) {
				rv = 1
			}
		case "nand":
			if !(av != 0 && bv != 0) {
				rv = 1
			}
		case "nor":
			if !(av != 0 || bv != 0) {
				rv = 1
			}
		case "orc1":
			if !(av != 0) || bv != 0 {
				rv = 1
			}
		case "orc2":
			if av != 0 || !(bv != 0) {
				rv = 1
			}
		case "andc1":
			if !(av != 0) && bv != 0 {
				rv = 1
			}
		case "andc2":
			if av != 0 && !(bv != 0) {
				rv = 1
			}
		}
		result[i] = vnum(rv)
	}
	v := gcv()
	v.typ = VArray
	v.array = &LispArray{
		elements: result,
		dims:     []int{n},
		fillPtr:  -1,
	}
	return v, nil
}

func builtinBitAnd(args []*Value) (*Value, error)    { return bitArrayOp(args, "and") }
func builtinBitIor(args []*Value) (*Value, error)    { return bitArrayOp(args, "ior") }
func builtinBitXor(args []*Value) (*Value, error)    { return bitArrayOp(args, "xor") }
func builtinBitEqv(args []*Value) (*Value, error)    { return bitArrayOp(args, "eqv") }
func builtinBitNand(args []*Value) (*Value, error)   { return bitArrayOp(args, "nand") }
func builtinBitNor(args []*Value) (*Value, error)    { return bitArrayOp(args, "nor") }
func builtinBitOrc1(args []*Value) (*Value, error)   { return bitArrayOp(args, "orc1") }
func builtinBitOrc2(args []*Value) (*Value, error)   { return bitArrayOp(args, "orc2") }
func builtinBitAndc1(args []*Value) (*Value, error) { return bitArrayOp(args, "andc1") }
func builtinBitAndc2(args []*Value) (*Value, error) { return bitArrayOp(args, "andc2") }

func builtinBitNot(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("bit-not: need a bit array")
	}
	a := args[0]
	if a.typ != VArray || a.array == nil {
		return nil, fmt.Errorf("bit-not: argument must be a bit array")
	}
	result := make([]*Value, len(a.array.elements))
	for i, el := range a.array.elements {
		v := toNum(primaryValue(el))
		if v != 0 {
			result[i] = vnum(0)
		} else {
			result[i] = vnum(1)
		}
	}
	rv := gcv()
	rv.typ = VArray
	rv.array = &LispArray{
		elements: result,
		dims:     []int{len(result)},
		fillPtr:  -1,
	}
	return rv, nil
}

// -------- Sequence operations --------

// seqParseKeys extracts keyword arguments from args[startIdx:]
func seqParseKeys(args []*Value, startIdx int) (keyFn, testFn, testNotFn *Value, fromEnd bool, count, start, end int, initialVal *Value, err error) {
	keyFn = vnil()
	testFn = vnil()
	testNotFn = vnil()
	count = -1
	start = 0
	end = -1
	initialVal = vnil()
	fromEnd = false
	for i := startIdx; i < len(args); i++ {
		if args[i].typ == VSym {
			switch args[i].str {
			case ":KEY":
				if i+1 < len(args) {
					i++
					keyFn = args[i]
				}
			case ":TEST":
				if i+1 < len(args) {
					i++
					testFn = args[i]
				}
			case ":TEST-NOT":
				if i+1 < len(args) {
					i++
					testNotFn = args[i]
				}
			case ":FROM-END":
				fromEnd = true
			case ":COUNT":
				if i+1 < len(args) {
					i++
					n, e := safeToNum(args[i], "keyword :count")
					if e != nil {
						err = e
						return
					}
					count = int(n)
				}
			case ":START":
				if i+1 < len(args) {
					i++
					n, e := safeToNum(args[i], "keyword :start")
					if e != nil {
						err = e
						return
					}
					start = int(n)
				}
			case ":END":
				if i+1 < len(args) {
					i++
					n, e := safeToNum(args[i], "keyword :end")
					if e != nil {
						err = e
						return
					}
					end = int(n)
				}
			case ":INITIAL-VALUE":
				if i+1 < len(args) {
					i++
					initialVal = args[i]
				}
			}
		}
	}
	return
}

// extractKeyFromRest extracts :key from the end of a sequence args slice.
// Returns the keyFn (or nil) and a new slice without the :key keyword arg.
func extractKeyFromRest(seqs []*Value) (*Value, []*Value) {
	keyFn := (*Value)(nil)
	// Scan from the end looking for :key keyword
	for i := len(seqs) - 1; i >= 0; i-- {
		if seqs[i].typ == VSym && seqs[i].str == ":KEY" && i+1 < len(seqs) {
			keyFn = seqs[i+1]
			// Build new slice without :key and its value
			result := make([]*Value, 0, len(seqs)-2)
			result = append(result, seqs[:i]...)
			result = append(result, seqs[i+2:]...)
			return keyFn, result
		}
	}
	return nil, seqs
}

// seqToList converts a sequence (list or vector-like) to a Go []*Value slice.
// For now, only lists are supported.
func seqToList(v *Value) []*Value {
	var result []*Value
	seen := make(map[*Value]bool)
	for !isNil(v) {
		if v.typ == VPair {
			if seen[v] {
				break // circular list
			}
			seen[v] = true
			result = append(result, v.car)
			v = v.cdr
		} else if v.typ == VStr {
			// Convert string to list of character values (CL: coerce "abc" 'list → #\a #\b #\c)
			for _, r := range v.str {
				result = append(result, vchar(r))
			}
			break
		} else if v.typ == VArray && v.array != nil {
			n := len(v.array.elements)
			if v.array.fillPtr >= 0 && v.array.fillPtr < n {
				n = v.array.fillPtr
			}
			for i := 0; i < n; i++ {
				result = append(result, v.array.elements[i])
			}
			break
		} else {
			break
		}
	}
	return result
}

func callFnOnSeq(fn *Value, args []*Value, env *Env) (*Value, error) {
	// Build argument list
	argList := vnil()
	for i := len(args) - 1; i >= 0; i-- {
		argList = &Value{typ: VPair, car: args[i], cdr: argList}
	}
	// Resolve if it's a symbol naming a function
	callFn := fn
	if fn.typ == VSym {
		resolved, err := env.Get(fn.str)
		if err == nil {
			callFn = resolved
		} else {
			return nil, fmt.Errorf("callFnOnSeq: undefined function %s", fn.str)
		}
	}
	switch callFn.typ {
	case VPrim:
		return callFn.fn(args)
	case VFunc:
		// Direct apply without re-evaluating args
		if callFn.name != "" && traceTable[callFn.name] {
			indent := strings.Repeat("  ", traceDepth)
			argStrs := make([]string, len(args))
			for i, a := range args {
				argStrs[i] = ToString(primaryValue(a))
			}
			fmt.Printf("%s%d: (%s %s)\n", indent, traceDepth, callFn.name, strings.Join(argStrs, " "))

			traceDepth++
		}
						newEnv := NewEnv(callFn.env)
		numRequired := len(callFn.params) - len(callFn.optDefaults) - len(callFn.keySpecs)
		if numRequired < 0 {
			numRequired = 0
		}

		keyVals := make(map[string]*Value)
		positionalArgs := args
		if len(callFn.keySpecs) > 0 {
			var nonKeyword []*Value
			i := 0
			for i < len(args) {
				if args[i] != nil && args[i].typ == VSym && len(args[i].str) > 0 && args[i].str[0] == ':' {
					keyName := args[i].str[1:]
					if i+1 < len(args) {
						keyVals[keyName] = args[i+1]
						i += 2
					} else {
						nonKeyword = append(nonKeyword, args[i])
						i++
					}
				} else {
					nonKeyword = append(nonKeyword, args[i])
					i++
				}
			}
			positionalArgs = nonKeyword
		}

		for i := 0; i < numRequired; i++ {
			if i < len(positionalArgs) {
				newEnv.Set(callFn.params[i], positionalArgs[i])
			} else {
				return nil, fmt.Errorf("call: missing required argument")
			}
		}

		paramIdx := numRequired
		for _, defaultExpr := range callFn.optDefaults {
			if paramIdx < len(positionalArgs) {
				newEnv.Set(callFn.params[paramIdx], positionalArgs[paramIdx])
			} else if !isNil(defaultExpr) {
				defVal, err := Eval(defaultExpr, newEnv)
				if err != nil {
					return nil, err
				}
				newEnv.Set(callFn.params[paramIdx], defVal)
			} else {
				newEnv.Set(callFn.params[paramIdx], vnil())
			}
			paramIdx++
		}

		paramIdx = numRequired + len(callFn.optDefaults)
		for _, spec := range callFn.keySpecs {
			if spec == nil || spec.typ != VPair || spec.car == nil || spec.cdr == nil || spec.cdr.typ != VPair || spec.cdr.cdr == nil || spec.cdr.cdr.typ != VPair {
				paramIdx++
				continue
			}
			keyName := spec.car.str
			if len(keyName) > 0 && keyName[0] == ':' {
				keyName = keyName[1:]
			}
			paramName := spec.cdr.car.str
			defaultExpr := spec.cdr.cdr.car
			if val, ok := keyVals[keyName]; ok {
				newEnv.Set(paramName, val)
			} else if !isNil(defaultExpr) {
				defVal, err := Eval(defaultExpr, newEnv)
				if err != nil {
					return nil, err
				}
				newEnv.Set(paramName, defVal)
			} else {
				newEnv.Set(paramName, vnil())
			}
			paramIdx++
		}

		if callFn.rest != "" {
			var restElems []*Value
			if len(callFn.keySpecs) > 0 {
				restElems = positionalArgs[paramIdx:]
			} else if callFn.optDefaults != nil {
				restElems = positionalArgs[len(callFn.params)-len(callFn.optDefaults):]
			} else {
				restElems = positionalArgs[len(callFn.params):]
			}
			newEnv.Set(callFn.rest, listFromSlice(restElems))
		}
		body := callFn.body
		if isNil(body) {
			ret := vnil()
			if callFn.name != "" && traceTable[callFn.name] {
				traceDepth--
				indent := strings.Repeat("  ", traceDepth)
				fmt.Printf("%s%d: <= %s\n", indent, traceDepth, ToString(ret))

			}
			return ret, nil
		}
		for body.typ == VPair && !isNil(body.cdr) {
			_, e := Eval(body.car, newEnv)
			if e != nil {
				if _, ok := e.(*tailCall); ok {
					return nil, e
				}
				return nil, e
			}
			body = body.cdr
		}
		// Evaluate the final expression, handling tail-call optimization
		for {
			ret, err := Eval(body.car, newEnv)
			if err == nil {
				if callFn.name != "" && traceTable[callFn.name] {
					traceDepth--
					indent := strings.Repeat("  ", traceDepth)
					fmt.Printf("%s%d: <= %s\n", indent, traceDepth, ToString(ret))
				}
				return ret, nil
			}
			if tc, ok := err.(*tailCall); ok {
				// TCO: update form/env and continue looping
				body = tc.form
				newEnv = tc.env
				continue
			}
			return nil, err
		}
	default:
		// Fallback: construct form and eval (for other function types)
		return Eval(&Value{typ: VPair, car: callFn, cdr: argList}, globalEnv)
	}
}

func builtinSeqMap(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("seq-map: need function and sequence")
	}
	fn := args[0]
	seq := args[1]
	elements := seqToList(seq)
	var result []*Value
	for _, el := range elements {
		val, err := callFnOnSeq(fn, []*Value{el}, globalEnv)
		if err != nil {
			return nil, err
		}
		result = append(result, val)
	}
	return listFromSlice(result), nil
}

func builtinSeqReduce(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("seq-reduce: need function and sequence")
	}
	fn := args[0]
	keyFn, _, _, fromEnd, _, start, end, initialVal, err := seqParseKeys(args, 2)
	if err != nil {
		return nil, err
	}
	seq := args[1]
	elements := seqToList(seq)
	if end < 0 || end > len(elements) {
		end = len(elements)
	}
	if start < 0 {
		start = 0
	}
	if start >= end || len(elements) == 0 {
		return initialVal, nil
	}
	hasInitialValue := boolFromKey(":initial-value", args, 2)

	if !fromEnd {
		// Left-to-right reduce (default)
		acc := initialVal
		startIdx := start
		if acc.typ == VNil && !hasInitialValue {
			acc = elements[startIdx]
			startIdx = start + 1
		}
		for i := startIdx; i < end; i++ {
			v := elements[i]
			if !isNil(keyFn) {
				var err error
				v, err = callFnOnSeq(keyFn, []*Value{v}, globalEnv)
				if err != nil {
					return nil, err
				}
			}
			var err error
			acc, err = callFnOnSeq(fn, []*Value{acc, v}, globalEnv)
			if err != nil {
				return nil, err
			}
		}
		return acc, nil
	} else {
		// Right-to-left reduce (:from-end t)
		acc := initialVal
		endIdx := end - 1
		if acc.typ == VNil && !hasInitialValue {
			acc = elements[endIdx]
			endIdx = end - 2
		}
		for i := endIdx; i >= start; i-- {
			v := elements[i]
			if !isNil(keyFn) {
				var err error
				v, err = callFnOnSeq(keyFn, []*Value{v}, globalEnv)
				if err != nil {
					return nil, err
				}
			}
			var err error
			// For :from-end, function is called as (fn element accumulator)
			acc, err = callFnOnSeq(fn, []*Value{v, acc}, globalEnv)
			if err != nil {
				return nil, err
			}
		}
		return acc, nil
	}
}

func boolFromKey(key string, args []*Value, startIdx int) bool {
	for i := startIdx; i < len(args); i++ {
		if args[i].typ == VSym && args[i].str == key {
			return true
		}
	}
	return false
}

func builtinSeqSort(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("seq-sort: need sequence and predicate")
	}
	seq := args[0]
	pred := args[1]
	keyFn, _, _, _, _, _, _, _, err := seqParseKeys(args, 2)
	if err != nil {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	elements := seqToList(seq)
	// Simple insertion sort (stable)
	for i := 1; i < len(elements); i++ {
		for j := i; j > 0; j-- {
			a, b := elements[j-1], elements[j]
			if !isNil(keyFn) {
				var err error
				a, err = callFnOnSeq(keyFn, []*Value{a}, globalEnv)
				if err != nil {
					return nil, err
				}
				b, err = callFnOnSeq(keyFn, []*Value{b}, globalEnv)
				if err != nil {
					return nil, err
				}
			}
			cmp, err := callFnOnSeq(pred, []*Value{b, a}, globalEnv) // (pred b a) means b < a → swap
			if err != nil {
				return nil, err
			}
			if isTruthy(cmp) {
				elements[j-1], elements[j] = elements[j], elements[j-1]
			} else {
				break
			}
		}
	}
	return listFromSlice(elements), nil
}

func builtinSeqRemoveIf(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("seq-remove-if: need predicate and sequence")
	}
	pred := args[0]
	seq := args[1]
	keyFn, _, _, _, count, start, end, _, err := seqParseKeys(args, 2)
	if err != nil {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	elements := seqToList(seq)
	if end < 0 || end > len(elements) {
		end = len(elements)
	}
	if start < 0 {
		start = 0
	}
	var result []*Value
	removed := 0
	for i, el := range elements {
		if i >= start && i < end && (count < 0 || removed < count) {
			v := el
			if !isNil(keyFn) {
				var err error
				v, err = callFnOnSeq(keyFn, []*Value{v}, globalEnv)
				if err != nil {
					return nil, err
				}
			}
			cmp, err := callFnOnSeq(pred, []*Value{v}, globalEnv)
			if err != nil {
				return nil, err
			}
			if isTruthy(cmp) {
				removed++
				continue
			}
		}
		result = append(result, el)
	}
	// If input was a string and all results are characters, return a string
	if seq.typ == VStr {
		var sb strings.Builder
		for _, v := range result {
			if v.typ == VChar {
				sb.WriteRune(v.ch)
			} else {
				return listFromSlice(result), nil
			}
		}
		return vstr(sb.String()), nil
	}
	// If input was a VArray, return a VArray
	if seq.typ == VArray {
		return varray(result), nil
	}
	return listFromSlice(result), nil
}

func builtinSeqFind(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("seq-find: need item and sequence")
	}
	item := args[0]
	seq := args[1]
	if err := checkSequenceArg(seq, "seq-find"); err != nil {
		return nil, err
	}
	keyFn, testFn, testNotFn, fromEnd, _, start, end, _, err := seqParseKeys(args, 2)
	if err != nil {
		return nil, err
	}
	elements := seqToList(seq)
	if end < 0 || end > len(elements) {
		end = len(elements)
	}
	if start < 0 {
		start = 0
	}
	if fromEnd {
		for i := end - 1; i >= start; i-- {
			if testItemMatchFull(item, elements[i], testFn, testNotFn, keyFn) {
				return elements[i], nil
			}
		}
	} else {
		for i := start; i < end; i++ {
			if testItemMatchFull(item, elements[i], testFn, testNotFn, keyFn) {
				return elements[i], nil
			}
		}
	}
	return vnil(), nil
}

func testItemMatch(item, el *Value, testFn, keyFn *Value) bool {
	return testItemMatchFull(item, el, testFn, nil, keyFn)
}

func testItemMatchFull(item, el, testFn, testNotFn, keyFn *Value) bool {
	// CL spec: test function is called as (test item (key element)).
	// The key function is applied to the ELEMENT only, not the item.
	a := el
	if !isNil(keyFn) {
		var err error
		a, err = callFnOnSeq(keyFn, []*Value{el}, globalEnv)
		if err != nil {
			return false
		}
	}
	b := item
	if !isNil(testNotFn) {
		cmp, err := callFnOnSeq(testNotFn, []*Value{b, a}, globalEnv)
		if err != nil {
			return false
		}
		return !isTruthy(cmp)
	}
	if !isNil(testFn) {
		cmp, err := callFnOnSeq(testFn, []*Value{b, a}, globalEnv)
		if err != nil {
			return false
		}
		return isTruthy(cmp)
	}
	return eqVal(a, b)
}

func builtinSeqPosition(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("seq-position: need item and sequence")
	}
	item := args[0]
	seq := args[1]
	keyFn, testFn, testNotFn, fromEnd, _, start, end, _, err := seqParseKeys(args, 2)
	if err != nil {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	elements := seqToList(seq)
	if end < 0 || end > len(elements) {
		end = len(elements)
	}
	if start < 0 {
		start = 0
	}
	if fromEnd {
		for i := end - 1; i >= start; i-- {
			if testItemMatchFull(item, elements[i], testFn, testNotFn, keyFn) {
				return vnum(float64(i)), nil
			}
		}
	} else {
		for i := start; i < end; i++ {
			if testItemMatchFull(item, elements[i], testFn, testNotFn, keyFn) {
				return vnum(float64(i)), nil
			}
		}
	}
	return vnil(), nil
}

func builtinSeqSubstitute(args []*Value) (*Value, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("seq-substitute: need new, old, and sequence")
	}
	newVal := args[0]
	old := args[1]
	seq := args[2]
	keyFn, testFn, testNotFn, fromEnd, count, start, end, _, err := seqParseKeys(args, 3)
	if err != nil {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	elements := seqToList(seq)
	if end < 0 || end > len(elements) {
		end = len(elements)
	}
	if start < 0 {
		start = 0
	}
	result := make([]*Value, len(elements))
	copy(result, elements)
	replaced := 0
	if fromEnd {
		for i := end - 1; i >= start; i-- {
			if count >= 0 && replaced >= count {
				break
			}
			if testItemMatchFull(old, elements[i], testFn, testNotFn, keyFn) {
				result[i] = newVal
				replaced++
			}
		}
	} else {
		for i := start; i < end; i++ {
			if count >= 0 && replaced >= count {
				break
			}
			if testItemMatchFull(old, elements[i], testFn, testNotFn, keyFn) {
				result[i] = newVal
				replaced++
			}
		}
	}
	// If input was a string and all results are characters, return a string
	if seq.typ == VStr {
		var sb strings.Builder
		for _, v := range result {
			if v.typ == VChar {
				sb.WriteRune(v.ch)
			} else {
				return listFromSlice(result), nil
			}
		}
		return vstr(sb.String()), nil
	}
	// If input was a VArray, return a VArray
	if seq.typ == VArray {
		return varray(result), nil
	}
	return listFromSlice(result), nil
}

// -------- subseq, concatenate, mapcan --------

func builtinSeqSubseq(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("subseq: need sequence and start")
	}
	seq := args[0]
	start := int(primaryValue(args[1]).num)
	// Handle strings specially: return a string, not a list
	if seq.typ == VStr {
		runes := []rune(seq.str)
		end := len(runes)
		if len(args) >= 3 && args[2].typ != VNil {
			end = int(primaryValue(args[2]).num)
		}
		if start < 0 {
			start = 0
		}
		if end < 0 {
			end = len(runes)
		}
		if start > len(runes) {
			start = len(runes)
		}
		if end > len(runes) {
			end = len(runes)
		}
		if start >= end {
			return vstr(""), nil
		}
		return vstr(string(runes[start:end])), nil
	}
	// Handle VArray specially: return a vector, not a list
	if seq.typ == VArray {
		return builtinSeqSubseqArray(args)
	}
	end := len(seqToList(seq))
	if len(args) >= 3 && args[2].typ != VNil {
		end = int(primaryValue(args[2]).num)
	}
	elements := seqToList(seq)
	if end < 0 {
		end = len(elements)
	}
	if start < 0 {
		start = 0
	}
	if start > len(elements) {
		start = len(elements)
	}
	if end > len(elements) {
		end = len(elements)
	}
	if start >= end {
		return vnil(), nil
	}
	return listFromSlice(elements[start:end]), nil
}

// subseq for VArray: return a new vector
func builtinSeqSubseqArray(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("subseq: need sequence and start")
	}
	seq := args[0]
	if seq.typ != VArray {
		return nil, fmt.Errorf("subseq: need array for array subseq")
	}
	elements := seqToList(seq)
	start := int(primaryValue(args[1]).num)
	end := len(elements)
	if len(args) >= 3 && args[2].typ != VNil {
		end = int(primaryValue(args[2]).num)
	}
	if start < 0 {
		start = 0
	}
	if end < 0 {
		end = len(elements)
	}
	if start > len(elements) {
		start = len(elements)
	}
	if end > len(elements) {
		end = len(elements)
	}
	if start >= end {
		return vnil(), nil
	}
	return varray(elements[start:end]), nil
}

func builtinSubseqSetf(args []*Value) (*Value, error) {
	if len(args) < 4 {
		return nil, fmt.Errorf("subseq-setf: need newval, seq, start, end")
	}
	newval := primaryValue(args[0])
	seq := primaryValue(args[1])
	start := int(primaryValue(args[2]).num)
	end := -1
	if args[3].typ != VNil {
		end = int(primaryValue(args[3]).num)
	}
	if seq.typ == VStr {
		// For strings, construct the modified string and update in place
		s := seq.str
		runes := []rune(s)
		if end < 0 || end > len(runes) {
			end = len(runes)
		}
		if start < 0 {
			start = 0
		}
		newStr := ""
		if newval.typ == VStr {
			newStr = newval.str
		}
		// Build new string: before + newStr + rest
		result := string(runes[:start]) + newStr + string(runes[end:])
		// Modify the string in place
		seq.str = result
		return newval, nil
	}
	// For lists, return as-is (modification of subseq is not meaningful)
	return newval, nil
}

func builtinSeqConcatenate(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("concatenate: need result-type and sequences")
	}
	resultType := primaryValue(args[0])
	typeName := ""
	if resultType.typ == VSym {
		typeName = strings.ToUpper(resultType.str)
	}
	// Collect all elements from input sequences
	var result []*Value
	for i := 1; i < len(args); i++ {
		result = append(result, seqToList(args[i])...)
	}
	switch typeName {
	case "STRING":
		var sb strings.Builder
		for _, v := range result {
			if v.typ == VChar {
				sb.WriteRune(v.ch)
			} else {
				sb.WriteString(ToString(v))
			}
		}
		return vstr(sb.String()), nil
	case "VECTOR", "SIMPLE-VECTOR", "ARRAY", "SIMPLE-ARRAY":
		return varray(result), nil
	case "LIST", "CONS", "NULL":
		return listFromSlice(result), nil
	case "BIT-VECTOR", "SIMPLE-BIT-VECTOR":
		bits := make([]*Value, len(result))
		for i, v := range result {
			if v.typ == VNum {
				if int(v.num) != 0 {
					bits[i] = vnum(1)
				} else {
					bits[i] = vnum(0)
				}
			} else {
				bits[i] = vnum(0)
			}
		}
		return varray(bits), nil
	default:
		// Default: return list
		return listFromSlice(result), nil
	}
}

// checkSequenceArg validates that v is a proper sequence for map functions.
func checkSequenceArg(v *Value, funcName string) error {
	v = primaryValue(v)
	if v.typ != VPair && v.typ != VNil && v.typ != VStr && v.typ != VArray {
		return fmt.Errorf("%s: expected a proper sequence", funcName)
	}
	return nil
}

// safeToNum returns the numeric value or an error for non-numeric types.
func safeToNum(v *Value, funcName string) (float64, error) {
	v = primaryValue(v)
	if v.typ != VNum && v.typ != VBigInt && v.typ != VRat {
		return 0, fmt.Errorf("%s: expected a number", funcName)
	}
	return toNum(v), nil
}

func builtinMapcan(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("mapcan: need function and at least one sequence")
	}
	fn := args[0]
	seqs := args[1:]
	for _, s := range seqs {
		if err := checkSequenceArg(s, "mapcan"); err != nil {
			return nil, err
		}
	}
	lists := make([][]*Value, len(seqs))
	for i, s := range seqs {
		lists[i] = seqToList(s)
	}
	if len(lists) == 0 {
		return vnil(), nil
	}
	maxLen := 0
	for _, l := range lists {
		if len(l) > maxLen {
			maxLen = len(l)
		}
	}
	var result []*Value
	for i := 0; i < maxLen; i++ {
		callArgs := make([]*Value, 0, len(lists))
		for _, l := range lists {
			if i < len(l) {
				callArgs = append(callArgs, l[i])
			}
		}
		callResult, err := callFnOnSeq(fn, callArgs, globalEnv)
		if err != nil {
			return nil, err
		}
		result = append(result, seqToList(callResult)...)
	}
	return listFromSlice(result), nil
}

func builtinMapcar(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("mapcar: need function and at least one list")
	}
	fn := args[0]
	seqs := args[1:]
	for _, s := range seqs {
		if err := checkSequenceArg(s, "mapcar"); err != nil {
			return nil, err
		}
	}
	lists := make([][]*Value, len(seqs))
	for i, s := range seqs {
		lists[i] = seqToList(s)
	}
	if len(lists) == 0 {
		return vnil(), nil
	}
	maxLen := 0
	for _, l := range lists {
		if len(l) > maxLen {
			maxLen = len(l)
		}
	}
	var result []*Value
	for i := 0; i < maxLen; i++ {
		callArgs := make([]*Value, 0, len(lists))
		for _, l := range lists {
			if i < len(l) {
				callArgs = append(callArgs, l[i])
			}
		}
		callResult, err := callFnOnSeq(fn, callArgs, globalEnv)
		if err != nil {
			return nil, err
		}
		result = append(result, callResult)
	}
	return listFromSlice(result), nil
}

func builtinMapInto(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("map-into: need result-sequence and function and sequence")
	}
	resultSeq := args[0]
	fn := args[1]
	seqs := args[2:]
	for _, s := range seqs {
		if err := checkSequenceArg(s, "map-into"); err != nil {
			return nil, err
		}
	}
	lists := make([][]*Value, len(seqs))
	for i, s := range seqs {
		lists[i] = seqToList(s)
	}
	maxLen := 0
	for _, l := range lists {
		if len(l) > maxLen {
			maxLen = len(l)
		}
	}
	var result []*Value
	if resultSeq.typ == VStr {
		var sb strings.Builder
		for i := 0; i < maxLen; i++ {
			callArgs := make([]*Value, 0, len(lists))
			for _, l := range lists {
				if i < len(l) {
					callArgs = append(callArgs, l[i])
				}
			}
			val, err := callFnOnSeq(fn, callArgs, globalEnv)
			if err != nil {
				return nil, err
			}
			sb.WriteString(ToString(primaryValue(val)))
		}
		return vstr(sb.String()), nil
	}
	// Handle VArray: modify elements in place and return the original array
	if resultSeq.typ == VArray && resultSeq.array != nil {
		elements := resultSeq.array.elements
		n := len(elements)
		if resultSeq.array.fillPtr >= 0 && resultSeq.array.fillPtr < n {
			n = resultSeq.array.fillPtr
		}
		for i := 0; i < maxLen && i < n; i++ {
			callArgs := make([]*Value, 0, len(lists))
			for _, l := range lists {
				if i < len(l) {
					callArgs = append(callArgs, l[i])
				}
			}
			val, err := callFnOnSeq(fn, callArgs, globalEnv)
			if err != nil {
				return nil, err
			}
			resultSeq.array.elements[i] = primaryValue(val)
		}
		return resultSeq, nil
	}
	result = seqToList(resultSeq)
	if len(result) == 0 && maxLen > 0 {
		result = make([]*Value, maxLen)
	}
	for i := 0; i < maxLen && i < len(result); i++ {
		callArgs := make([]*Value, 0, len(lists))
		for _, l := range lists {
			if i < len(l) {
				callArgs = append(callArgs, l[i])
			}
		}
		val, err := callFnOnSeq(fn, callArgs, globalEnv)
		if err != nil {
			return nil, err
		}
		result[i] = val
	}
	return listFromSlice(result), nil
}

func builtinMap(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("map: need result-type, function and sequences")
	}
	resultType := primaryValue(args[0])
	fn := args[1]
	seqs := args[2:]
	if len(seqs) == 0 {
		return nil, fmt.Errorf("map: need at least one sequence")
	}
	for _, s := range seqs {
		if err := checkSequenceArg(s, "map"); err != nil {
			return nil, err
		}
	}
	lists := make([][]*Value, len(seqs))
	for i, s := range seqs {
		lists[i] = seqToList(s)
	}
	maxLen := 0
	for _, l := range lists {
		if len(l) > maxLen {
			maxLen = len(l)
		}
	}
	var result []*Value
	for i := 0; i < maxLen; i++ {
		callArgs := make([]*Value, 0, len(lists))
		for _, l := range lists {
			if i < len(l) {
				callArgs = append(callArgs, l[i])
			}
		}
		val, err := callFnOnSeq(fn, callArgs, globalEnv)
		if err != nil {
			return nil, err
		}
		result = append(result, val)
	}
	// Convert based on result-type
	if isNil(resultType) || (resultType.typ == VSym && resultType.str == "NIL") {
		return vnil(), nil
	}
	if resultType.typ != VSym {
		return nil, fmt.Errorf("map: result-type must be a symbol, got %v", resultType)
	}
	switch resultType.str {
	case "LIST", "CONS":
		return listFromSlice(result), nil
	case "VECTOR":
		av := gcv()
		av.typ = VArray
		av.array = &LispArray{dims: []int{len(result)}, elements: result, fillPtr: -1}
		return av, nil
	case "STRING":
		var sb strings.Builder
		for _, v := range result {
			if v.typ == VChar {
				sb.WriteRune(v.ch)
			} else {
				sb.WriteString(ToString(primaryValue(v)))
			}
		}
		return vstr(sb.String()), nil
	default:
		return nil, fmt.Errorf("map: unsupported result-type: %s", resultType.str)
	}
}

func builtinRevappend(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("revappend: need two lists")
	}
	list1 := seqToList(args[0])
	list2 := seqToList(args[1])
	// Reverse list1 and append list2
	for i, j := 0, len(list1)-1; i < j; i, j = i+1, j-1 {
		list1[i], list1[j] = list1[j], list1[i]
	}
	result := make([]*Value, len(list1)+len(list2))
	copy(result, list1)
	copy(result[len(list1):], list2)
	return listFromSlice(result), nil
}

func builtinLdiff(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("ldiff: need two lists")
	}
	list := args[0]
	obj := args[1]
	var result []*Value
	seen := make(map[*Value]bool)
	for !isNil(list) && list.typ == VPair {
		if seen[list] {
			break
		}
		seen[list] = true
		if list == obj {
			break
		}
		result = append(result, list.car)
		list = list.cdr
	}
	return listFromSlice(result), nil
}

func builtinTailp(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return vbool(false), nil
	}
	sublist := args[0]
	list := args[1]
	seen := make(map[*Value]bool)
	for !isNil(list) && list.typ == VPair {
		if seen[list] {
			break
		}
		seen[list] = true
		if list == sublist {
			return vbool(true), nil
		}
		list = list.cdr
	}
	return vbool(list == sublist), nil
}

func builtinNthValue(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("nth-value: need index and form")
	}
	n := int(toNum(args[0]))
	form := args[1]
	result, err := Eval(form, globalEnv)
	if err != nil {
		return nil, err
	}
	// Get the nth value from a multiple-values result
	if result != nil && result.typ == VMultiVal {
		vals := cons(result.car, result.cdr) // full list of values
		i := 0
		for !isNil(vals) && vals.typ == VPair {
			if i == n {
				return vals.car, nil
			}
			vals = vals.cdr
			i++
		}
		return vnil(), nil
	}
	if n == 0 {
		return primaryValue(result), nil
	}
	return vnil(), nil
}

func builtinSome(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("some: need predicate and sequence")
	}
	fn := args[0]
	seqs := args[1:]
	keyFn, seqs := extractKeyFromRest(seqs)
	lists := make([][]*Value, len(seqs))
	for i, s := range seqs {
		lists[i] = seqToList(s)
	}
	maxLen := 0
	for _, l := range lists {
		if len(l) > maxLen {
			maxLen = len(l)
		}
	}
	for i := 0; i < maxLen; i++ {
		callArgs := make([]*Value, 0, len(lists))
		for _, l := range lists {
			if i < len(l) {
				el := l[i]
				if keyFn != nil {
					el, _ = callFnOnSeq(keyFn, []*Value{el}, globalEnv)
				}
				callArgs = append(callArgs, el)
			}
		}
		result, err := callFnOnSeq(fn, callArgs, globalEnv)
		if err != nil {
			return nil, err
		}
		if isTruthy(result) {
			return result, nil
		}
	}
	return vnil(), nil
}

func builtinEvery(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("every: need predicate and sequence")
	}
	fn := args[0]
	seqs := args[1:]
	keyFn, seqs := extractKeyFromRest(seqs)
	lists := make([][]*Value, len(seqs))
	for i, s := range seqs {
		lists[i] = seqToList(s)
	}
	maxLen := 0
	for _, l := range lists {
		if len(l) > maxLen {
			maxLen = len(l)
		}
	}
	for i := 0; i < maxLen; i++ {
		callArgs := make([]*Value, 0, len(lists))
		for _, l := range lists {
			if i < len(l) {
				el := l[i]
				if keyFn != nil {
					el, _ = callFnOnSeq(keyFn, []*Value{el}, globalEnv)
				}
				callArgs = append(callArgs, el)
			}
		}
		result, err := callFnOnSeq(fn, callArgs, globalEnv)
		if err != nil {
			return nil, err
		}
		if !isTruthy(result) {
			return vbool(false), nil
		}
	}
	return vbool(true), nil
}

func builtinNotany(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("notany: need predicate and sequence")
	}
	fn := args[0]
	seqs := args[1:]
	keyFn, seqs := extractKeyFromRest(seqs)
	lists := make([][]*Value, len(seqs))
	for i, s := range seqs {
		lists[i] = seqToList(s)
	}
	maxLen := 0
	for _, l := range lists {
		if len(l) > maxLen {
			maxLen = len(l)
		}
	}
	for i := 0; i < maxLen; i++ {
		callArgs := make([]*Value, 0, len(lists))
		for _, l := range lists {
			if i < len(l) {
				el := l[i]
				if keyFn != nil {
					el, _ = callFnOnSeq(keyFn, []*Value{el}, globalEnv)
				}
				callArgs = append(callArgs, el)
			}
		}
		result, err := callFnOnSeq(fn, callArgs, globalEnv)
		if err != nil {
			return nil, err
		}
		if isTruthy(result) {
			return vbool(false), nil
		}
	}
	return vbool(true), nil
}

func builtinNotevery(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("notevery: need predicate and sequence")
	}
	fn := args[0]
	seqs := args[1:]
	keyFn, seqs := extractKeyFromRest(seqs)
	lists := make([][]*Value, len(seqs))
	for i, s := range seqs {
		lists[i] = seqToList(s)
	}
	maxLen := 0
	for _, l := range lists {
		if len(l) > maxLen {
			maxLen = len(l)
		}
	}
	for i := 0; i < maxLen; i++ {
		callArgs := make([]*Value, 0, len(lists))
		for _, l := range lists {
			if i < len(l) {
				el := l[i]
				if keyFn != nil {
					el, _ = callFnOnSeq(keyFn, []*Value{el}, globalEnv)
				}
				callArgs = append(callArgs, el)
			}
		}
		result, err := callFnOnSeq(fn, callArgs, globalEnv)
		if err != nil {
			return nil, err
		}
		if !isTruthy(result) {
			return vbool(true), nil
		}
	}
	return vbool(false), nil
}

func builtinNconc(args []*Value) (*Value, error) {
	// nconc: destructive list concatenation
	var result, tail *Value
	for _, arg := range args {
		if isNil(arg) {
			continue
		}
		list := seqToList(arg)
		if len(list) == 0 {
			continue
		}
		if isNil(result) {
			result = listFromSlice(list)
			tail = result
		} else {
			tail.cdr = listFromSlice(list)
		}
		// Find new tail with cycle detection
		seen := make(map[*Value]bool)
		for !isNil(tail) && tail.typ == VPair && !isNil(tail.cdr) {
			if seen[tail] {
				break
			}
			seen[tail] = true
			tail = tail.cdr
		}
	}
	if isNil(result) {
		return vnil(), nil
	}
	return result, nil
}

func builtinAdjoin(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("adjoin: need item and list")
	}
	item := args[0]
	lst := args[1]
	// Check if item is already in list
	elements := seqToList(lst)
	for _, el := range elements {
		if eqVal(item, el) {
			return lst, nil
		}
	}
	return cons(item, lst), nil
}

func substHelper(new, old, tree *Value) *Value {
	if eqVal(tree, old) {
		return new
	}
	if tree.typ == VPair {
		// Cycle detection: use map-based tracking to prevent infinite loops on circular structures
		visited := make(map[*Value]bool)
		return substHelperCycle(new, old, tree, visited)
	}
	return tree
}

func substHelperCycle(new, old, tree *Value, visited map[*Value]bool) *Value {
	if eqVal(tree, old) {
		return new
	}
	if tree.typ != VPair {
		return tree
	}
	if visited[tree] {
		return tree // cycle detected, stop recursion
	}
	visited[tree] = true
	return cons(substHelperCycle(new, old, tree.car, visited), substHelperCycle(new, old, tree.cdr, visited))
}

func builtinSubst(args []*Value) (*Value, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("subst: need new, old, and tree")
	}
	return substHelper(args[0], args[1], args[2]), nil
}

func sublisHelper(alist, tree *Value) *Value {
	visited := make(map[*Value]bool)
	return sublisHelperCycle(alist, tree, visited)
}

func sublisHelperCycle(alist, tree *Value, visited map[*Value]bool) *Value {
	if tree.typ == VPair {
		if visited[tree] {
			return tree // cycle detected
		}
		visited[tree] = true
		// Check if tree itself is a key in alist
		for cur := alist; !isNil(cur) && cur.typ == VPair; cur = cur.cdr {
			pair := cur.car
			if pair.typ == VPair && eqVal(pair.car, tree) {
				return pair.cdr
			}
		}
		return cons(sublisHelperCycle(alist, tree.car, visited), sublisHelperCycle(alist, tree.cdr, visited))
	}
	// For atoms, check alist
	for cur := alist; !isNil(cur) && cur.typ == VPair; cur = cur.cdr {
		pair := cur.car
		if pair.typ == VPair && eqVal(pair.car, tree) {
			return pair.cdr
		}
	}
	return tree
}

func builtinSublis(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("sublis: need alist and tree")
	}
	return sublisHelper(args[0], args[1]), nil
}

func builtinTreeEqual(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return vbool(false), nil
	}
	// Cycle detection to prevent infinite recursion on circular structures
	visitedA := make(map[*Value]bool)
	visitedB := make(map[*Value]bool)
	var teq func(a, b *Value) bool
	teq = func(a, b *Value) bool {
		if a == b {
			return true
		}
		if visitedA[a] || visitedB[b] {
			return true // already compared, assume equal to avoid infinite loop
		}
		if a.typ == VPair {
			visitedA[a] = true
		}
		if b.typ == VPair {
			visitedB[b] = true
		}
		if a.typ == VPair && b.typ == VPair {
			return teq(a.car, b.car) && teq(a.cdr, b.cdr)
		}
		if a.typ == VNil && b.typ == VNil {
			return true
		}
		return eqVal(a, b)
	}
	return vbool(teq(args[0], args[1])), nil
}

// -------- subst-if --------
func builtinSubstIf(args []*Value) (*Value, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("subst-if: need new, predicate, and tree")
	}
	newVal := args[0]
	pred := args[1]
	tree := args[2]
	visited := make(map[*Value]bool)
	var helper func(t *Value) *Value
	helper = func(t *Value) *Value {
		if visited[t] {
			return t
		}
		if t.typ == VPair {
			visited[t] = true
		}
		result, err := callFnOnSeq(pred, []*Value{t}, globalEnv)
		if err == nil && isTruthy(result) {
			return newVal
		}
		if t.typ == VPair {
			return cons(helper(t.car), helper(t.cdr))
		}
		return t
	}
	return helper(tree), nil
}

// -------- subst-if-not --------
func builtinSubstIfNot(args []*Value) (*Value, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("subst-if-not: need new, predicate, and tree")
	}
	newVal := args[0]
	pred := args[1]
	tree := args[2]
	visited := make(map[*Value]bool)
	var helper func(t *Value) *Value
	helper = func(t *Value) *Value {
		if visited[t] {
			return t
		}
		if t.typ == VPair {
			visited[t] = true
		}
		result, err := callFnOnSeq(pred, []*Value{t}, globalEnv)
		if err == nil && !isTruthy(result) {
			return newVal
		}
		if t.typ == VPair {
			return cons(helper(t.car), helper(t.cdr))
		}
		return t
	}
	return helper(tree), nil
}

// -------- nsubst / nsubst-if / nsublis --------
func builtinNsubst(args []*Value) (*Value, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("nsubst: need new, old, and tree")
	}
	newVal := args[0]
	old := args[1]
	visited := make(map[*Value]bool)
	var helper func(t *Value) *Value
	helper = func(t *Value) *Value {
		if eqVal(t, old) {
			return newVal
		}
		if t.typ == VPair {
			if visited[t] {
				return t
			}
			visited[t] = true
			t.car = helper(t.car)
			t.cdr = helper(t.cdr)
		}
		return t
	}
	return helper(args[2]), nil
}

func builtinNsubstIf(args []*Value) (*Value, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("nsubst-if: need new, predicate, and tree")
	}
	newVal := args[0]
	pred := args[1]
	visited := make(map[*Value]bool)
	var helper func(t *Value) *Value
	helper = func(t *Value) *Value {
		result, err := callFnOnSeq(pred, []*Value{t}, globalEnv)
		if err == nil && isTruthy(result) {
			return newVal
		}
		if t.typ == VPair {
			if visited[t] {
				return t
			}
			visited[t] = true
			t.car = helper(t.car)
			t.cdr = helper(t.cdr)
		}
		return t
	}
	return helper(args[2]), nil
}

func builtinNsubstIfNot(args []*Value) (*Value, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("nsubst-if-not: need new, predicate, and tree")
	}
	newVal := args[0]
	pred := args[1]
	visited := make(map[*Value]bool)
	var helper func(t *Value) *Value
	helper = func(t *Value) *Value {
		result, err := callFnOnSeq(pred, []*Value{t}, globalEnv)
		if err == nil && !isTruthy(result) {
			return newVal
		}
		if t.typ == VPair {
			if visited[t] {
				return t
			}
			visited[t] = true
			t.car = helper(t.car)
			t.cdr = helper(t.cdr)
		}
		return t
	}
	return helper(args[2]), nil
}

func builtinNsublis(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("nsublis: need alist and tree")
	}
	alist := args[0]
	visited := make(map[*Value]bool)
	var helper func(t *Value) *Value
	helper = func(t *Value) *Value {
		for cur := alist; !isNil(cur) && cur.typ == VPair; cur = cur.cdr {
			pair := cur.car
			if pair.typ == VPair && eqVal(pair.car, t) {
				return pair.cdr
			}
		}
		if t.typ == VPair {
			if visited[t] {
				return t
			}
			visited[t] = true
			t.car = helper(t.car)
			t.cdr = helper(t.cdr)
		}
		return t
	}
	return helper(args[1]), nil
}



// -------- stable-sort (builtin) --------
func builtinStableSort(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("stable-sort: need sequence and predicate")
	}
	seq := args[0]
	pred := args[1]
	keyFn := vnil()
	for i := 2; i < len(args); i++ {
		if args[i].typ == VSym && args[i].str == ":KEY" && i+1 < len(args) {
			i++
			keyFn = args[i]
		}
	}
	elems := seqToList(seq)
	// Insertion sort (stable)
	for i := 1; i < len(elems); i++ {
		key := elems[i]
		var keyVal *Value
		if !isNil(keyFn) {
			var err error
			keyVal, err = callFnOnSeq(keyFn, []*Value{key}, globalEnv)
			if err != nil {
				return nil, err
			}
		} else {
			keyVal = key
		}
		j := i - 1
		for j >= 0 {
			var jVal *Value
			if !isNil(keyFn) {
				var err error
				jVal, err = callFnOnSeq(keyFn, []*Value{elems[j]}, globalEnv)
				if err != nil {
					return nil, err
				}
			} else {
				jVal = elems[j]
			}
			cmp, err := callFnOnSeq(pred, []*Value{keyVal, jVal}, globalEnv)
			if err != nil {
				return nil, err
			}
			if !isTruthy(cmp) {
				break
			}
			elems[j+1] = elems[j]
			j--
		}
		elems[j+1] = key
	}
	if seq.typ == VStr {
		var sb strings.Builder
		for _, v := range elems {
			sb.WriteString(ToString(v))
		}
		return vstr(sb.String()), nil
	}
	return listFromSlice(elems), nil
}

func builtinUnion(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("union: need two lists")
	}
	list1 := seqToList(args[0])
	list2 := seqToList(args[1])
	seen := make(map[string]bool)
	var result []*Value
	for _, v := range list1 {
		key := ToString(v)
		if !seen[key] {
			seen[key] = true
			result = append(result, v)
		}
	}
	for _, v := range list2 {
		key := ToString(v)
		if !seen[key] {
			seen[key] = true
			result = append(result, v)
		}
	}
	return listFromSlice(result), nil
}

func builtinIntersection(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("intersection: need two lists")
	}
	list1 := seqToList(args[0])
	list2 := seqToList(args[1])
	set2 := make(map[string]bool)
	for _, v := range list2 {
		set2[ToString(v)] = true
	}
	seen := make(map[string]bool)
	var result []*Value
	for _, v := range list1 {
		key := ToString(v)
		if set2[key] && !seen[key] {
			seen[key] = true
			result = append(result, v)
		}
	}
	return listFromSlice(result), nil
}

func builtinSetDifference(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("set-difference: need two lists")
	}
	list1 := seqToList(args[0])
	list2 := seqToList(args[1])
	set2 := make(map[string]bool)
	for _, v := range list2 {
		set2[ToString(v)] = true
	}
	var result []*Value
	for _, v := range list1 {
		if !set2[ToString(v)] {
			result = append(result, v)
		}
	}
	return listFromSlice(result), nil
}

func builtinSetExclusiveOr(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("set-exclusive-or: need two lists")
	}
	list1 := seqToList(args[0])
	list2 := seqToList(args[1])
	set1 := make(map[string]bool)
	set2 := make(map[string]bool)
	for _, v := range list1 {
		set1[ToString(v)] = true
	}
	for _, v := range list2 {
		set2[ToString(v)] = true
	}
	var result []*Value
	for _, v := range list1 {
		if !set2[ToString(v)] {
			result = append(result, v)
		}
	}
	for _, v := range list2 {
		if !set1[ToString(v)] {
			result = append(result, v)
		}
	}
	return listFromSlice(result), nil
}

func builtinNsetExclusiveOr(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("nset-exclusive-or: need two lists")
	}
	list1 := seqToList(args[0])
	list2 := seqToList(args[1])
	set1 := make(map[string]bool)
	set2 := make(map[string]bool)
	for _, v := range list1 {
		set1[ToString(v)] = true
	}
	for _, v := range list2 {
		set2[ToString(v)] = true
	}
	var result []*Value
	for _, v := range list1 {
		if !set2[ToString(v)] {
			result = append(result, v)
		}
	}
	for _, v := range list2 {
		if !set1[ToString(v)] {
			result = append(result, v)
		}
	}
	return listFromSlice(result), nil
}

func builtinSubsetp(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return vbool(false), nil
	}
	list1 := seqToList(args[0])
	list2 := seqToList(args[1])
	set2 := make(map[string]bool)
	for _, v := range list2 {
		set2[ToString(v)] = true
	}
	for _, v := range list1 {
		if !set2[ToString(v)] {
			return vbool(false), nil
		}
	}
	return vbool(true), nil
}

func builtinCopyStructure(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("copy-structure: need an instance")
	}
	inst := args[0]
	if inst.typ != VInstance {
		return nil, fmt.Errorf("copy-structure: expected a structure instance, got %s", typeStr(inst))
	}
	newSlots := make(map[string]*Value, len(inst.instSlots))
	for k, v := range inst.instSlots {
		newSlots[k] = v // shallow copy of slots
	}
	result := gcv()
	result.typ = VInstance
	result.instClass = inst.instClass
	result.instSlots = newSlots
	return result, nil
}

func builtinCopyList(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	elems := seqToList(args[0])
	result := make([]*Value, len(elems))
	copy(result, elems)
	return listFromSlice(result), nil
}

func copyTreeHelper(v *Value) *Value {
	return copyTreeCycle(v, make(map[*Value]bool))
}

func copyTreeCycle(v *Value, seen map[*Value]bool) *Value {
	if v == nil || v.typ != VPair {
		return v
	}
	if seen[v] {
		// Cycle detected — create a self-referencing pair by returning nil
		return vnil()
	}
	seen[v] = true
	return cons(copyTreeCycle(v.car, seen), copyTreeCycle(v.cdr, seen))
}

func builtinCopyTree(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	return copyTreeHelper(args[0]), nil
}

func builtinListLength(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnum(0), nil
	}
	count := 0
	v := args[0]
	seen := make(map[*Value]bool)
	for v != nil && v.typ == VPair {
		if seen[v] {
			return nil, fmt.Errorf("list-length: circular list")
		}
		seen[v] = true
		count++
		v = v.cdr
	}
	return vnum(float64(count)), nil
}

func builtinLast(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	n := 1
	if len(args) > 1 {
		n = int(toNum(args[1]))
	}
	v := args[0]
	var elems []*Value
	seen := make(map[*Value]bool)
	for v != nil && v.typ == VPair {
		if seen[v] {
			return nil, fmt.Errorf("last: circular list")
		}
		seen[v] = true
		elems = append(elems, v)
		v = v.cdr
	}
	if n <= 0 || len(elems) == 0 {
		return vnil(), nil
	}
	if n >= len(elems) {
		return args[0], nil
	}
	return elems[len(elems)-n], nil
}

func builtinLastPair(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	v := args[0]
	if v.typ != VPair {
		return vnil(), nil
	}
	seen := make(map[*Value]bool)
	for v.typ == VPair && !isNil(v.cdr) && v.cdr.typ == VPair {
		if seen[v] {
			return nil, fmt.Errorf("last-pair: circular list")
		}
		seen[v] = true
		v = v.cdr
	}
	return v, nil
}

func builtinButlast(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	n := 1
	if len(args) > 1 {
		n = int(toNum(args[1]))
	}
	list := args[0]
	if n <= 0 {
		// butlast with n=0 returns a copy of the list (including dotted tail)
		var result *Value = vnil()
		var tail *Value
		c := list
		for !isNil(c) && c.typ == VPair {
			newCell := cons(c.car, vnil())
			if result == nil || result.typ != VPair {
				result = newCell
				tail = newCell
			} else {
				tail.cdr = newCell
				tail = newCell
			}
			c = c.cdr
		}
		// Preserve dotted tail
		if !isNil(c) && result.typ == VPair {
			// find last pair
			last := result
			for last.cdr.typ == VPair {
				last = last.cdr
			}
			last.cdr = c
		}
		return result, nil
	}
	// n > 0: walk list counting elements
	// When n > 0, the dotted tail is part of the last cons cells being removed,
	// so we should NOT preserve it in the result.
	cur := list
	var elems []*Value
	for cur.typ == VPair {
		elems = append(elems, cur.car)
		cur = cur.cdr
	}
	if n >= len(elems) {
		return vnil(), nil
	}
	keep := len(elems) - n
	// Rebuild with first 'keep' elements as a proper list
	result := vnil()
	for i := keep - 1; i >= 0; i-- {
		result = cons(elems[i], result)
	}
	return result, nil
}

func builtinNbutlast(args []*Value) (*Value, error) {
	return builtinButlast(args)
}

func builtinPairlis(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("pairlis: need two lists")
	}
	keys := seqToList(args[0])
	vals := seqToList(args[1])
	var result []*Value
	for i := 0; i < len(keys) && i < len(vals); i++ {
		result = append(result, cons(keys[i], vals[i]))
	}
	alist := vnil()
	if len(args) >= 3 {
		alist = args[2]
	}
	// Append result to alist: (nconc result alist)
	if len(result) == 0 {
		return alist, nil
	}
	res := listFromSlice(result)
	// Find end of result list and set cdr to alist
	t := res
	for t.typ == VPair && !isNil(t.cdr) {
		t = t.cdr
	}
	t.cdr = alist
	return res, nil
}

// -------- assoc --------
func builtinAssoc(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("assoc: need item and alist")
	}
	item := args[0]
	alist := args[1]
	keyFn, testFn, testNotFn, _, _, _, _, _, err := seqParseKeys(args, 2)
	if err != nil {
		return nil, err
	}
	cur := alist
	for !isNil(cur) && cur.typ == VPair {
		entry := cur.car
		// Skip nil elements (per CL spec)
		if isNil(entry) {
			cur = cur.cdr
			continue
		}
		if entry.typ == VPair {
			if testItemMatchFull(item, entry.car, testFn, testNotFn, keyFn) {
				return entry, nil
			}
		}
		cur = cur.cdr
	}
	return vnil(), nil
}

func builtinAssocIf(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("assoc-if: need predicate and alist")
	}
	fn := args[0]
	alist := args[1]
	seen := make(map[*Value]bool)
	for !isNil(alist) && alist.typ == VPair {
		if seen[alist] {
			break
		}
		seen[alist] = true
		pair := alist.car
		if pair.typ == VPair {
			result, err := callFnOnSeq(fn, []*Value{pair.car}, globalEnv)
			if err != nil {
				return nil, err
			}
			if isTruthy(result) {
				return pair, nil
			}
		}
		alist = alist.cdr
	}
	return vnil(), nil
}

func builtinMemberIf(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("member-if: need predicate and list")
	}
	fn := args[0]
	lst := args[1]
	seen := make(map[*Value]bool)
	for !isNil(lst) && lst.typ == VPair {
		if seen[lst] {
			break
		}
		seen[lst] = true
		result, err := callFnOnSeq(fn, []*Value{lst.car}, globalEnv)
		if err != nil {
			return nil, err
		}
		if isTruthy(result) {
			return lst, nil
		}
		lst = lst.cdr
	}
	return vnil(), nil
}

func builtinMemberIfNot(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("member-if-not: need predicate and list")
	}
	fn := args[0]
	lst := args[1]
	seen := make(map[*Value]bool)
	for !isNil(lst) && lst.typ == VPair {
		if seen[lst] {
			break
		}
		seen[lst] = true
		result, err := callFnOnSeq(fn, []*Value{lst.car}, globalEnv)
		if err != nil {
			return nil, err
		}
		if !isTruthy(result) {
			return lst, nil
		}
		lst = lst.cdr
	}
	return vnil(), nil
}

func builtinAssocIfNot(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("assoc-if-not: need predicate and alist")
	}
	fn := args[0]
	alist := args[1]
	seen := make(map[*Value]bool)
	for !isNil(alist) && alist.typ == VPair {
		if seen[alist] {
			break
		}
		seen[alist] = true
		pair := alist.car
		if pair.typ == VPair {
			result, err := callFnOnSeq(fn, []*Value{pair.car}, globalEnv)
			if err != nil {
				return nil, err
			}
			if !isTruthy(result) {
				return pair, nil
			}
		}
		alist = alist.cdr
	}
	return vnil(), nil
}

func builtinRassocIfNot(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("rassoc-if-not: need predicate and alist")
	}
	fn := args[0]
	alist := args[1]
	cur := alist
	for cur != nil && cur.typ == VPair {
		entry := cur.car
		if entry.typ == VPair {
			result, err := callFnOnSeq(fn, []*Value{entry.cdr}, globalEnv)
			if err != nil {
				return nil, err
			}
			if !isTruthy(result) {
				return entry, nil
			}
		}
		cur = cur.cdr
	}
	return vnil(), nil
}

func builtinMember(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("member: need item and list")
	}
	item := args[0]
	lst := args[1]
	if lst.typ != VPair && lst.typ != VNil {
		return nil, fmt.Errorf("member: expected a proper list")
	}
	keyFn, testFn, testNotFn, _, _, _, _, _, err := seqParseKeys(args, 2)
	if err != nil {
		return nil, err
	}
	seen := make(map[*Value]bool)
	for !isNil(lst) && lst.typ == VPair {
		if seen[lst] {
			break
		}
		seen[lst] = true
		el := lst.car
		if testItemMatchFull(item, el, testFn, testNotFn, keyFn) {
			return lst, nil
		}
		lst = lst.cdr
	}
	return vnil(), nil
}

func builtinPosition(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("position: need item and sequence")
	}
	item := args[0]
	seq := args[1]
	if seq.typ != VStr {
		if err := checkSequenceArg(seq, "position"); err != nil {
			return nil, err
		}
	}
	start, end := 0, -1
	for i := 2; i < len(args); i++ {
		if args[i].typ == VSym {
			switch args[i].str {
			case ":START":
				if i+1 < len(args) {
					i++
					n, e := safeToNum(args[i], "position")
					if e != nil {
						return nil, e
					}
					start = int(n)
				}
			case ":END":
				if i+1 < len(args) {
					i++
					n, e := safeToNum(args[i], "position")
					if e != nil {
						return nil, e
					}
					end = int(n)
				}
			}
		}
	}
	if seq.typ == VStr {
		s := seq.str
		runes := []rune(s)
		var targetCh rune
		if item.typ == VChar {
			targetCh = item.ch
		} else {
			return vnil(), nil
		}
		if end < 0 || end > len(runes) {
			end = len(runes)
		}
		for i := start; i < end; i++ {
			if runes[i] == targetCh {
				return vnum(float64(i)), nil
			}
		}
		return vnil(), nil
	}
	elems := seqToList(seq)
	if end < 0 || end > len(elems) {
		end = len(elems)
	}
	for i := start; i < end; i++ {
		if eqVal(elems[i], item) {
			return vnum(float64(i)), nil
		}
	}
	return vnil(), nil
}

func builtinPositionIf(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("position-if: need predicate and sequence")
	}
	fn := args[0]
	seq := args[1]
	elems := seqToList(seq)
	for i, el := range elems {
		result, err := callFnOnSeq(fn, []*Value{el}, globalEnv)
		if err != nil {
			return nil, err
		}
		if isTruthy(result) {
			return vnum(float64(i)), nil
		}
	}
	return vnil(), nil
}

func builtinSeqCount(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("seq-count: need item and sequence")
	}
	item := args[0]
	seq := args[1]
	keyFn, testFn, testNotFn, _, _, start, end, _, err := seqParseKeys(args, 2)
	if err != nil {
		return nil, err
	}
	elements := seqToList(seq)
	if end < 0 || end > len(elements) {
		end = len(elements)
	}
	if start < 0 {
		start = 0
	}
	cnt := 0
	for i := start; i < end; i++ {
		if testItemMatchFull(item, elements[i], testFn, testNotFn, keyFn) {
			cnt++
		}
	}
	return vnum(float64(cnt)), nil
}

func builtinSeqCountIf(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("seq-count-if: need predicate and sequence")
	}
	pred := args[0]
	seq := args[1]
	keyFn, _, _, _, _, start, end, _, err := seqParseKeys(args, 2)
	if err != nil {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	elements := seqToList(seq)
	if end < 0 || end > len(elements) {
		end = len(elements)
	}
	if start < 0 {
		start = 0
	}
	cnt := 0
	for i := start; i < end; i++ {
		v := elements[i]
		if !isNil(keyFn) {
			var err error
			v, err = callFnOnSeq(keyFn, []*Value{v}, globalEnv)
			if err != nil {
				return nil, err
			}
		}
		cmp, err := callFnOnSeq(pred, []*Value{v}, globalEnv)
		if err != nil {
			return nil, err
		}
		if isTruthy(cmp) {
			cnt++
		}
	}
	return vnum(float64(cnt)), nil
}

func builtinSeqRemove(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("seq-remove: need item and sequence")
	}
	item := args[0]
	seq := args[1]
	keyFn, testFn, testNotFn, fromEnd, count, start, end, _, err := seqParseKeys(args, 2)
	if err != nil {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	elements := seqToList(seq)
	if end < 0 || end > len(elements) {
		end = len(elements)
	}
	if start < 0 {
		start = 0
	}
	var result []*Value
	removed := 0
	if fromEnd {
		// Build from end: collect indices to remove
		removeSet := map[int]bool{}
		for i := end - 1; i >= start; i-- {
			if count >= 0 && removed >= count {
				break
			}
			if testItemMatchFull(item, elements[i], testFn, testNotFn, keyFn) {
				removeSet[i] = true
				removed++
			}
		}
		for i, el := range elements {
			if !removeSet[i] {
				result = append(result, el)
			}
		}
	} else {
		for i, el := range elements {
			if i >= start && i < end && (count < 0 || removed < count) {
				if testItemMatchFull(item, el, testFn, testNotFn, keyFn) {
					removed++
					continue
				}
			}
			result = append(result, el)
		}
	}
	// If input was a string and all results are characters, return a string
	if seq.typ == VStr {
		var sb strings.Builder
		for _, v := range result {
			if v.typ == VChar {
				sb.WriteRune(v.ch)
			} else {
				return listFromSlice(result), nil
			}
		}
		return vstr(sb.String()), nil
	}
	// If input was a VArray, return a VArray
	if seq.typ == VArray {
		return varray(result), nil
	}
	return listFromSlice(result), nil
}

// builtinDelete is the destructive version of remove - it modifies the list in-place
func builtinDelete(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("delete: need item and sequence")
	}
	item := args[0]
	seq := args[1]
	if seq.typ == VNil {
		return seq, nil
	}
	// For strings, delegate to remove (strings are immutable in CL)
	if seq.typ == VStr {
		return builtinSeqRemove(args)
	}
	if seq.typ != VPair {
		return nil, fmt.Errorf("delete: expected a list, got %s", typeStr(seq))
	}
	keyFn, testFn, _, fromEnd, count, start, end, _, err := seqParseKeys(args, 2)
	if err != nil {
		return nil, err
	}

	// Normalize start/end
	if end < 0 {
		// Compute length to get real end
		seen := make(map[*Value]bool)
		length := 0
		for cur := seq; !isNil(cur) && cur.typ == VPair; {
			if seen[cur] {
				break
			}
			seen[cur] = true
			length++
			cur = cur.cdr
		}
		end = length
	}
	if start < 0 {
		start = 0
	}

	// Destructive in-place deletion by rewiring .cdr pointers
	var head *Value = seq
	if fromEnd {
		// First pass: find indices to remove (count from end)
			// First pass: find indices to remove (count from end)
		elements := seqToList(seq)
		removeSet := map[int]bool{}
		removed := 0
		for i := len(elements) - 1; i >= 0; i-- {
			if count >= 0 && removed >= count {
				break
			}
			if testItemMatch(item, elements[i], testFn, keyFn) {
				removeSet[i] = true
				removed++
			}
		}
		// Second pass: rewiring (skip matching elements)
		// head stays as is; we rewire the tail
		prev := (*Value)(nil)
		cur := head
		i := 0
		for !isNil(cur) && cur.typ == VPair {
			if !removeSet[i] {
				prev = cur
			} else {
				if prev == nil {
					head = cur.cdr
				} else {
					prev.cdr = cur.cdr
				}
			}
			cur = cur.cdr
			i++
		}
	} else {
		// Forward pass: build result list by rewiring .cdr
		// head may change if first element is removed
		prev := (*Value)(nil)
		cur := head
		i := 0
		removed := 0
		for !isNil(cur) && cur.typ == VPair {
			withinRange := i >= start && (count < 0 || removed < count)
			if withinRange && testItemMatch(item, cur.car, testFn, keyFn) {
				// Remove this cell by rewiring prev.cdr
				if prev == nil {
					head = cur.cdr
				} else {
					prev.cdr = cur.cdr
				}
				cur = cur.cdr
				removed++
			} else {
				prev = cur
				cur = cur.cdr
			}
			i++
		}
	}
	return head, nil
}

// builtinDeleteIf - destructive version of remove-if
func builtinDeleteIf(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("delete-if: need predicate and sequence")
	}
	pred := args[0]
	seq := args[1]
	if seq.typ == VNil {
		return seq, nil
	}
	// For strings and vectors, delegate to remove-if (non-destructive for vectors)
	if seq.typ == VStr || seq.typ == VArray {
		return builtinSeqRemoveIf(args)
	}
	if seq.typ != VPair {
		return nil, fmt.Errorf("delete-if: expected a list, got %s", typeStr(seq))
	}
	keyFn, _, _, _, count, start, end, _, err := seqParseKeys(args, 2)
	if err != nil {
		return nil, err
	}
	if end < 0 {
		seen := make(map[*Value]bool)
		length := 0
		for cur := seq; !isNil(cur) && cur.typ == VPair; {
			if seen[cur] { break }
			seen[cur] = true
			length++
			cur = cur.cdr
		}
		end = length
	}
	if start < 0 { start = 0 }
	head := seq
	prev := (*Value)(nil)
	cur := seq
	i := 0
	removed := 0
	for !isNil(cur) && cur.typ == VPair {
		match := false
		if i >= start && i < end && (count < 0 || removed < count) {
			v := cur.car
			if !isNil(keyFn) {
				var err2 error
				v, err2 = callFnOnSeq(keyFn, []*Value{cur.car}, globalEnv)
				if err2 != nil {
					return nil, err2
				}
			}
			res, err := callFnOnSeq(pred, []*Value{v}, globalEnv)
			if err == nil && isTruthy(res) {
				match = true
				removed++
			}
		}
		if match {
			if prev == nil {
				head = cur.cdr
			} else {
				prev.cdr = cur.cdr
			}
			cur = cur.cdr
		} else {
			prev = cur
			cur = cur.cdr
		}
		i++
	}
	return head, nil
}

// builtinDeleteIfNot - destructive version of remove-if-not
func builtinDeleteIfNot(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("delete-if-not: need predicate and sequence")
	}
	pred := args[0]
	seq := args[1]
	if seq.typ == VNil {
		return seq, nil
	}
	// For strings and vectors, delegate to remove-if-not (non-destructive)
	if seq.typ == VStr || seq.typ == VArray {
		return builtinSeqRemoveIfNot(args)
	}
	if seq.typ != VPair {
		return nil, fmt.Errorf("delete-if-not: expected a list, got %s", typeStr(seq))
	}
	negPred := &Value{typ: VFunc, params: []string{"x"}, body: list(list(vsym("not"), list(pred, vsym("x")))), env: globalEnv}
	newArgs := []*Value{negPred, seq}
	newArgs = append(newArgs, args[2:]...)
	return builtinDeleteIf(newArgs)
}

// builtinDeleteDuplicates - destructive version of remove-duplicates
func builtinDeleteDuplicates(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("delete-duplicates: need sequence")
	}
	seq := args[0]
	if seq.typ == VNil {
		return seq, nil
	}
	// For vectors and strings, delegate to remove-duplicates (non-destructive)
	if seq.typ == VArray || seq.typ == VStr {
		return builtinSeqRemoveDuplicates(args)
	}
	if seq.typ != VPair {
		return nil, fmt.Errorf("delete-duplicates: expected a list, got %s", typeStr(seq))
	}
	keyFn, _, _, _, _, start, end, _, err := seqParseKeys(args, 1)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool)
	head := seq
	prev := (*Value)(nil)
	cur := seq
	i := 0
	for !isNil(cur) && cur.typ == VPair {
		withinRange := i >= start && (end < 0 || i < end)
		if withinRange {
			key := cur.car
			if !isNil(keyFn) {
				key, _ = callFnOnSeq(keyFn, []*Value{cur.car}, globalEnv)
			}
			keyStr := ToString(key)
			if seen[keyStr] {
				// duplicate: remove by rewiring
				if prev == nil {
					head = cur.cdr
				} else {
					prev.cdr = cur.cdr
				}
				cur = cur.cdr
			} else {
				seen[keyStr] = true
				prev = cur
				cur = cur.cdr
			}
		} else {
			prev = cur
			cur = cur.cdr
		}
		i++
	}
	return head, nil
}

// builtinNsubstituteIf - destructive version of substitute-if
func builtinNsubstituteIf(args []*Value) (*Value, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("nsubstitute-if: need new, predicate, and sequence")
	}
	newVal := args[0]
	pred := args[1]
	seq := args[2]
	keyFn, _, _, fromEnd, count, start, end, _, err := seqParseKeys(args, 3)
	if err != nil {
		return nil, err
	}

	// Handle VStr: strings are immutable in Go, so create a new one
	if seq.typ == VStr {
		runes := []rune(seq.str)
		if end < 0 || end > len(runes) {
			end = len(runes)
		}
		if start < 0 {
			start = 0
		}
		replaced := 0
		if fromEnd {
			for i := end - 1; i >= start; i-- {
				if count >= 0 && replaced >= count {
					break
				}
				v := vchar(runes[i])
				if !isNil(keyFn) {
					v2, err2 := callFnOnSeq(keyFn, []*Value{v}, globalEnv)
					if err2 != nil {
						return nil, err2
					}
					v = v2
				}
				predVal, perr := callFnOnSeq(pred, []*Value{v}, globalEnv)
				if perr == nil && isTruthy(predVal) {
					if newVal.typ == VChar {
						runes[i] = newVal.ch
					} else if newVal.typ == VStr && len(newVal.str) == 1 {
						runes[i] = rune(newVal.str[0])
					}
					replaced++
				}
			}
		} else {
			for i := start; i < end; i++ {
				if count >= 0 && replaced >= count {
					break
				}
				v := vchar(runes[i])
				if !isNil(keyFn) {
					v2, err2 := callFnOnSeq(keyFn, []*Value{vchar(runes[i])}, globalEnv)
					if err2 != nil {
						return nil, err2
					}
					v = v2
				}
				predVal, perr := callFnOnSeq(pred, []*Value{v}, globalEnv)
				if perr == nil && isTruthy(predVal) {
					if newVal.typ == VChar {
						runes[i] = newVal.ch
					} else if newVal.typ == VStr && len(newVal.str) == 1 {
						runes[i] = rune(newVal.str[0])
					}
					replaced++
				}
			}
		}
		seq.str = string(runes)
		return seq, nil
	}

	// Handle VArray: modify elements in-place
	if seq.typ == VArray {
		elems := seq.array.elements
		seqLen := len(elems)
		if end < 0 || end > seqLen {
			end = seqLen
		}
		if start < 0 {
			start = 0
		}
		replaced := 0
		if fromEnd {
			for i := end - 1; i >= start; i-- {
				if count >= 0 && replaced >= count { break }
				v := elems[i]
				if !isNil(keyFn) {
					v, err2 := callFnOnSeq(keyFn, []*Value{elems[i]}, globalEnv)
					if err2 != nil {
						return nil, err2
					}
					_ = v
				}
				predVal, perr := callFnOnSeq(pred, []*Value{v}, globalEnv)
				if perr == nil && isTruthy(predVal) {
					elems[i] = newVal
					replaced++
				}
			}
		} else {
			for i := start; i < end; i++ {
				if count >= 0 && replaced >= count { break }
				v := elems[i]
				if !isNil(keyFn) {
					v2, err2 := callFnOnSeq(keyFn, []*Value{elems[i]}, globalEnv)
					if err2 != nil {
						return nil, err2
					}
					v = v2
				}
				predVal, perr := callFnOnSeq(pred, []*Value{v}, globalEnv)
				if perr == nil && isTruthy(predVal) {
					elems[i] = newVal
					replaced++
				}
			}
		}
		return seq, nil
	}

	if end < 0 {
		seen := make(map[*Value]bool)
		length := 0
		for cur := seq; !isNil(cur) && cur.typ == VPair; {
			if seen[cur] { break }
			seen[cur] = true
			length++
			cur = cur.cdr
		}
		end = length
	}
	if start < 0 { start = 0 }
	replaced := 0
	if fromEnd {
		var elements []*Value
		var cellPtrs []*Value
		for cur := seq; !isNil(cur) && cur.typ == VPair; {
			elements = append(elements, cur.car)
			cellPtrs = append(cellPtrs, cur)
			cur = cur.cdr
		}
		for i := end - 1; i >= start; i-- {
			if count >= 0 && replaced >= count { break }
			v := elements[i]
			if !isNil(keyFn) {
				var err2 error
				v, err2 = callFnOnSeq(keyFn, []*Value{elements[i]}, globalEnv)
				if err2 != nil {
					return nil, err2
				}
			}
			predVal, perr := callFnOnSeq(pred, []*Value{v}, globalEnv)
			if perr == nil && isTruthy(predVal) {
				cellPtrs[i].car = newVal
				replaced++
			}
		}
	} else {
		i := 0
		for cur := seq; !isNil(cur) && cur.typ == VPair; {
			if i >= start && i < end && (count < 0 || replaced < count) {
				v := cur.car
				if !isNil(keyFn) {
					var err2 error
					v, err2 = callFnOnSeq(keyFn, []*Value{cur.car}, globalEnv)
					if err2 != nil {
						return nil, err2
					}
				}
				predVal, perr := callFnOnSeq(pred, []*Value{v}, globalEnv)
				if perr == nil && isTruthy(predVal) {
					cur.car = newVal
					replaced++
				}
			}
			i++
			cur = cur.cdr
		}
	}
	return seq, nil
}

// builtinNsubstituteIfNot - destructive version of substitute-if-not
func builtinNsubstituteIfNot(args []*Value) (*Value, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("nsubstitute-if-not: need new, predicate, and sequence")
	}
	newVal := args[0]
	pred := args[1]
	seq := args[2]
	keyFn, _, _, fromEnd, count, start, end, _, err := seqParseKeys(args, 3)
	if err != nil {
		return nil, err
	}

	// Handle VStr: strings are immutable in Go, so create a new one
	if seq.typ == VStr {
		runes := []rune(seq.str)
		if end < 0 || end > len(runes) {
			end = len(runes)
		}
		if start < 0 {
			start = 0
		}
		replaced := 0
		if fromEnd {
			for i := end - 1; i >= start; i-- {
				if count >= 0 && replaced >= count {
					break
				}
				v := vchar(runes[i])
				if !isNil(keyFn) {
					v2, err2 := callFnOnSeq(keyFn, []*Value{v}, globalEnv)
					if err2 != nil {
						return nil, err2
					}
					v = v2
				}
				predVal, perr := callFnOnSeq(pred, []*Value{v}, globalEnv)
				if perr == nil && !isTruthy(predVal) {
					if newVal.typ == VChar {
						runes[i] = newVal.ch
					} else if newVal.typ == VStr && len(newVal.str) == 1 {
						runes[i] = rune(newVal.str[0])
					}
					replaced++
				}
			}
		} else {
			for i := start; i < end; i++ {
				if count >= 0 && replaced >= count {
					break
				}
				v := vchar(runes[i])
				if !isNil(keyFn) {
					v2, err2 := callFnOnSeq(keyFn, []*Value{vchar(runes[i])}, globalEnv)
					if err2 != nil {
						return nil, err2
					}
					v = v2
				}
				predVal, perr := callFnOnSeq(pred, []*Value{v}, globalEnv)
				if perr == nil && !isTruthy(predVal) {
					if newVal.typ == VChar {
						runes[i] = newVal.ch
					} else if newVal.typ == VStr && len(newVal.str) == 1 {
						runes[i] = rune(newVal.str[0])
					}
					replaced++
				}
			}
		}
		seq.str = string(runes)
		return seq, nil
	}

	// Handle VArray: modify elements in-place
	if seq.typ == VArray {
		elems := seq.array.elements
		seqLen := len(elems)
		if end < 0 || end > seqLen {
			end = seqLen
		}
		if start < 0 {
			start = 0
		}
		replaced := 0
		if fromEnd {
			for i := end - 1; i >= start; i-- {
				if count >= 0 && replaced >= count {
					break
				}
				v := elems[i]
				if !isNil(keyFn) {
					v2, err2 := callFnOnSeq(keyFn, []*Value{elems[i]}, globalEnv)
					if err2 != nil {
						return nil, err2
					}
					v = v2
				}
				predVal, perr := callFnOnSeq(pred, []*Value{v}, globalEnv)
				if perr == nil && !isTruthy(predVal) {
					elems[i] = newVal
					replaced++
				}
			}
		} else {
			for i := start; i < end; i++ {
				if count >= 0 && replaced >= count {
					break
				}
				v := elems[i]
				if !isNil(keyFn) {
					v2, err2 := callFnOnSeq(keyFn, []*Value{elems[i]}, globalEnv)
					if err2 != nil {
						return nil, err2
					}
					v = v2
				}
				predVal, perr := callFnOnSeq(pred, []*Value{v}, globalEnv)
				if perr == nil && !isTruthy(predVal) {
					elems[i] = newVal
					replaced++
				}
			}
		}
		return seq, nil
	}

	// Handle lists: modify cons cells in-place
	if end < 0 {
		seen := make(map[*Value]bool)
		length := 0
		for cur := seq; !isNil(cur) && cur.typ == VPair; {
			if seen[cur] {
				break
			}
			seen[cur] = true
			length++
			cur = cur.cdr
		}
		end = length
	}
	if start < 0 {
		start = 0
	}
	replaced := 0
	if fromEnd {
		var elements []*Value
		var cellPtrs []*Value
		for cur := seq; !isNil(cur) && cur.typ == VPair; {
			elements = append(elements, cur.car)
			cellPtrs = append(cellPtrs, cur)
			cur = cur.cdr
		}
		for i := end - 1; i >= start; i-- {
			if count >= 0 && replaced >= count {
				break
			}
			v := elements[i]
			if !isNil(keyFn) {
				var err2 error
				v, err2 = callFnOnSeq(keyFn, []*Value{elements[i]}, globalEnv)
				if err2 != nil {
					return nil, err2
				}
			}
			predVal, perr := callFnOnSeq(pred, []*Value{v}, globalEnv)
			if perr == nil && !isTruthy(predVal) {
				cellPtrs[i].car = newVal
				replaced++
			}
		}
	} else {
		i := 0
		for cur := seq; !isNil(cur) && cur.typ == VPair; {
			if i >= start && i < end && (count < 0 || replaced < count) {
				v := cur.car
				if !isNil(keyFn) {
					var err2 error
					v, err2 = callFnOnSeq(keyFn, []*Value{cur.car}, globalEnv)
					if err2 != nil {
						return nil, err2
					}
				}
				predVal, perr := callFnOnSeq(pred, []*Value{v}, globalEnv)
				if perr == nil && !isTruthy(predVal) {
					cur.car = newVal
					replaced++
				}
			}
			i++
			cur = cur.cdr
		}
	}
	return seq, nil
}

func builtinSeqRemoveDuplicates(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("seq-remove-duplicates: need sequence")
	}
	seq := args[0]
	keyFn, testFn, _, fromEnd, _, start, end, _, err := seqParseKeys(args, 1)
	if err != nil {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	elements := seqToList(seq)
	if end < 0 || end > len(elements) {
		end = len(elements)
	}
	if start < 0 {
		start = 0
	}
	// Track seen indices
	seen := map[int]bool{}
	dupSet := map[int]bool{}
	if fromEnd {
		for i := end - 1; i >= start; i-- {
			if seen[i] {
				continue
			}
			for j := i - 1; j >= start; j-- {
				if dupSet[j] {
					continue
				}
				if testItemMatch(elements[i], elements[j], testFn, keyFn) {
					dupSet[j] = true
				}
			}
			seen[i] = true
		}
	} else {
		for i := start; i < end; i++ {
			if seen[i] {
				continue
			}
			for j := i + 1; j < end; j++ {
				if dupSet[j] {
					continue
				}
				if testItemMatch(elements[i], elements[j], testFn, keyFn) {
					dupSet[j] = true
				}
			}
			seen[i] = true
		}
	}
	var result []*Value
	for i, el := range elements {
		if !dupSet[i] {
			result = append(result, el)
		}
	}
	// Return same type as input
	if seq.typ == VArray && seq.array != nil {
		arr := &LispArray{
			dims:     []int{len(result)},
			elements: result,
			fillPtr:  -1, // no fill-pointer for result
		}
		return &Value{typ: VArray, array: arr}, nil
	}
	if seq.typ == VStr {
		var b strings.Builder
		for _, el := range result {
			if el != nil && el.typ == VChar {
				b.WriteRune(el.ch)
			}
		}
		return vstr(b.String()), nil
	}
	return listFromSlice(result), nil
}

func builtinSeqFindIf(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("seq-find-if: need predicate and sequence")
	}
	pred := args[0]
	seq := args[1]
	keyFn, _, _, fromEnd, _, start, end, _, err := seqParseKeys(args, 2)
	if err != nil {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	elements := seqToList(seq)
	if end < 0 || end > len(elements) {
		end = len(elements)
	}
	if start < 0 {
		start = 0
	}
	if fromEnd {
		for i := end - 1; i >= start; i-- {
			v := elements[i]
			if !isNil(keyFn) {
				var err error
				v, err = callFnOnSeq(keyFn, []*Value{v}, globalEnv)
				if err != nil {
					return nil, err
				}
			}
			cmp, err := callFnOnSeq(pred, []*Value{v}, globalEnv)
			if err != nil {
				return nil, err
			}
			if isTruthy(cmp) {
				return elements[i], nil
			}
		}
	} else {
		for i := start; i < end; i++ {
			v := elements[i]
			if !isNil(keyFn) {
				var err error
				v, err = callFnOnSeq(keyFn, []*Value{v}, globalEnv)
				if err != nil {
					return nil, err
				}
			}
			cmp, err := callFnOnSeq(pred, []*Value{v}, globalEnv)
			if err != nil {
				return nil, err
			}
			if isTruthy(cmp) {
				return elements[i], nil
			}
		}
	}
	return vnil(), nil
}

func builtinSeqPositionIf(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("seq-position-if: need predicate and sequence")
	}
	pred := args[0]
	seq := args[1]
	keyFn, _, _, fromEnd, _, start, end, _, err := seqParseKeys(args, 2)
	if err != nil {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	elements := seqToList(seq)
	if end < 0 || end > len(elements) {
		end = len(elements)
	}
	if start < 0 {
		start = 0
	}
	if fromEnd {
		for i := end - 1; i >= start; i-- {
			v := elements[i]
			if !isNil(keyFn) {
				var err error
				v, err = callFnOnSeq(keyFn, []*Value{v}, globalEnv)
				if err != nil {
					return nil, err
				}
			}
			cmp, err := callFnOnSeq(pred, []*Value{v}, globalEnv)
			if err != nil {
				return nil, err
			}
			if isTruthy(cmp) {
				return vnum(float64(i)), nil
			}
		}
	} else {
		for i := start; i < end; i++ {
			v := elements[i]
			if !isNil(keyFn) {
				var err error
				v, err = callFnOnSeq(keyFn, []*Value{v}, globalEnv)
				if err != nil {
					return nil, err
				}
			}
			cmp, err := callFnOnSeq(pred, []*Value{v}, globalEnv)
			if err != nil {
				return nil, err
			}
			if isTruthy(cmp) {
				return vnum(float64(i)), nil
			}
		}
	}
	return vnil(), nil
}

func builtinSeqRemoveIfNot(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("seq-remove-if-not: need predicate and sequence")
	}
	negPred := &Value{typ: VFunc, params: []string{"x"}, body: list(list(vsym("not"), list(args[0], vsym("x")))), env: globalEnv}
	newArgs := append([]*Value{negPred}, args[1:]...)
	return builtinSeqRemoveIf(newArgs)
}

func builtinSeqCountIfNot(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("seq-count-if-not: need predicate and sequence")
	}
	negPred := &Value{typ: VFunc, params: []string{"x"}, body: list(list(vsym("not"), list(args[0], vsym("x")))), env: globalEnv}
	newArgs := append([]*Value{negPred}, args[1:]...)
	return builtinSeqCountIf(newArgs)
}

func builtinSeqFindIfNot(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("seq-find-if-not: need predicate and sequence")
	}
	negPred := &Value{typ: VFunc, params: []string{"x"}, body: list(list(vsym("not"), list(args[0], vsym("x")))), env: globalEnv}
	newArgs := append([]*Value{negPred}, args[1:]...)
	return builtinSeqFindIf(newArgs)
}

func builtinSeqPositionIfNot(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("seq-position-if-not: need predicate and sequence")
	}
	negPred := &Value{typ: VFunc, params: []string{"x"}, body: list(list(vsym("not"), list(args[0], vsym("x")))), env: globalEnv}
	newArgs := append([]*Value{negPred}, args[1:]...)
	return builtinSeqPositionIf(newArgs)
}

func builtinSeqSubstituteIf(args []*Value) (*Value, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("seq-substitute-if: need new, predicate, and sequence")
	}
	newVal := args[0]
	pred := args[1]
	seq := args[2]
	keyFn, _, _, fromEnd, count, start, end, _, err := seqParseKeys(args, 3)
	if err != nil {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	elements := seqToList(seq)
	if end < 0 || end > len(elements) {
		end = len(elements)
	}
	if start < 0 {
		start = 0
	}
	result := make([]*Value, len(elements))
	copy(result, elements)
	replaced := 0
	if fromEnd {
		for i := end - 1; i >= start; i-- {
			if count >= 0 && replaced >= count {
				break
			}
			v := elements[i]
			if !isNil(keyFn) {
				var err error
				v, err = callFnOnSeq(keyFn, []*Value{v}, globalEnv)
				if err != nil {
					return nil, err
				}
			}
			cmp, err := callFnOnSeq(pred, []*Value{v}, globalEnv)
			if err != nil {
				return nil, err
			}
			if isTruthy(cmp) {
				result[i] = newVal
				replaced++
			}
		}
	} else {
		for i := start; i < end; i++ {
			if count >= 0 && replaced >= count {
				break
			}
			v := elements[i]
			if !isNil(keyFn) {
				var err error
				v, err = callFnOnSeq(keyFn, []*Value{v}, globalEnv)
				if err != nil {
					return nil, err
				}
			}
			cmp, err := callFnOnSeq(pred, []*Value{v}, globalEnv)
			if err != nil {
				return nil, err
			}
			if isTruthy(cmp) {
				result[i] = newVal
				replaced++
			}
		}
	}
	// If input was a string and all results are characters, return a string
	if seq.typ == VStr {
		var sb strings.Builder
		for _, v := range result {
			if v.typ == VChar {
				sb.WriteRune(v.ch)
			} else {
				return listFromSlice(result), nil
			}
		}
		return vstr(sb.String()), nil
	}
	// If input was a VArray, return a VArray
	if seq.typ == VArray {
		return varray(result), nil
	}
	return listFromSlice(result), nil
}

// builtinNsubstitute is the destructive version - modifies the list in-place
func builtinNsubstitute(args []*Value) (*Value, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("nsubstitute: need new, old, and sequence")
	}
	newVal := args[0]
	oldVal := args[1]
	seq := args[2]
	keyFn, testFn, _, fromEnd, count, start, end, _, err := seqParseKeys(args, 3)
	if err != nil {
		return nil, err
	}

	// Handle VArray: modify elements in-place
	if seq.typ == VArray {
		elems := seq.array.elements
		seqLen := len(elems)
		if end < 0 || end > seqLen {
			end = seqLen
		}
		if start < 0 {
			start = 0
		}
		replaced := 0
		if fromEnd {
			for i := end - 1; i >= start; i-- {
				if count >= 0 && replaced >= count { break }
				if testItemMatch(oldVal, elems[i], testFn, keyFn) {
					elems[i] = newVal
					replaced++
				}
			}
		} else {
			for i := start; i < end; i++ {
				if count >= 0 && replaced >= count { break }
				if testItemMatch(oldVal, elems[i], testFn, keyFn) {
					elems[i] = newVal
					replaced++
				}
			}
		}
		return seq, nil
	}

	// Normalize end
	if end < 0 {
		seen := make(map[*Value]bool)
		length := 0
		for cur := seq; !isNil(cur) && cur.typ == VPair; {
			if seen[cur] { break }
			seen[cur] = true
			length++
			cur = cur.cdr
		}
		end = length
	}
	if start < 0 { start = 0 }

	// Destructive: modify .car of matching cells in-place
	replaced := 0
	if fromEnd {
		// First, build index of elements
		seen := make(map[*Value]bool)
		var elements []*Value
		var cellPtrs []*Value
		for cur := seq; !isNil(cur) && cur.typ == VPair; {
			if seen[cur] { break }
			seen[cur] = true
			elements = append(elements, cur.car)
			cellPtrs = append(cellPtrs, cur)
			cur = cur.cdr
		}
		for i := end - 1; i >= start; i-- {
			if count >= 0 && replaced >= count { break }
			if testItemMatch(oldVal, elements[i], testFn, keyFn) {
				cellPtrs[i].car = newVal
				replaced++
			}
		}
	} else {
		i := 0
		for cur := seq; !isNil(cur) && cur.typ == VPair; {
			if i >= start && i < end && (count < 0 || replaced < count) {
				if testItemMatch(oldVal, cur.car, testFn, keyFn) {
					cur.car = newVal
					replaced++
				}
			}
			i++
			cur = cur.cdr
		}
	}
	return seq, nil
}

func builtinSeqSubstituteIfNot(args []*Value) (*Value, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("seq-substitute-if-not: need new, predicate, and sequence")
	}
	negPred := &Value{typ: VFunc, params: []string{"x"}, body: list(list(vsym("not"), list(args[1], vsym("x")))), env: globalEnv}
	newArgs := []*Value{args[0], negPred, args[2]}
	newArgs = append(newArgs, args[3:]...)
	return builtinSeqSubstituteIf(newArgs)
}

func builtinSeqMerge(args []*Value) (*Value, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("seq-merge: need two sequences and a predicate")
	}
	seq1 := args[0]
	seq2 := args[1]
	pred := args[2]
	a := seqToList(seq1)
	b := seqToList(seq2)
	var result []*Value
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		cmp, err := callFnOnSeq(pred, []*Value{a[i], b[j]}, globalEnv)
		if err != nil {
			return nil, err
		}
		if isTruthy(cmp) {
			result = append(result, a[i])
			i++
		} else {
			result = append(result, b[j])
			j++
		}
	}
	for ; i < len(a); i++ {
		result = append(result, a[i])
	}
	for ; j < len(b); j++ {
		result = append(result, b[j])
	}
	return listFromSlice(result), nil
}

func builtinSeqFill(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("fill: need sequence and item")
	}
	seq := args[0]
	item := args[1]
	start := 0
	end := -1
	for i := 2; i < len(args); i++ {
		if args[i].typ == VSym {
			switch args[i].str {
			case ":START":
				if i+1 < len(args) {
					i++
					start = int(toNum(args[i]))
				}
			case ":END":
				if i+1 < len(args) {
					i++
					end = int(toNum(args[i]))
				}
			}
		}
	}
	if seq.typ == VStr {
		s := seq.str
		if end < 0 || end > len(s) {
			end = len(s)
		}
		if start < 0 {
			start = 0
		}
		c := " "
		if item.typ == VStr && len(item.str) > 0 {
			c = item.str[:1]
		} else if item.typ == VChar {
			c = string(item.ch)
		}
		result := s[:start] + strings.Repeat(c, end-start) + s[end:]
		return vstr(result), nil
	}
	// For vectors, modify in place
	if seq.typ == VArray && seq.array != nil {
		elements := seq.array.elements
		n := len(elements)
		if seq.array.fillPtr >= 0 && seq.array.fillPtr < n {
			n = seq.array.fillPtr
		}
		if end < 0 || end > n {
			end = n
		}
		if start < 0 {
			start = 0
		}
		for i := start; i < end; i++ {
			elements[i] = item
		}
		return seq, nil
	}
	elements := seqToList(seq)
	if end < 0 || end > len(elements) {
		end = len(elements)
	}
	if start < 0 {
		start = 0
	}
	result := make([]*Value, len(elements))
	copy(result, elements)
	for i := start; i < end; i++ {
		result[i] = item
	}
	return listFromSlice(result), nil
}

func builtinSeqReplace(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("replace: need destination and source sequences")
	}
	dest := args[0]
	src := args[1]
	start1 := 0
	end1 := -1
	start2 := 0
	end2 := -1
	for i := 2; i < len(args); i++ {
		if args[i].typ == VSym {
			switch args[i].str {
			case ":START1":
				if i+1 < len(args) {
					i++
					start1 = int(toNum(args[i]))
				}
			case ":END1":
				if i+1 < len(args) {
					i++
					end1 = int(toNum(args[i]))
				}
			case ":START2":
				if i+1 < len(args) {
					i++
					start2 = int(toNum(args[i]))
				}
			case ":END2":
				if i+1 < len(args) {
					i++
					end2 = int(toNum(args[i]))
				}
			}
		}
	}
	// String destination: must create new string (strings are immutable)
	if dest.typ == VStr {
		destRunes := []rune(dest.str)
		srcRunes := []rune(src.str)
		if end1 < 0 || end1 > len(destRunes) {
			end1 = len(destRunes)
		}
		if end2 < 0 || end2 > len(srcRunes) {
			end2 = len(srcRunes)
		}
		result := make([]rune, len(destRunes))
		copy(result, destRunes)
		j := start2
		for i := start1; i < end1 && j < end2; i++ {
			result[i] = srcRunes[j]
			j++
		}
		return vstr(string(result)), nil
	}
	// Array/vector destination: modify in place
	if dest.typ == VArray && dest.array != nil {
		srcList := seqToList(src)
		if end1 < 0 || end1 > len(dest.array.elements) {
			end1 = len(dest.array.elements)
		}
		if end2 < 0 || end2 > len(srcList) {
			end2 = len(srcList)
		}
		j := start2
		for i := start1; i < end1 && j < end2; i++ {
			dest.array.elements[i] = srcList[j]
			j++
		}
		return dest, nil
	}
	// List destination: modify in place
	destList := seqToList(dest)
	srcList := seqToList(src)
	if end1 < 0 || end1 > len(destList) {
		end1 = len(destList)
	}
	if end2 < 0 || end2 > len(srcList) {
		end2 = len(srcList)
	}
	// Modify the original list cons cells in place
	cur := dest
	j := start2
	for i := 0; i < end1 && cur != nil && cur.typ == VPair; i++ {
		if i >= start1 && j < end2 {
			cur.car = srcList[j]
			j++
		}
		cur = cur.cdr
	}
	return dest, nil
}

// -------- Additional CL sequence/string functions --------
func builtinSeqSearch(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("search: need two sequences")
	}
	seq1 := args[0]
	seq2 := args[1]
	var testFn, testNotFn, keyFn *Value
	testFn = nil
	testNotFn = nil
	keyFn = nil
	start1, end1, start2, end2 := 0, -1, 0, -1
	fromEnd := false
	for i := 2; i < len(args); i++ {
		if args[i].typ == VSym {
			switch args[i].str {
			case ":TEST":
				if i+1 < len(args) {
					i++
					testFn = args[i]
				}
			case ":TEST-NOT":
				if i+1 < len(args) {
					i++
					testNotFn = args[i]
				}
			case ":KEY":
				if i+1 < len(args) {
					i++
					keyFn = args[i]
				}
			case ":START1":
				if i+1 < len(args) {
					i++
					start1 = int(toNum(args[i]))
				}
			case ":END1":
				if i+1 < len(args) {
					i++
					end1 = int(toNum(args[i]))
				}
			case ":START2":
				if i+1 < len(args) {
					i++
					start2 = int(toNum(args[i]))
				}
			case ":END2":
				if i+1 < len(args) {
					i++
					end2 = int(toNum(args[i]))
				}
			case ":FROM-END":
				if i+1 < len(args) {
					i++
					fromEnd = isTruthy(args[i])
				}
			}
		}
	}
	s1 := seqToList(seq1)
	s2 := seqToList(seq2)
	if end1 < 0 || end1 > len(s1) {
		end1 = len(s1)
	}
	if end2 < 0 || end2 > len(s2) {
		end2 = len(s2)
	}
	// Helper to apply :key function
	applyKey := func(v *Value) *Value {
		if keyFn != nil {
			r, err := callFnOnSeq(keyFn, []*Value{v}, globalEnv)
			if err != nil {
				return v
			}
			return r
		}
		return v
	}
	// Helper to compare two elements
	elemsEqual := func(a, b *Value) bool {
		ka := applyKey(a)
		kb := applyKey(b)
		if testNotFn != nil {
			cmp, err := callFnOnSeq(testNotFn, []*Value{ka, kb}, globalEnv)
			if err != nil {
				return false
			}
			return !isTruthy(cmp)
		}
		if testFn != nil {
			cmp, err := callFnOnSeq(testFn, []*Value{ka, kb}, globalEnv)
			if err != nil {
				return false
			}
			return isTruthy(cmp)
		}
		return eqVal(ka, kb)
	}
	slen := end1 - start1
	if slen <= 0 {
		return vnil(), nil
	}
	if fromEnd {
		for i := end2 - slen; i >= start2; i-- {
			match := true
			for j := 0; j < slen; j++ {
				if !elemsEqual(s1[start1+j], s2[i+j]) {
					match = false
					break
				}
			}
			if match {
				return vnum(float64(i)), nil
			}
		}
		return vnil(), nil
	}
	for i := start2; i <= end2-slen; i++ {
		match := true
		for j := 0; j < slen; j++ {
			if !elemsEqual(s1[start1+j], s2[i+j]) {
				match = false
				break
			}
		}
		if match {
			return vnum(float64(i)), nil
		}
	}
	return vnil(), nil
}

func builtinSeqCopySeq(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("copy-seq: need sequence")
	}
	seq := args[0]
	if seq.typ == VStr {
		return vstr(seq.str), nil
	}
	if seq.typ == VArray && seq.array != nil {
		n := len(seq.array.elements)
		elems := make([]*Value, n)
		copy(elems, seq.array.elements)
		arr := &LispArray{
			dims:     make([]int, len(seq.array.dims)),
			elements: elems,
			fillPtr:  seq.array.fillPtr,
		}
		copy(arr.dims, seq.array.dims)
		result := gcv()
		result.typ = VArray
		result.array = arr
		return result, nil
	}
	elems := seqToList(seq)
	result := make([]*Value, len(elems))
	copy(result, elems)
	return listFromSlice(result), nil
}

func builtinSeqNReverse(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("nreverse: need sequence")
	}
	seq := args[0]
	if seq.typ == VStr {
		runes := []rune(seq.str)
		for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
			runes[i], runes[j] = runes[j], runes[i]
		}
		return vstr(string(runes)), nil
	}
	if seq.typ == VArray {
		elems := seq.array.elements
		for i, j := 0, len(elems)-1; i < j; i, j = i+1, j-1 {
			elems[i], elems[j] = elems[j], elems[i]
		}
		return seq, nil
	}
	// Walk the list collecting elements and the final tail
	elems := []*Value{}
	cur := seq
	for cur != nil && cur.typ == VPair {
		elems = append(elems, cur.car)
		cur = cur.cdr
	}
	// cur is the final tail (nil for proper list, dotted value otherwise)
	tail := cur
	// Rebuild reversed list with the same tail
	for i := 0; i < len(elems); i++ {
		tail = cons(elems[i], tail)
	}
	return tail, nil
}

func builtinStringTrim(args []*Value) (*Value, error) {
	// CL: (string-trim char-bag string)
	if len(args) < 2 {
		return nil, fmt.Errorf("string-trim: need char-bag and string")
	}
	if args[1].typ != VStr {
		return nil, fmt.Errorf("string-trim: second argument must be a string")
	}
	s := args[1].str
	cSeq := seqToList(args[0])
	var cb strings.Builder
	for _, v := range cSeq {
		if v.typ == VStr {
			cb.WriteString(v.str)
		} else if v.typ == VChar {
			cb.WriteRune(v.ch)
		}
	}
	chars := cb.String()
	if len(chars) == 0 {
		chars = " \t\n\r"
	}
	charSet := make(map[rune]bool)
	for _, r := range chars {
		charSet[r] = true
	}
	runes := []rune(s)
	start := 0
	for _, r := range runes {
		if charSet[r] {
			start++
		} else {
			break
		}
	}
	end := len(runes)
	for i := len(runes) - 1; i >= 0; i-- {
		if charSet[runes[i]] {
			end = i
		} else {
			break
		}
	}
	if start >= end {
		return vstr(""), nil
	}
	return vstr(string(runes[start:end])), nil
}

func builtinStringLeftTrim(args []*Value) (*Value, error) {
	// CL: (string-left-trim char-bag string)
	if len(args) < 2 {
		return nil, fmt.Errorf("string-left-trim: need char-bag and string")
	}
	if args[1].typ != VStr {
		return nil, fmt.Errorf("string-left-trim: second argument must be a string")
	}
	s := args[1].str
	cSeq := seqToList(args[0])
	var cb strings.Builder
	for _, v := range cSeq {
		if v.typ == VStr {
			cb.WriteString(v.str)
		} else if v.typ == VChar {
			cb.WriteRune(v.ch)
		}
	}
	chars := cb.String()
	if len(chars) == 0 {
		chars = " \t\n\r"
	}
	charSet := make(map[rune]bool)
	for _, r := range chars {
		charSet[r] = true
	}
	runes := []rune(s)
	start := 0
	for _, r := range runes {
		if charSet[r] {
			start++
		} else {
			break
		}
	}
	return vstr(string(runes[start:])), nil
}

func builtinStringRightTrim(args []*Value) (*Value, error) {
	// CL: (string-right-trim char-bag string)
	if len(args) < 2 {
		return nil, fmt.Errorf("string-right-trim: need char-bag and string")
	}
	if args[1].typ != VStr {
		return nil, fmt.Errorf("string-right-trim: second argument must be a string")
	}
	s := args[1].str
	cSeq := seqToList(args[0])
	var cb strings.Builder
	for _, v := range cSeq {
		if v.typ == VStr {
			cb.WriteString(v.str)
		} else if v.typ == VChar {
			cb.WriteRune(v.ch)
		}
	}
	chars := cb.String()
	if len(chars) == 0 {
		chars = " \t\n\r"
	}
	charSet := make(map[rune]bool)
	for _, r := range chars {
		charSet[r] = true
	}
	runes := []rune(s)
	end := len(runes)
	for i := len(runes) - 1; i >= 0; i-- {
		if charSet[runes[i]] {
			end = i
		} else {
			break
		}
	}
	return vstr(string(runes[:end])), nil
}

func builtinStrCapitalize(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("string-capitalize: expected a string designator")
	}
	s, err := coerceStringDesignator(args[0])
	if err != nil {
		return nil, fmt.Errorf("string-capitalize: %v", err)
	}
	runes := []rune(s)
	start, end := 0, len(runes)
	for i := 1; i < len(args); i++ {
		if args[i].typ == VSym {
			switch args[i].str {
			case ":START":
				if i+1 < len(args) {
					i++
					n, err := safeToNum(args[i], "string-capitalize :start")
					if err != nil {
						return nil, err
					}
					start = int(n)
				}
			case ":END":
				if i+1 < len(args) {
					i++
					n, err := safeToNum(args[i], "string-capitalize :end")
					if err != nil {
						return nil, err
					}
					end = int(n)
				}
			}
		}
	}
	if end > len(runes) {
		end = len(runes)
	}
	var result strings.Builder
	prevAlpha := false
	for i, r := range runes {
		if i >= start && i < end {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				if !prevAlpha {
					result.WriteRune(unicode.ToUpper(r))
				} else {
					result.WriteRune(unicode.ToLower(r))
				}
				prevAlpha = true
			} else {
				result.WriteRune(r)
				prevAlpha = false
			}
		} else {
			result.WriteRune(r)
		}
	}
	return vstr(result.String()), nil
}

// checkStringArg returns the string value or an error for string comparison builtins.
// Accepts string designators (string, symbol, or character).
func checkStringArg(v *Value, funcName string) (string, error) {
	v = primaryValue(v)
	s, err := coerceStringDesignator(v)
	if err != nil {
		return "", fmt.Errorf("%s: %v", funcName, err)
	}
	return s, nil
}

// strCmpKw holds parsed :start1/:end1/:start2/:end2 keyword arguments
type strCmpKw struct {
	start1 int
	end1   int
	start2 int
	end2   int
}

// runesSlice returns a slice of runes from start to end.
func runesSlice(rs []rune, start, end int) []rune {
	if start < 0 {
		start = 0
	}
	if end > len(rs) {
		end = len(rs)
	}
	if start >= end {
		return []rune{}
	}
	return rs[start:end]
}

// parseStrCmpKwArgs parses :start1/:end1/:start2/:end2 from keyword args.
// s1len and s2len are the rune lengths of the two strings.
func parseStrCmpKwArgs(args []*Value, s1len, s2len int) strCmpKw {
	p := strCmpKw{start1: 0, end1: s1len, start2: 0, end2: s2len}
	for i := 2; i < len(args); i++ {
		if args[i].typ != VSym {
			continue
		}
		switch args[i].str {
		case ":START1":
			if i+1 < len(args) {
				i++
				p.start1 = int(toNum(args[i]))
			}
		case ":END1":
			if i+1 < len(args) {
				i++
				p.end1 = int(toNum(args[i]))
			}
		case ":START2":
			if i+1 < len(args) {
				i++
				p.start2 = int(toNum(args[i]))
			}
		case ":END2":
			if i+1 < len(args) {
				i++
				p.end2 = int(toNum(args[i]))
			}
		}
	}
	if p.start1 < 0 {
		p.start1 = 0
	}
	if p.start1 > s1len {
		p.start1 = s1len
	}
	if p.end1 < 0 || p.end1 > s1len {
		p.end1 = s1len
	}
	if p.start2 < 0 {
		p.start2 = 0
	}
	if p.start2 > s2len {
		p.start2 = s2len
	}
	if p.end2 < 0 || p.end2 > s2len {
		p.end2 = s2len
	}
	return p
}

func builtinStrEqual(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("string=: need two strings")
	}
	s1, err := checkStringArg(args[0], "string=")
	if err != nil {
		return nil, err
	}
	s2, err := checkStringArg(args[1], "string=")
	if err != nil {
		return nil, err
	}
	r1 := []rune(s1)
	r2 := []rune(s2)
	p := parseStrCmpKwArgs(args, len(r1), len(r2))
	sub1 := runesSlice(r1, p.start1, p.end1)
	sub2 := runesSlice(r2, p.start2, p.end2)
	return vbool(string(sub1) == string(sub2)), nil
}

func builtinStrLess(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("string<: need two strings")
	}
	s1, err := checkStringArg(args[0], "string<")
	if err != nil {
		return nil, err
	}
	s2, err := checkStringArg(args[1], "string<")
	if err != nil {
		return nil, err
	}
	r1 := []rune(s1)
	r2 := []rune(s2)
	p := parseStrCmpKwArgs(args, len(r1), len(r2))
	runes1 := runesSlice(r1, p.start1, p.end1)
	runes2 := runesSlice(r2, p.start2, p.end2)
	for i := 0; i < len(runes1) && i < len(runes2); i++ {
		if runes1[i] < runes2[i] {
			return vnum(float64(i)), nil
		}
		if runes1[i] > runes2[i] {
			return vnil(), nil
		}
	}
	if len(runes1) < len(runes2) {
		return vnum(float64(len(runes1))), nil
	}
	return vnil(), nil
}

func builtinIdentity(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("identity: need argument")
	}
	return args[0], nil
}

func builtinComplement(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("complement: need function")
	}
	fn := args[0]
	if fn.typ != VPrim && fn.typ != VFunc {
		return nil, fmt.Errorf("complement: first argument must be a function")
	}
	// Return a VPrim that calls the original function and returns its logical NOT
	return &Value{typ: VPrim, fn: func(innerArgs []*Value) (*Value, error) {
		var result *Value
		var err error
		switch fn.typ {
		case VPrim:
			result, err = fn.fn(innerArgs)
		case VFunc:
			newEnv := NewEnv(fn.env)
			for i, p := range fn.params {
				if i < len(innerArgs) {
					newEnv.Set(p, innerArgs[i])
				} else {
					newEnv.Set(p, vnil())
				}
			}
			if fn.rest != "" {
				newEnv.Set(fn.rest, listFromSlice(innerArgs[len(fn.params):]))
			}
			body := fn.body
			if isNil(body) {
				return vbool(true), nil // not(nil) = true
			}
			for body.typ == VPair && !isNil(body.cdr) {
				_, e := Eval(body.car, newEnv)
				if e != nil {
					return nil, e
				}
				body = body.cdr
			}
			result, err = Eval(body.car, newEnv)
		}
		if err != nil {
			return nil, err
		}
		return vbool(!isTruthy(result)), nil
	}}, nil
}

func builtinConstantly(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("constantly: need at least one argument")
	}
	constVal := args[0]
	// Return a function that ignores its args and returns constVal
	return &Value{typ: VFunc, params: []string{},
		body: list(list(vsym("quote"), constVal)), env: globalEnv}, nil
}

// -------- parse-integer --------

// -------- getf --------
func builtinGetf(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("getf: need plist and indicator")
	}
	plist := args[0]
	indicator := args[1]
	defaultVal := vnil()
	if len(args) >= 3 {
		defaultVal = args[2]
	}
	cur := plist
	for cur != nil && cur.typ == VPair {
		key := cur.car
		if cur.cdr != nil && cur.cdr.typ == VPair {
			val := cur.cdr.car
			if eqVal(key, indicator) {
				return val, nil
			}
			cur = cur.cdr.cdr
		} else {
			break
		}
	}
	return defaultVal, nil
}

// -------- get-properties --------
func builtinGetProperties(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("get-properties: need plist and indicator-list")
	}
	plist := args[0]
	indicators := seqToList(args[1])
	cur := plist
	for cur != nil && cur.typ == VPair {
		key := cur.car
		if cur.cdr != nil && cur.cdr.typ == VPair {
			val := cur.cdr.car
			for _, ind := range indicators {
				if eqVal(key, ind) {
					// ANSI CL: returns (values indicator value tail)
					return multiVal(key, val, cur), nil
				}
			}
			cur = cur.cdr.cdr
		} else {
			break
		}
	}
	return multiVal(vnil(), vnil(), vnil()), nil
}

// -------- remf --------
func builtinRemf(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("remf: need plist and indicator")
	}
	plist := args[0]
	indicator := args[1]
	// Find and remove the indicator/value pair
	elems := seqToList(plist)
	result := make([]*Value, 0)
	i := 0
	for i < len(elems)-1 {
		if eqVal(elems[i], indicator) {
			i += 2 // skip key and value
			continue
		}
		result = append(result, elems[i], elems[i+1])
		i += 2
	}
	return listFromSlice(result), nil
}

// -------- make-string --------
func builtinMakeString(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("make-string: need size")
	}
	size := int(toNum(args[0]))
	initChar := ' '
	for i := 1; i < len(args); i++ {
		if args[i].typ == VSym && args[i].str == ":INITIAL-ELEMENT" {
			if i+1 < len(args) {
				i++
				c := args[i]
				if c.typ == VChar {
					initChar = c.ch
				} else if c.typ == VStr && len(c.str) > 0 {
					initChar = []rune(c.str)[0]
				}
			}
		}
	}
	if size <= 0 {
		return vstr(""), nil
	}
	runes := make([]rune, size)
	for i := range runes {
		runes[i] = initChar
	}
	return vstr(string(runes)), nil
}

// -------- make-list --------
func builtinMakeList(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("make-list: need size")
	}
	size := int(toNum(args[0]))
	initVal := vnil()
	for i := 1; i < len(args); i++ {
		if args[i].typ == VSym && args[i].str == ":INITIAL-ELEMENT" {
			if i+1 < len(args) {
				i++
				initVal = args[i]
			}
		}
	}
	result := make([]*Value, size)
	for i := range result {
		result[i] = initVal
	}
	return listFromSlice(result), nil
}

// -------- make-sequence --------
func builtinMakeSequence(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("make-sequence: need type and size")
	}
	typeVal := args[0]
	size := int(toNum(args[1]))
	initVal := vnil()
	for i := 2; i < len(args); i++ {
		if args[i].typ == VSym && strings.EqualFold(args[i].str, ":INITIAL-ELEMENT") {
			if i+1 < len(args) {
				i++
				initVal = args[i]
			}
		}
	}

	typeName := ""
	switch typeVal.typ {
	case VSym:
		typeName = typeVal.str
	case VStr:
		typeName = typeVal.str
	default:
		typeName = ToString(typeVal)
	}
	typeName = strings.ToUpper(typeName)

	switch typeName {
	case "LIST", "CONS", "NULL":
		result := make([]*Value, size)
		for i := range result {
			result[i] = initVal
		}
		return listFromSlice(result), nil
	case "VECTOR", "SIMPLE-VECTOR", "ARRAY", "SIMPLE-ARRAY":
		return makeArrayWithInit(size, initVal), nil
	case "STRING", "SIMPLE-STRING", "BASE-STRING":
		var ch rune = ' '
		if initVal.typ == VChar {
			ch = initVal.ch
		} else if initVal.typ == VStr && len(initVal.str) > 0 {
			ch = []rune(initVal.str)[0]
		} else if initVal.typ == VSym && len(initVal.str) > 0 {
			ch = []rune(initVal.str)[0]
		} else if !isNil(initVal) {
			return nil, fmt.Errorf("make-sequence: :initial-element is not a character designator: %s", ToString(initVal))
		}
		return vstr(strings.Repeat(string(ch), size)), nil
	case "BIT-VECTOR", "SIMPLE-BIT-VECTOR":
		var bitVal int
		if initVal.typ == VNum {
			bitVal = int(initVal.num)
			if bitVal != 0 && bitVal != 1 {
				return nil, fmt.Errorf("make-sequence: bit must be 0 or 1")
			}
		}
		elems := make([]*Value, size)
		for i := range elems {
			elems[i] = vnum(float64(bitVal))
		}
		arr := &LispArray{dims: []int{size}, elements: elems, adjustable: false}
		v := gcv()
		v.typ = VArray
		v.array = arr
		return v, nil
	default:
		return nil, fmt.Errorf("make-sequence: unsupported type: %s", typeName)
	}
}

func makeArrayWithInit(size int, initVal *Value) *Value {
	elems := make([]*Value, size)
	for i := range elems {
		elems[i] = initVal
	}
	arr := &LispArray{dims: []int{size}, elements: elems, adjustable: false}
	v := gcv()
	v.typ = VArray
	v.array = arr
	return v
}

// -------- random --------
func builtinRandom(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("random: need limit")
	}
	v := primaryValue(args[0])
	if v.typ != VNum && v.typ != VBigInt && v.typ != VRat {
		return nil, fmt.Errorf("random: argument must be a positive number")
	}
	// Determine which rng to use
	var randState *rand.Rand
	for i := 1; i+1 < len(args); i++ {
		if args[i].typ == VSym && args[i].str == ":RANDOM-STATE" && args[i+1].typ == VRandomState {
			if args[i+1].randState != nil {
				randState = args[i+1].randState
			}
			break
		}
	}
	// If no :random-state provided, use *random-state* special variable
	if randState == nil {
		if rv, err := globalEnv.Get("*random-state*"); err == nil && rv.typ == VRandomState && rv.randState != nil {
			randState = rv.randState
		}
	}
	// Big integer: use big.Int for proper arbitrary-precision random
	if v.typ == VBigInt {
		if v.bigInt.Sign() <= 0 {
			return nil, fmt.Errorf("random: limit must be > 0")
		}
		var rnd *rand.Rand
		if randState != nil {
			rnd = randState
		} else {
			rnd = rand.New(rand.NewSource(time.Now().UnixNano()))
		}
		result := new(big.Int).Rand(rnd, v.bigInt)
		return vbigInt(result), nil
	}
	// Rational: truncate to integer
	if v.typ == VRat {
		if v.irat <= 0 {
			return nil, fmt.Errorf("random: limit must be > 0")
		}
		intLimit := int(v.irat / v.iden)
		if intLimit < 1 {
			return nil, fmt.Errorf("random: limit must be >= 1")
		}
		if randState != nil {
			return vnum(float64(randState.Intn(intLimit))), nil
		}
		return vnum(float64(rand.Intn(intLimit))), nil
	}
	// VNum: float returns random float in [0, limit); integer returns random int in [0, limit)
	limit := v.num
	if limit <= 0 {
		return nil, fmt.Errorf("random: limit must be > 0")
	}
	if math.Abs(limit-math.Trunc(limit)) > 1e-12 {
		// Float limit: return random float
		if randState != nil {
			return vnum(randState.Float64() * limit), nil
		}
		return vnum(rand.Float64() * limit), nil
	}
	// Integer limit
	intLimit := int(limit)
	if intLimit < 1 {
		return nil, fmt.Errorf("random: limit must be >= 1")
	}
	if randState != nil {
		return vnum(float64(randState.Intn(intLimit))), nil
	}
	return vnum(float64(rand.Intn(intLimit))), nil
}

// vecToString converts a VArray of characters to a Go string.
// Uses fill-pointer if present.
func vecToString(v *Value) string {
	arr := v.array
	end := len(arr.elements)
	if arr.fillPtr >= 0 && arr.fillPtr <= len(arr.elements) {
		end = arr.fillPtr
	}
	var sb strings.Builder
	for i := 0; i < end; i++ {
		elem := arr.elements[i]
		if elem != nil {
			if elem.typ == VChar {
				sb.WriteRune(elem.ch)
			} else if elem.typ == VStr && len(elem.str) == 1 {
				sb.WriteRune(rune(elem.str[0]))
			}
		}
	}
	return sb.String()
}

func builtinNStringUpcase(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("nstring-upcase: need a string")
	}
	v := primaryValue(args[0])
	start, end := 0, -1
	for i := 1; i < len(args); i++ {
		if args[i].typ == VSym {
			switch args[i].str {
			case ":START":
				if i+1 < len(args) {
					i++
					n, err := safeToNum(args[i], "nstring-upcase :start")
					if err != nil {
						return nil, err
					}
					start = int(n)
				}
			case ":END":
				if i+1 < len(args) {
					i++
					n, err := safeToNum(args[i], "nstring-upcase :end")
					if err != nil {
						return nil, err
					}
					end = int(n)
				}
			}
		}
	}
	switch v.typ {
	case VStr:
		runes := []rune(v.str)
		e := len(runes)
		if end >= 0 && end < e {
			e = end
		}
		for i := start; i < e; i++ {
			runes[i] = unicode.ToUpper(runes[i])
		}
		v.str = string(runes)
		return v, nil
	case VArray:
		arr := v.array
		e := len(arr.elements)
		if arr.fillPtr >= 0 && arr.fillPtr < e {
			e = arr.fillPtr
		}
		if end >= 0 && end < e {
			e = end
		}
		for i := start; i < e; i++ {
			elem := arr.elements[i]
			if elem != nil && elem.typ == VChar {
				arr.elements[i] = vchar(unicode.ToUpper(elem.ch))
			}
		}
		return v, nil
	default:
		return nil, fmt.Errorf("nstring-upcase: need a string")
	}
}

func builtinNStringDowncase(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("nstring-downcase: need a string")
	}
	v := primaryValue(args[0])
	start, end := 0, -1
	for i := 1; i < len(args); i++ {
		if args[i].typ == VSym {
			switch args[i].str {
			case ":START":
				if i+1 < len(args) {
					i++
					n, err := safeToNum(args[i], "nstring-downcase :start")
					if err != nil {
						return nil, err
					}
					start = int(n)
				}
			case ":END":
				if i+1 < len(args) {
					i++
					n, err := safeToNum(args[i], "nstring-downcase :end")
					if err != nil {
						return nil, err
					}
					end = int(n)
				}
			}
		}
	}
	switch v.typ {
	case VStr:
		runes := []rune(v.str)
		e := len(runes)
		if end >= 0 && end < e {
			e = end
		}
		for i := start; i < e; i++ {
			runes[i] = unicode.ToLower(runes[i])
		}
		v.str = string(runes)
		return v, nil
	case VArray:
		arr := v.array
		e := len(arr.elements)
		if arr.fillPtr >= 0 && arr.fillPtr < e {
			e = arr.fillPtr
		}
		if end >= 0 && end < e {
			e = end
		}
		for i := start; i < e; i++ {
			elem := arr.elements[i]
			if elem != nil && elem.typ == VChar {
				arr.elements[i] = vchar(unicode.ToLower(elem.ch))
			}
		}
		return v, nil
	default:
		return nil, fmt.Errorf("nstring-downcase: need a string")
	}
}

func builtinNStringCapitalize(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("nstring-capitalize: need a string")
	}
	v := primaryValue(args[0])
	start, end := 0, -1
	for i := 1; i < len(args); i++ {
		if args[i].typ == VSym {
			switch args[i].str {
			case ":START":
				if i+1 < len(args) {
					i++
					n, err := safeToNum(args[i], "nstring-capitalize :start")
					if err != nil {
						return nil, err
					}
					start = int(n)
				}
			case ":END":
				if i+1 < len(args) {
					i++
					n, err := safeToNum(args[i], "nstring-capitalize :end")
					if err != nil {
						return nil, err
					}
					end = int(n)
				}
			}
		}
	}
	switch v.typ {
	case VStr:
		runes := []rune(v.str)
		e := len(runes)
		if end >= 0 && end < e {
			e = end
		}
		prevAlpha := false
		for i := start; i < e; i++ {
			if (runes[i] >= 'a' && runes[i] <= 'z') || (runes[i] >= 'A' && runes[i] <= 'Z') {
				if !prevAlpha {
					runes[i] = unicode.ToUpper(runes[i])
				} else {
					runes[i] = unicode.ToLower(runes[i])
				}
				prevAlpha = true
			} else {
				prevAlpha = false
			}
		}
		v.str = string(runes)
		return v, nil
	case VArray:
		arr := v.array
		e := len(arr.elements)
		if arr.fillPtr >= 0 && arr.fillPtr < e {
			e = arr.fillPtr
		}
		if end >= 0 && end < e {
			e = end
		}
		prevAlpha := false
		for i := start; i < e; i++ {
			elem := arr.elements[i]
			if elem != nil && elem.typ == VChar {
				r := elem.ch
				if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
					if !prevAlpha {
						arr.elements[i] = vchar(unicode.ToUpper(r))
					} else {
						arr.elements[i] = vchar(unicode.ToLower(r))
					}
					prevAlpha = true
				} else {
					prevAlpha = false
				}
			} else {
				prevAlpha = false
			}
		}
		return v, nil
	default:
		return nil, fmt.Errorf("nstring-capitalize: need a string")
	}
}

func titleCaseString(s string) string {
	var result []rune
	capitalize := true
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			capitalize = true
		}
		if capitalize && unicode.IsLetter(r) {
			result = append(result, unicode.ToTitle(r))
			capitalize = false
		} else {
			result = append(result, r)
		}
	}
	return string(result)
}

// -------- string-equal (case-insensitive) --------
func builtinStrEqualCI(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("string-equal: need two strings")
	}
	s1, err := checkStringArg(args[0], "string-equal")
	if err != nil {
		return nil, err
	}
	s2, err := checkStringArg(args[1], "string-equal")
	if err != nil {
		return nil, err
	}
	r1 := []rune(s1)
	r2 := []rune(s2)
	p := parseStrCmpKwArgs(args, len(r1), len(r2))
	sub1 := runesSlice(r1, p.start1, p.end1)
	sub2 := runesSlice(r2, p.start2, p.end2)
	if len(sub1) != len(sub2) {
		return vbool(false), nil
	}
	for i := range sub1 {
		if unicode.ToLower(sub1[i]) != unicode.ToLower(sub2[i]) {
			return vbool(false), nil
		}
	}
	return vbool(true), nil
}

// -------- string-not-equal, string-greaterp, string-lessp, etc. --------
func builtinStringNotEqual(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("string-not-equal: need two strings")
	}
	s1, err := checkStringArg(args[0], "string-not-equal")
	if err != nil {
		return nil, err
	}
	s2, err := checkStringArg(args[1], "string-not-equal")
	if err != nil {
		return nil, err
	}
	r1 := []rune(s1)
	r2 := []rune(s2)
	p := parseStrCmpKwArgs(args, len(r1), len(r2))
	runes1 := runesSlice(r1, p.start1, p.end1)
	runes2 := runesSlice(r2, p.start2, p.end2)
	if len(runes1) != len(runes2) {
		return vnum(float64(0)), nil
	}
	for i := 0; i < len(runes1); i++ {
		if unicode.ToLower(runes1[i]) != unicode.ToLower(runes2[i]) {
			return vnum(float64(i)), nil
		}
	}
	return vnil(), nil
}

func builtinStringGreaterp(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("string-greaterp: need two strings")
	}
	s1, err := checkStringArg(args[0], "string-greaterp")
	if err != nil {
		return nil, err
	}
	s2, err := checkStringArg(args[1], "string-greaterp")
	if err != nil {
		return nil, err
	}
	r1 := []rune(s1)
	r2 := []rune(s2)
	p := parseStrCmpKwArgs(args, len(r1), len(r2))
	runes1 := runesSlice(r1, p.start1, p.end1)
	runes2 := runesSlice(r2, p.start2, p.end2)
	for i := 0; i < len(runes1) && i < len(runes2); i++ {
		c1 := unicode.ToLower(runes1[i])
		c2 := unicode.ToLower(runes2[i])
		if c1 > c2 {
			return vnum(float64(i)), nil
		}
		if c1 < c2 {
			return vnil(), nil
		}
	}
	if len(runes1) > len(runes2) {
		return vnum(float64(len(runes2))), nil
	}
	return vnil(), nil
}

func builtinStringLessp(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("string-lessp: need two strings")
	}
	s1, err := checkStringArg(args[0], "string-lessp")
	if err != nil {
		return nil, err
	}
	s2, err := checkStringArg(args[1], "string-lessp")
	if err != nil {
		return nil, err
	}
	r1 := []rune(s1)
	r2 := []rune(s2)
	p := parseStrCmpKwArgs(args, len(r1), len(r2))
	runes1 := runesSlice(r1, p.start1, p.end1)
	runes2 := runesSlice(r2, p.start2, p.end2)
	for i := 0; i < len(runes1) && i < len(runes2); i++ {
		c1 := unicode.ToLower(runes1[i])
		c2 := unicode.ToLower(runes2[i])
		if c1 < c2 {
			return vnum(float64(i)), nil
		}
		if c1 > c2 {
			return vnil(), nil
		}
	}
	if len(runes1) < len(runes2) {
		return vnum(float64(len(runes1))), nil
	}
	return vnil(), nil
}

func builtinStringNotGreaterp(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("string-not-greaterp: need two strings")
	}
	s1, err := checkStringArg(args[0], "string-not-greaterp")
	if err != nil {
		return nil, err
	}
	s2, err := checkStringArg(args[1], "string-not-greaterp")
	if err != nil {
		return nil, err
	}
	r1 := []rune(s1)
	r2 := []rune(s2)
	p := parseStrCmpKwArgs(args, len(r1), len(r2))
	runes1 := runesSlice(r1, p.start1, p.end1)
	runes2 := runesSlice(r2, p.start2, p.end2)
	for i := 0; i < len(runes1) && i < len(runes2); i++ {
		c1 := unicode.ToLower(runes1[i])
		c2 := unicode.ToLower(runes2[i])
		if c1 > c2 {
			return vnil(), nil
		}
		if c1 < c2 {
			return vnum(float64(i)), nil
		}
	}
	if len(runes1) > len(runes2) {
		return vnil(), nil
	}
	return vnum(float64(len(runes1))), nil
}

func builtinStringNotLessp(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("string-not-lessp: need two strings")
	}
	s1, err := checkStringArg(args[0], "string-not-lessp")
	if err != nil {
		return nil, err
	}
	s2, err := checkStringArg(args[1], "string-not-lessp")
	if err != nil {
		return nil, err
	}
	r1 := []rune(s1)
	r2 := []rune(s2)
	p := parseStrCmpKwArgs(args, len(r1), len(r2))
	runes1 := runesSlice(r1, p.start1, p.end1)
	runes2 := runesSlice(r2, p.start2, p.end2)
	for i := 0; i < len(runes1) && i < len(runes2); i++ {
		c1 := unicode.ToLower(runes1[i])
		c2 := unicode.ToLower(runes2[i])
		if c1 < c2 {
			return vnil(), nil
		}
		if c1 > c2 {
			return vnum(float64(i)), nil
		}
	}
	if len(runes1) < len(runes2) {
		return vnil(), nil
	}
	return vnum(float64(len(runes2))), nil
}

// -------- write-to-string (already defined above, skip duplicate) --------

// -------- prin1-to-string --------
func builtinPrin1ToString(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("prin1-to-string: need an object")
	}
	return vstr(ToString(primaryValue(args[0]))), nil
}

// -------- princ-to-string --------
func builtinPrincToString(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("princ-to-string: need an object")
	}
	return vstr(princToString(primaryValue(args[0]))), nil
}

// -------- string-elt --------
func builtinStringElt(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("string-elt: need string and index")
	}
	s, err := checkStringArg(args[0], "string-elt")
	if err != nil {
		return nil, err
	}
	idx := int(toNum(args[1]))
	runes := []rune(s)
	if idx < 0 || idx >= len(runes) {
		return nil, fmt.Errorf("string-elt: index %d out of bounds", idx)
	}
	return vchar(runes[idx]), nil
}

// -------- nreverse (already exists as builtinSeqNReverse, add alias) --------

// -------- reverse --------
func builtinReverse(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("reverse: need sequence")
	}
	seq := args[0]
	if err := checkSequenceArg(seq, "reverse"); err != nil {
		return nil, err
	}
	if seq.typ == VStr {
		runes := []rune(seq.str)
		for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
			runes[i], runes[j] = runes[j], runes[i]
		}
		return vstr(string(runes)), nil
	}
	if seq.typ == VArray {
		elems := seq.array.elements
		result := make([]*Value, len(elems))
		copy(result, elems)
		for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
			result[i], result[j] = result[j], result[i]
		}
		arr := &LispArray{dims: seq.array.dims, elements: result}
		r := gcv()
		r.typ = VArray
		r.array = arr
		return r, nil
	}
	// Walk the list collecting elements and the final tail
	elems := []*Value{}
	cur := seq
	for cur != nil && cur.typ == VPair {
		elems = append(elems, cur.car)
		cur = cur.cdr
	}
	// cur is the final tail (nil for proper list, dotted value otherwise)
	tail := cur
	// Rebuild reversed list with the same tail
	for i := 0; i < len(elems); i++ {
		tail = cons(elems[i], tail)
	}
	return tail, nil
}

// -------- acons --------
func builtinAcons(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("acons: need key, value, and alist")
	}
	key := args[0]
	val := args[1]
	alist := vnil()
	if len(args) >= 3 {
		alist = args[2]
	}
	return cons(cons(key, val), alist), nil
}

// -------- pairlis (already exists) --------

// -------- rassoc --------
func builtinRassoc(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("rassoc: need item and alist")
	}
	item := args[0]
	alist := args[1]
	pred := vnil()
	for i := 2; i < len(args); i++ {
		if args[i].typ == VSym && args[i].str == ":TEST" && i+1 < len(args) {
			i++
			pred = args[i]
		}
	}
	cur := alist
	for cur != nil && cur.typ == VPair {
		entry := cur.car
		if entry.typ == VPair {
			if pred.typ != VNil {
				cmp, err := callFnOnSeq(pred, []*Value{entry.cdr, item}, globalEnv)
				if err == nil && isTruthy(cmp) {
					return entry, nil
				}
			} else {
				if eqVal(entry.cdr, item) {
					return entry, nil
				}
			}
		}
		cur = cur.cdr
	}
	return vnil(), nil
}

// -------- rassoc-if --------
func builtinRassocIf(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("rassoc-if: need predicate and alist")
	}
	fn := args[0]
	alist := args[1]
	cur := alist
	for cur != nil && cur.typ == VPair {
		entry := cur.car
		if entry.typ == VPair {
			result, err := callFnOnSeq(fn, []*Value{entry.cdr}, globalEnv)
			if err == nil && isTruthy(result) {
				return entry, nil
			}
		}
		cur = cur.cdr
	}
	return vnil(), nil
}

// -------- nth --------
func builtinNth(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("nth: need index and list")
	}
	n, err := safeToNum(args[0], "nth")
	if err != nil {
		return nil, err
	}
	lst := args[1]
	cur := lst
	for i := 0; i < int(n); i++ {
		if cur == nil || cur.typ != VPair {
			return vnil(), nil
		}
		cur = cur.cdr
	}
	if cur == nil || cur.typ != VPair {
		return vnil(), nil
	}
	return cur.car, nil
}

// -------- nthcdr --------
func builtinNthCdr(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("nthcdr: need index and list")
	}
	n, err := safeToNum(args[0], "nthcdr")
	if err != nil {
		return nil, err
	}
	lst := args[1]
	cur := lst
	for i := 0; i < int(n); i++ {
		if cur == nil || cur.typ != VPair {
			return vnil(), nil
		}
		cur = cur.cdr
	}
	return cur, nil
}

// -------- set-difference (already exists) --------

// -------- string= (already exists as builtinStrEqual) --------

// -------- string/= --------
func builtinStringNotEq(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("string/=: need two strings")
	}
	s1, err := checkStringArg(args[0], "string/=")
	if err != nil {
		return nil, err
	}
	s2, err := checkStringArg(args[1], "string/=")
	if err != nil {
		return nil, err
	}
	r1 := []rune(s1)
	r2 := []rune(s2)
	p := parseStrCmpKwArgs(args, len(r1), len(r2))
	runes1 := runesSlice(r1, p.start1, p.end1)
	runes2 := runesSlice(r2, p.start2, p.end2)
	if string(runes1) == string(runes2) {
		return vnil(), nil
	}
	minLen := len(runes1)
	if len(runes2) < minLen {
		minLen = len(runes2)
	}
	for i := 0; i < minLen; i++ {
		if runes1[i] != runes2[i] {
			return vnum(float64(i)), nil
		}
	}
	return vnum(float64(minLen)), nil
}

func builtinStringGreater(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("string>: need two strings")
	}
	s1, err := checkStringArg(args[0], "string>")
	if err != nil {
		return nil, err
	}
	s2, err := checkStringArg(args[1], "string>")
	if err != nil {
		return nil, err
	}
	r1 := []rune(s1)
	r2 := []rune(s2)
	p := parseStrCmpKwArgs(args, len(r1), len(r2))
	runes1 := runesSlice(r1, p.start1, p.end1)
	runes2 := runesSlice(r2, p.start2, p.end2)
	for i := 0; i < len(runes1) && i < len(runes2); i++ {
		if runes1[i] > runes2[i] {
			return vnum(float64(i)), nil
		}
		if runes1[i] < runes2[i] {
			return vnil(), nil
		}
	}
	if len(runes1) > len(runes2) {
		return vnum(float64(len(runes2))), nil
	}
	return vnil(), nil
}

func builtinStringLe(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("string<=: need two strings")
	}
	s1, err := checkStringArg(args[0], "string<=")
	if err != nil {
		return nil, err
	}
	s2, err := checkStringArg(args[1], "string<=")
	if err != nil {
		return nil, err
	}
	r1 := []rune(s1)
	r2 := []rune(s2)
	p := parseStrCmpKwArgs(args, len(r1), len(r2))
	runes1 := runesSlice(r1, p.start1, p.end1)
	runes2 := runesSlice(r2, p.start2, p.end2)
	for i := 0; i < len(runes1) && i < len(runes2); i++ {
		if runes1[i] < runes2[i] {
			return vnum(float64(i)), nil
		}
		if runes1[i] > runes2[i] {
			return vnil(), nil
		}
	}
	if len(runes1) <= len(runes2) {
		return vnum(float64(len(runes1))), nil
	}
	return vnil(), nil
}

func builtinStringGe(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("string>=: need two strings")
	}
	s1, err := checkStringArg(args[0], "string>=")
	if err != nil {
		return nil, err
	}
	s2, err := checkStringArg(args[1], "string>=")
	if err != nil {
		return nil, err
	}
	r1 := []rune(s1)
	r2 := []rune(s2)
	p := parseStrCmpKwArgs(args, len(r1), len(r2))
	runes1 := runesSlice(r1, p.start1, p.end1)
	runes2 := runesSlice(r2, p.start2, p.end2)
	for i := 0; i < len(runes1) && i < len(runes2); i++ {
		if runes1[i] > runes2[i] {
			return vnum(float64(i)), nil
		}
		if runes1[i] < runes2[i] {
			return vnil(), nil
		}
	}
	if len(runes1) >= len(runes2) {
		return vnum(float64(len(runes2))), nil
	}
	return vnil(), nil
}

// -------- abs --------
func builtinAbs(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("abs: need a number")
	}
	v := args[0]
	switch v.typ {
	case VNum:
		return vnum(math.Abs(v.num)), nil
	case VRat:
		n, d := v.irat, v.iden
		if n < 0 {
			n = -n
		}
		return vrat(n, d), nil
	case VBigInt:
		result := new(big.Int).Abs(v.bigInt)
		return vbigint(result), nil
	case VComplex:
		return vnum(math.Hypot(v.num, v.imag)), nil
	}
	return nil, fmt.Errorf("abs: not a number")
}

// isReal returns true if v is a real numeric type (not complex)
func isReal(v *Value) bool {
	return v.typ == VNum || v.typ == VRat || v.typ == VBigInt
}

func isNumber(v *Value) bool {
	return v.typ == VNum || v.typ == VRat || v.typ == VBigInt || v.typ == VComplex
}

// -------- max / min --------
func builtinMax(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("max: need at least one number")
	}
	for _, a := range args {
		if !isReal(a) {
			return nil, fmt.Errorf("max: not a real number: %v", a)
		}
	}
	result := args[0]
	for i := 1; i < len(args); i++ {
		if compareNumeric(result, args[i]) < 0 {
			result = args[i]
		}
	}
	// If result is VRat, return as-is; if VBigInt or VNum, return as-is
	return result, nil
}

func builtinMin(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("min: need at least one number")
	}
	for _, a := range args {
		if !isReal(a) {
			return nil, fmt.Errorf("min: not a real number: %v", a)
		}
	}
	result := args[0]
	for i := 1; i < len(args); i++ {
		if compareNumeric(result, args[i]) > 0 {
			result = args[i]
		}
	}
	return result, nil
}

// -------- mod / rem --------
func builtinMod(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("mod: need two numbers")
	}
	if isBigIntInt(args) {
		n := toBigInt(args[0])
		d := toBigInt(args[1])
		if n == nil || d == nil {
			// Fall back to float
			nf := toNum(args[0])
			df := toNum(args[1])
			if df == 0 {
				return nil, fmt.Errorf("mod: division by zero")
			}
			r := math.Mod(nf, df)
			if r != 0 && (r > 0) != (df > 0) {
				r += df
			}
			return numOrFloat(r, args), nil
		}
		if d.Sign() == 0 {
			return nil, fmt.Errorf("mod: division by zero")
		}
		r := new(big.Int).Mod(n, d)
		return vbigint(r), nil
	}
	n := toNum(args[0])
	d := toNum(args[1])
	if d == 0 {
		return nil, fmt.Errorf("mod: division by zero")
	}
	r := math.Mod(n, d)
	// CL mod: result has same sign as divisor
	if r != 0 && (r > 0) != (d > 0) {
		r += d
	}
	return numOrFloat(r, args), nil
}

func builtinRem(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("rem: need two numbers")
	}
	if isBigIntInt(args) {
		n := toBigInt(args[0])
		d := toBigInt(args[1])
		if n == nil || d == nil {
			nf := toNum(args[0])
			df := toNum(args[1])
			if df == 0 {
				return nil, fmt.Errorf("rem: division by zero")
			}
			return vnum(math.Mod(nf, df)), nil
		}
		if d.Sign() == 0 {
			return nil, fmt.Errorf("rem: division by zero")
		}
		r := new(big.Int).Rem(n, d)
		return vbigint(r), nil
	}
	n := toNum(args[0])
	d := toNum(args[1])
	if d == 0 {
		return nil, fmt.Errorf("rem: division by zero")
	}
	return vnum(math.Mod(n, d)), nil
}

// -------- floor / ceiling / truncate / round --------
func builtinFloor(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("floor: need a number")
	}
	n := toNum(args[0])
	f := math.Floor(n)
	if len(args) >= 2 {
		div := toNum(args[1])
		if div == 0 {
			return nil, fmt.Errorf("floor: division by zero")
		}
		q := math.Floor(n / div)
		r := n - q*div
		if r != 0 && (r > 0) != (div > 0) {
			r += div
		}
		return multiVal(vnum(q), vnum(r)), nil
	}
	return multiVal(vnum(f), vnum(n-f)), nil
}

func builtinCeiling(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("ceiling: need a number")
	}
	n := toNum(args[0])
	c := math.Ceil(n)
	if len(args) >= 2 {
		div := toNum(args[1])
		if div == 0 {
			return nil, fmt.Errorf("ceiling: division by zero")
		}
		q := math.Ceil(n / div)
		r := n - q*div
		return multiVal(vnum(q), vnum(r)), nil
	}
	return multiVal(vnum(c), vnum(n-c)), nil
}

func builtinTruncate(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("truncate: need a number")
	}
	n := toNum(args[0])
	t := math.Trunc(n)
	if len(args) >= 2 {
		div := toNum(args[1])
		if div == 0 {
			return nil, fmt.Errorf("truncate: division by zero")
		}
		q := math.Trunc(n / div)
		r := n - q*div
		return multiVal(vnum(q), vnum(r)), nil
	}
	return multiVal(vnum(t), vnum(n-t)), nil
}

func builtinRound(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("round: need a number")
	}
	n := toNum(args[0])
	r := math.Round(n)
	// Round half to even
	if diff := math.Abs(n - r); diff == 0.5 {
		if math.Mod(r, 2) != 0 {
			if n > 0 {
				r--
			} else {
				r++
			}
		}
	}
	if len(args) >= 2 {
		div := toNum(args[1])
		if div == 0 {
			return nil, fmt.Errorf("round: division by zero")
		}
		q := math.Round(n / div)
		// Round half to even
		if diff := math.Abs(n/div - q); diff == 0.5 {
			if math.Mod(q, 2) != 0 {
				if n/div > 0 {
					q--
				} else {
					q++
				}
			}
		}
		rem := n - q*div
		return multiVal(vnum(q), vnum(rem)), nil
	}
	return multiVal(vnum(r), vnum(n-r)), nil
}

// -------- ffloor, fceiling, ftruncate, fround --------
// Same as floor/ceiling/truncate/round but the first return value is a float.

func builtinFfloor(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("ffloor: need a number")
	}
	n := toNum(args[0])
	f := math.Floor(n)
	if len(args) >= 2 {
		div := toNum(args[1])
		if div == 0 {
			return nil, fmt.Errorf("ffloor: division by zero")
		}
		q := math.Floor(n / div)
		r := n - q*div
		if r != 0 && (r > 0) != (div > 0) {
			r += div
		}
		return multiVal(vfloat(q), vnum(r)), nil
	}
	return multiVal(vfloat(f), vnum(n-f)), nil
}

func builtinFceiling(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("fceiling: need a number")
	}
	n := toNum(args[0])
	c := math.Ceil(n)
	if len(args) >= 2 {
		div := toNum(args[1])
		if div == 0 {
			return nil, fmt.Errorf("fceiling: division by zero")
		}
		q := math.Ceil(n / div)
		r := n - q*div
		return multiVal(vfloat(q), vnum(r)), nil
	}
	return multiVal(vfloat(c), vnum(n-c)), nil
}

func builtinFtruncate(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("ftruncate: need a number")
	}
	n := toNum(args[0])
	t := math.Trunc(n)
	if len(args) >= 2 {
		div := toNum(args[1])
		if div == 0 {
			return nil, fmt.Errorf("ftruncate: division by zero")
		}
		q := math.Trunc(n / div)
		r := n - q*div
		return multiVal(vfloat(q), vnum(r)), nil
	}
	return multiVal(vfloat(t), vnum(n-t)), nil
}

func builtinFround(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("fround: need a number")
	}
	n := toNum(args[0])
	r := math.Round(n)
	// Round half to even
	if diff := math.Abs(n - r); diff == 0.5 {
		if math.Mod(r, 2) != 0 {
			if n > 0 {
				r--
			} else {
				r++
			}
		}
	}
	if len(args) >= 2 {
		div := toNum(args[1])
		if div == 0 {
			return nil, fmt.Errorf("fround: division by zero")
		}
		q := math.Round(n / div)
		// Round half to even
		if diff := math.Abs(n/div - q); diff == 0.5 {
			if math.Mod(q, 2) != 0 {
				if n/div > 0 {
					q--
				} else {
					q++
				}
			}
		}
		rem := n - q*div
		return multiVal(vfloat(q), vnum(rem)), nil
	}
	return multiVal(vfloat(r), vnum(n-r)), nil
}

// -------- signum --------
func builtinSignum(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("signum: need a number")
	}
	v := args[0]
	if v.typ == VBigInt {
		sign := v.bigInt.Sign()
		if sign > 0 {
			return vnum(1), nil
		}
		if sign < 0 {
			return vnum(-1), nil
		}
		return vnum(0), nil
	}
	n := toNum(args[0])
	if n > 0 {
		return vnum(1), nil
	}
	if n < 0 {
		return vnum(-1), nil
	}
	return vnum(0), nil
}

// -------- gcd --------
func builtinGCD(args []*Value) (*Value, error) {
	if len(args) == 0 {
		return vnum(0), nil
	}
	if isBigIntInt(args) {
		result := new(big.Int)
		bi := toBigInt(args[0])
		if bi != nil {
			result.Abs(bi)
		} else {
			result.SetInt64(int64(math.Abs(toNum(args[0]))))
		}
		for i := 1; i < len(args); i++ {
			bi := toBigInt(args[i])
			n := new(big.Int)
			if bi != nil {
				n.Abs(bi)
			} else {
				n.SetInt64(int64(math.Abs(toNum(args[i]))))
			}
			result.GCD(nil, nil, result, n)
		}
		return vbigint(result), nil
	}
	result := int64(math.Abs(toNum(args[0])))
	for i := 1; i < len(args); i++ {
		n := int64(math.Abs(toNum(args[i])))
		for n != 0 {
			result, n = n, result%n
		}
	}
	return vnum(float64(result)), nil
}

// -------- lcm --------
func builtinLCM(args []*Value) (*Value, error) {
	if len(args) == 0 {
		return vnum(1), nil
	}
	if isBigIntInt(args) {
		result := new(big.Int)
		bi := toBigInt(args[0])
		if bi != nil {
			result.Abs(bi)
		} else {
			result.SetInt64(int64(math.Abs(toNum(args[0]))))
		}
		if result.Sign() == 0 {
			return vnum(0), nil
		}
		for i := 1; i < len(args); i++ {
			bi := toBigInt(args[i])
			n := new(big.Int)
			if bi != nil {
				n.Abs(bi)
			} else {
				n.SetInt64(int64(math.Abs(toNum(args[i]))))
			}
			if n.Sign() == 0 {
				return vnum(0), nil
			}
			g := new(big.Int).GCD(nil, nil, result, n)
			result.Mul(result, n)
			result.Quo(result, g)
			result.Abs(result)
		}
		return vbigint(result), nil
	}
	gcd := func(a, b int64) int64 {
		for b != 0 {
			a, b = b, a%b
		}
		return a
	}
	result := int64(math.Abs(toNum(args[0])))
	if result == 0 {
		return vnum(0), nil
	}
	for i := 1; i < len(args); i++ {
		n := int64(math.Abs(toNum(args[i])))
		if n == 0 {
			return vnum(0), nil
		}
		result = result / gcd(result, n) * n
	}
	return vnum(float64(result)), nil
}

// -------- log --------
func builtinLog(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("log: need a number")
	}
	n := toNum(args[0])
	if n <= 0 {
		return nil, fmt.Errorf("log: argument must be positive")
	}
	if len(args) >= 2 {
		base := toNum(args[1])
		if base <= 0 || base == 1 {
			return nil, fmt.Errorf("log: invalid base")
		}
		return vnum(math.Log(n) / math.Log(base)), nil
	}
	return vnum(math.Log(n)), nil
}

// -------- sqrt --------
func builtinSqrt(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("sqrt: need a number")
	}
	n := toNum(args[0])
	if n < 0 {
		// Return complex
		return vcomplex(0, math.Sqrt(-n)), nil
	}
	return vnum(math.Sqrt(n)), nil
}

func toRational(f float64) *Value {
	const maxDen = 1000000
	num := int64(math.Round(f * float64(maxDen)))
	den := int64(maxDen)
	g := gcd(num, den)
	num /= g
	den /= g
	if den == 1 {
		return vnum(float64(num))
	}
	return vrat(num, den)
}

func builtinExpt(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("expt: need base and exponent")
	}
	base, exp := args[0], args[1]
	// Check if exponent is an integer
	expIsInt := (exp.typ == VNum && exp.num == math.Trunc(exp.num)) || exp.typ == VBigInt
	if !expIsInt {
		return vnum(math.Pow(toNum(base), toNum(exp))), nil
	}
	// Get exponent as int64
	var e int64
	if exp.typ == VBigInt {
		if !exp.bigInt.IsInt64() {
			return nil, fmt.Errorf("expt: exponent too large")
		}
		e = exp.bigInt.Int64()
	} else {
		e = int64(exp.num)
	}
	// Handle complex base with integer exponent
	if base.typ == VComplex {
		if e == 0 {
			return vcomplexAlways(1, 0), nil
		}
		negExp := e < 0
		if negExp {
			e = -e
		}
		// Binary exponentiation for complex numbers
		rReal := 1.0
		rImag := 0.0
		bReal := base.num
		bImag := base.imag
		for e > 0 {
			if e&1 == 1 {
				newR := rReal*bReal - rImag*bImag
				newI := rReal*bImag + rImag*bReal
				rReal = newR
				rImag = newI
			}
			newB := bReal*bReal - bImag*bImag
			bImag = 2 * bReal * bImag
			bReal = newB
			e >>= 1
		}
		result := vcomplexAlways(rReal, rImag)
		if negExp {
			// 1 / result
			mag := rReal*rReal + rImag*rImag
			if mag == 0 {
				return nil, fmt.Errorf("expt: division by zero")
			}
			result = vcomplexAlways(rReal/mag, -rImag/mag)
		}
		return result, nil
	}
	if e < 0 {
		// 1 / base^|e| — return float
		absE := new(big.Int).Abs(big.NewInt(e))
		var result *big.Int
		bi := toBigInt(base)
		if bi != nil {
			result = new(big.Int).Exp(bi, absE, nil)
		} else {
			result = big.NewInt(int64(toNum(base)))
			result.Exp(result, absE, nil)
		}
		f, _ := new(big.Float).SetInt(result).Float64()
		return vnum(1.0 / f), nil
	}
	if e == 0 {
		return vnum(1), nil
	}
	// Try big.Int exponentiation for integer bases
	bi := toBigInt(base)
	if bi != nil {
		result := new(big.Int).Exp(bi, big.NewInt(e), nil)
		return vbigint(result), nil
	}
	// Non-integer base, integer exponent — use float
	baseF := toNum(base)
	if baseF == 0 {
		return vnum(0), nil
	}
	if e == 1 {
		return vnum(baseF), nil
	}
	if e == -1 {
		return vnum(1 / baseF), nil
	}
	return vnum(math.Pow(baseF, float64(e))), nil
}

// -------- Trig functions --------
func builtinSin(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("sin: need a number")
	}
	n := toNum(args[0])
	return vnum(float64(math.Sin(n))), nil
}
func builtinCos(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("cos: need a number")
	}
	n := toNum(args[0])
	return vnum(float64(math.Cos(n))), nil
}
func builtinTan(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("tan: need a number")
	}
	n := toNum(args[0])
	return vnum(float64(math.Tan(n))), nil
}
func builtinAtan(args []*Value) (*Value, error) {
	if len(args) == 1 {
		n := toNum(args[0])
		return vnum(float64(math.Atan(n))), nil
	}
	// atan2: (atan y x)
	y := toNum(args[0])
	x := toNum(args[1])
	return vnum(float64(math.Atan2(y, x))), nil
}
func builtinAtan2(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("atan2: need y and x")
	}
	y := toNum(args[0])
	x := toNum(args[1])
	return vnum(float64(math.Atan2(y, x))), nil
}
func builtinExp(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("exp: need a number")
	}
	n := toNum(args[0])
	return vnum(float64(math.Exp(n))), nil
}
func builtinSinh(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("sinh: need a number")
	}
	n := toNum(args[0])
	return vnum(float64(math.Sinh(n))), nil
}
func builtinCosh(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("cosh: need a number")
	}
	n := toNum(args[0])
	return vnum(float64(math.Cosh(n))), nil
}
func builtinTanh(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("tanh: need a number")
	}
	n := toNum(args[0])
	return vnum(float64(math.Tanh(n))), nil
}

// -------- asinh / acosh / atanh (ANSI CL inverse hyperbolic) --------
func builtinAsinh(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("asinh: need a number")
	}
	n := toNum(args[0])
	// asinh(x) = log(x + sqrt(x*x + 1))
	return vnum(math.Log(n + math.Sqrt(n*n+1))), nil
}

func builtinAcosh(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("acosh: need a number")
	}
	n := toNum(args[0])
	if n < 1 {
		// acosh undefined for x < 1, return complex result
		// acosh(x) = log(x + sqrt(x-1)*sqrt(x+1)) but for x<1 we compute:
		// acosh(x) = i*acos(x)  => complex result
		ac := math.Acos(n)
		return vcomplex(0, ac), nil
	}
	// acosh(x) = log(x + sqrt(x-1)*sqrt(x+1))
	return vnum(math.Log(n + math.Sqrt(n-1)*math.Sqrt(n+1))), nil
}

func builtinAtanh(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("atanh: need a number")
	}
	n := toNum(args[0])
	if n <= -1 || n >= 1 {
		// atanh undefined for |x| >= 1, return complex result
		// atanh(x) = 0.5*log((1+x)/(1-x)) => complex when |x|>=1
		// For x=1: pi/2, for x=-1: -pi/2, both imaginary part infinity
		// We compute the principal value using log of complex numbers
		// atanh(x) = 0.5 * (log(1+x) - log(1-x))
		// When x>1: log(1+x) real, log(1-x) = log(-(x-1)) = log(x-1) + i*pi
		// => atanh(x) = 0.5*(log(1+x) - log(x-1)) - i*pi/2
		if n == 1 || n == -1 {
			return nil, fmt.Errorf("atanh: argument %v is out of domain", args[0])
		}
		if n > 1 {
			return vcomplex(0.5*(math.Log(1+n)-math.Log(n-1)), -math.Pi/2), nil
		}
		// n < -1
		return vcomplex(0.5*(math.Log(1-n)-math.Log(-1-n)), math.Pi/2), nil
	}
	// atanh(x) = 0.5*log((1+x)/(1-x)) for |x| < 1
	return vnum(0.5 * math.Log((1+n)/(1-n))), nil
}

// -------- evenp / oddp --------
func builtinEvenp(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("evenp: need a number")
	}
	v := args[0]
	if v.typ == VBigInt {
		return vbool(new(big.Int).Mod(v.bigInt, big.NewInt(2)).Sign() == 0), nil
	}
	return vbool(int64(toNum(args[0]))%2 == 0), nil
}

func builtinOddp(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("oddp: need a number")
	}
	v := args[0]
	if v.typ == VBigInt {
		return vbool(new(big.Int).Mod(v.bigInt, big.NewInt(2)).Sign() != 0), nil
	}
	return vbool(int64(toNum(args[0]))%2 != 0), nil
}

// -------- plusp / minusp / zerop --------
func builtinPlusp(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("plusp: need a number")
	}
	v := args[0]
	if v.typ == VBigInt {
		return vbool(v.bigInt.Sign() > 0), nil
	}
	return vbool(toNum(args[0]) > 0), nil
}

func builtinMinusp(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("minusp: need a number")
	}
	v := args[0]
	if v.typ == VBigInt {
		return vbool(v.bigInt.Sign() < 0), nil
	}
	return vbool(toNum(args[0]) < 0), nil
}

func builtinZerop(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("zerop: need a number")
	}
	v := args[0]
	if v.typ == VBigInt {
		return vbool(v.bigInt.Sign() == 0), nil
	}
	return vbool(toNum(args[0]) == 0), nil
}

// -------- 1+ / 1- --------
func builtinOnePlus(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("1+: need a number")
	}
	v := args[0]
	if v.typ == VBigInt {
		return vbigint(new(big.Int).Add(v.bigInt, big.NewInt(1))), nil
	}
	return vnum(toNum(args[0]) + 1), nil
}

func builtinOneMinus(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("1-: need a number")
	}
	v := args[0]
	if v.typ == VBigInt {
		return vbigint(new(big.Int).Sub(v.bigInt, big.NewInt(1))), nil
	}
	return vnum(toNum(args[0]) - 1), nil
}

// -------- incf / decf (implemented as special forms in eval) --------

// -------- digit-char --------
func builtinDigitChar(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("digit-char: need weight")
	}
	weight := int(toNum(args[0]))
	radix := 10
	if len(args) >= 2 {
		radix = int(toNum(args[1]))
	}
	if weight < 0 || weight >= radix {
		return vnil(), nil
	}
	if weight < 10 {
		return vchar(rune('0' + weight)), nil
	}
	return vchar(rune('A' + weight - 10)), nil
}

// -------- digit-char-p --------

// -------- alphanumericp --------
func builtinAlphanumericp(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("alphanumericp: need a character")
	}
	var ch rune
	if args[0].typ == VChar {
		ch = args[0].ch
	} else if args[0].typ == VStr && len(args[0].str) > 0 {
		ch = []rune(args[0].str)[0]
	} else {
		return vbool(false), nil
	}
	return vbool(unicode.IsLetter(ch) || unicode.IsDigit(ch)), nil
}

// -------- alpha-char-p --------
func builtinAlphaCharP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("alpha-char-p: need a character")
	}
	var ch rune
	if args[0].typ == VChar {
		ch = args[0].ch
	} else if args[0].typ == VStr && len(args[0].str) > 0 {
		ch = []rune(args[0].str)[0]
	} else {
		return vbool(false), nil
	}
	return vbool(unicode.IsLetter(ch)), nil
}

// -------- graphic-char-p --------
func builtinGraphicCharP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("graphic-char-p: need a character")
	}
	var ch rune
	if args[0].typ == VChar {
		ch = args[0].ch
	} else if args[0].typ == VStr && len(args[0].str) > 0 {
		ch = []rune(args[0].str)[0]
	} else {
		return vbool(false), nil
	}
	// Graphic chars are printable characters EXCLUDING whitespace (space, tab, newline, etc.)
	// Per ANSI CL: space, newline, tab, page, return, backspace are NOT graphic characters
	if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' || ch == '\f' || ch == '\b' {
		return vbool(false), nil
	}
	return vbool(unicode.IsPrint(ch) && !unicode.IsSpace(ch)), nil
}

// -------- standard-char-p --------
// Standard chars are: space, newline, tab, page, return, backspace,
// and all 95 printable ASCII characters (codes 32-126)
// isStandardChar checks if a character is a standard-char
func isStandardChar(ch rune) bool {
	return (ch >= 32 && ch <= 126) || ch == 10 || ch == 9 || ch == 12 || ch == 13 || ch == 8
}

func builtinStandardCharP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("standard-char-p: need a character")
	}
	var ch rune
	if args[0].typ == VChar {
		ch = args[0].ch
	} else if args[0].typ == VStr && len(args[0].str) > 0 {
		ch = []rune(args[0].str)[0]
	} else {
		return vbool(false), nil
	}
	// Standard chars are space (32), newline (10), tab (9), page (12), return (13),
	// backspace (8), and all 95 printable ASCII chars (32-126)
	return vbool(isStandardChar(ch)), nil
}

// -------- upper-case-p / lower-case-p / both-case-p --------
func builtinUpperCaseP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("upper-case-p: need a character")
	}
	var ch rune
	if args[0].typ == VChar {
		ch = args[0].ch
	} else if args[0].typ == VStr && len(args[0].str) > 0 {
		ch = []rune(args[0].str)[0]
	} else {
		return vbool(false), nil
	}
	return vbool(unicode.IsUpper(ch) && unicode.IsLetter(ch)), nil
}

func builtinLowerCaseP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("lower-case-p: need a character")
	}
	var ch rune
	if args[0].typ == VChar {
		ch = args[0].ch
	} else if args[0].typ == VStr && len(args[0].str) > 0 {
		ch = []rune(args[0].str)[0]
	} else {
		return vbool(false), nil
	}
	return vbool(unicode.IsLower(ch) && unicode.IsLetter(ch)), nil
}

func builtinBothCaseP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("both-case-p: need a character")
	}
	var ch rune
	if args[0].typ == VChar {
		ch = args[0].ch
	} else if args[0].typ == VStr && len(args[0].str) > 0 {
		ch = []rune(args[0].str)[0]
	} else {
		return vbool(false), nil
	}
	return vbool(unicode.IsLetter(ch)), nil
}

// -------- char-upcase / char-downcase --------

// -------- char-equal (case-insensitive) / char-not-equal --------
func builtinCharEqual(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("char-equal: need at least two characters")
	}
	runes := make([]rune, len(args))
	for i, a := range args {
		if a.typ != VChar {
			return nil, fmt.Errorf("char-equal: expected a character")
		}
		runes[i] = unicode.ToLower(a.ch)
	}
	for i := 1; i < len(runes); i++ {
		if runes[i] != runes[0] {
			return vbool(false), nil
		}
	}
	return vbool(true), nil
}

func builtinCharNotEqual(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("char-not-equal: need at least two characters")
	}
	runes := make([]rune, len(args))
	for i, a := range args {
		if a.typ != VChar {
			return nil, fmt.Errorf("char-not-equal: expected a character")
		}
		runes[i] = unicode.ToLower(a.ch)
	}
	// All pairs must be distinct (not just adjacent)
	for i := 0; i < len(runes); i++ {
		for j := i + 1; j < len(runes); j++ {
			if runes[i] == runes[j] {
				return vbool(false), nil
			}
		}
	}
	return vbool(true), nil
}

func charVal(v *Value) rune {
	if v.typ == VChar {
		return v.ch
	}
	if v.typ == VStr && len(v.str) > 0 {
		return []rune(v.str)[0]
	}
	return 0
}

// -------- char-lessp / char-greaterp / char-not-lessp / char-not-greaterp (case-insensitive, multi-arg) --------
func charCompareCI(op string, args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("char%s: expected at least 2 characters", op)
	}
	runes := make([]rune, len(args))
	for i, a := range args {
		if a.typ != VChar {
			return nil, fmt.Errorf("char%s: expected a character", op)
		}
		runes[i] = unicode.ToLower(a.ch)
	}
	for i := 0; i < len(runes)-1; i++ {
		a, b := runes[i], runes[i+1]
		switch op {
		case "lessp":
			if !(a < b) {
				return vbool(false), nil
			}
		case "greaterp":
			if !(a > b) {
				return vbool(false), nil
			}
		case "not-lessp":
			if !(a >= b) {
				return vbool(false), nil
			}
		case "not-greaterp":
			if !(a <= b) {
				return vbool(false), nil
			}
		}
	}
	return vbool(true), nil
}

func builtinCharLessp(args []*Value) (*Value, error) { return charCompareCI("lessp", args) }
func builtinCharGreaterp(args []*Value) (*Value, error) { return charCompareCI("greaterp", args) }
func builtinCharNotLessp(args []*Value) (*Value, error) { return charCompareCI("not-lessp", args) }
func builtinCharNotGreaterp(args []*Value) (*Value, error) { return charCompareCI("not-greaterp", args) }

// builtinCharNotEq - ANSI CL char/=: all pairs must be distinct (case-sensitive)
func builtinCharNotEq(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("char/=: need at least two characters")
	}
	runes := make([]rune, len(args))
	for i, a := range args {
		if a.typ != VChar {
			return nil, fmt.Errorf("char/=: expected a character")
		}
		runes[i] = a.ch
	}
	// All pairs must be distinct (not just adjacent)
	for i := 0; i < len(runes); i++ {
		for j := i + 1; j < len(runes); j++ {
			if runes[i] == runes[j] {
				return vbool(false), nil
			}
		}
	}
	return vbool(true), nil
}

// -------- revappend (already exists) --------

// -------- nreconc --------
func builtinNreconc(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("nreconc: need two lists")
	}
	list1 := seqToList(args[0])
	list2 := seqToList(args[1])
	// Reverse list1 and prepend to list2
	for i, j := 0, len(list1)-1; i < j; i, j = i+1, j-1 {
		list1[i], list1[j] = list1[j], list1[i]
	}
	result := make([]*Value, len(list1)+len(list2))
	copy(result, list1)
	copy(result[len(list1):], list2)
	return listFromSlice(result), nil
}

// -------- concatenate (already exists) --------


// -------- mapl / maplist / mapc / mapcon --------
func builtinMaplist(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("maplist: need function and list")
	}
	fn := args[0]
	lst := args[1]
	var results []*Value
	seen := make(map[*Value]bool)
	cur := lst
	for cur != nil && cur.typ == VPair {
		if seen[cur] {
			break
		}
		seen[cur] = true
		r, err := callFnOnSeq(fn, []*Value{cur}, globalEnv)
		if err != nil {
			return nil, err
		}
		results = append(results, r)
		cur = cur.cdr
	}
	return listFromSlice(results), nil
}

func builtinMapc(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("mapc: need function and list")
	}
	fn := args[0]
	lst := args[1]
	seen := make(map[*Value]bool)
	cur := lst
	for cur != nil && cur.typ == VPair {
		if seen[cur] {
			break
		}
		seen[cur] = true
		_, err := callFnOnSeq(fn, []*Value{cur.car}, globalEnv)
		if err != nil {
			return nil, err
		}
		cur = cur.cdr
	}
	return lst, nil
}

func builtinMapl(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("mapl: need function and list")
	}
	fn := args[0]
	lst := args[1]
	seen := make(map[*Value]bool)
	cur := lst
	for cur != nil && cur.typ == VPair {
		if seen[cur] {
			break
		}
		seen[cur] = true
		_, err := callFnOnSeq(fn, []*Value{cur}, globalEnv)
		if err != nil {
			return nil, err
		}
		cur = cur.cdr
	}
	return lst, nil
}

func builtinMapcon(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("mapcon: need function and list")
	}
	fn := args[0]
	lst := args[1]
	seen := make(map[*Value]bool)
	cur := lst
	// nconc-2: append list2 to end of list1, returning list1 (destructive)
	nconc2 := func(list1, list2 *Value) *Value {
		if isNil(list2) {
			return list1
		}
		if isNil(list1) {
			return list2
		}
		t := list1
		for t.typ == VPair && !isNil(t.cdr) {
			t = t.cdr
		}
		t.cdr = list2
		return list1
	}
	var result *Value
	for !isNil(cur) && cur.typ == VPair {
		if seen[cur] {
			break
		}
		seen[cur] = true
		r, err := callFnOnSeq(fn, []*Value{cur}, globalEnv)
		if err != nil {
			return nil, err
		}
		result = nconc2(result, r)
		cur = cur.cdr
	}
	if result == nil {
		return vnil(), nil
	}
	return result, nil
}

// -------- list* --------
func builtinListStar(args []*Value) (*Value, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("list*: need at least one argument")
	}
	if len(args) == 1 {
		return args[0], nil
	}
	result := make([]*Value, len(args)-1)
	for i := 0; i < len(args)-1; i++ {
		result[i] = args[i]
	}
	return appendList(listFromSlice(result), args[len(args)-1]), nil
}

// appendList appends a tail to a proper list
func appendList(lst, tail *Value) *Value {
	if isNil(lst) {
		return tail
	}
	seen := make(map[*Value]bool)
	return appendListRec(lst, tail, seen)
}
func appendListRec(lst, tail *Value, seen map[*Value]bool) *Value {
	if isNil(lst) {
		return tail
	}
	if seen[lst] {
		return tail // break cycle
	}
	seen[lst] = true
	if lst.typ == VPair {
		return cons(lst.car, appendListRec(lst.cdr, tail, seen))
	}
	return tail
}

// -------- realpart / imagpart --------
func builtinRealpart(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("realpart: need a number")
	}
	v := args[0]
	if v.typ == VComplex {
		return vnum(v.num), nil
	}
	return v, nil
}

func builtinImagpart(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("imagpart: need a number")
	}
	v := args[0]
	if v.typ == VComplex {
		return vnum(v.imag), nil
	}
	return vnum(0), nil
}

// -------- conjugate --------
func builtinConjugate(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("conjugate: need a number")
	}
	v := args[0]
	if v.typ == VComplex {
		return vcomplex(v.num, -v.imag), nil
	}
	return v, nil
}

// -------- phase --------
func builtinPhase(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("phase: need a number")
	}
	v := args[0]
	if v.typ == VComplex {
		return vnum(math.Atan2(v.imag, v.num)), nil
	}
	n := toNum(v)
	if n >= 0 {
		return vnum(0), nil
	}
	return vnum(math.Pi), nil
}

// -------- cis --------
func builtinCis(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("cis: need a number")
	}
	radians := toNum(args[0])
	return vcomplex(math.Cos(radians), math.Sin(radians)), nil
}

// -------- asin --------
func builtinAsin(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("asin: need a number")
	}
	n := toNum(args[0])
	if args[0].typ == VComplex {
		// complex asin: asin(z) = -i*log(iz + sqrt(1-z^2))
		return nil, fmt.Errorf("asin: complex arguments not yet supported")
	}
	if n < -1 || n > 1 {
		// Result is complex for |n| > 1
		return vcomplex(0, math.Asin(n)), nil
	}
	return vnum(math.Asin(n)), nil
}

// -------- acos --------
func builtinAcos(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("acos: need a number")
	}
	n := toNum(args[0])
	if args[0].typ == VComplex {
		return nil, fmt.Errorf("acos: complex arguments not yet supported")
	}
	if n < -1 || n > 1 {
		return vcomplex(math.Acos(n), 0), nil
	}
	return vnum(math.Acos(n)), nil
}

// -------- rationalize --------
func builtinRationalize(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("rationalize: need a number")
	}
	v := args[0]
	if v.typ == VRat {
		return v, nil
	}
	n := toNum(v)
	if n == math.Trunc(n) && !v.isFloat {
		return vnum(n), nil
	}
	// Convert float to rational using continued fraction algorithm
	frac := big.NewRat(1, 1)
	frac.SetFloat64(n)
	// Reduce to integer numerator/denominator
	num := frac.Num()
	den := frac.Denom()
	if den.IsUint64() && num.IsUint64() {
		return vrat(int64(num.Uint64()), int64(den.Uint64())), nil
	}
	// For very large values, just return as float
	return vnum(n), nil
}

// -------- complex --------
func builtinComplex(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("complex: need real and imaginary parts")
	}
	realPart := toNum(args[0])
	imagPart := toNum(args[1])
	if imagPart == 0 {
		return vnum(realPart), nil
	}
	return vcomplex(realPart, imagPart), nil
}

// -------- type predicates --------
func isIntegerValue(v *Value) bool {
	if v.typ == VBigInt {
		return true
	}
	if v.typ == VRat {
		return v.iden == 1
	}
	if v.typ == VNum {
		// VNum with isFloat=true was explicitly created as a float (e.g., 3.0).
		// Per CLHS, floats are NOT integers, so reject them for bitwise ops.
		if v.isFloat {
			return false
		}
		// VNum with isFloat=false represents an integer literal (e.g., 3).
		return v.num == math.Trunc(v.num) && !math.IsInf(v.num, 0) && !math.IsNaN(v.num)
	}
	return false
}

func signalTypeError(arg *Value) (*Value, error) {
	cond := &Value{typ: VInstance, instClass: findClass("type-error"), instSlots: map[string]*Value{
		"message": vstr(fmt.Sprintf("type error: expected integer, got %s", ToString(arg))),
	}}
	return builtinError([]*Value{cond})
}

func builtinIntegerp(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	v := args[0]
	if v.typ == VNum {
		return vbool(v.num == math.Trunc(v.num)), nil
	}
	if v.typ == VRat {
		return vbool(v.iden == 1), nil
	}
	if v.typ == VBigInt {
		return vbool(true), nil
	}
	return vbool(false), nil
}

func builtinFloatp(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	v := args[0]
	if v.typ == VNum {
		// floatp true for values explicitly created as float or non-integer VNum
		return vbool(v.isFloat || v.num != math.Trunc(v.num)), nil
	}
	return vbool(false), nil
}

func builtinRationalp(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	v := args[0]
	return vbool(v.typ == VRat || v.typ == VBigInt || (v.typ == VNum && !v.isFloat)), nil
}

func builtinRealp(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	v := args[0]
	return vbool(v.typ == VNum || v.typ == VRat || v.typ == VBigInt), nil
}

func builtinComplexp(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	return vbool(args[0].typ == VComplex), nil
}

// -------- float / rational --------
func builtinFloat(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("float: need a number")
	}
	n := toNum(args[0])
	return vfloat(n), nil
}

func builtinRational(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("rational: need a number")
	}
	v := args[0]
	if v.typ == VRat {
		return v, nil
	}
	if v.typ == VNum {
		// Convert float to rational approximation
		r := big.NewRat(0, 1)
		r.SetFloat64(v.num)
		return vrat(r.Num().Int64(), r.Denom().Int64()), nil
	}
	return nil, fmt.Errorf("rational: not a real number")
}

// -------- numerator / denominator --------
func builtinNumerator(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("numerator: need a rational")
	}
	v := args[0]
	if v.typ == VRat {
		return vnum(float64(v.irat)), nil
	}
	if v.typ == VNum {
		return vnum(v.num), nil
	}
	return nil, fmt.Errorf("numerator: not a rational")
}

func builtinDenominator(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("denominator: need a rational")
	}
	v := args[0]
	if v.typ == VRat {
		return vnum(float64(v.iden)), nil
	}
	if v.typ == VNum {
		return vnum(1), nil
	}
	return nil, fmt.Errorf("denominator: not a rational")
}

// -------- ash (arithmetic shift) --------
func builtinAsh(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("ash: need integer and count")
	}
	n := toBigInt(args[0])
	if n == nil {
		// Fallback for non-bignum values
		n = big.NewInt(int64(toNum(args[0])))
	}
	count := int(toNum(args[1]))
	var result *big.Int
	if count >= 0 {
		result = new(big.Int).Lsh(n, uint(count))
	} else {
		result = new(big.Int).Rsh(n, uint(-count))
	}
	return vbigint(result), nil
}

// -------- logand / logior / logxor / lognot --------
func builtinLogand(args []*Value) (*Value, error) {
	if len(args) == 0 {
		return vnum(-1), nil // all ones
	}
	for _, a := range args {
		if !isIntegerValue(a) {
			return signalTypeError(a)
		}
	}
	if isBigIntInt(args) {
		result := toBigInt(args[0])
		if result == nil {
			result = big.NewInt(int64(toNum(args[0])))
		}
		for i := 1; i < len(args); i++ {
			bi := toBigInt(args[i])
			if bi == nil {
				bi = big.NewInt(int64(toNum(args[i])))
			}
			result.And(result, bi)
		}
		return vbigint(result), nil
	}
	result := int64(toNum(args[0]))
	for i := 1; i < len(args); i++ {
		result &= int64(toNum(args[i]))
	}
	return vnum(float64(result)), nil
}

func builtinLogior(args []*Value) (*Value, error) {
	if len(args) == 0 {
		return vnum(0), nil
	}
	for _, a := range args {
		if !isIntegerValue(a) {
			return signalTypeError(a)
		}
	}
	if isBigIntInt(args) {
		result := toBigInt(args[0])
		if result == nil {
			result = big.NewInt(int64(toNum(args[0])))
		}
		for i := 1; i < len(args); i++ {
			bi := toBigInt(args[i])
			if bi == nil {
				bi = big.NewInt(int64(toNum(args[i])))
			}
			result.Or(result, bi)
		}
		return vbigint(result), nil
	}
	result := int64(toNum(args[0]))
	for i := 1; i < len(args); i++ {
		result |= int64(toNum(args[i]))
	}
	return vnum(float64(result)), nil
}

func builtinLogxor(args []*Value) (*Value, error) {
	if len(args) == 0 {
		return vnum(0), nil
	}
	for _, a := range args {
		if !isIntegerValue(a) {
			return signalTypeError(a)
		}
	}
	if isBigIntInt(args) {
		result := toBigInt(args[0])
		if result == nil {
			result = big.NewInt(int64(toNum(args[0])))
		}
		for i := 1; i < len(args); i++ {
			bi := toBigInt(args[i])
			if bi == nil {
				bi = big.NewInt(int64(toNum(args[i])))
			}
			result.Xor(result, bi)
		}
		return vbigint(result), nil
	}
	result := int64(toNum(args[0]))
	for i := 1; i < len(args); i++ {
		result ^= int64(toNum(args[i]))
	}
	return vnum(float64(result)), nil
}

func builtinLognot(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("lognot: need an integer")
	}
	v := args[0]
	if v.typ == VBigInt {
		result := new(big.Int).Not(v.bigInt)
		return vbigint(result), nil
	}
	return vnum(float64(^int64(toNum(args[0])))), nil
}

// logandInts converts two args to int64 if possible, or returns big.Ints
func logAndInts(a, b *Value) (int64, int64, *big.Int, *big.Int, bool) {
	if a.typ == VBigInt || b.typ == VBigInt || a.typ == VNum && float64(int64(toNum(a))) != toNum(a) || b.typ == VNum && float64(int64(toNum(b))) != toNum(b) {
		ai := toBigInt(a)
		if ai == nil {
			ai = big.NewInt(int64(toNum(a)))
		}
		bi := toBigInt(b)
		if bi == nil {
			bi = big.NewInt(int64(toNum(b)))
		}
		return 0, 0, ai, bi, true
	}
	return int64(toNum(a)), int64(toNum(b)), nil, nil, false
}

func builtinLognand(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("lognand: need two integers")
	}
	a, b, ai, bi, isBig := logAndInts(args[0], args[1])
	if isBig {
		result := new(big.Int).Not(new(big.Int).And(ai, bi))
		return vbigint(result), nil
	}
	return vnum(float64(^(a & b))), nil
}

func builtinLognor(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("lognor: need two integers")
	}
	a, b, ai, bi, isBig := logAndInts(args[0], args[1])
	if isBig {
		result := new(big.Int).Not(new(big.Int).Or(ai, bi))
		return vbigint(result), nil
	}
	return vnum(float64(^(a | b))), nil
}

func builtinLogandc1(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("logandc1: need two integers")
	}
	a, b, ai, bi, isBig := logAndInts(args[0], args[1])
	if isBig {
		result := new(big.Int).And(new(big.Int).Not(ai), bi)
		return vbigint(result), nil
	}
	return vnum(float64((^a) & b)), nil
}

func builtinLogandc2(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("logandc2: need two integers")
	}
	a, b, ai, bi, isBig := logAndInts(args[0], args[1])
	if isBig {
		result := new(big.Int).And(ai, new(big.Int).Not(bi))
		return vbigint(result), nil
	}
	return vnum(float64(a & (^b))), nil
}

func builtinLogorc1(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("logorc1: need two integers")
	}
	a, b, ai, bi, isBig := logAndInts(args[0], args[1])
	if isBig {
		result := new(big.Int).Or(new(big.Int).Not(ai), bi)
		return vbigint(result), nil
	}
	return vnum(float64((^a) | b)), nil
}

func builtinLogorc2(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("logorc2: need two integers")
	}
	a, b, ai, bi, isBig := logAndInts(args[0], args[1])
	if isBig {
		result := new(big.Int).Or(ai, new(big.Int).Not(bi))
		return vbigint(result), nil
	}
	return vnum(float64(a | (^b))), nil
}

// -------- logcount --------
func builtinLogcount(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("logcount: need an integer")
	}
	v := args[0]
	if v.typ == VBigInt {
		n := new(big.Int)
		if v.bigInt.Sign() < 0 {
			n.Not(v.bigInt)
		} else {
			n.Set(v.bigInt)
		}
		count := 0
		for n.Sign() > 0 {
			count++
			n.And(n, new(big.Int).Sub(n, big.NewInt(1)))
		}
		return vnum(float64(count)), nil
	}
	n := int64(toNum(args[0]))
	if n < 0 {
		n = ^n
	}
	count := 0
	for n != 0 {
		count++
		n &= n - 1
	}
	return vnum(float64(count)), nil
}

// -------- integer-length --------
func builtinIntegerLength(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("integer-length: need an integer")
	}
	v := args[0]
	if v.typ == VBigInt {
		n := new(big.Int)
		if v.bigInt.Sign() < 0 {
			n.Not(v.bigInt)
		} else {
			n.Set(v.bigInt)
		}
		bits := 0
		for n.Sign() > 0 {
			bits++
			n.Rsh(n, 1)
		}
		return vnum(float64(bits)), nil
	}
	n := int64(toNum(args[0]))
	if n < 0 {
		n = ^n
	}
	bits := 0
	for n != 0 {
		bits++
		n >>= 1
	}
	return vnum(float64(bits)), nil
}

// -------- logbitp --------
func builtinLogbitp(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("logbitp: need bit index and integer")
	}
	bit := int(toNum(args[0]))
	v := args[1]
	if v.typ == VBigInt {
		bitmask := new(big.Int).Rsh(v.bigInt, uint(bit))
		return vbool(bitmask.Bit(0) == 1), nil
	}
	n := int64(toNum(args[1]))
	return vbool((n>>uint(bit))&1 == 1), nil
}

// -------- logtest --------
func builtinLogtest(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("logtest: need two integers")
	}
	a := int64(toNum(args[0]))
	b := int64(toNum(args[1]))
	if a&b != 0 {
		return vsym("T"), nil
	}
	return vnil(), nil
}

// -------- copy-alist --------
func builtinCopyAlist(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("copy-alist: need an alist")
	}
	alist := args[0]
	if isNil(alist) {
		return vnil(), nil
	}
	var result *Value = vnil()
	elems := seqToList(alist)
	for i := len(elems) - 1; i >= 0; i-- {
		entry := elems[i]
		if entry.typ == VPair {
			result = cons(cons(entry.car, entry.cdr), result)
		} else {
			result = cons(entry, result)
		}
	}
	return result, nil
}

// -------- mismatch --------
func builtinMismatch(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("mismatch: need two sequences")
	}
	s1 := seqToList(args[0])
	s2 := seqToList(args[1])
	start1, end1, start2, end2 := 0, -1, 0, -1
	var testFn, testNotFn, keyFn *Value
	testFn = nil
	testNotFn = nil
	keyFn = nil
	fromEnd := false
	for i := 2; i < len(args); i++ {
		if args[i].typ == VSym {
			switch args[i].str {
			case ":START1":
				if i+1 < len(args) {
					i++
					start1 = int(toNum(args[i]))
				}
			case ":END1":
				if i+1 < len(args) {
					i++
					end1 = int(toNum(args[i]))
				}
			case ":START2":
				if i+1 < len(args) {
					i++
					start2 = int(toNum(args[i]))
				}
			case ":END2":
				if i+1 < len(args) {
					i++
					end2 = int(toNum(args[i]))
				}
			case ":TEST":
				if i+1 < len(args) {
					i++
					testFn = args[i]
				}
			case ":TEST-NOT":
				if i+1 < len(args) {
					i++
					testNotFn = args[i]
				}
			case ":KEY":
				if i+1 < len(args) {
					i++
					keyFn = args[i]
				}
			case ":FROM-END":
				if i+1 < len(args) {
					i++
					fromEnd = isTruthy(args[i])
				}
			}
		}
	}
	if end1 < 0 || end1 > len(s1) {
		end1 = len(s1)
	}
	if end2 < 0 || end2 > len(s2) {
		end2 = len(s2)
	}
	// Helper to apply :key function
	applyKey := func(v *Value) *Value {
		if keyFn != nil {
			r, err := callFnOnSeq(keyFn, []*Value{v}, globalEnv)
			if err != nil {
				return v
			}
			return r
		}
		return v
	}
	// Helper to compare two elements using :test or :test-not or default eqVal
	elemsEqual := func(a, b *Value) bool {
		ka := applyKey(a)
		kb := applyKey(b)
		if testNotFn != nil {
			cmp, err := callFnOnSeq(testNotFn, []*Value{ka, kb}, globalEnv)
			if err != nil {
				return false
			}
			return !isTruthy(cmp)
		}
		if testFn != nil {
			cmp, err := callFnOnSeq(testFn, []*Value{ka, kb}, globalEnv)
			if err != nil {
				return false
			}
			return isTruthy(cmp)
		}
		return eqVal(ka, kb)
	}
	len1 := end1 - start1
	len2 := end2 - start2
	if fromEnd {
		// from-end: find the rightmost mismatch
		minLen := len1
		if len2 < minLen {
			minLen = len2
		}
		for i := minLen - 1; i >= 0; i-- {
			if !elemsEqual(s1[start1+i], s2[start2+i]) {
				return vnum(float64(i)), nil
			}
		}
		if len1 != len2 {
			return vnum(float64(minLen)), nil
		}
		return vnil(), nil
	}
	// Forward direction
	minLen := len1
	if len2 < minLen {
		minLen = len2
	}
	for i := 0; i < minLen; i++ {
		if !elemsEqual(s1[start1+i], s2[start2+i]) {
			return vnum(float64(i)), nil
		}
	}
	if len1 != len2 {
		return vnum(float64(minLen)), nil
	}
	return vnil(), nil
}
func builtinByte(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("byte: need size and position")
	}
	size := int(toNum(args[0]))
	position := int(toNum(args[1]))
	return list(vnum(float64(size)), vnum(float64(position))), nil
}

func builtinByteSize(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("byte-size: need a byte specifier")
	}
	bs := seqToList(args[0])
	if len(bs) >= 1 {
		return bs[0], nil
	}
	return vnum(0), nil
}

func builtinBytePosition(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("byte-position: need a byte specifier")
	}
	bs := seqToList(args[0])
	if len(bs) >= 2 {
		return bs[1], nil
	}
	return vnum(0), nil
}

func builtinLdb(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("ldb: need byte specifier and integer")
	}
	bs := seqToList(args[0])
	if len(bs) < 2 {
		return nil, fmt.Errorf("ldb: invalid byte specifier")
	}
	size := int(toNum(bs[0]))
	pos := int(toNum(bs[1]))
	n := int64(toNum(args[1]))
	if size <= 0 {
		return vnum(0), nil
	}
	if size > 63 {
		size = 63
	}
	mask := int64((1<<uint(size)) - 1)
	return vnum(float64((n >> uint(pos)) & mask)), nil
}

func builtinDpb(args []*Value) (*Value, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("dpb: need newbyte, byte specifier, and integer")
	}
	newByte := int64(toNum(args[0]))
	bs := seqToList(args[1])
	if len(bs) < 2 {
		return nil, fmt.Errorf("dpb: invalid byte specifier")
	}
	size := int(toNum(bs[0]))
	pos := int(toNum(bs[1]))
	n := int64(toNum(args[2]))
	if size <= 0 {
		return args[2], nil // zero-width field: no change
	}
	if size > 63 {
		size = 63
	}
	mask := int64((1 << uint(size)) - 1)
	return vnum(float64((n & ^(mask << uint(pos))) | ((newByte & mask) << uint(pos)))), nil
}

// -------- Byte manipulation helpers --------
func builtinLdbTest(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("ldb-test: need byte specifier and integer")
	}
	bs := seqToList(args[0])
	if len(bs) < 2 {
		return nil, fmt.Errorf("ldb-test: invalid byte specifier")
	}
	size := int(toNum(bs[0]))
	pos := int(toNum(bs[1]))
	n := int64(toNum(args[1]))
	if size <= 0 || size > 63 {
		return vbool(false), nil
	}
	mask := int64((1 << uint(size)) - 1)
	return vbool(((n >> uint(pos)) & mask) != 0), nil
}

func builtinMaskField(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("mask-field: need byte specifier and integer")
	}
	bs := seqToList(args[0])
	if len(bs) < 2 {
		return nil, fmt.Errorf("mask-field: invalid byte specifier")
	}
	size := int(toNum(bs[0]))
	pos := int(toNum(bs[1]))
	n := int64(toNum(args[1]))
	if size <= 0 {
		return vnum(0), nil
	}
	if size > 63 {
		size = 63
	}
	mask := int64((1 << uint(size)) - 1)
	return vnum(float64((n >> uint(pos)) & mask)), nil
}

func builtinDepositField(args []*Value) (*Value, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("deposit-field: need newbyte, byte specifier, and integer")
	}
	newByte := int64(toNum(args[0]))
	bs := seqToList(args[1])
	if len(bs) < 2 {
		return nil, fmt.Errorf("deposit-field: invalid byte specifier")
	}
	size := int(toNum(bs[0]))
	pos := int(toNum(bs[1]))
	n := int64(toNum(args[2]))
	if size <= 0 {
		return args[2], nil // zero-width field: no change
	}
	if size > 63 {
		size = 63
	}
	mask := int64((1 << uint(size)) - 1)
	return vnum(float64((n & ^(mask << uint(pos))) | ((newByte & mask) << uint(pos)))), nil
}

// -------- boole --------
func builtinBoole(args []*Value) (*Value, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("boole: need opcode, integer1, and integer2")
	}
	op := int(toNum(args[0]))
	a := int64(toNum(args[1]))
	b := int64(toNum(args[2]))
	var result int64
	switch op {
	case 0:
		result = 0 // boole-clr
	case 1:
		result = a & b // boole-and
	case 2:
		result = a & ^b // boole-andc1
	case 3:
		result = a // boole-1
	case 4:
		result = ^a & b // boole-andc2
	case 5:
		result = b // boole-2
	case 6:
		result = a ^ b // boole-xor
	case 7:
		result = a | b // boole-ior
	case 8:
		result = ^(a | b) // boole-nor
	case 9:
		result = ^(a ^ b) // boole-eqv
	case 10:
		result = ^b // boole-c2
	case 11:
		result = a | ^b // boole-orc2
	case 12:
		result = ^a // boole-c1
	case 13:
		result = ^a | b // boole-orc1
	case 14:
		result = ^(a & b) // boole-nand
	case 15:
		result = -1 // boole-set
	default:
		return nil, fmt.Errorf("boole: invalid opcode %d", op)
	}
	return vnum(float64(result)), nil
}

// -------- with-output-to-string (special form) --------
// Implemented as special form in eval

// -------- coerce improvements --------
func builtinCoerce(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("coerce: need object and result-type")
	}
	obj := args[0]
	resultType := args[1]
	typeStr := ""
	typeSub := ""
	if resultType.typ == VSym {
		typeStr = strings.ToLower(resultType.str)
	} else if isPair(resultType) && resultType.car != nil && resultType.car.typ == VSym {
		// Compound type specifier like (complex float)
		typeStr = strings.ToLower(resultType.car.str)
		if isPair(resultType.cdr) && resultType.cdr.car != nil && resultType.cdr.car.typ == VSym {
			typeSub = strings.ToLower(resultType.cdr.car.str)
		}
	}

	switch typeStr {
	case "string":
		if obj.typ == VStr {
			return obj, nil
		}
		if obj.typ == VChar {
			return vstr(string(obj.ch)), nil
		}
		if obj.typ == VSym {
			return vstr(obj.str), nil
		}
		if obj.typ == VPair || isNil(obj) {
			// list of characters/numbers/symbols/strings -> string
			var sb strings.Builder
			cur := obj
			for !isNil(cur) {
				if cur.typ == VPair {
					elt := cur.car
					if elt.typ == VChar {
						sb.WriteRune(elt.ch)
					} else if elt.typ == VNum {
						sb.WriteRune(rune(toNum(elt)))
					} else if elt.typ == VSym {
						sb.WriteString(elt.str)
					} else if elt.typ == VStr {
						sb.WriteString(elt.str)
					}
					cur = cur.cdr
				} else {
					break
				}
			}
			return vstr(sb.String()), nil
		}
		elems := seqToList(obj)
		var sb2 strings.Builder
		for _, v := range elems {
			if v.typ == VChar {
				sb2.WriteRune(v.ch)
			} else {
				sb2.WriteString(ToString(v))
			}
		}
		return vstr(sb2.String()), nil
	case "list", ":list":
		if obj.typ == VPair || isNil(obj) {
			return obj, nil
		}
		if obj.typ == VComplex {
			return listFromSlice([]*Value{vnum(obj.num), vnum(obj.imag)}), nil
		}
		if obj.typ == VStr {
			return listFromSlice(stringToCharList(obj.str)), nil
		}
		return listFromSlice(seqToList(obj)), nil
	case "float", ":float", "single-float", "double-float":
		if obj.typ != VNum && obj.typ != VRat && obj.typ != VBigInt && obj.typ != VComplex {
			return nil, fmt.Errorf("coerce: %v cannot be coerced to type %s", ToString(obj), typeStr)
		}
		return vfloat(toNum(obj)), nil
	case "rational", "ratio", ":rational", ":ratio":
		if obj.typ == VRat {
			return obj, nil
		}
		if obj.typ == VNum {
			n := toNum(obj)
			if n == float64(int(n)) {
				return vrat(int64(n), 1), nil
			}
			return toRational(n), nil
		}
		return nil, fmt.Errorf("coerce: cannot coerce to rational")
	case "complex", ":complex", "COMPLEX":
		// Check for compound type specifier: (complex float), (complex single-float), (complex double-float)
		switch typeSub {
		case "float", "single-float", "FLOAT", "SINGLE-FLOAT":
			if obj.typ == VComplex {
				r := float64(float32(obj.num))
				i := float64(float32(obj.imag))
				return vcomplexAlways(r, i), nil
			}
			r := float64(float32(toNum(obj)))
			return vcomplexAlways(r, 0), nil
		case "double-float", "DOUBLE-FLOAT":
			if obj.typ == VComplex {
				r := toNum(obj)
				i := obj.imag
				return vcomplexAlways(r, i), nil
			}
			return vcomplexAlways(toNum(obj), 0), nil
		case "rational", "integer", "RATIONAL", "INTEGER":
			// (complex rational) or (complex integer) - must always produce a VComplex
			if obj.typ == VComplex {
				return obj, nil
			}
			return vcomplexAlways(toNum(obj), 0), nil
		default:
			// Plain (complex) or unknown subtype - use vcomplex which simplifies #c(x 0) to x
			if obj.typ == VComplex {
				return obj, nil
			}
			return vcomplex(toNum(obj), 0), nil
		}
	case "character", ":character":
		if obj.typ == VChar {
			return obj, nil
		}
		if obj.typ == VStr && len(obj.str) == 1 {
			return vchar([]rune(obj.str)[0]), nil
		}
		if obj.typ == VStr && len(obj.str) > 1 {
			return nil, fmt.Errorf("coerce: string has more than one character")
		}
		if obj.typ == VNum {
			return vchar(rune(int(toNum(obj)))), nil
		}
		// Symbol designator: (coerce 'a 'character) => #\a
		if obj.typ == VSym && len(obj.str) == 1 {
			return vchar(rune(obj.str[0])), nil
		}
		return nil, fmt.Errorf("coerce: cannot coerce to character")
	case "standard-char", "base-char", ":standard-char", ":base-char":
		// Coerce to character, then verify it's a standard-char/base-char
		var ch rune
		switch obj.typ {
		case VChar:
			ch = obj.ch
		case VStr:
			if len(obj.str) == 0 {
				return nil, fmt.Errorf("coerce: string is empty")
			}
			ch = []rune(obj.str)[0]
		case VNum:
			ch = rune(int(toNum(obj)))
		default:
			return nil, fmt.Errorf("coerce: cannot coerce to %s", typeStr)
		}
		// Check if it's a standard-char
		if !isStandardChar(ch) {
			return nil, fmt.Errorf("coerce: %c is not of type %s", ch, strings.ToUpper(typeStr))
		}
		return vchar(ch), nil
	case "function", ":function":
		if obj.typ == VFunc || obj.typ == VPrim {
			return obj, nil
		}
		// (coerce 'name 'function) - look up function by name
		if obj.typ == VSym {
			fn, err := globalEnv.Get(obj.str)
			if err == nil && (fn.typ == VFunc || fn.typ == VPrim) {
				return fn, nil
			}
		}
		return nil, fmt.Errorf("coerce: cannot coerce to function")
	case "integer", ":integer":
		if obj.typ != VNum && obj.typ != VRat && obj.typ != VBigInt {
			return nil, fmt.Errorf("coerce: %v cannot be coerced to type INTEGER", ToString(obj))
		}
		n := toNum(obj)
		return vnum(math.Floor(n)), nil
	case "sequence", ":sequence":
		return obj, nil
	case "vector", ":vector", "simple-vector", ":simple-vector":
		if obj.typ == VArray && len(obj.array.dims) == 1 {
			return obj, nil
		}
		var elems []*Value
		if obj.typ == VStr {
			elems = stringToCharList(obj.str)
		} else {
			elems = seqToList(obj)
		}
		arr := &LispArray{dims: []int{len(elems)}, elements: elems, fillPtr: -1, adjustable: false}
		v := gcv()
		v.typ = VArray
		v.array = arr
		return v, nil
	case "bit-vector", "simple-bit-vector", ":bit-vector", ":simple-bit-vector":
		// Bit vector: 1D array containing only 0s and 1s
		var elems []*Value
		if obj.typ == VArray && len(obj.array.dims) == 1 {
			elems = obj.array.elements
		} else if obj.typ == VStr {
			// String of 0/1 characters
			for _, ch := range obj.str {
				if ch == '0' {
					elems = append(elems, vnum(0))
				} else if ch == '1' {
					elems = append(elems, vnum(1))
				} else {
					return nil, fmt.Errorf("coerce: string contains non-bit character %c", ch)
				}
			}
		} else {
			elems = seqToList(obj)
		}
		// Verify all elements are 0 or 1
		for i, e := range elems {
			if e.typ != VNum {
				return nil, fmt.Errorf("coerce: element %d is not a number", i)
			}
			n := toNum(e)
			if n != 0 && n != 1 {
				return nil, fmt.Errorf("coerce: element %d is not a bit (%d)", i, int(n))
			}
		}
		arr := &LispArray{dims: []int{len(elems)}, elements: elems, fillPtr: -1, adjustable: false}
		v := gcv()
		v.typ = VArray
		v.array = arr
		return v, nil
	case "array", ":array":
		if obj.typ == VArray {
			return obj, nil
		}
		var elems []*Value
		if obj.typ == VStr {
			elems = stringToCharList(obj.str)
		} else {
			elems = seqToList(obj)
		}
		arr := &LispArray{dims: []int{len(elems)}, elements: elems, fillPtr: -1, adjustable: false}
		v := gcv()
		v.typ = VArray
		v.array = arr
		return v, nil
	case "simple-array":
		// (coerce obj '(simple-array type (dim))) or (coerce obj 'simple-array)
		if obj.typ == VArray {
			return obj, nil
		}
		var elems []*Value
		if obj.typ == VStr {
			elems = stringToCharList(obj.str)
		} else {
			elems = seqToList(obj)
		}
		arr := &LispArray{dims: []int{len(elems)}, elements: elems, fillPtr: -1, adjustable: false}
		v := gcv()
		v.typ = VArray
		v.array = arr
		return v, nil
	case "real":
		if obj.typ == VComplex {
			return nil, fmt.Errorf("coerce: cannot coerce complex to type REAL")
		}
		return obj, nil
	case "number":
		return obj, nil
	case "symbol":
		if obj.typ == VSym {
			return obj, nil
		}
		if obj.typ == VStr {
			return vsym(obj.str), nil
		}
		return nil, fmt.Errorf("coerce: cannot coerce to symbol")
	default:
		return nil, fmt.Errorf("coerce: unsupported result-type %s", typeStr)
	}
}

func stringToCharList(s string) []*Value {
	runes := []rune(s)
	result := make([]*Value, len(runes))
	for i, r := range runes {
		result[i] = vchar(r)
	}
	return result
}

// character takes a character designator and returns the character.
// Designators: character, string of length 1, symbol of length 1, integer (code point).
func builtinCharacter(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("character: need a character designator")
	}
	designator := args[0]
	if designator.typ == VChar {
		return designator, nil
	}
	if designator.typ == VStr && len(designator.str) == 1 {
		return vchar([]rune(designator.str)[0]), nil
	}
	if designator.typ == VNum {
		return vchar(rune(int(toNum(designator)))), nil
	}
	if designator.typ == VSym && len(designator.str) == 1 {
		return vchar(rune(designator.str[0])), nil
	}
	return nil, fmt.Errorf("character: %v is not a character designator", designator)
}

// constantp returns true if the form is a constant at compile time.
// Constants are: numbers, characters, strings, symbols with constant values,
// and lists whose car is a special operator like quote.
func builtinConstantp(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("constantp: need a form")
	}
	form := args[0]
	if isConstant(form) {
		return vbool(true), nil
	}
	return vbool(false), nil
}

func isConstant(v *Value) bool {
	if v == nil {
		return false
	}
	// Numbers, characters, strings are self-evaluating constants
	if v.typ == VNum || v.typ == VChar || v.typ == VStr {
		return true
	}
	// Quoted forms: (quote x)
	if isPair(v) && v.car != nil && v.car.typ == VSym {
		symName := v.car.str
		if symName == "QUOTE" || symName == "FUNCTION" {
			return true
		}
	}
	return false
}

// -------- ANSI CL Environment Inquiry Functions --------

// variable-information returns information about a variable binding.
// Returns (values binding-type local-p decls), where binding-type is
// :SPECIAL, :LEXICAL, :SYMBOL-MACRO, :CONSTANT, or NIL.
func builtinVariableInformation(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("variable-information: need a symbol")
	}
	sym := args[0]
	if sym.typ != VSym {
		return nil, fmt.Errorf("variable-information: need a symbol")
	}
	name := sym.str
	// Check if it's a constant (like T, NIL, PI, etc.)
	constants := map[string]bool{
		"T": true, "NIL": true, "PI": true,
		"MOST-POSITIVE-FIXNUM": true, "MOST-NEGATIVE-FIXNUM": true,
		"CHAR-CODE-LIMIT": true,
	}
	if constants[name] {
		return multiVal(vsym(":CONSTANT"), vbool(false), vnil()), nil
	}
	// Check if it's a special variable (*var* pattern)
	if strings.HasPrefix(name, "*") && strings.HasSuffix(name, "*") && len(name) > 1 {
		return multiVal(vsym(":SPECIAL"), vbool(false), vnil()), nil
	}
	// Check global environment
	_, err := globalEnv.Get(name)
	if err != nil {
		// Not bound at all
		return multiVal(vnil(), vbool(false), vnil()), nil
	}
	// Default: treat as special (global) binding
	return multiVal(vsym(":SPECIAL"), vbool(false), vnil()), nil
}

// function-information returns information about a function binding.
// Returns (values binding-type local-p decls), where binding-type is
// :FUNCTION, :MACRO, :SPECIAL-FORM, or NIL.
func builtinFunctionInformation(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("function-information: need a symbol")
	}
	sym := args[0]
	if sym.typ != VSym {
		return nil, fmt.Errorf("function-information: need a symbol")
	}
	name := sym.str
	// Check if it's a special operator
	specialOps := map[string]bool{
		"IF": true, "QUOTE": true, "SETQ": true, "BLOCK": true, "RETURN-FROM": true,
		"LET": true, "LET*": true, "PROGN": true, "TAGBODY": true, "GO": true,
		"FLET": true, "LABELS": true, "MACROLET": true, "FUNCTION": true,
		"MULTIPLE-VALUE-BIND": true, "MULTIPLE-VALUE-PROG1": true,
		"CATCH": true, "THROW": true, "UNWIND-PROTECT": true,
		"THE": true, "LOCALLY": true, "EVAL-WHEN": true,
		"SYMBOL-MACROLET": true, "LOAD-TIME-VALUE": true,
	}
	if specialOps[name] {
		return multiVal(vsym(":SPECIAL-FORM"), vbool(false), vnil()), nil
	}
	// Check global environment for function/macro
	fn, err := globalEnv.Get(name)
	if err != nil {
		return multiVal(vnil(), vbool(false), vnil()), nil
	}
	if fn.typ == VMacro {
		return multiVal(vsym(":MACRO"), vbool(false), vnil()), nil
	}
	if fn.typ == VPrim || fn.typ == VFunc || fn.typ == VGeneric {
		return multiVal(vsym(":FUNCTION"), vbool(false), vnil()), nil
	}
	return multiVal(vnil(), vbool(false), vnil()), nil
}

// declaration-information returns information about a declaration.
// Returns (values info), where info is declaration-specific.
func builtinDeclarationInformation(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("declaration-information: need a declaration specifier")
	}
	spec := args[0]
	if spec.typ != VSym {
		return multiVal(vnil(), vbool(false)), nil
	}
	name := strings.ToUpper(spec.str)
	switch name {
	case "OPTIMIZE":
		// Return default optimization qualities
		return multiVal(list(
			list(vsym("SPEED"), vnum(1)),
			list(vsym("SAFETY"), vnum(1)),
			list(vsym("DEBUG"), vnum(1)),
			list(vsym("SPACE"), vnum(1)),
			list(vsym("COMPILATION-SPEED"), vnum(1)),
		), vbool(false)), nil
	case "DECLARATION":
		// Return known declaration names
		return multiVal(list(vsym("OPTIMIZE"), vsym("DECLARATION"), vsym("DYNAMIC-EXTENT"), vsym("TYPE"), vsym("FTYPE"), vsym("NOTINLINE"), vsym("INLINE"), vsym("SPECIAL")), vbool(false)), nil
	default:
		return multiVal(vnil(), vbool(false)), nil
	}
}

func builtinIsqrt(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("isqrt: need a number")
	}
	n := toNum(args[0])
	if n < 0 {
		return nil, fmt.Errorf("isqrt: negative argument")
	}
	r := int(math.Sqrt(n))
	return vnum(float64(r)), nil
}

// -------- ANSI CL floating-point introspection --------

func builtinDecodeFloat(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("decode-float: need a float")
	}
	f := toNum(args[0])
	if f == 0 {
		return multiVal(vfloat(0), vnum(0), vnum(1)), nil
	}
	sign := 1.0
	if f < 0 {
		sign = -1.0
		f = -f
	}
	mantissa, exp := math.Frexp(f)
	return multiVal(vfloat(mantissa*sign), vnum(float64(exp)), vnum(1.0)), nil
}

func builtinIntegerDecodeFloat(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("integer-decode-float: need a float")
	}
	f := toNum(args[0])
	if f == 0 {
		return multiVal(vnum(0), vnum(0), vnum(1)), nil
	}
	sign := float64(1)
	if f < 0 {
		sign = -1
		f = -f
	}
	mantissa, exp := math.Frexp(f)
	intSig := int64(mantissa * (1 << 53))
	intExp := exp - 53
	return multiVal(vnum(float64(intSig)), vnum(float64(intExp)), vnum(sign)), nil
}

func builtinScaleFloat(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("scale-float: need float and integer")
	}
	f := toNum(args[0])
	n := toNum(args[1])
	return vfloat(f * math.Pow(2, n)), nil
}

func builtinFloatRadix(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("float-radix: need a float")
	}
	return vnum(2), nil
}

func builtinFloatDigits(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("float-digits: need a float")
	}
	return vnum(53), nil
}

func builtinFloatPrecision(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("float-precision: need a float")
	}
	f := toNum(args[0])
	if f == 0 {
		return vnum(0), nil
	}
	return vnum(53), nil
}

// -------- Advanced format --------

type fmtState struct {
	ctrl      string
	pos       int
	args      []*Value
	argIdx    int
	buf       strings.Builder
	escaped   bool
	remaining int // items remaining in current ~{ iteration (-1 = not in iteration)
}

func (fs *fmtState) done() bool { return fs.pos >= len(fs.ctrl) }
func (fs *fmtState) peek() byte {
	if fs.done() {
		return 0
	}
	return fs.ctrl[fs.pos]
}
func (fs *fmtState) next() byte {
	c := fs.ctrl[fs.pos]
	fs.pos++
	return c
}

func (fs *fmtState) popArg() *Value {
	if fs.argIdx < len(fs.args) {
		v := fs.args[fs.argIdx]
		fs.argIdx++
		return v
	}
	return vnil()
}

// formatBigIntBase formats a big.Int in the given base (2, 8, 16).
func formatBigIntBase(n *big.Int, base int) string {
	if base >= 2 && base <= 36 {
		return new(big.Int).Set(n).Text(base)
	}
	return new(big.Int).Set(n).Text(10)
}

func formatRoman(n int) string {
	if n <= 0 || n >= 4000 {
		return ""
	}
	vals := []int{1000, 900, 500, 400, 100, 90, 50, 40, 10, 9, 5, 4, 1}
	romans := []string{"m", "cm", "d", "cd", "c", "xc", "l", "xl", "x", "ix", "v", "iv", "i"}
	var b strings.Builder
	for i, v := range vals {
		for n >= v {
			b.WriteString(romans[i])
			n -= v
		}
	}
	return b.String()
}

func formatRomanUpper(n int) string {
	return strings.ToUpper(formatRoman(n))
}

func formatOldRoman(n int) string {
	// Old-style Roman: like regular Roman but uses simpler additive notation
	if n <= 0 || n >= 4000 {
		return ""
	}
	vals := []int{1000, 500, 100, 50, 10, 5, 1}
	romans := []string{"M", "D", "C", "L", "X", "V", "I"}
	var b strings.Builder
	for i, v := range vals {
		for n >= v {
			b.WriteString(romans[i])
			n -= v
		}
	}
	return b.String()
}

// formatCardinal converts a number to English cardinal word form.
// 0 -> "zero", 1 -> "one", 21 -> "twenty-one", 123 -> "one hundred twenty-three"
func formatCardinal(n int) string {
	if n == 0 {
		return "zero"
	}
	if n < 0 {
		return "minus " + formatCardinalPositive(-n)
	}
	return formatCardinalPositive(n)
}

func formatCardinalPositive(n int) string {
	if n == 0 {
		return ""
	}
	if n >= 1000000000 {
		return formatCardinalHelper(n/1000000000, "billion", formatCardinalPositive(n%1000000000))
	}
	if n >= 1000000 {
		return formatCardinalHelper(n/1000000, "million", formatCardinalPositive(n%1000000))
	}
	if n >= 1000 {
		return formatCardinalHelper(n/1000, "thousand", formatCardinalPositive(n%1000))
	}
	if n >= 100 {
		return formatCardinalHelper(n/100, "hundred", formatCardinalPositive(n%100))
	}
	tens := []string{"", "", "twenty", "thirty", "forty", "fifty", "sixty", "seventy", "eighty", "ninety"}
	teens := []string{"ten", "eleven", "twelve", "thirteen", "fourteen", "fifteen", "sixteen", "seventeen", "eighteen", "nineteen"}
	units := []string{"", "one", "two", "three", "four", "five", "six", "seven", "eight", "nine"}
	switch {
	case n >= 20:
		s := tens[n/10]
		if n%10 != 0 {
			s += "-" + units[n%10]
		}
		return s
	case n >= 10:
		return teens[n-10]
	default:
		return units[n]
	}
}

func formatCardinalHelper(val int, name, rest string) string {
	s := formatCardinalPositive(val) + " " + name
	if rest != "" {
		s += " " + rest
	}
	return s
}

// formatOrdinal converts a number to English ordinal word form.
// 0 -> "zeroth", 1 -> "first", 21 -> "twenty-first", 123 -> "one hundred twenty-third"
func formatOrdinal(n int) string {
	if n == 0 {
		return "zeroth"
	}
	if n < 0 {
		return "minus " + formatOrdinalPositive(-n)
	}
	return formatOrdinalPositive(n)
}

func formatOrdinalPositive(n int) string {
	ordinals := []string{
		"zeroth", "first", "second", "third", "fourth", "fifth",
		"sixth", "seventh", "eighth", "ninth", "tenth",
		"eleventh", "twelfth", "thirteenth", "fourteenth", "fifteenth",
		"sixteenth", "seventeenth", "eighteenth", "nineteenth",
		"twentieth", "twenty-first", "twenty-second", "twenty-third",
		"twenty-fourth", "twenty-fifth", "twenty-sixth", "twenty-seventh",
		"twenty-eighth", "twenty-ninth", "thirtieth", "thirty-first",
	}
	if n < len(ordinals) {
		return ordinals[n]
	}
	// For larger numbers, use cardinal form with ordinal ending on last word
	cardinal := formatCardinalPositive(n)
	words := strings.Split(cardinal, " ")
	last := words[len(words)-1]
	ordLast := lastOrdinal(last)
	if ordLast == last {
		// Fallback: just append "th"
		ordLast = last + "th"
	}
	words[len(words)-1] = ordLast
	return strings.Join(words, " ")
}

func lastOrdinal(cardinal string) string {
	ordinals := map[string]string{
		"zero": "zeroth", "one": "first", "two": "second", "three": "third",
		"four": "fourth", "five": "fifth", "six": "sixth", "seven": "seventh",
		"eight": "eighth", "nine": "ninth", "ten": "tenth",
		"eleven": "eleventh", "twelve": "twelfth", "thirteen": "thirteenth",
		"fourteen": "fourteenth", "fifteen": "fifteenth", "sixteen": "sixteenth",
		"seventeen": "seventeenth", "eighteen": "eighteenth", "nineteen": "nineteenth",
		"twenty": "twentieth", "thirty": "thirtieth", "forty": "fortieth",
		"fifty": "fiftieth", "sixty": "sixtieth", "seventy": "seventieth",
		"eighty": "eightieth", "ninety": "ninetieth",
	}
	// Check if cardinal ends with hyphenated word like "twenty-one"
	if idx := strings.LastIndex(cardinal, "-"); idx != -1 {
		lastWord := cardinal[idx+1:]
		if ord, ok := ordinals[lastWord]; ok {
			return cardinal[:idx+1] + ord
		}
	}
	if ord, ok := ordinals[cardinal]; ok {
		return ord
	}
	return cardinal
}

func (fs *fmtState) parseFmtDirective() (params []interface{}, colon, at bool, cmd byte) {
	colon = false
	at = false
	params = nil
	gotValue := false
	for !fs.done() {
		c := fs.peek()
		if c == ':' {
			colon = true
			fs.next()
		} else if c == '@' {
			at = true
			fs.next()
		} else if c == '\'' {
			fs.next()
			if !fs.done() {
				params = append(params, fs.next())
			}
			gotValue = true
		} else if c == 'V' || c == 'v' {
			fs.next()
			params = append(params, 'V')
			gotValue = true
		} else if c == '#' {
			fs.next()
			params = append(params, '#')
			gotValue = true
		} else if c >= '0' && c <= '9' {
			n := 0
			for !fs.done() && fs.peek() >= '0' && fs.peek() <= '9' {
				n = n*10 + int(fs.next()-'0')
			}
			params = append(params, n)
			gotValue = true
		} else if c == ',' {
			fs.next()
			if !gotValue {
				params = append(params, -1)
			}
			gotValue = false
		} else {
			break
		}
	}
	if !fs.done() {
		cmd = fs.next()
	}
	return
}

func (fs *fmtState) getParam(params []interface{}, idx, defaultVal int) int {
	if idx < len(params) {
		if v, ok := params[idx].(int); ok {
			return v
		}
	}
	// Check for V param
	if idx < len(params) {
		if _, ok := params[idx].(byte); ok {
			arg := fs.popArg()
			if arg.typ == VNum {
				return int(toNum(arg))
			}
			return defaultVal
		}
	}
	return defaultVal
}

func (fs *fmtState) getCharParam(params []interface{}, idx int, defaultVal byte) byte {
	if idx < len(params) {
		switch v := params[idx].(type) {
		case byte:
			return v
		case int:
			return byte(v)
		}
	}
	return defaultVal
}

func builtinFormat(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("format: need stream and control-string")
	}
	stream := args[0]
	ctrl := args[1]
	if ctrl.typ != VStr {
		return nil, fmt.Errorf("format: control-string must be a string")
	}
	fs := &fmtState{
		ctrl: ctrl.str,
		args: args[2:],
	}
	formatRun(fs)
	result := fs.buf.String()
	if stream == globalEnv.bindings["#t"] {
		fmt.Print(result)
		return vnil(), nil
	}
	// If stream is a real stream (not nil), write to it
	if stream.typ == VStream && stream.stream != nil && stream.stream.isOutput {
		if stream.stream.isString && stream.stream.strBuf != nil {
			stream.stream.strBuf.WriteString(result)
		}
		return vnil(), nil
	}
	return vstr(result), nil
}

func newFmtState(ctrl string, args []*Value, argIdx int) *fmtState {
	return &fmtState{ctrl: ctrl, args: args, argIdx: argIdx, remaining: -1}
}

func formatRun(fs *fmtState) {
	for !fs.done() && !fs.escaped {
		c := fs.peek()
		if c == '~' {
			fs.next()
			if fs.done() {
				fs.buf.WriteByte('~')
				return
			}
			formatDispatch(fs)
		} else {
			fs.buf.WriteByte(fs.next())
		}
	}
}

func formatDispatch(fs *fmtState) {
	params, colon, at, cmd := fs.parseFmtDirective()
	// Format directives are case-insensitive
	if cmd >= 'a' && cmd <= 'z' {
		cmd = cmd - 'a' + 'A'
	}
	switch cmd {
	case 'A':
		arg := fs.popArg()
		mincol := fs.getParam(params, 0, 0)
		colinc := fs.getParam(params, 1, 1)
		minpad := fs.getParam(params, 2, 0)
		padchar := fs.getCharParam(params, 3, ' ')
		s := princToString(arg)
		if isNil(arg) && colon {
			s = "()"
		}
		padlen := 0
		if int(mincol) > len(s) {
			padlen = int(mincol) - len(s)
			if int(minpad) > padlen {
				padlen = int(minpad)
			}
			if int(colinc) > 1 {
				for padlen < int(mincol)-len(s) || (padlen-int(minpad))%int(colinc) != 0 {
					padlen++
				}
			}
		}
		if at {
			for i := 0; i < padlen; i++ {
				fs.buf.WriteByte(byte(padchar))
			}
			fs.buf.WriteString(s)
		} else {
			fs.buf.WriteString(s)
			for i := 0; i < padlen; i++ {
				fs.buf.WriteByte(byte(padchar))
			}
		}
	case 'S':
		arg := fs.popArg()
		mincol := fs.getParam(params, 0, 0)
		padchar := fs.getCharParam(params, 3, ' ')
		s := writeToString(arg)
		padlen := 0
		if int(mincol) > len(s) {
			padlen = int(mincol) - len(s)
		}
		if at {
			for i := 0; i < padlen; i++ {
				fs.buf.WriteByte(byte(padchar))
			}
			fs.buf.WriteString(s)
		} else {
			fs.buf.WriteString(s)
			for i := 0; i < padlen; i++ {
				fs.buf.WriteByte(byte(padchar))
			}
		}
	case 'D':
		width := fs.getParam(params, 0, 0)
		padchar := fs.getCharParam(params, 1, ' ')
		arg := fs.popArg()
		var s string
		if arg.typ == VBigInt {
			s = arg.bigInt.String()
		} else {
			s = strconv.FormatInt(int64(toNum(arg)), 10)
		}
		n := 0
		if arg.typ == VBigInt {
			n = arg.bigInt.Sign()
		} else {
			n = int(toNum(arg))
		}
		if n >= 0 && at {
			s = "+" + s
		}
		for len(s) < width {
			s = string(padchar) + s
		}
		if colon && !at {
			var b strings.Builder
			for i, c := range s {
				if i > 0 && (len(s)-i)%3 == 0 && c != '-' {
					b.WriteByte(',')
				}
				b.WriteRune(c)
			}
			s = b.String()
		}
		fs.buf.WriteString(s)
	case 'B':
		arg := fs.popArg()
		var s string
		if arg.typ == VBigInt {
			s = formatBigIntBase(arg.bigInt, 2)
		} else {
			s = strconv.FormatInt(int64(toNum(arg)), 2)
		}
		if at {
			fs.buf.WriteString("#b" + s)
		} else {
			fs.buf.WriteString(s)
		}
	case 'O':
		arg := fs.popArg()
		var s string
		if arg.typ == VBigInt {
			s = formatBigIntBase(arg.bigInt, 8)
		} else {
			s = strconv.FormatInt(int64(toNum(arg)), 8)
		}
		if at {
			fs.buf.WriteString("#o" + s)
		} else {
			fs.buf.WriteString(s)
		}
	case 'X':
		arg := fs.popArg()
		var s string
		if arg.typ == VBigInt {
			s = formatBigIntBase(arg.bigInt, 16)
		} else {
			s = strconv.FormatInt(int64(toNum(arg)), 16)
		}
		if at {
			fs.buf.WriteString("#x" + s)
		} else {
			fs.buf.WriteString(s)
		}
	case 'F':
		decimals := fs.getParam(params, 1, -1)
		arg := fs.popArg()
		f := toNum(arg)
		var s string
		if decimals >= 0 {
			s = strconv.FormatFloat(f, 'f', decimals, 64)
		} else {
			s = strconv.FormatFloat(f, 'f', -1, 64)
		}
		// ~F must always produce a float: ensure decimal point is present
		if !strings.Contains(s, ".") {
			s = s + ".0"
		}
		fs.buf.WriteString(s)
	case '$':
		// ~$, ~:$, ~@$ - dollar float formatting
		// Params: d n w padchar
		// d = digits after decimal (default 2)
		// n = minimum digits before decimal point (default 1)
		// w = minimum field width (default 0)
		// padchar = padding character (default space)
		d := fs.getParam(params, 0, 2)
		n := fs.getParam(params, 1, 1)
		w := fs.getParam(params, 2, 0)
		padchar := fs.getCharParam(params, 3, ' ')
		arg := fs.popArg()
		f := toNum(arg)
		s := strconv.FormatFloat(f, 'f', int(d), 64)
		// Ensure minimum digits before decimal point
		if idx := strings.Index(s, "."); idx >= 0 {
			beforeDot := s[:idx]
			sign := ""
			if len(beforeDot) > 0 && (beforeDot[0] == '-' || beforeDot[0] == '+') {
				sign = string(beforeDot[0])
				beforeDot = beforeDot[1:]
			}
			for len(beforeDot) < int(n) {
				beforeDot = "0" + beforeDot
			}
			s = sign + beforeDot + s[idx:]
		}
		// Handle @ modifier: always print sign
		if at && f >= 0 {
			s = "+" + s
		}
		// Handle colon modifier: sign appears before padding
		if colon {
			// ~:$ sign before padding
			padlen := int(w) - len(s)
			if padlen < 0 {
				padlen = 0
			}
			signPart := ""
			if len(s) > 0 && (s[0] == '-' || s[0] == '+') {
				signPart = string(s[0])
				s = s[1:]
			}
			fs.buf.WriteString(signPart)
			for i := 0; i < padlen; i++ {
				fs.buf.WriteRune(rune(padchar))
			}
			fs.buf.WriteString(s)
		} else {
			padlen := int(w) - len(s)
			if padlen < 0 {
				padlen = 0
			}
			for i := 0; i < padlen; i++ {
				fs.buf.WriteRune(rune(padchar))
			}
			fs.buf.WriteString(s)
		}
	case 'E':
		// ~E: scientific notation
		decimals := fs.getParam(params, 1, -1)
		eMinDigits := fs.getParam(params, 2, 1)
		arg := fs.popArg()
		f := toNum(arg)
		prec := -1
		if decimals >= 0 {
			prec = int(decimals) + 1
		}
		s := strconv.FormatFloat(f, 'E', prec, 64)
		idx := strings.Index(s, "E")
		if idx < 0 {
			fs.buf.WriteString(s)
			break
		}
		mantissa := s[:idx]
		expStr := s[idx+1:]
		// Ensure mantissa has a decimal point
		if !strings.Contains(mantissa, ".") {
			mantissa += ".0"
		}
		// Normalize exponent: strip leading zeros
		expSign := ""
		if len(expStr) > 0 && (expStr[0] == '+' || expStr[0] == '-') {
			expSign = string(expStr[0])
			expStr = expStr[1:]
		}
		expDigits := strings.TrimLeft(expStr, "0")
		if expDigits == "" {
			expDigits = "0"
		}
		// Pad to minimum digit count
		for len(expDigits) < int(eMinDigits) {
			expDigits = "0" + expDigits
		}
		fs.buf.WriteString(mantissa + "E" + expSign + expDigits)
	case 'R':
		arg := fs.popArg()
		n := int(toNum(arg))
		// ~nR with radix parameter: print in base n (ANSI CL)
		if len(params) >= 1 && !colon && !at {
			radix := int(fs.getParam(params, 0, 10))
			if radix < 2 || radix > 36 {
				fs.buf.WriteString(formatCardinal(n))
			} else {
				fs.buf.WriteString(formatBigIntBase(big.NewInt(int64(n)), radix))
			}
			break
		}
		switch {
		case at && colon:
			// ~:@R: old-style Roman numerals
			fs.buf.WriteString(formatOldRoman(n))
		case at:
			// ~@R: Roman numerals (uppercase)
			fs.buf.WriteString(formatRomanUpper(n))
		case colon:
			// ~:R: ordinal
			fs.buf.WriteString(formatOrdinal(n))
		default:
			// ~R: cardinal
			fs.buf.WriteString(formatCardinal(n))
		}
	case 'C':
		arg := fs.popArg()
		if arg.typ == VChar {
			if at {
				// ~@C: print with escape syntax like prin1
				fs.buf.WriteString(ToString(arg))
			} else if colon {
				// ~:C: spell out the character name for non-printing chars
				ch := arg.ch
				switch ch {
				case ' ':
					fs.buf.WriteString("Space")
				case '\n':
					fs.buf.WriteString("Newline")
				case '\t':
					fs.buf.WriteString("Tab")
				case '\r':
					fs.buf.WriteString("Return")
				case '\x08':
					fs.buf.WriteString("Backspace")
				case '\x7f':
					fs.buf.WriteString("Rubout")
				case '\f':
					fs.buf.WriteString("Page")
				default:
					if ch < ' ' || ch == '\x7f' {
						// Non-printing: use #\name form
						fs.buf.WriteString(ToString(arg))
					} else {
						fs.buf.WriteRune(ch)
					}
				}
			} else {
				// ~C: print just the character itself (like princ)
				fs.buf.WriteRune(arg.ch)
			}
		} else {
			fs.buf.WriteString(princToString(arg))
		}
	case '%':
		n := int(fs.getParam(params, 0, 1))
		for i := 0; i < n; i++ {
			fs.buf.WriteString("\n")
		}
	case '&':
		// fresh-line: output newline unless already at start of line
		if fs.buf.Len() > 0 {
			n := int(fs.getParam(params, 0, 1))
			for i := 0; i < n; i++ {
				fs.buf.WriteString("\n")
			}
		}
	case '|':
		fs.buf.WriteString("\f")
	case '~':
		fs.buf.WriteString("~")
	case 'T':
		colnum := fs.getParam(params, 0, 1)
		colinc := fs.getParam(params, 1, 1)
		current := fs.buf.Len()
		if at {
			// ~@T: output colnum spaces, then enough to reach next multiple of colinc
			for i := 0; i < int(colnum); i++ {
				fs.buf.WriteByte(' ')
			}
			current = fs.buf.Len()
			if int(colinc) > 0 {
				rem := current % int(colinc)
				if rem != 0 {
					for i := 0; i < int(colinc)-rem; i++ {
						fs.buf.WriteByte(' ')
					}
				}
			}
		} else {
			// ~T: advance to column colnum, or if already past, advance by colinc
			if current < int(colnum) {
				for i := current; i < int(colnum); i++ {
					fs.buf.WriteByte(' ')
				}
			} else if int(colinc) > 0 {
				target := current
				if target%int(colinc) != 0 {
					target = ((target / int(colinc)) + 1) * int(colinc)
				} else {
					target = current + int(colinc)
				}
				for i := current; i < target; i++ {
					fs.buf.WriteByte(' ')
				}
			}
		}
	case '*':
		n := fs.getParam(params, 0, 1)
		if at {
			// ~@* with no param: go to arg 0 (first argument)
			if len(params) == 0 {
				n = 0
			}
			fs.argIdx = n
		} else {
			fs.argIdx += n
		}
	case '?':
		if at {
			// ~@?: pop control string from args, use remaining args for recursive format
			newCtrl := fs.popArg()
			if newCtrl.typ == VStr {
				subFs := &fmtState{ctrl: newCtrl.str, args: fs.args, argIdx: fs.argIdx}
				formatRun(subFs)
				fs.buf.WriteString(subFs.buf.String())
				fs.argIdx = subFs.argIdx
			}
		} else {
			// ~?: pop control string and argument list
			newCtrl := fs.popArg()
			newArgs := fs.popArg()
			if newCtrl.typ == VStr {
				subFs := &fmtState{ctrl: newCtrl.str, args: seqToList(newArgs)}
				formatRun(subFs)
				fs.buf.WriteString(subFs.buf.String())
			}
		}
	case '(':
		// ~( ... ~) - case conversion
		depth := 1
		bodyStart := fs.pos
		for !fs.done() && depth > 0 {
			c := fs.next()
			if c == '~' && !fs.done() {
				nc := fs.next()
				if nc == '(' {
					depth++
				} else if nc == ')' {
					depth--
				}
			}
		}
		bodyEnd := fs.pos - 2
		if bodyEnd < bodyStart {
			bodyEnd = bodyStart
		}
		body := fs.ctrl[bodyStart:bodyEnd]
		subFs := newFmtState(body, fs.args, fs.argIdx)
		formatRun(subFs)
		fs.argIdx = subFs.argIdx
		s := subFs.buf.String()
		if colon && at {
			s = titleCaseString(strings.ToLower(s))
		} else if colon {
			s = strings.ToUpper(s)
		} else if at {
			if len(s) > 0 {
				s = strings.ToUpper(s[:1]) + strings.ToLower(s[1:])
			}
		} else {
			s = strings.ToLower(s)
		}
		fs.buf.WriteString(s)
	case ')':
	case '[':
		sections := formatCollectSections(fs, ']')
		selector := fs.popArg()
		sel := int(toNum(selector))
		if sel < 0 {
			sel = 0
		}
		if sel >= len(sections) {
			sel = len(sections) - 1
		}
		if sel >= 0 && sel < len(sections) {
			subFs := &fmtState{ctrl: sections[sel], args: fs.args, argIdx: fs.argIdx}
			formatRun(subFs)
			fs.buf.WriteString(subFs.buf.String())
			fs.argIdx = subFs.argIdx
		}
	case ';', ']':
	case '{':
		body := formatCollectBody(fs, '}')
		limit := fs.getParam(params, 0, -1) // -1 means no limit
		if colon {
			listArg := fs.popArg()
			elements := seqToList(listArg)
			for i, el := range elements {
				if limit >= 0 && i >= limit {
					break
				}
				rem := len(elements) - i - 1
				subFs := &fmtState{ctrl: body, args: seqToList(el), remaining: rem}
				formatRun(subFs)
				fs.buf.WriteString(subFs.buf.String())
			}
		} else if at {
			count := 0
			prevArgIdx := fs.argIdx
			iterCount := 0
			for fs.argIdx < len(fs.args) {
				if limit >= 0 && count >= limit {
					break
				}
				if iterCount > len(fs.args)+10 {
					break
				}
				rem := len(fs.args) - fs.argIdx - 1
				subFs := &fmtState{ctrl: body, args: fs.args, argIdx: fs.argIdx, remaining: rem}
				formatRun(subFs)
				fs.buf.WriteString(subFs.buf.String())
				if subFs.argIdx <= fs.argIdx {
					break
				}
				fs.argIdx = subFs.argIdx
				if fs.argIdx == prevArgIdx {
					break
				}
				count++
				iterCount++
			}
		} else {
			listArg := fs.popArg()
			elements := seqToList(listArg)
			for i, el := range elements {
				if limit >= 0 && i >= limit {
					break
				}
				rem := len(elements) - i - 1
				subFs := &fmtState{ctrl: body, args: []*Value{el}, remaining: rem}
				formatRun(subFs)
				fs.buf.WriteString(subFs.buf.String())
			}
		}
	case '}':
	case '^':
		if fs.remaining >= 0 {
			// Inside ~{ iteration: check if more items remain
			if fs.remaining == 0 {
				fs.escaped = true
			}
		} else if fs.argIdx >= len(fs.args) {
			fs.escaped = true
		}
	case '<':
		// ~mincol,colinc,minpad,padchar<text~> — Justification
		// ~<seg1~;seg2~;seg3~> — segmented: distribute segments across width
		// Extract body between ~< and ~>
		depth := 1
		bodyStart := fs.pos
		for !fs.done() && depth > 0 {
			c := fs.next()
			if c == '~' && !fs.done() {
				nc := fs.next()
				if nc == '<' {
					depth++
				} else if nc == '>' {
					depth--
				}
			}
		}
		bodyEnd := fs.pos - 2
		if bodyEnd < bodyStart {
			bodyEnd = bodyStart
		}
		body := fs.ctrl[bodyStart:bodyEnd]

		// Check for segments separated by ~; and detect fill separator (~:;)
		segments, fillSegIdx := formatCollectJustifySegments(body)

		if !colon && !at && len(segments) <= 1 {
			// ~<~> without modifiers, no segments: process body and output
			subFs := newFmtState(body, fs.args, fs.argIdx)
			formatRun(subFs)
			fs.argIdx = subFs.argIdx
			fs.buf.WriteString(subFs.buf.String())
		} else if len(segments) > 1 {
			// Segmented justification: process each segment and distribute
			// padding between them to fill mincol
			mincol := fs.getParam(params, 0, 0)
			colinc := fs.getParam(params, 1, 1)
			padchar := fs.getCharParam(params, 3, ' ')

			// Process each segment
			processedSegs := make([]string, 0, len(segments))
			for _, seg := range segments {
				subFs := newFmtState(seg, fs.args, fs.argIdx)
				formatRun(subFs)
				fs.argIdx = subFs.argIdx
				processedSegs = append(processedSegs, subFs.buf.String())
			}

			totalContentLen := 0
			for _, s := range processedSegs {
				totalContentLen += len(s)
			}

			// Calculate total padding needed
			numGaps := len(processedSegs) - 1
			if numGaps <= 0 {
				numGaps = 1
			}
			totalPadLen := 0
			if int(mincol) > totalContentLen {
				totalPadLen = int(mincol) - totalContentLen
				if int(colinc) > 1 && numGaps > 0 {
					// Round up totalPadLen to a multiple of colinc
					for totalPadLen%int(colinc) != 0 && totalPadLen+totalContentLen < int(mincol)+int(colinc) {
						totalPadLen++
					}
				}
			}

			// Distribute padding between gaps
			gapPad := make([]int, numGaps)
			remaining := totalPadLen
			if fillSegIdx >= 0 && fillSegIdx < numGaps {
				// Fill section: all extra padding goes to the gap after fill separator
				for i := 0; i < numGaps; i++ {
					if i == fillSegIdx {
						gapPad[i] = remaining
					} else {
						gapPad[i] = 0
					}
				}
			} else {
				// Distribute evenly
				base := 0
				if numGaps > 0 {
					base = remaining / numGaps
				}
				extra := remaining - base*numGaps
				for i := 0; i < numGaps; i++ {
					gapPad[i] = base
					if i < extra {
						gapPad[i]++
					}
				}
			}

			// Output segments with padding
			for i, s := range processedSegs {
				fs.buf.WriteString(s)
				if i < numGaps {
					for j := 0; j < gapPad[i]; j++ {
						fs.buf.WriteByte(byte(padchar))
					}
				}
			}
		} else {
			// ~:@<~> or ~@<~> or ~:<~>: single-segment justification
			mincol := fs.getParam(params, 0, 0)
			colinc := fs.getParam(params, 1, 1)
			minpad := fs.getParam(params, 2, 0)
			padchar := fs.getCharParam(params, 3, ' ')

			subFs := newFmtState(body, fs.args, fs.argIdx)
			formatRun(subFs)
			fs.argIdx = subFs.argIdx
			s := subFs.buf.String()

			// Calculate padding to reach mincol
			padlen := 0
			if int(mincol) > len(s) {
				padlen = int(mincol) - len(s)
				if int(minpad) > padlen {
					padlen = int(minpad)
				}
				if int(colinc) > 1 {
					for padlen < int(mincol)-len(s) || (padlen-int(minpad))%int(colinc) != 0 {
						padlen++
					}
				}
			}

			if colon && at {
				// ~:@< : center (pad on both sides equally)
				leftPad := padlen / 2
				rightPad := padlen - leftPad
				for i := 0; i < leftPad; i++ {
					fs.buf.WriteByte(byte(padchar))
				}
				fs.buf.WriteString(s)
				for i := 0; i < rightPad; i++ {
					fs.buf.WriteByte(byte(padchar))
				}
			} else if at {
				// ~@< : right-justify (pad on left)
				for i := 0; i < padlen; i++ {
					fs.buf.WriteByte(byte(padchar))
				}
				fs.buf.WriteString(s)
			} else {
				// ~:< : left-justify (pad on right)
				fs.buf.WriteString(s)
				for i := 0; i < padlen; i++ {
					fs.buf.WriteByte(byte(padchar))
				}
			}
		}
	case '>':
	case 'P':
		n := int(toNum(fs.popArg()))
		if at {
			if n != 1 {
				fs.buf.WriteString("ies")
			} else {
				fs.buf.WriteString("y")
			}
		} else if colon {
			if n != 1 {
				fs.buf.WriteString("es")
			}
		} else {
			if n != 1 {
				fs.buf.WriteString("s")
			}
		}
	case 'G':
		// General format: scientific for very large/small, fixed otherwise
		digits := fs.getParam(params, 0, 6)
		if digits < 1 {
			digits = 1
		}
		arg := fs.popArg()
		f := toNum(arg)
		if f == 0 {
			fs.buf.WriteString("0.0")
		} else {
			absF := math.Abs(f)
			// Calculate exponent for determining format
			exp := int(math.Floor(math.Log10(absF)))
			// CL ~g: use fixed-point when -4 <= exp < digits
			if absF >= 1e16 || (absF < 1e-3 && absF > 0) || exp >= digits || exp < -4 {
				// Use exponential format with ~E-like logic
				// Format as mantissa + E + exponent, stripping trailing zeros
				prec := digits - 1
				if prec < 0 {
					prec = 0
				}
				s := strconv.FormatFloat(f, 'E', prec, 64)
				idx := strings.Index(s, "E")
				if idx < 0 {
					fs.buf.WriteString(s)
				} else {
					mantissa := s[:idx]
					expStr := s[idx+1:]
					// Strip trailing zeros from mantissa
					mantissa = strings.TrimRight(mantissa, "0")
					// If mantissa ends with '.', remove it
					mantissa = strings.TrimSuffix(mantissa, ".")
					// If mantissa is empty, use "0"
					if mantissa == "" {
						mantissa = "0"
					}
					// Ensure mantissa has a decimal point
					if !strings.Contains(mantissa, ".") {
						mantissa += ".0"
					}
					// Normalize exponent: strip leading zeros
					expSign := ""
					if len(expStr) > 0 && (expStr[0] == '+' || expStr[0] == '-') {
						expSign = string(expStr[0])
						expStr = expStr[1:]
					}
					expDigits := strings.TrimLeft(expStr, "0")
					if expDigits == "" {
						expDigits = "0"
					}
					fs.buf.WriteString(mantissa + "E" + expSign + expDigits)
				}
			} else {
				// Use fixed-point format with appropriate decimal places
				decPlaces := digits - 1 - exp
				if decPlaces < 0 {
					decPlaces = 0
				}
				s := strconv.FormatFloat(f, 'f', decPlaces, 64)
				// Strip unnecessary trailing zeros, but keep at least one decimal digit
				if dotIdx := strings.Index(s, "."); dotIdx >= 0 {
					// Remove trailing zeros after decimal point
					s = strings.TrimRight(s, "0")
					// If only '.' remains, add one '0' to keep "42."
					s = strings.TrimSuffix(s, ".")
					if s == "" || !strings.Contains(s, ".") {
						s += ".0"
					}
				} else {
					// No decimal point - add ".0" for floating-point clarity
					s += ".0"
				}
				fs.buf.WriteString(s)
			}
		}
	case 'W':
		// ~W: write format - use writeToString for canonical output with escape
		// ~:W uses princ output (no escape), ~@W uses print (with newline), ~:@W uses princ + newline
		arg := fs.popArg()
		if colon && at {
			fs.buf.WriteString(princToString(arg))
			fs.buf.WriteByte('\n')
		} else if colon {
			fs.buf.WriteString(princToString(arg))
		} else if at {
			fs.buf.WriteString(writeToString(arg))
			fs.buf.WriteByte('\n')
		} else {
			fs.buf.WriteString(writeToString(arg))
		}
	case '_':
		// Conditional newline: output a newline (simplified)
		fs.buf.WriteByte('\n')
	case 'I':
		// Indent: output spaces for pretty-printing
		n := fs.getParam(params, 0, 0)
		for i := 0; i < n; i++ {
			fs.buf.WriteByte(' ')
		}
	case '/':
		// ~/function-name/ — call a user-defined function to format
		var fnName strings.Builder
		for !fs.done() {
			c := fs.next()
			if c == '/' {
				break
			}
			fnName.WriteByte(c)
		}
		name := fnName.String()
		fnVal, fnErr := globalEnv.Get(strings.ToUpper(name))
		if fnErr != nil || fnVal == nil || (fnVal.typ != VPrim && fnVal.typ != VFunc && fnVal.typ != VGeneric) {
			fs.buf.WriteString("?")
		} else {
			arg := fs.popArg()
			colVal := vbool(colon)
			atVal := vbool(at)
			callArgs := []*Value{arg, colVal, atVal}
			for _, p := range params {
				if f64, ok := p.(float64); ok {
					callArgs = append(callArgs, vnum(f64))
				} else {
					callArgs = append(callArgs, vnum(0))
				}
			}
			result, _ := callFnOnSeq(fnVal, callArgs, nil)
			if result != nil {
				fs.buf.WriteString(princToString(result))
			}
		}
	}
}

// formatCollectJustifySegments splits a ~<...~> body by ~; separators,
// respecting nested directives. Returns segments and the index of the
// gap that follows the ~:; fill separator (-1 if none).
func formatCollectJustifySegments(body string) ([]string, int) {
	var segments []string
	fillGapIdx := -1
	depth := 0
	start := 0
	pos := 0
	for pos < len(body) {
		if body[pos] == '~' && pos+1 < len(body) {
			nc := body[pos+1]
			if nc == '<' {
				depth++
				pos += 2
			} else if nc == '>' {
				depth--
				pos += 2
			} else if nc == ':' && pos+2 < len(body) && body[pos+2] == ';' && depth == 0 {
				// ~:; fill separator
				segments = append(segments, body[start:pos])
				fillGapIdx = len(segments) // gap after this segment gets fill padding
				pos += 3
				start = pos
			} else if nc == ';' && depth == 0 {
				// ~; regular separator
				segments = append(segments, body[start:pos])
				pos += 2
				start = pos
			} else {
				pos += 2
			}
		} else {
			pos++
		}
	}
	if start < len(body) {
		segments = append(segments, body[start:])
	}
	return segments, fillGapIdx
}

func formatCollectSections(fs *fmtState, endChar byte) []string {
	var sections []string
	depth := 0
	start := fs.pos
	for !fs.done() {
		if fs.peek() == '~' {
			fs.next()
			if fs.done() {
				break
			}
			nc := fs.peek()
			if nc == '[' {
				depth++
				fs.next()
			} else if nc == ']' {
				if depth == 0 {
					sections = append(sections, fs.ctrl[start:fs.pos-1])
					fs.next()
					return sections
				}
				depth--
				fs.next()
			} else if nc == ':' {
				// Check for ~:; (default clause marker)
				pos := fs.pos
				if pos+1 < len(fs.ctrl) && fs.ctrl[pos+1] == ';' && depth == 0 {
					// ~:; — mark everything before this as section,
					// everything after as default, skip the next ~;
					sections = append(sections, fs.ctrl[start:fs.pos-1])
					fs.next() // consume ':'
					fs.next() // consume ';'
					start = fs.pos
					// Now continue collecting, but when we see the next ~; at depth 0,
					// skip it (it's the paired ~; with ~:)
					for !fs.done() {
						if fs.peek() == '~' {
							fs.next()
							if fs.done() {
								break
							}
							nc2 := fs.peek()
							if nc2 == '[' {
								depth++
								fs.next()
							} else if nc2 == ']' {
								if depth == 0 {
									sections = append(sections, fs.ctrl[start:fs.pos-1])
									fs.next()
									return sections
								}
								depth--
								fs.next()
							} else if nc2 == '{' {
								depth++
								fs.next()
							} else if nc2 == '}' {
								depth--
								fs.next()
							} else if nc2 == ';' && depth == 0 {
								// Skip the paired ~; — this is the second half of ~:;
								fs.next()
								start = fs.pos
							} else {
								fs.next()
							}
						} else {
							fs.next()
						}
					}
					sections = append(sections, fs.ctrl[start:fs.pos])
					return sections
				}
				// Not ~:; just consume
				fs.next()
			} else if nc == ';' && depth == 0 {
				sections = append(sections, fs.ctrl[start:fs.pos-1])
				fs.next()
				start = fs.pos
			} else if nc == '{' {
				depth++
				fs.next()
			} else if nc == '}' {
				depth--
				fs.next()
			} else {
				fs.next()
			}
		} else {
			fs.next()
		}
	}
	sections = append(sections, fs.ctrl[start:fs.pos])
	return sections
}

func formatCollectBody(fs *fmtState, endChar byte) string {
	depth := 0
	start := fs.pos
	for !fs.done() {
		if fs.peek() == '~' {
			fs.next()
			if fs.done() {
				break
			}
			nc := fs.next()
			if nc == '{' {
				depth++
			} else if nc == '}' {
				if depth == 0 {
					return fs.ctrl[start : fs.pos-2]
				}
				depth--
			} else if nc == '[' {
				depth++
			} else if nc == ']' {
				depth--
			}
		} else {
			fs.next()
		}
	}
	return fs.ctrl[start:fs.pos]
}

func princToString(v *Value) string {
	switch v.typ {
	case VBigInt:
		return v.bigInt.String()
	case VStr:
		return v.str
	case VChar:
		return string(v.ch)
	case VNil:
		return "nil"
	case VBool:
		if v == globalEnv.bindings["#t"] {
			return "#t"
		}
		return "#f"
	case VPair, VArray:
		// Use writeToString for circular reference detection
		return writeToString(v)
	case VInstance:
		// Check for defstruct :print-function / :print-object first
		if v.instClass != nil {
			if printFn, ok := structPrintFns[strings.ToUpper(v.instClass.str)]; ok && printFn != nil {
				// Create a string output stream and call the print function
				outStream := newStringOutput()
				_, callErr := callFnOnSeq(printFn, []*Value{v, outStream, vnum(0)}, globalEnv)
				if callErr == nil {
					return outStream.stream.strBuf.String()
				}
				// If print function fails, fall through to default
			}
		}
		// Check if this is a condition instance with format-control/format-arguments
		if v.instClass != nil && classHasAncestor(v.instClass, "condition") {
			fc := instanceSlotWithInheritance(v, "format-control")
			fa := instanceSlotWithInheritance(v, "format-arguments")
			if fc != nil && fc.typ == VStr && len(fc.str) > 0 {
				var args []*Value
				if fa != nil && fa.typ != VNil {
					cur := fa
					for cur.typ == VPair {
						args = append(args, cur.car)
						cur = cur.cdr
					}
				}
				return formatMessage(fc.str, args)
			}
		}
		return writeToString(v)
	default:
		return ToString(v)
	}
}

// -------- CLOS --------

// c3Linearize computes the C3 class precedence list
// parents is a list of VClass values in declaration order
func c3Linearize(c *Value, parents []*Value) []*Value {
	if len(parents) == 0 {
		return []*Value{c}
	}
	// Build the merge lists: each parent's CPL + the direct parents list
	lists := make([][]*Value, 0, len(parents)+1)
	for _, p := range parents {
		if p.typ == VClass {
			lists = append(lists, p.cpl)
		}
	}
	// Add direct parents as a list
	direct := make([]*Value, len(parents))
	copy(direct, parents)
	lists = append(lists, direct)

	result := []*Value{c}
	// C3 merge
	for {
		// Find a candidate: first element of any list that's not in the tail of any list
		candidate := -1
		for i, lst := range lists {
			if len(lst) == 0 {
				continue
			}
			cand := lst[0]
			inTail := false
			for j, lst2 := range lists {
				if j == i || len(lst2) <= 1 {
					continue
				}
				for k := 1; k < len(lst2); k++ {
					if lst2[k] == cand {
						inTail = true
						break
					}
				}
				if inTail {
					break
				}
			}
			if !inTail {
				candidate = i
				break
			}
		}
		if candidate < 0 {
			// All lists consumed or inconsistency
			break
		}
		cand := lists[candidate][0]
		result = append(result, cand)
		// Remove cand from all lists
		for i := range lists {
			if len(lists[i]) > 0 && lists[i][0] == cand {
				lists[i] = lists[i][1:]
			}
		}
	}
	return result
}



func lispToReflect(v *Value, t reflect.Type) reflect.Value {
	switch v.typ {
	case VNum:
		switch t.Kind() {
		case reflect.Float64:
			return reflect.ValueOf(v.num)
		case reflect.Float32:
			return reflect.ValueOf(float32(v.num))
		case reflect.Int:
			return reflect.ValueOf(int(v.num))
		case reflect.Int64:
			return reflect.ValueOf(int64(v.num))
		case reflect.Uint:
			return reflect.ValueOf(uint(v.num))
		default:
			return reflect.ValueOf(v.num)
		}
	case VRat:
		f := float64(v.irat) / float64(v.iden)
		switch t.Kind() {
		case reflect.Float64:
			return reflect.ValueOf(f)
		case reflect.Float32:
			return reflect.ValueOf(float32(f))
		case reflect.Int:
			return reflect.ValueOf(int(f))
		case reflect.Int64:
			return reflect.ValueOf(int64(f))
		default:
			return reflect.ValueOf(f)
		}
	case VBigInt:
		if v.bigInt != nil {
			f, _ := new(big.Float).SetInt(v.bigInt).Float64()
			return reflect.ValueOf(f)
		}
		return reflect.ValueOf(0.0)
	case VComplex:
		return reflect.ValueOf(v.num)
	case VChar:
		return reflect.ValueOf(string(v.ch))
	case VStr:
		return reflect.ValueOf(v.str)
	case VBool:
		return reflect.ValueOf(v == globalEnv.bindings["#t"])
	default:
		return reflect.ValueOf(ToString(v))
	}
}

func reflectToLisp(v reflect.Value) *Value {
	switch v.Kind() {
	case reflect.Float64, reflect.Float32:
		return vnum(v.Float())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return vnum(float64(v.Int()))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return vnum(float64(v.Uint()))
	case reflect.String:
		return vstr(v.String())
	case reflect.Bool:
		return vbool(v.Bool())
	case reflect.Slice:
		if v.Type().Elem().Kind() == reflect.Uint8 {
			return vstr(string(v.Bytes()))
		}
		return vnil()
	default:
		return vstr(fmt.Sprint(v.Interface()))
	}
}

// -------- Printer --------
func ToString(v *Value) string {
	if v == nil {
		return "()"
	}
	switch v.typ {
	case VNil:
		return "()"
	case VNum:
		f := v.num
		if v.isFloat {
			// explicitly a float: always print with decimal point
			s := strconv.FormatFloat(f, 'g', -1, 64)
			if !strings.ContainsAny(s, ".eE") {
				s += ".0"
			}
			return s
		}
		if f == math.Trunc(f) && !math.IsInf(f, 0) && math.Abs(f) < 1e15 {
			return strconv.FormatInt(int64(f), 10)
		}
		return strconv.FormatFloat(f, 'g', -1, 64)
	case VStr:
		return `"` + strings.ReplaceAll(v.str, `"`, `\"`) + `"`
	case VSym:
		return v.str
	case VBool:
		if v == globalEnv.bindings["#t"] {
			return "#t"
		}
		return "#f"
	case VPair:
		return listToString(v)
	case VPrim:
		return "#<primitive>"
	case VFunc:
		return "#<procedure>"
	case VMacro:
		return "#<macro>"
	case VRat:
		return strconv.FormatInt(v.irat, 10) + "/" + strconv.FormatInt(v.iden, 10)
	case VComplex:
		r := formatComplexPart(v.num, v.isFloat)
		i := formatComplexPart(v.imag, v.isFloat)
		return "#c(" + r + " " + i + ")"
	case VBigInt:
		return v.bigInt.String()
	case VPathname:
		return "#P\"" + pathnameToString(v.pathname) + "\""
	case VPackage:
		if v.pkg != nil {
			return "#<PACKAGE " + v.pkg.name + ">"
		}
		return "#<PACKAGE>"
	case VReadtable:
		return "#<READTABLE>"
	case VClass:
		return "#<class " + v.str + ">"
	case VGeneric:
		return "#<generic " + v.str + ">"
	case VInstance:
		if v.instClass != nil {
			if printFn, ok := structPrintFns[strings.ToUpper(v.instClass.str)]; ok && printFn != nil {
				outStream := newStringOutput()
				_, callErr := callFnOnSeq(printFn, []*Value{v, outStream, vnum(0)}, globalEnv)
				if callErr == nil {
					return outStream.stream.strBuf.String()
				}
			}
			return "#<instance " + v.instClass.str + ">"
		}
		return "#<instance>"
	case VVHash:
		return "#<hash-table " + strconv.Itoa(v.hashTab.count) + ">"
	case VThread:
		return "#<thread " + strconv.FormatInt(int64(v.num), 10) + ">"
	case VLock:
		return "#<lock " + strconv.FormatInt(int64(v.num), 10) + ">"
	case VChar:
		switch v.ch {
		case ' ':
			return "#\\space"
		case '\n':
			return "#\\newline"
		case '\t':
			return "#\\tab"
		case '\r':
			return "#\\return"
		case '\x08':
			return "#\\backspace"
		case '\x7f':
			return "#\\rubout"
		case '\f':
			return "#\\page"
		case '\x00':
			return "#\\null"
		case '\x07':
			return "#\\bell"
		case '\x1b':
			return "#\\escape"
		case '\x01':
			return "#\\soh"
		case '\x02':
			return "#\\stx"
		case '\x03':
			return "#\\etx"
		case '\x04':
			return "#\\eot"
		case '\x05':
			return "#\\enq"
		case '\x06':
			return "#\\ack"
		case '\x15':
			return "#\\nak"
		case '\x16':
			return "#\\syn"
		case '\x17':
			return "#\\etb"
		case '\x18':
			return "#\\can"
		case '\x19':
			return "#\\em"
		case '\x1a':
			return "#\\sub"
		case '\x1c':
			return "#\\fs"
		case '\x1d':
			return "#\\gs"
		case '\x1e':
			return "#\\rs"
		case '\x1f':
			return "#\\us"
		case '\x11':
			return "#\\xon"
		case '\x13':
			return "#\\xoff"
		default:
			return "#\\" + string(v.ch)
		}
	case VStream:
		return "#<stream>"
	case VArray:
		return arrayToString(v)
	case VMultiVal:
		return listToString(cons(v.car, v.cdr))
	case VRandomState:
		return "#<random-state>"
	case VMethod:
		return "#<method " + v.str + ">"
	case VRestart:
		return "#<restart " + v.str + ">"
	default:
		return "#<unknown>"
	}
}

// -------- Circular structure printing (*print-circle*) --------

// circleState tracks visited values for circular reference detection
type circleState struct {
	seen    map[*Value]int // value -> label number
	counter int
}

// writeToString returns a string representation respecting *print-circle*
func writeToString(v *Value) string {
	pc, _ := globalEnv.Get("*print-circle*")
	useCircle := pc != nil && !isNil(pc)
	// Always detect circular structures to prevent infinite loops
	cs := &circleState{seen: make(map[*Value]int), counter: 1}
	findShared(v, cs, make(map[*Value]bool))
	if useCircle && len(cs.seen) > 0 {
		// Print with #n= and #n# labels
		return toStringCircle(v, cs)
	}
	if len(cs.seen) > 0 {
		// Circular but *print-circle* is nil — truncate with ...
		return toStringSafe(v, cs)
	}
	return ToString(v)
}

// toStringSafe prints with ... for circular references (when *print-circle* is nil)
func toStringSafe(v *Value, cs *circleState) string {
	if v == nil {
		return "()"
	}
	switch v.typ {
	case VNil:
		return "()"
	case VPair:
		if _, ok := cs.seen[v]; ok {
			return "..."
		}
		return listToStringSafe(v, cs)
	case VArray:
		if _, ok := cs.seen[v]; ok {
			return "..."
		}
		return arrayToStringSafe(v, cs)
	default:
		return ToString(v)
	}
}

func listToStringSafe(v *Value, cs *circleState) string {
	var b strings.Builder
	b.WriteString("(")
	first := true
	localSeen := make(map[*Value]bool)
	for !isNil(v) {
		if v.typ != VPair {
			b.WriteString(" . ")
			b.WriteString(toStringSafe(v, cs))
			break
		}
		if _, ok := cs.seen[v]; ok && !first {
			b.WriteString("...")
			break
		}
		if localSeen[v] {
			b.WriteString("...")
			break
		}
		localSeen[v] = true
		if !first {
			b.WriteString(" ")
		}
		first = false
		b.WriteString(toStringSafe(v.car, cs))
		v = v.cdr
	}
	b.WriteString(")")
	return b.String()
}

func arrayToStringSafe(v *Value, cs *circleState) string {
	if v.array == nil {
		return "#()"
	}
	var b strings.Builder
	b.WriteString("#(")
	for i, elem := range v.array.elements {
		if i > 0 {
			b.WriteString(" ")
		}
		b.WriteString(toStringSafe(elem, cs))
	}
	b.WriteString(")")
	return b.String()
}

// findShared walks the structure and marks values referenced more than once (or circularly)
func findShared(v *Value, cs *circleState, visited map[*Value]bool) {
	if v == nil || v.typ == VNil {
		return
	}
	if v.typ == VPair {
		if visited[v] {
			// Already visited — mark as shared/circular
			if _, exists := cs.seen[v]; !exists {
				cs.seen[v] = cs.counter
				cs.counter++
			}
			return // Don't recurse into already-visited
		}
		visited[v] = true
		findShared(v.car, cs, visited)
		findShared(v.cdr, cs, visited)
	} else if v.typ == VArray && v.array != nil {
		if visited[v] {
			if _, exists := cs.seen[v]; !exists {
				cs.seen[v] = cs.counter
				cs.counter++
			}
			return
		}
		visited[v] = true
		for _, elem := range v.array.elements {
			findShared(elem, cs, visited)
		}
	} else if v.typ == VInstance {
		if visited[v] {
			if _, exists := cs.seen[v]; !exists {
				cs.seen[v] = cs.counter
				cs.counter++
			}
			return
		}
		visited[v] = true
		for _, slot := range v.instSlots {
			findShared(slot, cs, visited)
		}
	}
}

// toStringCircle prints with #n= and #n# syntax for circular/shared references
func toStringCircle(v *Value, cs *circleState) string {
	if v == nil {
		return "()"
	}
	switch v.typ {
	case VNil:
		return "()"
	case VPair:
		if label, ok := cs.seen[v]; ok {
			// Check if already printed (replace the entry with negative to mark)
			if label < 0 {
				return "#" + strconv.Itoa(-label) + "#"
			}
			cs.seen[v] = -label // mark as printed
			prefix := "#" + strconv.Itoa(label) + "="
			return prefix + listToStringCircle(v, cs)
		}
		return listToStringCircle(v, cs)
	case VArray:
		if label, ok := cs.seen[v]; ok {
			if label < 0 {
				return "#" + strconv.Itoa(-label) + "#"
			}
			cs.seen[v] = -label
			prefix := "#" + strconv.Itoa(label) + "="
			return prefix + arrayToStringCircle(v, cs)
		}
		return arrayToStringCircle(v, cs)
	case VInstance:
		if label, ok := cs.seen[v]; ok {
			if label < 0 {
				return "#" + strconv.Itoa(-label) + "#"
			}
			cs.seen[v] = -label
			prefix := "#" + strconv.Itoa(label) + "="
			return prefix + ToString(v)
		}
		return ToString(v)
	default:
		return ToString(v)
	}
}

func listToStringCircle(v *Value, cs *circleState) string {
	var b strings.Builder
	b.WriteString("(")
	for !isNil(v) {
		if v.typ != VPair {
			b.WriteString(" . ")
			b.WriteString(toStringCircle(v, cs))
			break
		}
		// Check cdr for circular reference before recursing
		if !isNil(v.cdr) {
			if label, ok := cs.seen[v.cdr]; ok {
				// cdr is shared — print (a b . #n#) style
				b.WriteString(toStringCircle(v.car, cs))
				b.WriteString(" . ")
				cs.seen[v.cdr] = -absInt(label) // ensure negative for #n#
				b.WriteString(toStringCircle(v.cdr, cs))
				b.WriteString(")")
				return b.String()
			}
		}
		b.WriteString(toStringCircle(v.car, cs))
		v = v.cdr
		if !isNil(v) {
			b.WriteString(" ")
		}
	}
	b.WriteString(")")
	return b.String()
}

func arrayToStringCircle(v *Value, cs *circleState) string {
	if v.array == nil {
		return "#()"
	}
	var b strings.Builder
	b.WriteString("#(")
	for i, elem := range v.array.elements {
		if i > 0 {
			b.WriteString(" ")
		}
		b.WriteString(toStringCircle(elem, cs))
	}
	b.WriteString(")")
	return b.String()
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func listToString(v *Value) string {
	if v == nil || v.typ != VPair {
		return "()"
	}
	var b strings.Builder
	b.WriteString("(")
	seen := make(map[*Value]bool)
	for !isNil(v) {
		if seen[v] {
			b.WriteString("...")
			break
		}
		seen[v] = true
		if v.typ != VPair {
			b.WriteString(" . ")
			b.WriteString(toStringWithSeen(v, seen))
			break
		}
		b.WriteString(toStringWithSeen(v.car, seen))
		v = v.cdr
		if !isNil(v) {
			b.WriteString(" ")
		}
	}
	b.WriteString(")")
	return b.String()
}

// toStringWithSeen is like toString but shares a seen map for cycle detection
func toStringWithSeen(v *Value, seen map[*Value]bool) string {
	if v == nil {
		return "()"
	}
	switch v.typ {
	case VNil:
		return "()"
	case VPair:
		if seen[v] {
			return "..."
		}
		// Don't add v to seen here; listToString will do it
		return listToStringShared(v, seen)
	default:
		return ToString(v)
	}
}

func listToStringShared(v *Value, seen map[*Value]bool) string {
	if v == nil || v.typ != VPair {
		return "()"
	}
	var b strings.Builder
	b.WriteString("(")
	for !isNil(v) {
		if seen[v] {
			b.WriteString("...")
			break
		}
		seen[v] = true
		if v.typ != VPair {
			b.WriteString(" . ")
			b.WriteString(toStringWithSeen(v, seen))
			break
		}
		b.WriteString(toStringWithSeen(v.car, seen))
		v = v.cdr
		if !isNil(v) {
			b.WriteString(" ")
		}
	}
	b.WriteString(")")
	return b.String()
}

// -------- File loading --------
func loadFile(fname string, env *Env) (*Value, error) {
	data, err := os.ReadFile(fname)
	if err != nil {
		return nil, fmt.Errorf("load: %v", err)
	}
	// Save old values of *load-pathname* and *load-truename*
	oldPathname, _ := env.Get("*load-pathname*")
	oldTruename, _ := env.Get("*load-truename*")
	// Set *load-pathname* to the pathname of the file being loaded
	absPath, _ := filepath.Abs(fname)
	env.Set("*load-pathname*", vpathname(parsePathnameString(absPath)))
	// Set *load-truename* to the truename (resolved absolute path)
	env.Set("*load-truename*", vpathname(parsePathnameString(absPath)))
	// Evaluate the file contents
	result, evalErr := EvalString(string(data), env)
	// Restore old values
	if oldPathname != nil {
		env.Set("*load-pathname*", oldPathname)
	} else {
		env.Set("*load-pathname*", vnil())
	}
	if oldTruename != nil {
		env.Set("*load-truename*", oldTruename)
	} else {
		env.Set("*load-truename*", vnil())
	}
	return result, evalErr
}

func builtinMakeThread(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("make-thread: need a function")
	}
	fn := args[0]
	fnArgs := args[1:]

	tid := atomic.AddInt64(&nextThreadID, 1)
	resultCh := make(chan threadResult, 1)

	threadChannelsMu.Lock()
	threadChannels[tid] = resultCh
	threadChannelsMu.Unlock()

	go func() {
		threadEnv := copyGlobalEnv()
		argList := listFromSlice(fnArgs)
		result, err := Apply(fn, argList, threadEnv)
		resultCh <- threadResult{value: result, err: err}
	}()

	return &Value{typ: VThread, num: float64(tid)}, nil
}

func copyGlobalEnv() *Env {
	env := NewEnv(nil)
	for k, v := range globalEnv.bindings {
		env.bindings[k] = v
	}
	return env
}

func builtinJoinThread(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VThread {
		return nil, fmt.Errorf("join-thread: need a thread")
	}
	tid := int64(args[0].num)
	threadChannelsMu.Lock()
	ch, ok := threadChannels[tid]
	threadChannelsMu.Unlock()
	if !ok {
		return nil, fmt.Errorf("join-thread: no such thread %d", tid)
	}
	tr := <-ch
	if tr.err != nil {
		return nil, tr.err
	}
	threadChannelsMu.Lock()
	delete(threadChannels, tid)
	threadChannelsMu.Unlock()
	return tr.value, nil
}

var nextLockID int64
var atomicCounter int64
var lockMutexMap = make(map[int64]*sync.Mutex)
var lockMapMu sync.Mutex
var condMu sync.Mutex
var condVars = make(map[int64]*sync.Cond)
var nextCondID int64

func builtinMakeLock(args []*Value) (*Value, error) {
	lid := atomic.AddInt64(&nextLockID, 1)
	lockMapMu.Lock()
	lockMutexMap[lid] = &sync.Mutex{}
	lockMapMu.Unlock()
	return &Value{typ: VLock, num: float64(lid)}, nil
}

func builtinLock(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VLock {
		return nil, fmt.Errorf("lock: need a lock object")
	}
	lid := int64(args[0].num)
	lockMapMu.Lock()
	mu, ok := lockMutexMap[lid]
	lockMapMu.Unlock()
	if !ok {
		return nil, fmt.Errorf("lock: invalid lock")
	}
	mu.Lock()
	return vnil(), nil
}

func builtinUnlock(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VLock {
		return nil, fmt.Errorf("unlock: need a lock object")
	}
	lid := int64(args[0].num)
	lockMapMu.Lock()
	mu, ok := lockMutexMap[lid]
	lockMapMu.Unlock()
	if !ok {
		return nil, fmt.Errorf("unlock: invalid lock")
	}
	mu.Unlock()
	return vnil(), nil
}

func builtinSleep(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VNum {
		return nil, fmt.Errorf("sleep: need a number of seconds")
	}
	secs := args[0].num
	duration := time.Duration(secs * float64(time.Second))
	time.Sleep(duration)
	return vnil(), nil
}

func builtinValues(args []*Value) (*Value, error) {
	// values returns a VMultiVal wrapping all arguments.
	// Primary value (car) is the first argument, or nil if none.
	v := gcv()
	v.typ = VMultiVal
	v.cdr = vnil()
	if len(args) > 0 {
		v.car = args[0]
		v.cdr = list(args[1:]...)
	}
	return v, nil
}

func builtinValuesList(args []*Value) (*Value, error) {
	// values-list: converts a list to multiple values.
	// (values-list '(a b c)) => values a b c
	if len(args) != 1 {
		return nil, fmt.Errorf("values-list: need exactly 1 argument")
	}
	lst := args[0]
	if isNil(lst) {
		v := gcv()
		v.typ = VMultiVal
		v.car = vnil()
		v.cdr = vnil()
		return v, nil
	}
	v := gcv()
	v.typ = VMultiVal
	v.car = lst.car
	v.cdr = lst.cdr
	return v, nil
}

// -------- Standard Library (embedded) --------
var initLib = `
;; SBCL test harness stub: with-test just evaluates the body
(define-macro (with-test spec . body)
  (cons (quote begin) body))

;; SBCL test harness stubs
(define (enable-test-parallelism) nil)
(define-macro (checked-compile form . rest)
  (list 'eval form))
(define (checked-eval form) (eval form))
(define (ctu:asm-search pattern source) nil)

;; SBCL test utility stubs
(define-macro (assert-error form . type)
  (list 'ignore-errors form))

(define-macro (assert-signal form condition)
  (list 'ignore-errors form))

(define-macro (assert-type fn expected-type)
  nil)

(define-macro (checked-compile-and-assert opts form cases)
  (let ((fn-sym (gensym "cca-")))
    (let ((is-pair (pair? cases))
          (is-nested #f))
      (if is-pair
          (set! is-nested (pair? (car cases)))
          (set! is-nested #f))
      (cons 'let
        (list (list (list fn-sym (list 'eval form)))
              (cons 'begin
                (let ((case-list (if is-nested (car cases) (list cases))))
                  (let ((gen-case (lambda (c)
                                    (if (pair? c)
                                        (let ((inputs (car c))
                                              (expected (if (pair? (cdr c)) (car (cdr c)) nil)))
                                          (if (and (pair? expected) (eq? (car expected) (quote condition)))
                                              (let ((raw-t (if (pair? (cdr expected)) (car (cdr expected)) (quote error))))
                                                (let ((cond-type (if (and (pair? raw-t) (eq? (car raw-t) (quote quote)) (pair? (cdr raw-t)))
                                                                       (car (cdr raw-t))
                                                                       raw-t)))
                                                  (list 'assert
                                                        (list 'handler-case
                                                              (list 'progn
                                                                    (list 'apply fn-sym (list 'quote inputs))
                                                                    (quote nil))
                                                              (list cond-type (list (quote c)) (quote t))
                                                              (list (quote error) (list (quote c)) (quote nil))))))
                                              (if (and (pair? expected) (eq? (car expected) (quote values)))
                                                  (list 'assert
                                                        (list 'equal
                                                              (list 'multiple-value-list
                                                                    (list 'apply fn-sym (list 'quote inputs)))
                                                              (list 'multiple-value-list
                                                                    (list 'quote (cdr expected)))))
                                                  (list 'assert
                                                        (list 'equal
                                                              (list 'apply fn-sym (list 'quote inputs))
                                                              (list 'quote expected))))))
                                        (list 'begin)))))
                    (mapcar gen-case case-list)))))))))

(define (ctu:find-code-constants fn) nil)
(define (ctu:ir1-named-calls fn) nil)
(define-macro (ctu:assert-no-consing form) form)
(define (ctu:find-named-callees fn) nil)
(define (sb-int:encapsulate fn name impl) nil)
(define (sb-int:unencapsulate fn name) nil)
(define (sb-impl::%remf indicator list) (remf list indicator))

(define (not x) (if x #f #t))
(define (caar x) (car (car x)))
(define (cadr x) (car (cdr x)))
(define (cdar x) (cdr (car x)))
(define (cddr x) (cdr (cdr x)))
(define (caaar x) (car (car (car x))))
(define (caadr x) (car (car (cdr x))))
(define (cadar x) (car (cdr (car x))))
(define (caddr x) (car (cdr (cdr x))))
(define (cdaar x) (cdr (car (car x))))
(define (cdadr x) (cdr (car (cdr x))))
(define (cddar x) (cdr (cdr (car x))))
(define (cdddr x) (cdr (cdr (cdr x))))

(define (list . x) x)
(define (filter f lst)
  (if (null? lst) '()
    (if (f (car lst))
      (cons (car lst) (filter f (cdr lst)))
      (filter f (cdr lst)))))
(define (fold f init lst)
  (if (null? lst) init
    (fold f (f (car lst) init) (cdr lst))))
(define (fold-right f init lst)
  (if (null? lst) init
    (f (car lst) (fold-right f init (cdr lst)))))
(define (append . lists)
  (cond
    ((null? lists) '())
    ((null? (cdr lists)) (car lists))
    ((null? (car lists)) (apply append (cdr lists)))
    (else (cons (car (car lists)) (apply append (cons (cdr (car lists)) (cdr lists)))))))
(define (range n)
  (if (<= n 0) '() (append (range (- n 1)) (list (- n 1)))))
(define (list-ref lst n)
  (cond
    ((null? lst) '())
    ((= n 0) (car lst))
    (#t (list-ref (cdr lst) (- n 1)))))
;; member is now a Go builtin with :key/:test support
(define (member? x lst)
  (if (null? lst) #f
    (if (equal? x (car lst)) #t (member? x (cdr lst)))))
(define (any pred lst)
  (cond
    ((null? lst) #f)
    ((pred (car lst)) #t)
    (else (any pred (cdr lst)))))
(define (all pred lst)
  (cond
    ((null? lst) #t)
    ((not (pred (car lst))) #f)
    (else (all pred (cdr lst)))))
(define (take n lst)
  (if (or (<= n 0) (null? lst)) '()
    (cons (car lst) (take (- n 1) (cdr lst)))))
(define (drop n lst)
  (if (or (<= n 0) (null? lst)) lst
    (drop (- n 1) (cdr lst))))
(define (zip a b)
  (if (or (null? a) (null? b)) '()
    (cons (list (car a) (car b)) (zip (cdr a) (cdr b)))))
(define (flatten lst)
  (cond
    ((null? lst) '())
    ((pair? (car lst)) (append (flatten (car lst)) (flatten (cdr lst))))
    (else (cons (car lst) (flatten (cdr lst))))))
(define (square x) (* x x))
(define (modulo n d) (- n (* d (ffi "math/floor" (/ n d)))))
(define (atom x) (not (pair? x)))
(define (list? x) (or (null? x) (pair? x)))
; butlast and nbutlast are implemented as Go builtins (builtinButlast, builtinNbutlast)
; which properly handle both proper and dotted lists.
(define (last lst . n)
  (let ((n (if (null? n) 1 (car n))))
    (drop (max 0 (- (length lst) n)) lst)))
(define (signum n)
  (cond ((> n 0) 1) ((< n 0) -1) (#t 0)))
(define (even? n) (= (modulo n 2) 0))
(define (odd? n) (not (= (modulo n 2) 0)))
(define (zero? n) (= n 0))
(define (positive? n) (> n 0))
(define (negative? n) (< n 0))
(define (nreverse lst) (reverse lst))
(define (nconc . lsts)
  (define (nconc-2 a b)
    (if (null? a) b
      (let ((last-pair (last a)))
        (set-cdr! last-pair b)
        a)))
  (if (null? lsts) '()
    (if (null? (cdr lsts)) (car lsts)
      (nconc-2 (car lsts) (apply nconc (cdr lsts))))))
(define (rassoc key alist . rest)
  (if (null? alist) '()
    (let ((pair (car alist)))
      (if (and (pair? pair) (equal? key (cdr pair))) pair
        (apply rassoc key (cdr alist) rest)))))
(define (union list1 list2 . rest)
  (let ((test (if (null? rest) equal?
                (let ((args rest))
                  (if (and (pair? args) (equal? (car args) :test) (pair? (cdr args)))
                      (cadr args) equal?)))))
    (append list1 (filter (lambda (x) (not (member x list1))) list2))))
(define (intersection list1 list2)
  (filter (lambda (x) (member x list2)) list1))
(define (set-difference list1 list2)
  (filter (lambda (x) (not (member x list2))) list1))
(define (adjoin item list . rest)
  (if (member item list) list (cons item list)))
(define (every pred lst . rest)
  (apply seq-every (cons pred (cons lst rest))))
(define (some pred lst . rest)
  (apply seq-some (cons pred (cons lst rest))))
(define (notany pred lst . rest)
  (apply seq-notany (cons pred (cons lst rest))))
(define (notevery pred lst . rest)
  (apply seq-notevery (cons pred (cons lst rest))))

;; -------- CL macros: cond, case, typecase, prog1, push, pop, incf, decf --------
(define-macro (cond . clauses)
  (if (null? clauses) #f
    (if (eq? (caar clauses) 'else)
      (cons 'begin (cdar clauses))
      (list 'if (caar clauses)
        (cons 'begin (cdar clauses))
        (cons 'cond (cdr clauses))))))

(define-macro (prog1 first . rest)
  (let ((v (gensym)))
    (list 'let (list (list v first))
      (cons 'begin (append rest (list v))))))

(define-macro (prog2 first second . rest)
  (list 'begin first (cons 'prog1 (cons second rest))))

;; prog: (prog (varlist) . body) => (block nil (let (bindings) (tagbody . body)))
;; Each var in varlist can be: var, (var), or (var init)
(define-macro (prog varlist . body)
  (let ((bindings (mapcar (lambda (v)
                            (if (pair? v)
                              (if (null? (cdr v)) (list (car v) nil) v)
                              (list v nil)))
                          varlist)))
    (list 'block 'nil (list 'let bindings (cons 'tagbody body)))))

;; prog*: (prog* (varlist) . body) => (block nil (let* (bindings) (tagbody . body)))
(define-macro (prog* varlist . body)
  (let ((bindings (mapcar (lambda (v)
                            (if (pair? v)
                              (if (null? (cdr v)) (list (car v) nil) v)
                              (list v nil)))
                          varlist)))
    (list 'block 'nil (list 'let* bindings (cons 'tagbody body)))))

(define-macro (push item place)
  (list 'setf place (list 'cons item place)))

(define-macro (pop place)
  (let ((v (gensym)))
    (list 'let (list (list v place))
      (list 'setf place (list 'cdr v))
      (list 'car v))))

(define-macro (pushnew item place)
  (list 'if (list 'member item place)
    place
    (list 'setf place (list 'cons item place))))

(define-macro (incf place . delta)
  (if (null? delta)
    (list 'setf place (list '+ place 1))
    (let ((g (gensym)))
      (list 'let (list (list g (car delta)))
        (list 'setf place (list '+ place g))))))

(define-macro (decf place . delta)
  (if (null? delta)
    (list 'setf place (list '- place 1))
    (let ((g (gensym)))
      (list 'let (list (list g (car delta)))
        (list 'setf place (list '- place g))))))

(define-macro (rotatef . places)
  (if (null? places) #f
    (if (null? (cdr places)) (car places)
      (let ((g (mapcar (lambda (_) (gensym)) places)))
        (list 'let (mapcar list g places)
          (cons 'begin
            (append
              (mapcar (lambda (p v) (list 'setf p v))
                   places
                   (append (cdr g) (list (car g))))
              (list (car g)))))))))

(define-macro (shiftf . args)
  (if (< (length args) 2)
    (error "shiftf: need at least 2 args")
    (let ((places (butlast args 1))
          (newval (car (last-pair args)))
          (g (mapcar (lambda (_) (gensym)) (butlast args 1))))
      (list 'let (mapcar list g (butlast args 1))
        (cons 'begin
          (append
            (mapcar (lambda (p v) (list 'setf p v))
                 places
                 (append (cdr g) (list newval)))
            (list (car g))))))))

(define-macro (psetq . pairs)
  (let ((places (quote ()))
        (values (quote ()))
        (rest pairs)
        (temps (quote ())))
    (while (not (null? rest))
      (set! places (cons (car rest) places))
      (set! rest (cdr rest))
      (set! values (cons (car rest) values))
      (set! rest (cdr rest)))
    (set! places (reverse places))
    (set! values (reverse values))
    (let ((i 0))
      (while (< i (length values))
        (set! temps (cons (gensym) temps))
        (set! i (+ i 1))))
    (set! temps (reverse temps))
    (list (quote let)
      (zip temps values)
      (cons (quote begin)
        (append
          (map2 (lambda (p t) (list (quote set!) p t)) places temps)
          (list (car temps)))))))

(define-macro (psetf . pairs)
  (let ((places (quote ()))
        (values (quote ()))
        (rest pairs)
        (temps (quote ())))
    (while (not (null? rest))
      (set! places (cons (car rest) places))
      (set! rest (cdr rest))
      (set! values (cons (car rest) values))
      (set! rest (cdr rest)))
    (set! places (reverse places))
    (set! values (reverse values))
    (let ((i 0))
      (while (< i (length values))
        (set! temps (cons (gensym) temps))
        (set! i (+ i 1))))
    (set! temps (reverse temps))
    (list (quote let)
      (zip temps values)
      (cons (quote begin)
        (append
          (map2 (lambda (p t) (list (quote setf) p t)) places temps)
          (list (car temps)))))))

(define (map2 f l1 l2)
  (if (or (null? l1) (null? l2)) '()
    (cons (f (car l1) (car l2))
          (map2 f (cdr l1) (cdr l2)))))

(define-macro (do varlist endlist . body)
  (let ((vars (mapcar car varlist))
        (inits (mapcar cadr varlist))
        (steps (mapcar (lambda (v) (if (null? (cddr v)) (car v) (caddr v))) varlist))
        (end-test (car endlist))
        (end-result (if (null? (cdr endlist)) #f (cadr endlist)))
        (lp (gensym)))
    (let ((step-args (mapcan2 list vars steps)))
      (list 'let (mapcar2 list vars inits)
        (list 'letrec
          (list (list lp (list 'lambda '()
                (list 'if end-test
                  end-result
                  (cons 'begin
                    (append body
                            (list (cons 'psetq step-args)
                                  (list lp))))))))
          (list lp))))))

(define-macro (do* varlist endlist . body)
  (let ((vars (mapcar car varlist))
        (inits (mapcar cadr varlist))
        (steps (mapcar (lambda (v) (if (null? (cddr v)) (car v) (caddr v))) varlist))
        (end-test (car endlist))
        (end-result (if (null? (cdr endlist)) #f (cadr endlist)))
        (lp (gensym)))
    (let ((step-forms (mapcar2 (lambda (v s) (list 'set! v s)) vars steps)))
      (list 'let* (mapcar2 list vars inits)
        (list 'letrec
          (list (list lp (list 'lambda '()
                (list 'if end-test
                  end-result
                  (cons 'begin
                    (append body
                            (append step-forms
                                    (list (list lp)))))))))
          (list lp))))))

(define (mapcar2 f l1 l2)
  (if (or (null? l1) (null? l2)) '()
    (cons (f (car l1) (car l2))
          (mapcar2 f (cdr l1) (cdr l2)))))

(define (mapcan2 f l1 l2)
  (if (or (null? l1) (null? l2)) '()
    (append (f (car l1) (car l2))
            (mapcan2 f (cdr l1) (cdr l2)))))

;; -------- Additional CL list functions --------
(define (copy-list lst)
  (if (null? lst) '()
    (if (pair? lst) (cons (car lst) (copy-list (cdr lst)))
      lst)))

(define (copy-tree x)
  (if (null? x) '()
    (if (pair? x) (cons (copy-tree (car x)) (copy-tree (cdr x)))
      x)))

(define (tree-equal x y . rest)
  (if (and (pair? x) (pair? y))
    (and (tree-equal (car x) (car y)) (tree-equal (cdr x) (cdr y)))
    (if (null? rest)
      (equal? x y)
      ((car rest) x y))))

(define (revappend x y)
  (if (null? x) y
    (revappend (cdr x) (cons (car x) y))))

(define (ldiff list sublist)
  (if (eq? list sublist) '()
    (if (null? list) '()
      (cons (car list) (ldiff (cdr list) sublist)))))

(define (tailp sublist list)
  (cond ((eq? sublist list) #t)
        ((null? list) #f)
        (else (tailp sublist (cdr list)))))

;; -------- type-of-matches helper --------
(define (type-of-matches obj type-spec)
  (cond
    ((eq? type-spec 'number) (number? obj))
    ((eq? type-spec 'string) (string? obj))
    ((eq? type-spec 'symbol) (symbol? obj))
    ((eq? type-spec 'list) (pair? obj))
    ((eq? type-spec 'cons) (pair? obj))
    ((eq? type-spec 'null) (null? obj))
    ((eq? type-spec 'boolean) (boolean? obj))
    ((eq? type-spec 'character) (char? obj))
    ((eq? type-spec 'function) (procedure? obj))
    ((eq? type-spec 'hash-table) (hash-table? obj))
    ((eq? type-spec 'stream) (streamp obj))
    ((eq? type-spec 'instance) (instance? obj))
    (else #f)))

;; destructure-pattern and destructuring-bind macro removed:
;; Go special form (bindPatternRec) handles &rest/&optional/&key properly
;; The old Lisp macro shadowed the Go implementation and was broken

(define (for-each f lst)
  (if (null? lst) #t
    (begin (f (car lst)) (for-each f (cdr lst)))))
(define-macro (when test . body)
  (list 'if test (cons 'begin body)))
(define-macro (unless test . body)
  (list 'if (list 'not test) (cons 'begin body)))

;; assert: simple version - (assert condition)
(define-macro (assert test . opts)
  (list 'if test
    nil
    (list 'error "assertion failed")))

;; builtin describe is used instead

;; -------- dotimes macro (letrec-based) --------
(define-macro (dotimes . args)
  (let ((var (caar args))
        (count (cadar args))
        (result (cdar args))
        (body (cdr args))
        (n (gensym))
        (lp (gensym)))
    (list 'let (list (list var 0) (list n count))
      (list 'letrec
        (list (list lp (list 'lambda '()
              (list 'if (list '>= var n)
                (cons 'begin result)
                (cons 'begin (append body
                  (list (list 'set! var (list '+ var 1))
                        (list lp))))))))
        (list lp)))))
(define-macro (defstruct name-and-options . slots)
  (letrec
    ((struct-name
       (if (pair? name-and-options) (car name-and-options) name-and-options))
     (options
       (if (pair? name-and-options) (cdr name-and-options) '()))
     (include-name #f)
     (constructor-name #f)
     (predicate-name #f)
     (copier-name #f)
     (conc-prefix #f)
     (print-fn #f)
     (repr-type 'instance)
     (slot-defs '())
     (expansions '())
     (parse-options
       (lambda (opts)
         (if (null? opts) 'done
           (begin
             (if (and (pair? (car opts)) (eq? (caar opts) :include))
               (set! include-name (cadar opts)))
             (if (and (pair? (car opts)) (eq? (caar opts) :constructor))
               (set! constructor-name (cadar opts)))
             (if (and (pair? (car opts)) (eq? (caar opts) :predicate))
               (set! predicate-name (cadar opts)))
             (if (and (pair? (car opts)) (eq? (caar opts) :copier))
               (set! copier-name (cadar opts)))
             (if (and (pair? (car opts)) (eq? (caar opts) :conc-name))
               (if (null? (cdar opts))
                 (set! conc-prefix "")
                 (set! conc-prefix (symbol->string (cadar opts))))
               )
             (if (and (pair? (car opts)) (eq? (caar opts) :print-object))
               (set! print-fn (cadar opts)))
             (if (and (pair? (car opts)) (eq? (caar opts) :print-function))
               (set! print-fn (cadar opts)))
             (if (and (pair? (car opts)) (eq? (caar opts) :type))
               (set! repr-type (cadar opts)))
             (parse-options (cdr opts))))))
     (parse-slots
       (lambda (sls)
         (if (null? sls) '()
           (if (symbol? (car sls))
             (cons (list (car sls) #f) (parse-slots (cdr sls)))
             (if (pair? (car sls))
               (cons (car sls) (parse-slots (cdr sls)))
               (parse-slots (cdr sls)))))))
     (slot-name
       (lambda (slot)
         (if (symbol? slot) slot
           (if (null? (cdr slot)) (car slot)
             (if (null? (cddr slot)) (car slot)
               (car slot))))))
     (gen-accessor
       (lambda (slot)
         (list 'define (list (accessor-name slot) 'obj)
               (list 'slot-value 'obj (list 'quote (slot-name slot))))))
     (gen-setter
       (lambda (slot read-only-slots)
         (if (member? (slot-name slot) read-only-slots)
           '#f
           (list 'define
                 (list (string->symbol
                       (string-append (symbol->string (accessor-name slot)) "-SETF"))
                       'val 'obj)
                 (list 'slot-set! 'obj (list 'quote (slot-name slot)) 'val)))))
     (accessor-name
       (lambda (slot)
         (if (string? conc-prefix)
           (if (eq? conc-prefix "")
             (slot-name slot)
             (string->symbol
               (string-append conc-prefix
                              (symbol->string (slot-name slot)))))
           (string->symbol
             (string-append (symbol->string struct-name) "-"
                            (symbol->string (slot-name slot)))))))
     (slot-keyword
       (lambda (slot)
         (string->symbol
           (string-append ":" (symbol->string (slot-name slot))))))
     (_g (gensym))
     (kw-args-sym (gensym)))
    ;; Body
    (parse-options options)
    (set! slot-defs (parse-slots slots))
    (if include-name
      (let ((parent-class (eval include-name)))
        (let ((parent-slots (class-slot-defs parent-class)))
          (let ((all-slots (append parent-slots slot-defs)))
            (let ((slot-names (mapcar slot-name all-slots))
                  (slot-defaults
                    (mapcar (lambda (s) (if (and (pair? s) (not (null? (cdr s)))) (cadr s) #f))
                         all-slots))
                  (read-only-slots
                    (mapcar (lambda (s)
                           (if (and (pair? s) (pair? (cddr s)) (eq? (caddr s) :read-only))
                             (car s) #f))
                         all-slots))
                  (pred-name
                    (if predicate-name predicate-name
                      (string->symbol
                        (string-append (symbol->string struct-name) "-p"))))
                  (cons-name
                    (if constructor-name constructor-name
                      (string->symbol
                        (string-append "make-" (symbol->string struct-name)))))
                  (copy-name
                    (if copier-name copier-name
                      (string->symbol
                        (string-append "copy-" (symbol->string struct-name))))))
            ;; Build expansion
            (set! expansions
              (cons (list 'defclass struct-name (list include-name) slot-names all-slots)
                    expansions))
            (let ((constructor-body
                    (cons 'let
                      (cons (list (list 'inst (list 'make-instance (list 'quote struct-name))))
                        (append
                          (mapcar (lambda (s)
                                 (list 'slot-set! 'inst (list 'quote (slot-name s))
                                   (list 'let (list (list _g (list 'member (slot-keyword s) kw-args-sym)))
                                     (list 'if _g (list 'cadr _g)
                                       (if (and (pair? s) (not (null? (cdr s)))) (cadr s) #f)))))
                               all-slots)
                          (list 'inst))))))
              (set! expansions
                (cons (list 'define (cons cons-name kw-args-sym)
                            constructor-body)
                      expansions)))
            (set! expansions (append (mapcar gen-accessor all-slots) expansions))
            (set! expansions (append (mapcar (lambda (s) (gen-setter s read-only-slots)) all-slots) expansions))
            (set! expansions
              (cons
                (list 'define (list pred-name 'obj)
                      (list 'is-a? 'obj (list 'quote struct-name)))
                expansions))
            (set! expansions
              (cons
                (list 'define (list copy-name 'obj)
                      (cons 'begin
                        (cons (list 'define 'new (list 'make-instance (list 'quote struct-name)))
                          (append
                            (mapcar (lambda (s)
                                   (list 'slot-set! 'new (list 'quote (slot-name s))
                                         (list (accessor-name s) 'obj)))
                                 all-slots)
                            (list 'new)))))
                expansions))
            (if print-fn
              (let ((print-fn-name
                      (string->symbol
                        (string-append (symbol->string struct-name) "-print"))))
                (set! expansions
                  (cons (list 'define print-fn-name print-fn) expansions))
                (set! expansions
                  (cons (list 'set-class-print-fn (list 'quote struct-name) print-fn-name)
                        expansions))))
            (cons 'begin (reverse expansions))))))
      ;; no :include branch
      (let ((all-slots slot-defs)
            (slot-names (mapcar slot-name slot-defs))
            (slot-defaults
              (mapcar (lambda (s) (if (and (pair? s) (not (null? (cdr s)))) (cadr s) #f))
                   slot-defs))
            (read-only-slots
              (mapcar (lambda (s)
                     (if (and (pair? s) (pair? (cddr s)) (eq? (caddr s) :read-only))
                       (car s) #f))
                   slot-defs))
            (pred-name
              (if predicate-name predicate-name
                (string->symbol
                  (string-append (symbol->string struct-name) "-p"))))
            (cons-name
              (if constructor-name constructor-name
                (string->symbol
                  (string-append "make-" (symbol->string struct-name)))))
            (copy-name
              (if copier-name copier-name
                (string->symbol
                  (string-append "copy-" (symbol->string struct-name))))))
        (set! expansions
          (cons (list 'defclass struct-name '() slot-names all-slots) expansions))
        (let ((constructor-body
                (cons 'let
                  (cons (list (list 'inst (list 'make-instance (list 'quote struct-name))))
                    (append
                      (mapcar (lambda (s)
                             (list 'slot-set! 'inst (list 'quote (slot-name s))
                               (list 'let (list (list _g (list 'member (slot-keyword s) kw-args-sym)))
                                 (list 'if _g (list 'cadr _g)
                                   (if (and (pair? s) (not (null? (cdr s)))) (cadr s) #f)))))
                           all-slots)
                      (list 'inst))))))
          (set! expansions
            (cons (list 'define (cons cons-name kw-args-sym)
                        constructor-body)
                  expansions)))
        (set! expansions (append (mapcar gen-accessor all-slots) expansions))
        (set! expansions (append (mapcar (lambda (s) (gen-setter s read-only-slots)) all-slots) expansions))
        (set! expansions
          (cons
            (list 'define (list pred-name 'obj)
                  (list 'is-a? 'obj (list 'quote struct-name)))
            expansions))
        (set! expansions
          (cons
            (list 'define (list copy-name 'obj)
                  (cons 'begin
                    (cons (list 'define 'new (list 'make-instance (list 'quote struct-name)))
                      (append
                        (mapcar (lambda (s)
                               (list 'slot-set! 'new (list 'quote (slot-name s))
                                     (list (accessor-name s) 'obj)))
                             all-slots)
                        (list 'new)))))
            expansions))
        (if print-fn
          (let ((print-fn-name
                  (string->symbol
                    (string-append (symbol->string struct-name) "-print"))))
            (set! expansions
              (cons (list 'define print-fn-name print-fn) expansions))
            (set! expansions
              (cons (list 'set-class-print-fn (list 'quote struct-name) print-fn-name)
                    expansions))))
        (cons 'begin (reverse expansions))))))

;; -------- let* macro --------
(define-macro (let* bindings . body)
  (if (null? bindings)
    (cons 'let (cons '() body))
    (list 'let (list (car bindings))
          (cons 'let* (cons (cdr bindings) body)))))

;; -------- while macro --------
(define-macro (while test . body)
  (let ((lp (gensym)))
    (list 'letrec
      (list (list lp (list 'lambda '()
            (list 'if test
              (cons 'begin (cons '(%loop-check) (append body (list (list lp)))))))))
      (list lp))))

;; -------- dolist macro --------
(define-macro (dolist spec . body)
  (let ((var (car spec))
        (list-expr (cadr spec))
        (result-cdr (cddr spec))
        (lst (gensym))
        (result-var (gensym)))
    (let ((result (if (null? result-cdr) #f (car result-cdr))))
      (list 'let*
        (list (list lst list-expr)
              (list var '())
              (list result-var (if result result #f)))
        (list 'while (list 'not (list 'null? lst))
          (list 'begin
            (list 'set! var (list 'car lst))
            (cons 'begin body)
            (list 'set! lst (list 'cdr lst))))
        result-var))))

;; -------- loop macro --------
(define-macro (loop . clauses)
  (let ((named #f) (initially '()) (finally '()) (with-list '())
        (for-list '()) (accum-list '()) (term-list '()) (do-list '())
        (return-val #f) (has-return #f)
        (finish-mode #f) (finish-val #f)
        (current-guard #f))


    ;; helper: get keyword from a list (first symbol)
    (define (get-keyword x)
      (if (and (not (null? x)) (symbol? (car x))) (car x) #f))

    (define (parse-clauses cls)
      (if (null? cls) 'done
        (let ((kw (car cls)) (rest (cdr cls)))
          (cond
            ((equal? kw 'for)
             (let ((var (car rest))
                   (kind (cadr rest))
                   (tail (cddr rest)))
               ;; Collect arguments until the next loop keyword
               (let ((args '()))
                 (while (and (pair? tail)
                             (not (member (car tail)
                                    '(being for with while until repeat collect sum count
                                      maximize minimize append nconc when unless
                                      if do named initially finally return always never thereis and by))))
                   (set! args (append args (list (car tail))))
                   (set! tail (cdr tail)))
                 (set! for-list
                   (cons (cons var (cons kind args)) for-list))
                 (parse-clauses tail))))
            ((equal? kw 'with)
             (let ((var (car rest))
                   (val (if (and (not (null? (cdr rest))) (equal? (cadr rest) '=))
                          (caddr rest) (cadr rest))))
               (set! with-list (cons (list var val) with-list))
               (parse-clauses (if (and (not (null? (cdr rest))) (equal? (cadr rest) '=))
                               (cdddr rest) (cddr rest)))))
            ((equal? kw 'while)
             (set! term-list (cons (list 'while (car rest)) term-list))
             (parse-clauses (cdr rest)))
            ((equal? kw 'until)
             (set! term-list (cons (list 'until (car rest)) term-list))
             (parse-clauses (cdr rest)))
            ((equal? kw 'repeat)
             (set! term-list (cons (list 'repeat (car rest)) term-list))
             (parse-clauses (cdr rest)))
            ((equal? kw 'and)
             ;; Parallel for clause: parse the next for clause
             (parse-clauses rest))
            ((equal? kw 'named)
             (set! named (car rest))
             (parse-clauses (cdr rest)))
            ((equal? kw 'initially)
             (let ((body '()))
               (define (collect-body xs)
                 (if (or (null? xs)
                         (member (car xs)
                           '(for with while until repeat collect sum count
                             maximize minimize append nconc when unless
                             if do named initially finally return always never thereis and)))
                   (begin (set! initially (append initially (reverse body)))
                          (parse-clauses xs))
                   (begin (set! body (cons (car xs) body))
                          (collect-body (cdr xs)))))
               (collect-body rest)))
            ((equal? kw 'finally)
             (let ((body '()))
               (define (collect-body xs)
                 (if (or (null? xs)
                         (member (car xs)
                           '(for with while until repeat collect sum count
                             maximize minimize append nconc when unless
                             if do named initially finally return always never thereis and)))
                   (begin (set! finally (append finally (reverse body)))
                          (parse-clauses xs))
                   (begin (set! body (cons (car xs) body))
                          (collect-body (cdr xs)))))
               (collect-body rest)))
            ((equal? kw 'do)
             (let ((body '()))
               (define (collect-body xs)
                 (if (or (null? xs)
                         (member (car xs)
                           '(for with while until repeat collect sum count
                             maximize minimize append nconc when unless
                             if do named initially finally return always never thereis and)))
                   (begin (if current-guard
                        (let ((bl (reverse body)))
                          (set! do-list (cons (list 'if current-guard
                                            (if (null? (cdr bl)) (car bl) (cons 'begin bl)))
                                          do-list))
                          (set! current-guard #f))
                        (set! do-list (append do-list (reverse body))))
                          (parse-clauses xs))
                   (begin (set! body (cons (car xs) body))
                          (collect-body (cdr xs)))))
               (collect-body rest)))
            ((equal? kw 'return)
             (if current-guard
               (begin
                 (set! do-list (cons (list 'if current-guard
                                      (list 'return-from 'nil (car rest)) '())
                                      do-list))
                 (set! current-guard #f))
               (begin
                 (set! has-return #t)
                 (set! return-val (car rest))))
             (parse-clauses (cdr rest)))
            ((equal? kw 'always)
             (set! finish-mode 'always)
             (if current-guard
               (set! do-list (cons (list 'if current-guard
                                    (list 'if (car rest) '()
                                      (list 'progn (list 'set! 'loop-result #f) (list 'loop-finish))))
                                    do-list))
               (set! do-list (cons (list 'if (car rest) '()
                                    (list 'progn (list 'set! 'loop-result #f) (list 'loop-finish)))
                                    do-list)))
             (set! current-guard #f)
             (parse-clauses (cdr rest)))
            ((equal? kw 'never)
             (set! finish-mode 'never)
             (if current-guard
               (set! do-list (cons (list 'if current-guard
                                    (list 'if (car rest)
                                      (list 'progn (list 'set! 'loop-result #f) (list 'loop-finish)) '()))
                                    do-list))
               (set! do-list (cons (list 'if (car rest)
                                    (list 'progn (list 'set! 'loop-result #f) (list 'loop-finish)) '())
                                    do-list)))
             (set! current-guard #f)
             (parse-clauses (cdr rest)))
            ((equal? kw 'thereis)
             (set! finish-mode 'thereis)
             (let ((tvar (gensym "thereis-")))
               (let ((new-item (list 'let (list (list tvar (car rest)))
                                      (list 'if tvar
                                        (list 'progn (list 'set! 'loop-result tvar) (list 'loop-finish)) '()))))
                 (if current-guard
                   (set! do-list (cons (list 'if current-guard new-item '()) do-list))
                   (set! do-list (cons new-item do-list)))))
             (set! current-guard #f)
             (parse-clauses (cdr rest)))
            ((equal? kw 'collect)
             (let ((expr (car rest))
                   (into-var (if (and (pair? (cdr rest)) (equal? (cadr rest) 'into))
                               (caddr rest) #f)))
               (set! accum-list
                 (cons (list 'collect expr current-guard into-var) accum-list))
               (set! current-guard #f)
               (if (and (pair? (cdr rest)) (equal? (cadr rest) 'into))
                 (parse-clauses (cdddr rest))
                 (parse-clauses (cdr rest)))))
            ((equal? kw 'sum)
             (let ((expr (car rest))
                   (into-var (if (and (pair? (cdr rest)) (equal? (cadr rest) 'into))
                               (caddr rest) #f)))
               (set! accum-list
                 (cons (list 'sum expr current-guard into-var) accum-list))
               (set! current-guard #f)
               (if (and (pair? (cdr rest)) (equal? (cadr rest) 'into))
                 (parse-clauses (cdddr rest))
                 (parse-clauses (cdr rest)))))
            ((equal? kw 'count)
             (let ((expr (car rest))
                   (into-var (if (and (pair? (cdr rest)) (equal? (cadr rest) 'into))
                               (caddr rest) #f)))
               (set! accum-list
                 (cons (list 'count expr current-guard into-var) accum-list))
               (set! current-guard #f)
               (if (and (pair? (cdr rest)) (equal? (cadr rest) 'into))
                 (parse-clauses (cdddr rest))
                 (parse-clauses (cdr rest)))))
            ((equal? kw 'maximize)
             (let ((expr (car rest))
                   (into-var (if (and (pair? (cdr rest)) (equal? (cadr rest) 'into))
                               (caddr rest) #f)))
               (set! accum-list
                 (cons (list 'maximize expr current-guard into-var) accum-list))
               (set! current-guard #f)
               (if (and (pair? (cdr rest)) (equal? (cadr rest) 'into))
                 (parse-clauses (cdddr rest))
                 (parse-clauses (cdr rest)))))
            ((equal? kw 'minimize)
             (let ((expr (car rest))
                   (into-var (if (and (pair? (cdr rest)) (equal? (cadr rest) 'into))
                               (caddr rest) #f)))
               (set! accum-list
                 (cons (list 'minimize expr current-guard into-var) accum-list))
               (set! current-guard #f)
               (if (and (pair? (cdr rest)) (equal? (cadr rest) 'into))
                 (parse-clauses (cdddr rest))
                 (parse-clauses (cdr rest)))))
            ((equal? kw 'append)
             (let ((expr (car rest))
                   (into-var (if (and (pair? (cdr rest)) (equal? (cadr rest) 'into))
                               (caddr rest) #f)))
               (set! accum-list
                 (cons (list 'append expr current-guard into-var) accum-list))
               (set! current-guard #f)
               (if (and (pair? (cdr rest)) (equal? (cadr rest) 'into))
                 (parse-clauses (cdddr rest))
                 (parse-clauses (cdr rest)))))
            ((equal? kw 'nconc)
             (let ((expr (car rest))
                   (into-var (if (and (pair? (cdr rest)) (equal? (cadr rest) 'into))
                               (caddr rest) #f)))
               (set! accum-list
                 (cons (list 'nconc expr current-guard into-var) accum-list))
               (set! current-guard #f)
               (if (and (pair? (cdr rest)) (equal? (cadr rest) 'into))
                 (parse-clauses (cdddr rest))
                 (parse-clauses (cdr rest)))))
            ((equal? kw 'by)
             ;; Set the step for the most recent for clause
             (if (null? for-list)
               (error "by: no for clause to modify")
               (let ((last-for (car for-list)))
                 (set-cdr! last-for (append (cdr last-for) (list 'by (car rest))))
                 (parse-clauses (cdr rest)))))
            ((or (equal? kw 'when) (equal? kw 'if))
             (let ((test (car rest)))
               (set! current-guard test)
               (parse-clauses (cdr rest))))
            ((equal? kw 'unless)
             (set! current-guard (list 'not (car rest)))
             (parse-clauses (cdr rest)))
            (else
             (set! do-list (cons kw do-list))
             (parse-clauses rest))))))

    ;; --- Parse ---
    (parse-clauses clauses)

    ;; --- Generate expansion ---
    (let* ((raw-for-vars (reverse for-list))
           (accums (reverse accum-list))
           (acc-names '())
           (collect-acc-names '())
           (acc-inits '())
           (acc-updates '())
           (acc-results '())
           (idx 0)
           ;; Expand in-clauses: (x in list) -> (tail-N in list) + (x (car tail-N))
           (in-lets '())
           ;; Expand = clauses without then: (x = expr) -> param with nil init + body set!
           (eq-lets '())
           (expanded '())
           ;; Collect destructuring specs for for...in with pattern vars
           (destr-specs '()))
      (for-each
        (lambda (f)
          (if (equal? (cadr f) 'in)
            (let* ((user-var (car f))
                   (list-expr (car (cddr f)))
                   (tail (gensym "in-")))
              ;; Check if user-var is a destructuring pattern (a list, not a symbol)
              (if (symbol? user-var)
                (begin
                  (set! in-lets (cons (list user-var tail list-expr #f) in-lets))
                  (set! expanded (cons (list tail 'in list-expr) expanded)))
                (let ((hidden-var (gensym "destr-")))
                  (set! in-lets (cons (list user-var hidden-var list-expr #t) in-lets))
                  (set! expanded (cons (list hidden-var 'in list-expr) expanded)))))
            (if (equal? (cadr f) 'on)
              (let* ((user-var (car f))
                     (list-expr (car (cddr f)))
                     (tail (gensym "on-")))
                ;; Check if user-var is a destructuring pattern (a list, not a symbol)
                (if (symbol? user-var)
                  (set! expanded (cons f expanded))
                  (let ((hidden-var (gensym "destr-")))
                    (set! in-lets (cons (list user-var hidden-var list-expr #t 'on) in-lets))
                    (set! expanded (cons (list hidden-var 'on list-expr) expanded)))))
            (if (equal? (cadr f) 'being)
              ;; Handle: for sym being each present-symbol of package
              ;; Handle: for k being the hash-keys of ht
              ;; Handle: for v being the hash-values of ht
              ;; Handle: for k being the hash-keys of ht using (hash-value v)
              ;;   args = (each/the present-symbol/hash-keys/hash-values of <expr> [using (hash-value/hash-key var)])
              (let* ((user-var (car f))
                     (args (cddr f))
                     (skip-article (if (and (pair? args) (or (equal? (car args) 'each) (equal? (car args) 'the))) (cdr args) args))
                     (sym-kind (if (pair? skip-article) (car skip-article) #f))
                     (after-kind (if (pair? skip-article) (cdr skip-article) '()))
                     (of-expr (if (and (pair? after-kind) (equal? (car after-kind) 'of) (pair? (cdr after-kind)))
                                   (cadr after-kind) #f))
                     ;; Check for using clause: (using (hash-value var)) or (using (hash-key var))
                     (rest-after-of (if (and (pair? after-kind) (equal? (car after-kind) 'of) (pair? (cdr after-kind)))
                                        (cddr after-kind) '()))
                     (using-clause (if (and (pair? rest-after-of) (equal? (car rest-after-of) 'using) (pair? (cdr rest-after-of)))
                                       (cadr rest-after-of) #f))
                     (using-kind (if (and using-clause (pair? using-clause)) (car using-clause) #f))
                     (using-var (if (and using-clause (pair? using-clause) (pair? (cdr using-clause))) (cadr using-clause) #f)))
                (if (and sym-kind of-expr
                         (or (equal? sym-kind 'present-symbol) (equal? sym-kind 'external-symbol)
                             (equal? sym-kind 'hash-keys) (equal? sym-kind 'hash-key)
                             (equal? sym-kind 'hash-values) (equal? sym-kind 'hash-value)))
                  (let* ((list-expr (cond
                                      ((equal? sym-kind 'present-symbol) (list 'package-symbols of-expr))
                                      ((equal? sym-kind 'external-symbol) (list 'package-external-symbols of-expr))
                                      ((or (equal? sym-kind 'hash-keys) (equal? sym-kind 'hash-key))
                                       (list 'hash-table-keys of-expr))
                                      ((or (equal? sym-kind 'hash-values) (equal? sym-kind 'hash-value))
                                       (list 'hash-table-values of-expr))
                                      (else (list 'package-symbols of-expr))))
                         (tail (gensym "in-")))
                    (set! in-lets (cons (list user-var tail list-expr) in-lets))
                    (set! expanded (cons (list tail 'in list-expr) expanded))
                    ;; Handle using clause: add parallel binding for hash-value or hash-key
                    (if (and using-var using-kind
                             (or (equal? using-kind 'hash-value) (equal? using-kind 'hash-values)
                                 (equal? using-kind 'hash-key) (equal? using-kind 'hash-keys)))
                      (let* ((using-list-expr (if (or (equal? using-kind 'hash-value) (equal? using-kind 'hash-values))
                                                   (list 'hash-table-values of-expr)
                                                   (list 'hash-table-keys of-expr)))
                             (using-tail (gensym "in-")))
                        (set! in-lets (cons (list using-var using-tail using-list-expr) in-lets))
                        (set! expanded (cons (list using-tail 'in using-list-expr) expanded)))))
                  (set! expanded (cons f expanded))))
            (if (equal? (cadr f) 'across)
              (let* ((var (car f))
                     (array-expr (car (cddr f)))
                     (idx (gensym "across-")))
                (set! eq-lets (cons (list var (list 'aref array-expr idx)) eq-lets))
                (set! expanded (cons (list idx 'from 0 'below (list 'length array-expr)) expanded))
                (set! expanded (cons (list var '= 'nil) expanded)))
            (if (equal? (cadr f) '=)
              (let* ((var (car f))
                     (args (cddr f))
                     (init-expr (car args)))
                (set! eq-lets (cons (list var init-expr) eq-lets))
                (set! expanded (cons (list var '= 'nil) expanded)))
              (set! expanded (cons f expanded))))))))
        raw-for-vars)
      (let* ((for-vars (reverse expanded))
             (in-lets (reverse in-lets))
             (eq-lets (reverse eq-lets)))

      ;; Build accumulator information
      (define (setup-accumulators)
        (for-each
          (lambda (a)
            (let* ((kind (car a))
                   (expr (cadr a))
                   (guard (or (caddr a) #t))
                   (into (if (null? (cdddr a)) #f (car (cdddr a))))
                   (name (if into into (string->symbol
                           (string-append
                             (symbol->string kind) "-acc-"
                             (number->string idx)))))
                   (_ (set! idx (+ idx 1))))
              (set! acc-names (cons name acc-names))
              (cond
                ((equal? kind 'collect)
                 (set! collect-acc-names (cons name collect-acc-names))
                 (set! acc-inits (cons (list name ''()) acc-inits))
                 (set! acc-updates
                   (cons (list 'set! name (list 'if guard (list 'cons expr name) name))
                         acc-updates))
                 (set! acc-results
                   (cons (list 'reverse name) acc-results)))
                ((equal? kind 'append)
                 (set! acc-inits (cons (list name ''()) acc-inits))
                 (set! acc-updates
                   (cons (list 'set! name (list 'if guard (list 'append name expr) name))
                         acc-updates))
                 (set! acc-results (cons name acc-results)))
                ((equal? kind 'nconc)
                 (set! acc-inits (cons (list name ''()) acc-inits))
                 (set! acc-updates
                   (cons (list 'set! name (list 'if guard (list 'append name expr) name))
                         acc-updates))
                 (set! acc-results (cons name acc-results)))
                ((equal? kind 'sum)
                 (set! acc-inits (cons (list name 0) acc-inits))
                 (set! acc-updates
                   (cons (list 'set! name (list 'if guard (list '+ name expr) name))
                         acc-updates))
                 (set! acc-results (cons name acc-results)))
                ((equal? kind 'count)
                 (set! acc-inits (cons (list name 0) acc-inits))
                 (set! acc-updates
                   (cons (list 'set! name
                           (list '+ name (list 'if (list 'if guard expr #f) 1 0)))
                         acc-updates))
                 (set! acc-results (cons name acc-results)))
                ((equal? kind 'maximize)
                 (set! acc-inits (cons (list name #f) acc-inits))
                 (set! acc-updates
                   (cons (list 'set! name
                           (list 'if guard
                             (list 'if (list 'or (list 'not name) (list '> expr name)) expr name)
                             name))
                         acc-updates))
                 (set! acc-results (cons name acc-results)))
                ((equal? kind 'minimize)
                 (set! acc-inits (cons (list name #f) acc-inits))
                 (set! acc-updates
                   (cons (list 'set! name
                           (list 'if guard
                             (list 'if (list 'or (list 'not name) (list '< expr name)) expr name)
                             name))
                         acc-updates))
                 (set! acc-results (cons name acc-results))))))
          accums))

      (setup-accumulators)

      ;; Build for-variable information
      (define (for-init f)
        (let ((var (car f)) (kind (cadr f)) (args (cddr f)))
          (cond
            ((or (equal? kind 'from) (equal? kind 'to)
                 (equal? kind 'below) (equal? kind 'above)
                 (equal? kind 'downto))
             (list var (car args)))
            ((equal? kind 'in)
             (list var (car args)))
            ((equal? kind 'on)
             (list var (car args)))
            ((equal? kind '=)
             ;; =: body rebinds each iteration
             (list var #f))
            (else (list var 0)))))

      (define (for-step f)
        (let ((var (car f)) (kind (cadr f)) (args (cddr f)))
          (cond
            ((equal? kind 'from)
             (let* ((start (car args))
                    (rest-args (cdr args))
                    (dir (if (null? rest-args) 'to (car rest-args)))
                    (end (cond
                           ((null? rest-args) start)
                           ((equal? (car rest-args) 'by) start)
                           ((null? (cdr rest-args)) (car rest-args))
                           ((equal? (cadr rest-args) 'by) (car rest-args))
                           (else (cadr rest-args))))
                    (by-val (cond
                              ((null? rest-args) 1)
                              ((equal? (car rest-args) 'by)
                               (if (null? (cdr rest-args)) 1 (cadr rest-args)))
                              ((null? (cdr rest-args)) 1)
                              ((equal? (cadr rest-args) 'by)
                               (if (null? (cddr rest-args)) 1 (caddr rest-args)))
                              ((null? (cddr rest-args)) 1)
                              ((equal? (caddr rest-args) 'by)
                               (if (null? (cdddr rest-args)) 1 (car (cdddr rest-args))))
                              (else 1))))
               (cond
                 ((equal? dir 'downto) (list '- var by-val))
                 ((equal? dir 'above) (list '- var by-val))
                 (else (list '+ var by-val)))))
            ((equal? kind 'downto)
             (let* ((end (car args))
                    (rest-args (cdr args))
                    (by-val (cond
                              ((null? rest-args) 1)
                              ((equal? (car rest-args) 'by)
                               (if (null? (cdr rest-args)) 1 (cadr rest-args)))
                              ((null? (cdr rest-args)) 1)
                              ((equal? (cadr rest-args) 'by)
                               (if (null? (cddr rest-args)) 1 (caddr rest-args)))
                              (else 1))))
               (list '- var by-val)))
            ((equal? kind 'in)
             (list 'cdr var))
            ((equal? kind 'on)
             (list 'cdr var))
            ((equal? kind '=)
             ;; =: body rebinds each iteration, step doesn't matter
             #f)
            (else var))))

      (define (for-termination f)
        (let ((var (car f)) (kind (cadr f)) (args (cddr f)))
          (cond
            ((equal? kind 'from)
             (let* ((start (car args))
                    (rest-args (cdr args))
                    (dir (if (null? rest-args) #f (car rest-args)))
                    (end (if (null? rest-args) #f
                           (cadr rest-args))))
               (cond
                 ((not dir) #f)
                 ((equal? dir 'to) (list '> var end))
                 ((equal? dir 'below) (list '>= var end))
                 ((equal? dir 'above) (list '<= var end))
                 ((equal? dir 'downto) (list '< var end))
                 ((equal? dir 'by) #f)
                 (else #f))))
            ((equal? kind 'downto)
             (list '< var (car args)))
            ((equal? kind 'to)
             (list '> var (car args)))
            ((equal? kind 'below)
             (list '>= var (car args)))
            ((equal? kind 'above)
             (list '<= var (car args)))
            ((equal? kind 'in)
             (list 'null? var))
            ((equal? kind 'on)
             (list 'null? var))
            (else #f))))

      ;; Build loop function body
      (let* ((params (mapcar car for-vars))
             (loop-name (if named named (gensym "loop-")))
             (term-tests '())
             (body-exprs '()))


        ;; Termination conditions from for clauses
        (for-each
          (lambda (f)
            (let ((t (for-termination f)))
              (if t (set! term-tests (cons t term-tests)))))
          for-vars)

        ;; Termination conditions from term-list
        (for-each
          (lambda (t)
            (cond
              ((equal? (car t) 'while)
               (set! term-tests
                 (cons (list 'not (cadr t)) term-tests)))
              ((equal? (car t) 'until)
               (set! term-tests (cons (cadr t) term-tests)))
              ((equal? (car t) 'repeat)
               (let ((rvar (gensym "repeat-")))
                 (set! acc-inits (cons (list rvar (cadr t)) acc-inits))
                 (set! term-tests
                   (cons (list '<= rvar 0) term-tests))
                 (set! acc-updates
                   (cons (list 'set! rvar (list '- rvar 1))
                         (reverse acc-updates)))))))
          (reverse term-list))

        ;; Build body: do clauses first, then accumulations
        (set! body-exprs (reverse do-list))

        ;; Build body: accumulations come AFTER do-clauses
        (set! body-exprs
          (append body-exprs (reverse acc-updates)))

        ;; If has-return and named, add return-from to body
        (if (and has-return named)
          (set! body-exprs
            (append body-exprs (list (list 'return-from loop-name return-val)))))


        ;; Add = clause bindings as pre-body set! operations
        (for-each
          (lambda (e)
            (let ((var (car e))
                  (init-expr (cadr e)))
              ;; recompute expr each iteration
              (set! body-exprs
                (cons (list 'set! var init-expr) body-exprs))))
          eq-lets)

        ;; Add in-clause element extraction as pre-body bindings (after eq-lets so in-lets come first)
        ;; For destructuring patterns: we use tail-var as the lambda param, and add
        ;; (let ((hidden-var (car tail-var))) ...) to the body. No set! needed.
        (for-each
          (lambda (p)
            (let ((user-var (car p))
                  (tail-var (cadr p))
                  (list-expr (caddr p))
                  (is-destr (car (cdddr p)))
                  (destr-mode (if (null? (cdr (cdddr p))) 'in (cadr (cdddr p)))))
              (if is-destr
                (begin
                  ;; Store (pattern tail-var mode) for wrapping
                  (set! destr-specs (cons (list user-var tail-var destr-mode) destr-specs)))
                (begin
                  ;; Add initial binding: (user-var (car list-expr))
                  (set! acc-inits
                    (cons (list user-var (list 'car list-expr)) acc-inits))
                  ;; Add set! at start of each iteration
                  (set! body-exprs
                    (cons (list 'set! user-var (list 'car tail-var))
                          body-exprs))))))
          in-lets)

        ;; Build the loop function
        (let* ((result-expr
                 (cond
                   (has-return
                    (let ((rv return-val))
                      (if (member rv acc-names)
                          (list 'reverse rv)
                          rv)))
                   (finish-mode
                    (cond ((equal? finish-mode 'always) #t)
                          ((equal? finish-mode 'never) #t)
                          ((equal? finish-mode 'thereis)
                           (if finish-val finish-val #f))
                          (else #f)))
                   ((not (null? acc-results))
                    (if (null? (cdr acc-results))
                      (car acc-results)
                      (cons 'list acc-results)))
                   (else #f)))
               (next-vals (mapcar for-step for-vars))
               (recurse-call (cons loop-name next-vals))
               (body-with-result-save
                 (if (and result-expr (not finish-mode))
                   (append body-exprs (list (list 'set! 'loop-result result-expr)))
                   body-exprs))
               (full-body
                 (if (null? term-tests)
                   (if (null? body-with-result-save)
                     (cons 'begin (list '(%loop-check) recurse-call))
                     (cons 'begin
                       (append (cons '(%loop-check) body-with-result-save) (list recurse-call))))
                   ;; With termination check
                   (let ((combined-term
                           (if (null? (cdr term-tests))
                             (car term-tests)
                             (cons 'or term-tests))))
                     (list 'if combined-term
                       (if result-expr result-expr '())
                       (if (null? body-with-result-save)
                         (cons 'begin (list '(%loop-check) recurse-call))
                         (cons 'begin
                           (append (cons '(%loop-check) body-with-result-save) (list recurse-call))))))))
               ;; finally forms
               (post-expr
                 (if (null? finally) '()
                   (if (null? (cdr finally))
                     (car finally)
                     (cons 'begin finally))))
               (loop-fn-body full-body)
               ;; Wrap full-body with destructuring-bind for any destructuring for-clauses
               (wrapped-body
                 (let ((result full-body))
                   (for-each
                     (lambda (d)
                       (let ((pattern (car d))
                             (tail-var (cadr d))
                             (mode (caddr d)))
                         (set! result (list 'destructuring-bind pattern
                                       (if (equal? mode 'on) tail-var (list 'car tail-var))
                                       result))))
                     destr-specs)
                   result)))

          ;; Wrap with finally
          (let* ((no-fin (cons 'letrec
                          (cons (list (list loop-name
                                     (cons 'lambda (cons params
                                       (list wrapped-body)))))
                                (list (cons loop-name
                                  (mapcar (lambda (f) (cadr (for-init f)))
                                       for-vars))))))
                 (final-expr
                   (if (not (null? post-expr))
                     (let ((rev-steps (mapcar (lambda (n) (list 'set! n (list 'reverse n))) collect-acc-names)))
                       (list 'let (list (list (gensym "loop-result-") no-fin))
                         (if (null? (cdr finally))
                           (if (null? rev-steps)
                             (car finally)
                             (cons 'begin (append rev-steps (list (car finally)))))
                           (cons 'begin (append rev-steps finally)))))
                     no-fin)))
            ;; Wrap with initial forms
            (let* ((with-bindings
                     (if (null? with-list) '()
                       (mapcar (lambda (w) (list (car w) (cadr w))) with-list)))
                   (all-bindings (append acc-inits (list (list 'loop-result '())) with-bindings))
                   (initially-expr
                     (if (null? initially) #f
                       (if (null? (cdr initially))
                         (car initially)
                         (cons 'begin initially))))
                   (wrapped
                     (if (null? all-bindings)
                       final-expr
                       (list 'let (reverse all-bindings) final-expr))))
              (if initially-expr
                (set! wrapped (list 'begin initially-expr wrapped)))
              (if named
                (list 'block named (list 'block 'nil wrapped))
                (list 'block 'nil wrapped))))))))))
;; The Go builtin format is registered in defPrim and takes precedence.

;; -------- CL-style sequence wrappers --------
(define (reduce fn seq . rest)
  (if (null? rest)
    (apply seq-reduce (list fn seq))
    (apply seq-reduce (append (list fn seq) rest))))
(define (find item seq . rest)
  (apply seq-find (cons item (cons seq rest))))
(define (position item seq . rest)
  (apply seq-position (cons item (cons seq rest))))
(define (remove-if pred seq . rest)
  (apply seq-remove-if (cons pred (cons seq rest))))
(define (remove-if-not pred seq . rest)
  (apply seq-remove-if-not (cons pred (cons seq rest))))
(define (substitute new old seq . rest)
  (apply seq-substitute (cons new (cons old (cons seq rest)))))
(define (substitute-if new pred seq . rest)
  (apply seq-substitute-if (cons new (cons pred (cons seq rest)))))
(define (substitute-if-not new pred seq . rest)
  (apply seq-substitute-if-not (cons new (cons pred (cons seq rest)))))
(define (sort seq pred . rest)
  (apply seq-sort (cons seq (cons pred rest))))
(define (stable-sort seq pred . rest)
  (apply seq-sort (cons seq (cons pred rest))))
(define (count item seq . rest)
  (apply seq-count (cons item (cons seq rest))))
(define (count-if pred seq . rest)
  (apply seq-count-if (cons pred (cons seq rest))))
(define (count-if-not pred seq . rest)
  (apply seq-count-if-not (cons pred (cons seq rest))))
(define (remove item seq . rest)
  (apply seq-remove (cons item (cons seq rest))))
(define (remove-duplicates seq . rest)
  (apply seq-remove-duplicates (cons seq rest)))
(define (find-if pred seq . rest)
  (apply seq-find-if (cons pred (cons seq rest))))
(define (find-if-not pred seq . rest)
  (apply seq-find-if-not (cons pred (cons seq rest))))
(define (position-if pred seq . rest)
  (apply seq-position-if (cons pred (cons seq rest))))
(define (position-if-not pred seq . rest)
  (apply seq-position-if-not (cons pred (cons seq rest))))
(define (merge seq1 seq2 pred . rest)
  (if (or (eq? seq1 'list) (eq? seq1 'vector) (eq? seq1 'string))
      ;; CL-style: (merge 'type seq1 seq2 pred) -- seq1 is type
      (let ((result (seq-merge seq2 pred (car rest))))
        (coerce result seq1))
      ;; MicroLisp-style: (merge seq1 seq2 pred)
      (seq-merge seq1 seq2 pred)))


;; -------- Condition system macros --------

;; with-simple-restart: establish a simple named restart
;; (with-simple-restart (name format-string &rest args) &body body)
(define-macro (with-simple-restart spec . body)
  (let ((rname (car spec)))
    (list 'restart-case
          (cons 'begin body)
          (list rname '() (list 'quote 'nil)))))

;; define-condition: macro for defining condition classes with :report support
;; (define-condition name (parents) (slots...))
;; (define-condition name (parents) (slots...) (:report fn))
(define-macro (define-condition name parents slots . options)
  (let ((report-expr nil))
    (dolist (opt options)
      (if (and (pair? opt) (eq (car opt) (quote :report)))
          (setq report-expr (cadr opt))))
    (let ((defclass-form (list (quote defclass) name parents slots)))
      (if report-expr
          (list (quote progn)
                defclass-form
                (list (quote defvar)
                      (string->symbol (string-append (symbol->string name) "-cond-report"))
                      report-expr))
          defclass-form))))




;; -------- with-slots, with-accessors --------
(define-macro (with-slots slots instance . body)
  (let ((inst (gensym)))
    (list 'let (list (list inst instance))
      (cons 'let
        (cons (mapcar (lambda (s)
                     (if (pair? s)
                       (list (car s) (list 'slot-value inst (list 'quote (cadr s))))
                       (list s (list 'slot-value inst (list 'quote s)))))
                   slots)
              body)))))

(define-macro (with-accessors accessors instance . body)
  (let ((inst (gensym)))
    (list 'let (list (list inst instance))
      (cons 'let
        (cons (mapcar (lambda (a)
                     (list (car a) (list (cadr a) inst)))
                   accessors)
              body)))))

;; -------- with-open-file --------
;; (with-open-file (var filespec &rest keys) &body body)
(define-macro (with-open-file spec . body)
  (let ((var (car spec))
        (filespec (cadr spec))
        (keys (cddr spec)))
    (list 'let (list (list var (cons 'open (cons filespec keys))))
      (list 'unwind-protect
        (cons 'progn body)
        (list 'close var)))))

;; -------- step macro --------
;; (step form) -- evaluate form with stepping enabled (currently just evaluates)
;; *step* variable controls stepping behavior
(define-macro (step form)
  form)

;; -------- defgeneric --------
;; (defgeneric name args &rest options)
;; Options: (:method-combination combo) (:method ...)
(define-macro (defgeneric name args . options)
  (let ((combo-opt (defpackage-find-opt options ':method-combination))
        (method-forms (filter (lambda (o) (and (pair? o) (equal? (car o) ':method))) options)))
    (let ((base (list 'defparameter name (list 'make-generic-function (list 'quote name))))
          (combo-form (if (null? combo-opt) '()
                         (list (list 'set-method-combination name (list 'quote (car combo-opt))))))
          (method-defs (mapcar (lambda (m) (cons 'defmethod (cons name (cdr m)))) method-forms)))
      (cons 'progn (append (list base) combo-form method-defs)))))

;; -------- defpackage --------
;; defpackage: create a package with optional use, export, nicknames
;; Usage: (defpackage :name (:use :cl) (:export :foo :bar) (:nickname :n))
(define-macro (defpackage name . options)
  ;; Strip leading colon from keyword package names
  (let ((pkg-str (if (keywordp name)
                   (substring (symbol->string name) 1 (string-length (symbol->string name)))
                   (if (symbol? name) (symbol->string name) name))))
    (let ((use-list (defpackage-find-opt options ':use))
          (exports (defpackage-find-opt options ':export))
          (nicknames (defpackage-find-opt options ':nickname)))
      (let ((base-forms (list (list 'make-package (list 'quote pkg-str))
                              (list 'in-package (list 'quote pkg-str))))
            (use-form (if (null? use-list)
                        '()
                        (list (list 'use-package
                                (cons 'list (mapcar (lambda (p) (list 'quote p)) use-list))))))
            (export-forms (mapcar (lambda (s) (list 'export (list 'quote s))) exports))
            (nick-forms (if (null? nicknames)
                          '()
                          (list (list 'rename-package
                                  (list 'quote pkg-str)
                                  (list 'quote pkg-str)
                                  (cons 'list (mapcar (lambda (n) (list 'quote n)) nicknames)))))))
        (cons 'progn
              (append base-forms
                      (append use-form
                              (append export-forms nick-forms))))))))

;; Helper function for defpackage: find option by key
(define (defpackage-find-opt opts key)
  (cond
    ((null? opts) '())
    ((and (pair? (car opts)) (equal? (caar opts) key)) (cdar opts))
    (else (defpackage-find-opt (cdr opts) key))))

;; remove-env: remove &environment and its following symbol from a lambda list
(define (remove-env ll)
  (cond
    ((null ll) nil)
    ((eq (car ll) '&environment) (cdr (cdr ll)))
    ((eq (car ll) '&ENVIRONMENT) (cdr (cdr ll)))
    (t (cons (car ll) (remove-env (cdr ll))))))

;; defsetf: (defsetf accessor (var* &environment env newval) body...)
;; or:      (defsetf accessor (var* newval) body...)
;; Supports optional &environment parameter. Filters it out from setter params.
(define-macro (defsetf name params . body)
  (let* ((params-list (if (and (pair? params) (pair? (car params)))
                          (car params)
                          params))
          (cleaned (remove-env params-list))
          (orig-vars (butlast cleaned))
          (newval-sym (car (last cleaned)))
          (setter-fn (string->symbol (string-append (symbol->string name) "-SETF"))))
    (eval (list 'define
                (cons setter-fn (cons newval-sym orig-vars))
                (cons 'begin body)))
    setter-fn))

;; get-setf-expansion: returns (vars vals store-vars setter-form) for a place form
(define (get-setf-expansion place)
  (cond
    ((and (pair? place) (symbol? (car place)))
     (let ((accessor (car place))
           (args (cdr place)))
       (let ((vars (mapcar (lambda (_) (gensym "gs-")) args))
             (store-var (gensym "store-")))
         (list vars
               args
               (list store-var)
               (list (string->symbol (string-append (symbol->string accessor) "-SETF"))
                     store-var
                     (cons (quote list) vars))))))
    (else
     (list '() '() (list (gensym "store-")) (quote (progn))))))

;; -------- Standard Condition Classes --------
;; ANSI CL condition hierarchy with appropriate slots and initargs
(defclass condition ()
  ((message :initform nil :initarg :message)
   (format-arguments :initform () :initarg :format-arguments)
   (format-control :initform nil :initarg :format-control)))

(defclass serious-condition (condition) ())

(defclass error (serious-condition) ())
(defclass simple-error (error) ())

(defclass warning (condition) ())
(defclass simple-warning (warning) ())

(defclass type-error (error)
  ((datum :initform nil :initarg :datum)
   (expected-type :initform nil :initarg :expected-type)))
(defclass simple-type-error (type-error) ())

(defclass program-error (error) ())
(defclass division-by-zero (error) ())
(defclass arithmetic-error (error)
  ((operation :initform nil :initarg :operation)
   (operands :initform () :initarg :operands)))
(defclass control-error (error) ())
(defclass stream-error (error)
  ((stream :initform nil :initarg :stream)))
(defclass end-of-file (stream-error) ())
(defclass file-error (error)
  ((pathname :initform nil :initarg :pathname)))
(defclass package-error (condition)
  ((package :initform nil :initarg :package)))
(defclass reader-error (stream-error) ())

(defclass break (serious-condition) ())
(defclass style-warning (warning) ())
(defclass storage-condition (serious-condition) ())
(defclass cell-error (error)
  ((name :initform nil :initarg :name)))
(defclass unbound-variable (cell-error) ())
(defclass unbound-slot (cell-error)
  ((instance :initform nil :initarg :instance)))
(defclass undefined-function (cell-error) ())
(defclass parse-error (error) ())
(defclass print-not-readable (error)
  ((object :initform nil :initarg :object)))
(defclass simple-condition (condition)
  ((format-control :initform nil :initarg :format-control)
   (format-arguments :initform () :initarg :format-arguments)))

;; -------- Condition Accessor Functions --------
(define (condition-message c) (slot-value c 'message))
(define (condition-format-string c) (slot-value c 'format-control))
(define (cell-error-name c) (slot-value c 'name))
(define (unbound-slot-instance c) (slot-value c 'instance))
(define (print-not-readable-object c) (slot-value c 'object))
(define (simple-condition-format-control c) (slot-value c 'format-control))
(define (simple-condition-format-arguments c) (slot-value c 'format-arguments))

;; -------- make-condition helper --------
;; make-condition is aliased to builtinMakeInstance (which calls make-instance).
;; Standard condition classes are defined above as CLOS defclass forms.

;; Exported condition classes:
;; condition, serious-condition, error, simple-error
;; warning, simple-warning, type-error, simple-type-error
;; program-error, division-by-zero, arithmetic-error
;; control-error, stream-error, end-of-file
;; file-error, package-error, reader-error

;; -------- with-condition-restarts macro --------
(defmacro with-condition-restarts (cond-form restarts-form &body body)
  (let ((cond-var (gensym "cond"))
        (restarts-var (gensym "restarts")))
    (list 'let
          (list (list cond-var cond-form)
                (list restarts-var restarts-form))
          (list 'unwind-protect
                (list 'progn
                      (list '%associate-restarts-with-condition cond-var restarts-var)
                      (cons 'progn body))
                (list '%dissociate-restarts-with-condition cond-var restarts-var)))))

;; -------- Standard Condition Accessor Functions --------
(defun type-error-datum (c) (slot-value c 'datum))
(defun type-error-expected-type (c) (slot-value c 'expected-type))
(defun stream-error-stream (c) (slot-value c 'stream))
(defun file-error-pathname (c) (slot-value c 'pathname))
(defun arithmetic-error-operation (c) (slot-value c 'operation))
(defun arithmetic-error-operands (c) (slot-value c 'operands))
(defun package-error-package (c) (slot-value c 'package))

;; -------- with-standard-io-syntax --------
;; (with-standard-io-syntax &body body)
;; Binds all standard read/write variables to their ANSI CL defaults,
;; executes body, then restores the original values.
(define-macro (with-standard-io-syntax . body)
  (list 'let
        (list (list '*old-print-base* '*print-base*)
              (list '*old-print-case* '*print-case*)
              (list '*old-print-radix* '*print-radix*)
              (list '*old-print-escape* '*print-escape*)
              (list '*old-print-circle* '*print-circle*)
              (list '*old-print-pretty* '*print-pretty*)
              (list '*old-print-length* '*print-length*)
              (list '*old-print-level* '*print-level*)
              (list '*old-print-readably* '*print-readably*)
              (list '*old-print-gensym* '*print-gensym*)
              (list '*old-print-array* '*print-array*)
              (list '*old-read-base* '*read-base*)
              (list '*old-read-default-float-format* '*read-default-float-format*)
              (list '*old-read-eval* '*read-eval*)
              (list '*old-read-suppress* '*read-suppress*))
        (list 'unwind-protect
              (list 'progn
                    (list 'set! '*print-base* 10)
                    (list 'set! '*print-case* (list 'quote ':upcase))
                    (list 'set! '*print-radix* 'nil)
                    (list 'set! '*print-escape* #t)
                    (list 'set! '*print-circle* 'nil)
                    (list 'set! '*print-pretty* 'nil)
                    (list 'set! '*print-length* 'nil)
                    (list 'set! '*print-level* 'nil)
                    (list 'set! '*print-readably* #t)
                    (list 'set! '*print-gensym* #t)
                    (list 'set! '*print-array* #t)
                    (list 'set! '*read-base* 10)
                    (list 'set! '*read-default-float-format* (list 'quote 'single-float))
                    (list 'set! '*read-eval* #t)
                    (list 'set! '*read-suppress* 'nil)
                    (cons 'progn body))
              (list 'progn
                    (list 'set! '*print-base* '*old-print-base*)
                    (list 'set! '*print-case* '*old-print-case*)
                    (list 'set! '*print-radix* '*old-print-radix*)
                    (list 'set! '*print-escape* '*old-print-escape*)
                    (list 'set! '*print-circle* '*old-print-circle*)
                    (list 'set! '*print-pretty* '*old-print-pretty*)
                    (list 'set! '*print-length* '*old-print-length*)
                    (list 'set! '*print-level* '*old-print-level*)
                    (list 'set! '*print-readably* '*old-print-readably*)
                    (list 'set! '*print-gensym* '*old-print-gensym*)
                    (list 'set! '*print-array* '*old-print-array*)
                    (list 'set! '*read-base* '*old-read-base*)
                    (list 'set! '*read-default-float-format* '*old-read-default-float-format*)
                    (list 'set! '*read-eval* '*old-read-eval*)
                    (list 'set! '*read-suppress* '*old-read-suppress*)))))

;; -------- with-hash-table-iterator --------
;; (with-hash-table-iterator (name hash-table) &body body)
;; Creates a local macro NAME that on each call returns (values key value t)
;; or (values nil nil nil) at end.
(define-macro (with-hash-table-iterator spec . body)
  (let ((name (car spec))
        (ht (cadr spec))
        (keys-var (gensym))
        (idx-var (gensym)))
    (list 'let (list (list keys-var (list 'hash-table-keys ht))
                     (list idx-var 0))
      (list 'macrolet
            (list (list name '()
                  (list 'let (list (list 'k (list 'nth idx-var keys-var)))
                    (list 'if (list '< idx-var (list 'length keys-var))
                          (list 'progn
                                (list 'set! idx-var (list '+ idx-var 1))
                                (list 'values 'k (list 'gethash 'k ht) #t))
                          (list 'values 'nil 'nil 'nil)))))
            (cons 'progn body)))))

;; -------- internal-time-units-per-second --------
(define (internal-time-units-per-second) 1000000)

;; -------- get-decoded-time --------
;; Convenience: calls decode-universal-time on current universal time
(define (get-decoded-time)
  (decode-universal-time (get-universal-time)))

;; -------- with-open-stream --------
;; (with-open-stream (var stream-form) &body body)
(define-macro (with-open-stream spec . body)
  (let ((var (car spec))
        (form (cadr spec)))
    (list 'let (list (list var form))
      (list 'unwind-protect
            (cons 'progn body)
            (list 'if var (list 'close var))))))

;; -------- with-input-from-string --------
;; (with-input-from-string (var string) &body body)
(define-macro (with-input-from-string spec . body)
  (let ((var (car spec))
        (str (cadr spec)))
    (list 'let (list (list var (list 'make-string-input-stream str)))
      (list 'unwind-protect
            (cons 'progn body)
            (list 'close var)))))

;; -------- with-output-to-string --------
;; (with-output-to-string (var) &body body)
(define-macro (with-output-to-string spec . body)
  (let ((var (car spec)))
    (list 'let (list (list var (list 'make-string-output-stream)))
      (cons 'progn (append body
        (list (list 'get-output-stream-string var)))))))

;; -------- pprint --------
;; Pretty-print a form to the current output stream
(define (pprint obj)
  (let ((*print-pretty* #t))
    (princ obj)
    (terpri)
    obj))

;; -------- pprint-newline --------
(define (pprint-newline kind) nil)

;; -------- pprint-dispatch --------
(define (pprint-dispatch object) nil)

;; -------- pprint-tab --------
(define (pprint-tab kind col1 col2) nil)

;; -------- pprint-logical-block --------
(define-macro (pprint-logical-block spec . body)
  (cons 'progn body))

;; -------- pprint-pop / pprint-exit-if-list-exhausted --------
(define (pprint-pop list) nil)
(define (pprint-exit-if-list-exhausted) nil)

;; -------- with-package-iterator --------
(define-macro (with-package-iterator spec . body)
  (cons 'progn body))

;; -------- pp (alias for pprint) --------
(define (pp obj)
  (let ((*print-pretty* #t))
    (princ obj)
    (terpri)
    obj))

;; -------- copy-structure (Lisp-level wrapper) --------
;; copy-structure is implemented as a Go builtin

`

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
			_, err := loadFile(os.Args[1], globalEnv)
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
