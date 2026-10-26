// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package gitx

import "io"

// confirm gates a git module action that changes a repository or its
// origin (LEDGER GIT-01), as docker_compose_upgrade does: true at once
// with yes (-y/--yes); headless or without a controlling terminal it
// refuses with `Error: Refusing to WHAT in headless mode without --yes.`
// (`without a terminal or --yes.` when only the terminal is missing);
// otherwise it prints plan on stdout and asks "Proceed? [y/N]", a decline
// printing `Info: Cancelled.`.
func (x *call) confirm(yes bool, what, plan string) bool {
	if yes {
		return true
	}
	if x.s.Headless {
		x.p.Error("Refusing to " + what + " in headless mode without --yes.")
		return false
	}
	if !x.hasTTY() {
		// 3.4: no terminal to ask on is not headless mode.
		x.p.Error("Refusing to " + what + " without a terminal or --yes.")
		return false
	}
	_, _ = io.WriteString(x.Stdout, plan+"\n")
	if !x.s.Confirm(x.Term, x.Stderr, "Proceed?") {
		x.p.Info("Cancelled.")
		return false
	}
	return true
}

// hasTTY reports whether the controlling terminal opens.
func (x *call) hasTTY() bool {
	f, err := x.Term.OpenTTY()
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}
