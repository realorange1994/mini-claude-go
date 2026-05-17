package microlisp

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

var traceTable = make(map[string]bool)

var traceDepth = 0

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

func builtinSleep(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VNum {
		return nil, fmt.Errorf("sleep: need a number of seconds")
	}
	secs := args[0].num
	duration := time.Duration(secs * float64(time.Second))
	time.Sleep(duration)
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

func builtinMakeLock(args []*Value) (*Value, error) {
	lid := atomic.AddInt64(&nextLockID, 1)
	lockMapMu.Lock()
	lockMutexMap[lid] = &sync.Mutex{}
	lockMapMu.Unlock()
	return &Value{typ: VLock, num: float64(lid)}, nil
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

func copyGlobalEnv() *Env {
	env := NewEnv(nil)
	for k, v := range globalEnv.bindings {
		env.bindings[k] = v
	}
	return env
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

var nextLockID int64
var atomicCounter int64
var lockMutexMap = make(map[int64]*sync.Mutex)
var lockMapMu sync.Mutex
var condMu sync.Mutex
var condVars = make(map[int64]*sync.Cond)
var nextCondID int64
