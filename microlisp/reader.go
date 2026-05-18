package microlisp

import (
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
	"unicode"
)

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

func vnum(f float64) *Value   { v := gcv(); v.typ = VNum; v.num = f; return v }
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
func vchar(ch rune) *Value        { v := gcv(); v.typ = VChar; v.ch = ch; return v }
func vnil() *Value                { return globalEnv.bindings["nil"] }
func cons(a, b *Value) *Value     { v := batchAlloc(); v.typ = VPair; v.car = a; v.cdr = b; return v }
func list3(a, b, c *Value) *Value { return cons(a, cons(b, cons(c, vnil()))) }

func isNil(v *Value) bool {
	return v == nil || v.typ == VNil || (v.typ == VSym && strings.EqualFold(v.str, "nil"))
}

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
	TVector     // #( elements ) vector literal
	TMacro      // readtable macro character
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
			if depth > 0 {
				// unclosed vector - no closing paren found
				l.tok = Tok{typ: TErr, lit: "unclosed vector literal #(", pos: p}
				return l.tok
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
		// Handle overflow: if the number is too large for float64,
		// return positive or negative infinity as a float.
		if numStr != "" && (numStr[0] == '-' || numStr[0] == '+') {
			sign := numStr[0]
			// Check if it's just an overflow (not a parse error)
			absStr := numStr[1:]
			if _, e2 := strconv.ParseFloat(absStr, 64); e2 != nil {
				if sign == '-' {
					l.tok = Tok{typ: TNum, num: math.Inf(-1), pos: start, isFlt: true}
				} else {
					l.tok = Tok{typ: TNum, num: math.Inf(1), pos: start, isFlt: true}
				}
				return l.tok
			}
		} else if numStr != "" {
			l.tok = Tok{typ: TNum, num: math.Inf(1), pos: start, isFlt: true}
			return l.tok
		}
		return l.lexSymFrom(start)
	}
	if math.IsInf(f, 0) {
		// Preserve infinity from overflow
		l.tok = Tok{typ: TNum, num: f, pos: start, isFlt: true}
		return l.tok
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
		"soh": '\x01',
		"stx": '\x02',
		"etx": '\x03',
		"eot": '\x04',
		"enq": '\x05',
		"ack": '\x06',
		"nak": '\x15',
		"syn": '\x16',
		"etb": '\x17',
		"can": '\x18',
		"em":  '\x19',
		"sub": '\x1a',
		"fs":  '\x1c',
		"gs":  '\x1d',
		"rs":  '\x1e',
		"us":  '\x1f',
		"del": '\x7f',
		// Common abbreviations
		"xoff": '\x13',
		"xon":  '\x11',
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
