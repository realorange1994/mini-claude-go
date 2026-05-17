package microlisp

import (
	"math/rand"
	"time"
)

func builtinCopyRandomState(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VRandomState {
		return builtinMakeRandomState(args)
	}
	v := gcv()
	v.typ = VRandomState
	v.randState = rand.New(rand.NewSource(args[0].randState.Int63()))
	return v, nil
}

func builtinRandomStateP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	return vbool(args[0].typ == VRandomState), nil
}

func builtinMakeRandomState(args []*Value) (*Value, error) {
	v := gcv()
	v.typ = VRandomState
	if len(args) > 0 && !isNil(args[0]) {
		// Copy from existing state
		if args[0].typ == VRandomState && args[0].randState != nil {
			v.randState = rand.New(rand.NewSource(args[0].randState.Int63()))
		} else {
			v.randState = rand.New(rand.NewSource(time.Now().UnixNano()))
		}
	} else {
		v.randState = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	return v, nil
}
