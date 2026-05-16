package microlisp

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

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
		// readExpr returning nil value without error means an unmatched
		// close parenthesis was encountered — report as syntax error.
		if v == nil {
			return nil, fmt.Errorf("syntax error: unmatched close parenthesis")
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
	// Resource safety check — aborts if step/time/memory limits exceeded
	if err := stepCheck(); err != nil {
		return nil, err
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
				if target.car == nil {
					return nil, fmt.Errorf("setf: empty accessor")
				}
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
				return LoadFile(fname, env)
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
	// Resource safety check
	if err := stepCheck(); err != nil {
		return nil, err
	}
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
