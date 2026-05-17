package microlisp

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

// builtinSet implements CL's (set symbol value) - sets the dynamic value of a symbol

// builtinSetCar is used for (setf (car x) val) -> (car-setf val x)

// builtinSetCdrAsSetter is used for (setf (cdr x) val) -> (cdr-setf val x)
