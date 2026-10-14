//go:build windows

package ccdata

import (
	"strconv"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

func isProcessAlive(pid int, procStart string) bool {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		// Access denied still means the process exists.
		return err == windows.ERROR_ACCESS_DENIED
	}
	defer windows.CloseHandle(h)
	var code uint32
	if windows.GetExitCodeProcess(h, &code) != nil || code != 259 { // STILL_ACTIVE
		return false
	}
	want, err := strconv.ParseUint(procStart, 10, 64)
	if err != nil {
		return true // no start time recorded: trust the PID
	}
	var created, exited, kernel, user windows.Filetime
	if windows.GetProcessTimes(h, &created, &exited, &kernel, &user) != nil {
		return true
	}
	got := uint64(created.HighDateTime)<<32 | uint64(created.LowDateTime)
	// Allow a small tolerance (1 ms) for rounding in the recorded value.
	diff := int64(got - want)
	return diff > -10000 && diff < 10000
}

// RunningClaudeProcesses returns the PIDs of processes whose image name is
// claude.exe (native Claude Code installs).
func RunningClaudeProcesses() []int {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(snap)
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	var pids []int
	for err = windows.Process32First(snap, &pe); err == nil; err = windows.Process32Next(snap, &pe) {
		if strings.EqualFold(windows.UTF16ToString(pe.ExeFile[:]), "claude.exe") {
			pids = append(pids, int(pe.ProcessID))
		}
	}
	return pids
}
