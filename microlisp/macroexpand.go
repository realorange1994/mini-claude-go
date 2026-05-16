package microlisp

import (
	"fmt"
	"strings"
)

func builtinMacroexpand(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("macroexpand: need form")
	}
	form := args[0]
	depth := 0
	const maxMacroExpandDepth = 1000

	// Handle quasiquote specially since it's not a VMacro
	if form.typ == VPair && form.car != nil && form.car.typ == VSym && strings.EqualFold(form.car.str, "QUASIQUOTE") {
		if len(args) >= 1 && args[0].cdr != nil && args[0].cdr.typ == VPair && args[0].cdr.car != nil {
			expanded, e := evalQuasiquote(args[0].cdr.car, 0, globalEnv)
			if e != nil {
				return nil, fmt.Errorf("macroexpand: %s", e)
			}
			return list(vsym("quote"), expanded), nil
		}
		return form, nil
	}

	for form.typ == VPair && form.car != nil && form.car.typ == VSym {
		fn, err := globalEnv.Get(form.car.str)
		if err != nil || fn.typ != VMacro {
			break
		}
		depth++
		if depth > maxMacroExpandDepth {
			return nil, fmt.Errorf("macroexpand: expansion depth exceeded (%d)", maxMacroExpandDepth)
		}
		expanded, err := expandMacro(fn, form.cdr, globalEnv)
		if err != nil {
			return nil, fmt.Errorf("macroexpand: %s", err)
		}
		form = expanded
	}
	return form, nil
}

func builtinMacroexpand1(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("macroexpand-1: need form")
	}
	form := args[0]
	if form.typ == VPair && form.car != nil && form.car.typ == VSym {
		fn, err := globalEnv.Get(form.car.str)
		if err == nil && fn.typ == VMacro {
			expanded, err := expandMacro(fn, form.cdr, globalEnv)
			if err != nil {
				return nil, fmt.Errorf("macroexpand-1: %s", err)
			}
			return expanded, nil
		}
	}
	return form, nil
}
