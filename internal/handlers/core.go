// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package handlers

import (
	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/registry"
	"github.com/mauriziofonte/gash/internal/ui"
)

// registerCore binds the binary-backed functions of lib/core/utils.sh and
// the compat operations of the help registry.
func registerCore(t *invoke.Table) {
	t.Handle("all_colors", allColors)
	t.HandleCompat("help", compatHelp)
}

// allColors is all_colors: the 256-color chart, colored on a color
// terminal.
func allColors(c *invoke.Call) int {
	if help.NeedsOf(c, c.Arg(0)) {
		return 0
	}
	ui.ColorChart(c.Stdout, c.Term.UseColor())
	return 0
}

// compatHelp is `gash __compat help NAME`: the registered help page of
// NAME (__gash_help_display), status 1 without output when NAME has none.
// The generated needs_help calls it when the caller's shell has the help
// registry.
func compatHelp(c *invoke.Call) int {
	f, ok := registry.Lookup(c.Arg(0))
	if !ok || !help.Display(c.Stdout, c.Term.UseColor(), f) {
		return 1
	}
	return 0
}
