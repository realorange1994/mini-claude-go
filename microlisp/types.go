package microlisp

import (
	"bufio"
	"bytes"
	"fmt"
	"math"
	"math/big"
	"math/rand"
	"os"
	"strings"
	"sync"
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
	typ         ValType
	mark        byte
	gen         int
	num         float64
	isFloat     bool // true when value was explicitly created as a floating-point number
	ch          rune
	irat        int64
	iden        int64
	imag        float64
	str         string
	name        string // function name for trace
	car         *Value
	cdr         *Value
	params      []string
	rest        string
	whole       string   // &whole binding
	optDefaults []*Value // default expressions for &optional params (nil = default to NIL)
	keySpecs    []*Value // (key-name param-name default) triples for &key params
	body        *Value
	env         *Env
	fn          NativeFunc

	// CLOS fields
	className    string
	classSlots   []string
	classParents []*Value
	cpl          []*Value
	genMethods   []genMethod
	methodCombo  string // method combination type: "standard", "progn", "and", "or", "list", "append", "nconc", "min", "max", "+"
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
	randState    *rand.Rand    // for VRandomState type
	methodGF     *Value        // generic function for VMethod type
	methodIdx    int           // method index in genMethods for VMethod type
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

func (tv *throwValue) Error() string { return fmt.Sprintf("<throw to tag: %s>", tv.tag) }

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
var signaledCondition *Value       // tracks the currently-being-handled condition
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
	globalEnv.Set("*query-io*", stdinStream)     // Default query-io uses stdin/stdout
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
