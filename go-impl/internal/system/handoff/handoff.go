// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package handoff is the protocol between the stub overrides of the
// shell-mutating functions and the binary.
//
// A 2.x function that changes the calling shell in the middle of its work
// (hgrep and the WSL stoppers run `history -a` after their checks,
// sudo_last runs sudo in the shell after its question,
// gash_ssh_auto_unlock exports the agent variables) is split at that
// point. The override calls the binary with GASH__HANDOFF=3 and file
// descriptor 3 on a capture:
//
//	{ v="$(GASH__HANDOFF=3 __gash_call NAME "$@" 3>&1 1>&4 4>&-)" || rc=$?; } 4>&1
//
// The binary prints what 2.x printed before the shell step, then writes
// the hand-off on descriptor 3: Go when the shell step runs, or quoted
// assignments for the override to evaluate. It writes nothing when 2.x
// stopped earlier (help, a refused argument, a declined question), and the
// override returns the binary's status. Where 2.x went on after the shell
// step, the override calls the binary again with GASH__RESUME=1. Without
// GASH__HANDOFF (a direct `gash NAME` run) the binary does all of its work
// but the shell step, which only the caller's shell can do.
package handoff

import (
	"errors"
	"io"
	"os"
	"syscall"

	"github.com/mauriziofonte/gash/internal/invoke"
)

const (
	// Var asks for the hand-off; its value is the descriptor, always 3.
	Var = "GASH__HANDOFF"
	// ResumeVar marks the call that runs after the shell step.
	ResumeVar = "GASH__RESUME"
	// Go tells the override to run its shell step.
	Go = "1"
	// fd is the descriptor of the hand-off.
	fd = 3
)

// Opener opens the hand-off descriptor for writing.
type Opener func() (io.WriteCloser, error)

// OpenFD opens descriptor 3 as the caller's shell passed it: the pipe of
// the override's capture, so anything else there (the variable set by
// hand) is refused. It marks it close-on-exec first, so that no child of
// the binary (an agent that daemonizes) keeps the capture open. Call it
// before the binary opens files or starts children.
func OpenFD() (io.WriteCloser, error) {
	var st syscall.Stat_t
	if err := syscall.Fstat(fd, &st); err != nil {
		return nil, err
	}
	if st.Mode&syscall.S_IFMT != syscall.S_IFIFO {
		return nil, errors.New("handoff: descriptor 3 is not a pipe")
	}
	syscall.CloseOnExec(fd)
	return os.NewFile(fd, "handoff"), nil
}

// Requested reports that the override asked for a hand-off.
func Requested(e invoke.Env) bool { return e.Get(Var) == "3" }

// Resumed reports the call after the shell step.
func Resumed(e invoke.Env) bool { return e.Get(ResumeVar) == "1" }

// Channel is an open hand-off; the nil Channel is the direct run.
type Channel struct {
	w io.WriteCloser
}

// Open opens the hand-off of a call that asked for one. It returns nil
// for a direct run, or when the descriptor is not usable (the variable
// set by hand): the binary then does all of its work.
func Open(e invoke.Env, open Opener) *Channel {
	if !Requested(e) || open == nil {
		return nil
	}
	w, err := open()
	if err != nil {
		return nil
	}
	return &Channel{w: w}
}

// Active reports an open hand-off.
func (ch *Channel) Active() bool { return ch != nil && ch.w != nil }

// Send writes data and closes the hand-off; a nil Channel ignores it.
func (ch *Channel) Send(data string) {
	if !ch.Active() {
		return
	}
	_, _ = io.WriteString(ch.w, data)
	_ = ch.w.Close()
	ch.w = nil
}

// Close closes the hand-off without data (the override then returns the
// binary's status).
func (ch *Channel) Close() {
	if ch.Active() {
		_ = ch.w.Close()
		ch.w = nil
	}
}
