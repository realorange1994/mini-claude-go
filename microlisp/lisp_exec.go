//go:build unix || windows

package microlisp

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// lispExec is the core pure-Go exec function for Lisp.
//
// Signature (from Lisp perspective):
//   (exec command &key args working-dir env timeout max-memory-mb max-cpu-ms)
//
// Returns a plist:
//   (:stdout "..." :stderr "..." :exit-code 0)
func builtinLispExec(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VStr {
		return nil, fmt.Errorf("exec: first argument must be the command string")
	}
	command := args[0].str

	var cmdArgs []string
	var workingDir string
	var env []string
	var timeoutMs int64 = 120000
	var maxMemoryMB int64
	var maxCPUMS int64

	for i := 1; i < len(args); {
		if i+1 >= len(args) {
			break
		}
		key := args[i]
		val := args[i+1]

		if key.typ != VSym {
			i += 2
			continue
		}

		keyStr := strings.ToLower(key.str)
		switch keyStr {
		case ":args":
			if val.typ == VPair || val.typ == VNil {
				cmdArgs = lispListToStringSlice(val)
			}
		case ":working-dir":
			if val.typ == VStr {
				workingDir = val.str
			}
		case ":env":
			if val.typ == VPair || val.typ == VNil {
				env = lispEnvToGoSlice(val)
			}
		case ":timeout":
			if val.typ == VNum {
				timeoutMs = int64(val.num)
			}
		case ":max-memory-mb":
			if val.typ == VNum {
				maxMemoryMB = int64(val.num)
			}
		case ":max-cpu-ms":
			if val.typ == VNum {
				maxCPUMS = int64(val.num)
			}
		}
		i += 2
	}

	if timeoutMs <= 0 {
		timeoutMs = 120000
	}
	if timeoutMs > 600000 {
		timeoutMs = 600000
	}

	return executeCommand(command, cmdArgs, workingDir, env, time.Duration(timeoutMs)*time.Millisecond, maxMemoryMB, maxCPUMS)
}

// lispListToStringSlice converts a Lisp list to a []string.
func lispListToStringSlice(v *Value) []string {
	var result []string
	for v.typ == VPair {
		if v.car.typ == VStr {
			result = append(result, v.car.str)
		}
		v = v.cdr
	}
	return result
}

// lispEnvToGoSlice converts a Lisp alist ((KEY . VAL) ...) to a Go []string.
func lispEnvToGoSlice(v *Value) []string {
	var result []string
	for v.typ == VPair {
		if v.car.typ == VPair && v.car.car.typ == VStr && v.car.cdr.typ == VStr {
			result = append(result, v.car.car.str+"="+v.car.cdr.str)
		}
		v = v.cdr
	}
	return result
}

// executeCommand runs a command with the given parameters.
func executeCommand(command string, cmdArgs []string, workingDir string, env []string, timeout time.Duration, maxMemoryMB int64, maxCPUMS int64) (*Value, error) {
	cmd := exec.Command(command, cmdArgs...)

	if workingDir != "" {
		cmd.Dir = workingDir
	}

	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}

	cmd.Stdin = nil // Disconnect stdin — interactive commands will stall

	cmd.SysProcAttr = setupExecProcessGroupAttr()

	// Create stdout/stderr pipes BEFORE Start().
	// Per Go docs: StdoutPipe/StderrPipe must be called before Start.
	// If either fails, fall back to direct buffer attachment (no stall detection).
	stdoutPipe, stdoutPipeErr := cmd.StdoutPipe()
	stderrPipe, stderrPipeErr := cmd.StderrPipe()
	usePipes := stdoutPipeErr == nil && stderrPipeErr == nil

	var stdoutBuf, stderrBuf bytes.Buffer

	if !usePipes {
		// Fallback: direct buffer attachment (no stall detection).
		cmd.Stdout = &stdoutBuf
		cmd.Stderr = &stderrBuf
	}

	// Apply resource limits. wrapWithResourceLimits returns a restore
	// function on platforms that support it (Unix setrlimit); nil otherwise.
	// We call restore() immediately after Start() — the child already
	// inherited the limits at fork time.
	restoreLimits := wrapWithResourceLimits(cmd, maxMemoryMB, maxCPUMS)

	if err := cmd.Start(); err != nil {
		if restoreLimits != nil {
			restoreLimits()
		}
		return makeExecResult("", err.Error(), 1), nil
	}

	// Restore parent's original rlimits now that the child has forked.
	if restoreLimits != nil {
		restoreLimits()
	}

	if runtime.GOOS == "windows" && (maxMemoryMB > 0 || maxCPUMS > 0) {
		_ = setupWindowsJobObject(cmd, maxMemoryMB, maxCPUMS)
	}

	if usePipes {
		// Pipe-based reading with stall detection.
		// When stdin is nil and a command waits for input (e.g. sudo password),
		// output stalls. We detect this and move the process to background.
		go func() {
			io.Copy(&stdoutBuf, stdoutPipe)
		}()
		go func() {
			io.Copy(&stderrBuf, stderrPipe)
		}()

		// Stall detection: if the process produces no output after 15 seconds
		// and is still running, it's likely waiting for interactive input.
		// Instead of killing, we move it to background and return early.
		stallTimer := time.NewTimer(15 * time.Second)
		defer stallTimer.Stop()

		done := make(chan error, 1)
		go func() {
			done <- cmd.Wait()
		}()

		select {
		case err := <-done:
			// Process completed normally
			stallTimer.Stop()
			exitCode := 0
			if err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					exitCode = exitErr.ExitCode()
				} else {
					exitCode = 1
				}
			}
			time.Sleep(50 * time.Millisecond) // let pipe readers finish
			return makeExecResult(stdoutBuf.String(), stderrBuf.String(), exitCode), nil

		case <-stallTimer.C:
			// 15s with no completion — check if process has produced any output.
			pid := cmd.Process.Pid
			if stdoutBuf.Len() == 0 && stderrBuf.Len() == 0 {
				// No output at all — likely waiting for interactive input.
				// Move to background instead of killing. The done goroutine
				// will reap the zombie when the process eventually exits.
				reason := fmt.Sprintf(
					"Command produced no output for 15s — likely waiting for interactive input. "+
						"Process moved to background (PID %d). "+
						"Try non-interactive flags: -y, --yes, --force, or pipe input via exec-with-input.",
					pid)
				return makeBackgroundExecResult("", "", pid, reason), nil
			}
			// Some output was produced — reset stall timer and wait for timeout
			stallTimer.Reset(timeout)
			select {
			case err := <-done:
				stallTimer.Stop()
				exitCode := 0
				if err != nil {
					if exitErr, ok := err.(*exec.ExitError); ok {
						exitCode = exitErr.ExitCode()
					} else {
						exitCode = 1
					}
				}
				time.Sleep(50 * time.Millisecond)
				return makeExecResult(stdoutBuf.String(), stderrBuf.String(), exitCode), nil
			case <-stallTimer.C:
				// Timeout with some output — move to background
				stdout := stdoutBuf.String()
				stderr := stderrBuf.String()
				reason := fmt.Sprintf(
					"Command timed out after %s but produced output — moved to background (PID %d). "+
						"It may still be running.",
					timeout, pid)
				return makeBackgroundExecResult(stdout, stderr, pid, reason), nil
			}

		case <-time.After(timeout):
			// Hard timeout — move to background instead of killing
			pid := cmd.Process.Pid
			stdout := stdoutBuf.String()
			stderr := stderrBuf.String()
			reason := fmt.Sprintf(
				"Command timed out after %s — moved to background (PID %d). "+
					"It may still be running.",
				timeout, pid)
			return makeBackgroundExecResult(stdout, stderr, pid, reason), nil
		}
	}

	// Fallback: direct buffer attachment (no stall detection).
	// Used when StdoutPipe/StderrPipe failed.
	err := cmd.Wait()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}
	return makeExecResult(stdoutBuf.String(), stderrBuf.String(), exitCode), nil
}

// makeExecResult creates a Lisp plist: (:stdout "..." :stderr "..." :exit-code N)
func makeExecResult(stdout, stderr string, exitCode int) *Value {
	return listToPlist(
		vsym(":stdout"), StringValue(stdout),
		vsym(":stderr"), StringValue(stderr),
		vsym(":exit-code"), vnum(float64(exitCode)),
	)
}

// makeBackgroundExecResult creates a plist with :background t and :stall-reason.
// :exit-code is -1 to signal the process is still running.
func makeBackgroundExecResult(stdout, stderr string, pid int, reason string) *Value {
	return listToPlist(
		vsym(":stdout"), StringValue(stdout),
		vsym(":stderr"), StringValue(stderr),
		vsym(":exit-code"), vnum(-1),
		vsym(":background"), vnum(1),
		vsym(":stall-reason"), StringValue(reason),
	)
}

// StringValue creates a VStr Value.
func StringValue(s string) *Value {
	return vstr(s)
}

// listToPlist builds a proper plist from alternating key-value pairs.
func listToPlist(pairs ...*Value) *Value {
	result := vnil()
	for i := len(pairs) - 1; i >= 0; i -= 2 {
		result = cons(pairs[i-1], cons(pairs[i], result))
	}
	return result
}

// ─── exec-simple ─────────────────────────────────────────────────────────────

// builtinLispExecSimple: (exec-simple command arg1 arg2 ...)
// Returns: (output . exit-code) as a dotted pair
func builtinLispExecSimple(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VStr {
		return nil, fmt.Errorf("exec-simple: first argument must be the command string")
	}
	command := args[0].str
	var cmdArgs []string
	for i := 1; i < len(args); i++ {
		if args[i].typ == VStr {
			cmdArgs = append(cmdArgs, args[i].str)
		} else {
			cmdArgs = append(cmdArgs, valueToString(args[i]))
		}
	}
	return runExecSimple(command, cmdArgs)
}

func runExecSimple(command string, cmdArgs []string) (*Value, error) {
	cmd := exec.Command(command, cmdArgs...)
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	cmd.Stdin = nil

	cmd.SysProcAttr = setupExecProcessGroupAttr()

	if err := cmd.Start(); err != nil {
		return makeSimpleResult("", err.Error(), 1), nil
	}

	err := cmd.Wait()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}

	return makeSimpleResult(stdoutBuf.String(), stderrBuf.String(), exitCode), nil
}

func makeSimpleResult(stdout, stderr string, exitCode int) *Value {
	output := stdout
	if stderr != "" {
		if output != "" {
			output += "\nSTDERR:\n" + stderr
		} else {
			output = "STDERR:\n" + stderr
		}
	}
	return cons(StringValue(output), cons(vnum(float64(exitCode)), vnil()))
}

// valueToString converts a Lisp Value to a Go string.
func valueToString(v *Value) string {
	switch v.typ {
	case VStr:
		return v.str
	case VNum:
		if v.isFloat {
			return strconv.FormatFloat(v.num, 'f', -1, 64)
		}
		return strconv.FormatInt(int64(v.num), 10)
	case VBool:
		if v == globalEnv.bindings["#t"] {
			return "t"
		}
		return "nil"
	case VNil:
		return "nil"
	default:
		return fmt.Sprintf("%v", v)
	}
}

// ─── which ───────────────────────────────────────────────────────────────────

// builtinLispWhich: (which command) -> path or nil
func builtinLispWhich(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VStr {
		return nil, fmt.Errorf("which: first argument must be the command string")
	}
	command := args[0].str
	path, err := exec.LookPath(command)
	if err != nil {
		return vnil(), nil
	}
	return vstr(path), nil
}

// ─── exec-with-input ─────────────────────────────────────────────────────────

// builtinLispExecWithInput: (exec-with-input command input &key args working-dir)
// Returns: (:stdout "..." :stderr "..." :exit-code N)
func builtinLispExecWithInput(args []*Value) (*Value, error) {
	if len(args) < 2 || args[0].typ != VStr {
		return nil, fmt.Errorf("exec-with-input: requires command and input arguments")
	}
	command := args[0].str
	input := valueToString(args[1])

	var cmdArgs []string
	var workingDir string

	for i := 2; i < len(args); {
		if i+1 >= len(args) {
			break
		}
		key := args[i]
		val := args[i+1]

		if key.typ != VSym {
			i += 2
			continue
		}

		keyStr := strings.ToLower(key.str)
		switch keyStr {
		case ":args":
			if val.typ == VPair || val.typ == VNil {
				cmdArgs = lispListToStringSlice(val)
			}
		case ":working-dir":
			if val.typ == VStr {
				workingDir = val.str
			}
		}
		i += 2
	}

	cmd := exec.Command(command, cmdArgs...)
	cmd.Stdin = strings.NewReader(input)
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	if workingDir != "" {
		cmd.Dir = workingDir
	}

	cmd.SysProcAttr = setupExecProcessGroupAttr()

	if err := cmd.Start(); err != nil {
		return makeExecResult("", err.Error(), 1), nil
	}

	err := cmd.Wait()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}

	return makeExecResult(stdoutBuf.String(), stderrBuf.String(), exitCode), nil
}

// ─── exec-pipe (streaming) ───────────────────────────────────────────────────

// builtinLispExecPipe: (exec-pipe command &key args)
// Returns: a pipe object (VGoVal)
func builtinLispExecPipe(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VStr {
		return nil, fmt.Errorf("exec-pipe: first argument must be the command string")
	}
	command := args[0].str
	var cmdArgs []string

	for i := 1; i < len(args); {
		if i+1 >= len(args) {
			break
		}
		key := args[i]
		val := args[i+1]

		if key.typ != VSym {
			i += 2
			continue
		}

		keyStr := strings.ToLower(key.str)
		switch keyStr {
		case ":args":
			if val.typ == VPair || val.typ == VNil {
				cmdArgs = lispListToStringSlice(val)
			}
		}
		i += 2
	}

	cmd := exec.Command(command, cmdArgs...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("exec-pipe: %v", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("exec-pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("exec-pipe: %v", err)
	}

	pipe := &execPipeState{cmd: cmd, stdout: stdout, stderr: stderr}
	return &Value{typ: VGoVal, goVal: pipe}, nil
}

// execPipeState holds the state of a streaming exec pipe.
type execPipeState struct {
	cmd    *exec.Cmd
	stdout io.ReadCloser
	stderr io.ReadCloser
	mu     sync.Mutex
}

// builtinExecPipeRead: (exec-read-line pipe &key stream)
func builtinExecPipeRead(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("exec-read-line: requires a pipe argument")
	}

	pipeVal := args[0]
	if pipeVal.typ != VGoVal {
		return nil, fmt.Errorf("exec-read-line: argument must be an exec pipe")
	}

	pipe, ok := pipeVal.goVal.(*execPipeState)
	if !ok {
		return nil, fmt.Errorf("exec-read-line: invalid pipe object")
	}

	stream := ":stdout"
	for i := 1; i < len(args); {
		if i+1 >= len(args) {
			break
		}
		key := args[i]
		val := args[i+1]
		if key.typ == VSym && strings.ToLower(key.str) == ":stream" && val.typ == VStr {
			stream = val.str
		}
		i += 2
	}

	pipe.mu.Lock()
	defer pipe.mu.Unlock()

	var r io.Reader
	switch stream {
	case ":stderr":
		r = pipe.stderr
	default:
		r = pipe.stdout
	}

	buf := make([]byte, 4096)
	n, err := r.Read(buf)
	if err != nil {
		if err == io.EOF {
			return vnil(), nil
		}
		return nil, fmt.Errorf("exec-read-line: %v", err)
	}

	return vstr(string(buf[:n])), nil
}

// builtinExecPipeWait: (exec-wait pipe) -> exit-code
func builtinExecPipeWait(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("exec-wait: requires a pipe argument")
	}

	pipeVal := args[0]
	if pipeVal.typ != VGoVal {
		return nil, fmt.Errorf("exec-wait: argument must be an exec pipe")
	}

	pipe, ok := pipeVal.goVal.(*execPipeState)
	if !ok {
		return nil, fmt.Errorf("exec-wait: invalid pipe object")
	}

	err := pipe.cmd.Wait()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}

	pipe.stdout.Close()
	pipe.stderr.Close()

	return vnum(float64(exitCode)), nil
}

// builtinExecPipeKill: (exec-kill pipe) -> t
func builtinExecPipeKill(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("exec-kill: requires a pipe argument")
	}

	pipeVal := args[0]
	if pipeVal.typ != VGoVal {
		return nil, fmt.Errorf("exec-kill: argument must be an exec pipe")
	}

	pipe, ok := pipeVal.goVal.(*execPipeState)
	if !ok {
		return nil, fmt.Errorf("exec-kill: invalid pipe object")
	}

	killExecProcessTree(pipe.cmd)

	return globalEnv.bindings["#t"], nil
}
