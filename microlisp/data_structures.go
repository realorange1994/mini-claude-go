package microlisp

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// -------- Array helpers --------

func equalpArray(a, b *Value) bool {
	if len(a.array.dims) != len(b.array.dims) {
		return false
	}
	for i := range a.array.dims {
		if a.array.dims[i] != b.array.dims[i] {
			return false
		}
	}
	if len(a.array.elements) != len(b.array.elements) {
		return false
	}
	for i := range a.array.elements {
		if !equalpVal(a.array.elements[i], b.array.elements[i]) {
			return false
		}
	}
	return true
}

func arrayToString(v *Value) string {
	if v.array == nil {
		return "#<array nil>"
	}
	arr := v.array
	if len(arr.dims) == 1 && arr.dims[0] == len(arr.elements) {
		// 1-D array (vector)
		parts := []string{"#("}
		for i, elem := range arr.elements {
			if i > 0 {
				parts = append(parts, " ")
			}
			if elem == nil || elem.typ == VNil {
				parts = append(parts, "NIL")
			} else if elem.typ == VStr {
				parts = append(parts, "\""+elem.str+"\"")
			} else {
				parts = append(parts, ToString(elem))
			}
		}
		parts = append(parts, ")")
		return strings.Join(parts, "")
	}
	// Multi-dimensional array
	dimStr := make([]string, len(arr.dims))
	for i, d := range arr.dims {
		dimStr[i] = strconv.Itoa(d)
	}
	return "#<" + strings.Join(dimStr, "x") + "-array>"
}

// Row-major index for given subscripts
func arrayRowMajorIndex(arr *LispArray, indices []int) (int, error) {
	if len(indices) != len(arr.dims) {
		return 0, fmt.Errorf("array access: expected %d subscripts, got %d", len(arr.dims), len(indices))
	}
	idx := 0
	stride := 1
	for i := len(arr.dims) - 1; i >= 0; i-- {
		if indices[i] < 0 || indices[i] >= arr.dims[i] {
			return 0, fmt.Errorf("array index %d out of bounds [0..%d] for dimension %d", indices[i], arr.dims[i]-1, i)
		}
		idx += indices[i] * stride
		stride *= arr.dims[i]
	}
	return idx, nil
}

// Total size of array
func arrayTotalSize(dims []int) int {
	size := 1
	for _, d := range dims {
		size *= d
	}
	return size
}

// null:

// -------- Array builtins --------

// arrayFillRecursive flattens nested lists into the flat elements array in row-major order.
func arrayFillRecursive(contents *Value, dims []int, elements []*Value, idx *int, visited map[*Value]bool) {
	if len(dims) == 1 {
		// Leaf level: fill elements directly
		// Support both VPair (list) and VArray (vector) as contents
		if contents.typ == VArray {
			arr := contents.array
			end := len(arr.elements)
			if arr.fillPtr >= 0 && arr.fillPtr < end {
				end = arr.fillPtr
			}
			for i := 0; i < end; i++ {
				if *idx >= len(elements) {
					return
				}
				elem := arr.elements[i]
				if elem == nil {
					elem = vnil()
				}
				elements[*idx] = elem
				*idx++
			}
		} else {
			for !isNil(contents) {
				if *idx >= len(elements) {
					return
				}
				if contents.typ == VPair {
					if visited[contents] {
						return // cycle detected
					}
					visited[contents] = true
				}
				elements[*idx] = contents.car
				*idx++
				contents = contents.cdr
			}
		}
	} else {
		// Nested: recurse into sublists
		// Support both VPair (list) and VArray (vector) as contents
		if contents.typ == VArray {
			arr := contents.array
			end := len(arr.elements)
			if arr.fillPtr >= 0 && arr.fillPtr < end {
				end = arr.fillPtr
			}
			for i := 0; i < end; i++ {
				subDims := dims[1:]
				elem := arr.elements[i]
				if elem == nil {
					elem = vnil()
				}
				arrayFillRecursive(elem, subDims, elements, idx, visited)
			}
		} else {
			for !isNil(contents) {
				if contents.typ == VPair {
					if visited[contents] {
						return // cycle detected
					}
					visited[contents] = true
				}
				subDims := dims[1:]
				arrayFillRecursive(contents.car, subDims, elements, idx, visited)
				contents = contents.cdr
			}
		}
	}
}

func builtinMakeArray(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("make-array: need dimensions")
	}
	dimArg := args[0]
	var dims []int
	if dimArg.typ == VNum {
		dims = []int{int(dimArg.num)}
	} else if dimArg.typ == VPair {
		for !isNil(dimArg) {
			if dimArg.car == nil || dimArg.car.typ != VNum {
				return nil, fmt.Errorf("make-array: dimension must be integer")
			}
			dims = append(dims, int(dimArg.car.num))
			dimArg = dimArg.cdr
		}
	} else {
		return nil, fmt.Errorf("make-array: dimensions must be integer or list")
	}
	if len(dims) == 0 {
		return nil, fmt.Errorf("make-array: need at least one dimension")
	}
	for i, d := range dims {
		if d < 0 {
			return nil, fmt.Errorf("make-array: dimension %d is negative: %d", i, d)
		}
	}
	// Parse keyword arguments
	initialElement := vnil()
	var initialContents *Value = nil
	fillPointer := -1
	adjustable := false
	elemType := "T"
	for i := 1; i < len(args); i++ {
		if args[i].typ == VSym {
			switch args[i].str {
			case ":INITIAL-ELEMENT":
				if i+1 < len(args) {
					i++
					initialElement = args[i]
				}
			case ":INITIAL-CONTENTS":
				if i+1 < len(args) {
					i++
					initialContents = args[i]
				}
			case ":FILL-POINTER":
				if i+1 < len(args) {
					i++
					if args[i] == globalEnv.bindings["#t"] {
						fillPointer = 0
					} else if args[i].typ == VNum {
						fillPointer = int(args[i].num)
					}
				}
			case ":ADJUSTABLE":
				if i+1 < len(args) {
					i++
					adjustable = isTruthy(args[i])
				}
			case ":ELEMENT-TYPE":
				if i+1 < len(args) {
					i++
					etName := strings.ToUpper(ToString(args[i]))
					switch etName {
					case "CHARACTER", "BASE-CHAR", "STANDARD-CHAR":
						elemType = "CHARACTER"
					case "BIT":
						elemType = "BIT"
					case "SINGLE-FLOAT", "DOUBLE-FLOAT", "FLOAT":
						elemType = "SINGLE-FLOAT"
					default:
						elemType = "T"
					}
				}
			}
		}
	}
	size := arrayTotalSize(dims)
	elements := make([]*Value, size)
	if initialContents != nil {
		idx := 0
		arrayFillRecursive(initialContents, dims, elements, &idx, make(map[*Value]bool))
		if idx < size {
			// Fill remaining with nil
			for i := idx; i < size; i++ {
				elements[i] = vnil()
			}
		}
	} else {
		for i := range elements {
			elements[i] = initialElement
		}
	}
	arr := &LispArray{
		dims:       dims,
		elements:   elements,
		fillPtr:    fillPointer,
		adjustable: adjustable,
		elemType:   elemType,
	}
	v := gcv()
	v.typ = VArray
	v.array = arr
	return v, nil
}

func builtinAref(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("aref: need array and subscripts")
	}
	arr := args[0]
	// Strings are also arrays in CL; (aref "hello" 0) returns a character
	if arr.typ == VStr {
		idx := int(args[1].num)
		if idx < 0 || idx >= len(arr.str) {
			return nil, fmt.Errorf("aref: index %d out of bounds for string of length %d", idx, len(arr.str))
		}
		ch := []rune(arr.str)[idx]
		return vchar(ch), nil
	}
	if arr.typ != VArray || arr.array == nil {
		return nil, fmt.Errorf("aref: first argument must be an array")
	}
	indices := make([]int, len(args)-1)
	for i := 1; i < len(args); i++ {
		if args[i].typ != VNum {
			return nil, fmt.Errorf("aref: subscript must be integer")
		}
		indices[i-1] = int(args[i].num)
	}
	idx, err := arrayRowMajorIndex(arr.array, indices)
	if err != nil {
		return nil, err
	}
	return arr.array.elements[idx], nil
}

func builtinSetAref(args []*Value) (*Value, error) {
	// (set-aref value array subscripts...)
	if len(args) < 3 {
		return nil, fmt.Errorf("set-aref: need value, array, and subscripts")
	}
	val := args[0]
	arr := args[1]
	if arr.typ != VArray || arr.array == nil {
		return nil, fmt.Errorf("set-aref: second argument must be an array")
	}
	indices := make([]int, len(args)-2)
	for i := 2; i < len(args); i++ {
		if args[i].typ != VNum {
			return nil, fmt.Errorf("set-aref: subscript must be integer")
		}
		indices[i-2] = int(args[i].num)
	}
	idx, err := arrayRowMajorIndex(arr.array, indices)
	if err != nil {
		return nil, err
	}
	arr.array.elements[idx] = val
	return val, nil
}

// builtinNthSetf is used for (setf (nth n list) val) -> (nth-setf val n list)

// builtinSymbolValueSetf is used for (setf (symbol-value sym) val)

// builtinEltSetf is used for (setf (elt seq n) val)

func builtinVector(args []*Value) (*Value, error) {
	arr := &LispArray{
		dims:     []int{len(args)},
		elements: make([]*Value, len(args)),
		fillPtr:  -1,
	}
	copy(arr.elements, args)
	v := gcv()
	v.typ = VArray
	v.array = arr
	return v, nil
}

func builtinArrayP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	return vbool(args[0].typ == VArray), nil
}

func builtinVectorP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	return vbool(args[0].typ == VArray && len(args[0].array.dims) == 1), nil
}

// isBitVector checks if a VArray contains only 0/1 integers.
func isBitVector(arr *LispArray) bool {
	if arr == nil {
		return false
	}
	for _, el := range arr.elements {
		if el == nil || el.typ == VNil {
			return false
		}
		if el.typ != VNum {
			return false
		}
		v := toNum(vnum(el.num))
		if v != 0 && v != 1 {
			return false
		}
	}
	return true
}

func builtinBitVectorP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	v := args[0]
	return vbool(v.typ == VArray && len(v.array.dims) == 1 && isBitVector(v.array)), nil
}

func builtinSimpleVectorP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	v := args[0]
	if v.typ != VArray || v.array == nil {
		return vbool(false), nil
	}
	// A simple vector is one-dimensional with no fill pointer
	return vbool(len(v.array.dims) == 1 && v.array.fillPtr < 0), nil
}

func builtinSimpleBitVectorP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	v := args[0]
	if v.typ != VArray || v.array == nil {
		return vbool(false), nil
	}
	if len(v.array.dims) != 1 || v.array.fillPtr >= 0 {
		return vbool(false), nil
	}
	return vbool(isBitVector(v.array)), nil
}

func builtinArrayDimensions(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VArray || args[0].array == nil {
		return nil, fmt.Errorf("array-dimensions: need an array")
	}
	result := vnil()
	for i := len(args[0].array.dims) - 1; i >= 0; i-- {
		result = cons(vnum(float64(args[0].array.dims[i])), result)
	}
	return result, nil
}

func builtinArrayDimension(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("array-dimension: need array and axis-number")
	}
	v := args[0]
	if v.typ != VArray {
		return nil, fmt.Errorf("array-dimension: not an array")
	}
	axis := int(toNum(args[1]))
	if axis < 0 || axis >= len(v.array.dims) {
		return nil, fmt.Errorf("array-dimension: axis %d out of range", axis)
	}
	return vnum(float64(v.array.dims[axis])), nil
}

func builtinArrayTotalSize(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VArray || args[0].array == nil {
		return nil, fmt.Errorf("array-total-size: need an array")
	}
	return vnum(float64(len(args[0].array.elements))), nil
}

func builtinArrayRank(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VArray || args[0].array == nil {
		return nil, fmt.Errorf("array-rank: need an array")
	}
	return vnum(float64(len(args[0].array.dims))), nil
}

func builtinFillPointer(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VArray || args[0].array == nil {
		return nil, fmt.Errorf("fill-pointer: need a vector with fill-pointer")
	}
	arr := args[0].array
	if arr.fillPtr < 0 {
		return nil, fmt.Errorf("fill-pointer: array has no fill-pointer")
	}
	return vnum(float64(arr.fillPtr)), nil
}

func builtinSetFillPointer(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("set-fill-pointer: need array and value")
	}
	arr := args[0]
	if arr.typ != VArray || arr.array == nil {
		return nil, fmt.Errorf("set-fill-pointer: first argument must be an array")
	}
	if arr.array.fillPtr < 0 {
		return nil, fmt.Errorf("set-fill-pointer: array has no fill-pointer")
	}
	newVal := int(args[1].num)
	if newVal < 0 || newVal > len(arr.array.elements) {
		return nil, fmt.Errorf("set-fill-pointer: value %d out of range [0..%d]", newVal, len(arr.array.elements))
	}
	arr.array.fillPtr = newVal
	return args[1], nil
}

// fill-pointer-setf: for (setf (fill-pointer arr) val) -> (fill-pointer-setf val arr)
func builtinFillPointerSetf(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("(setf fill-pointer): need array and value")
	}
	newVal := int(args[0].num)
	arr := args[1]
	if arr.typ != VArray || arr.array == nil {
		return nil, fmt.Errorf("(setf fill-pointer): first arg must be an array")
	}
	if arr.array.fillPtr < 0 {
		return nil, fmt.Errorf("(setf fill-pointer): array has no fill-pointer")
	}
	if newVal < 0 || newVal > len(arr.array.elements) {
		return nil, fmt.Errorf("(setf fill-pointer): value %d out of range [0..%d]", newVal, len(arr.array.elements))
	}
	arr.array.fillPtr = newVal
	return args[0], nil
}

func builtinVectorPush(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("vector-push: need element and vector")
	}
	newEl := args[0]
	vec := args[1]
	if vec.typ != VArray || vec.array == nil {
		return nil, fmt.Errorf("vector-push: second argument must be a vector")
	}
	arr := vec.array
	if arr.fillPtr < 0 {
		return nil, fmt.Errorf("vector-push: vector has no fill-pointer")
	}
	if arr.fillPtr >= len(arr.elements) {
		return vnil(), nil // no room
	}
	arr.elements[arr.fillPtr] = newEl
	fp := arr.fillPtr
	arr.fillPtr++
	return vnum(float64(fp)), nil
}

func builtinVectorPop(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VArray || args[0].array == nil {
		return nil, fmt.Errorf("vector-pop: need a vector")
	}
	arr := args[0].array
	if arr.fillPtr <= 0 {
		return nil, fmt.Errorf("vector-pop: fill-pointer is 0")
	}
	arr.fillPtr--
	return arr.elements[arr.fillPtr], nil
}

func builtinArrayElementType(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("array-element-type: need an array")
	}
	arr := args[0]
	if arr.typ == VStr {
		// Strings are vectors of CHARACTER
		return vsym("CHARACTER"), nil
	}
	if arr.typ != VArray || arr.array == nil {
		return nil, fmt.Errorf("array-element-type: argument must be an array")
	}
	et := arr.array.elemType
	if et == "" {
		et = "T"
	}
	return vsym(et), nil
}

func builtinUpgradedArrayElementType(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("upgraded-array-element-type: need a type specifier")
	}
	typeArg := args[0]
	typeName := strings.ToUpper(ToString(typeArg))
	// Map known element types to their upgraded types
	switch typeName {
	case "CHARACTER", "BASE-CHAR", "STANDARD-CHAR":
		return vsym("CHARACTER"), nil
	case "BIT":
		return vsym("BIT"), nil
	case "SINGLE-FLOAT", "DOUBLE-FLOAT", "FLOAT", "SHORT-FLOAT", "LONG-FLOAT":
		return vsym("SINGLE-FLOAT"), nil
	case "FIXNUM", "BIGNUM", "INTEGER", "RATIONAL", "RATIO", "REAL", "NUMBER":
		return vsym("T"), nil
	default:
		return vsym("T"), nil
	}
}

// builtinUpgradedComplexPartType implements CL: UPGRADED-COMPLEX-PART-TYPE
// (upgraded-complex-part-type typespec)
// Returns the element type of the parts of a complex number created with the given typespec.
// In our implementation, float types upgrade to SINGLE-FLOAT; others remain as-is.

func builtinAdjustArray(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("adjust-array: need array and new dimensions")
	}
	arr := args[0]
	if arr.typ != VArray || arr.array == nil {
		return nil, fmt.Errorf("adjust-array: first argument must be an array")
	}
	dimArg := args[1]
	var newDims []int
	if dimArg.typ == VNum {
		newDims = []int{int(dimArg.num)}
	} else if dimArg.typ == VPair {
		for !isNil(dimArg) {
			if dimArg.car == nil || dimArg.car.typ != VNum {
				return nil, fmt.Errorf("adjust-array: dimension must be integer")
			}
			newDims = append(newDims, int(dimArg.car.num))
			dimArg = dimArg.cdr
		}
	}
	initialElement := vnil()
	for i := 2; i < len(args); i++ {
		if args[i].typ == VSym && args[i].str == ":INITIAL-ELEMENT" && i+1 < len(args) {
			i++
			initialElement = args[i]
		}
	}
	newSize := arrayTotalSize(newDims)
	newElements := make([]*Value, newSize)
	copy(newElements, arr.array.elements)
	for i := len(arr.array.elements); i < newSize; i++ {
		newElements[i] = initialElement
	}
	arr.array.dims = newDims
	arr.array.elements = newElements
	return arr, nil
}

func builtinArrayHasFillPointerP(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VArray || args[0].array == nil {
		return vbool(false), nil
	}
	return vbool(args[0].array.fillPtr >= 0), nil
}

func builtinAdjustableArrayP(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VArray || args[0].array == nil {
		return vbool(false), nil
	}
	return vbool(args[0].array.adjustable), nil
}

func builtinArrayDisplacement(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VArray || args[0].array == nil {
		return multiVal(vnil(), vbool(false)), nil
	}
	// microlisp does not support displaced arrays
	return multiVal(vnil(), vbool(false)), nil
}

func builtinArrayInBoundsP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("array-in-bounds-p: need an array")
	}
	v := args[0]
	if v.typ != VArray || v.array == nil {
		return nil, fmt.Errorf("array-in-bounds-p: not an array")
	}
	if len(args)-1 != len(v.array.dims) {
		return vbool(false), nil
	}
	for i := 0; i < len(v.array.dims); i++ {
		idx := int(toNum(args[i+1]))
		if idx < 0 || idx >= v.array.dims[i] {
			return vbool(false), nil
		}
	}
	return vbool(true), nil
}

func builtinArrayRowMajorIndex(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("array-row-major-index: need an array")
	}
	v := args[0]
	if v.typ != VArray || v.array == nil {
		return nil, fmt.Errorf("array-row-major-index: not an array")
	}
	if len(args)-1 != len(v.array.dims) {
		return nil, fmt.Errorf("array-row-major-index: wrong number of subscripts")
	}
	idx := 0
	for i := 0; i < len(v.array.dims); i++ {
		sub := int(toNum(args[i+1]))
		if sub < 0 || sub >= v.array.dims[i] {
			return nil, fmt.Errorf("array-row-major-index: subscript %d out of range", i)
		}
		idx = idx*v.array.dims[i] + sub
	}
	return vnum(float64(idx)), nil
}

// -------- Hash Table support --------

func sxhashVal(v *Value) uint64 {
	return sxhashSeen(v, make(map[*Value]bool))
}

func sxhashSeen(v *Value, seen map[*Value]bool) uint64 {
	if seen[v] {
		return 0
	}
	seen[v] = true
	switch v.typ {
	case VNil:
		return 0
	case VBool:
		if v == globalEnv.bindings["#t"] {
			return 1
		}
		return 2
	case VNum:
		return uint64(math.Float64bits(v.num))
	case VRat:
		return uint64(v.irat)*31 + uint64(v.iden)
	case VComplex:
		return uint64(math.Float64bits(v.num))*31 + uint64(math.Float64bits(v.imag))
	case VStr, VSym:
		h := uint64(0)
		for i := 0; i < len(v.str); i++ {
			h = h*31 + uint64(v.str[i])
		}
		return h
	case VPair:
		h := uint64(0x9e3779b97f4a7c15) // golden ratio constant for mixing
		h ^= sxhashSeen(v.car, seen)
		h *= 31
		h += sxhashSeen(v.cdr, seen)
		h ^= (h >> 33)
		return h
	case VArray:
		if v.array == nil {
			return 0
		}
		h := uint64(0x517cc1b727220a95) // different seed for arrays
		for _, elem := range v.array.elements {
			h = h*31 + sxhashSeen(elem, seen)
		}
		h ^= (h >> 33)
		return h
	case VVHash:
		return 3 // pointer-based
	default:
		return 3
	}
}

func hashTableKeyEqual(ht *HashTable, a, b *Value) bool {
	if ht.testFn == nil || ht.testFn.typ == VSym {
		// Default: use eqVal (like eql)
		return eqVal(a, b)
	}
	// Call the test function with pre-evaluated arguments
	result, err := callWithValueArgs(ht.testFn, []*Value{a, b})
	if err != nil {
		return false
	}
	return !isNil(result)
}

// callWithValueArgs calls a function with already-evaluated argument values

func builtinSxhash(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("sxhash: need argument")
	}
	return vnum(float64(sxhashVal(args[0]))), nil
}

func builtinMakeHashTable(args []*Value) (*Value, error) {
	ht := &HashTable{
		table:           make(map[uint64][]*hashEntry),
		rehashSize:      1.5,
		rehashThreshold: 1.0,
	}
	// Parse keyword args
	for i := 0; i < len(args); i++ {
		if args[i].typ == VSym && strings.HasPrefix(args[i].str, ":") {
			switch args[i].str {
			case ":TEST":
				if i+1 < len(args) {
					i++
					tv := args[i]
					if tv.typ != VSym && tv.typ != VPrim && tv.typ != VFunc && tv.typ != VGeneric {
						return nil, fmt.Errorf("make-hash-table: :test must be a function or symbol")
					}
					ht.testFn = args[i]
				}
			case ":HASH-FUNCTION":
				if i+1 < len(args) {
					i++
					ht.hashFn = args[i]
				}
			case ":REHASH-SIZE":
				if i+1 < len(args) {
					i++
					rsz := toNum(primaryValue(args[i]))
					if rsz > 0 {
						ht.rehashSize = rsz
					}
				}
			case ":REHASH-THRESHOLD":
				if i+1 < len(args) {
					i++
					rt := toNum(primaryValue(args[i]))
					if rt > 0 {
						ht.rehashThreshold = rt
					}
				}
			}
		}
	}
	v := gcv()
	v.typ = VVHash
	v.hashTab = ht
	return v, nil
}

func builtinHashTableP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	return vbool(args[0].typ == VVHash), nil
}

func builtinGethash(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("gethash: need key and hash-table")
	}
	key := args[0]
	ht := args[1]
	defaultVal := vnil()
	if len(args) > 2 {
		defaultVal = args[2]
	}
	if ht.typ != VVHash || ht.hashTab == nil {
		return multiVal(defaultVal, vnil()), nil
	}
	h := sxhashVal(key)
	bucket := ht.hashTab.table[h]
	for _, entry := range bucket {
		if hashTableKeyEqual(ht.hashTab, entry.key, key) {
			return multiVal(entry.value, vbool(true)), nil
		}
	}
	return multiVal(defaultVal, vnil()), nil
}

func builtinSetGethash(args []*Value) (*Value, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("gethash-setf: need value, key, and hash-table")
	}
	val := args[0]
	key := args[1]
	ht := args[2]
	if ht.typ != VVHash || ht.hashTab == nil {
		return nil, fmt.Errorf("gethash-setf: not a hash table")
	}
	h := sxhashVal(key)
	bucket := ht.hashTab.table[h]
	for i, entry := range bucket {
		if hashTableKeyEqual(ht.hashTab, entry.key, key) {
			bucket[i].value = val
			ht.hashTab.table[h] = bucket
			return val, nil
		}
	}
	// New entry
	entry := &hashEntry{key: key, value: val}
	ht.hashTab.table[h] = append(bucket, entry)
	ht.hashTab.count++
	return val, nil
}

func builtinRemhash(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("remhash: need key and hash-table")
	}
	key := args[0]
	ht := args[1]
	if ht.typ != VVHash || ht.hashTab == nil {
		return vnil(), nil
	}
	h := sxhashVal(key)
	bucket := ht.hashTab.table[h]
	for i, entry := range bucket {
		if hashTableKeyEqual(ht.hashTab, entry.key, key) {
			ht.hashTab.table[h] = append(bucket[:i], bucket[i+1:]...)
			ht.hashTab.count--
			return vbool(true), nil
		}
	}
	return vnil(), nil
}

func builtinHashTableExists(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("hash-table-exists?: need hash-table and key")
	}
	ht := args[0]
	key := args[1]
	if ht.typ != VVHash || ht.hashTab == nil {
		return vbool(false), nil
	}
	h := sxhashVal(key)
	bucket := ht.hashTab.table[h]
	for _, entry := range bucket {
		if hashTableKeyEqual(ht.hashTab, entry.key, key) {
			return vbool(true), nil
		}
	}
	return vbool(false), nil
}

func builtinClrhash(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("clrhash: need hash-table")
	}
	ht := args[0]
	if ht.typ != VVHash || ht.hashTab == nil {
		return nil, fmt.Errorf("clrhash: not a hash table")
	}
	ht.hashTab.table = make(map[uint64][]*hashEntry)
	ht.hashTab.count = 0
	return args[0], nil
}

func builtinHashTableCount(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("hash-table-count: need hash-table")
	}
	ht := args[0]
	if ht.typ != VVHash || ht.hashTab == nil {
		return nil, fmt.Errorf("hash-table-count: not a hash table")
	}
	return vnum(float64(ht.hashTab.count)), nil
}

func builtinMaphash(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("maphash: need function and hash-table")
	}
	fn := args[0]
	if fn.typ != VPrim && fn.typ != VFunc && fn.typ != VGeneric {
		return nil, fmt.Errorf("maphash: first argument must be a function, got %s", typeStr(fn))
	}
	ht := args[1]
	if ht.typ != VVHash || ht.hashTab == nil {
		return nil, fmt.Errorf("maphash: not a hash table")
	}
	for _, bucket := range ht.hashTab.table {
		for _, entry := range bucket {
			callArgs := []*Value{entry.key, entry.value}
			_, err := callWithValueArgs(fn, callArgs)
			if err != nil {
				return nil, err
			}
		}
	}
	return vnil(), nil
}

func builtinHashTableSize(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("hash-table-size: need hash-table")
	}
	ht := args[0]
	if ht.typ != VVHash || ht.hashTab == nil {
		return nil, fmt.Errorf("hash-table-size: not a hash table")
	}
	return vnum(float64(len(ht.hashTab.table))), nil
}

func builtinHashTableTest(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("hash-table-test: need hash-table")
	}
	ht := args[0]
	if ht.typ != VVHash || ht.hashTab == nil {
		return nil, fmt.Errorf("hash-table-test: not a hash table")
	}
	if ht.hashTab.testFn != nil && !isNil(ht.hashTab.testFn) {
		return ht.hashTab.testFn, nil
	}
	return vsym("eql"), nil
}

func builtinHashTableKeys(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("hash-table-keys: need hash-table")
	}
	ht := args[0]
	if ht.typ != VVHash || ht.hashTab == nil {
		return nil, fmt.Errorf("hash-table-keys: not a hash table")
	}
	var result *Value
	for _, bucket := range ht.hashTab.table {
		for _, entry := range bucket {
			result = cons(entry.key, result)
		}
	}
	return result, nil
}

func builtinHashTableValues(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("hash-table-values: need hash-table")
	}
	ht := args[0]
	if ht.typ != VVHash || ht.hashTab == nil {
		return nil, fmt.Errorf("hash-table-values: not a hash table")
	}
	var result *Value
	for _, bucket := range ht.hashTab.table {
		for _, entry := range bucket {
			result = cons(entry.value, result)
		}
	}
	return result, nil
}

func builtinHashTableRehashSize(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("hash-table-rehash-size: need hash-table")
	}
	ht := args[0]
	if ht.typ != VVHash || ht.hashTab == nil {
		return nil, fmt.Errorf("hash-table-rehash-size: not a hash table")
	}
	return vnum(args[0].hashTab.rehashSize), nil
}

func builtinHashTableRehashThreshold(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("hash-table-rehash-threshold: need hash-table")
	}
	ht := args[0]
	if ht.typ != VVHash || ht.hashTab == nil {
		return nil, fmt.Errorf("hash-table-rehash-threshold: not a hash table")
	}
	return vnum(ht.hashTab.rehashThreshold), nil
}

func builtinVectorPushExtend(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("vector-push-extend: need element and vector")
	}
	newEl := args[0]
	vec := args[1]
	extension := -1
	for i := 2; i < len(args); i++ {
		if args[i].typ == VSym && args[i].str == ":EXTENSION" && i+1 < len(args) {
			n, err := safeToNum(args[i+1], "vector-push-extend")
			if err != nil {
				return nil, err
			}
			extension = int(n)
			i++
		}
	}
	if vec.typ != VArray || vec.array == nil {
		return nil, fmt.Errorf("vector-push-extend: second argument must be a vector")
	}
	arr := vec.array
	if arr.fillPtr < 0 {
		return nil, fmt.Errorf("vector-push-extend: vector has no fill-pointer")
	}
	if arr.fillPtr >= len(arr.elements) {
		// Extend the vector
		ext := extension
		if ext <= 0 {
			ext = len(arr.elements)
			if ext < 16 {
				ext = 16
			}
		}
		newElems := make([]*Value, len(arr.elements)+ext)
		copy(newElems, arr.elements)
		for i := len(arr.elements); i < len(newElems); i++ {
			newElems[i] = vnil()
		}
		arr.elements = newElems
	}
	arr.elements[arr.fillPtr] = newEl
	fp := arr.fillPtr
	arr.fillPtr++
	return vnum(float64(fp)), nil
}

func builtinRowMajorAref(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("row-major-aref: need array and index")
	}
	arr := args[0]
	idx := args[1]
	if arr.typ != VArray || arr.array == nil {
		return nil, fmt.Errorf("row-major-aref: first argument must be an array")
	}
	n, err := safeToNum(idx, "row-major-aref")
	if err != nil {
		return nil, err
	}
	i := int(n)
	if i < 0 || i >= len(arr.array.elements) {
		return nil, fmt.Errorf("row-major-aref: index %d out of bounds", i)
	}
	return arr.array.elements[i], nil
}

func builtinRowMajorArefSetf(args []*Value) (*Value, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("row-major-aref-setf: need newval, array, and index")
	}
	newVal := args[0]
	arr := args[1]
	idx := args[2]
	if arr.typ != VArray || arr.array == nil {
		return nil, fmt.Errorf("row-major-aref-setf: second argument must be an array")
	}
	n, err := safeToNum(idx, "row-major-aref-setf")
	if err != nil {
		return nil, err
	}
	i := int(n)
	if i < 0 || i >= len(arr.array.elements) {
		return nil, fmt.Errorf("row-major-aref-setf: index %d out of bounds", i)
	}
	arr.array.elements[i] = primaryValue(newVal)
	return newVal, nil
}

var processStartTime = time.Now()

// -------- Bit array operations --------

func bitArrayOp(args []*Value, op string) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("bit-%s: need two bit arrays", op)
	}
	a := args[0]
	b := args[1]
	if a.typ != VArray || a.array == nil {
		return nil, fmt.Errorf("bit-%s: first argument must be a bit array", op)
	}
	if b.typ != VArray || b.array == nil {
		return nil, fmt.Errorf("bit-%s: second argument must be a bit array", op)
	}
	aElems := a.array.elements
	bElems := b.array.elements
	n := len(aElems)
	if len(bElems) < n {
		n = len(bElems)
	}
	result := make([]*Value, n)
	for i := 0; i < n; i++ {
		av := toNum(primaryValue(aElems[i]))
		bv := toNum(primaryValue(bElems[i]))
		var rv float64
		switch op {
		case "and":
			if av != 0 && bv != 0 {
				rv = 1
			}
		case "ior":
			if av != 0 || bv != 0 {
				rv = 1
			}
		case "xor":
			if (av != 0) != (bv != 0) {
				rv = 1
			}
		case "eqv":
			if (av != 0) == (bv != 0) {
				rv = 1
			}
		case "nand":
			if !(av != 0 && bv != 0) {
				rv = 1
			}
		case "nor":
			if !(av != 0 || bv != 0) {
				rv = 1
			}
		case "orc1":
			if !(av != 0) || bv != 0 {
				rv = 1
			}
		case "orc2":
			if av != 0 || !(bv != 0) {
				rv = 1
			}
		case "andc1":
			if !(av != 0) && bv != 0 {
				rv = 1
			}
		case "andc2":
			if av != 0 && !(bv != 0) {
				rv = 1
			}
		}
		result[i] = vnum(rv)
	}
	v := gcv()
	v.typ = VArray
	v.array = &LispArray{
		elements: result,
		dims:     []int{n},
		fillPtr:  -1,
	}
	return v, nil
}

func builtinBitAnd(args []*Value) (*Value, error)   { return bitArrayOp(args, "and") }
func builtinBitIor(args []*Value) (*Value, error)   { return bitArrayOp(args, "ior") }
func builtinBitXor(args []*Value) (*Value, error)   { return bitArrayOp(args, "xor") }
func builtinBitEqv(args []*Value) (*Value, error)   { return bitArrayOp(args, "eqv") }
func builtinBitNand(args []*Value) (*Value, error)  { return bitArrayOp(args, "nand") }
func builtinBitNor(args []*Value) (*Value, error)   { return bitArrayOp(args, "nor") }
func builtinBitOrc1(args []*Value) (*Value, error)  { return bitArrayOp(args, "orc1") }
func builtinBitOrc2(args []*Value) (*Value, error)  { return bitArrayOp(args, "orc2") }
func builtinBitAndc1(args []*Value) (*Value, error) { return bitArrayOp(args, "andc1") }
func builtinBitAndc2(args []*Value) (*Value, error) { return bitArrayOp(args, "andc2") }

func builtinBitNot(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("bit-not: need a bit array")
	}
	a := args[0]
	if a.typ != VArray || a.array == nil {
		return nil, fmt.Errorf("bit-not: argument must be a bit array")
	}
	result := make([]*Value, len(a.array.elements))
	for i, el := range a.array.elements {
		v := toNum(primaryValue(el))
		if v != 0 {
			result[i] = vnum(0)
		} else {
			result[i] = vnum(1)
		}
	}
	rv := gcv()
	rv.typ = VArray
	rv.array = &LispArray{
		elements: result,
		dims:     []int{len(result)},
		fillPtr:  -1,
	}
	return rv, nil
}
