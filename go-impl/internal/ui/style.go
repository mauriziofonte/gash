// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package ui

import (
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/mauriziofonte/gash/internal/term"
)

// Style is what a 2.x layout primitive read from the shell when it ran:
//
//   - Color: the runtime gate (__gash_use_color: the environment permits
//     colors and stdout is a terminal);
//   - P: the palette in scope (term.Colors, or term.NoColors where 2.x had
//     the variables zeroed at load time or shadowed by
//     __GASH_COLOR_OFF_LOCALS for a --no-color call, which leaves Color
//     as it was);
//   - G: the glyph set of the locale;
//   - Width: __gash_tty_width.
//
// The primitives below are methods of Style, so a handler builds one per
// call (StyleOf) and passes it around, as 2.x passed its globals.
type Style struct {
	Color bool
	P     term.Palette
	G     term.Glyphs
	Width int
	// Debug is GASH_DEBUG=1, for the Debug emitter.
	Debug bool
	// Columns is ${COLUMNS:-80} as __gash_section reads it: COLUMNS when
	// it is a run of digits, else 80, never evaluated (LEDGER SYS-34).
	Columns int
	// UTF8 is __gash_locale_utf8 (the progress bar's own glyphs).
	UTF8 bool
	// Headless is GASH_HEADLESS=1.
	Headless bool
	// StderrTTY is [[ -t 2 ]]; with Color it is __gash_stderr_fancy.
	StderrTTY bool
}

// StyleOf returns the style of a call on t. noColor is a --no-color flag
// of the function: 2.x then shadowed the palette (eval
// "$__GASH_COLOR_OFF_LOCALS"), which leaves the runtime gate unchanged.
func StyleOf(t *term.Term, noColor bool) Style {
	p := t.Palette()
	if noColor {
		p = term.NoColors
	}
	return Style{
		Color: t.UseColor(), P: p, G: t.Glyphs(), Width: t.Width(), Debug: t.Getenv("GASH_DEBUG") == "1",
		Columns: Columns(t.Getenv("COLUMNS")), UTF8: t.Glyphs() == term.UnicodeGlyphs,
		Headless: t.Headless(), StderrTTY: t.StderrTTY(),
	}
}

// Columns is the width ${COLUMNS:-80} gives 2.x arithmetic, for a COLUMNS
// that is a run of digits (a leading 0 is octal, as in Bash); any other
// value, empty included, is 80 and is never evaluated (LEDGER SYS-34).
func Columns(v string) int {
	if !isDigits(v) {
		return 80
	}
	return Int(v)
}

// StderrFancy is __gash_stderr_fancy: stderr is a terminal and colors are
// on.
func (s Style) StderrFancy() bool { return s.StderrTTY && s.Color }

// Printer returns the message emitters writing to w with this style.
func (s Style) Printer(w io.Writer) Printer {
	return Printer{W: w, Color: s.Color, P: s.P, G: s.G, DebugOn: s.Debug}
}

// ColorOf is __gash_color_of with the palette in scope.
func (s Style) ColorOf(name string) string { return s.P.Style(name) }

// Len is ${#s}: the number of characters (an invalid byte counts as one).
// 2.x counted characters in a UTF-8 locale and bytes in C; 3.x always
// counts characters (LEDGER CORE-16).
func Len(s string) int { return utf8.RuneCountInString(s) }

// Sub is ${s:off:n} on characters, n < 0 meaning to the end. off is
// clamped to the string.
func Sub(s string, off, n int) string {
	if off < 0 {
		off = 0
	}
	i := 0
	for k := 0; k < off && i < len(s); k++ {
		_, sz := utf8.DecodeRuneInString(s[i:])
		i += sz
	}
	if n < 0 {
		return s[i:]
	}
	j := i
	for k := 0; k < n && j < len(s); k++ {
		_, sz := utf8.DecodeRuneInString(s[j:])
		j += sz
	}
	return s[i:j]
}

// Tail is ${s: -n}: the last n characters; ${s: -0} is the whole string.
func Tail(s string, n int) string {
	l := Len(s)
	if n <= 0 || n >= l {
		return s
	}
	return Sub(s, l-n, -1)
}

// PadRight is printf '%-*s' WIDTH S: Bash's printf counts the width in
// bytes, whatever the locale, so a multibyte cell gets fewer spaces than
// its character count suggests (reproduced from 2.x). A negative width
// pads the same way.
func PadRight(s string, width int) string {
	if width < 0 {
		width = -width
	}
	if n := width - len(s); n > 0 {
		return s + strings.Repeat(" ", n)
	}
	return s
}

// PadLeft is printf '%*s' WIDTH S, the width in bytes; a negative width
// left-justifies, as printf does.
func PadLeft(s string, width int) string {
	if width < 0 {
		return PadRight(s, -width)
	}
	if n := width - len(s); n > 0 {
		return strings.Repeat(" ", n) + s
	}
	return s
}

// Spaces is printf '%*s' N with an empty string: |N| spaces.
func Spaces(n int) string {
	if n < 0 {
		n = -n
	}
	return strings.Repeat(" ", n)
}

// Int is the value Bash arithmetic gives a word that the 2.x code
// validated as an optionally signed run of digits: base 10, or base 8
// with a leading 0. Anything else (including an invalid octal number,
// which Bash reports as an error) is 0; the word is never evaluated as an
// expression (LEDGER CORE-02).
func Int(s string) int {
	neg := false
	d := s
	if strings.HasPrefix(d, "-") {
		neg, d = true, d[1:]
	} else if strings.HasPrefix(d, "+") {
		d = d[1:]
	}
	if d == "" || strings.Trim(d, "0123456789") != "" {
		return 0
	}
	base := 10
	if len(d) > 1 && d[0] == '0' {
		base = 8
	}
	n, err := strconv.ParseInt(d, base, 64)
	if err != nil {
		if ne, ok := err.(*strconv.NumError); ok && ne.Err == strconv.ErrRange {
			n = wrapInt(d, base)
		} else {
			return 0
		}
	}
	if neg {
		n = -n
	}
	return int(n)
}

// wrapInt is the value of an out-of-range number in Bash's intmax_t
// arithmetic (two's complement wrap-around).
func wrapInt(d string, base int) int64 {
	var n uint64
	for i := 0; i < len(d); i++ {
		n = n*uint64(base) + uint64(d[i]-'0')
	}
	return int64(n)
}

// isDigits reports [[ s =~ ^[0-9]+$ ]].
func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
