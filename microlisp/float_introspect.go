package microlisp

import (
	"fmt"
	"math"
)

// -------- ANSI CL floating-point introspection --------

func builtinDecodeFloat(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("decode-float: need a float")
	}
	f := toNum(args[0])
	if f == 0 {
		return multiVal(vfloat(0), vnum(0), vnum(1)), nil
	}
	sign := 1.0
	if f < 0 {
		sign = -1.0
		f = -f
	}
	mantissa, exp := math.Frexp(f)
	return multiVal(vfloat(mantissa*sign), vnum(float64(exp)), vnum(1.0)), nil
}

func builtinIntegerDecodeFloat(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("integer-decode-float: need a float")
	}
	f := toNum(args[0])
	if f == 0 {
		return multiVal(vnum(0), vnum(0), vnum(1)), nil
	}
	sign := float64(1)
	if f < 0 {
		sign = -1
		f = -f
	}
	mantissa, exp := math.Frexp(f)
	intSig := int64(mantissa * (1 << 53))
	intExp := exp - 53
	return multiVal(vnum(float64(intSig)), vnum(float64(intExp)), vnum(sign)), nil
}

func builtinScaleFloat(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("scale-float: need float and integer")
	}
	f := toNum(args[0])
	n := toNum(args[1])
	return vfloat(f * math.Pow(2, n)), nil
}

func builtinFloatRadix(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("float-radix: need a float")
	}
	return vnum(2), nil
}

func builtinFloatDigits(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("float-digits: need a float")
	}
	return vnum(53), nil
}

func builtinFloatPrecision(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("float-precision: need a float")
	}
	f := toNum(args[0])
	if f == 0 {
		return vnum(0), nil
	}
	return vnum(53), nil
}
