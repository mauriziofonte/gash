// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package ui reimplements the 2.x output primitives of lib/core/output.sh
// byte for byte (same palette, glyphs, widths and stream policy).
package ui

import (
	"io"
	"strings"

	"github.com/mauriziofonte/gash/internal/term"
)

// Heading writes a bold title line without rules (__gash_heading
// [--indent N] [--note TEXT] TITLE) to w (stdout in 2.x). color is the
// runtime gate (__gash_use_color).
func Heading(w io.Writer, color bool, indent int, note, title string) {
	var b strings.Builder
	if indent > 0 {
		b.WriteString(strings.Repeat(" ", indent))
	}
	if color {
		b.WriteString(term.SeqBold + title + term.SeqOff)
		if note != "" {
			b.WriteString("  " + term.SeqMuted + note + term.SeqOff)
		}
	} else {
		b.WriteString(title)
		if note != "" {
			b.WriteString("  " + note)
		}
	}
	b.WriteByte('\n')
	_, _ = io.WriteString(w, b.String())
}
