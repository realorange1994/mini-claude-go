package microlisp

import (
	"fmt"
	"strings"
)

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
	// Resource safety check
	if err := stepCheck(); err != nil {
		return nil, err
	}
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
