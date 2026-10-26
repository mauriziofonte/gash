// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package proctab

import (
	"os"
	"syscall"
)

// HasProcFS reports whether this system has a Linux /proc to poll.
func HasProcFS() bool {
	_, err := os.Stat("/proc/self/stat")
	return err == nil
}

// ownerUID is the owner of a /proc/PID directory: the process's effective
// uid (-1 when it is gone).
func ownerUID(dir string) int {
	fi, err := os.Stat(dir)
	if err != nil {
		return -1
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		return int(st.Uid)
	}
	return -1
}
