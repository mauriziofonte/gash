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
}

// NewPrinter returns the printer of a call: the runtime gate of t, its
// glyph set, and the palette (empty when noColor, as the shadowed locals
// of 2.x).
func NewPrinter(w io.Writer, t *term.Term, noColor bool) Printer {
	p := t.Palette()
	if noColor {
		p = term.NoColors
	}
	return Printer{W: w, Color: t.UseColor(), P: p, G: t.Glyphs()}
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
