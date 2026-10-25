// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package ui

import (
	"io"
	"strings"

	"github.com/mauriziofonte/gash/internal/lazyre"
)

// Bar is __gash_bar VALUE MAX WIDTH: the full and the empty part of a
// WIDTH-cell bar, VALUE/MAX of it full (rounded, at least one cell when
// VALUE > 0, at most WIDTH).
func (s Style) Bar(value, maxV, width int) (full, empty string) {
	f := 0
	if maxV > 0 {
		f = (value*width + maxV/2) / maxV
	}
	if f < 1 && value > 0 && maxV > 0 {
		f = 1
	}
	if f > width {
		f = width
	}
	var fb, eb strings.Builder
	for i := 0; i < f; i++ {
		fb.WriteString(s.G.BarFull)
	}
	for i := f; i < width; i++ {
		eb.WriteString(s.G.BarEmpty)
	}
	return fb.String(), eb.String()
}

var sgrRe = lazyre.MustCompile("\x1b\\[[0-9;]*m")

// StripSGR removes every ANSI color sequence (ESC [ digits/; m), repeating
// until none is left, as the 2.x loops over BASH_REMATCH did.
func StripSGR(s string) string {
	for sgrRe.MatchString(s) {
		s = sgrRe.ReplaceAllString(s, "")
	}
	return s
}

// VisibleLen is __gash_visible_len: the character count of s without its
// color sequences.
func VisibleLen(s string) int { return Len(StripSGR(s)) }

// GroupDigits is __gash_group_digits: 1234567 -> 1,234,567 (a leading "-"
// kept; any other text grouped by its last three characters as 2.x did).
func GroupDigits(n string) string {
	sign := ""
	if strings.HasPrefix(n, "-") {
		sign, n = "-", n[1:]
	}
	res := ""
	for Len(n) > 3 {
		res = "," + Tail(n, 3) + res
		n = Sub(n, 0, Len(n)-3)
	}
	return sign + n + res
}

// Box writes lines inside a rounded box as __gash_box [--color ESC] LINE...
// does, to w (stdout in 2.x): sized to the longest visible line, capped at
// the terminal width minus 6 (at least 20); a line too wide loses its
// colors and is cut with an ellipsis. border is the --color sequence: the
// border takes it when s.Color is on.
func (s Style) Box(w io.Writer, border string, lines ...string) {
	_, _ = io.WriteString(w, strings.Join(s.BoxLines(border, lines...), "\n")+"\n")
}

// BoxLines returns the lines Box prints.
func (s Style) BoxLines(border string, lines ...string) []string {
	on, off := "", ""
	if s.Color && border != "" {
		on, off = border, s.P.Off
	}
	inner := 0
	for _, l := range lines {
		if n := VisibleLen(l); n > inner {
			inner = n
		}
	}
	limit := s.Width - 6
	if limit < 20 {
		limit = 20
	}
	if inner > limit {
		inner = limit
	}
	corners := []rune(s.G.Corners)
	for len(corners) < 4 {
		corners = append(corners, ' ')
	}
	rule := strings.Repeat(s.G.HLine, inner+4)
	v := s.G.VLine
	out := make([]string, 0, len(lines)+2)
	out = append(out, on+string(corners[0])+rule+string(corners[1])+off)
	for _, l := range lines {
		n := VisibleLen(l)
		if n > inner {
			l = Sub(StripSGR(l), 0, inner-1) + s.G.Ellipsis
			n = inner
		}
		out = append(out, on+v+off+"  "+l+Spaces(inner-n)+"  "+on+v+off)
	}
	out = append(out, on+string(corners[2])+rule+string(corners[3])+off)
	return out
}

// Wrap is __gash_wrap WIDTH TEXT: TEXT word-wrapped to WIDTH characters,
// one element per line. A newline in TEXT starts a new line (an empty line
// stays empty, and TEXT "" is one empty line); a line keeps its leading
// spaces, and one starting with "- " wraps with two more. Words are split
// on spaces and tabs.
func Wrap(width int, text string) []string {
	var out []string
	for _, para := range strings.Split(text, "\n") {
		if para == "" {
			out = append(out, "")
			continue
		}
		lead := para[:len(para)-len(strings.TrimLeft(para, " "))]
		ind := lead
		if strings.HasPrefix(para[len(lead):], "- ") {
			ind += "  "
		}
		line := ""
		for _, word := range strings.FieldsFunc(para, func(r rune) bool { return r == ' ' || r == '\t' }) {
			switch {
			case line == "":
				line = lead + word
			case Len(line)+1+Len(word) > width:
				out = append(out, line)
				line = ind + word
			default:
				line += " " + word
			}
		}
		out = append(out, line)
	}
	return out
}
