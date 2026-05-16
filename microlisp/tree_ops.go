package microlisp

import "fmt"

func substHelper(new, old, tree *Value) *Value {
	if eqVal(tree, old) {
		return new
	}
	if tree.typ == VPair {
		// Cycle detection: use map-based tracking to prevent infinite loops on circular structures
		visited := make(map[*Value]bool)
		return substHelperCycle(new, old, tree, visited)
	}
	return tree
}

func substHelperCycle(new, old, tree *Value, visited map[*Value]bool) *Value {
	if eqVal(tree, old) {
		return new
	}
	if tree.typ != VPair {
		return tree
	}
	if visited[tree] {
		return tree // cycle detected, stop recursion
	}
	visited[tree] = true
	return cons(substHelperCycle(new, old, tree.car, visited), substHelperCycle(new, old, tree.cdr, visited))
}

func builtinSubst(args []*Value) (*Value, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("subst: need new, old, and tree")
	}
	return substHelper(args[0], args[1], args[2]), nil
}

func sublisHelper(alist, tree *Value) *Value {
	visited := make(map[*Value]bool)
	return sublisHelperCycle(alist, tree, visited)
}

func sublisHelperCycle(alist, tree *Value, visited map[*Value]bool) *Value {
	if tree.typ == VPair {
		if visited[tree] {
			return tree // cycle detected
		}
		visited[tree] = true
		// Check if tree itself is a key in alist
		for cur := alist; !isNil(cur) && cur.typ == VPair; cur = cur.cdr {
			pair := cur.car
			if pair.typ == VPair && eqVal(pair.car, tree) {
				return pair.cdr
			}
		}
		return cons(sublisHelperCycle(alist, tree.car, visited), sublisHelperCycle(alist, tree.cdr, visited))
	}
	// For atoms, check alist
	for cur := alist; !isNil(cur) && cur.typ == VPair; cur = cur.cdr {
		pair := cur.car
		if pair.typ == VPair && eqVal(pair.car, tree) {
			return pair.cdr
		}
	}
	return tree
}

func builtinSublis(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("sublis: need alist and tree")
	}
	return sublisHelper(args[0], args[1]), nil
}

func builtinTreeEqual(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return vbool(false), nil
	}
	// Cycle detection to prevent infinite recursion on circular structures
	visitedA := make(map[*Value]bool)
	visitedB := make(map[*Value]bool)
	var teq func(a, b *Value) bool
	teq = func(a, b *Value) bool {
		if a == b {
			return true
		}
		if visitedA[a] || visitedB[b] {
			return true // already compared, assume equal to avoid infinite loop
		}
		if a.typ == VPair {
			visitedA[a] = true
		}
		if b.typ == VPair {
			visitedB[b] = true
		}
		if a.typ == VPair && b.typ == VPair {
			return teq(a.car, b.car) && teq(a.cdr, b.cdr)
		}
		if a.typ == VNil && b.typ == VNil {
			return true
		}
		return eqVal(a, b)
	}
	return vbool(teq(args[0], args[1])), nil
}

func builtinSubstIf(args []*Value) (*Value, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("subst-if: need new, predicate, and tree")
	}
	newVal := args[0]
	pred := args[1]
	tree := args[2]
	visited := make(map[*Value]bool)
	var helper func(t *Value) *Value
	helper = func(t *Value) *Value {
		if visited[t] {
			return t
		}
		if t.typ == VPair {
			visited[t] = true
		}
		result, err := callFnOnSeq(pred, []*Value{t}, globalEnv)
		if err == nil && isTruthy(result) {
			return newVal
		}
		if t.typ == VPair {
			return cons(helper(t.car), helper(t.cdr))
		}
		return t
	}
	return helper(tree), nil
}

func builtinSubstIfNot(args []*Value) (*Value, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("subst-if-not: need new, predicate, and tree")
	}
	newVal := args[0]
	pred := args[1]
	tree := args[2]
	visited := make(map[*Value]bool)
	var helper func(t *Value) *Value
	helper = func(t *Value) *Value {
		if visited[t] {
			return t
		}
		if t.typ == VPair {
			visited[t] = true
		}
		result, err := callFnOnSeq(pred, []*Value{t}, globalEnv)
		if err == nil && !isTruthy(result) {
			return newVal
		}
		if t.typ == VPair {
			return cons(helper(t.car), helper(t.cdr))
		}
		return t
	}
	return helper(tree), nil
}

func builtinNsubst(args []*Value) (*Value, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("nsubst: need new, old, and tree")
	}
	newVal := args[0]
	old := args[1]
	visited := make(map[*Value]bool)
	var helper func(t *Value) *Value
	helper = func(t *Value) *Value {
		if eqVal(t, old) {
			return newVal
		}
		if t.typ == VPair {
			if visited[t] {
				return t
			}
			visited[t] = true
			t.car = helper(t.car)
			t.cdr = helper(t.cdr)
		}
		return t
	}
	return helper(args[2]), nil
}

func builtinNsubstIf(args []*Value) (*Value, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("nsubst-if: need new, predicate, and tree")
	}
	newVal := args[0]
	pred := args[1]
	visited := make(map[*Value]bool)
	var helper func(t *Value) *Value
	helper = func(t *Value) *Value {
		result, err := callFnOnSeq(pred, []*Value{t}, globalEnv)
		if err == nil && isTruthy(result) {
			return newVal
		}
		if t.typ == VPair {
			if visited[t] {
				return t
			}
			visited[t] = true
			t.car = helper(t.car)
			t.cdr = helper(t.cdr)
		}
		return t
	}
	return helper(args[2]), nil
}

func builtinNsubstIfNot(args []*Value) (*Value, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("nsubst-if-not: need new, predicate, and tree")
	}
	newVal := args[0]
	pred := args[1]
	visited := make(map[*Value]bool)
	var helper func(t *Value) *Value
	helper = func(t *Value) *Value {
		result, err := callFnOnSeq(pred, []*Value{t}, globalEnv)
		if err == nil && !isTruthy(result) {
			return newVal
		}
		if t.typ == VPair {
			if visited[t] {
				return t
			}
			visited[t] = true
			t.car = helper(t.car)
			t.cdr = helper(t.cdr)
		}
		return t
	}
	return helper(args[2]), nil
}

func builtinNsublis(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("nsublis: need alist and tree")
	}
	alist := args[0]
	visited := make(map[*Value]bool)
	var helper func(t *Value) *Value
	helper = func(t *Value) *Value {
		for cur := alist; !isNil(cur) && cur.typ == VPair; cur = cur.cdr {
			pair := cur.car
			if pair.typ == VPair && eqVal(pair.car, t) {
				return pair.cdr
			}
		}
		if t.typ == VPair {
			if visited[t] {
				return t
			}
			visited[t] = true
			t.car = helper(t.car)
			t.cdr = helper(t.cdr)
		}
		return t
	}
	return helper(args[1]), nil
}
