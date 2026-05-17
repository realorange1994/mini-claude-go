package microlisp

import (
	"fmt"
)

func builtinRest(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	if args[0] == nil || args[0].typ != VPair {
		return vnil(), nil
	}
	return args[0].cdr, nil
}

func builtinFirst(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	v := args[0]
	if v.typ == VPair {
		return v.car, nil
	}
	return vnil(), nil
}

func builtinSecond(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	v := args[0]
	for i := 0; i < 1; i++ {
		if v == nil || v.typ != VPair {
			return vnil(), nil
		}
		v = v.cdr
	}
	if v != nil && v.typ == VPair {
		return v.car, nil
	}
	return vnil(), nil
}

func builtinThird(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	v := args[0]
	for i := 0; i < 2; i++ {
		if v == nil || v.typ != VPair {
			return vnil(), nil
		}
		v = v.cdr
	}
	if v != nil && v.typ == VPair {
		return v.car, nil
	}
	return vnil(), nil
}

func builtinFourth(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	v := args[0]
	for i := 0; i < 3; i++ {
		if v == nil || v.typ != VPair {
			return vnil(), nil
		}
		v = v.cdr
	}
	if v != nil && v.typ == VPair {
		return v.car, nil
	}
	return vnil(), nil
}

func builtinFifth(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	v := args[0]
	for i := 0; i < 4; i++ {
		if v == nil || v.typ != VPair {
			return vnil(), nil
		}
		v = v.cdr
	}
	if v != nil && v.typ == VPair {
		return v.car, nil
	}
	return vnil(), nil
}

func builtinSixth(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	v := args[0]
	for i := 0; i < 5; i++ {
		if v == nil || v.typ != VPair {
			return vnil(), nil
		}
		v = v.cdr
	}
	if v != nil && v.typ == VPair {
		return v.car, nil
	}
	return vnil(), nil
}

func builtinSeventh(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	v := args[0]
	for i := 0; i < 6; i++ {
		if v == nil || v.typ != VPair {
			return vnil(), nil
		}
		v = v.cdr
	}
	if v != nil && v.typ == VPair {
		return v.car, nil
	}
	return vnil(), nil
}

func builtinEighth(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	v := args[0]
	for i := 0; i < 7; i++ {
		if v == nil || v.typ != VPair {
			return vnil(), nil
		}
		v = v.cdr
	}
	if v != nil && v.typ == VPair {
		return v.car, nil
	}
	return vnil(), nil
}

func builtinNinth(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	v := args[0]
	for i := 0; i < 8; i++ {
		if v == nil || v.typ != VPair {
			return vnil(), nil
		}
		v = v.cdr
	}
	if v != nil && v.typ == VPair {
		return v.car, nil
	}
	return vnil(), nil
}

func builtinTenth(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	v := args[0]
	for i := 0; i < 9; i++ {
		if v == nil || v.typ != VPair {
			return vnil(), nil
		}
		v = v.cdr
	}
	if v != nil && v.typ == VPair {
		return v.car, nil
	}
	return vnil(), nil
}

func builtinCopyList(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	elems := seqToList(args[0])
	result := make([]*Value, len(elems))
	copy(result, elems)
	return listFromSlice(result), nil
}

func copyTreeHelper(v *Value) *Value {
	return copyTreeCycle(v, make(map[*Value]bool))
}

func copyTreeCycle(v *Value, seen map[*Value]bool) *Value {
	if v == nil || v.typ != VPair {
		return v
	}
	if seen[v] {
		// Cycle detected — create a self-referencing pair by returning nil
		return vnil()
	}
	seen[v] = true
	return cons(copyTreeCycle(v.car, seen), copyTreeCycle(v.cdr, seen))
}

func builtinCopyTree(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	return copyTreeHelper(args[0]), nil
}

func builtinListLength(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnum(0), nil
	}
	count := 0
	v := args[0]
	seen := make(map[*Value]bool)
	for v != nil && v.typ == VPair {
		if seen[v] {
			return nil, fmt.Errorf("list-length: circular list")
		}
		seen[v] = true
		count++
		v = v.cdr
	}
	return vnum(float64(count)), nil
}

func builtinLast(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	n := 1
	if len(args) > 1 {
		n = int(toNum(args[1]))
	}
	v := args[0]
	var elems []*Value
	seen := make(map[*Value]bool)
	for v != nil && v.typ == VPair {
		if seen[v] {
			return nil, fmt.Errorf("last: circular list")
		}
		seen[v] = true
		elems = append(elems, v)
		v = v.cdr
	}
	if n <= 0 || len(elems) == 0 {
		return vnil(), nil
	}
	if n >= len(elems) {
		return args[0], nil
	}
	return elems[len(elems)-n], nil
}

func builtinLastPair(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	v := args[0]
	if v.typ != VPair {
		return vnil(), nil
	}
	seen := make(map[*Value]bool)
	for v.typ == VPair && !isNil(v.cdr) && v.cdr.typ == VPair {
		if seen[v] {
			return nil, fmt.Errorf("last-pair: circular list")
		}
		seen[v] = true
		v = v.cdr
	}
	return v, nil
}

func builtinButlast(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	n := 1
	if len(args) > 1 {
		n = int(toNum(args[1]))
	}
	list := args[0]
	if n <= 0 {
		// butlast with n=0 returns a copy of the list (including dotted tail)
		var result *Value = vnil()
		var tail *Value
		c := list
		for !isNil(c) && c.typ == VPair {
			newCell := cons(c.car, vnil())
			if result == nil || result.typ != VPair {
				result = newCell
				tail = newCell
			} else {
				tail.cdr = newCell
				tail = newCell
			}
			c = c.cdr
		}
		// Preserve dotted tail
		if !isNil(c) && result.typ == VPair {
			// find last pair
			last := result
			for last.cdr.typ == VPair {
				last = last.cdr
			}
			last.cdr = c
		}
		return result, nil
	}
	// n > 0: walk list counting elements
	// When n > 0, the dotted tail is part of the last cons cells being removed,
	// so we should NOT preserve it in the result.
	cur := list
	var elems []*Value
	for cur.typ == VPair {
		elems = append(elems, cur.car)
		cur = cur.cdr
	}
	if n >= len(elems) {
		return vnil(), nil
	}
	keep := len(elems) - n
	// Rebuild with first 'keep' elements as a proper list
	result := vnil()
	for i := keep - 1; i >= 0; i-- {
		result = cons(elems[i], result)
	}
	return result, nil
}

func builtinNbutlast(args []*Value) (*Value, error) {
	return builtinButlast(args)
}

func builtinPairlis(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("pairlis: need two lists")
	}
	keys := seqToList(args[0])
	vals := seqToList(args[1])
	var result []*Value
	for i := 0; i < len(keys) && i < len(vals); i++ {
		result = append(result, cons(keys[i], vals[i]))
	}
	alist := vnil()
	if len(args) >= 3 {
		alist = args[2]
	}
	// Append result to alist: (nconc result alist)
	if len(result) == 0 {
		return alist, nil
	}
	res := listFromSlice(result)
	// Find end of result list and set cdr to alist
	t := res
	for t.typ == VPair && !isNil(t.cdr) {
		t = t.cdr
	}
	t.cdr = alist
	return res, nil
}

func builtinAssoc(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("assoc: need item and alist")
	}
	item := args[0]
	alist := args[1]
	keyFn, testFn, testNotFn, _, _, _, _, _, err := seqParseKeys(args, 2)
	if err != nil {
		return nil, err
	}
	cur := alist
	for !isNil(cur) && cur.typ == VPair {
		entry := cur.car
		// Skip nil elements (per CL spec)
		if isNil(entry) {
			cur = cur.cdr
			continue
		}
		if entry.typ == VPair {
			if testItemMatchFull(item, entry.car, testFn, testNotFn, keyFn) {
				return entry, nil
			}
		}
		cur = cur.cdr
	}
	return vnil(), nil
}

func builtinAssocIf(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("assoc-if: need predicate and alist")
	}
	fn := args[0]
	alist := args[1]
	seen := make(map[*Value]bool)
	for !isNil(alist) && alist.typ == VPair {
		if seen[alist] {
			break
		}
		seen[alist] = true
		pair := alist.car
		if pair.typ == VPair {
			result, err := callFnOnSeq(fn, []*Value{pair.car}, globalEnv)
			if err != nil {
				return nil, err
			}
			if isTruthy(result) {
				return pair, nil
			}
		}
		alist = alist.cdr
	}
	return vnil(), nil
}

func builtinMemberIf(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("member-if: need predicate and list")
	}
	fn := args[0]
	lst := args[1]
	seen := make(map[*Value]bool)
	for !isNil(lst) && lst.typ == VPair {
		if seen[lst] {
			break
		}
		seen[lst] = true
		result, err := callFnOnSeq(fn, []*Value{lst.car}, globalEnv)
		if err != nil {
			return nil, err
		}
		if isTruthy(result) {
			return lst, nil
		}
		lst = lst.cdr
	}
	return vnil(), nil
}

func builtinMemberIfNot(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("member-if-not: need predicate and list")
	}
	fn := args[0]
	lst := args[1]
	seen := make(map[*Value]bool)
	for !isNil(lst) && lst.typ == VPair {
		if seen[lst] {
			break
		}
		seen[lst] = true
		result, err := callFnOnSeq(fn, []*Value{lst.car}, globalEnv)
		if err != nil {
			return nil, err
		}
		if !isTruthy(result) {
			return lst, nil
		}
		lst = lst.cdr
	}
	return vnil(), nil
}

func builtinAssocIfNot(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("assoc-if-not: need predicate and alist")
	}
	fn := args[0]
	alist := args[1]
	seen := make(map[*Value]bool)
	for !isNil(alist) && alist.typ == VPair {
		if seen[alist] {
			break
		}
		seen[alist] = true
		pair := alist.car
		if pair.typ == VPair {
			result, err := callFnOnSeq(fn, []*Value{pair.car}, globalEnv)
			if err != nil {
				return nil, err
			}
			if !isTruthy(result) {
				return pair, nil
			}
		}
		alist = alist.cdr
	}
	return vnil(), nil
}

func builtinRassocIfNot(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("rassoc-if-not: need predicate and alist")
	}
	fn := args[0]
	alist := args[1]
	cur := alist
	for cur != nil && cur.typ == VPair {
		entry := cur.car
		if entry.typ == VPair {
			result, err := callFnOnSeq(fn, []*Value{entry.cdr}, globalEnv)
			if err != nil {
				return nil, err
			}
			if !isTruthy(result) {
				return entry, nil
			}
		}
		cur = cur.cdr
	}
	return vnil(), nil
}

func builtinMember(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("member: need item and list")
	}
	item := args[0]
	lst := args[1]
	if lst.typ != VPair && lst.typ != VNil {
		return nil, fmt.Errorf("member: expected a proper list")
	}
	keyFn, testFn, testNotFn, _, _, _, _, _, err := seqParseKeys(args, 2)
	if err != nil {
		return nil, err
	}
	seen := make(map[*Value]bool)
	for !isNil(lst) && lst.typ == VPair {
		if seen[lst] {
			break
		}
		seen[lst] = true
		el := lst.car
		if testItemMatchFull(item, el, testFn, testNotFn, keyFn) {
			return lst, nil
		}
		lst = lst.cdr
	}
	return vnil(), nil
}

func builtinPosition(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("position: need item and sequence")
	}
	item := args[0]
	seq := args[1]
	if seq.typ != VStr {
		if err := checkSequenceArg(seq, "position"); err != nil {
			return nil, err
		}
	}
	start, end := 0, -1
	for i := 2; i < len(args); i++ {
		if args[i].typ == VSym {
			switch args[i].str {
			case ":START":
				if i+1 < len(args) {
					i++
					n, e := safeToNum(args[i], "position")
					if e != nil {
						return nil, e
					}
					start = int(n)
				}
			case ":END":
				if i+1 < len(args) {
					i++
					n, e := safeToNum(args[i], "position")
					if e != nil {
						return nil, e
					}
					end = int(n)
				}
			}
		}
	}
	if seq.typ == VStr {
		s := seq.str
		runes := []rune(s)
		var targetCh rune
		if item.typ == VChar {
			targetCh = item.ch
		} else {
			return vnil(), nil
		}
		if end < 0 || end > len(runes) {
			end = len(runes)
		}
		for i := start; i < end; i++ {
			if runes[i] == targetCh {
				return vnum(float64(i)), nil
			}
		}
		return vnil(), nil
	}
	elems := seqToList(seq)
	if end < 0 || end > len(elems) {
		end = len(elems)
	}
	for i := start; i < end; i++ {
		if eqVal(elems[i], item) {
			return vnum(float64(i)), nil
		}
	}
	return vnil(), nil
}

func builtinPositionIf(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("position-if: need predicate and sequence")
	}
	fn := args[0]
	seq := args[1]
	elems := seqToList(seq)
	for i, el := range elems {
		result, err := callFnOnSeq(fn, []*Value{el}, globalEnv)
		if err != nil {
			return nil, err
		}
		if isTruthy(result) {
			return vnum(float64(i)), nil
		}
	}
	return vnil(), nil
}

func builtinAcons(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("acons: need key, value, and alist")
	}
	key := args[0]
	val := args[1]
	alist := vnil()
	if len(args) >= 3 {
		alist = args[2]
	}
	return cons(cons(key, val), alist), nil
}

func builtinRassoc(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("rassoc: need item and alist")
	}
	item := args[0]
	alist := args[1]
	pred := vnil()
	for i := 2; i < len(args); i++ {
		if args[i].typ == VSym && args[i].str == ":TEST" && i+1 < len(args) {
			i++
			pred = args[i]
		}
	}
	cur := alist
	for cur != nil && cur.typ == VPair {
		entry := cur.car
		if entry.typ == VPair {
			if pred.typ != VNil {
				cmp, err := callFnOnSeq(pred, []*Value{entry.cdr, item}, globalEnv)
				if err == nil && isTruthy(cmp) {
					return entry, nil
				}
			} else {
				if eqVal(entry.cdr, item) {
					return entry, nil
				}
			}
		}
		cur = cur.cdr
	}
	return vnil(), nil
}

func builtinRassocIf(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("rassoc-if: need predicate and alist")
	}
	fn := args[0]
	alist := args[1]
	cur := alist
	for cur != nil && cur.typ == VPair {
		entry := cur.car
		if entry.typ == VPair {
			result, err := callFnOnSeq(fn, []*Value{entry.cdr}, globalEnv)
			if err == nil && isTruthy(result) {
				return entry, nil
			}
		}
		cur = cur.cdr
	}
	return vnil(), nil
}

func builtinNth(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("nth: need index and list")
	}
	n, err := safeToNum(args[0], "nth")
	if err != nil {
		return nil, err
	}
	lst := args[1]
	cur := lst
	for i := 0; i < int(n); i++ {
		if cur == nil || cur.typ != VPair {
			return vnil(), nil
		}
		cur = cur.cdr
	}
	if cur == nil || cur.typ != VPair {
		return vnil(), nil
	}
	return cur.car, nil
}

func builtinNthCdr(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("nthcdr: need index and list")
	}
	n, err := safeToNum(args[0], "nthcdr")
	if err != nil {
		return nil, err
	}
	lst := args[1]
	cur := lst
	for i := 0; i < int(n); i++ {
		if cur == nil || cur.typ != VPair {
			return vnil(), nil
		}
		cur = cur.cdr
	}
	return cur, nil
}
func builtinNreconc(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("nreconc: need two lists")
	}
	list1 := seqToList(args[0])
	list2 := seqToList(args[1])
	// Reverse list1 and prepend to list2
	for i, j := 0, len(list1)-1; i < j; i, j = i+1, j-1 {
		list1[i], list1[j] = list1[j], list1[i]
	}
	result := make([]*Value, len(list1)+len(list2))
	copy(result, list1)
	copy(result[len(list1):], list2)
	return listFromSlice(result), nil
}

// -------- concatenate (already exists) --------

// -------- mapl / maplist / mapc / mapcon --------
func builtinMaplist(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("maplist: need function and list")
	}
	fn := args[0]
	lst := args[1]
	var results []*Value
	seen := make(map[*Value]bool)
	cur := lst
	for cur != nil && cur.typ == VPair {
		if seen[cur] {
			break
		}
		seen[cur] = true
		r, err := callFnOnSeq(fn, []*Value{cur}, globalEnv)
		if err != nil {
			return nil, err
		}
		results = append(results, r)
		cur = cur.cdr
	}
	return listFromSlice(results), nil
}

func builtinMapc(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("mapc: need function and list")
	}
	fn := args[0]
	lst := args[1]
	seen := make(map[*Value]bool)
	cur := lst
	for cur != nil && cur.typ == VPair {
		if seen[cur] {
			break
		}
		seen[cur] = true
		_, err := callFnOnSeq(fn, []*Value{cur.car}, globalEnv)
		if err != nil {
			return nil, err
		}
		cur = cur.cdr
	}
	return lst, nil
}

func builtinMapl(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("mapl: need function and list")
	}
	fn := args[0]
	lst := args[1]
	seen := make(map[*Value]bool)
	cur := lst
	for cur != nil && cur.typ == VPair {
		if seen[cur] {
			break
		}
		seen[cur] = true
		_, err := callFnOnSeq(fn, []*Value{cur}, globalEnv)
		if err != nil {
			return nil, err
		}
		cur = cur.cdr
	}
	return lst, nil
}

func builtinMapcon(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("mapcon: need function and list")
	}
	fn := args[0]
	lst := args[1]
	seen := make(map[*Value]bool)
	cur := lst
	// nconc-2: append list2 to end of list1, returning list1 (destructive)
	nconc2 := func(list1, list2 *Value) *Value {
		if isNil(list2) {
			return list1
		}
		if isNil(list1) {
			return list2
		}
		t := list1
		for t.typ == VPair && !isNil(t.cdr) {
			t = t.cdr
		}
		t.cdr = list2
		return list1
	}
	var result *Value
	for !isNil(cur) && cur.typ == VPair {
		if seen[cur] {
			break
		}
		seen[cur] = true
		r, err := callFnOnSeq(fn, []*Value{cur}, globalEnv)
		if err != nil {
			return nil, err
		}
		result = nconc2(result, r)
		cur = cur.cdr
	}
	if result == nil {
		return vnil(), nil
	}
	return result, nil
}

// -------- list* --------
func builtinListStar(args []*Value) (*Value, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("list*: need at least one argument")
	}
	if len(args) == 1 {
		return args[0], nil
	}
	result := make([]*Value, len(args)-1)
	for i := 0; i < len(args)-1; i++ {
		result[i] = args[i]
	}
	return appendList(listFromSlice(result), args[len(args)-1]), nil
}

// appendList appends a tail to a proper list
func appendList(lst, tail *Value) *Value {
	if isNil(lst) {
		return tail
	}
	seen := make(map[*Value]bool)
	return appendListRec(lst, tail, seen)
}
func appendListRec(lst, tail *Value, seen map[*Value]bool) *Value {
	if isNil(lst) {
		return tail
	}
	if seen[lst] {
		return tail // break cycle
	}
	seen[lst] = true
	if lst.typ == VPair {
		return cons(lst.car, appendListRec(lst.cdr, tail, seen))
	}
	return tail
}
func builtinCopyAlist(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("copy-alist: need an alist")
	}
	alist := args[0]
	if isNil(alist) {
		return vnil(), nil
	}
	var result *Value = vnil()
	elems := seqToList(alist)
	for i := len(elems) - 1; i >= 0; i-- {
		entry := elems[i]
		if entry.typ == VPair {
			result = cons(cons(entry.car, entry.cdr), result)
		} else {
			result = cons(entry, result)
		}
	}
	return result, nil
}
func builtinList(args []*Value) (*Value, error) {
	return listFromSlice(args), nil
}

func builtinSetCdrAsSetter(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("setf (cdr): need 2 arguments")
	}
	val := args[0]
	cons := args[1]
	if !isPair(cons) {
		return nil, fmt.Errorf("setf (cdr): not a pair")
	}
	cons.cdr = val
	return val, nil
}

func builtinSetCarAsSetter(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("setf (car): need 2 arguments")
	}
	val := args[0]
	cons := args[1]
	if !isPair(cons) {
		return nil, fmt.Errorf("setf (car): not a pair")
	}
	cons.car = val
	return val, nil
}

func builtinSetCdr(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("set-cdr!: need pair and value")
	}
	if !isPair(args[0]) {
		return nil, fmt.Errorf("set-cdr!: not a pair")
	}
	args[0].cdr = args[1]
	return args[1], nil
}

func builtinSetCar(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("set-car!: need pair and value")
	}
	if !isPair(args[0]) {
		return nil, fmt.Errorf("set-car!: not a pair")
	}
	args[0].car = args[1]
	return args[1], nil
}

func builtinCdr(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("cdr: need 1 argument")
	}
	v := args[0]
	if v != nil && v.typ == VMultiVal {
		v = primaryValue(v)
	}
	if isNil(v) {
		return vnil(), nil
	}
	if !isPair(v) {
		return nil, fmt.Errorf("cdr: not a pair")
	}
	return v.cdr, nil
}

func builtinCar(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("car: need 1 argument")
	}
	v := args[0]
	if v != nil && v.typ == VMultiVal {
		return primaryValue(v), nil
	}
	if isNil(v) {
		return vnil(), nil
	}
	if !isPair(v) {
		return nil, fmt.Errorf("car: not a pair")
	}
	return v.car, nil
}

func builtinCons(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("cons: need 2 arguments")
	}
	return cons(args[0], args[1]), nil
}
func builtinPushnew(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("pushnew: need item and place")
	}
	item := args[0]
	place := args[1]
	testFn := vnil()
	for i := 2; i < len(args); i++ {
		if args[i].typ == VSym && args[i].str == ":TEST" && i+1 < len(args) {
			testFn = args[i+1]
			i++
		}
	}
	if place.typ == VSym {
		currentVal, err := globalEnv.Get(place.str)
		if err != nil {
			currentVal = vnil()
		}
		// Check if item already in list
		for lst := currentVal; lst != nil && lst.typ == VPair; lst = lst.cdr {
			if !isNil(testFn) {
				res, err := callFnOnSeq(testFn, []*Value{item, lst.car}, globalEnv)
				if err != nil {
					return nil, err
				}
				if isTruthy(res) {
					return currentVal, nil
				}
			} else {
				if eqVal(item, lst.car) {
					return currentVal, nil
				}
			}
		}
		newVal := cons(item, currentVal)
		globalEnv.Set(place.str, newVal)
		return newVal, nil
	}
	return nil, fmt.Errorf("pushnew: second argument must be a symbol")
}

func builtinPush(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("push: need item and place")
	}
	item := args[0]
	place := args[1]
	if place.typ == VSym {
		currentVal, err := globalEnv.Get(place.str)
		if err != nil {
			currentVal = vnil()
		}
		newVal := cons(item, currentVal)
		globalEnv.Set(place.str, newVal)
		return newVal, nil
	}
	return nil, fmt.Errorf("push: second argument must be a symbol")
}

func builtinNull(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(true), nil
	}
	return vbool(isNil(args[0])), nil
}

func builtinNthSetf(args []*Value) (*Value, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("setf (nth): need value, n, and list")
	}
	val := args[0]
	n := args[1]
	lst := args[2]
	if n.typ != VNum {
		return nil, fmt.Errorf("setf (nth): index must be a number")
	}
	idx := int(n.num)
	if idx < 0 {
		return nil, fmt.Errorf("setf (nth): index must be non-negative")
	}
	// Walk down to the nth element
	target := lst
	for i := 0; i < idx; i++ {
		if !isPair(target) {
			return nil, fmt.Errorf("setf (nth): index %d out of range", idx)
		}
		target = target.cdr
	}
	if !isPair(target) {
		return nil, fmt.Errorf("setf (nth): index %d out of range", idx)
	}
	target.car = val
	return val, nil
}
