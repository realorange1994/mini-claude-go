package microlisp

import (
	"fmt"
	"strings"
)

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

func checkSequenceArg(v *Value, funcName string) error {
	v = primaryValue(v)
	if v.typ != VPair && v.typ != VNil && v.typ != VStr && v.typ != VArray {
		return fmt.Errorf("%s: expected a proper sequence", funcName)
	}
	return nil
}

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
		return nil, fmt.Errorf("ldiff: need two arguments")
	}
	list := args[0]
	obj := args[1]
	// If first arg is not a list, return nil per CL spec
	if list.typ != VPair && !isNil(list) {
		return vnil(), nil
	}
	// Convert obj to element list for structural comparison
	objElems := seqToList(obj)
	var result []*Value
	seen := make(map[*Value]bool)
	for !isNil(list) && list.typ == VPair {
		if seen[list] {
			break
		}
		seen[list] = true
		// Collect elements from current tail position
		var tailElems []*Value
		cur := list
		tailSeen := make(map[*Value]bool)
		for !isNil(cur) && cur.typ == VPair {
			if tailSeen[cur] {
				break
			}
			tailSeen[cur] = true
			tailElems = append(tailElems, cur.car)
			cur = cur.cdr
		}
		if len(tailElems) == len(objElems) {
			match := true
			for i := range objElems {
				if !eqVal(objElems[i], tailElems[i]) {
					match = false
					break
				}
			}
			if match {
				break
			}
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
	// CL spec: tailp uses eq (pointer identity) to check if sublist is a tail.
	// Since our reader always allocates fresh cons cells for quoted lists,
	// we check if sublist's elements match a tail's elements structurally.
	subElems := seqToList(sublist)
	seen := make(map[*Value]bool)
	for !isNil(list) && list.typ == VPair {
		if seen[list] {
			break
		}
		seen[list] = true
		// Collect elements from current tail position
		var tailElems []*Value
		cur := list
		tailSeen := make(map[*Value]bool)
		for !isNil(cur) && cur.typ == VPair {
			if tailSeen[cur] {
				break
			}
			tailSeen[cur] = true
			tailElems = append(tailElems, cur.car)
			cur = cur.cdr
		}
		if len(tailElems) == len(subElems) {
			match := true
			for i := range subElems {
				if !eqVal(subElems[i], tailElems[i]) {
					match = false
					break
				}
			}
			if match {
				return vbool(true), nil
			}
		}
		list = list.cdr
	}
	// Also check if both are nil (nil is a tail of every proper list)
	return vbool(isNil(list) && isNil(sublist)), nil
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
