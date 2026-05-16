package microlisp

import (
	"fmt"
	"strings"
)

func builtinReadtableP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	return vbool(isReadtable(args[0])), nil
}

func builtinMakeReadtable(args []*Value) (*Value, error) {
	rt := &Readtable{
		macroFns: make(map[rune]*macroEntry),
		dispFns:  make(map[rune]*macroEntry),
		caseMode: ":UPCASE",
	}
	return vrt(rt), nil
}

func builtinCopyReadtable(args []*Value) (*Value, error) {
	src := currentReadtable
	if len(args) >= 1 && isReadtable(args[0]) {
		src = args[0].readtable
	}
	rt := &Readtable{
		macroFns: make(map[rune]*macroEntry),
		dispFns:  make(map[rune]*macroEntry),
		caseMode: src.caseMode,
	}
	for k, v := range src.macroFns {
		entry := *v
		rt.macroFns[k] = &entry
	}
	for k, v := range src.dispFns {
		entry := *v
		rt.dispFns[k] = &entry
	}
	return vrt(rt), nil
}

func builtinReadtableCase(args []*Value) (*Value, error) {
	if len(args) < 1 || !isReadtable(args[0]) {
		return nil, fmt.Errorf("readtable-case: expected a readtable")
	}
	return mkKeyword(args[0].readtable.caseMode), nil
}

func builtinSetReadtableCase(args []*Value) (*Value, error) {
	if len(args) < 2 || !isReadtable(args[0]) {
		return nil, fmt.Errorf("set-readtable-case: expected readtable and case mode")
	}
	mode := args[1]
	var modeStr string
	if mode.typ == VSym && isKeyword(mode.str) {
		modeStr = mode.str
	} else if mode.typ == VStr {
		modeStr = ":" + strings.ToUpper(mode.str)
	} else {
		return nil, fmt.Errorf("set-readtable-case: invalid case mode %v", mode)
	}
	switch modeStr {
	case ":UPCASE", ":DOWNCASE", ":PRESERVE", ":INVERT":
		args[0].readtable.caseMode = modeStr
	default:
		return nil, fmt.Errorf("set-readtable-case: invalid case mode %s (expected :UPCASE, :DOWNCASE, :PRESERVE, or :INVERT)", modeStr)
	}
	return mkKeyword(modeStr), nil
}

func builtinSetMacroCharacter(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("set-macro-character: need char and function")
	}
	var ch rune
	if args[0].typ == VChar {
		ch = args[0].ch
	} else {
		return nil, fmt.Errorf("set-macro-character: first argument must be a character")
	}
	fn := args[1]
	// CL spec: nil means remove the macro character
	if fn.typ == VNil {
		delete(currentReadtable.macroFns, ch)
		return vbool(true), nil
	}
	if fn.typ != VFunc && fn.typ != VPrim {
		return nil, fmt.Errorf("set-macro-character: second argument must be a function")
	}
	nonTerm := false
	if len(args) >= 3 {
		nonTerm = isTruthy(args[2])
	}
	rt := currentReadtable
	if len(args) >= 4 && isReadtable(args[3]) {
		rt = args[3].readtable
	}
	// Recognize close-paren sentinel: register as terminating macro character
	// (overrides the non-terminating flag if passed)
	isCloseParen := (fn == closeParenSentinel)
	rt.macroFns[ch] = &macroEntry{
		lispFn:      fn,
		terminating: isCloseParen || !nonTerm,
	}
	return vbool(true), nil
}

func builtinGetMacroCharacter(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("get-macro-character: need a character")
	}
	var ch rune
	if args[0].typ == VChar {
		ch = args[0].ch
	} else {
		return nil, fmt.Errorf("get-macro-character: argument must be a character")
	}
	rt := currentReadtable
	if len(args) >= 2 && isReadtable(args[1]) {
		rt = args[1].readtable
	}
	entry, ok := rt.macroFns[ch]
	if !ok || entry == nil {
		return vnil(), nil
	}
	if entry.goFn != nil {
		// Return nil for Go-level macro functions (not introspectable as Lisp fns)
		return vnil(), nil
	}
	if entry.lispFn != nil {
		return entry.lispFn, nil
	}
	// Standard macro character with no Lisp-level or Go-level function.
	// For close-paren ')', return the sentinel so set-macro-character can
	// recognize it and register the target character as close-paren equivalent.
	if ch == ')' {
		return closeParenSentinel, nil
	}
	// For other standard macro chars, return a wrapper that signals an error
	// when called as a reader macro (since the real handling is in the lexer).
	return &Value{typ: VPrim, fn: func(args []*Value) (*Value, error) {
		return nil, fmt.Errorf("standard macro character %q cannot be invoked as a reader macro function", string(ch))
	}}, nil
}

func builtinMakeDispatchMacroCharacter(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("make-dispatch-macro-character: need a character")
	}
	var ch rune
	if args[0].typ == VChar {
		ch = args[0].ch
	} else {
		return nil, fmt.Errorf("make-dispatch-macro-character: first argument must be a character")
	}
	nonTerm := false
	if len(args) >= 2 {
		nonTerm = isTruthy(args[1])
	}
	rt := currentReadtable
	if len(args) >= 3 && isReadtable(args[2]) {
		rt = args[2].readtable
	}
	// Register the dispatch character itself as a macro
	rt.macroFns[ch] = &macroEntry{
		goFn:        nil, // dispatch handled in parser
		terminating: !nonTerm,
	}
	// Initialize dispatch table if needed
	if rt.dispFns == nil {
		rt.dispFns = make(map[rune]*macroEntry)
	}
	return vbool(true), nil
}

func builtinSetDispatchMacroCharacter(args []*Value) (*Value, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("set-dispatch-macro-character: need disp-char, sub-char, and function")
	}
	var dispCh rune
	if args[0].typ == VChar {
		dispCh = args[0].ch
	} else {
		return nil, fmt.Errorf("set-dispatch-macro-character: first argument must be a character")
	}
	var subCh rune
	if args[1].typ == VChar {
		subCh = args[1].ch
	} else {
		return nil, fmt.Errorf("set-dispatch-macro-character: second argument must be a character")
	}
	fn := args[2]
	if fn.typ != VFunc && fn.typ != VPrim {
		return nil, fmt.Errorf("set-dispatch-macro-character: third argument must be a function")
	}
	rt := currentReadtable
	if len(args) >= 4 && isReadtable(args[3]) {
		rt = args[3].readtable
	}
	_ = dispCh // dispCh validated above (ensures it's a character)
	if rt.dispFns == nil {
		rt.dispFns = make(map[rune]*macroEntry)
	}
	rt.dispFns[subCh] = &macroEntry{
		lispFn:      fn,
		terminating: false,
	}
	return vbool(true), nil
}

func builtinGetDispatchMacroCharacter(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("get-dispatch-macro-character: need disp-char and sub-char")
	}
	var dispCh rune
	if args[0].typ == VChar {
		dispCh = args[0].ch
	} else {
		return nil, fmt.Errorf("get-dispatch-macro-character: first argument must be a character")
	}
	var subCh rune
	if args[1].typ == VChar {
		subCh = args[1].ch
	} else {
		return nil, fmt.Errorf("get-dispatch-macro-character: second argument must be a character")
	}
	rt := currentReadtable
	if len(args) >= 3 && isReadtable(args[2]) {
		rt = args[2].readtable
	}
	_ = dispCh // dispCh validated above
	if rt.dispFns == nil {
		return vnil(), nil
	}
	entry, ok := rt.dispFns[subCh]
	if !ok || entry == nil {
		return vnil(), nil
	}
	if entry.lispFn != nil {
		return entry.lispFn, nil
	}
	return vnil(), nil
}
