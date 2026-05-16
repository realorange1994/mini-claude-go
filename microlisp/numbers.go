package microlisp

import (
	"fmt"
	"math"
	"math/big"
	"unicode"
)

// -------- abs --------
func builtinAbs(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("abs: need a number")
	}
	v := args[0]
	switch v.typ {
	case VNum:
		return vnum(math.Abs(v.num)), nil
	case VRat:
		n, d := v.irat, v.iden
		if n < 0 {
			n = -n
		}
		return vrat(n, d), nil
	case VBigInt:
		result := new(big.Int).Abs(v.bigInt)
		return vbigint(result), nil
	case VComplex:
		return vnum(math.Hypot(v.num, v.imag)), nil
	}
	return nil, fmt.Errorf("abs: not a number")
}

// isReal returns true if v is a real numeric type (not complex)
func isReal(v *Value) bool {
	return v.typ == VNum || v.typ == VRat || v.typ == VBigInt
}

func isNumber(v *Value) bool {
	return v.typ == VNum || v.typ == VRat || v.typ == VBigInt || v.typ == VComplex
}

// -------- max / min --------
func builtinMax(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("max: need at least one number")
	}
	for _, a := range args {
		if !isReal(a) {
			return nil, fmt.Errorf("max: not a real number: %v", a)
		}
	}
	result := args[0]
	for i := 1; i < len(args); i++ {
		if compareNumeric(result, args[i]) < 0 {
			result = args[i]
		}
	}
	// If result is VRat, return as-is; if VBigInt or VNum, return as-is
	return result, nil
}

func builtinMin(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("min: need at least one number")
	}
	for _, a := range args {
		if !isReal(a) {
			return nil, fmt.Errorf("min: not a real number: %v", a)
		}
	}
	result := args[0]
	for i := 1; i < len(args); i++ {
		if compareNumeric(result, args[i]) > 0 {
			result = args[i]
		}
	}
	return result, nil
}

// -------- mod / rem --------
func builtinMod(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("mod: need two numbers")
	}
	if isBigIntInt(args) {
		n := toBigInt(args[0])
		d := toBigInt(args[1])
		if n == nil || d == nil {
			// Fall back to float
			nf := toNum(args[0])
			df := toNum(args[1])
			if df == 0 {
				return nil, fmt.Errorf("mod: division by zero")
			}
			r := math.Mod(nf, df)
			if r != 0 && (r > 0) != (df > 0) {
				r += df
			}
			return numOrFloat(r, args), nil
		}
		if d.Sign() == 0 {
			return nil, fmt.Errorf("mod: division by zero")
		}
		r := new(big.Int).Mod(n, d)
		return vbigint(r), nil
	}
	n := toNum(args[0])
	d := toNum(args[1])
	if d == 0 {
		return nil, fmt.Errorf("mod: division by zero")
	}
	r := math.Mod(n, d)
	// CL mod: result has same sign as divisor
	if r != 0 && (r > 0) != (d > 0) {
		r += d
	}
	return numOrFloat(r, args), nil
}

func builtinRem(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("rem: need two numbers")
	}
	if isBigIntInt(args) {
		n := toBigInt(args[0])
		d := toBigInt(args[1])
		if n == nil || d == nil {
			nf := toNum(args[0])
			df := toNum(args[1])
			if df == 0 {
				return nil, fmt.Errorf("rem: division by zero")
			}
			return vnum(math.Mod(nf, df)), nil
		}
		if d.Sign() == 0 {
			return nil, fmt.Errorf("rem: division by zero")
		}
		r := new(big.Int).Rem(n, d)
		return vbigint(r), nil
	}
	n := toNum(args[0])
	d := toNum(args[1])
	if d == 0 {
		return nil, fmt.Errorf("rem: division by zero")
	}
	return vnum(math.Mod(n, d)), nil
}

// -------- floor / ceiling / truncate / round --------
func builtinFloor(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("floor: need a number")
	}
	n := toNum(args[0])
	f := math.Floor(n)
	if len(args) >= 2 {
		div := toNum(args[1])
		if div == 0 {
			return nil, fmt.Errorf("floor: division by zero")
		}
		q := math.Floor(n / div)
		r := n - q*div
		if r != 0 && (r > 0) != (div > 0) {
			r += div
		}
		return multiVal(vnum(q), vnum(r)), nil
	}
	return multiVal(vnum(f), vnum(n-f)), nil
}

func builtinCeiling(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("ceiling: need a number")
	}
	n := toNum(args[0])
	c := math.Ceil(n)
	if len(args) >= 2 {
		div := toNum(args[1])
		if div == 0 {
			return nil, fmt.Errorf("ceiling: division by zero")
		}
		q := math.Ceil(n / div)
		r := n - q*div
		return multiVal(vnum(q), vnum(r)), nil
	}
	return multiVal(vnum(c), vnum(n-c)), nil
}

func builtinTruncate(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("truncate: need a number")
	}
	n := toNum(args[0])
	t := math.Trunc(n)
	if len(args) >= 2 {
		div := toNum(args[1])
		if div == 0 {
			return nil, fmt.Errorf("truncate: division by zero")
		}
		q := math.Trunc(n / div)
		r := n - q*div
		return multiVal(vnum(q), vnum(r)), nil
	}
	return multiVal(vnum(t), vnum(n-t)), nil
}

func builtinRound(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("round: need a number")
	}
	n := toNum(args[0])
	r := math.Round(n)
	// Round half to even
	if diff := math.Abs(n - r); diff == 0.5 {
		if math.Mod(r, 2) != 0 {
			if n > 0 {
				r--
			} else {
				r++
			}
		}
	}
	if len(args) >= 2 {
		div := toNum(args[1])
		if div == 0 {
			return nil, fmt.Errorf("round: division by zero")
		}
		q := math.Round(n / div)
		// Round half to even
		if diff := math.Abs(n/div - q); diff == 0.5 {
			if math.Mod(q, 2) != 0 {
				if n/div > 0 {
					q--
				} else {
					q++
				}
			}
		}
		rem := n - q*div
		return multiVal(vnum(q), vnum(rem)), nil
	}
	return multiVal(vnum(r), vnum(n-r)), nil
}

// -------- ffloor, fceiling, ftruncate, fround --------
// Same as floor/ceiling/truncate/round but the first return value is a float.

func builtinFfloor(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("ffloor: need a number")
	}
	n := toNum(args[0])
	f := math.Floor(n)
	if len(args) >= 2 {
		div := toNum(args[1])
		if div == 0 {
			return nil, fmt.Errorf("ffloor: division by zero")
		}
		q := math.Floor(n / div)
		r := n - q*div
		if r != 0 && (r > 0) != (div > 0) {
			r += div
		}
		return multiVal(vfloat(q), vnum(r)), nil
	}
	return multiVal(vfloat(f), vnum(n-f)), nil
}

func builtinFceiling(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("fceiling: need a number")
	}
	n := toNum(args[0])
	c := math.Ceil(n)
	if len(args) >= 2 {
		div := toNum(args[1])
		if div == 0 {
			return nil, fmt.Errorf("fceiling: division by zero")
		}
		q := math.Ceil(n / div)
		r := n - q*div
		return multiVal(vfloat(q), vnum(r)), nil
	}
	return multiVal(vfloat(c), vnum(n-c)), nil
}

func builtinFtruncate(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("ftruncate: need a number")
	}
	n := toNum(args[0])
	t := math.Trunc(n)
	if len(args) >= 2 {
		div := toNum(args[1])
		if div == 0 {
			return nil, fmt.Errorf("ftruncate: division by zero")
		}
		q := math.Trunc(n / div)
		r := n - q*div
		return multiVal(vfloat(q), vnum(r)), nil
	}
	return multiVal(vfloat(t), vnum(n-t)), nil
}

func builtinFround(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("fround: need a number")
	}
	n := toNum(args[0])
	r := math.Round(n)
	// Round half to even
	if diff := math.Abs(n - r); diff == 0.5 {
		if math.Mod(r, 2) != 0 {
			if n > 0 {
				r--
			} else {
				r++
			}
		}
	}
	if len(args) >= 2 {
		div := toNum(args[1])
		if div == 0 {
			return nil, fmt.Errorf("fround: division by zero")
		}
		q := math.Round(n / div)
		// Round half to even
		if diff := math.Abs(n/div - q); diff == 0.5 {
			if math.Mod(q, 2) != 0 {
				if n/div > 0 {
					q--
				} else {
					q++
				}
			}
		}
		rem := n - q*div
		return multiVal(vfloat(q), vnum(rem)), nil
	}
	return multiVal(vfloat(r), vnum(n-r)), nil
}

// -------- signum --------
func builtinSignum(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("signum: need a number")
	}
	v := args[0]
	if v.typ == VBigInt {
		sign := v.bigInt.Sign()
		if sign > 0 {
			return vnum(1), nil
		}
		if sign < 0 {
			return vnum(-1), nil
		}
		return vnum(0), nil
	}
	n := toNum(args[0])
	if n > 0 {
		return vnum(1), nil
	}
	if n < 0 {
		return vnum(-1), nil
	}
	return vnum(0), nil
}

// -------- gcd --------
func builtinGCD(args []*Value) (*Value, error) {
	if len(args) == 0 {
		return vnum(0), nil
	}
	if isBigIntInt(args) {
		result := new(big.Int)
		bi := toBigInt(args[0])
		if bi != nil {
			result.Abs(bi)
		} else {
			result.SetInt64(int64(math.Abs(toNum(args[0]))))
		}
		for i := 1; i < len(args); i++ {
			bi := toBigInt(args[i])
			n := new(big.Int)
			if bi != nil {
				n.Abs(bi)
			} else {
				n.SetInt64(int64(math.Abs(toNum(args[i]))))
			}
			result.GCD(nil, nil, result, n)
		}
		return vbigint(result), nil
	}
	result := int64(math.Abs(toNum(args[0])))
	for i := 1; i < len(args); i++ {
		n := int64(math.Abs(toNum(args[i])))
		for n != 0 {
			result, n = n, result%n
		}
	}
	return vnum(float64(result)), nil
}

// -------- lcm --------
func builtinLCM(args []*Value) (*Value, error) {
	if len(args) == 0 {
		return vnum(1), nil
	}
	if isBigIntInt(args) {
		result := new(big.Int)
		bi := toBigInt(args[0])
		if bi != nil {
			result.Abs(bi)
		} else {
			result.SetInt64(int64(math.Abs(toNum(args[0]))))
		}
		if result.Sign() == 0 {
			return vnum(0), nil
		}
		for i := 1; i < len(args); i++ {
			bi := toBigInt(args[i])
			n := new(big.Int)
			if bi != nil {
				n.Abs(bi)
			} else {
				n.SetInt64(int64(math.Abs(toNum(args[i]))))
			}
			if n.Sign() == 0 {
				return vnum(0), nil
			}
			g := new(big.Int).GCD(nil, nil, result, n)
			result.Mul(result, n)
			result.Quo(result, g)
			result.Abs(result)
		}
		return vbigint(result), nil
	}
	gcd := func(a, b int64) int64 {
		for b != 0 {
			a, b = b, a%b
		}
		return a
	}
	result := int64(math.Abs(toNum(args[0])))
	if result == 0 {
		return vnum(0), nil
	}
	for i := 1; i < len(args); i++ {
		n := int64(math.Abs(toNum(args[i])))
		if n == 0 {
			return vnum(0), nil
		}
		result = result / gcd(result, n) * n
	}
	return vnum(float64(result)), nil
}

// -------- log --------
func builtinLog(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("log: need a number")
	}
	n := toNum(args[0])
	if n <= 0 {
		return nil, fmt.Errorf("log: argument must be positive")
	}
	if len(args) >= 2 {
		base := toNum(args[1])
		if base <= 0 || base == 1 {
			return nil, fmt.Errorf("log: invalid base")
		}
		return vnum(math.Log(n) / math.Log(base)), nil
	}
	return vnum(math.Log(n)), nil
}

// -------- sqrt --------
func builtinSqrt(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("sqrt: need a number")
	}
	n := toNum(args[0])
	if n < 0 {
		// Return complex
		return vcomplex(0, math.Sqrt(-n)), nil
	}
	return vnum(math.Sqrt(n)), nil
}

func toRational(f float64) *Value {
	const maxDen = 1000000
	num := int64(math.Round(f * float64(maxDen)))
	den := int64(maxDen)
	g := gcd(num, den)
	num /= g
	den /= g
	if den == 1 {
		return vnum(float64(num))
	}
	return vrat(num, den)
}

func builtinExpt(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("expt: need base and exponent")
	}
	base, exp := args[0], args[1]
	// Check if exponent is an integer
	expIsInt := (exp.typ == VNum && exp.num == math.Trunc(exp.num)) || exp.typ == VBigInt
	if !expIsInt {
		return vnum(math.Pow(toNum(base), toNum(exp))), nil
	}
	// Get exponent as int64
	var e int64
	if exp.typ == VBigInt {
		if !exp.bigInt.IsInt64() {
			return nil, fmt.Errorf("expt: exponent too large")
		}
		e = exp.bigInt.Int64()
	} else {
		e = int64(exp.num)
	}
	// Handle complex base with integer exponent
	if base.typ == VComplex {
		if e == 0 {
			return vcomplexAlways(1, 0), nil
		}
		negExp := e < 0
		if negExp {
			e = -e
		}
		// Binary exponentiation for complex numbers
		rReal := 1.0
		rImag := 0.0
		bReal := base.num
		bImag := base.imag
		for e > 0 {
			if e&1 == 1 {
				newR := rReal*bReal - rImag*bImag
				newI := rReal*bImag + rImag*bReal
				rReal = newR
				rImag = newI
			}
			newB := bReal*bReal - bImag*bImag
			bImag = 2 * bReal * bImag
			bReal = newB
			e >>= 1
		}
		result := vcomplexAlways(rReal, rImag)
		if negExp {
			// 1 / result
			mag := rReal*rReal + rImag*rImag
			if mag == 0 {
				return nil, fmt.Errorf("expt: division by zero")
			}
			result = vcomplexAlways(rReal/mag, -rImag/mag)
		}
		return result, nil
	}
	if e < 0 {
		// 1 / base^|e| — return float
		absE := new(big.Int).Abs(big.NewInt(e))
		var result *big.Int
		bi := toBigInt(base)
		if bi != nil {
			result = new(big.Int).Exp(bi, absE, nil)
		} else {
			result = big.NewInt(int64(toNum(base)))
			result.Exp(result, absE, nil)
		}
		f, _ := new(big.Float).SetInt(result).Float64()
		return vnum(1.0 / f), nil
	}
	if e == 0 {
		return vnum(1), nil
	}
	// Try big.Int exponentiation for integer bases
	bi := toBigInt(base)
	if bi != nil {
		result := new(big.Int).Exp(bi, big.NewInt(e), nil)
		return vbigint(result), nil
	}
	// Non-integer base, integer exponent — use float
	baseF := toNum(base)
	if baseF == 0 {
		return vnum(0), nil
	}
	if e == 1 {
		return vnum(baseF), nil
	}
	if e == -1 {
		return vnum(1 / baseF), nil
	}
	return vnum(math.Pow(baseF, float64(e))), nil
}

// -------- Trig functions --------
func builtinSin(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("sin: need a number")
	}
	n := toNum(args[0])
	return vnum(float64(math.Sin(n))), nil
}
func builtinCos(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("cos: need a number")
	}
	n := toNum(args[0])
	return vnum(float64(math.Cos(n))), nil
}
func builtinTan(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("tan: need a number")
	}
	n := toNum(args[0])
	return vnum(float64(math.Tan(n))), nil
}
func builtinAtan(args []*Value) (*Value, error) {
	if len(args) == 1 {
		n := toNum(args[0])
		return vnum(float64(math.Atan(n))), nil
	}
	// atan2: (atan y x)
	y := toNum(args[0])
	x := toNum(args[1])
	return vnum(float64(math.Atan2(y, x))), nil
}
func builtinAtan2(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("atan2: need y and x")
	}
	y := toNum(args[0])
	x := toNum(args[1])
	return vnum(float64(math.Atan2(y, x))), nil
}
func builtinExp(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("exp: need a number")
	}
	n := toNum(args[0])
	return vnum(float64(math.Exp(n))), nil
}
func builtinSinh(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("sinh: need a number")
	}
	n := toNum(args[0])
	return vnum(float64(math.Sinh(n))), nil
}
func builtinCosh(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("cosh: need a number")
	}
	n := toNum(args[0])
	return vnum(float64(math.Cosh(n))), nil
}
func builtinTanh(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("tanh: need a number")
	}
	n := toNum(args[0])
	return vnum(float64(math.Tanh(n))), nil
}

// -------- asinh / acosh / atanh (ANSI CL inverse hyperbolic) --------
func builtinAsinh(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("asinh: need a number")
	}
	n := toNum(args[0])
	// asinh(x) = log(x + sqrt(x*x + 1))
	return vnum(math.Log(n + math.Sqrt(n*n+1))), nil
}

func builtinAcosh(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("acosh: need a number")
	}
	n := toNum(args[0])
	if n < 1 {
		// acosh undefined for x < 1, return complex result
		// acosh(x) = log(x + sqrt(x-1)*sqrt(x+1)) but for x<1 we compute:
		// acosh(x) = i*acos(x)  => complex result
		ac := math.Acos(n)
		return vcomplex(0, ac), nil
	}
	// acosh(x) = log(x + sqrt(x-1)*sqrt(x+1))
	return vnum(math.Log(n + math.Sqrt(n-1)*math.Sqrt(n+1))), nil
}

func builtinAtanh(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("atanh: need a number")
	}
	n := toNum(args[0])
	if n <= -1 || n >= 1 {
		// atanh undefined for |x| >= 1, return complex result
		// atanh(x) = 0.5*log((1+x)/(1-x)) => complex when |x|>=1
		// For x=1: pi/2, for x=-1: -pi/2, both imaginary part infinity
		// We compute the principal value using log of complex numbers
		// atanh(x) = 0.5 * (log(1+x) - log(1-x))
		// When x>1: log(1+x) real, log(1-x) = log(-(x-1)) = log(x-1) + i*pi
		// => atanh(x) = 0.5*(log(1+x) - log(x-1)) - i*pi/2
		if n == 1 || n == -1 {
			return nil, fmt.Errorf("atanh: argument %v is out of domain", args[0])
		}
		if n > 1 {
			return vcomplex(0.5*(math.Log(1+n)-math.Log(n-1)), -math.Pi/2), nil
		}
		// n < -1
		return vcomplex(0.5*(math.Log(1-n)-math.Log(-1-n)), math.Pi/2), nil
	}
	// atanh(x) = 0.5*log((1+x)/(1-x)) for |x| < 1
	return vnum(0.5 * math.Log((1+n)/(1-n))), nil
}

// -------- evenp / oddp --------
func builtinEvenp(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("evenp: need a number")
	}
	v := args[0]
	if v.typ == VBigInt {
		return vbool(new(big.Int).Mod(v.bigInt, big.NewInt(2)).Sign() == 0), nil
	}
	return vbool(int64(toNum(args[0]))%2 == 0), nil
}

func builtinOddp(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("oddp: need a number")
	}
	v := args[0]
	if v.typ == VBigInt {
		return vbool(new(big.Int).Mod(v.bigInt, big.NewInt(2)).Sign() != 0), nil
	}
	return vbool(int64(toNum(args[0]))%2 != 0), nil
}

// -------- plusp / minusp / zerop --------
func builtinPlusp(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("plusp: need a number")
	}
	v := args[0]
	if v.typ == VBigInt {
		return vbool(v.bigInt.Sign() > 0), nil
	}
	return vbool(toNum(args[0]) > 0), nil
}

func builtinMinusp(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("minusp: need a number")
	}
	v := args[0]
	if v.typ == VBigInt {
		return vbool(v.bigInt.Sign() < 0), nil
	}
	return vbool(toNum(args[0]) < 0), nil
}

func builtinZerop(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("zerop: need a number")
	}
	v := args[0]
	if v.typ == VBigInt {
		return vbool(v.bigInt.Sign() == 0), nil
	}
	return vbool(toNum(args[0]) == 0), nil
}

// -------- 1+ / 1- --------
func builtinOnePlus(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("1+: need a number")
	}
	v := args[0]
	if v.typ == VBigInt {
		return vbigint(new(big.Int).Add(v.bigInt, big.NewInt(1))), nil
	}
	return vnum(toNum(args[0]) + 1), nil
}

func builtinOneMinus(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("1-: need a number")
	}
	v := args[0]
	if v.typ == VBigInt {
		return vbigint(new(big.Int).Sub(v.bigInt, big.NewInt(1))), nil
	}
	return vnum(toNum(args[0]) - 1), nil
}

// -------- incf / decf (implemented as special forms in eval) --------

// -------- digit-char --------
func builtinDigitChar(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("digit-char: need weight")
	}
	weight := int(toNum(args[0]))
	radix := 10
	if len(args) >= 2 {
		radix = int(toNum(args[1]))
	}
	if weight < 0 || weight >= radix {
		return vnil(), nil
	}
	if weight < 10 {
		return vchar(rune('0' + weight)), nil
	}
	return vchar(rune('A' + weight - 10)), nil
}

// -------- digit-char-p --------

// -------- alphanumericp --------
func builtinAlphanumericp(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("alphanumericp: need a character")
	}
	var ch rune
	if args[0].typ == VChar {
		ch = args[0].ch
	} else if args[0].typ == VStr && len(args[0].str) > 0 {
		ch = []rune(args[0].str)[0]
	} else {
		return vbool(false), nil
	}
	return vbool(unicode.IsLetter(ch) || unicode.IsDigit(ch)), nil
}

// -------- alpha-char-p --------
func builtinAlphaCharP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("alpha-char-p: need a character")
	}
	var ch rune
	if args[0].typ == VChar {
		ch = args[0].ch
	} else if args[0].typ == VStr && len(args[0].str) > 0 {
		ch = []rune(args[0].str)[0]
	} else {
		return vbool(false), nil
	}
	return vbool(unicode.IsLetter(ch)), nil
}

// -------- graphic-char-p --------
func builtinGraphicCharP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("graphic-char-p: need a character")
	}
	var ch rune
	if args[0].typ == VChar {
		ch = args[0].ch
	} else if args[0].typ == VStr && len(args[0].str) > 0 {
		ch = []rune(args[0].str)[0]
	} else {
		return vbool(false), nil
	}
	// Graphic chars are printable characters EXCLUDING whitespace (space, tab, newline, etc.)
	// Per ANSI CL: space, newline, tab, page, return, backspace are NOT graphic characters
	if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' || ch == '\f' || ch == '\b' {
		return vbool(false), nil
	}
	return vbool(unicode.IsPrint(ch) && !unicode.IsSpace(ch)), nil
}

// -------- standard-char-p --------
// Standard chars are: space, newline, tab, page, return, backspace,
// and all 95 printable ASCII characters (codes 32-126)
// isStandardChar checks if a character is a standard-char
func isStandardChar(ch rune) bool {
	return (ch >= 32 && ch <= 126) || ch == 10 || ch == 9 || ch == 12 || ch == 13 || ch == 8
}

func builtinStandardCharP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("standard-char-p: need a character")
	}
	var ch rune
	if args[0].typ == VChar {
		ch = args[0].ch
	} else if args[0].typ == VStr && len(args[0].str) > 0 {
		ch = []rune(args[0].str)[0]
	} else {
		return vbool(false), nil
	}
	// Standard chars are space (32), newline (10), tab (9), page (12), return (13),
	// backspace (8), and all 95 printable ASCII chars (32-126)
	return vbool(isStandardChar(ch)), nil
}

// -------- upper-case-p / lower-case-p / both-case-p --------
func builtinUpperCaseP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("upper-case-p: need a character")
	}
	var ch rune
	if args[0].typ == VChar {
		ch = args[0].ch
	} else if args[0].typ == VStr && len(args[0].str) > 0 {
		ch = []rune(args[0].str)[0]
	} else {
		return vbool(false), nil
	}
	return vbool(unicode.IsUpper(ch) && unicode.IsLetter(ch)), nil
}

func builtinLowerCaseP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("lower-case-p: need a character")
	}
	var ch rune
	if args[0].typ == VChar {
		ch = args[0].ch
	} else if args[0].typ == VStr && len(args[0].str) > 0 {
		ch = []rune(args[0].str)[0]
	} else {
		return vbool(false), nil
	}
	return vbool(unicode.IsLower(ch) && unicode.IsLetter(ch)), nil
}

func builtinBothCaseP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("both-case-p: need a character")
	}
	var ch rune
	if args[0].typ == VChar {
		ch = args[0].ch
	} else if args[0].typ == VStr && len(args[0].str) > 0 {
		ch = []rune(args[0].str)[0]
	} else {
		return vbool(false), nil
	}
	return vbool(unicode.IsLetter(ch)), nil
}

// -------- char-upcase / char-downcase --------

// -------- char-equal (case-insensitive) / char-not-equal --------
func builtinCharEqual(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("char-equal: need at least two characters")
	}
	runes := make([]rune, len(args))
	for i, a := range args {
		if a.typ != VChar {
			return nil, fmt.Errorf("char-equal: expected a character")
		}
		runes[i] = unicode.ToLower(a.ch)
	}
	for i := 1; i < len(runes); i++ {
		if runes[i] != runes[0] {
			return vbool(false), nil
		}
	}
	return vbool(true), nil
}

func builtinCharNotEqual(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("char-not-equal: need at least two characters")
	}
	runes := make([]rune, len(args))
	for i, a := range args {
		if a.typ != VChar {
			return nil, fmt.Errorf("char-not-equal: expected a character")
		}
		runes[i] = unicode.ToLower(a.ch)
	}
	// All pairs must be distinct (not just adjacent)
	for i := 0; i < len(runes); i++ {
		for j := i + 1; j < len(runes); j++ {
			if runes[i] == runes[j] {
				return vbool(false), nil
			}
		}
	}
	return vbool(true), nil
}

func charVal(v *Value) rune {
	if v.typ == VChar {
		return v.ch
	}
	if v.typ == VStr && len(v.str) > 0 {
		return []rune(v.str)[0]
	}
	return 0
}

// -------- char-lessp / char-greaterp / char-not-lessp / char-not-greaterp (case-insensitive, multi-arg) --------
func charCompareCI(op string, args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("char%s: expected at least 2 characters", op)
	}
	runes := make([]rune, len(args))
	for i, a := range args {
		if a.typ != VChar {
			return nil, fmt.Errorf("char%s: expected a character", op)
		}
		runes[i] = unicode.ToLower(a.ch)
	}
	for i := 0; i < len(runes)-1; i++ {
		a, b := runes[i], runes[i+1]
		switch op {
		case "lessp":
			if !(a < b) {
				return vbool(false), nil
			}
		case "greaterp":
			if !(a > b) {
				return vbool(false), nil
			}
		case "not-lessp":
			if !(a >= b) {
				return vbool(false), nil
			}
		case "not-greaterp":
			if !(a <= b) {
				return vbool(false), nil
			}
		}
	}
	return vbool(true), nil
}

func builtinCharLessp(args []*Value) (*Value, error)    { return charCompareCI("lessp", args) }
func builtinCharGreaterp(args []*Value) (*Value, error) { return charCompareCI("greaterp", args) }
func builtinCharNotLessp(args []*Value) (*Value, error) { return charCompareCI("not-lessp", args) }
func builtinCharNotGreaterp(args []*Value) (*Value, error) {
	return charCompareCI("not-greaterp", args)
}

// builtinCharNotEq - ANSI CL char/=: all pairs must be distinct (case-sensitive)
func builtinCharNotEq(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("char/=: need at least two characters")
	}
	runes := make([]rune, len(args))
	for i, a := range args {
		if a.typ != VChar {
			return nil, fmt.Errorf("char/=: expected a character")
		}
		runes[i] = a.ch
	}
	// All pairs must be distinct (not just adjacent)
	for i := 0; i < len(runes); i++ {
		for j := i + 1; j < len(runes); j++ {
			if runes[i] == runes[j] {
				return vbool(false), nil
			}
		}
	}
	return vbool(true), nil
}

// -------- revappend (already exists) --------

// -------- nreconc --------
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

// -------- realpart / imagpart --------
func builtinRealpart(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("realpart: need a number")
	}
	v := args[0]
	if v.typ == VComplex {
		return vnum(v.num), nil
	}
	return v, nil
}

func builtinImagpart(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("imagpart: need a number")
	}
	v := args[0]
	if v.typ == VComplex {
		return vnum(v.imag), nil
	}
	return vnum(0), nil
}

// -------- conjugate --------
func builtinConjugate(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("conjugate: need a number")
	}
	v := args[0]
	if v.typ == VComplex {
		return vcomplex(v.num, -v.imag), nil
	}
	return v, nil
}

// -------- phase --------
func builtinPhase(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("phase: need a number")
	}
	v := args[0]
	if v.typ == VComplex {
		return vnum(math.Atan2(v.imag, v.num)), nil
	}
	n := toNum(v)
	if n >= 0 {
		return vnum(0), nil
	}
	return vnum(math.Pi), nil
}

// -------- cis --------
func builtinCis(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("cis: need a number")
	}
	radians := toNum(args[0])
	return vcomplex(math.Cos(radians), math.Sin(radians)), nil
}

// -------- asin --------
func builtinAsin(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("asin: need a number")
	}
	n := toNum(args[0])
	if args[0].typ == VComplex {
		// complex asin: asin(z) = -i*log(iz + sqrt(1-z^2))
		return nil, fmt.Errorf("asin: complex arguments not yet supported")
	}
	if n < -1 || n > 1 {
		// Result is complex for |n| > 1
		return vcomplex(0, math.Asin(n)), nil
	}
	return vnum(math.Asin(n)), nil
}

// -------- acos --------
func builtinAcos(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("acos: need a number")
	}
	n := toNum(args[0])
	if args[0].typ == VComplex {
		return nil, fmt.Errorf("acos: complex arguments not yet supported")
	}
	if n < -1 || n > 1 {
		return vcomplex(math.Acos(n), 0), nil
	}
	return vnum(math.Acos(n)), nil
}

// -------- rationalize --------
func builtinRationalize(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("rationalize: need a number")
	}
	v := args[0]
	if v.typ == VRat {
		return v, nil
	}
	n := toNum(v)
	if n == math.Trunc(n) && !v.isFloat {
		return vnum(n), nil
	}
	// Convert float to rational using continued fraction algorithm
	frac := big.NewRat(1, 1)
	frac.SetFloat64(n)
	// Reduce to integer numerator/denominator
	num := frac.Num()
	den := frac.Denom()
	if den.IsUint64() && num.IsUint64() {
		return vrat(int64(num.Uint64()), int64(den.Uint64())), nil
	}
	// For very large values, just return as float
	return vnum(n), nil
}

// -------- complex --------
func builtinComplex(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("complex: need real and imaginary parts")
	}
	realPart := toNum(args[0])
	imagPart := toNum(args[1])
	if imagPart == 0 {
		return vnum(realPart), nil
	}
	return vcomplex(realPart, imagPart), nil
}

// -------- type predicates --------
func isIntegerValue(v *Value) bool {
	if v.typ == VBigInt {
		return true
	}
	if v.typ == VRat {
		return v.iden == 1
	}
	if v.typ == VNum {
		// VNum with isFloat=true was explicitly created as a float (e.g., 3.0).
		// Per CLHS, floats are NOT integers, so reject them for bitwise ops.
		if v.isFloat {
			return false
		}
		// VNum with isFloat=false represents an integer literal (e.g., 3).
		return v.num == math.Trunc(v.num) && !math.IsInf(v.num, 0) && !math.IsNaN(v.num)
	}
	return false
}

func signalTypeError(arg *Value) (*Value, error) {
	cond := &Value{typ: VInstance, instClass: findClass("type-error"), instSlots: map[string]*Value{
		"message": vstr(fmt.Sprintf("type error: expected integer, got %s", ToString(arg))),
	}}
	return builtinError([]*Value{cond})
}

func builtinIntegerp(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	v := args[0]
	if v.typ == VNum {
		return vbool(v.num == math.Trunc(v.num)), nil
	}
	if v.typ == VRat {
		return vbool(v.iden == 1), nil
	}
	if v.typ == VBigInt {
		return vbool(true), nil
	}
	return vbool(false), nil
}

func builtinFloatp(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	v := args[0]
	if v.typ == VNum {
		// floatp true for values explicitly created as float or non-integer VNum
		return vbool(v.isFloat || v.num != math.Trunc(v.num)), nil
	}
	return vbool(false), nil
}

func builtinRationalp(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	v := args[0]
	return vbool(v.typ == VRat || v.typ == VBigInt || (v.typ == VNum && !v.isFloat)), nil
}

func builtinRealp(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	v := args[0]
	return vbool(v.typ == VNum || v.typ == VRat || v.typ == VBigInt), nil
}

func builtinComplexp(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	return vbool(args[0].typ == VComplex), nil
}

// -------- float / rational --------
func builtinFloat(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("float: need a number")
	}
	n := toNum(args[0])
	return vfloat(n), nil
}

func builtinRational(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("rational: need a number")
	}
	v := args[0]
	if v.typ == VRat {
		return v, nil
	}
	if v.typ == VNum {
		// Convert float to rational approximation
		r := big.NewRat(0, 1)
		r.SetFloat64(v.num)
		return vrat(r.Num().Int64(), r.Denom().Int64()), nil
	}
	return nil, fmt.Errorf("rational: not a real number")
}

// -------- numerator / denominator --------
func builtinNumerator(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("numerator: need a rational")
	}
	v := args[0]
	if v.typ == VRat {
		return vnum(float64(v.irat)), nil
	}
	if v.typ == VNum {
		return vnum(v.num), nil
	}
	return nil, fmt.Errorf("numerator: not a rational")
}

func builtinDenominator(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("denominator: need a rational")
	}
	v := args[0]
	if v.typ == VRat {
		return vnum(float64(v.iden)), nil
	}
	if v.typ == VNum {
		return vnum(1), nil
	}
	return nil, fmt.Errorf("denominator: not a rational")
}

// -------- ash (arithmetic shift) --------
func builtinAsh(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("ash: need integer and count")
	}
	n := toBigInt(args[0])
	if n == nil {
		// Fallback for non-bignum values
		n = big.NewInt(int64(toNum(args[0])))
	}
	count := int(toNum(args[1]))
	var result *big.Int
	if count >= 0 {
		result = new(big.Int).Lsh(n, uint(count))
	} else {
		result = new(big.Int).Rsh(n, uint(-count))
	}
	return vbigint(result), nil
}

// -------- logand / logior / logxor / lognot --------
func builtinLogand(args []*Value) (*Value, error) {
	if len(args) == 0 {
		return vnum(-1), nil // all ones
	}
	for _, a := range args {
		if !isIntegerValue(a) {
			return signalTypeError(a)
		}
	}
	if isBigIntInt(args) {
		result := toBigInt(args[0])
		if result == nil {
			result = big.NewInt(int64(toNum(args[0])))
		}
		for i := 1; i < len(args); i++ {
			bi := toBigInt(args[i])
			if bi == nil {
				bi = big.NewInt(int64(toNum(args[i])))
			}
			result.And(result, bi)
		}
		return vbigint(result), nil
	}
	result := int64(toNum(args[0]))
	for i := 1; i < len(args); i++ {
		result &= int64(toNum(args[i]))
	}
	return vnum(float64(result)), nil
}

func builtinLogior(args []*Value) (*Value, error) {
	if len(args) == 0 {
		return vnum(0), nil
	}
	for _, a := range args {
		if !isIntegerValue(a) {
			return signalTypeError(a)
		}
	}
	if isBigIntInt(args) {
		result := toBigInt(args[0])
		if result == nil {
			result = big.NewInt(int64(toNum(args[0])))
		}
		for i := 1; i < len(args); i++ {
			bi := toBigInt(args[i])
			if bi == nil {
				bi = big.NewInt(int64(toNum(args[i])))
			}
			result.Or(result, bi)
		}
		return vbigint(result), nil
	}
	result := int64(toNum(args[0]))
	for i := 1; i < len(args); i++ {
		result |= int64(toNum(args[i]))
	}
	return vnum(float64(result)), nil
}

func builtinLogxor(args []*Value) (*Value, error) {
	if len(args) == 0 {
		return vnum(0), nil
	}
	for _, a := range args {
		if !isIntegerValue(a) {
			return signalTypeError(a)
		}
	}
	if isBigIntInt(args) {
		result := toBigInt(args[0])
		if result == nil {
			result = big.NewInt(int64(toNum(args[0])))
		}
		for i := 1; i < len(args); i++ {
			bi := toBigInt(args[i])
			if bi == nil {
				bi = big.NewInt(int64(toNum(args[i])))
			}
			result.Xor(result, bi)
		}
		return vbigint(result), nil
	}
	result := int64(toNum(args[0]))
	for i := 1; i < len(args); i++ {
		result ^= int64(toNum(args[i]))
	}
	return vnum(float64(result)), nil
}

func builtinLognot(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("lognot: need an integer")
	}
	v := args[0]
	if v.typ == VBigInt {
		result := new(big.Int).Not(v.bigInt)
		return vbigint(result), nil
	}
	return vnum(float64(^int64(toNum(args[0])))), nil
}

// logandInts converts two args to int64 if possible, or returns big.Ints
func logAndInts(a, b *Value) (int64, int64, *big.Int, *big.Int, bool) {
	if a.typ == VBigInt || b.typ == VBigInt || a.typ == VNum && float64(int64(toNum(a))) != toNum(a) || b.typ == VNum && float64(int64(toNum(b))) != toNum(b) {
		ai := toBigInt(a)
		if ai == nil {
			ai = big.NewInt(int64(toNum(a)))
		}
		bi := toBigInt(b)
		if bi == nil {
			bi = big.NewInt(int64(toNum(b)))
		}
		return 0, 0, ai, bi, true
	}
	return int64(toNum(a)), int64(toNum(b)), nil, nil, false
}

func builtinLognand(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("lognand: need two integers")
	}
	a, b, ai, bi, isBig := logAndInts(args[0], args[1])
	if isBig {
		result := new(big.Int).Not(new(big.Int).And(ai, bi))
		return vbigint(result), nil
	}
	return vnum(float64(^(a & b))), nil
}

func builtinLognor(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("lognor: need two integers")
	}
	a, b, ai, bi, isBig := logAndInts(args[0], args[1])
	if isBig {
		result := new(big.Int).Not(new(big.Int).Or(ai, bi))
		return vbigint(result), nil
	}
	return vnum(float64(^(a | b))), nil
}

func builtinLogandc1(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("logandc1: need two integers")
	}
	a, b, ai, bi, isBig := logAndInts(args[0], args[1])
	if isBig {
		result := new(big.Int).And(new(big.Int).Not(ai), bi)
		return vbigint(result), nil
	}
	return vnum(float64((^a) & b)), nil
}

func builtinLogandc2(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("logandc2: need two integers")
	}
	a, b, ai, bi, isBig := logAndInts(args[0], args[1])
	if isBig {
		result := new(big.Int).And(ai, new(big.Int).Not(bi))
		return vbigint(result), nil
	}
	return vnum(float64(a & (^b))), nil
}

func builtinLogorc1(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("logorc1: need two integers")
	}
	a, b, ai, bi, isBig := logAndInts(args[0], args[1])
	if isBig {
		result := new(big.Int).Or(new(big.Int).Not(ai), bi)
		return vbigint(result), nil
	}
	return vnum(float64((^a) | b)), nil
}

func builtinLogorc2(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("logorc2: need two integers")
	}
	a, b, ai, bi, isBig := logAndInts(args[0], args[1])
	if isBig {
		result := new(big.Int).Or(ai, new(big.Int).Not(bi))
		return vbigint(result), nil
	}
	return vnum(float64(a | (^b))), nil
}

// -------- logcount --------
func builtinLogcount(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("logcount: need an integer")
	}
	v := args[0]
	if v.typ == VBigInt {
		n := new(big.Int)
		if v.bigInt.Sign() < 0 {
			n.Not(v.bigInt)
		} else {
			n.Set(v.bigInt)
		}
		count := 0
		for n.Sign() > 0 {
			count++
			n.And(n, new(big.Int).Sub(n, big.NewInt(1)))
		}
		return vnum(float64(count)), nil
	}
	n := int64(toNum(args[0]))
	if n < 0 {
		n = ^n
	}
	count := 0
	for n != 0 {
		count++
		n &= n - 1
	}
	return vnum(float64(count)), nil
}

// -------- integer-length --------
func builtinIntegerLength(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("integer-length: need an integer")
	}
	v := args[0]
	if v.typ == VBigInt {
		n := new(big.Int)
		if v.bigInt.Sign() < 0 {
			n.Not(v.bigInt)
		} else {
			n.Set(v.bigInt)
		}
		bits := 0
		for n.Sign() > 0 {
			bits++
			n.Rsh(n, 1)
		}
		return vnum(float64(bits)), nil
	}
	n := int64(toNum(args[0]))
	if n < 0 {
		n = ^n
	}
	bits := 0
	for n != 0 {
		bits++
		n >>= 1
	}
	return vnum(float64(bits)), nil
}

// -------- logbitp --------
func builtinLogbitp(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("logbitp: need bit index and integer")
	}
	bit := int(toNum(args[0]))
	v := args[1]
	if v.typ == VBigInt {
		bitmask := new(big.Int).Rsh(v.bigInt, uint(bit))
		return vbool(bitmask.Bit(0) == 1), nil
	}
	n := int64(toNum(args[1]))
	return vbool((n>>uint(bit))&1 == 1), nil
}

// -------- logtest --------
func builtinLogtest(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("logtest: need two integers")
	}
	a := int64(toNum(args[0]))
	b := int64(toNum(args[1]))
	if a&b != 0 {
		return vbool(true), nil
	}
	return vbool(false), nil
}

// -------- copy-alist --------
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

// -------- mismatch --------
func builtinMismatch(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("mismatch: need two sequences")
	}
	s1 := seqToList(args[0])
	s2 := seqToList(args[1])
	start1, end1, start2, end2 := 0, -1, 0, -1
	var testFn, testNotFn, keyFn *Value
	testFn = nil
	testNotFn = nil
	keyFn = nil
	fromEnd := false
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
			case ":TEST":
				if i+1 < len(args) {
					i++
					testFn = args[i]
				}
			case ":TEST-NOT":
				if i+1 < len(args) {
					i++
					testNotFn = args[i]
				}
			case ":KEY":
				if i+1 < len(args) {
					i++
					keyFn = args[i]
				}
			case ":FROM-END":
				if i+1 < len(args) {
					i++
					fromEnd = isTruthy(args[i])
				}
			}
		}
	}
	if end1 < 0 || end1 > len(s1) {
		end1 = len(s1)
	}
	if end2 < 0 || end2 > len(s2) {
		end2 = len(s2)
	}
	// Helper to apply :key function
	applyKey := func(v *Value) *Value {
		if keyFn != nil {
			r, err := callFnOnSeq(keyFn, []*Value{v}, globalEnv)
			if err != nil {
				return v
			}
			return r
		}
		return v
	}
	// Helper to compare two elements using :test or :test-not or default eqVal
	elemsEqual := func(a, b *Value) bool {
		ka := applyKey(a)
		kb := applyKey(b)
		if testNotFn != nil {
			cmp, err := callFnOnSeq(testNotFn, []*Value{ka, kb}, globalEnv)
			if err != nil {
				return false
			}
			return !isTruthy(cmp)
		}
		if testFn != nil {
			cmp, err := callFnOnSeq(testFn, []*Value{ka, kb}, globalEnv)
			if err != nil {
				return false
			}
			return isTruthy(cmp)
		}
		return eqVal(ka, kb)
	}
	len1 := end1 - start1
	len2 := end2 - start2
	if fromEnd {
		// from-end: find the rightmost mismatch
		minLen := len1
		if len2 < minLen {
			minLen = len2
		}
		for i := minLen - 1; i >= 0; i-- {
			if !elemsEqual(s1[start1+i], s2[start2+i]) {
				return vnum(float64(i)), nil
			}
		}
		if len1 != len2 {
			return vnum(float64(minLen)), nil
		}
		return vnil(), nil
	}
	// Forward direction
	minLen := len1
	if len2 < minLen {
		minLen = len2
	}
	for i := 0; i < minLen; i++ {
		if !elemsEqual(s1[start1+i], s2[start2+i]) {
			return vnum(float64(i)), nil
		}
	}
	if len1 != len2 {
		return vnum(float64(minLen)), nil
	}
	return vnil(), nil
}
func builtinByte(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("byte: need size and position")
	}
	size := int(toNum(args[0]))
	position := int(toNum(args[1]))
	return list(vnum(float64(size)), vnum(float64(position))), nil
}

func builtinByteSize(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("byte-size: need a byte specifier")
	}
	bs := seqToList(args[0])
	if len(bs) >= 1 {
		return bs[0], nil
	}
	return vnum(0), nil
}

func builtinBytePosition(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("byte-position: need a byte specifier")
	}
	bs := seqToList(args[0])
	if len(bs) >= 2 {
		return bs[1], nil
	}
	return vnum(0), nil
}

func builtinLdb(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("ldb: need byte specifier and integer")
	}
	bs := seqToList(args[0])
	if len(bs) < 2 {
		return nil, fmt.Errorf("ldb: invalid byte specifier")
	}
	size := int(toNum(bs[0]))
	pos := int(toNum(bs[1]))
	n := int64(toNum(args[1]))
	if size <= 0 {
		return vnum(0), nil
	}
	if size > 63 {
		size = 63
	}
	mask := int64((1 << uint(size)) - 1)
	return vnum(float64((n >> uint(pos)) & mask)), nil
}

func builtinDpb(args []*Value) (*Value, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("dpb: need newbyte, byte specifier, and integer")
	}
	newByte := int64(toNum(args[0]))
	bs := seqToList(args[1])
	if len(bs) < 2 {
		return nil, fmt.Errorf("dpb: invalid byte specifier")
	}
	size := int(toNum(bs[0]))
	pos := int(toNum(bs[1]))
	n := int64(toNum(args[2]))
	if size <= 0 {
		return args[2], nil // zero-width field: no change
	}
	if size > 63 {
		size = 63
	}
	mask := int64((1 << uint(size)) - 1)
	return vnum(float64((n & ^(mask << uint(pos))) | ((newByte & mask) << uint(pos)))), nil
}

// -------- Byte manipulation helpers --------
func builtinLdbTest(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("ldb-test: need byte specifier and integer")
	}
	bs := seqToList(args[0])
	if len(bs) < 2 {
		return nil, fmt.Errorf("ldb-test: invalid byte specifier")
	}
	size := int(toNum(bs[0]))
	pos := int(toNum(bs[1]))
	n := int64(toNum(args[1]))
	if size <= 0 || size > 63 {
		return vbool(false), nil
	}
	mask := int64((1 << uint(size)) - 1)
	return vbool(((n >> uint(pos)) & mask) != 0), nil
}

func builtinMaskField(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("mask-field: need byte specifier and integer")
	}
	bs := seqToList(args[0])
	if len(bs) < 2 {
		return nil, fmt.Errorf("mask-field: invalid byte specifier")
	}
	size := int(toNum(bs[0]))
	pos := int(toNum(bs[1]))
	n := int64(toNum(args[1]))
	if size <= 0 {
		return vnum(0), nil
	}
	if size > 63 {
		size = 63
	}
	mask := int64((1 << uint(size)) - 1)
	return vnum(float64((n >> uint(pos)) & mask)), nil
}

func builtinDepositField(args []*Value) (*Value, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("deposit-field: need newbyte, byte specifier, and integer")
	}
	newByte := int64(toNum(args[0]))
	bs := seqToList(args[1])
	if len(bs) < 2 {
		return nil, fmt.Errorf("deposit-field: invalid byte specifier")
	}
	size := int(toNum(bs[0]))
	pos := int(toNum(bs[1]))
	n := int64(toNum(args[2]))
	if size <= 0 {
		return args[2], nil // zero-width field: no change
	}
	if size > 63 {
		size = 63
	}
	mask := int64((1 << uint(size)) - 1)
	return vnum(float64((n & ^(mask << uint(pos))) | ((newByte & mask) << uint(pos)))), nil
}

// -------- boole --------
func builtinBoole(args []*Value) (*Value, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("boole: need opcode, integer1, and integer2")
	}
	op := int(toNum(args[0]))
	a := int64(toNum(args[1]))
	b := int64(toNum(args[2]))
	var result int64
	switch op {
	case 0:
		result = 0 // boole-clr
	case 1:
		result = a & b // boole-and
	case 2:
		result = a & ^b // boole-andc1
	case 3:
		result = a // boole-1
	case 4:
		result = ^a & b // boole-andc2
	case 5:
		result = b // boole-2
	case 6:
		result = a ^ b // boole-xor
	case 7:
		result = a | b // boole-ior
	case 8:
		result = ^(a | b) // boole-nor
	case 9:
		result = ^(a ^ b) // boole-eqv
	case 10:
		result = ^b // boole-c2
	case 11:
		result = a | ^b // boole-orc2
	case 12:
		result = ^a // boole-c1
	case 13:
		result = ^a | b // boole-orc1
	case 14:
		result = ^(a & b) // boole-nand
	case 15:
		result = -1 // boole-set
	default:
		return nil, fmt.Errorf("boole: invalid opcode %d", op)
	}
	return vnum(float64(result)), nil
}
