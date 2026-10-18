// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package ui

import (
	"io"

	"github.com/mauriziofonte/gash/internal/term"
)

// Printer writes the 2.x message emitters (__gash_info, __gash_error, ...)
// to W, which is stderr in 2.x.
type Printer struct {
	W io.Writer
	// Color is the runtime gate (__gash_use_color: stdout is a terminal
	// and the environment permits colors).
	Color bool
	// P is the palette in scope: term.Colors, or term.NoColors where 2.x
	// had shadowed it (--no-color, __GASH_COLOR_OFF_LOCALS).
	P term.Palette
	G term.Glyphs
	// DebugOn is GASH_DEBUG=1 (__gash_debug prints only then).
	DebugOn bool
}

// NewPrinter returns the printer of a call: the runtime gate of t, its
// glyph set, and the palette (empty when noColor, as the shadowed locals
// of 2.x).
func NewPrinter(w io.Writer, t *term.Term, noColor bool) Printer {
	p := t.Palette()
	if noColor {
		p = term.NoColors
	}
	return Printer{W: w, Color: t.UseColor(), P: p, G: t.Glyphs(), DebugOn: t.Getenv("GASH_DEBUG") == "1"}
}

// emit is __gash_emit COLOR COLORED_LABEL PLAIN_LABEL MESSAGE.
func (p Printer) emit(color, colored, plain, msg string) {
	if p.Color {
		_, _ = io.WriteString(p.W, color+colored+p.P.Off+" "+msg+"\n")
		return
	}
	_, _ = io.WriteString(p.W, plain+" "+msg+"\n")
}

// Info is __gash_info.
func (p Printer) Info(msg string) { p.emit(p.P.Info, p.G.Info, "Info:", msg) }

// Error is __gash_error.
func (p Printer) Error(msg string) { p.emit(p.P.Error, p.G.Err+" Error:", "Error:", msg) }

// Success is __gash_success.
func (p Printer) Success(msg string) { p.emit(p.P.Success, p.G.OK, "OK:", msg) }

// Warning is __gash_warning.
func (p Printer) Warning(msg string) { p.emit(p.P.Warning, p.G.Warn+" Warning:", "Warning:", msg) }

// Hint is __gash_hint.
func (p Printer) Hint(msg string) {
	if p.Color {
		_, _ = io.WriteString(p.W, p.P.Muted+"Hint: "+msg+p.P.Off+"\n")
		return
	}
	_, _ = io.WriteString(p.W, "Hint: "+msg+"\n")
}

// Note is __gash_note: "<glyph> TEXT" on a color terminal, "TEXT"
// otherwise.
func (p Printer) Note(msg string) {
	if p.Color {
		_, _ = io.WriteString(p.W, p.P.Info+p.G.Info+p.P.Off+" "+msg+"\n")
		return
	}
	_, _ = io.WriteString(p.W, msg+"\n")
}

// Step is __gash_step CURRENT TOTAL MESSAGE: "[CURRENT/TOTAL] MESSAGE".
func (p Printer) Step(current, total, msg string) {
	if p.Color {
		_, _ = io.WriteString(p.W, p.P.Primary+"["+current+"/"+total+"]"+p.P.Off+" "+msg+"\n")
		return
	}
	_, _ = io.WriteString(p.W, "["+current+"/"+total+"] "+msg+"\n")
}

// Debug is __gash_debug: "Debug: MESSAGE", only with GASH_DEBUG=1.
func (p Printer) Debug(msg string) {
	if !p.DebugOn {
		return
	}
	p.emit(p.P.Muted, p.G.Info+" Debug:", "Debug:", msg)
}

// Usage is __gash_usage: "Usage: TEXT" after a usage error.
func (p Printer) Usage(msg string) { p.emit(p.P.Info, p.G.Info+" Usage:", "Usage:", msg) }

// SSH is __gash_ssh MESSAGE [COLOR]: "SSH: MESSAGE", with the glyph and
// color of COLOR (info, the default for "", success, warning, error) on a
// color terminal.
func (p Printer) SSH(msg, color string) {
	if !p.Color {
		_, _ = io.WriteString(p.W, "SSH: "+msg+"\n")
		return
	}
	c, g := p.P.Info, p.G.Info
	switch color {
	case "success":
		c, g = p.P.Success, p.G.OK
	case "warning":
		c, g = p.P.Warning, p.G.Warn
	case "error":
		c, g = p.P.Error, p.G.Err
	}
	_, _ = io.WriteString(p.W, c+g+p.P.Off+" "+p.P.Primary+"SSH:"+p.P.Off+" "+msg+"\n")
}
