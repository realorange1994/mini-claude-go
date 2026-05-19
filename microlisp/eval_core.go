package microlisp

// Eval is the core evaluator — a single large function that cannot be safely
// split without restructuring how special forms share closure state.
// It handles: control flow, binding, iteration, macro, CLOS, and all other forms.

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// -------- Evaluator --------

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
				// Lazy-load hook: check GoFFILazyTable for auto-import
				if loaded, loadErr := tryLazyGoImport(v.str, env); loadErr == nil && loaded != nil {
					return loaded, nil
				}
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
						// Lazy-load hook
						if loaded, loadErr := tryLazyGoImport(arg.str, env); loadErr == nil && loaded != nil {
							return loaded, nil
						}
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
						// CL spec: defvar returns the value when initform is provided
						return ev, nil
					}
				}
				// defvar without initial value or already bound: return symbol
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
				// CL spec: defparameter returns the value of the initform
				return ev, nil
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
						return nil, fmt.Errorf("letrec: malformed bindings")
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
						// Support implicit lambda: (name lambda-list body...)
						if !isNil(b.cdr.cdr) && b.cdr.cdr.typ == VPair {
							// (name lambda-list body...) — construct a VFunc
							fparams := b.cdr.car
							fbody := b.cdr.cdr
							params, rest, optDefaults, keySpecs, e := parseParams(fparams)
							if e != nil {
								return nil, fmt.Errorf("letrec: %v", e)
							}
							fn := gcv()
							fn.typ = VFunc
							fn.params = params
							fn.rest = rest
							fn.optDefaults = optDefaults
							fn.keySpecs = keySpecs
							fn.body = fbody
							// fn.env will be set after newEnv is created
							vals = append(vals, fn)
						} else {
							// Simple (name value) binding
							vals = append(vals, b.cdr.car)
						}
					} else {
						vals = append(vals, vnil())
					}
					bindings = bindings.cdr
				}
				newEnv := &Env{parent: env, bindings: make(map[string]*Value)}
				// Pre-bind function stubs for recursion support
				for i, name := range names {
					if vals[i] != nil && vals[i].typ == VFunc {
						vals[i].env = newEnv
					}
					newEnv.bindings[name] = vals[i]
				}
				// Evaluate initforms (non-function values need evaluation)
				for i, val := range vals {
					if val != nil && val.typ != VFunc {
						evald, e := Eval(val, newEnv)
						if e != nil {
							return nil, e
						}
						newEnv.bindings[names[i]] = evald
					}
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
				var result *Value = vnil()
				for !isNil(body) {
					stmt := body.car
					if stmt.typ != VSym && stmt.typ != VNum {
						var ev *Value
						ev, err = Eval(stmt, env)
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
						result = ev
					} else {
						body = body.cdr
						continue
					}
					body = body.cdr
				}
				return result, nil
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
								// Not a handledError — check for control flow errors
								var br *blockReturn
								if b, ok := r.(*blockReturn); ok {
									br = b
								} else if e, ok := r.(error); ok {
									br, _ = e.(*blockReturn)
								}
								if br != nil {
									// blockReturn from handler-case body
									hcErr = br
									return
								}
								var tc *tailCall
								if t, ok := r.(*tailCall); ok {
									tc = t
								} else if e, ok := r.(error); ok {
									tc, _ = e.(*tailCall)
								}
								if tc != nil {
									hcErr = tc
									return
								}
								var tv *throwValue
								if t, ok := r.(*throwValue); ok {
									tv = t
								} else if e, ok := r.(error); ok {
									tv, _ = e.(*throwValue)
								}
								if tv != nil {
									hcErr = tv
									return
								}
								// Genuine panic — re-panic
								panic(r)
							}
						}
						handlerStack = handlerStack[:savedLen]
					}()
					hcResult, hcErr = Eval(valForm, env)
				}()
				// If eval returned a Go error, check for control flow errors first
				if hcErr != nil {
					if br, ok := hcErr.(*blockReturn); ok && br.name == "NIL" {
						// handler-case has implicit block nil — resolve it
						hcResult = br.value
						hcErr = nil
					} else if isControlFlowError(hcErr) {
						return nil, hcErr
					}
				}
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
						// :no-error vars receive the result values as a list (CL spec)
						valuesList := listFromSlice([]*Value{hcResult})
						for i, vname := range noErrorVars {
							if i == 0 {
								noErrorEnv.Set(vname, valuesList)
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
							// Catch blockReturn for "NIL" and use its value as result
							// Note: control flow errors may be wrapped in error interface
							var br *blockReturn
							if b, ok := r.(*blockReturn); ok {
								br = b
							} else if e, ok := r.(error); ok {
								br, _ = e.(*blockReturn)
							}
							if br != nil && br.name == "NIL" {
								hbResult = br.value
								hbErr = nil
								return
							}
							// handledError means handler ran but didn't return-from;
							// the condition is still being signaled, so propagate as error
							var he *handledError
							if h, ok := r.(*handledError); ok {
								he = h
							} else if e, ok := r.(error); ok {
								he, _ = e.(*handledError)
							}
							if he != nil {
								// handler ran but didn't return-from; propagate as error
								hbErr = fmt.Errorf("%s", princToString(he.condition))
								return
							}
							// Propagate blockReturn for other names, tailCall, and throwValue
							// as error returns (not panics) so outer block/catch can handle them
							switch v := r.(type) {
							case *blockReturn:
								hbErr = v
								return
							case *tailCall:
								hbErr = v
								return
							case *throwValue:
								hbErr = v
								return
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
				// (setf place1 newval1 place2 newval2 ...)
				// Collect all place-value pairs and evaluate them in order
				if v.cdr == nil || v.cdr.typ != VPair || v.cdr.cdr == nil || v.cdr.cdr.typ != VPair {
					return nil, fmt.Errorf("setf: malformed form")
				}
				var pairs []*Value
				cur := v.cdr
				for !isNil(cur) && cur.typ == VPair {
					place := cur.car
					if cur.cdr == nil || cur.cdr.typ != VPair {
						return nil, fmt.Errorf("setf: odd number of arguments")
					}
					val := cur.cdr.car
					pairs = append(pairs, place, val)
					cur = cur.cdr.cdr
				}
				if len(pairs) < 2 {
					return nil, fmt.Errorf("setf: malformed form")
				}
				var result *Value
				for i := 0; i < len(pairs); i += 2 {
					target := pairs[i]
					newValExpr := pairs[i+1]
					if target.typ == VSym {
						name := target.str
						ev, e := Eval(newValExpr, env)
						if e != nil {
							return nil, e
						}
						found := false
						for scope := env; scope != nil; scope = scope.parent {
							if _, ok := scope.bindings[name]; ok {
								scope.bindings[name] = ev
								result = ev
								found = true
								break
							}
						}
						if !found {
							globalEnv.Set(name, ev)
							result = ev
						}
					} else if target.typ == VPair && target.car != nil && target.car.typ == VSym && target.car.str == "VALUES" {
						vals, err := Eval(newValExpr, env)
						if err != nil {
							return nil, err
						}
						valList := multiValList(vals)
						places := target.cdr
						placeIdx := 0
						for ; !isNil(places); places = places.cdr {
							val := vnil()
							cur2 := valList
							for j := 0; cur2 != nil && cur2.typ == VPair && j < placeIdx; j++ {
								cur2 = cur2.cdr
							}
							if cur2 != nil && cur2.typ == VPair && cur2.car != nil {
								val = cur2.car
							}
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
					} else if target.typ == VPair {
						if target.car == nil {
							return nil, fmt.Errorf("setf: empty accessor")
						}
						accessorName := target.car.str
						args := target.cdr
						setter, err := env.Get(accessorName + "-setf")
						if err != nil {
							return nil, fmt.Errorf("setf: no setter for %s", accessorName)
						}
						callArgs := &Value{typ: VPair, car: newValExpr, cdr: vnil()}
						if !isNil(args) {
							tail := callArgs
							for ; !isNil(args); args = args.cdr {
								tail.cdr = &Value{typ: VPair, car: args.car, cdr: vnil()}
								tail = tail.cdr
							}
						}
						r, err := Eval(&Value{typ: VPair, car: setter, cdr: callArgs}, env)
						if err != nil {
							return nil, err
						}
						result = r
					} else {
						return nil, fmt.Errorf("setf: target must be a list or symbol")
					}
				}
				if result == nil {
					result = vnil()
				}
				return result, nil
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
