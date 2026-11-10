// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package help renders the 2.x help registry from the static registry:
// the full help page of a function (__gash_help_display) and the
// needs_help fallback.
package help

import (
	"io"
	"slices"
	"strings"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/lazyre"
	"github.com/mauriziofonte/gash/internal/registry"
	"github.com/mauriziofonte/gash/internal/term"
	"github.com/mauriziofonte/gash/internal/ui"
)

var (
	// [[ $line =~ ^[A-Z][A-Z\ ]+$ ]]
	sectionRe = lazyre.MustCompile(`^[A-Z][A-Z ]+$`)
	// [[ $line =~ ^[[:space:]]*# ]]
	commentRe = lazyre.MustCompile(`^[ \t\n\v\f\r]*#`)
)

// Display writes the help page of f to w (stdout): since 3.5 the page of
// its canonical command (gash files largest), with the text of its
// registered help rewritten for the canonical names
// (registry.CanonicalHelp), then its shortcut and its other names. It
// returns false when f has no registered help. color is the runtime gate.
func Display(w io.Writer, color bool, f *registry.Function) bool {
	if f == nil || !f.HasHelp {
		return false
	}
	var a, b, m, r string
	if color {
		a, b, m, r = term.SeqAccent, term.SeqBold, term.SeqMuted, term.SeqOff
	}
	cmd, hasCmd := registry.CommandOf(f.Name)
	h := f.Help
	title := registry.PageName(f)
	if hasCmd {
		h = registry.CanonicalHelp(f)
	}
	var sb strings.Builder
	line := func(s string) { sb.WriteString(s); sb.WriteByte('\n') }
	heading := func(title string) { ui.Heading(&sb, color, 2, "", title) }

	line("")
	line("  " + a + title + r)
	if h.Short != "" {
		line("")
		line("  " + b + h.Short + r)
	}
	// Plan S9: an agent function's page names the raw command shapes it
	// replaces, before anything else.
	if routes := registry.RoutesOf(f.Name); len(routes) > 0 {
		line("")
		heading("REPLACES")
		for _, rt := range routes {
			line("    " + rt.Shape)
			if rt.Use != rt.Fn {
				line("      " + m + "-> " + rt.Use + r)
			}
		}
	}
	if !hasCmd && h.Aliases != "" {
		line("")
		heading("ALIASES")
		var list []string
		for _, al := range splitComma(h.Aliases) {
			list = append(list, a+al+r)
		}
		line("    " + strings.Join(list, ", "))
	}
	if h.Body != "" {
		line("")
		for _, l := range strings.Split(h.Body, "\n") {
			switch {
			case sectionRe.MatchString(l):
				heading(l)
			case commentRe.MatchString(l):
				line("  " + m + l + r)
			case l == "":
				line("")
			default:
				line("  " + l)
			}
		}
	}
	if hasCmd {
		if cmd.Shortcut != "" {
			line("")
			heading("SHORTCUT")
			line("    " + a + cmd.Shortcut + r)
		}
		if cmd.Permanent() {
			line("")
			heading("FUNCTION")
			line("    " + a + f.Name + r + m + "  the same tool, a stable name for agents and scripts" + r)
		} else if former := cmd.FormerNames(); len(former) > 0 && slices.Contains(registry.Kept, former[0]) {
			line("")
			heading("FUNCTION")
			line("    " + a + former[0] + r + m + "  the same command, still a function for prompts and scripts" + r)
		} else if len(former) > 0 {
			line("")
			heading("FORMER NAME")
			line("    " + a + strings.Join(former, ", ") + r + m + "  the Gash 3.x name, defined with GASH_LEGACY_NAMES=1" + r)
		}
	} else if registry.Legacy(f.Name) {
		// gash_migrate: a legacy name whose command is not in the tree.
		line("")
		heading("FORMER NAME")
		line("    " + a + f.Name + r + m + "  the Gash 3.x name, defined with GASH_LEGACY_NAMES=1" + r)
	}
	if see := registry.SeeAlso(f); len(see) > 0 {
		line("")
		heading("SEE ALSO")
		var list []string
		for _, s := range see {
			list = append(list, a+s+r)
		}
		line("    " + strings.Join(list, ", "))
	}
	line("")
	_, _ = io.WriteString(w, sb.String())
	return true
}

// splitComma splits like IFS=',' read -r -a: fields kept as written
// (spaces included), a trailing empty field dropped.
func splitComma(s string) []string {
	parts := strings.Split(s, ",")
	if len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	return parts
}

// Fallback writes the 3-line help of needs_help when the registry has no
// entry: program, "Usage: USAGE", description.
func Fallback(w io.Writer, color bool, program, usage, description string) {
	if color {
		_, _ = io.WriteString(w, term.SeqAccent+program+term.SeqOff+"\n"+
			term.SeqBold+"Usage:"+term.SeqOff+" "+usage+"\n"+description+"\n")
		return
	}
	_, _ = io.WriteString(w, program+"\nUsage: "+usage+"\n"+description+"\n")
}

// Needs is the port of needs_help PROGRAM USAGE DESCRIPTION ARG: when ARG
// is -h or --help it prints the registered help of program (when the
// caller's shell had the help registry loaded) or the fallback, and
// returns true; otherwise it returns false. The fallback names a former
// flat name by its command (registry.CanonicalCall), as the help pages do.
func Needs(c *invoke.Call, program, usage, description, arg string) bool {
	if arg != "--help" && arg != "-h" {
		return false
	}
	color := c.Term.UseColor()
	if c.HelpLoaded() {
		if f, ok := registry.Lookup(program); ok && Display(c.Stdout, color, f) {
			return true
		}
	}
	Fallback(c.Stdout, color, registry.CanonicalCall(program), registry.CanonicalCall(usage), description)
	return true
}

// NeedsOf is Needs with the fallback triple the registry recorded for the
// function of the call.
//
// A function new in 3.x (Decision "new") has no 2.x needs_help triple: its
// fallback is derived from its registered help (the first USAGE line and
// the short description), so --help always prints something.
func NeedsOf(c *invoke.Call, arg string) bool {
	f, ok := registry.Lookup(c.Name)
	if !ok {
		return false
	}
	u := f.Fallback
	if u == nil && f.Decision == "new" && f.HasHelp {
		u = &registry.Usage{Program: f.Name, Usage: firstUsage(f.Help.Body), Description: f.Help.Short}
	}
	if u == nil {
		return false
	}
	return Needs(c, u.Program, u.Usage, u.Description, arg)
}

// firstUsage is the first line of a help body's USAGE section, trimmed.
func firstUsage(body string) string {
	_, rest, ok := strings.Cut(body, "USAGE\n")
	if !ok {
		return ""
	}
	line, _, _ := strings.Cut(rest, "\n")
	return strings.TrimSpace(line)
}
