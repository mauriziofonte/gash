// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package ui

import (
	"fmt"
	"io"
	"strings"
)

// The layout emitters of lib/core/output.sh. Those with options take the
// 2.x words (flags first, as the Bash call had them), so a call ports
// verbatim and keeps the 2.x parsing of odd input.

func write(w io.Writer, s string) { _, _ = io.WriteString(w, s) }

// indentOf is the [[ $indent =~ ^[0-9]+$ ]] || indent=DEF check of the
// emitters, then printf's reading of the width (a leading 0 is octal).
func indentOf(v string, def int) int {
	if !isDigits(v) {
		return def
	}
	return Int(v)
}

// Print is __gash_print STYLE MESSAGE (stdout): the message in a style
// (the semantic names of ColorOf or the hue names red, green, yellow,
// blue, cyan, white and bold_*; another word gives no style), plain
// without colors.
func (s Style) Print(w io.Writer, style, msg string) {
	if !s.Color {
		write(w, msg+"\n")
		return
	}
	code := ""
	switch style {
	case "red":
		code = s.P.Red
	case "green":
		code = s.P.Green
	case "yellow":
		code = s.P.Yellow
	case "blue":
		code = s.P.Blue
	case "cyan":
		code = s.P.Cyan
	case "white":
		code = s.P.White
	case "bold_red":
		code = s.P.BoldRed
	case "bold_green":
		code = s.P.BoldGreen
	case "bold_yellow":
		code = s.P.BoldYellow
	case "bold_blue":
		code = s.P.BoldBlue
	case "bold_cyan":
		code = s.P.BoldCyan
	case "bold_white":
		code = s.P.BoldWhite
	case "primary", "secondary", "accent", "muted", "success", "warning", "error", "info", "bold", "dim", "underline":
		code = s.ColorOf(style)
	}
	write(w, code+msg+s.P.Off+"\n")
}

// Section is __gash_section TITLE (stdout): an empty line, a rule of
// min(Columns, 60) glyphs, the title, an empty line.
func (s Style) Section(w io.Writer, title string) {
	width := min(s.Columns, 60)
	line := strings.Repeat(s.G.HLine, max(width, 0))
	if s.Color {
		write(w, "\n"+s.P.Primary+line+s.P.Off+"\n"+s.P.Bold+title+s.P.Off+"\n\n")
		return
	}
	write(w, "\n"+line+"\n"+title+"\n\n")
}

// Heading is __gash_heading [--indent N] [--note TEXT] TITLE (stdout): a
// bold title, the note muted two spaces after it. Options are read only
// while more than one word is left, as in 2.x.
func (s Style) Heading(w io.Writer, args ...string) {
	indent, note := "0", ""
	for len(args) > 1 {
		switch args[0] {
		case "--indent":
			indent, args = args[1], args[2:]
			continue
		case "--note":
			note, args = args[1], args[2:]
			continue
		}
		break
	}
	title := ""
	if len(args) > 0 {
		title = args[0]
	}
	var b strings.Builder
	b.WriteString(Spaces(indentOf(indent, 0)))
	if s.Color {
		b.WriteString(s.P.Bold + title + s.P.Off)
		if note != "" {
			b.WriteString("  " + s.P.Muted + note + s.P.Off)
		}
	} else {
		b.WriteString(title)
		if note != "" {
			b.WriteString("  " + note)
		}
	}
	b.WriteByte('\n')
	write(w, b.String())
}

// option is one "--name VALUE" step of the 2.x option loops written as
// `name="${2-DEF}"; shift 2 || break`: the value (DEF when it is the last
// word), the words left, and whether the loop goes on.
func option(args []string, def string) (string, []string, bool) {
	if len(args) < 2 {
		return def, args, false
	}
	return args[1], args[2:], true
}

// KV is __gash_kv [--indent N] [--width N] [--sep STR] [--value-color
// STYLE] KEY VALUE [KEY VALUE...] (stdout): one "KEY VALUE" line per pair,
// keys padded to the longest (or to --width; 0 is no padding), joined by
// --sep (one space by default); on a color terminal the keys are bold and
// the values take --value-color.
func (s Style) KV(w io.Writer, args ...string) {
	indent, width, sep, vstyle := "0", "-1", " ", ""
	more := true
	for more && len(args) > 0 {
		switch args[0] {
		case "--indent":
			indent, args, more = option(args, "0")
		case "--width":
			width, args, more = option(args, "0")
		case "--sep":
			sep, args, more = option(args, "")
		case "--value-color":
			vstyle, args, more = option(args, "")
		default:
			more = false
		}
	}
	ind := indentOf(indent, 0)
	wd := -1
	if v := strings.TrimPrefix(width, "-"); isDigits(v) {
		wd = Int(width)
	}
	if wd < 0 {
		wd = 0
		for i := 0; i < len(args); i += 2 {
			wd = max(wd, Len(args[i]))
		}
	}
	kon, von, off := "", "", ""
	if s.Color {
		kon, off = s.P.Bold, s.P.Off
		von = s.ColorOf(vstyle)
	}
	voff := ""
	if von != "" {
		voff = off
	}
	var b strings.Builder
	for i := 0; i < len(args); i += 2 {
		val := ""
		if i+1 < len(args) {
			val = args[i+1]
		}
		b.WriteString(Spaces(ind) + kon + PadRight(args[i], wd) + off + sep + von + val + voff + "\n")
	}
	write(w, b.String())
}

// List is __gash_list [--indent N] [--color STYLE] ITEM... (stdout): a
// bullet line per item, indented two spaces by default, the bullet in the
// primary color (or STYLE) on a color terminal.
func (s Style) List(w io.Writer, args ...string) {
	indent, style := "2", "primary"
	more := true
	for more && len(args) > 0 {
		switch args[0] {
		case "--indent":
			indent, args, more = option(args, "2")
		case "--color":
			style, args, more = option(args, "")
		default:
			more = false
		}
	}
	on, off := "", ""
	if s.Color {
		on = s.ColorOf(style)
		if on != "" {
			off = s.P.Off
		}
	}
	pad := Spaces(indentOf(indent, 2))
	var b strings.Builder
	for _, item := range args {
		b.WriteString(pad + on + s.G.Bullet + off + " " + item + "\n")
	}
	write(w, b.String())
}

// Mark is __gash_mark [--indent N] STATUS TEXT (stdout): a finding of a
// report, "<glyph> TEXT" in the status color on a color terminal,
// "Word: TEXT" otherwise. STATUS: ok (or success), warning, error; any
// other is info.
func (s Style) Mark(w io.Writer, args ...string) {
	indent := "0"
	if len(args) > 0 && args[0] == "--indent" {
		indent = "0"
		if len(args) > 1 {
			indent = args[1]
		}
		args = args[min(2, len(args)):]
	}
	status, text := "info", ""
	if len(args) > 0 {
		status = args[0]
	}
	if len(args) > 1 {
		text = args[1]
	}
	c, g, word := s.P.Info, s.G.Info, "Info:"
	switch status {
	case "ok", "success":
		c, g, word = s.P.Success, s.G.OK, "OK:"
	case "warning":
		c, g, word = s.P.Warning, s.G.Warn, "Warning:"
	case "error":
		c, g, word = s.P.Error, s.G.Err, "Error:"
	}
	pad := Spaces(indentOf(indent, 0))
	if s.Color {
		write(w, pad+c+g+s.P.Off+" "+text+"\n")
		return
	}
	write(w, pad+word+" "+text+"\n")
}

// AskRender is __gash_ask_render QUESTION ANSWERS, written to w (stderr in
// 2.x): "QUESTION [ANSWERS] ", the answers bold on a color terminal.
func (s Style) AskRender(w io.Writer, question, answers string) {
	if s.Color {
		write(w, question+" "+s.P.Bold+"["+answers+"]"+s.P.Off+" ")
		return
	}
	write(w, question+" ["+answers+"] ")
}

// TaskBegin is the first half of __gash_task MSG CMD...: "MSG... " on w
// (stderr in 2.x), after the info glyph on a color terminal.
func (s Style) TaskBegin(w io.Writer, msg string) {
	if s.StderrFancy() {
		write(w, s.P.Info+s.G.Info+s.P.Off+" "+msg+"... ")
		return
	}
	write(w, msg+"... ")
}

// TaskEnd ends the line of TaskBegin: "done" when ok, else "failed", in
// the success or error color on a color terminal.
func (s Style) TaskEnd(w io.Writer, ok bool) {
	word, c := "done", s.P.Success
	if !ok {
		word, c = "failed", s.P.Error
	}
	if s.StderrFancy() {
		write(w, c+word+s.P.Off+"\n")
		return
	}
	write(w, word+"\n")
}

// Task is __gash_task MSG CMD...: TaskBegin, run, TaskEnd; it returns the
// status of run (the command's output passes through as it writes it).
func (s Style) Task(w io.Writer, msg string, run func() int) int {
	s.TaskBegin(w, msg)
	rc := run()
	s.TaskEnd(w, rc == 0)
	return rc
}

// ClearScreen is __gash_clear_screen: the clear-screen sequence on a color
// terminal outside headless mode, nothing otherwise.
func (s Style) ClearScreen(w io.Writer) {
	if !s.Headless && s.Color {
		write(w, "\x1b[H\x1b[2J")
	}
}

// Progress is __gash_progress CURRENT TOTAL MESSAGE on w (stderr in 2.x),
// on a terminal outside headless mode only: a 30-cell bar, the percentage
// and the message, redrawn with a carriage return; a newline once
// CURRENT reaches TOTAL.
func (s Style) Progress(w io.Writer, current, total int, msg string) {
	if !s.StderrTTY || s.Headless {
		return
	}
	if total <= 0 {
		total = 1
	}
	if current > total {
		current = total
	}
	full, empty := "#", "."
	if s.UTF8 {
		full, empty = "█", "░"
	}
	percent := current * 100 / total
	filled := percent * 30 / 100
	var bar strings.Builder
	for i := 0; i < 30; i++ {
		if i < filled {
			bar.WriteString(full)
		} else {
			bar.WriteString(empty)
		}
	}
	on, off, el := "", "", ""
	if s.StderrFancy() {
		on, off, el = s.P.Primary, s.P.Off, "\x1b[K"
	}
	out := fmt.Sprintf("\r%s%s%s %3d%% %s%s", on, bar.String(), off, percent, msg, el)
	if current >= total {
		out += "\n"
	}
	write(w, out)
}
