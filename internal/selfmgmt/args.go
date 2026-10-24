// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package selfmgmt

import (
	"slices"
	"strings"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/ui"
)

// usageStatus is the status of a refused argument (the registry's usage
// status).
const usageStatus = 2

// badArgs is the argument check of a self-management command, run before
// any side effect (LEDGER SELF-15): the first argument that is not one of
// allowed is refused with "Error: Unknown option: X" (a word starting with
// "-") or "Error: Unexpected argument: X", then the usage, and badArgs
// reports true; the command then returns usageStatus. 2.x ignored such
// words: gash_env_init --bogus created the file, gash_upgrade --bogus
// upgraded.
func badArgs(c *invoke.Call, usage string, allowed ...string) bool {
	for _, a := range c.Args {
		if slices.Contains(allowed, a) {
			continue
		}
		what := "Unexpected argument: "
		if strings.HasPrefix(a, "-") && a != "-" {
			what = "Unknown option: "
		}
		off := !c.Term.WantColors(slices.Contains(c.Args, "--no-color"))
		ui.StyleOf(c.Term, off).Printer(c.Stderr).Error(what + a + " (usage: " + usage + ")")
		return true
	}
	return false
}
