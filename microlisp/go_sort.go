package microlisp

import (
	"fmt"
	"reflect"
	"sort"
)

// lispSortInterface implements sort.Interface by delegating to Lisp functions.
type lispSortInterface struct {
	n      int
	lenFn  *Value // optional: if nil, n is used directly
	lessFn *Value
	swapFn *Value
	data   []interface{} // underlying data for in-place mutation
}

func (s *lispSortInterface) Len() int {
	if s.lenFn != nil {
		result, err := Eval(makeApplyForm(s.lenFn, nil), globalEnv)
		if err != nil {
			panic(fmt.Sprintf("sort-interface Len: %v", err))
		}
		if !isNumeric(result) {
			return 0
		}
		return int(toNum(result))
	}
	return s.n
}

func (s *lispSortInterface) Less(i, j int) bool {
	args := []*Value{vnum(float64(i)), vnum(float64(j))}
	result, err := Eval(makeApplyForm(s.lessFn, args), globalEnv)
	if err != nil {
		panic(fmt.Sprintf("sort-interface Less: %v", err))
	}
	return result.typ == VBool && result == globalEnv.bindings["#t"]
}

func (s *lispSortInterface) Swap(i, j int) {
	args := []*Value{vnum(float64(i)), vnum(float64(j))}
	_, err := Eval(makeApplyForm(s.swapFn, args), globalEnv)
	if err != nil {
		panic(fmt.Sprintf("sort-interface Swap: %v", err))
	}
}

// builtinSortInterface creates a sort.Interface from Lisp callbacks.
// (sort-interface n less-fn swap-fn) or (sort-interface len-fn less-fn swap-fn)
func builtinSortInterface(args []*Value) (*Value, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("sort-interface: need n, less-fn, swap-fn")
	}

	var n int
	var lenFn *Value

	if isNumeric(args[0]) {
		n = int(toNum(args[0]))
		lenFn = nil
	} else {
		lenFn = args[0]
		// Call len-fn to get n
		result, err := Eval(makeApplyForm(lenFn, nil), globalEnv)
		if err != nil {
			return nil, fmt.Errorf("sort-interface: len-fn error: %v", err)
		}
		if !isNumeric(result) {
			return nil, fmt.Errorf("sort-interface: len-fn must return a number")
		}
		n = int(toNum(result))
	}

	lessFn := args[1]
	swapFn := args[2]

	// Create mutable data slice for in-place sorting
	data := make([]interface{}, n)
	for i := 0; i < n; i++ {
		data[i] = i
	}

	si := &lispSortInterface{
		n:      n,
		lenFn:  lenFn,
		lessFn: lessFn,
		swapFn: swapFn,
		data:   data,
	}

	return &Value{
		typ:       VGoVal,
		goVal:     si,
		goValType: reflect.TypeOf((*sort.Interface)(nil)).Elem(),
	}, nil
}

// builtinSortWithInterface sorts a Lisp list using Go's sort.Sort with a Lisp less function.
// (sort-list lst less-fn) — mutates lst in place
func builtinSortWithInterface(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("sort-list: need list and less-fn")
	}

	lst := args[0]
	if !isList(lst) && lst.typ != VArray {
		return nil, fmt.Errorf("sort-list: first arg must be a list or vector")
	}

	lessFn := args[1]

	// Convert to slice
	var elems []*Value
	if lst.typ == VArray {
		elems = make([]*Value, len(lst.array.elements))
		copy(elems, lst.array.elements)
	} else {
		for p := lst; !isNil(p); p = p.cdr {
			elems = append(elems, p.car)
		}
	}

	// Sort using Go's sort.Slice
	sort.SliceStable(elems, func(i, j int) bool {
		a := elems[i]
		b := elems[j]
		lessArgs := []*Value{a, b}
		result, err := Eval(makeApplyForm(lessFn, lessArgs), globalEnv)
		if err != nil {
			panic(fmt.Sprintf("sort-list less-fn: %v", err))
		}
		return result.typ == VBool && result == globalEnv.bindings["#t"]
	})

	// Return sorted list
	return listFromSlice(elems), nil
}

// builtinSortIsSortedWith checks if a list is sorted using a Lisp less function.
// (sorted-p lst less-fn)
func builtinSortIsSortedWith(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("sorted-p: need list and less-fn")
	}

	lst := args[0]
	lessFn := args[1]

	var elems []*Value
	if lst.typ == VArray {
		elems = lst.array.elements
	} else {
		for p := lst; !isNil(p); p = p.cdr {
			elems = append(elems, p.car)
		}
	}

	for i := 1; i < len(elems); i++ {
		lessArgs := []*Value{elems[i], elems[i-1]}
		result, err := Eval(makeApplyForm(lessFn, lessArgs), globalEnv)
		if err != nil {
			return nil, fmt.Errorf("sorted-p less-fn: %v", err)
		}
		if result.typ == VBool && result == globalEnv.bindings["#t"] {
			return vbool(false), nil
		}
	}
	return vbool(true), nil
}

func init() {
	// These are registered as builtins because they take *Value parameters
}
