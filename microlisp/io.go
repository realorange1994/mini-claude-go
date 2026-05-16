package microlisp

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// -------- File loading --------
func LoadFile(fname string, env *Env) (*Value, error) {
	data, err := os.ReadFile(fname)
	if err != nil {
		return nil, fmt.Errorf("load: %v", err)
	}
	// Save old values of *load-pathname* and *load-truename*
	oldPathname, _ := env.Get("*load-pathname*")
	oldTruename, _ := env.Get("*load-truename*")
	// Set *load-pathname* to the pathname of the file being loaded
	absPath, _ := filepath.Abs(fname)
	env.Set("*load-pathname*", vpathname(parsePathnameString(absPath)))
	// Set *load-truename* to the truename (resolved absolute path)
	env.Set("*load-truename*", vpathname(parsePathnameString(absPath)))
	// Evaluate the file contents
	result, evalErr := EvalString(string(data), env)
	// Restore old values
	if oldPathname != nil {
		env.Set("*load-pathname*", oldPathname)
	} else {
		env.Set("*load-pathname*", vnil())
	}
	if oldTruename != nil {
		env.Set("*load-truename*", oldTruename)
	} else {
		env.Set("*load-truename*", vnil())
	}
	return result, evalErr
}

func builtinMakeThread(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("make-thread: need a function")
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

	return &Value{typ: VThread, num: float64(tid)}, nil
}

func copyGlobalEnv() *Env {
	env := NewEnv(nil)
	for k, v := range globalEnv.bindings {
		env.bindings[k] = v
	}
	return env
}

func builtinJoinThread(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VThread {
		return nil, fmt.Errorf("join-thread: need a thread")
	}
	tid := int64(args[0].num)
	threadChannelsMu.Lock()
	ch, ok := threadChannels[tid]
	threadChannelsMu.Unlock()
	if !ok {
		return nil, fmt.Errorf("join-thread: no such thread %d", tid)
	}
	tr := <-ch
	if tr.err != nil {
		return nil, tr.err
	}
	threadChannelsMu.Lock()
	delete(threadChannels, tid)
	threadChannelsMu.Unlock()
	return tr.value, nil
}

var nextLockID int64
var atomicCounter int64
var lockMutexMap = make(map[int64]*sync.Mutex)
var lockMapMu sync.Mutex
var condMu sync.Mutex
var condVars = make(map[int64]*sync.Cond)
var nextCondID int64

func builtinMakeLock(args []*Value) (*Value, error) {
	lid := atomic.AddInt64(&nextLockID, 1)
	lockMapMu.Lock()
	lockMutexMap[lid] = &sync.Mutex{}
	lockMapMu.Unlock()
	return &Value{typ: VLock, num: float64(lid)}, nil
}

func builtinLock(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VLock {
		return nil, fmt.Errorf("lock: need a lock object")
	}
	lid := int64(args[0].num)
	lockMapMu.Lock()
	mu, ok := lockMutexMap[lid]
	lockMapMu.Unlock()
	if !ok {
		return nil, fmt.Errorf("lock: invalid lock")
	}
	mu.Lock()
	return vnil(), nil
}

func builtinUnlock(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VLock {
		return nil, fmt.Errorf("unlock: need a lock object")
	}
	lid := int64(args[0].num)
	lockMapMu.Lock()
	mu, ok := lockMutexMap[lid]
	lockMapMu.Unlock()
	if !ok {
		return nil, fmt.Errorf("unlock: invalid lock")
	}
	mu.Unlock()
	return vnil(), nil
}

func builtinSleep(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VNum {
		return nil, fmt.Errorf("sleep: need a number of seconds")
	}
	secs := args[0].num
	duration := time.Duration(secs * float64(time.Second))
	time.Sleep(duration)
	return vnil(), nil
}

func builtinValues(args []*Value) (*Value, error) {
	// values returns a VMultiVal wrapping all arguments.
	// Primary value (car) is the first argument, or nil if none.
	v := gcv()
	v.typ = VMultiVal
	v.cdr = vnil()
	if len(args) > 0 {
		v.car = args[0]
		v.cdr = list(args[1:]...)
	}
	return v, nil
}

func builtinValuesList(args []*Value) (*Value, error) {
	// values-list: converts a list to multiple values.
	// (values-list '(a b c)) => values a b c
	if len(args) != 1 {
		return nil, fmt.Errorf("values-list: need exactly 1 argument")
	}
	lst := args[0]
	if isNil(lst) {
		v := gcv()
		v.typ = VMultiVal
		v.car = vnil()
		v.cdr = vnil()
		return v, nil
	}
	v := gcv()
	v.typ = VMultiVal
	v.car = lst.car
	v.cdr = lst.cdr
	return v, nil
}
