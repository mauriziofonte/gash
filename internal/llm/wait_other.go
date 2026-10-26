// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

//go:build !linux

package llm

// newWaker is the plain sleeper: without inotify, llm_wait looks at the
// file at every poll interval.
func newWaker() waker { return sleepWaker{} }
