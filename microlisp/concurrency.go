package microlisp

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"unicode"
)

var traceTable = make(map[string]bool)

var traceDepth = 0

func builtinTrace(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("trace: need function name")
	}
	var result []*Value
	for _, arg := range args {
		v := primaryValue(arg)
		name := ""
		switch v.typ {
		case VFunc, VPrim:
			name = v.name
		case VSym:
			name = v.str
		}
		if name != "" {
			traceTable[name] = true
			result = append(result, vsym(name))
		}
	}
	return listFromSlice(result), nil
}

func builtinUntrace(args []*Value) (*Value, error) {
	if len(args) < 1 {
		names := make([]string, 0, len(traceTable))
		for name := range traceTable {
			names = append(names, name)
		}
		traceTable = make(map[string]bool)
		result := make([]*Value, len(names))
		for i, n := range names {
			result[i] = vsym(n)
		}
		return listFromSlice(result), nil
	}
	var result []*Value
	for _, arg := range args {
		v := primaryValue(arg)
		name := ""
		switch v.typ {
		case VFunc, VPrim:
			name = v.name
		case VSym:
			name = v.str
		}
		if name != "" {
			delete(traceTable, name)
		}
		result = append(result, vsym(name))
	}
	return listFromSlice(result), nil
}

func builtinMakeCondVar(args []*Value) (*Value, error) {
	cid := atomic.AddInt64(&nextCondID, 1)
	condMu.Lock()
	// Create a condition variable associated with no specific lock initially
	// When wait is called with a lock, we associate the cond with that lock
	condVars[cid] = sync.NewCond(&sync.Mutex{})
	condMu.Unlock()
	return &Value{typ: VCondition, num: float64(cid)}, nil
}

func builtinConditionWait(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("condition-wait: need condition and lock")
	}
	if args[0].typ != VCondition || args[1].typ != VLock {
		return nil, fmt.Errorf("condition-wait: need condition and lock objects")
	}
	cid := int64(args[0].num)
	lid := int64(args[1].num)
	// Release the user lock, wait on condition, then reacquire
	condMu.Lock()
	cv, ok := condVars[cid]
	condMu.Unlock()
	if !ok {
		return nil, fmt.Errorf("condition-wait: invalid condition")
	}
	// The condition variable uses its own internal mutex for signaling
	cv.L.Lock()
	// Signal that we're about to wait (release user lock)
	lockMapMu.Lock()
	userMu, ok2 := lockMutexMap[lid]
	lockMapMu.Unlock()
	if !ok2 {
		return nil, fmt.Errorf("condition-wait: invalid lock")
	}
	userMu.Unlock() // Release the user lock
	cv.Wait()       // Wait on condition
	userMu.Lock()   // Reacquire the user lock
	return vnil(), nil
}

func builtinConditionNotify(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VCondition {
		return nil, fmt.Errorf("condition-notify: need a condition object")
	}
	cid := int64(args[0].num)
	condMu.Lock()
	cv, ok := condVars[cid]
	condMu.Unlock()
	if !ok {
		return nil, fmt.Errorf("condition-notify: invalid condition")
	}
	cv.Signal()
	return vnil(), nil
}

func builtinConditionBroadcast(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VCondition {
		return nil, fmt.Errorf("condition-broadcast: need a condition object")
	}
	cid := int64(args[0].num)
	condMu.Lock()
	cv, ok := condVars[cid]
	condMu.Unlock()
	if !ok {
		return nil, fmt.Errorf("condition-broadcast: invalid condition")
	}
	cv.Broadcast()
	return vnil(), nil
}

func builtinThreadP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	return vbool(args[0].typ == VThread), nil
}

func builtinLockP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	return vbool(args[0].typ == VLock), nil
}

func builtinCondVarP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	return vbool(args[0].typ == VCondition), nil
}

func builtinAtomicIncf(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("atomic-incf: need a reference")
	}
	delta := int64(1)
	if len(args) >= 2 {
		delta = int64(primaryValue(args[1]).num)
	}
	newVal := atomic.AddInt64(&atomicCounter, delta)
	return vnum(float64(newVal)), nil
}

func builtinAtomicDecf(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("atomic-decf: need a reference")
	}
	delta := int64(1)
	if len(args) >= 2 {
		delta = int64(primaryValue(args[1]).num)
	}
	newVal := atomic.AddInt64(&atomicCounter, -delta)
	return vnum(float64(newVal)), nil
}

func builtinAtomicGet(args []*Value) (*Value, error) {
	return vnum(float64(atomic.LoadInt64(&atomicCounter))), nil
}

func builtinAtomicSet(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("atomic-set: need a value")
	}
	val := int64(primaryValue(args[0]).num)
	atomic.StoreInt64(&atomicCounter, val)
	return vnum(float64(val)), nil
}

func builtinEval(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("eval: need expression")
	}
	return Eval(args[0], globalEnv)
}

func builtinEvalString(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("eval-string: need a string")
	}
	s := primaryValue(args[0])
	if s.typ != VStr {
		return nil, fmt.Errorf("eval-string: need a string, got %s", ToString(s))
	}
	return EvalString(s.str, globalEnv)
}

func builtinHandlerEval(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("handler-eval: need expression")
	}
	result, err := Eval(args[0], globalEnv)
	if err != nil {
		// Signal as a condition so handler-case can catch it
		return builtinError([]*Value{vstr(err.Error())})
	}
	return result, nil
}

func builtinFuncall(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("funcall: need function")
	}
	fn := args[0]
	callArgs := args[1:]
	return callFnOnSeq(fn, callArgs, globalEnv)
}

func builtinEql(args []*Value) (*Value, error) {
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

func builtinEqual(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return vbool(false), nil
	}
	return vbool(eqVal(args[0], args[1])), nil
}

func builtinEqualp(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return vbool(false), nil
	}
	return vbool(equalpVal(args[0], args[1])), nil
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
