// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package proctab

import "os"

// HasProcFS reports whether this system has a Linux /proc to poll.
func HasProcFS() bool {
	_, err := os.Stat("/proc/self/stat")
	return err == nil
}
