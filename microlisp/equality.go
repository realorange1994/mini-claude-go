package microlisp

import (
	"fmt"
	"strings"
	"unicode"
	"unsafe"
)

func builtinEqualP(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("equal?: need 2 arguments")
	}
	return vbool(eqVal(args[0], args[1])), nil
}

func builtinEqvP(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("eqv?: need 2 arguments")
	}
	a, b := args[0], args[1]
	if a.typ != b.typ {
		return vbool(false), nil
	}
	switch a.typ {
	case VNum:
		return vbool(a.num == b.num), nil
	case VRat:
		return vbool(a.str == b.str), nil
	case VComplex:
		return vbool(a.str == b.str), nil
	case VStr:
		return vbool(a.str == b.str), nil
	case VChar:
		return vbool(a.ch == b.ch), nil
	case VPackage:
		return vbool(a.pkg == b.pkg), nil
	case VReadtable:
		return vbool(a.readtable == b.readtable), nil
	default:
		return vbool(a == b), nil
	}
}

func eqValSeen(a, b *Value, seen map[[2]uintptr]bool) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if a.typ != b.typ {
		return false
	}
	switch a.typ {
	case VNum:
		return a.num == b.num
	case VRat:
		return a.irat == b.irat && a.iden == b.iden
	case VComplex:
		return a.num == b.num && a.imag == b.imag
	case VStr:
		return a.str == b.str
	case VSym:
		return a.str == b.str
	case VChar:
		return a.ch == b.ch
	case VPackage:
		return a.pkg == b.pkg
	case VReadtable:
		return a.readtable == b.readtable
	case VBool, VNil, VVHash:
		return a == b
	case VPair:
		ka := [2]uintptr{uintptr(unsafe.Pointer(a)), uintptr(unsafe.Pointer(b))}
		if seen[ka] {
			return true
		}
		seen[ka] = true
		return eqValSeen(a.car, b.car, seen) && eqValSeen(a.cdr, b.cdr, seen)
	case VArray:
		if a.array == nil || b.array == nil {
			return a.array == b.array
		}
		if len(a.array.elements) != len(b.array.elements) {
			return false
		}
		for i := range a.array.elements {
			if !eqValSeen(a.array.elements[i], b.array.elements[i], seen) {
				return false
			}
		}
		return true
	}
	return false
}

func eqVal(a, b *Value) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	// In CL, nil (symbol) and () (empty list/VNil) are equal
	if a.typ == VNil && b.typ == VNil {
		return true
	}
	if a.typ == VNil && b.typ == VSym && strings.EqualFold(b.str, "nil") ||
		b.typ == VNil && a.typ == VSym && strings.EqualFold(a.str, "nil") {
		return true
	}
	if a.typ != b.typ {
		return false
	}
	return eqValSeen(a, b, make(map[[2]uintptr]bool))
}

func equalpNumeric(a, b *Value) bool {
	switch a.typ {
	case VNum:
		switch b.typ {
		case VNum:
			return a.num == b.num
		case VRat:
			return float64(a.num) == float64(b.irat)/float64(b.iden)
		}
	case VRat:
		switch b.typ {
		case VNum:
			return float64(a.irat)/float64(a.iden) == b.num
		case VRat:
			return a.irat == b.irat && a.iden == b.iden
		}
	case VComplex:
		if b.typ == VComplex {
			return a.num == b.num && a.imag == b.imag
		}
	}

	return false
}

func equalpVal(a, b *Value) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	// Numbers: same mathematical value
	if isNumeric(a) && isNumeric(b) {
		return equalpNumeric(a, b)
	}
	// Characters: case-insensitive
	if a.typ == VChar && b.typ == VChar {
		ac, bc := unicode.ToLower(a.ch), unicode.ToLower(b.ch)
		return ac == bc
	}
	// Strings: case-insensitive
	if a.typ == VStr && b.typ == VStr {
		return strings.EqualFold(a.str, b.str)
	}
	// Lists: recursively compare
	if a.typ == VPair && b.typ == VPair {
		return equalpVal(a.car, b.car) && equalpVal(a.cdr, b.cdr)
	}
	// Symbols: case-insensitive
	if a.typ == VSym && b.typ == VSym {
		return strings.EqualFold(a.str, b.str)
	}
	// Arrays: element-wise comparison
	if a.typ == VArray && b.typ == VArray {
		return equalpArray(a, b)
	}
	// Otherwise use eql
	return eqVal(a, b)
}

func builtinEqualp(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return vbool(false), nil
	}
	return vbool(equalpVal(args[0], args[1])), nil
}

func builtinEqual(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return vbool(false), nil
	}
	return vbool(eqVal(args[0], args[1])), nil
}

func builtinEqIdentity(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return vbool(false), nil
	}
	a, b := args[0], args[1]
	if a == b {
		return vbool(true), nil
	}
	// In CL, nil (symbol) and () (empty list/VNil) are eq
	if isNil(a) || isNil(b) {
		if (a.typ == VSym && strings.EqualFold(a.str, "nil") && b.typ == VNil) ||
			(b.typ == VSym && strings.EqualFold(b.str, "nil") && a.typ == VNil) {
			return vbool(true), nil
		}
	}
	return vbool(false), nil
}

func builtinEql(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return vbool(false), nil
	}
	return vbool(eqVal(args[0], args[1])), nil
}
