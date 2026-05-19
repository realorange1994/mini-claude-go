package microlisp

import (
	"fmt"
	"reflect"
	"strings"
)

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
			depth := traceDepth + 1 // CL trace is 1-indexed
			if len(argStrs) > 0 {
				fmt.Printf("%s%d: (%s %s)\n", indent, depth, fn.name, strings.Join(argStrs, " "))
			} else {
				fmt.Printf("%s%d: (%s)\n", indent, depth, fn.name)
			}
			traceDepth++
			defer func() {
				indent2 := strings.Repeat("  ", traceDepth)
				depth2 := traceDepth + 1
				fmt.Printf("%s%d: %s returned %s\n", indent2, depth2, fn.name, ToString(result))
				traceDepth--
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
	case VGoVal:
		// Go function value — call via reflect
		if fn.goValType == nil || fn.goValType.Kind() != reflect.Func {
			return nil, fmt.Errorf("not a procedure: %s", typeStr(fn))
		}
		fnType := fn.goValType
		argSlice := toSlice(args)
		goArgs := make([]reflect.Value, fnType.NumIn())
		for i := 0; i < fnType.NumIn(); i++ {
			paramType := fnType.In(i)
			if fnType.IsVariadic() && i == fnType.NumIn()-1 {
				paramType = paramType.Elem()
			}
			if i < len(argSlice) {
				val, _ := lispToReflectSafe(argSlice[i], paramType)
				goArgs[i] = val
			} else {
				goArgs[i] = reflect.Zero(paramType)
			}
		}
		var results []reflect.Value
		if fnType.IsVariadic() {
			results = fn.goValReflect.CallSlice(goArgs)
		} else {
			results = fn.goValReflect.Call(goArgs)
		}
		switch len(results) {
		case 0:
			return vnil(), nil
		case 1:
			return reflectToLisp(results[0]), nil
		default:
			lispResults := make([]*Value, len(results))
			for i, r := range results {
				lispResults[i] = reflectToLisp(r)
			}
			return listFromSlice(lispResults), nil
		}
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
func builtinFuncall(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("funcall: need function")
	}
	fn := args[0]
	callArgs := args[1:]
	return callFnOnSeq(fn, callArgs, globalEnv)
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

func bindPattern(pattern *Value, val *Value, env *Env) error {
	return bindPatternRec(pattern, val, env, make(map[*Value]bool))
}
