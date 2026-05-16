package microlisp

import (
	"fmt"
	"strings"
)

func builtinStableSort(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("stable-sort: need sequence and predicate")
	}
	seq := args[0]
	pred := args[1]
	keyFn := vnil()
	for i := 2; i < len(args); i++ {
		if args[i].typ == VSym && args[i].str == ":KEY" && i+1 < len(args) {
			i++
			keyFn = args[i]
		}
	}
	elems := seqToList(seq)
	// Insertion sort (stable)
	for i := 1; i < len(elems); i++ {
		key := elems[i]
		var keyVal *Value
		if !isNil(keyFn) {
			var err error
			keyVal, err = callFnOnSeq(keyFn, []*Value{key}, globalEnv)
			if err != nil {
				return nil, err
			}
		} else {
			keyVal = key
		}
		j := i - 1
		for j >= 0 {
			var jVal *Value
			if !isNil(keyFn) {
				var err error
				jVal, err = callFnOnSeq(keyFn, []*Value{elems[j]}, globalEnv)
				if err != nil {
					return nil, err
				}
			} else {
				jVal = elems[j]
			}
			cmp, err := callFnOnSeq(pred, []*Value{keyVal, jVal}, globalEnv)
			if err != nil {
				return nil, err
			}
			if !isTruthy(cmp) {
				break
			}
			elems[j+1] = elems[j]
			j--
		}
		elems[j+1] = key
	}
	if seq.typ == VStr {
		var sb strings.Builder
		for _, v := range elems {
			sb.WriteString(ToString(v))
		}
		return vstr(sb.String()), nil
	}
	return listFromSlice(elems), nil
}

func builtinUnion(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("union: need two lists")
	}
	list1 := seqToList(args[0])
	list2 := seqToList(args[1])
	seen := make(map[string]bool)
	var result []*Value
	for _, v := range list1 {
		key := ToString(v)
		if !seen[key] {
			seen[key] = true
			result = append(result, v)
		}
	}
	for _, v := range list2 {
		key := ToString(v)
		if !seen[key] {
			seen[key] = true
			result = append(result, v)
		}
	}
	return listFromSlice(result), nil
}

func builtinIntersection(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("intersection: need two lists")
	}
	list1 := seqToList(args[0])
	list2 := seqToList(args[1])
	set2 := make(map[string]bool)
	for _, v := range list2 {
		set2[ToString(v)] = true
	}
	seen := make(map[string]bool)
	var result []*Value
	for _, v := range list1 {
		key := ToString(v)
		if set2[key] && !seen[key] {
			seen[key] = true
			result = append(result, v)
		}
	}
	return listFromSlice(result), nil
}

func builtinSetDifference(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("set-difference: need two lists")
	}
	list1 := seqToList(args[0])
	list2 := seqToList(args[1])
	set2 := make(map[string]bool)
	for _, v := range list2 {
		set2[ToString(v)] = true
	}
	var result []*Value
	for _, v := range list1 {
		if !set2[ToString(v)] {
			result = append(result, v)
		}
	}
	return listFromSlice(result), nil
}

func builtinSetExclusiveOr(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("set-exclusive-or: need two lists")
	}
	list1 := seqToList(args[0])
	list2 := seqToList(args[1])
	set1 := make(map[string]bool)
	set2 := make(map[string]bool)
	for _, v := range list1 {
		set1[ToString(v)] = true
	}
	for _, v := range list2 {
		set2[ToString(v)] = true
	}
	var result []*Value
	for _, v := range list1 {
		if !set2[ToString(v)] {
			result = append(result, v)
		}
	}
	for _, v := range list2 {
		if !set1[ToString(v)] {
			result = append(result, v)
		}
	}
	return listFromSlice(result), nil
}

func builtinNsetExclusiveOr(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("nset-exclusive-or: need two lists")
	}
	list1 := seqToList(args[0])
	list2 := seqToList(args[1])
	set1 := make(map[string]bool)
	set2 := make(map[string]bool)
	for _, v := range list1 {
		set1[ToString(v)] = true
	}
	for _, v := range list2 {
		set2[ToString(v)] = true
	}
	var result []*Value
	for _, v := range list1 {
		if !set2[ToString(v)] {
			result = append(result, v)
		}
	}
	for _, v := range list2 {
		if !set1[ToString(v)] {
			result = append(result, v)
		}
	}
	return listFromSlice(result), nil
}

func builtinSubsetp(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return vbool(false), nil
	}
	list1 := seqToList(args[0])
	list2 := seqToList(args[1])
	set2 := make(map[string]bool)
	for _, v := range list2 {
		set2[ToString(v)] = true
	}
	for _, v := range list1 {
		if !set2[ToString(v)] {
			return vbool(false), nil
		}
	}
	return vbool(true), nil
}
