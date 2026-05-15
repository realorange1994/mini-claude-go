package microlisp

import "sync"

// evalMu protects all microlisp evaluation from concurrent access.
// The interpreter has extensive global mutable state (globalEnv, evalDepth,
// handlerStack, etc.) and is NOT safe for concurrent use.
var evalMu sync.Mutex

// SafeEvalString evaluates a Lisp expression with thread safety.
// InitGlobalEnv() must be called once before first use.
func SafeEvalString(s string) (string, error) {
	evalMu.Lock()
	defer evalMu.Unlock()
	result, err := EvalString(s, globalEnv)
	if err != nil {
		return "", err
	}
	return ToString(result), nil
}

// ResetGlobalEnv reinitializes the interpreter to its clean state.
// This clears all user-defined variables, functions, macros, and classes,
// and restores the standard library and built-in functions.
func ResetGlobalEnv() {
	evalMu.Lock()
	defer evalMu.Unlock()
	// Reset all mutable global state. InitGlobalEnv will recreate
	// maps that it initializes (lispFeatures, lispModules, packages).
	globalEnv = NewEnv(nil)
	evalDepth = 0
	handlerStack = nil
	restartStack = nil
	innermostRestartFrame = -1
	classRegistry = make(map[string]*Value)
	compilerMacros = make(map[string]*Value)
	structPrintFns = make(map[string]*Value)
	symbolTable = make(map[string]*Value)
	docstrings = make(map[string]string)
	traceTable = make(map[string]bool)
	traceDepth = 0
	gensymCounter = 0
	nextThreadID = 0
	threadChannels = make(map[int64]chan threadResult)
	lockMutexMap = make(map[int64]*sync.Mutex)
	condVars = make(map[int64]*sync.Cond)
	logicalPathnameTranslations = map[string]*Value{}
	// These are set by InitGlobalEnv sub-calls (initFeatures/initPackages),
	// so we clear them to empty; InitGlobalEnv will repopulate.
	lispFeatures = make(map[string]bool)
	lispModules = make(map[string]bool)
	// packages is map[string]*Package — clear via reflection-free approach:
	// initPackages creates fresh packages, so just empty the map.
	for k := range packages {
		delete(packages, k)
	}
	// Re-initialize the global environment with builtins and stdlib
	InitGlobalEnv()
}
