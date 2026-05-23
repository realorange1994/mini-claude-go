//go:build unix

package microlisp

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
)

// wrapWithResourceLimits returns a bash command wrapper that applies ulimit
// settings to the child process only, without affecting the parent.
//
// This replaces the old approach of temporarily setting rlimits on the parent
// process, which could kill the parent if it exceeded the limits during the
// fork/exec window.
//
// Returns ("bash", []string{"-c", wrapped}, nil) when limits are set,
// or (command, args, nil) with no wrapper when limits are zero.
func wrapWithResourceLimits(command string, args []string, maxMemoryMB int64, maxCPUMS int64) (cmdName string, cmdArgs []string, err error) {
	if maxMemoryMB <= 0 && maxCPUMS <= 0 {
		return command, args, nil
	}

	bashPath, bashErr := findBash()
	if bashErr != nil {
		// No bash available — skip resource limits rather than risk killing parent.
		return command, args, nil
	}

	// Build ulimit prefix
	var ulimitParts []string
	if maxMemoryMB > 0 {
		// ulimit -v: virtual memory limit in kilobytes
		kb := maxMemoryMB * 1024
		ulimitParts = append(ulimitParts, fmt.Sprintf("ulimit -v %d", kb))
	}
	if maxCPUMS > 0 {
		// ulimit -t: CPU time limit in seconds (minimum 1)
		sec := maxCPUMS / 1000
		if sec == 0 {
			sec = 1
		}
		ulimitParts = append(ulimitParts, fmt.Sprintf("ulimit -t %d", sec))
	}

	// Escape command and arguments for shell
	escapedCmd := escapeShellArg(command)
	escapedArgs := make([]string, len(args))
	for i, a := range args {
		escapedArgs[i] = escapeShellArg(a)
	}

	// Build the wrapped command:
	// ulimit ... ; exec -- original-command 'arg1' 'arg2' ...
	// 'exec' replaces the bash process with the target, so ulimit limits
	// are inherited by the child directly.
	inner := strings.Join(ulimitParts, "; ") + "; exec -- " + escapedCmd
	if len(escapedArgs) > 0 {
		inner += " " + strings.Join(escapedArgs, " ")
	}

	return bashPath, []string{"-c", inner}, nil
}

// escapeShellArg escapes a string for safe inclusion in a single-quoted shell argument.
// ' becomes '\'' (end quote, escaped quote, start quote).
func escapeShellArg(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

var (
	bashPathOnce sync.Once
	bashPathVal  string
	bashPathErr  error
)

func findBash() (string, error) {
	bashPathOnce.Do(func() {
		bashPathVal, bashPathErr = exec.LookPath("bash")
		if bashPathErr != nil {
			// Try common locations
			for _, p := range []string{"/bin/bash", "/usr/bin/bash"} {
				if _, err := os.Stat(p); err == nil {
					bashPathVal = p
					bashPathErr = nil
					return
				}
			}
			bashPathErr = fmt.Errorf("bash not found")
		}
	})
	return bashPathVal, bashPathErr
}
