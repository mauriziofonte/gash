// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package system

import (
	"strings"

	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/system/handoff"
	"github.com/mauriziofonte/gash/internal/ui"
)

// sudoLast is the binary part of sudo_last. The stub override runs
// `sudo "$@"` itself when given a command; otherwise it reads `fc -ln
// -1`, passes the recalled command on stdin with a hand-off, and runs
// `sudo bash -c "$CMD"` when the binary says Go. The binary prints the
// warning and asks; declining prints Aborted. with status 1.
func (m *Module) sudoLast(c *invoke.Call) int {
	if help.NeedsOf(c, c.Arg(0)) {
		return 0
	}
	if c.Arg(0) != "" {
		// A direct `gash sudo_last COMMAND...`: the sudo run of 2.x.
		return m.passthrough(c, "sudo", c.Args...)
	}
	ch := handoff.Open(c.Env, m.open)
	defer ch.Close()
	last, _ := readAll(c.Stdin)
	p := ui.NewPrinter(c.Stderr, c.Term, false)
	// LEDGER SYS-12: no recalled command is an error before any warning,
	// and declining returns 1 like the other confirmations.
	if strings.TrimSpace(last) == "" {
		p.Error("No previous command in history.")
		return 1
	}
	p.Warning("About to run as root: " + last)
	if !ui.StyleOf(c.Term, false).Confirm(c.Term, c.Stderr, "Proceed?") {
		p.Info("Aborted.")
		return 1
	}
	if !ch.Active() {
		// Only the caller's shell can run its own last command.
		p.Error("sudo_last runs the previous command of the calling shell: use it from a Gash shell.")
		return 1
	}
	ch.Send(handoff.Go)
	return 0
}
