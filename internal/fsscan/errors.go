// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package fsscan

import (
	"io"
	"strconv"

	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/ui"
)

// fsError is a structured scanner error (__gash_fs_error): a JSON envelope
// in machine mode, a colored "Error:"/"Hint:" pair otherwise. It always
// stands for exit status 1.
type fsError struct {
	code    string
	details string
	action  contract.Action
	hint    string
}

// errf builds an fsError. The code sets the action through the contract
// table, so a code is only ever emitted with its 2.x action.
func errf(code, details, hint string) *fsError {
	a, ok := contract.ActionOf(code)
	if !ok {
		a = contract.Stop
	}
	return &fsError{code: code, details: details, action: a, hint: hint}
}

// emit writes the error as __gash_fs_error did: the JSON envelope on w
// (stderr) in machine mode, else the "Error:" line (details appended as
// "code: details") and the "Hint:" line through the core emitters.
func (e *fsError) emit(w io.Writer, s ui.Style, machine bool) int {
	if machine {
		env := contract.Legacy(e.code, e.details, string(e.action), e.hint)
		_ = env.Write(w)
		return 1
	}
	p := s.Printer(w)
	msg := e.code
	if e.details != "" {
		msg = msg + ": " + e.details
	}
	p.Error(msg)
	if e.hint != "" {
		p.Hint(e.hint)
	}
	return 1
}

// scanWarn is __gash_fs_scan_warn: a partial-results warning on stderr
// when count > 0. Machine mode emits the compact JSON warning line; human
// mode the colored "Warning:" line. It never changes the exit status.
func scanWarn(w io.Writer, s ui.Style, count int, machine bool) {
	if count <= 0 {
		return
	}
	if machine {
		var o contract.Object
		o.String("warning", "partial_results")
		o.Int("skipped_entries", int64(count))
		o.String("hint", "Some paths were unreadable (permissions)")
		b, _ := o.Bytes(contract.EscapeBash)
		_, _ = w.Write(append(b, '\n'))
		return
	}
	s.Printer(w).Warning("Scan completed with " + strconv.Itoa(count) + " unreadable path(s) (permission denied).")
}
