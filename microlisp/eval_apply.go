package microlisp

import (
	"fmt"
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
