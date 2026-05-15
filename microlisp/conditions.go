package microlisp

import (
	"fmt"
	"os"
	"strings"
)

// applyAndResolveTailCall applies fn to args and resolves any tailCall errors.
// This is needed because apply returns tailCall for VFunc, which defers (like warn's)
// would run before the tailCall body is actually evaluated.
func applyAndResolveTailCall(fn *Value, args *Value, env *Env) (*Value, error) {
	result, err := Apply(fn, args, env)
	if err != nil {
		if tc, ok := err.(*tailCall); ok {
			// Resolve the tailCall by evaluating in the eval loop
			for {
				result, err = Eval(tc.form, tc.env)
				if err != nil {
					if tc2, ok := err.(*tailCall); ok {
						tc = tc2
						continue
					}
					return nil, err
				}
				return result, nil
			}
		}
		return nil, err
	}
	return result, nil
}

// If matched, enters debugger (or calls *debugger-hook*) before handlers run.
func checkBreakOnSignals(cond *Value) {
	breakOn, err := globalEnv.Get("*break-on-signals*")
	if err != nil || isNil(breakOn) {
		return
	}
	if typepCheck(cond, breakOn, globalEnv) {
		if hook, e := globalEnv.Get("*debugger-hook*"); e == nil && hook != nil && hook.typ == VFunc {
			Apply(hook, list(cond, vnil()), globalEnv)
			return
		}
		// Default debugger: print message and continue
		fmt.Fprintf(os.Stderr, "\n;; DEBUGGER BREAK: condition signaled: %s\n", ToString(cond))
	}
}

// -------- Condition System Builtins --------

// conditionMatchesType checks if a condition instance matches a handler type symbol
// by traversing the CLOS class hierarchy.
func conditionMatchesType(cond *Value, typeSymbol string) bool {
	if cond.instClass == nil {
		return typeSymbol == "condition"
	}
	return classHasAncestor(cond.instClass, typeSymbol)
}

// classHasAncestor checks if cls (or its ancestors) has a class with the given name.
func classHasAncestor(cls *Value, name string) bool {
	seen := make(map[*Value]bool)
	return classHasAncestorRec(cls, name, seen)
}

func classHasAncestorRec(cls *Value, name string, seen map[*Value]bool) bool {
	if cls == nil {
		return strings.EqualFold(name, "condition")
	}
	if seen[cls] {
		return false // cycle detected
	}
	seen[cls] = true
	if strings.EqualFold(cls.str, name) {
		return true
	}
	if cls.classParents != nil {
		for _, parent := range cls.classParents {
			if classHasAncestorRec(parent, name, seen) {
				return true
			}
		}
	}
	return false
}

// classMatchesCondition checks if a handler type symbol matches a condition.
func classMatchesCondition(typeSym string, cond *Value) bool {
	if cond == nil || cond.typ != VInstance || cond.instClass == nil {
		return typeSym == "condition"
	}
	return classHasAncestor(cond.instClass, typeSym)
}

// findClass looks up a class by name, checking the class registry first.
func findClass(name string) *Value {
	if cls, ok := classRegistry[name]; ok {
		return cls
	}
	// Case-insensitive lookup: reader uppercases symbols, but builtins may use lowercase
	upper := strings.ToUpper(name)
	if cls, ok := classRegistry[upper]; ok {
		return cls
	}
	return globalEnv.bindings[name]
}

// instanceSlotWithInheritance looks up a slot value on an instance,
// walking up the class hierarchy to find inherited slot values.
func instanceSlotWithInheritance(inst *Value, slotName string) *Value {
	if inst.typ != VInstance || inst.instClass == nil {
		return nil
	}
	upperName := strings.ToUpper(slotName)
	// Check instance's own slots first
	if inst.instSlots != nil {
		if v, ok := inst.instSlots[upperName]; ok {
			return v
		}
	}
	// Check inherited slots by walking class CPL
	if inst.instClass.cpl != nil {
		for _, cls := range inst.instClass.cpl {
			if cls.typ == VClass && cls.body != nil && cls.body.typ == VPair {
				if v := findSlotInitformRec(cls.body, upperName); v != nil {
					return v
				}
			}
		}
	}
	return nil
}

// findSlotInitformRec walks a slot definition list looking for a slot with the given name
// and returns its :initform value if found.
func findSlotInitformRec(slotDefs *Value, slotName string) *Value {
	for !isNil(slotDefs) {
		slot := slotDefs.car
		var specName string
		var options *Value
		if slot.typ == VSym {
			specName = slot.str
			options = nil
		} else if slot.typ == VPair && slot.car != nil {
			if slot.car.typ == VSym {
				specName = slot.car.str
			}
			options = slot.cdr
		}
		if specName == slotName {
			// Parse options for :initform
			for !isNil(options) {
				opt := options.car
				if opt.typ == VSym && strings.EqualFold(opt.str, ":INITFORM") {
					if options.cdr != nil && options.cdr.typ == VPair {
						return options.cdr.car
					}
				}
				options = options.cdr
			}
			return nil // slot found but no :initform
		}
		slotDefs = slotDefs.cdr
	}
	return nil
}

// signalDivisionByZero creates a division-by-zero condition and checks handlers.
// If a handler catches it, panics with handledError. Otherwise returns a fallback error.
func signalDivisionByZero() error {
	cond := makeSimpleCondition("division-by-zero", "division by zero")
	checkBreakOnSignals(cond)
	if len(handlerStack) > 0 {
		for i := len(handlerStack) - 1; i >= 0; i-- {
			h := handlerStack[i]
			if conditionMatchesType(cond, h.typeSymbol) {
				fn := h.handlerFn
				if fn.typ == VPrim {
					result, err := fn.fn([]*Value{cond})
					if err != nil {
						panic(fmt.Errorf("handler-function panicked: %v", err))
					}
					panic(&handledError{condition: cond, result: result})
				} else if fn.typ == VFunc {
					result, err := Apply(fn, cons(cond, vnil()), h.env)
					if err != nil {
						panic(fmt.Errorf("handler-function panicked: %v", err))
					}
					panic(&handledError{condition: cond, result: result})
				}
			}
		}
	}
	return fmt.Errorf("division by zero")
}

// makeSimpleCondition is a helper to create a condition instance of the given class.
func makeSimpleCondition(className, msg string) *Value {
	cond := gcv()
	cond.typ = VInstance
	cond.instClass = findClass(className)
	if cond.instClass == nil {
		cond.instClass = findClass("condition")
	}
	cond.instSlots = map[string]*Value{
		"MESSAGE":          vstr(msg),
		"FORMAT-CONTROL":   vstr(msg),
		"FORMAT-ARGUMENTS": vnil(),
	}
	return cond
}

// formatMessage applies ~a/~A substitutions to a format string.
func formatMessage(format string, args []*Value) string {
	for _, a := range args {
		format = strings.Replace(format, "~a", ToString(primaryValue(a)), 1)
		format = strings.Replace(format, "~A", ToString(primaryValue(a)), 1)
	}
	return format
}

func builtinError(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("error: need at least 1 argument")
	}
	datum := args[0]
	var msg string
	var cond *Value
	if datum.typ == VStr {
		msg = datum.str
	} else if datum.typ == VInstance {
		cond = datum
		if len(args) > 1 {
			msg = formatMessage(ToString(primaryValue(args[1])), args[2:])
			cond.instSlots["MESSAGE"] = vstr(msg)
		}
	} else if datum.typ == VSym {
		cls := findClass(datum.str)
		if cls != nil && cls.typ == VClass {
			var err error
			cond, err = builtinMakeInstance(args)
			if err != nil {
				return nil, err
			}
		}
	}
	// For non-VInstance datum without condition class: create simple-error condition
	if cond == nil {
		if datum.typ != VStr {
			msg = ToString(datum)
		}
		if len(args) > 1 {
			msg = formatMessage(msg, args[1:])
		}
		cond = makeSimpleCondition("simple-error", msg)
	}

	// Track the signaled condition for compute-restarts filtering
	oldSignaled := signaledCondition
	signaledCondition = cond
	defer func() { signaledCondition = oldSignaled }()

	// ANSI CL: restart-case implicitly associates its restarts with the
	// condition being signaled. We temporarily associate restarts from the
	// innermost restart-case frame with the signaled condition, then restore.
	startIdx := innermostRestartFrame
	if startIdx < 0 {
		startIdx = 0
	}
	associatedIndices := []int{}
	for i := startIdx; i < len(restartStack); i++ {
		if restartStack[i].condition == nil {
			restartStack[i].condition = cond
			associatedIndices = append(associatedIndices, i)
		}
	}
	// Restore condition associations after handler walk
	defer func() {
		for _, idx := range associatedIndices {
			if idx < len(restartStack) && restartStack[idx].condition == cond {
				restartStack[idx].condition = nil
			}
		}
	}()

	checkBreakOnSignals(cond)
	// Walk handler stack
	if len(handlerStack) > 0 {
		for i := len(handlerStack) - 1; i >= 0; i-- {
			h := handlerStack[i]
			if conditionMatchesType(cond, h.typeSymbol) {
				fn := h.handlerFn
				if fn.typ == VPrim {
					_, err := fn.fn([]*Value{cond})
					if err != nil {
						if _, ok := err.(*tailCall); ok {
							panic(err)
						}
						if he, ok := err.(*handledError); ok {
							panic(he)
						}
						panic(fmt.Errorf("handler-function panicked: %v", err))
					}
					// Handler returned normally - continue signaling (handler-bind semantics)
				} else if fn.typ == VFunc {
					_, err := applyAndResolveTailCall(fn, cons(cond, vnil()), h.env)
					if err != nil {
						if _, ok := err.(*tailCall); ok {
							panic(err)
						}
						if he, ok := err.(*handledError); ok {
							panic(he)
						}
						panic(fmt.Errorf("handler-function panicked: %v", err))
					}
					// Handler returned normally - continue signaling (handler-bind semantics)
				}
			}
		}
	}
	// No handler caught it - return as Go error, wrapped with condition
	var goErr error
	if slotMsg, ok := cond.instSlots["MESSAGE"]; ok {
		goErr = fmt.Errorf("error: %s", ToString(slotMsg))
	} else if msg != "" {
		goErr = fmt.Errorf("error: %s", msg)
	} else {
		goErr = fmt.Errorf("error: %s", ToString(cond))
	}
	return nil, &conditionError{condition: cond, err: goErr}
}

// builtinCError implements (cerror continue-format-control datum &rest arguments)
// CL spec: signals a simple-error condition, establishes a continue restart.
// If no handler catches it, prints the error and returns nil (implicit continue).
func builtinCError(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("cerror: need at least 2 arguments (continue-format-control datum)")
	}
	contMsg := ToString(primaryValue(args[0]))
	datum := args[1]
	var errMsg string
	if datum.typ == VStr {
		errMsg = datum.str
		if len(args) > 2 {
			errMsg = formatMessage(errMsg, args[2:])
		}
	} else {
		errMsg = ToString(primaryValue(datum))
	}

	cond := makeSimpleCondition("simple-error", errMsg)

	// Establish continue restart (CL spec: cerror establishes a continue restart)
	continueEntry := restartEntry{
		name: "continue",
		handlerFn: &Value{typ: VPrim, fn: func(_ []*Value) (*Value, error) {
			return vnil(), nil
		}},
		env: globalEnv,
	}
	restartStack = append(restartStack, continueEntry)
	defer func() {
		restartStack = restartStack[:len(restartStack)-1]
	}()

	checkBreakOnSignals(cond)

	// Walk handler stack
	if len(handlerStack) > 0 {
		for i := len(handlerStack) - 1; i >= 0; i-- {
			h := handlerStack[i]
			if conditionMatchesType(cond, h.typeSymbol) {
				fn := h.handlerFn
				if fn.typ == VPrim {
					result, err := fn.fn([]*Value{cond})
					if err != nil {
						panic(fmt.Errorf("handler-function panicked: %v", err))
					}
					panic(&handledError{condition: cond, result: result})
				} else if fn.typ == VFunc {
					result, err := Apply(fn, cons(cond, vnil()), h.env)
					if err != nil {
						panic(fmt.Errorf("handler-function panicked: %v", err))
					}
					panic(&handledError{condition: cond, result: result})
				}
			}
		}
		panic(&handledError{condition: cond, result: nil})
	}

	// No handler matched — print error with continue message, return nil
	fmt.Fprintf(os.Stderr, "Error: %s\nContinue: %s\n", errMsg, contMsg)
	return vnil(), nil
}

// builtinWarn implements (warn format-string &rest args)
// CL spec: signals a simple-warning condition, establishes muffle-warning restart.
// If no handler handles it (or handler invokes muffle-warning), suppresses printing.
// Unlike error, warn does NOT transfer control via panic — handlers are called
// and if they return normally, warn continues to check the muffled flag.
func builtinWarn(args []*Value) (*Value, error) {
	msg := "Warning"
	if len(args) >= 1 {
		msg = ToString(primaryValue(args[0]))
	}
	if len(args) > 1 {
		msg = formatMessage(msg, args[1:])
	}

	cond := makeSimpleCondition("simple-warning", msg)

	// Check if muffle-warning restart already exists on the stack
	// (e.g., from an outer restart-case). If so, don't establish our own —
	// we want invoke-restart to find the outer one so restart-case can
	// evaluate its body and return the result.
	hasOuterMuffle := false
	for i := len(restartStack) - 1; i >= 0; i-- {
		if restartStack[i].name == "muffle-warning" {
			hasOuterMuffle = true
			break
		}
	}

	// Establish muffle-warning restart (CL spec: warn establishes this restart).
	// Only establish if no outer one exists, to avoid shadowing.
	muffled := false
	savedLen := len(restartStack)
	if !hasOuterMuffle {
		restartStack = append(restartStack, restartEntry{
			name: "muffle-warning",
			handlerFn: &Value{typ: VPrim, fn: func(_ []*Value) (*Value, error) {
				muffled = true
				return vnil(), nil
			}},
			env: globalEnv,
		})
		defer func() {
			restartStack = restartStack[:savedLen]
		}()
	}

	checkBreakOnSignals(cond)

	// Walk handler stack (like signal: do NOT panic — just call handlers).
	for i := len(handlerStack) - 1; i >= 0; i-- {
		h := handlerStack[i]
		if conditionMatchesType(cond, h.typeSymbol) {
			fn := h.handlerFn
			if fn.typ == VPrim {
				fn.fn([]*Value{cond})
			} else if fn.typ == VFunc {
				applyAndResolveTailCall(fn, cons(cond, vnil()), h.env)
			}
			break
		}
	}

	// After signaling, try to invoke muffle-warning. If an outer restart-case
	// established it, invoke-restart finds its nil entry and panics with
	// restartInvoke — the panic propagates to restart-case's defer, which
	// evaluates the body and returns the result. If no outer restart-case,
	// invoke-restart finds our VPrim entry, sets muffled=true, and returns.
	if !muffled {
		_, _ = builtinInvokeRestart([]*Value{vsym("muffle-warning")})
	}

	// If not muffled, print warning
	if !muffled {
		fmt.Fprintf(os.Stderr, "WARNING: %s\n", msg)
	}
	return vnil(), nil
}

func builtinSignal(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("signal: need at least 1 argument")
	}
	datum := args[0]
	var cond *Value
	if datum.typ == VInstance {
		cond = datum
	} else if datum.typ == VSym {
		cond = gcv()
		cond.typ = VInstance
		cond.instClass = findClass(datum.str)
		if cond.instClass == nil {
			cond.instClass = findClass("condition")
		}
		cond.instSlots = map[string]*Value{}
		if len(args) > 1 {
			cond.instSlots["MESSAGE"] = vstr(ToString(primaryValue(args[1])))
		}
	} else {
		cond = datum
	}

	checkBreakOnSignals(cond)

	for i := len(handlerStack) - 1; i >= 0; i-- {
		h := handlerStack[i]
		if conditionMatchesType(cond, h.typeSymbol) {
			fn := h.handlerFn
			if fn.typ == VPrim {
				result, err := fn.fn([]*Value{cond})
				if err != nil {
					return nil, err
				}
				return result, nil
			} else if fn.typ == VFunc {
				result, err := Apply(fn, cons(cond, vnil()), h.env)
				if err != nil {
					return nil, err
				}
				return result, nil
			}
		}
	}
	return vnil(), nil
}

func builtinInvokeRestart(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("invoke-restart: need restart name")
	}
	pv := primaryValue(args[0])
	if pv == nil || pv.typ != VSym {
		return nil, fmt.Errorf("invoke-restart: restart name must be a symbol")
	}
	name := pv.str
	restArgs := vnil()
	if len(args) > 1 {
		restArgs = list(args[1:]...)
	}
	for i := len(restartStack) - 1; i >= 0; i-- {
		r := restartStack[i]
		if r.name == name {
			if r.handlerFn != nil {
				return Apply(r.handlerFn, restArgs, r.env)
			}
			panic(&restartInvoke{name: name, args: restArgs, id: r.id})
		}
	}
	return nil, fmt.Errorf("invoke-restart: no restart named %s", name)
}

func builtinComputeRestarts(args []*Value) (*Value, error) {
	var result *Value = vnil()
	for i := len(restartStack) - 1; i >= 0; i-- {
		r := restartStack[i]
		result = cons(vsym(r.name), result)
	}
	return result, nil
}

func builtinRestartName(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("restart-name: need a restart")
	}
	v := primaryValue(args[0])
	if v == nil || v.typ != VSym {
		return nil, fmt.Errorf("restart-name: argument must be a restart identifier (symbol)")
	}
	return vsym(v.str), nil
}

func builtinFindRestart(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("find-restart: need restart name")
	}
	pv := primaryValue(args[0])
	if pv == nil || pv.typ != VSym {
		return nil, fmt.Errorf("find-restart: restart name must be a symbol")
	}
	name := pv.str
	for i := len(restartStack) - 1; i >= 0; i-- {
		if restartStack[i].name == name {
			return vsym(name), nil
		}
	}
	return vnil(), nil
}

func builtinMakeCondition(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("make-condition: need type")
	}
	typeVal := primaryValue(args[0])
	typeName := ""
	if typeVal.typ == VSym {
		typeName = typeVal.str
	} else if typeVal.typ == VInstance && typeVal.instClass != nil {
		typeName = typeVal.instClass.str
	}
	cls := findClass(typeName)
	if cls == nil || cls.typ != VClass {
		cls = findClass("condition")
		if cls == nil {
			return nil, fmt.Errorf("make-condition: unknown class %s", typeName)
		}
	}

	cond := gcv()
	cond.typ = VInstance
	cond.instClass = cls
	cond.instSlots = map[string]*Value{}

	// Apply :initform values from class slots (walk CPL in reverse for proper precedence)
	if cls.cpl != nil {
		for i := len(cls.cpl) - 1; i >= 0; i-- {
			c := cls.cpl[i]
			if c.typ == VClass && c.body != nil {
				applySlotInitforms(cond, c.body)
			}
		}
	}
	// Also apply this class's own slot initforms
	if cls.body != nil {
		applySlotInitforms(cond, cls.body)
	}

	// Apply keyword initargs (these override initforms)
	for i := 1; i+1 < len(args); i += 2 {
		key := primaryValue(args[i])
		val := primaryValue(args[i+1])
		if key.typ == VSym {
			slotName := key.str
			if len(slotName) > 0 && slotName[0] == ':' {
				slotName = slotName[1:]
			}
			// Resolve initarg to slot name (in case initarg differs from slot name)
			resolved := resolveInitargToSlot(cls, slotName)
			if resolved != "" {
				slotName = resolved
			}
			cond.instSlots[strings.ToUpper(slotName)] = val
		} else if key.typ == VSym {
			cond.instSlots[key.str] = val
		}
	}

	return cond, nil
}

// applySlotInitforms evaluates and applies :initform values from a class's slot definitions
func applySlotInitforms(inst *Value, slotDefs *Value) {
	for !isNil(slotDefs) {
		slot := slotDefs.car
		var slotName string
		var options *Value
		if slot.typ == VSym {
			slotName = slot.str
			options = nil
		} else if slot.typ == VPair && slot.car != nil {
			if slot.car.typ == VSym {
				slotName = slot.car.str
			}
			options = slot.cdr
		} else {
			slotDefs = slotDefs.cdr
			continue
		}

		// Parse slot options for :initform
		if options != nil {
			optCur := options
			for !isNil(optCur) {
				opt := optCur.car
				if opt.typ == VSym && strings.EqualFold(opt.str, ":INITFORM") {
					if optCur.cdr != nil && optCur.cdr.typ == VPair {
						initVal := optCur.cdr.car
						// Evaluate the initform if it's a form
						if initVal.typ == VPair || (initVal.typ == VSym && initVal.str != "NIL" && initVal.str != "T") {
							evaled, err := Eval(initVal, globalEnv)
							if err == nil {
								initVal = evaled
							}
						}
						inst.instSlots[strings.ToUpper(slotName)] = initVal
					}
					break
				}
				optCur = optCur.cdr
			}
		}

		slotDefs = slotDefs.cdr
	}
}

// resolveInitargToSlot maps an initarg keyword to its slot name in the class hierarchy
func resolveInitargToSlot(cls *Value, initarg string) string {
	classes := []*Value{cls}
	if cls.cpl != nil {
		classes = append(classes, cls.cpl...)
	}
	for _, c := range classes {
		if c.typ == VClass && c.body != nil {
			if name := findSlotNameForInitarg(c.body, initarg); name != "" {
				return name
			}
		}
	}
	return ""
}

// findSlotNameForInitarg searches slot definitions for an :initarg matching the given keyword
func findSlotNameForInitarg(slotDefs *Value, initarg string) string {
	for !isNil(slotDefs) {
		slot := slotDefs.car
		var slotName string
		var options *Value
		if slot.typ == VSym {
			slotName = slot.str
			options = nil
		} else if slot.typ == VPair && slot.car != nil {
			if slot.car.typ == VSym {
				slotName = slot.car.str
			}
			options = slot.cdr
		} else {
			slotDefs = slotDefs.cdr
			continue
		}

		// Check if any :initarg matches
		if options != nil {
			optCur := options
			for !isNil(optCur) {
				opt := optCur.car
				if opt.typ == VSym {
					optStr := opt.str
					if strings.HasPrefix(optStr, ":") {
						optStr = optStr[1:]
					}
					if strings.EqualFold(optStr, initarg) && !strings.EqualFold(optStr, "INITFORM") {
						return slotName
					}
				}
				optCur = optCur.cdr
			}
		}

		slotDefs = slotDefs.cdr
	}
	return ""
}

// -------- typep --------

func builtinTypep(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("typep: need value and type-specifier")
	}
	return vbool(typepCheck(args[0], args[1], globalEnv)), nil
}

// -------- Symbol property lists --------

func builtinCopySymbol(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("copy-symbol: need a symbol")
	}
	sym := args[0]
	if sym.typ != VSym {
		return nil, fmt.Errorf("copy-symbol: expected a symbol")
	}
	copy := gcv()
	copy.typ = VSym
	copy.str = sym.str
	// Copy plist if second arg is non-nil (CL: copy-symbol returns a new uninterned symbol)
	if len(args) >= 2 && !isNil(args[1]) {
		copy.plist = sym.plist
	}
	return copy, nil
}

func builtinGet(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("get: need symbol and indicator")
	}
	sym := args[0]
	indicator := args[1]
	if sym.typ != VSym {
		return nil, fmt.Errorf("get: expected a symbol")
	}
	// Walk plist: (indicator1 value1 indicator2 value2 ...)
	plist := sym.plist
	seen := make(map[*Value]bool)
	for !isNil(plist) && plist.typ == VPair && !isNil(plist.cdr) && plist.cdr.typ == VPair {
		if seen[plist] {
			break
		} // circular plist
		seen[plist] = true
		if eqVal(plist.car, indicator) {
			return plist.cdr.car, nil
		}
		plist = plist.cdr.cdr
	}
	return vnil(), nil
}

func builtinPutprop(args []*Value) (*Value, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("putprop: need symbol, value, indicator")
	}
	sym := args[0]
	value := args[1]
	indicator := args[2]
	if sym.typ != VSym {
		return nil, fmt.Errorf("putprop: expected a symbol")
	}
	// Walk plist: if indicator exists, update value; else append
	plist := sym.plist
	ppSeen := make(map[*Value]bool)
	for !isNil(plist) && plist.typ == VPair && !isNil(plist.cdr) && plist.cdr.typ == VPair {
		if ppSeen[plist] {
			break
		}
		ppSeen[plist] = true
		if eqVal(plist.car, indicator) {
			plist.cdr.car = value
			return value, nil
		}
		plist = plist.cdr.cdr
	}
	// Not found, append (indicator value) to plist
	newEntry := cons(indicator, cons(value, vnil()))
	if sym.plist == nil || isNil(sym.plist) {
		sym.plist = newEntry
	} else {
		// Append to end of plist (iterative, with cycle detection)
		appendToList := sym.plist
		appendToSeen := make(map[*Value]bool)
		for !isNil(appendToList.cdr) && appendToList.cdr.typ == VPair {
			if appendToSeen[appendToList] {
				break
			}
			appendToSeen[appendToList] = true
			appendToList = appendToList.cdr
		}
		appendToList.cdr = newEntry
	}
	return value, nil
}

func builtinRemprop(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("remprop: need symbol and indicator")
	}
	sym := args[0]
	indicator := args[1]
	if sym.typ != VSym {
		return nil, fmt.Errorf("remprop: expected a symbol")
	}
	plist := sym.plist
	var prev *Value = nil
	remSeen := make(map[*Value]bool)
	for !isNil(plist) && plist.typ == VPair && !isNil(plist.cdr) && plist.cdr.typ == VPair {
		if remSeen[plist] {
			break
		}
		remSeen[plist] = true
		if eqVal(plist.car, indicator) {
			// Remove this indicator+value pair
			if prev == nil {
				sym.plist = plist.cdr.cdr
			} else {
				prev.cdr = plist.cdr.cdr
			}
			return vsym("t"), nil
		}
		prev = plist
		plist = plist.cdr.cdr
	}
	return vnil(), nil
}

// -------- get-setf for (setf (get s 'foo) val) --------
func builtinGetSetf(args []*Value) (*Value, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("get-setf: need newval, symbol, indicator")
	}
	newVal := args[0]
	sym := args[1]
	indicator := args[2]
	_, err := builtinPutprop([]*Value{sym, newVal, indicator})
	if err != nil {
		return nil, err
	}
	return newVal, nil
}

// arrayElemTypeMatches checks if a declared array element type matches a type specifier.
// Used as a fast path in typepCheckRec for compound (vector/array element-type) checks.
func arrayElemTypeMatches(declaredType string, etSpec *Value, env *Env) bool {
	dt := strings.ToUpper(declaredType)
	if etSpec.typ == VSym {
		specName := strings.ToUpper(etSpec.str)
		if specName == "*" || specName == "T" {
			return true
		}
		if dt == "CHARACTER" {
			return specName == "CHARACTER" || specName == "BASE-CHAR" || specName == "STANDARD-CHAR"
		}
		if dt == "BIT" {
			return specName == "BIT" || specName == "INTEGER" || specName == "RATIONAL" || specName == "REAL" || specName == "NUMBER" || specName == "T"
		}
		if dt == "SINGLE-FLOAT" {
			return specName == "SINGLE-FLOAT" || specName == "FLOAT" || specName == "REAL" || specName == "NUMBER" || specName == "T"
		}
		if dt == "T" {
			return true
		}
		return dt == specName
	}
	return false
}

func typepCheck(val *Value, typeSpec *Value, env *Env) bool {
	return typepCheckRec(val, typeSpec, env, make(map[*Value]bool))
}

func typepCheckRec(val *Value, typeSpec *Value, env *Env, seen map[*Value]bool) bool {
	if seen[typeSpec] {
		return false
	}
	if isNil(typeSpec) || typeSpec.typ != VSym {
		if typeSpec.typ == VPair && typeSpec.car != nil && typeSpec.car.typ == VSym {
			seen[typeSpec] = true
			switch strings.ToUpper(typeSpec.car.str) {
			case "AND":
				// (and type1 type2 ...) - all must match
				body := typeSpec.cdr
				for !isNil(body) {
					if !typepCheckRec(val, body.car, env, seen) {
						return false
					}
					body = body.cdr
				}
				return true
			case "OR":
				// (or type1 type2 ...) - any must match
				body := typeSpec.cdr
				for !isNil(body) {
					if typepCheckRec(val, body.car, env, seen) {
						return true
					}
					body = body.cdr
				}
				return false
			case "NOT":
				// (not type) - must NOT match
				if typeSpec.cdr == nil || isNil(typeSpec.cdr) || typeSpec.cdr.typ != VPair {
					return false
				}
				return !typepCheckRec(val, typeSpec.cdr.car, env, seen)
			case "SATISFIES":
				// (satisfies fn) - predicate must return true
				body := typeSpec.cdr
				if !isNil(body) && body.typ == VPair && body.car != nil && body.car.typ == VSym {
					fnName := body.car.str
					fn, err := env.Get(fnName)
					if err == nil {
						if fn.typ == VPrim {
							result, err := fn.fn([]*Value{val})
							return err == nil && !isNil(result)
						} else if fn.typ == VFunc {
							result, err := Apply(fn, cons(val, vnil()), env)
							return err == nil && !isNil(result)
						}
					}
				}
				return false
			case "EQL":
				// (eql value) - must be eql to value
				body := typeSpec.cdr
				if !isNil(body) {
					return eqlCheck(val, body.car)
				}
				return false
			case "MEMBER":
				// (member v1 v2 ...) - must be eql to one of the values
				body := typeSpec.cdr
				for !isNil(body) {
					if eqlCheck(val, body.car) {
						return true
					}
					body = body.cdr
				}
				return false
case "ARRAY":
				// (array element-type) - check if array (strings are also arrays in CL)
				if val.typ != VArray && val.typ != VStr {
					return false
				}
				// Check element-type if specified
				elemTypeSpec := typeSpec.cdr
				if elemTypeSpec != nil && !isNil(elemTypeSpec) && elemTypeSpec.typ == VPair {
					etSpec := elemTypeSpec.car
					if !(etSpec.typ == VSym && strings.ToUpper(etSpec.str) == "*") {
						// Check declared elemType first for fast path
						if val.typ == VArray && val.array.elemType != "" && val.array.elemType != "T" {
							if !arrayElemTypeMatches(val.array.elemType, etSpec, env) {
								return false
							}
						} else {
							// Fall back to checking each element
							var elements []*Value
							if val.typ == VArray {
								elements = val.array.elements
							} else if val.typ == VStr {
								elements = make([]*Value, 0, len(val.str))
								for _, ch := range val.str {
									elements = append(elements, vchar(ch))
								}
							}
							for _, elem := range elements {
								if !typepCheckRec(primaryValue(elem), etSpec, env, seen) {
									return false
								}
							}
						}
					}
				}
				return true
			case "VECTOR":
				// (vector) - check if 1D array or string (strings are vectors in CL)
				// (vector element-type) - check if 1D array with matching element type
				if val.typ != VArray && val.typ != VStr {
					return false
				}
				if val.typ == VStr {
					// Strings are always 1D vectors
					// Check element-type if specified
					elemTypeSpec := typeSpec.cdr
					if elemTypeSpec != nil && !isNil(elemTypeSpec) && elemTypeSpec.typ == VPair {
						etSpec := elemTypeSpec.car
						if !(etSpec.typ == VSym && strings.ToUpper(etSpec.str) == "*") {
							// For strings, element type must be CHARACTER or equivalent
							if !typepCheckRec(vchar('a'), etSpec, env, seen) {
								return false
							}
						}
					}
					return true
				}
				if len(val.array.dims) != 1 {
					return false
				}
				// Check element-type if specified
				elemTypeSpec := typeSpec.cdr
				if elemTypeSpec != nil && !isNil(elemTypeSpec) && elemTypeSpec.typ == VPair {
					etSpec := elemTypeSpec.car
					if !(etSpec.typ == VSym && strings.ToUpper(etSpec.str) == "*") {
						// Check declared elemType first for fast path
						if val.array.elemType != "" && val.array.elemType != "T" {
							if !arrayElemTypeMatches(val.array.elemType, etSpec, env) {
								return false
							}
						} else {
							// Fall back to checking each element
							for _, elem := range val.array.elements {
								if !typepCheckRec(primaryValue(elem), etSpec, env, seen) {
									return false
								}
							}
						}
					}
				}
				return true
			case "CONS":
				// (cons) - check if it's a pair
				// (cons car-type) - check car matches car-type
				// (cons car-type cdr-type) - check car and cdr match their types
				if val.typ != VPair {
					return false
				}
				// Check cdr (second element if present)
				if typeSpec.cdr != nil && !isNil(typeSpec.cdr) {
					if typeSpec.cdr.typ == VPair && typeSpec.cdr.car != nil {
						carType := typeSpec.cdr.car
						cdrRemaining := typeSpec.cdr.cdr
						// Check car against car-type
						if !typepCheckRec(val.car, carType, env, seen) {
							return false
						}
						// If there's a cdr-type, check val.cdr against it
						if cdrRemaining != nil && !isNil(cdrRemaining) {
							if cdrRemaining.typ == VPair && cdrRemaining.car != nil {
								if !typepCheckRec(val.cdr, cdrRemaining.car, env, seen) {
									return false
								}
							}
						}
					}
				}
				return true
			case "INTEGER":
				return (val.typ == VNum && !val.isFloat && val.num == float64(int64(val.num))) || val.typ == VRat || val.typ == VBigInt
			case "FLOAT", "SINGLE-FLOAT", "DOUBLE-FLOAT", "SHORT-FLOAT", "LONG-FLOAT":
				return val.typ == VNum && val.isFloat
			case "NUMBER":
				return val.typ == VNum || val.typ == VRat || val.typ == VComplex || val.typ == VBigInt
			case "REAL":
				return val.typ == VNum || val.typ == VRat || val.typ == VBigInt
			case "STRING":
				return val.typ == VStr
			case "SYMBOL":
				return val.typ == VSym || val.typ == VNil
			case "LIST":
				return val.typ == VPair || val.typ == VNil
			case "FUNCTION":
				return val.typ == VPrim || val.typ == VFunc || val.typ == VGeneric
			case "PATHNAME":
				return val.typ == VPathname
			case "RANDOM-STATE":
				return val.typ == VRandomState
			case "PACKAGE":
				return val.typ == VPackage
			case "READTABLE":
				return val.typ == VReadtable
			case "METHOD":
				return val.typ == VMethod
			case "RESTART":
				return val.typ == VRestart
			case "GENERIC", "GENERIC-FUNCTION", "STANDARD-GENERIC-FUNCTION":
				return val.typ == VGeneric
			case "INSTANCE", "STANDARD-OBJECT", "STRUCTURE-OBJECT":
				return val.typ == VInstance
			case "HASH-TABLE":
				return val.typ == VVHash
			case "CHARACTER":
				return val.typ == VChar
			case "BASE-CHAR", "STANDARD-CHAR":
				return val.typ == VChar
			case "STREAM":
				return val.typ == VStream
			case "CLASS":
				return val.typ == VClass
			case "MACRO":
				return val.typ == VMacro
			case "BOOLEAN":
				return val.typ == VBool || isNil(val)
			case "SEQUENCE":
				return val.typ == VStr || val.typ == VPair || val.typ == VNil || val.typ == VArray
			case "ATOM":
				return val.typ != VPair
			case "RATIONAL":
				return val.typ == VRat || val.typ == VBigInt || (val.typ == VNum && !val.isFloat)
			case "COMPLEX":
				return val.typ == VComplex
			default:
				// Try as a class name
				if cls := findClass(typeSpec.car.str); cls != nil && cls.typ == VClass {
					if val.typ == VInstance && val.instClass != nil {
						return classHasAncestor(val.instClass, cls.str)
					}
				}
				return false
			}
		}
		return false
	}
	// Symbol type specifier
	typeName := strings.ToUpper(typeSpec.str)
	if typeName == "T" {
		return true
	}
	if typeName == "NULL" {
		return isNil(val)
	}
	if typeName == "NIL" {
		return false // nil is the empty type - never matches any value
	}
	if typeName == "INTEGER" {
		return (val.typ == VNum && !val.isFloat && val.num == float64(int64(val.num))) || val.typ == VRat || val.typ == VBigInt
	}
	if typeName == "FLOAT" || typeName == "SINGLE-FLOAT" || typeName == "DOUBLE-FLOAT" || typeName == "SHORT-FLOAT" || typeName == "LONG-FLOAT" {
		return val.typ == VNum && val.isFloat
	}
	if typeName == "NUMBER" {
		return val.typ == VNum || val.typ == VRat || val.typ == VComplex || val.typ == VBigInt
	}
	if typeName == "REAL" {
		return val.typ == VNum || val.typ == VRat || val.typ == VBigInt
	}
	if typeName == "RATIONAL" {
		return val.typ == VRat || val.typ == VBigInt || (val.typ == VNum && !val.isFloat)
	}
	if typeName == "COMPLEX" {
		return val.typ == VComplex
	}
	if typeName == "STRING" {
		return val.typ == VStr
	}
	if typeName == "SYMBOL" {
		return val.typ == VSym || val.typ == VNil
	}
	if typeName == "LIST" {
		return val.typ == VPair || val.typ == VNil
	}
	if typeName == "CONS" || typeName == "PAIR" {
		return val.typ == VPair
	}
	if typeName == "FUNCTION" {
		return val.typ == VPrim || val.typ == VFunc || val.typ == VGeneric
	}
	if typeName == "HASH-TABLE" {
		return val.typ == VVHash
	}
	if typeName == "CHARACTER" {
		return val.typ == VChar
	}
	if typeName == "BASE-CHAR" {
		return val.typ == VChar
	}
	if typeName == "STANDARD-CHAR" {
		if val.typ != VChar {
			return false
		}
		ch := val.ch
		// Standard chars: graphic chars in range 32-126 plus #\Newline
		return (ch >= 32 && ch <= 126) || ch == '\n'
	}
	if typeName == "EXTENDED-CHAR" {
		// In non-Unicode implementations, there are no extended chars
		return false
	}
	if typeName == "STREAM" {
		return val.typ == VStream
	}
	if typeName == "ARRAY" {
		return val.typ == VArray || val.typ == VStr
	}
	if typeName == "VECTOR" {
		return val.typ == VArray && len(val.array.dims) == 1 || val.typ == VStr
	}
	if typeName == "PROCEDURE" {
		return val.typ == VPrim || val.typ == VFunc
	}
	if typeName == "MACRO" {
		return val.typ == VMacro
	}
	if typeName == "CLASS" {
		return val.typ == VClass
	}
	if typeName == "BOOLEAN" {
		return val.typ == VBool || isNil(val)
	}
	if typeName == "SEQUENCE" {
		return val.typ == VStr || val.typ == VPair || val.typ == VNil || val.typ == VArray
	}
	if typeName == "ATOM" {
		return val.typ != VPair
	}
	if typeName == "PATHNAME" {
		return val.typ == VPathname
	}
	if typeName == "RANDOM-STATE" {
		return val.typ == VRandomState
	}
	if typeName == "PACKAGE" {
		return val.typ == VPackage
	}
	if typeName == "READTABLE" {
		return val.typ == VReadtable
	}
	if typeName == "METHOD" {
		return val.typ == VMethod
	}
	if typeName == "RESTART" {
		return val.typ == VRestart
	}
	if typeName == "GENERIC" || typeName == "GENERIC-FUNCTION" || typeName == "STANDARD-GENERIC-FUNCTION" {
		return val.typ == VGeneric
	}
	if typeName == "INSTANCE" || typeName == "STANDARD-OBJECT" || typeName == "STRUCTURE-OBJECT" {
		return val.typ == VInstance
	}
	if typeName == "THREAD" {
		return val.typ == VThread
	}
	if typeName == "LOCK" || typeName == "MUTEX" {
		return val.typ == VLock
	}
	if typeName == "SIMPLE-ARRAY" {
		return val.typ == VArray || val.typ == VStr
	}
	if typeName == "SIMPLE-STRING" {
		return val.typ == VStr
	}
	if typeName == "SIMPLE-VECTOR" {
		return val.typ == VArray
	}
	if typeName == "BIT-VECTOR" || typeName == "SIMPLE-BIT-VECTOR" {
		// Bit vector: a 1D array of bits
		if val.typ == VArray && len(val.array.dims) == 1 {
			return true // could be a bit vector
		}
		return false
	}
	// Try as a class name
	if val.typ == VInstance && val.instClass != nil {
		return classHasAncestor(val.instClass, typeName)
	}
	return false
}

func builtinAbort(args []*Value) (*Value, error) {
	return builtinInvokeRestart(append([]*Value{vsym("abort")}, args...))
}

func builtinContinue(args []*Value) (*Value, error) {
	return builtinInvokeRestart(append([]*Value{vsym("continue")}, args...))
}

func builtinMuffleWarning(args []*Value) (*Value, error) {
	return builtinInvokeRestart(append([]*Value{vsym("muffle-warning")}, args...))
}

func builtinStoreValue(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("store-value: need a value")
	}
	return builtinInvokeRestart(append([]*Value{vsym("store-value")}, args...))
}

func builtinUseValue(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("use-value: need a value")
	}
	return builtinInvokeRestart(append([]*Value{vsym("use-value")}, args...))
}

func eqlCheck(a, b *Value) bool {
	a = primaryValue(a)
	b = primaryValue(b)
	if a.typ != b.typ {
		return false
	}
	switch a.typ {
	case VNum:
		return a.num == b.num
	case VStr:
		return a.str == b.str
	case VSym:
		return a == b
	case VChar:
		return a.ch == b.ch
	case VBool:
		return a == b
	case VNil:
		return true
	default:
		return a == b
	}
}

// -------- Debugging Builtins --------

// builtinBreak implements (break format-string &rest args)
// CL spec: signals a break condition (class `break`, subclass of serious-condition),
// establishes a continue restart, and enters the debugger if not handled.
func builtinBreak(args []*Value) (*Value, error) {
	msg := "BREAK"
	if len(args) >= 1 {
		if args[0].typ == VStr {
			msg = args[0].str
			if len(args) > 1 {
				msg = formatMessage(msg, args[1:])
			}
		} else {
			msg = ToString(primaryValue(args[0]))
		}
	}

	// Create break condition (class `break`, subclass of serious-condition)
	cond := gcv()
	cond.typ = VInstance
	cond.instClass = findClass("break")
	if cond.instClass == nil {
		cond.instClass = findClass("serious-condition")
	}
	cond.instSlots = map[string]*Value{
		"MESSAGE":          vstr(msg),
		"FORMAT-CONTROL":   vstr(msg),
		"FORMAT-ARGUMENTS": vnil(),
	}

	// Establish continue restart (CL spec: break establishes continue restart)
	continueEntry := restartEntry{
		name: "continue",
		handlerFn: &Value{typ: VPrim, fn: func(_ []*Value) (*Value, error) {
			return vnil(), nil
		}},
		env: globalEnv,
	}
	restartStack = append(restartStack, continueEntry)
	defer func() {
		restartStack = restartStack[:len(restartStack)-1]
	}()

	// Walk handler stack
	if len(handlerStack) > 0 {
		for i := len(handlerStack) - 1; i >= 0; i-- {
			h := handlerStack[i]
			if conditionMatchesType(cond, h.typeSymbol) {
				fn := h.handlerFn
				if fn.typ == VPrim {
					return fn.fn([]*Value{cond})
				} else if fn.typ == VFunc {
					return Apply(fn, cons(cond, vnil()), h.env)
				}
			}
		}
	}

	// Check *debugger-hook*
	if hook, err := globalEnv.Get("*debugger-hook*"); err == nil && hook != nil && hook.typ == VFunc {
		result, _ := Apply(hook, list(cond, vnil()), globalEnv)
		return result, nil
	}

	// Default debugger behavior: print message and return nil
	fmt.Fprintf(os.Stderr, "\n;; BREAK: %s\n", msg)
	return vnil(), nil
}

// goErrorToCondition converts a Go error to a Lisp condition object.
// For file-related errors (containing "load: open"), creates a file-error condition.
// Otherwise creates a simple-error condition.
func goErrorToCondition(err error) *Value {
	msg := err.Error()
	cond := gcv()
	cond.typ = VInstance

	// Try to create a file-error for file-related errors
	if strings.Contains(msg, "load: open") || strings.Contains(msg, "open ") {
		cond.instClass = findClass("file-error")
		if cond.instClass != nil {
			// Try to extract filename from error message
			// Format is typically: "load: open <filename>: <error>"
			parts := strings.SplitN(msg, "open ", 2)
			if len(parts) >= 2 {
				fnameParts := strings.SplitN(parts[1], ":", 2)
				fname := strings.TrimSpace(fnameParts[0])
				fname = strings.Trim(fname, "\"")
				cond.instSlots = map[string]*Value{
					"file-pathname":  vstr(fname),
					"message":        vstr(msg),
					"format-control": vstr(msg),
					"format-arguments": vnil(),
				}
				return cond
			}
		}
	}

	// Default: simple-error condition
	cond.instClass = findClass("simple-error")
	if cond.instClass == nil {
		cond.instClass = findClass("error")
		if cond.instClass == nil {
			cond.instClass = findClass("condition")
		}
	}
	cond.instSlots = map[string]*Value{
		"MESSAGE":          vstr(msg),
		"FORMAT-CONTROL":   vstr(msg),
		"FORMAT-ARGUMENTS": vnil(),
	}
	return cond
}
// -------- with-condition-restarts support builtins --------

// builtinAssociateRestarts associates restart objects with a condition.
// Called by the with-condition-restarts macro.
func builtinAssociateRestarts(args []*Value) (*Value, error) {
	// Stub: in a full implementation, this would link restartEntry.condition
	// in the restartStack to the given condition. For now, the restart-case
	// mechanism already auto-associates restarts via conditionError wrapping.
	return vnil(), nil
}

// builtinDissociateRestarts removes condition associations from restart objects.
// Called by the with-condition-restarts macro's cleanup form.
func builtinDissociateRestarts(args []*Value) (*Value, error) {
	// Stub: cleanup is handled by unwind-protect naturally.
	return vnil(), nil
}
