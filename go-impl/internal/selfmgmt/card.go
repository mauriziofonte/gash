// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package selfmgmt

import (
	"strings"

	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/registry"
	"github.com/mauriziofonte/gash/internal/ui"
)

// card is the gash function: `gash migrate` runs gash_migrate, -h shows
// the registered help, a topic shows its reference card and an unknown
// word gets the 2.x unknown-topic error. Only the argument handling and
// the unknown topic are ported in M0; the card itself is not yet.
func (m *Module) card(c *invoke.Call) int {
	if c.Arg(0) == "migrate" {
		return c.Handlers.Run(&invoke.Call{
			Ctx: c.Ctx, Name: "gash_migrate", Args: c.Args[1:], Env: c.Env,
			Stdin: c.Stdin, Stdout: c.Stdout, Stderr: c.Stderr, Term: c.Term,
		})
	}
	noColor, topic := false, ""
	for _, a := range c.Args {
		switch a {
		case "--no-color":
			noColor = true
		case "-h", "--help":
			topic = "--help"
		default:
			topic = a
		}
	}
	color := c.Term.WantColors(noColor)
	if topic == "--help" {
		if f, ok := registry.Lookup("gash"); ok {
			help.Display(c.Stdout, color, f)
		}
		return 0
	}
	switch topic {
	case "navigation":
		topic = "nav"
	case "management":
		topic = "gash"
	case "php", "node":
		topic = "dev"
	}
	if topic != "" && topic != "all" {
		var names []string
		known := false
		for _, t := range registry.Topics() {
			names = append(names, t.Name)
			known = known || t.Name == topic
		}
		if !known {
			ui.NewPrinter(c.Stderr, c.Term, noColor).
				Error("Unknown topic: " + topic + ". Topics: " + strings.Join(names, " ") + ", all")
			return 1
		}
	}
	return c.Fail(invoke.NotPorted("gash"))
}
