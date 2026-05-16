package microlisp

import (
	"fmt"
	"math"
	"math/big"
)

func toNum(v *Value) float64 {
	switch v.typ {
	case VNum:
		return v.num
	case VRat:
		return float64(v.irat) / float64(v.iden)
	case VComplex:
		return v.num
	case VBigInt:
		f, _ := new(big.Float).SetInt(v.bigInt).Float64()
		return f
	}
	return 0
}

// isNumeric returns true if v is a numeric type
func isNumeric(v *Value) bool {
	return v.typ == VNum || v.typ == VRat || v.typ == VComplex || v.typ == VBigInt
}

// toRatParts extracts rational form. isInt indicates VNum with integer value.
func toRatParts(v *Value) (n, d int64, isInt bool) {
	switch v.typ {
	case VRat:
		return v.irat, v.iden, true
	case VNum:
		if v.num == math.Trunc(v.num) && !math.IsInf(v.num, 0) && v.num >= -9e15 && v.num <= 9e15 {
			return int64(v.num), 1, true
		}
	case VBigInt:
		if v.bigInt.IsInt64() {
			return v.bigInt.Int64(), 1, true
		}
	}
	return 0, 0, false
}

// toComplexParts extracts real and imaginary parts from any numeric value.
func toComplexParts(v *Value) (r, i float64) {
	switch v.typ {
	case VComplex:
		return v.num, v.imag
	case VRat:
		return float64(v.irat) / float64(v.iden), 0
	default:
		return toNum(v), 0
	}
}

// needComplex checks if any arg is a complex number.
func needComplex(args []*Value) bool {
	for _, a := range args {
		if a.typ == VComplex {
			return true
		}
	}
	return false
}

// needRat checks if any arg is a rational (and none are complex).
func needRat(args []*Value) bool {
	for _, a := range args {
		if a.typ == VRat || a.typ == VBigInt {
			return true
		}
	}
	return false
}

// isBigIntInt checks if any arg is a VBigInt.
func isBigIntInt(args []*Value) bool {
	for _, a := range args {
		if a.typ == VBigInt {
			return true
		}
	}
	return false
}

// toBigInt converts a Value to *big.Int (0 if not integer).
func toBigInt(v *Value) *big.Int {
	switch v.typ {
	case VNum:
		if v.num == math.Trunc(v.num) && !math.IsInf(v.num, 0) {
			return big.NewInt(int64(v.num))
		}
	case VRat:
		if v.iden == 1 {
			return big.NewInt(v.irat)
		}
	case VBigInt:
		return new(big.Int).Set(v.bigInt)
	}
	return nil
}

// toBigIntExact converts a Value to *big.Int if it is an exact integer.
// Returns nil for non-integer types (float, rational, complex).
func toBigIntExact(v *Value) *big.Int {
	switch v.typ {
	case VNum:
		if !v.isFloat && v.num == math.Trunc(v.num) && !math.IsInf(v.num, 0) {
			return big.NewInt(int64(v.num))
		}
	case VRat:
		if v.iden == 1 {
			return big.NewInt(v.irat)
		}
	case VBigInt:
		return new(big.Int).Set(v.bigInt)
	}
	return nil
}

// toBigRat converts a Value to big.Rat for exact rational comparison.
func toBigRat(v *Value) big.Rat {
	switch v.typ {
	case VNum:
		if v.isFloat {
			return *new(big.Rat).SetFloat64(v.num)
		}
		return *big.NewRat(int64(v.num), 1)
	case VRat:
		return *big.NewRat(v.irat, v.iden)
	case VBigInt:
		r := new(big.Rat).SetInt(v.bigInt)
		return *r
	}
	return *big.NewRat(0, 1)
}

// compareNumeric returns -1 if a < b, 0 if a == b, 1 if a > b.

func compareNumeric(a, b *Value) int {
	// Handle complex numbers specially - need to compare both parts
	if a.typ == VComplex || b.typ == VComplex {
		aReal, aImag := toComplexParts(a)
		bReal, bImag := toComplexParts(b)
		if aReal < bReal {
			return -1
		}
		if aReal > bReal {
			return 1
		}
		if aImag < bImag {
			return -1
		}
		if aImag > bImag {
			return 1
		}
		return 0
	}
	// Use big.Int for exact comparison when either operand is VBigInt
	aBi := toBigIntExact(a)
	bBi := toBigIntExact(b)
	if aBi != nil && bBi != nil {
		return aBi.Cmp(bBi)
	}
	// Use big.Rat for exact comparison when either operand is VRat
	if a.typ == VRat || b.typ == VRat {
		aRat := toBigRat(a)
		bRat := toBigRat(b)
		return aRat.Cmp(&bRat)
	}
	// Fall back to float64 comparison
	aReal, _ := toComplexParts(a)
	bReal, _ := toComplexParts(b)
	if aReal < bReal {
		return -1
	}
	if aReal > bReal {
		return 1
	}
	return 0
}

func builtinAdd(args []*Value) (*Value, error) {
	if len(args) == 0 {
		return vnum(0), nil
	}
	// Type check: all args must be numeric
	for _, a := range args {
		if !isNumeric(a) {
			return nil, fmt.Errorf("+: not a number: %s", ToString(a))
		}
	}
	if needComplex(args) {
		r, i := 0.0, 0.0
		for _, a := range args {
			ar, ai := toComplexParts(a)
			r += ar
			i += ai
		}
		return vcomplex(r, i), nil
	}
	// Try big.Int if any arg is VBigInt or if rational arithmetic might overflow
	if isBigIntInt(args) {
		result := new(big.Int)
		for _, a := range args {
			bi := toBigInt(a)
			if bi != nil {
				result.Add(result, bi)
				continue
			}
			// Not an exact integer - fall back to float
			f, _ := new(big.Float).SetInt(result).Float64()
			for _, a2 := range args {
				f += toNum(a2)
			}
			return vfloat(f), nil
		}
		return vbigint(result), nil
	}
	if needRat(args) {
		// Track as rational
		n, d := int64(0), int64(1)
		hasFloat := false
		for _, a := range args {
			an, ad, isInt := toRatParts(a)
			if !isInt {
				hasFloat = true
				break
			}
			n = n*ad + an*d
			d = d * ad
			g := gcd(n, d)
			if g < 0 {
				g = -g
			}
			n /= g
			d /= g
		}
		if hasFloat {
			r := 0.0
			for _, a := range args {
				r += toNum(a)
			}
			return vfloat(r), nil
		}
		return vrat(n, d), nil
	}
	r := 0.0
	for _, a := range args {
		r += toNum(a)
	}
	return numOrFloat(r, args), nil
}

func builtinSub(args []*Value) (*Value, error) {
	if len(args) == 0 {
		return vnum(0), nil
	}
	for _, a := range args {
		if !isNumeric(a) {
			return nil, fmt.Errorf("-: not a number: %s", ToString(a))
		}
	}
	if needComplex(args) {
		ar, ai := toComplexParts(args[0])
		if len(args) == 1 {
			return vcomplex(-ar, -ai), nil
		}
		for _, a := range args[1:] {
			br, bi := toComplexParts(a)
			ar -= br
			ai -= bi
		}
		return vcomplex(ar, ai), nil
	}
	if isBigIntInt(args) {
		result := toBigInt(args[0])
		if result == nil {
			f := toNum(args[0])
			for _, a := range args[1:] {
				f -= toNum(a)
			}
			return vfloat(f), nil
		}
		for _, a := range args[1:] {
			bi := toBigInt(a)
			if bi != nil {
				result.Sub(result, bi)
			} else {
				f, _ := new(big.Float).SetInt(result).Float64()
				for _, a2 := range args {
					f -= toNum(a2)
				}
				return vfloat(f), nil
			}
		}
		return vbigint(result), nil
	}
	if len(args) == 1 {
		if args[0].typ == VRat {
			return vrat(-args[0].irat, args[0].iden), nil
		}
		return vnum(-toNum(args[0])), nil
	}
	if needRat(args) {
		n0, d0, isInt0 := toRatParts(args[0])
		hasFloat := !isInt0
		for _, a := range args[1:] {
			an, ad, isInt := toRatParts(a)
			if !isInt {
				hasFloat = true
				break
			}
			n0 = n0*ad - an*d0
			d0 = d0 * ad
			g := gcd(n0, d0)
			if g < 0 {
				g = -g
			}
			n0 /= g
			d0 /= g
		}
		if hasFloat {
			r := toNum(args[0])
			for _, a := range args[1:] {
				r -= toNum(a)
			}
			return vfloat(r), nil
		}
		return vrat(n0, d0), nil
	}
	r := toNum(args[0])
	for _, a := range args[1:] {
		r -= toNum(a)
	}
	return numOrFloat(r, args), nil
}

func builtinMul(args []*Value) (*Value, error) {
	if len(args) == 0 {
		return vnum(1), nil
	}
	for _, a := range args {
		if !isNumeric(a) {
			return nil, fmt.Errorf("*: not a number: %s", ToString(a))
		}
	}
	if needComplex(args) {
		r, i := 1.0, 0.0 // start with 1+0i
		for _, a := range args {
			ar, ai := toComplexParts(a)
			// (r + i*i) * (ar + ai*i) = (r*ar - i*ai) + (r*ai + i*ar)*i
			newR := r*ar - i*ai
			newI := r*ai + i*ar
			r, i = newR, newI
		}
		return vcomplex(r, i), nil
	}
	if isBigIntInt(args) {
		result := big.NewInt(1)
		for _, a := range args {
			bi := toBigInt(a)
			if bi != nil {
				result.Mul(result, bi)
				continue
			}
			f, _ := new(big.Float).SetInt(result).Float64()
			for _, a2 := range args {
				f *= toNum(a2)
			}
			return vfloat(f), nil
		}
		return vbigint(result), nil
	}
	if needRat(args) {
		n, d := int64(1), int64(1)
		hasFloat := false
		for _, a := range args {
			an, ad, isInt := toRatParts(a)
			if !isInt {
				hasFloat = true
				break
			}
			n *= an
			d *= ad
			g := gcd(n, d)
			if g < 0 {
				g = -g
			}
			n /= g
			d /= g
		}
		if hasFloat {
			r := 1.0
			for _, a := range args {
				r *= toNum(a)
			}
			return vfloat(r), nil
		}
		return vrat(n, d), nil
	}
	// All args are VNum — check if they are all integer-valued
	// If so, use big.Int to avoid overflow
	allInt := true
	for _, a := range args {
		if a.typ != VNum || a.num != math.Trunc(a.num) || math.IsInf(a.num, 0) {
			allInt = false
			break
		}
	}
	if allInt {
		result := big.NewInt(1)
		for _, a := range args {
			bi := big.NewInt(int64(a.num))
			result.Mul(result, bi)
		}
		return vbigint(result), nil
	}
	r := 1.0
	for _, a := range args {
		r *= toNum(a)
	}
	return numOrFloat(r, args), nil
}

func builtinDiv(args []*Value) (*Value, error) {
	if len(args) == 0 {
		return vnum(1), nil
	}
	for _, a := range args {
		if !isNumeric(a) {
			return nil, fmt.Errorf("/: not a number: %s", ToString(a))
		}
	}
	if needComplex(args) {
		ar, ai := toComplexParts(args[0])
		if len(args) == 1 {
			// 1 / (ar + ai*i) = ar/(ar²+ai²) - ai/(ar²+ai²)*i
			den := ar*ar + ai*ai
			if den == 0 {
				return nil, signalDivisionByZero()
			}
			return vcomplex(ar/den, -ai/den), nil
		}
		for _, a := range args[1:] {
			br, bi := toComplexParts(a)
			den := br*br + bi*bi
			if den == 0 {
				return nil, signalDivisionByZero()
			}
			// (ar + ai*i) / (br + bi*i) = (ar*br + ai*bi)/den + (ai*br - ar*bi)/den * i
			newR := (ar*br + ai*bi) / den
			newI := (ai*br - ar*bi) / den
			ar, ai = newR, newI
		}
		return vcomplex(ar, ai), nil
	}
	if len(args) == 1 {
		if args[0].typ == VBigInt {
			if args[0].bigInt.Sign() == 0 {
				return nil, signalDivisionByZero()
			}
			return vnum(1.0 / toNum(args[0])), nil
		}
		if args[0].typ == VRat {
			if args[0].irat == 0 {
				return nil, signalDivisionByZero()
			}
			// 1 / (a/b) = b/a
			n := args[0].iden
			d := args[0].irat
			if d < 0 {
				n = -n
				d = -d
			}
			return vrat(n, d), nil
		}
		if toNum(args[0]) == 0 {
			return nil, signalDivisionByZero()
		}
		return vnum(1.0 / toNum(args[0])), nil
	}
	if isBigIntInt(args) {
		num := toBigInt(args[0])
		den := big.NewInt(1)
		if num == nil {
			r := toNum(args[0])
			for _, a := range args[1:] {
				if toNum(a) == 0 {
					return nil, signalDivisionByZero()
				}
				r /= toNum(a)
			}
			return vfloat(r), nil
		}
		for _, a := range args[1:] {
			bi := toBigInt(a)
			if bi == nil {
				r := toNum(args[0])
				for _, a2 := range args[1:] {
					if toNum(a2) == 0 {
						return nil, signalDivisionByZero()
					}
					r /= toNum(a2)
				}
				return vfloat(r), nil
			}
			if bi.Sign() == 0 {
				return nil, signalDivisionByZero()
			}
			den.Mul(den, bi)
		}
		g := new(big.Int).GCD(nil, nil, num, den)
		if g.Sign() != 0 {
			num.Quo(num, g)
			den.Quo(den, g)
		}
		if den.Sign() < 0 {
			num.Neg(num)
			den.Neg(den)
		}
		if den.IsInt64() && den.Int64() == 1 {
			return vbigint(num), nil
		}
		// Result is not an integer: try to reduce to int64 rational
		if num.IsInt64() && den.IsInt64() {
			return vrat(num.Int64(), den.Int64()), nil
		}
		// Fallback: return as float
		f, _ := new(big.Float).Quo(
			new(big.Float).SetInt(num),
			new(big.Float).SetInt(den),
		).Float64()
		return vfloat(f), nil
	}
	if needRat(args) {
		n0, d0, isInt0 := toRatParts(args[0])
		hasFloat := !isInt0
		for _, a := range args[1:] {
			an, ad, isInt := toRatParts(a)
			if !isInt {
				hasFloat = true
				break
			}
			if an == 0 {
				return nil, signalDivisionByZero()
			}
			// (n0/d0) / (an/ad) = n0*ad / (d0*an)
			n0 *= ad
			d0 *= an
			if d0 < 0 {
				n0 = -n0
				d0 = -d0
			}
			g := gcd(n0, d0)
			if g < 0 {
				g = -g
			}
			n0 /= g
			d0 /= g
		}
		if hasFloat {
			r := toNum(args[0])
			for _, a := range args[1:] {
				if toNum(a) == 0 {
					return nil, signalDivisionByZero()
				}
				r /= toNum(a)
			}
			return vfloat(r), nil
		}
		return vrat(n0, d0), nil
	}
	// If all args are integers (not floats), use rational division
	allInt := true
	for _, a := range args {
		if a.typ == VNum && a.isFloat {
			allInt = false
			break
		}
	}
	if allInt {
		n0, d0 := int64(toNum(args[0])), int64(1)
		for _, a := range args[1:] {
			an := int64(toNum(a))
			if an == 0 {
				return nil, signalDivisionByZero()
			}
			n0 *= an
			d0 *= 1
		}
		// Simplify: n0/d0 where d0 is the product of all denominators
		// Actually we need n0 / (product of all args[1:])
		// Redo: numerator = args[0], denominator = product of args[1:]
		num := int64(toNum(args[0]))
		den := int64(1)
		for _, a := range args[1:] {
			den *= int64(toNum(a))
		}
		if den == 0 {
			return nil, signalDivisionByZero()
		}
		return vrat(num, den), nil
	}

	r := toNum(args[0])
	for _, a := range args[1:] {
		if toNum(a) == 0 {
			return nil, signalDivisionByZero()
		}
		r /= toNum(a)
	}
	return numOrFloat(r, args), nil
}
