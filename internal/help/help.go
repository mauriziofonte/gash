// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package help renders the 2.x help registry from the static registry:
// the full help page of a function (__gash_help_display) and the
// needs_help fallback.
package help

import (
	"io"
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

// Display writes the help page of f as __gash_help_display did, to w
// (stdout). It returns false when f has no registered help. color is the
// runtime gate (__gash_use_color).
func Display(w io.Writer, color bool, f *registry.Function) bool {
	if f == nil || !f.HasHelp {
		return false
	}
	var a, b, m, r string
	if color {
		a, b, m, r = term.SeqAccent, term.SeqBold, term.SeqMuted, term.SeqOff
	}
	h := f.Help
	var sb strings.Builder
	line := func(s string) { sb.WriteString(s); sb.WriteByte('\n') }
	heading := func(title string) { ui.Heading(&sb, color, 2, "", title) }

	line("")
	if h.Module != "" {
		line("  " + a + f.Name + r + m + "  (module: " + h.Module + ")" + r)
	} else {
		line("  " + a + f.Name + r)
	}
	if h.Short != "" {
		line("")
		line("  " + b + h.Short + r)
	}
	if h.Aliases != "" {
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
	if h.SeeAlso != "" {
		line("")
		heading("SEE ALSO")
		var list []string
		for _, s := range strings.Fields(h.SeeAlso) {
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
// returns true; otherwise it returns false.
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
	Fallback(c.Stdout, color, program, usage, description)
	return true
}

// NeedsOf is Needs with the fallback triple the registry recorded for the
// function of the call.
func NeedsOf(c *invoke.Call, arg string) bool {
	f, ok := registry.Lookup(c.Name)
	if !ok || f.Fallback == nil {
		return false
	}
	u := f.Fallback
	return Needs(c, u.Program, u.Usage, u.Description, arg)
}
