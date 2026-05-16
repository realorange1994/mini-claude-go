package microlisp

import (
	"fmt"
	"strings"
)

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
				if count >= 0 && replaced >= count {
					break
				}
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
				if count >= 0 && replaced >= count {
					break
				}
				if testItemMatch(oldVal, elems[i], testFn, keyFn) {
					elems[i] = newVal
					replaced++
				}
			}
		} else {
			for i := start; i < end; i++ {
				if count >= 0 && replaced >= count {
					break
				}
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

	// Destructive: modify .car of matching cells in-place
	replaced := 0
	if fromEnd {
		// First, build index of elements
		seen := make(map[*Value]bool)
		var elements []*Value
		var cellPtrs []*Value
		for cur := seq; !isNil(cur) && cur.typ == VPair; {
			if seen[cur] {
				break
			}
			seen[cur] = true
			elements = append(elements, cur.car)
			cellPtrs = append(cellPtrs, cur)
			cur = cur.cdr
		}
		for i := end - 1; i >= start; i-- {
			if count >= 0 && replaced >= count {
				break
			}
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
