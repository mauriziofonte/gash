// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

//go:build !linux

package app

import "io"

// serve needs /proc to reach the shell's streams: elsewhere it declines to
// start, and gash.sh execs the binary for every call.
func serve(_ []string, _ io.Reader, _ io.Writer) int { return 1 }
