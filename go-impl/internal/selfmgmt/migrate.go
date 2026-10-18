// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package selfmgmt

import (
	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/ui"
)

// migrate is gash_migrate (and gash migrate), decision D-6: 3.x does not
// migrate a Gash 1.x configuration. -h/--help shows the registered help;
// any other call, options included, prints one error line pointing to the
// 2.x route, writes nothing and returns 1 (design/upgrade.md 3.5).
func (m *Module) migrate(c *invoke.Call) int {
	if help.NeedsOf(c, c.Arg(0)) {
		return 0
	}
	ui.StyleOf(c.Term, false).Printer(c.Stderr).Error(MigratePointer(installDir(c)))
	return 1
}

// MigratePointer is the line gash_migrate prints for the install in dir.
func MigratePointer(dir string) string {
	return "gash_migrate: Gash 3 does not migrate a Gash 1.x configuration; check out v2.1.0 in " +
		dir + ", open a new terminal, run gash_migrate there, then gash_upgrade"
}

// installDir is the Gash install directory of the call: GASH_DIR, else
// ~/.gash.
func installDir(c *invoke.Call) string {
	if d := c.Env.Get("GASH_DIR"); d != "" {
		return d
	}
	return c.Env.Get("HOME") + "/.gash"
}
