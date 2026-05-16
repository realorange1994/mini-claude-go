package microlisp

import "fmt"

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
