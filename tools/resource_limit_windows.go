//go:build windows

package tools

import (
	"fmt"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ResourceLimits specifies resource constraints for an exec command.
type ResourceLimits struct {
	// MaxMemoryBytes sets the maximum memory the process can use.
	// 0 means no limit.
	MaxMemoryBytes int64

	// MaxCPUMillis sets the maximum CPU time in milliseconds.
	// 0 means no limit.
	MaxCPUMillis int64
}

// JobObject wraps a Windows Job Object handle for resource limiting.
type JobObject struct {
	handle windows.Handle
}

// CreateJob creates a Windows Job Object with the specified limits.
// Call this before cmd.Start(). After cmd.Start(), call AssignProcess()
// to assign the child process to the job. Call Close() when done.
func (rl ResourceLimits) CreateJob() (*JobObject, error) {
	if rl.MaxMemoryBytes == 0 && rl.MaxCPUMillis == 0 {
		return nil, nil
	}

	jobHandle, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("CreateJobObject: %w", err)
	}

	if rl.MaxMemoryBytes > 0 {
		if err := setJobMemoryLimit(jobHandle, rl.MaxMemoryBytes); err != nil {
			windows.CloseHandle(jobHandle)
			return nil, err
		}
	}
	if rl.MaxCPUMillis > 0 {
		if err := setJobCpuLimit(jobHandle, rl.MaxCPUMillis); err != nil {
			windows.CloseHandle(jobHandle)
			return nil, err
		}
	}

	return &JobObject{handle: jobHandle}, nil
}

// AssignProcess assigns the started process to the Job Object.
// Call this immediately after cmd.Start(), before reading from pipes.
func (j *JobObject) AssignProcess(cmd *exec.Cmd) error {
	if j == nil || j.handle == 0 || cmd == nil || cmd.Process == nil {
		return nil
	}
	procHandle, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err != nil {
		return fmt.Errorf("OpenProcess: %w", err)
	}
	defer windows.CloseHandle(procHandle)

	err = windows.AssignProcessToJobObject(j.handle, procHandle)
	if err != nil {
		// ERROR_INVALID_PARAMETER (87): process already in a job or
		// CREATE_BREAKAWAY_FROM_JOB. Non-fatal — limits won't apply.
		if errno, ok := err.(syscall.Errno); ok && errno == 87 {
			return nil
		}
		return fmt.Errorf("AssignProcessToJobObject: %w", err)
	}
	return nil
}

// Close closes the Job Object handle.
func (j *JobObject) Close() error {
	if j == nil || j.handle == 0 {
		return nil
	}
	return windows.CloseHandle(j.handle)
}

// Format returns a human-readable description of the limits.
func (rl ResourceLimits) Format() string {
	if rl.MaxMemoryBytes == 0 && rl.MaxCPUMillis == 0 {
		return ""
	}
	var parts []string
	if rl.MaxMemoryBytes > 0 {
		parts = append(parts, fmt.Sprintf("max memory: %d MB", rl.MaxMemoryBytes/1024/1024))
	}
	if rl.MaxCPUMillis > 0 {
		parts = append(parts, fmt.Sprintf("max CPU: %d ms", rl.MaxCPUMillis))
	}
	return "(" + joinStr(parts, ", ") + ")"
}

func setJobMemoryLimit(job windows.Handle, maxBytes int64) error {
	type ExtendedLimitInfo struct {
		PerProcessUserTimeUlimit  windows.Filetime
		PerJobUserTimeLimit       windows.Filetime
		LimitFlags                uint32
		MinimumWorkingSetSize     uintptr
		MaximumWorkingSetSize     uintptr
		ActiveProcessLimit        uint32
		Affinity                  uintptr
		PriorityClass             uint32
		SchedulingClass           uint32
		_                         uint32
		ProcessMemoryLimit        uintptr
		JobMemoryLimit            uintptr
		PeakProcessMemoryUsed     uintptr
		PeakJobMemoryUsed         uintptr
	}

	const JOB_OBJECT_LIMIT_PROCESS_MEMORY = 0x00000100
	var info ExtendedLimitInfo
	info.LimitFlags = JOB_OBJECT_LIMIT_PROCESS_MEMORY
	info.ProcessMemoryLimit = uintptr(maxBytes)

	_, _, err := windows.NewLazySystemDLL("kernel32.dll").
		NewProc("SetInformationJobObject").Call(
		uintptr(job),
		20, // JobObjectExtendedLimitInformation
		uintptr(unsafe.Pointer(&info)),
		uintptr(unsafe.Sizeof(info)),
	)
	if err != nil && err.(syscall.Errno) != 0 {
		return fmt.Errorf("SetInformationJobObject (memory): %w", err)
	}
	return nil
}

func setJobCpuLimit(job windows.Handle, maxMillis int64) error {
	type BasicLimitInfo struct {
		PerProcessUserTimeUlimit  windows.Filetime
		PerJobUserTimeLimit       windows.Filetime
		LimitFlags                uint32
		MinimumWorkingSetSize     uintptr
		MaximumWorkingSetSize     uintptr
		ActiveProcessLimit        uint32
		Affinity                  uintptr
		PriorityClass             uint32
		SchedulingClass           uint32
	}

	timeUnits := maxMillis * 10000
	low := uint32(timeUnits & 0xFFFFFFFF)
	high := uint32(timeUnits >> 32)

	const JOB_OBJECT_LIMIT_JOB_TIME = 0x00000004
	var info BasicLimitInfo
	info.LimitFlags = JOB_OBJECT_LIMIT_JOB_TIME
	info.PerJobUserTimeLimit = windows.Filetime{
		LowDateTime:  low,
		HighDateTime: high,
	}

	_, _, err := windows.NewLazySystemDLL("kernel32.dll").
		NewProc("SetInformationJobObject").Call(
		uintptr(job),
		2, // JobObjectBasicLimitInformation
		uintptr(unsafe.Pointer(&info)),
		uintptr(unsafe.Sizeof(info)),
	)
	if err != nil && err.(syscall.Errno) != 0 {
		return fmt.Errorf("SetInformationJobObject (CPU): %w", err)
	}
	return nil
}

func joinStr(ss []string, sep string) string {
	if len(ss) == 0 {
		return ""
	}
	r := ss[0]
	for i := 1; i < len(ss); i++ {
		r += sep + ss[i]
	}
	return r
}
