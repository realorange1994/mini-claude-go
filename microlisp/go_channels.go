package microlisp

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// -------- Go Channel Integration --------
// Provides Lisp-level access to Go channels for CSP-style concurrency.
//
// Design principles:
//  1. All blocking operations integrate with activeLimits.CancelChan via
//     reflect.Select, so context cancellation breaks blocking immediately
//     and releases evalMu — preventing deadlocks for concurrent eval.
//  2. Non-blocking try variants let agents poll channels without holding evalMu.
//  3. Timeout support for blocking operations via go:select-with-timeout.
//  4. Idiomatic Lisp names (chan-send, chan-recv) alongside go: aliases.

// VChannel type for Lisp channels
const VChannel ValType = 50

// LispChannel wraps a Go channel with metadata
type LispChannel struct {
	ch     reflect.Value // the actual Go channel
	buf    int           // buffer size (0 = unbuffered)
	closed bool
}

var channelID int64
var channelMap = make(map[int64]*LispChannel)
var channelMapMu sync.Mutex

// getLispChannel safely retrieves a LispChannel by ID.
func getLispChannel(v *Value) (*LispChannel, error) {
	if v.typ != VChannel {
		return nil, fmt.Errorf("expected a channel, got %s", typeStr(v))
	}
	cid := int64(v.num)
	channelMapMu.Lock()
	lch, ok := channelMap[cid]
	channelMapMu.Unlock()
	if !ok {
		return nil, fmt.Errorf("invalid channel")
	}
	return lch, nil
}

// evalSelectOps iterates a linked-list of raw select operation lists,
// evaluates channel/value sub-expressions in each, and returns a slice
// of evaluated operation lists ready for makeSelectCases.
func evalSelectOps(rawOps *Value, env *Env) ([]*Value, error) {
	var ops []*Value
	for rawOps.typ == VPair && !isNil(rawOps) {
		op := rawOps.car
		// Each op should be a pair like (:send ch val) or (:recv ch) or (:default)
		if op.typ != VPair {
			return nil, fmt.Errorf("select op must be a list, got %s", typeStr(op))
		}
		keyword := op.car
		if keyword.typ != VSym {
			return nil, fmt.Errorf("select op must start with :send/:recv/:default")
		}
		rest := op.cdr

		switch strings.ToUpper(keyword.str) {
		case ":SEND":
			if isNil(rest) || isNil(rest.cdr) {
				return nil, fmt.Errorf(":send needs channel and value")
			}
			ch, e := Eval(rest.car, env)
			if e != nil {
				return nil, fmt.Errorf(":send channel eval: %v", e)
			}
			val, e := Eval(rest.cdr.car, env)
			if e != nil {
				return nil, fmt.Errorf(":send value eval: %v", e)
			}
			ops = append(ops, listFromSlice([]*Value{keyword, ch, val}))

		case ":RECV":
			if isNil(rest) {
				return nil, fmt.Errorf(":recv needs a channel")
			}
			ch, e := Eval(rest.car, env)
			if e != nil {
				return nil, fmt.Errorf(":recv channel eval: %v", e)
			}
			ops = append(ops, listFromSlice([]*Value{keyword, ch}))

		case ":DEFAULT":
			ops = append(ops, listFromSlice([]*Value{keyword}))

		default:
			return nil, fmt.Errorf("unknown select op %s", keyword.str)
		}
		rawOps = rawOps.cdr
	}
	return ops, nil
}

// makeSelectCases builds reflect.SelectCase slices from Lisp arguments.
// Each arg is (:send channel value) or (:recv channel).
// Returns the cases slice and a mapping back to Lisp args for result construction.
func makeSelectCases(ops []*Value) ([]reflect.SelectCase, error) {
	cases := make([]reflect.SelectCase, 0, len(ops))
	for i, arg := range ops {
		if arg.typ != VPair {
			return nil, fmt.Errorf("select op %d must be a list, got %s", i+1, typeStr(arg))
		}
		op := arg.car
		if op.typ != VSym {
			return nil, fmt.Errorf("select op %d must start with :send or :recv", i+1)
		}
		opStr := op.str
		rest := arg.cdr

		switch opStr {
		case ":SEND", ":send", "SEND", "send":
			if isNil(rest) || isNil(rest.cdr) {
				return nil, fmt.Errorf(":send needs channel and value")
			}
			lch, err := getLispChannel(rest.car)
			if err != nil {
				return nil, fmt.Errorf("select :send: %v", err)
			}
			if lch.closed {
				return nil, fmt.Errorf("select :send: channel is closed")
			}
			goVal := lispToInterface(rest.cdr.car)
			cases = append(cases, reflect.SelectCase{
				Dir:  reflect.SelectSend,
				Chan: lch.ch,
				Send: reflect.ValueOf(goVal),
			})
		case ":RECV", ":recv", "RECV", "recv":
			if isNil(rest) {
				return nil, fmt.Errorf(":recv needs a channel")
			}
			lch, err := getLispChannel(rest.car)
			if err != nil {
				return nil, fmt.Errorf("select :recv: %v", err)
			}
			cases = append(cases, reflect.SelectCase{
				Dir:  reflect.SelectRecv,
				Chan: lch.ch,
			})
		case ":DEFAULT", ":default", "DEFAULT", "default":
			cases = append(cases, reflect.SelectCase{
				Dir:  reflect.SelectDefault,
				Chan: reflect.Value{},
				Send: reflect.Value{},
			})
		default:
			return nil, fmt.Errorf("unknown select op %s, use :send/:recv/:default", opStr)
		}
	}
	return cases, nil
}

// runSelectWithCancel performs reflect.Select with CancelChan integration.
// If activeLimits.CancelChan is set, it adds a cancel case to break blocking.
// Returns (chosenIndex, recvValue, recvOk, wasCancelled, error).
func runSelectWithCancel(cases []reflect.SelectCase) (chosen int, value reflect.Value, ok bool, cancelled bool, err error) {
	// Check if we have an active cancel channel to integrate
	cancelIdx := -1
	if limitsActive && activeLimits.CancelChan != nil {
		cancelCase := reflect.SelectCase{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(activeLimits.CancelChan),
		}
		cancelIdx = len(cases)
		cases = append(cases, cancelCase)
	}

	defer func() { recover() }()
	chosen, value, ok = reflect.Select(cases)

	// Check if the cancel case won
	if chosen == cancelIdx {
		return 0, reflect.Value{}, false, true, fmt.Errorf("operation cancelled")
	}
	return chosen, value, ok, false, nil
}

// -------- Channel creation --------

// make-channel: (make-channel &optional buf-size) => channel
func builtinMakeChannel(args []*Value) (*Value, error) {
	bufSize := 0
	if len(args) >= 1 && args[0].typ == VNum {
		bufSize = int(args[0].num)
		if bufSize < 0 {
			return nil, fmt.Errorf("make-channel: buffer size must be non-negative, got %d", bufSize)
		}
	}

	chType := reflect.ChanOf(reflect.BothDir, reflect.TypeOf((*interface{})(nil)).Elem())
	chVal := reflect.MakeChan(chType, bufSize)

	lch := &LispChannel{ch: chVal, buf: bufSize, closed: false}

	cid := atomic.AddInt64(&channelID, 1)
	channelMapMu.Lock()
	channelMap[cid] = lch
	channelMapMu.Unlock()

	v := gcv()
	v.typ = VChannel
	v.num = float64(cid)
	return v, nil
}

// -------- Blocking send (with CancelChan integration) --------

// chan-send: (chan-send channel value) => nil
// Blocks until the send succeeds, the channel is closed, or CancelChan fires.
func builtinChanSend(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("chan-send: needs channel and value")
	}
	lch, err := getLispChannel(args[0])
	if err != nil {
		return nil, fmt.Errorf("chan-send: %v", err)
	}
	if lch.closed {
		return nil, fmt.Errorf("chan-send: channel is closed")
	}

	goVal := lispToInterface(args[1])
	cases := []reflect.SelectCase{
		{Dir: reflect.SelectSend, Chan: lch.ch, Send: reflect.ValueOf(goVal)},
	}
	_, _, _, cancelled, err := runSelectWithCancel(cases)
	if cancelled {
		return nil, err
	}
	return vnil(), nil
}

// -------- Blocking recv (with CancelChan integration) --------

// chan-recv: (chan-recv channel) => value
// Blocks until a value is received, the channel is closed, or CancelChan fires.
func builtinChanRecv(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("chan-recv: needs a channel")
	}
	lch, err := getLispChannel(args[0])
	if err != nil {
		return nil, fmt.Errorf("chan-recv: %v", err)
	}

	cases := []reflect.SelectCase{
		{Dir: reflect.SelectRecv, Chan: lch.ch},
	}
	_, value, ok, cancelled, err := runSelectWithCancel(cases)
	if cancelled {
		return nil, err
	}
	if !ok {
		return vnil(), nil // channel closed
	}
	return interfaceToLisp(value.Interface()), nil
}

// -------- Non-blocking try-send --------

// chan-try-send: (chan-try-send channel value) => (:ok) or (:would-block) or (:closed)
func builtinChanTrySend(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("chan-try-send: needs channel and value")
	}
	lch, err := getLispChannel(args[0])
	if err != nil {
		return nil, fmt.Errorf("chan-try-send: %v", err)
	}
	if lch.closed {
		return listFromSlice([]*Value{vsym("closed")}), nil
	}

	goVal := lispToInterface(args[1])
	cases := []reflect.SelectCase{
		{Dir: reflect.SelectSend, Chan: lch.ch, Send: reflect.ValueOf(goVal)},
		{Dir: reflect.SelectDefault},
	}
	defer func() { recover() }()
	chosen, _, _ := reflect.Select(cases)
	if chosen == 0 {
		return listFromSlice([]*Value{vsym("ok")}), nil
	}
	return listFromSlice([]*Value{vsym("would-block")}), nil
}

// -------- Non-blocking try-recv --------

// chan-try-recv: (chan-try-recv channel) => (:ok value) or (:would-block) or (:closed)
func builtinChanTryRecv(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("chan-try-recv: needs a channel")
	}
	lch, err := getLispChannel(args[0])
	if err != nil {
		return nil, fmt.Errorf("chan-try-recv: %v", err)
	}

	cases := []reflect.SelectCase{
		{Dir: reflect.SelectRecv, Chan: lch.ch},
		{Dir: reflect.SelectDefault},
	}
	defer func() { recover() }()
	chosen, value, ok := reflect.Select(cases)
	if chosen == 0 {
		if !ok {
			return listFromSlice([]*Value{vsym("closed")}), nil
		}
		return listFromSlice([]*Value{vsym("ok"), interfaceToLisp(value.Interface())}), nil
	}
	return listFromSlice([]*Value{vsym("would-block")}), nil
}

// -------- Select with timeout --------

// chan-select-timeout: (chan-select-timeout timeout-ms op1 op2 ...) => (index result) or (:timeout)
// Like go:select but with a millisecond timeout case. Returns (:timeout) if no case fires within timeout.
func builtinChanSelectTimeout(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("chan-select-timeout: needs timeout-ms and at least one operation")
	}
	timeoutMs := args[0].num
	if timeoutMs <= 0 {
		return nil, fmt.Errorf("chan-select-timeout: timeout must be positive")
	}

	ops := args[1:]
	cases, err := makeSelectCases(ops)
	if err != nil {
		return nil, fmt.Errorf("chan-select-timeout: %v", err)
	}

	// Add timeout case
	timeoutCh := make(chan time.Time, 1)
	go func() {
		time.Sleep(time.Duration(timeoutMs) * time.Millisecond)
		select {
		case timeoutCh <- time.Now():
		default:
		}
	}()
	cases = append(cases, reflect.SelectCase{
		Dir:  reflect.SelectRecv,
		Chan: reflect.ValueOf(timeoutCh),
	})
	timeoutIdx := len(cases) - 1

	// Also add cancel case if active
	cancelIdx := -1
	if limitsActive && activeLimits.CancelChan != nil {
		cancelIdx = len(cases)
		cancelCase := reflect.SelectCase{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(activeLimits.CancelChan),
		}
		cases = append(cases, cancelCase)
	}

	defer func() { recover() }()
	chosen, value, ok := reflect.Select(cases)

	if chosen == cancelIdx {
		return nil, fmt.Errorf("operation cancelled")
	}
	if chosen == timeoutIdx {
		return listFromSlice([]*Value{vsym("timeout")}), nil
	}

	// Map chosen index back (we added timeout/cancel at the end)
	if !ok && cases[chosen].Dir == reflect.SelectRecv && chosen != timeoutIdx {
		return listFromSlice([]*Value{vnum(float64(chosen)), vnil()}), nil
	}

	var result *Value
	if cases[chosen].Dir == reflect.SelectRecv && chosen != timeoutIdx {
		result = interfaceToLisp(value.Interface())
	} else {
		result = vnil()
	}
	return listFromSlice([]*Value{vnum(float64(chosen)), result}), nil
}

// -------- Select (updated with CancelChan integration and :default support) --------

// go:select: (go:select op1 op2 ...) => (operation-index result)
// Each op is (:send channel value), (:recv channel), or (:default).
func builtinGoSelect(args []*Value) (*Value, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("go:select: need at least one operation")
	}

	cases, err := makeSelectCases(args)
	if err != nil {
		return nil, fmt.Errorf("go:select: %v", err)
	}

	chosen, value, ok, cancelled, err := runSelectWithCancel(cases)
	if cancelled {
		return nil, err
	}

	if !ok && cases[chosen].Dir == reflect.SelectRecv {
		return listFromSlice([]*Value{vnum(float64(chosen)), vnil()}), nil
	}

	var result *Value
	if cases[chosen].Dir == reflect.SelectRecv {
		result = interfaceToLisp(value.Interface())
	} else {
		result = vnil()
	}
	return listFromSlice([]*Value{vnum(float64(chosen)), result}), nil
}

// -------- Close --------

// chan-close: (chan-close channel) => nil
func builtinChanClose(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("chan-close: needs a channel")
	}
	lch, err := getLispChannel(args[0])
	if err != nil {
		return nil, fmt.Errorf("chan-close: %v", err)
	}

	defer func() { recover() }()
	lch.ch.Close()
	lch.closed = true
	return vnil(), nil
}

// -------- Predicate --------

// chan-p: (chan-p value) => boolean
func builtinChanP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	return vbool(args[0].typ == VChannel), nil
}

// -------- Channel info --------

// chan-info: (chan-info channel) => (:buffer N :closed T/NIL)
func builtinChanInfo(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("chan-info: needs a channel")
	}
	lch, err := getLispChannel(args[0])
	if err != nil {
		return nil, fmt.Errorf("chan-info: %v", err)
	}
	info := cons(cons(vsym("closed"), vbool(lch.closed)),
		cons(cons(vsym("buffer"), vnum(float64(lch.buf))), vnil()))
	return info, nil
}

// -------- Spawn (unchanged) --------

// go:spawn: (go:spawn function &rest args) => thread-id
func builtinGoSpawn(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("go:spawn: need a function")
	}
	fn := args[0]
	fnArgs := args[1:]

	tid := atomic.AddInt64(&nextThreadID, 1)
	resultCh := make(chan threadResult, 1)

	threadChannelsMu.Lock()
	threadChannels[tid] = resultCh
	threadChannelsMu.Unlock()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				resultCh <- threadResult{err: fmt.Errorf("panic in spawned thread: %v", r)}
			}
		}()
		threadEnv := copyGlobalEnv()
		argList := listFromSlice(fnArgs)
		result, err := Apply(fn, argList, threadEnv)
		resultCh <- threadResult{value: result, err: err}
	}()

	return &Value{typ: VThread, num: float64(tid)}, nil
}

// interfaceToLisp converts a Go interface{} value back to a Lisp Value.
func interfaceToLisp(v interface{}) *Value {
	if v == nil {
		return vnil()
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Float64:
		return vfloat(rv.Float())
	case reflect.Float32:
		return vfloat(float64(rv.Float()))
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return vnum(float64(rv.Int()))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return vnum(float64(rv.Uint()))
	case reflect.Bool:
		return vbool(rv.Bool())
	case reflect.String:
		return vstr(rv.String())
	case reflect.Slice:
		if rv.Type().Elem().Kind() == reflect.Uint8 {
			return vstr(string(rv.Bytes()))
		}
		// Convert slice to Lisp list
		result := make([]*Value, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			result[i] = interfaceToLisp(rv.Index(i).Interface())
		}
		return listFromSlice(result)
	case reflect.Map:
		// Convert map to Lisp alist
		var alist *Value
		iter := rv.MapRange()
		for iter.Next() {
			key := interfaceToLisp(iter.Key().Interface())
			val := interfaceToLisp(iter.Value().Interface())
			alist = cons(cons(key, val), alist)
		}
		return alist
	case reflect.Complex64, reflect.Complex128:
		c := rv.Complex()
		return &Value{typ: VComplex, num: real(c), imag: imag(c)}
	default:
		return vstr(fmt.Sprintf("%v", v))
	}
}
