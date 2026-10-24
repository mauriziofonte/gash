// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package system

import (
	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/ui"
)

// mkdirCD is the binary part of mkdir_cd: the help and the missing
// argument. The stub override keeps `mkdir -p "$dir" && cd "$dir"` in the
// caller's shell (CDPATH and option-like names as 2.x, SYS-11). A direct
// `gash mkdir_cd DIR` creates the directory; the cd needs the shell.
func (m *Module) mkdirCD(c *invoke.Call) int {
	if help.NeedsOf(c, c.Arg(0)) {
		return 0
	}
	dir := c.Arg(0)
	if dir == "" {
		ui.NewPrinter(c.Stderr, c.Term, false).Error("Missing directory. Usage: mkdir_cd <directory>")
		return 1
	}
	if len(c.Args) > 1 {
		ui.NewPrinter(c.Stderr, c.Term, false).Error(unexpectedArg(c.Args[1]))
		return 1
	}
	if m.passthrough(c, "mkdir", "-p", dir) != 0 {
		return 1
	}
	return 0
}
