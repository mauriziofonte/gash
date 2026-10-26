// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

//go:build !linux

package proctab

// HasProcFS reports whether this system has a Linux /proc to poll: never
// here, so the callers read ps.
func HasProcFS() bool { return false }

func ownerUID(string) int { return -1 }
