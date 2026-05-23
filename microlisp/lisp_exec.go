//go:build unix || windows

package microlisp

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
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
	var stdin string
	var timeoutMs int64 = 120000
	var maxMemoryMB int64
	var maxCPUMS int64

	for i := 1; i < len(args); {
		// Handle odd remaining arg (positional without a pair)
		if i+1 >= len(args) {
			if args[i].typ == VStr {
				cmdArgs = append(cmdArgs, args[i].str)
			}
			break
		}

		key := args[i]
		val := args[i+1]

		// Check if this is a keyword arg (VSym starting with ":")
		isKw := key.typ == VSym && len(key.str) > 0 && key.str[0] == ':'

		if !isKw {
			// Both key and val are treated as positional command args
			if key.typ == VStr {
				cmdArgs = append(cmdArgs, key.str)
			}
			if val.typ == VStr {
				cmdArgs = append(cmdArgs, val.str)
			}
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
		case ":stdin":
			if val.typ == VStr {
				stdin = val.str
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

	return executeCommand(command, cmdArgs, workingDir, env, stdin, time.Duration(timeoutMs)*time.Millisecond, maxMemoryMB, maxCPUMS)
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
func executeCommand(command string, cmdArgs []string, workingDir string, env []string, stdin string, timeout time.Duration, maxMemoryMB int64, maxCPUMS int64) (*Value, error) {
	// Detect shell builtins or shell syntax and auto-wrap with bash -c.
	// This handles cases like "cd /home/work", "export FOO=bar", "source script", etc.
	cmdName := command
	cmdArgsList := cmdArgs
	captureCwd := false

	if needShellWrap(command) || containsShellSyntax(cmdArgs) {
		cmdName, cmdArgsList = wrapShellBuiltin(command, cmdArgs)
		captureCwd = true
	}

	// Special case: "cd /some/path" alone → change working dir for this exec call.
	// Since each exec is stateless, cd alone only affects this invocation.
	// The workingDir is used to set cmd.Dir so the child starts in that directory.
	if command == "cd" && len(cmdArgs) >= 1 && !containsShellSyntax(cmdArgs) {
		targetDir := cmdArgs[0]
		if workingDir == "" {
			workingDir = targetDir
		} else {
			if !filepath.IsAbs(targetDir) {
				workingDir = filepath.Join(workingDir, targetDir)
			} else {
				workingDir = targetDir
			}
		}
		// cd with no further commands: just run pwd to verify the path is accessible
		cmdName = "bash"
		cmdArgsList = []string{"-c", "cd " + targetDir + " && pwd"}
		captureCwd = true
	}

	// When shell-wrapped, append a sentinel to capture the shell's final $PWD.
	// This lets us report the working directory back to the agent.
	const cwdSentinel = "__SHELL_CWD__:"
	if captureCwd && len(cmdArgsList) >= 2 {
		inner := cmdArgsList[1] // the -c argument
		cmdArgsList[1] = inner + "; echo '" + cwdSentinel + "'$PWD"
	}

	// Helper: extract CWD sentinel from stdout and return result with :cwd field.
	// Called once stdoutBuf is fully populated.
	makeResult := func(stdout, stderr string, exitCode int) *Value {
		cwd := ""
		if captureCwd {
			stdout, cwd = extractCwdFromStdout(stdout)
		}
		return makeExecResult(stdout, stderr, exitCode, cwd)
	}
	makeBgResult := func(stdout, stderr string, pid int, reason string) *Value {
		cwd := ""
		if captureCwd {
			stdout, cwd = extractCwdFromStdout(stdout)
		}
		return makeBackgroundExecResult(stdout, stderr, pid, reason, cwd)
	}

	cmd := exec.Command(cmdName, cmdArgsList...)

	if workingDir != "" {
		cmd.Dir = workingDir
	}

	// If :env is empty, inherit os.Environ().
	// If :env is provided, merge with os.Environ() (child overrides parent).
	// Always ensure PATH is set — if the parent has no PATH, use a sensible default.
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	} else {
		cmd.Env = os.Environ()
	}
	// Ensure PATH exists in cmd.Env (handles case where parent has no PATH)
	hasPath := false
	for _, e := range cmd.Env {
		if len(e) >= 5 && e[:5] == "PATH=" {
			hasPath = true
			break
		}
	}
	if !hasPath {
		cmd.Env = append(cmd.Env, defaultPathEnv())
	}

	// Set stdin: if :stdin was provided, pipe it in; otherwise /dev/null
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}

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

	if err := cmd.Start(); err != nil {
		return makeExecResult("", err.Error(), 1, ""), nil
	}

	// Start goroutine-based resource monitor for child process (pure Go, no bash dependency).
	// If the child exceeds memory/CPU limits, the monitor kills it and sends a reason.
	limitCh := startResourceMonitor(cmd, maxMemoryMB, maxCPUMS)

	if usePipes {
		// Pipe-based reading with stall detection.
		// When stdin is nil and a command waits for input (e.g. sudo password),
		// output stalls. We detect this and move the process to background.
		var pipeWg sync.WaitGroup
		pipeWg.Add(2)
		go func() {
			defer pipeWg.Done()
			io.Copy(&stdoutBuf, stdoutPipe)
		}()
		go func() {
			defer pipeWg.Done()
			io.Copy(&stderrBuf, stderrPipe)
		}()

		// Stall detection: if the process produces no output for a while
		// and is still running, it's likely waiting for interactive input.
		// Instead of killing, we move it to background and return early.
		//
		// The stall timeout is proportional to the command timeout:
		//   - For short timeouts (<60s): use timeout/3 (at least 10s)
		//   - For longer timeouts (>=60s): use 60s (allows compilation time)
		// This avoids false positives for commands like "go test" that have
		// long silent periods during compilation.
		//
		// Additionally, we only move to background if the total timeout
		// is under 3 minutes. For very long commands, we wait for the
		// full timeout rather than guessing "stall".
		stallTimeout := timeout / 3
		if timeout >= 60*time.Second {
			stallTimeout = 60 * time.Second
		}
		if stallTimeout < 10*time.Second {
			stallTimeout = 10 * time.Second
		}
		stallTimer := time.NewTimer(stallTimeout)
		defer stallTimer.Stop()

		done := make(chan error, 1)
		go func() {
			done <- cmd.Wait()
		}()

		select {
		case err := <-done:
			// Process completed normally
			stallTimer.Stop()
			pipeWg.Wait() // Ensure all pipe data is captured
			exitCode := 0
			if err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					exitCode = exitErr.ExitCode()
				} else {
					exitCode = 1
				}
			}
			return makeResult(stdoutBuf.String(), stderrBuf.String(), exitCode), nil

		case reason := <-limitCh:
			// Resource limit exceeded — process was killed by monitor
			stallTimer.Stop()
			pipeWg.Wait()
			if reason != "" {
				stderrBuf.WriteString("\n[resource limit] " + reason)
			}
			return makeResult(stdoutBuf.String(), stderrBuf.String(), -1), nil

		case <-stallTimer.C:
			// Stall period with no completion — check if process has produced any output.
			pid := cmd.Process.Pid
			if stdoutBuf.Len() == 0 && stderrBuf.Len() == 0 {
				// No output at all — likely waiting for interactive input.
				// Move to background instead of killing. The done goroutine
				// will reap the zombie when the process eventually exits.
				reason := fmt.Sprintf(
					"Command produced no output for %s — likely waiting for interactive input. "+
						"Process moved to background (PID %d). "+
						"Try non-interactive flags: -y, --yes, --force, or pipe input via exec-with-input.",
					stallTimeout, pid)
				return makeBgResult("", "", pid, reason), nil
			}
			// Some output was produced — reset stall timer and wait for timeout
			stallTimer.Reset(timeout)
			select {
			case err := <-done:
				stallTimer.Stop()
				pipeWg.Wait() // Ensure all pipe data is captured
				exitCode := 0
				if err != nil {
					if exitErr, ok := err.(*exec.ExitError); ok {
						exitCode = exitErr.ExitCode()
					} else {
						exitCode = 1
					}
				}
				return makeResult(stdoutBuf.String(), stderrBuf.String(), exitCode), nil
			case reason := <-limitCh:
				stallTimer.Stop()
				pipeWg.Wait()
				if reason != "" {
					stderrBuf.WriteString("\n[resource limit] " + reason)
				}
				return makeResult(stdoutBuf.String(), stderrBuf.String(), -1), nil
			case <-stallTimer.C:
				// Timeout with some output — move to background
				stdout := stdoutBuf.String()
				stderr := stderrBuf.String()
				reason := fmt.Sprintf(
					"Command timed out after %s but produced output — moved to background (PID %d). "+
						"It may still be running.",
					timeout, pid)
				return makeBgResult(stdout, stderr, pid, reason), nil
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
			return makeBgResult(stdout, stderr, pid, reason), nil
		}
	}

	// Fallback: direct buffer attachment (no stall detection).
	// Used when StdoutPipe/StderrPipe failed.
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		exitCode := 0
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else {
				exitCode = 1
			}
		}
		return makeResult(stdoutBuf.String(), stderrBuf.String(), exitCode), nil
	case reason := <-limitCh:
		// Resource limit exceeded — process was killed by monitor
		if reason != "" {
			stderrBuf.WriteString("\n[resource limit] " + reason)
		}
		<-done // Wait for goroutine cleanup
		return makeResult(stdoutBuf.String(), stderrBuf.String(), -1), nil
	case <-time.After(timeout):
		// Hard timeout — move to background
		pid := cmd.Process.Pid
		stdout := stdoutBuf.String()
		stderr := stderrBuf.String()
		reason := fmt.Sprintf(
			"Command timed out after %s — moved to background (PID %d). "+
				"It may still be running.",
			timeout, pid)
		return makeBgResult(stdout, stderr, pid, reason), nil
	}
}

// makeExecResult creates a Lisp plist: (:stdout "..." :stderr "..." :exit-code N :cwd "...")
func makeExecResult(stdout, stderr string, exitCode int, cwd string) *Value {
	parts := []*Value{
		vsym(":stdout"), StringValue(stdout),
		vsym(":stderr"), StringValue(stderr),
		vsym(":exit-code"), vnum(float64(exitCode)),
	}
	if cwd != "" {
		parts = append(parts, vsym(":cwd"), StringValue(cwd))
	}
	return listToPlist(parts...)
}

// makeBackgroundExecResult creates a plist with :background t and :stall-reason.
// :exit-code is -1 to signal the process is still running.
func makeBackgroundExecResult(stdout, stderr string, pid int, reason string, cwd string) *Value {
	parts := []*Value{
		vsym(":stdout"), StringValue(stdout),
		vsym(":stderr"), StringValue(stderr),
		vsym(":exit-code"), vnum(-1),
		vsym(":background"), vnum(1),
		vsym(":stall-reason"), StringValue(reason),
	}
	if cwd != "" {
		parts = append(parts, vsym(":cwd"), StringValue(cwd))
	}
	return listToPlist(parts...)
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
		return makeExecResult("", err.Error(), 1, ""), nil
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

	return makeExecResult(stdoutBuf.String(), stderrBuf.String(), exitCode, ""), nil
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

// ─── shell builtin auto-detection ────────────────────────────────────────────

// shellBuiltins is a set of common POSIX shell builtins that are not
// standalone executables. If the command matches one of these, we
// auto-wrap it with "bash -c '...'" so it can execute.
var shellBuiltins = map[string]bool{
	"cd": true, "export": true, "source": true, "alias": true,
	"unalias": true, "set": true, "unset": true, "readonly": true,
	"local": true, "declare": true, "typeset": true, "eval": true,
	"exec": true, "ulimit": true, "umask": true,
	"trap": true, "return": true, "exit": true, "shift": true,
	"break": true, "continue": true, "wait": true, "jobs": true,
	"fg": true, "bg": true, "disown": true, "history": true,
	"shopt": true, "bind": true, "builtin": true, "command": true,
	"enable": true, "help": true, "let": true, "logout": true,
	"mapfile": true, "readarray": true, "read": true, "type": true,
	"hash": true, "true": true, "false": true, "test": true,
	"times": true,
}

// needShellWrap checks if a command is likely to be a shell builtin.
func needShellWrap(cmd string) bool {
	return shellBuiltins[strings.ToLower(cmd)]
}

// containsShellSyntax checks if any argument contains shell syntax
// that requires a shell to interpret (pipes, redirects, &&, ||, etc.).
func containsShellSyntax(args []string) bool {
	shellChars := []string{"|", ">", "<", "&&", "||", ";", "$(", "`", ">>", ">&", "2>", "1>", "<<<"}
	for _, a := range args {
		for _, s := range shellChars {
			if strings.Contains(a, s) {
				return true
			}
		}
	}
	return false
}

// wrapShellBuiltin wraps a command and its arguments into a bash -c invocation.
// Returns ("bash", []string{"-c", "cmd arg1 arg2 ..."}).
func wrapShellBuiltin(command string, args []string) (string, []string) {
	shellCmd := command
	for _, a := range args {
		shellCmd += " " + a
	}
	return "bash", []string{"-c", shellCmd}
}

// extractCwdFromStdout parses the CWD sentinel from stdout.
// It removes the sentinel line and returns the cleaned stdout and extracted CWD.
func extractCwdFromStdout(stdout string) (cleaned string, cwd string) {
	const sentinel = "__SHELL_CWD__:"
	idx := strings.LastIndex(stdout, sentinel)
	if idx < 0 {
		return stdout, ""
	}
	// Extract CWD: everything after sentinel on the same line
	after := stdout[idx+len(sentinel):]
	newlineIdx := strings.IndexByte(after, '\n')
	if newlineIdx >= 0 {
		cwd = strings.TrimRight(after[:newlineIdx], "\r")
	} else {
		cwd = strings.TrimRight(after, "\r")
	}
	// Remove sentinel line (and the newline before it) from stdout
	before := stdout[:idx]
	// Trim trailing newline before sentinel
	if len(before) > 0 && before[len(before)-1] == '\n' {
		before = before[:len(before)-1]
	}
	return before, cwd
}
