// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package ui

import (
	"io"
	"strings"

	"github.com/mauriziofonte/gash/internal/term"
)

// TrimIFS is what `read -r VAR` keeps of a line with the default IFS: the
// leading and trailing spaces, tabs and newlines removed.
func TrimIFS(s string) string { return strings.Trim(s, " \t\n") }

// readTTY reads one answer line from the controlling terminal of t, as
// `read -r VAR </dev/tty` did; ok is false when there is no terminal or
// the input ends before a newline.
func readTTY(t *term.Term) (string, bool) {
	f, err := t.OpenTTY()
	if err != nil {
		return "", false
	}
	defer func() { _ = f.Close() }()
	line, ok := ReadLine(f)
	return TrimIFS(line), ok
}

// Confirm is needs_confirm_prompt PROMPT: "PROMPT [y/N] " on w (stderr in
// 2.x), then one line read from the controlling terminal; true only for y
// or Y. It refuses without a word in headless mode, and after the
// question when there is no terminal or the input ends: the shell's own
// /dev/tty error of 2.x is not printed (LEDGER CORE-01).
func (s Style) Confirm(t *term.Term, w io.Writer, prompt string) bool {
	if s.Headless {
		return false
	}
	s.AskRender(w, prompt, "y/N")
	reply, ok := readTTY(t)
	return ok && (reply == "y" || reply == "Y")
}

// ConfirmPlan is __gash_confirm_plan ASSUME_YES: true at once when
// assumeYes is "1" (a -y flag); otherwise "Do you want to continue? [Y/n]
// " on w (stderr) and an answer from the controlling terminal, where
// Enter, y and yes (any case) go ahead and anything else prints
// "Aborted.". In headless mode or without a terminal it refuses with
// "No terminal to ask for confirmation: pass -y to go ahead."; when the
// input ends it prints a newline and refuses.
func (s Style) ConfirmPlan(t *term.Term, w io.Writer, assumeYes string) bool {
	if assumeYes == "1" {
		return true
	}
	p := s.Printer(w)
	if s.Headless {
		p.Error("No terminal to ask for confirmation: pass -y to go ahead.")
		return false
	}
	f, err := t.OpenTTY()
	if err != nil {
		p.Error("No terminal to ask for confirmation: pass -y to go ahead.")
		return false
	}
	defer func() { _ = f.Close() }()
	s.AskRender(w, "Do you want to continue?", "Y/n")
	line, ok := ReadLine(f)
	if !ok {
		write(w, "\n")
		return false
	}
	switch TrimIFS(line) {
	case "", "y", "Y", "yes", "yeS", "yEs", "yES", "Yes", "YeS", "YEs", "YES":
		return true
	}
	p.Info("Aborted.")
	return false
}
