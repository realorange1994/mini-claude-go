package microlisp

import (
	"fmt"
	"math"
	"math/big"
	"math/rand"
	"strings"
	"time"
)

func builtinGetf(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("getf: need plist and indicator")
	}
	plist := args[0]
	indicator := args[1]
	defaultVal := vnil()
	if len(args) >= 3 {
		defaultVal = args[2]
	}
	cur := plist
	for cur != nil && cur.typ == VPair {
		key := cur.car
		if cur.cdr != nil && cur.cdr.typ == VPair {
			val := cur.cdr.car
			if eqVal(key, indicator) {
				return val, nil
			}
			cur = cur.cdr.cdr
		} else {
			break
		}
	}
	return defaultVal, nil
}

func builtinGetProperties(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("get-properties: need plist and indicator-list")
	}
	plist := args[0]
	indicators := seqToList(args[1])
	cur := plist
	for cur != nil && cur.typ == VPair {
		key := cur.car
		if cur.cdr != nil && cur.cdr.typ == VPair {
			val := cur.cdr.car
			for _, ind := range indicators {
				if eqVal(key, ind) {
					// ANSI CL: returns (values indicator value tail)
					return multiVal(key, val, cur), nil
				}
			}
			cur = cur.cdr.cdr
		} else {
			break
		}
	}
	return multiVal(vnil(), vnil(), vnil()), nil
}

func builtinRemf(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("remf: need plist and indicator")
	}
	plist := args[0]
	indicator := args[1]
	// Find and remove the indicator/value pair
	elems := seqToList(plist)
	result := make([]*Value, 0)
	i := 0
	for i < len(elems)-1 {
		if eqVal(elems[i], indicator) {
			i += 2 // skip key and value
			continue
		}
		result = append(result, elems[i], elems[i+1])
		i += 2
	}
	return listFromSlice(result), nil
}

func builtinMakeList(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("make-list: need size")
	}
	size := int(toNum(args[0]))
	initVal := vnil()
	for i := 1; i < len(args); i++ {
		if args[i].typ == VSym && args[i].str == ":INITIAL-ELEMENT" {
			if i+1 < len(args) {
				i++
				initVal = args[i]
			}
		}
	}
	result := make([]*Value, size)
	for i := range result {
		result[i] = initVal
	}
	return listFromSlice(result), nil
}

func builtinMakeSequence(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("make-sequence: need type and size")
	}
	typeVal := args[0]
	size := int(toNum(args[1]))
	initVal := vnil()
	for i := 2; i < len(args); i++ {
		if args[i].typ == VSym && strings.EqualFold(args[i].str, ":INITIAL-ELEMENT") {
			if i+1 < len(args) {
				i++
				initVal = args[i]
			}
		}
	}

	typeName := ""
	switch typeVal.typ {
	case VSym:
		typeName = typeVal.str
	case VStr:
		typeName = typeVal.str
	default:
		typeName = ToString(typeVal)
	}
	typeName = strings.ToUpper(typeName)

	switch typeName {
	case "LIST", "CONS", "NULL":
		result := make([]*Value, size)
		for i := range result {
			result[i] = initVal
		}
		return listFromSlice(result), nil
	case "VECTOR", "SIMPLE-VECTOR", "ARRAY", "SIMPLE-ARRAY":
		return makeArrayWithInit(size, initVal), nil
	case "STRING", "SIMPLE-STRING", "BASE-STRING":
		var ch rune = ' '
		if initVal.typ == VChar {
			ch = initVal.ch
		} else if initVal.typ == VStr && len(initVal.str) > 0 {
			ch = []rune(initVal.str)[0]
		} else if initVal.typ == VSym && len(initVal.str) > 0 {
			ch = []rune(initVal.str)[0]
		} else if !isNil(initVal) {
			return nil, fmt.Errorf("make-sequence: :initial-element is not a character designator: %s", ToString(initVal))
		}
		return vstr(strings.Repeat(string(ch), size)), nil
	case "BIT-VECTOR", "SIMPLE-BIT-VECTOR":
		var bitVal int
		if initVal.typ == VNum {
			bitVal = int(initVal.num)
			if bitVal != 0 && bitVal != 1 {
				return nil, fmt.Errorf("make-sequence: bit must be 0 or 1")
			}
		}
		elems := make([]*Value, size)
		for i := range elems {
			elems[i] = vnum(float64(bitVal))
		}
		arr := &LispArray{dims: []int{size}, elements: elems, adjustable: false}
		v := gcv()
		v.typ = VArray
		v.array = arr
		return v, nil
	default:
		return nil, fmt.Errorf("make-sequence: unsupported type: %s", typeName)
	}
}

func makeArrayWithInit(size int, initVal *Value) *Value {
	elems := make([]*Value, size)
	for i := range elems {
		elems[i] = initVal
	}
	arr := &LispArray{dims: []int{size}, elements: elems, adjustable: false}
	v := gcv()
	v.typ = VArray
	v.array = arr
	return v
}

func builtinRandom(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("random: need limit")
	}
	v := primaryValue(args[0])
	if v.typ != VNum && v.typ != VBigInt && v.typ != VRat {
		return nil, fmt.Errorf("random: argument must be a positive number")
	}
	// Determine which rng to use
	var randState *rand.Rand
	for i := 1; i+1 < len(args); i++ {
		if args[i].typ == VSym && args[i].str == ":RANDOM-STATE" && args[i+1].typ == VRandomState {
			if args[i+1].randState != nil {
				randState = args[i+1].randState
			}
			break
		}
	}
	// If no :random-state provided, use *random-state* special variable
	if randState == nil {
		if rv, err := globalEnv.Get("*random-state*"); err == nil && rv.typ == VRandomState && rv.randState != nil {
			randState = rv.randState
		}
	}
	// Big integer: use big.Int for proper arbitrary-precision random
	if v.typ == VBigInt {
		if v.bigInt.Sign() <= 0 {
			return nil, fmt.Errorf("random: limit must be > 0")
		}
		var rnd *rand.Rand
		if randState != nil {
			rnd = randState
		} else {
			rnd = rand.New(rand.NewSource(time.Now().UnixNano()))
		}
		result := new(big.Int).Rand(rnd, v.bigInt)
		return vbigInt(result), nil
	}
	// Rational: truncate to integer
	if v.typ == VRat {
		if v.irat <= 0 {
			return nil, fmt.Errorf("random: limit must be > 0")
		}
		intLimit := int(v.irat / v.iden)
		if intLimit < 1 {
			return nil, fmt.Errorf("random: limit must be >= 1")
		}
		if randState != nil {
			return vnum(float64(randState.Intn(intLimit))), nil
		}
		return vnum(float64(rand.Intn(intLimit))), nil
	}
	// VNum: float returns random float in [0, limit); integer returns random int in [0, limit)
	limit := v.num
	if limit <= 0 {
		return nil, fmt.Errorf("random: limit must be > 0")
	}
	if math.Abs(limit-math.Trunc(limit)) > 1e-12 {
		// Float limit: return random float
		if randState != nil {
			return vnum(randState.Float64() * limit), nil
		}
		return vnum(rand.Float64() * limit), nil
	}
	// Integer limit
	intLimit := int(limit)
	if intLimit < 1 {
		return nil, fmt.Errorf("random: limit must be >= 1")
	}
	if randState != nil {
		return vnum(float64(randState.Intn(intLimit))), nil
	}
	return vnum(float64(rand.Intn(intLimit))), nil
}
