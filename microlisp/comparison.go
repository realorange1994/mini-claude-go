package microlisp

import "fmt"

func builtinEq(args []*Value) (*Value, error) {
	if len(args) == 0 {
		return vbool(true), nil
	}
	for _, a := range args {
		if !isNumber(a) {
			return signalTypeError(a)
		}
	}
	if len(args) == 1 {
		return vbool(true), nil
	}
	for i := 1; i < len(args); i++ {
		if compareNumeric(args[i-1], args[i]) != 0 {
			return vbool(false), nil
		}
	}
	return vbool(true), nil
}

func builtinNe(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return vbool(false), nil
	}
	for i := 0; i < len(args); i++ {
		for j := i + 1; j < len(args); j++ {
			if compareNumeric(args[i], args[j]) == 0 {
				return vbool(false), nil
			}
		}
	}
	return vbool(true), nil
}

func builtinLt(args []*Value) (*Value, error) {
	for _, a := range args {
		if !isReal(a) {
			return signalTypeError(a)
		}
	}
	if len(args) < 2 {
		return vbool(true), nil
	}
	for i := 1; i < len(args); i++ {
		if compareNumeric(args[i-1], args[i]) >= 0 {
			return vbool(false), nil
		}
	}
	return vbool(true), nil
}

func builtinGt(args []*Value) (*Value, error) {
	for _, a := range args {
		if !isReal(a) {
			return signalTypeError(a)
		}
	}
	if len(args) < 2 {
		return vbool(true), nil
	}
	for i := 1; i < len(args); i++ {
		if compareNumeric(args[i-1], args[i]) <= 0 {
			return vbool(false), nil
		}
	}
	return vbool(true), nil
}

func builtinLe(args []*Value) (*Value, error) {
	for _, a := range args {
		if !isReal(a) {
			return signalTypeError(a)
		}
	}
	if len(args) < 2 {
		return vbool(true), nil
	}
	for i := 1; i < len(args); i++ {
		if compareNumeric(args[i-1], args[i]) > 0 {
			return vbool(false), nil
		}
	}
	return vbool(true), nil
}

func builtinGe(args []*Value) (*Value, error) {
	for _, a := range args {
		if !isReal(a) {
			return signalTypeError(a)
		}
	}
	if len(args) < 2 {
		return vbool(true), nil
	}
	for i := 1; i < len(args); i++ {
		if compareNumeric(args[i-1], args[i]) < 0 {
			return vbool(false), nil
		}
	}
	return vbool(true), nil
}

func builtinCons(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("cons: need 2 arguments")
	}
	return cons(args[0], args[1]), nil
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

// builtinSet implements CL's (set symbol value) - sets the dynamic value of a symbol
func builtinSet(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("set: need symbol and value")
	}
	sym := args[0]
	if sym.typ != VSym {
		return nil, fmt.Errorf("set: first argument must be a symbol")
	}
	val := args[1]
	globalEnv.Set(sym.str, val)
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

// builtinSetCar is used for (setf (car x) val) -> (car-setf val x)
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

// builtinSetCdrAsSetter is used for (setf (cdr x) val) -> (cdr-setf val x)
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

func builtinList(args []*Value) (*Value, error) {
	return listFromSlice(args), nil
}
