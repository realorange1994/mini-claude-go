package microlisp

import (
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
)

// -------- Go Channel Integration --------
// Provides Lisp-level access to Go channels for CSP-style concurrency.

// VChannel type for Lisp channels
const VChannel ValType = 50 // new type after VMethod (49)

// LispChannel wraps a Go channel with metadata
type LispChannel struct {
	ch   reflect.Value // the actual Go channel (reflect.Value for type flexibility)
	buf  int           // buffer size (0 = unbuffered)
	closed bool
}

var channelID int64
var channelMap = make(map[int64]*LispChannel)
var channelMapMu sync.Mutex

// builtinGoChannel creates a Go channel accessible from Lisp.
// (go:channel &optional buf-size) => channel object
func builtinGoChannel(args []*Value) (*Value, error) {
	bufSize := 0
	if len(args) >= 1 && args[0].typ == VNum {
		bufSize = int(args[0].num)
		if bufSize < 0 {
			return nil, fmt.Errorf("go:channel: buffer size must be non-negative, got %d", bufSize)
		}
	}

	// Create a channel of interface{} type
	chType := reflect.ChanOf(reflect.BothDir, reflect.TypeOf((*interface{})(nil)).Elem())
	chVal := reflect.MakeChan(chType, bufSize)

	lch := &LispChannel{
		ch:     chVal,
		buf:    bufSize,
		closed: false,
	}

	cid := atomic.AddInt64(&channelID, 1)
	channelMapMu.Lock()
	channelMap[cid] = lch
	channelMapMu.Unlock()

	v := gcv()
	v.typ = VChannel
	v.num = float64(cid)
	return v, nil
}

// builtinGoChannelSend sends a value to a channel.
// (go:send channel value) => nil
func builtinGoChannelSend(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("go:send: need channel and value")
	}
	if args[0].typ != VChannel {
		return nil, fmt.Errorf("go:send: first argument must be a channel, got %s", typeStr(args[0]))
	}

	cid := int64(args[0].num)
	channelMapMu.Lock()
	lch, ok := channelMap[cid]
	channelMapMu.Unlock()
	if !ok {
		return nil, fmt.Errorf("go:send: invalid channel")
	}
	if lch.closed {
		return nil, fmt.Errorf("go:send: channel is closed")
	}

	// Convert Lisp value to interface{} and send
	defer func() {
		recover() // ignore send panics (e.g. closed channel races)
	}()
	goVal := lispToInterface(args[1])
	lch.ch.Send(reflect.ValueOf(goVal))
	return vnil(), nil
}

// builtinGoChannelRecv receives a value from a channel.
// (go:recv channel) => value
func builtinGoChannelRecv(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("go:recv: need a channel")
	}
	if args[0].typ != VChannel {
		return nil, fmt.Errorf("go:recv: first argument must be a channel, got %s", typeStr(args[0]))
	}

	cid := int64(args[0].num)
	channelMapMu.Lock()
	lch, ok := channelMap[cid]
	channelMapMu.Unlock()
	if !ok {
		return nil, fmt.Errorf("go:recv: invalid channel")
	}

	var result reflect.Value
	var recvOk bool
	func() {
		defer func() { recover() }()
		result, recvOk = lch.ch.Recv()
	}()
	if !recvOk {
		// Channel closed
		return vnil(), nil
	}
	return interfaceToLisp(result.Interface()), nil
}

// builtinGoChannelClose closes a channel.
// (go:close channel) => nil
func builtinGoChannelClose(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("go:close: need a channel")
	}
	if args[0].typ != VChannel {
		return nil, fmt.Errorf("go:close: first argument must be a channel, got %s", typeStr(args[0]))
	}

	cid := int64(args[0].num)
	channelMapMu.Lock()
	lch, ok := channelMap[cid]
	channelMapMu.Unlock()
	if !ok {
		return nil, fmt.Errorf("go:close: invalid channel")
	}

	defer func() { recover() }()
	lch.ch.Close()
	lch.closed = true
	return vnil(), nil
}

// builtinGoChannelP checks if a value is a channel.
func builtinGoChannelP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	return vbool(args[0].typ == VChannel), nil
}

// builtinGoSelect performs a Go-style select on multiple channel operations.
// Returns (operation-index result) or nil if all channels closed
func builtinGoSelect(args []*Value) (*Value, error) {
	// Build select cases from arguments
	cases := make([]reflect.SelectCase, 0, len(args))

	for i, arg := range args {
		if arg.typ != VPair {
			return nil, fmt.Errorf("go:select: argument %d must be a list", i+1)
		}
		op := arg.car
		if op.typ != VSym {
			return nil, fmt.Errorf("go:select: argument %d must start with :send or :recv", i+1)
		}

		opStr := op.str
		rest := arg.cdr

		switch opStr {
		case ":SEND", ":send", "SEND", "send":
			if isNil(rest) || isNil(rest.cdr) {
				return nil, fmt.Errorf("go:select: :send needs channel and value")
			}
			chArg := rest.car
			valArg := rest.cdr.car
			if chArg.typ != VChannel {
				return nil, fmt.Errorf("go:select: :send channel must be a channel")
			}
			cid := int64(chArg.num)
			channelMapMu.Lock()
			lch, ok := channelMap[cid]
			channelMapMu.Unlock()
			if !ok {
				return nil, fmt.Errorf("go:select: invalid channel")
			}
			goVal := lispToInterface(valArg)
			cases = append(cases, reflect.SelectCase{
				Dir:  reflect.SelectSend,
				Chan: lch.ch,
				Send: reflect.ValueOf(goVal),
			})
		case ":RECV", ":recv", "RECV", "recv":
			if isNil(rest) {
				return nil, fmt.Errorf("go:select: :recv needs a channel")
			}
			chArg := rest.car
			if chArg.typ != VChannel {
				return nil, fmt.Errorf("go:select: :recv channel must be a channel")
			}
			cid := int64(chArg.num)
			channelMapMu.Lock()
			lch, ok := channelMap[cid]
			channelMapMu.Unlock()
			if !ok {
				return nil, fmt.Errorf("go:select: invalid channel")
			}
			cases = append(cases, reflect.SelectCase{
				Dir:  reflect.SelectRecv,
				Chan: lch.ch,
			})
		default:
			return nil, fmt.Errorf("go:select: unknown operation %s, use :send or :recv", opStr)
		}
	}

	if len(cases) == 0 {
		return nil, fmt.Errorf("go:select: need at least one operation")
	}

	// Use panic recovery for reflect.Select (can panic on invalid cases)
	defer func() { recover() }()
	chosen, value, ok := reflect.Select(cases)

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

// builtinGoSpawn launches a goroutine that evaluates a Lisp function.
// (go:spawn function &rest args) => thread-id (integer)
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
		threadEnv := copyGlobalEnv()
		argList := listFromSlice(fnArgs)
		result, err := Apply(fn, argList, threadEnv)
		resultCh <- threadResult{value: result, err: err}
	}()

	return vnum(float64(tid)), nil
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