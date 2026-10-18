// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package ui

import (
	"context"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/mauriziofonte/gash/internal/term"
)

// ReadLine is `IFS= read -r VAR` on r: one line without its newline, read
// one byte at a time so that nothing after the newline is consumed (Bash
// reads a pipe unbuffered); NUL bytes are dropped. ok is false at the end
// of input, also after a partial last line (Bash then fails too, with the
// partial text in VAR).
func ReadLine(r io.Reader) (line string, ok bool) {
	var b strings.Builder
	var c [1]byte
	for {
		n, err := r.Read(c[:])
		if n == 1 {
			switch c[0] {
			case '\n':
				return b.String(), true
			case 0:
			default:
				b.WriteByte(c[0])
			}
			continue
		}
		if err != nil {
			return b.String(), false
		}
	}
}

// Choose is __gash_choose VAR DEFAULT OPTION...: it lets the user pick one
// of opts and returns its 1-based index; ok is false on cancel or at the
// end of input, and when there are no options.
//
// On a terminal (stdin and stdout, GASH_TUI not plain, TERM not dumb) the
// options are listed once and a selection line is redrawn in place: arrow
// keys, j/k or a digit move it, Enter confirms, q or Esc cancels. Otherwise
// a numbered prompt reads a line (an empty answer takes def), three times
// at most. Everything goes to out (stdout in 2.x).
func (s Style) Choose(ctx context.Context, t *term.Term, in io.Reader, out io.Writer, def int, opts []string) (int, bool) {
	n := len(opts)
	if n == 0 {
		return 0, false
	}
	sel := def
	if sel < 1 || sel > n {
		sel = 1
	}
	on, off := "", ""
	if s.Color {
		on, off = s.P.Primary+s.P.Bold, s.P.Off
	}
	write := func(str string) { _, _ = io.WriteString(out, str) }

	if t.Getenv("GASH_TUI") == "plain" || t.Getenv("TERM") == "dumb" || !t.StdinTTY() || !t.StdoutTTY() {
		var b strings.Builder
		for i, o := range opts {
			b.WriteString("  " + strconv.Itoa(i+1) + ") " + o + "\n")
		}
		write(b.String())
		for try := 1; try <= 3; try++ {
			write("  Choice [" + strconv.Itoa(sel) + "]: ")
			line, ok := ReadLine(in)
			if !ok {
				write("\n")
				return 0, false
			}
			line = removeSpace(line)
			if line == "" {
				line = strconv.Itoa(sel)
			}
			if line == "q" {
				return 0, false
			}
			if v, ok := decimalInRange(line, n); ok {
				return v, true
			}
			write("  Type a number from 1 to " + strconv.Itoa(n) + " (attempt " + strconv.Itoa(try) + " of 3).\n")
		}
		return 0, false
	}

	var b strings.Builder
	for i, o := range opts {
		b.WriteString("    " + strconv.Itoa(i+1) + ") " + o + "\n")
	}
	write(b.String())
	keys, _ := t.OpenKeys() // on an error: at the end of input, as read fails
	defer keys.Close()
	el := ""
	if s.Color {
		el = term.EraseEOL
	}
	prev, cancel := 0, false
	up := func() {
		if sel > 1 {
			sel--
		} else {
			sel = n
		}
	}
	down := func() {
		if sel < n {
			sel++
		} else {
			sel = 1
		}
	}
loop:
	for {
		text := s.G.Pointer + " " + strconv.Itoa(sel) + ") " + opts[sel-1]
		line := "\r  " + on + text + off + el
		if el == "" && Len(text) < prev {
			line += Spaces(prev - Len(text))
		}
		write(line)
		prev = Len(text)
		k, err := keys.Key(ctx)
		if err != nil {
			cancel = true
			break
		}
		switch {
		case k == 0x1b:
			switch string(keys.More(2, 50*time.Millisecond)) {
			case "[A", "OA":
				up()
			case "[B", "OB":
				down()
			case "":
				cancel = true
				break loop
			}
		case k == 'k':
			up()
		case k == 'j':
			down()
		case k >= '1' && k <= '9':
			if int(k-'0') <= n {
				sel = int(k - '0')
			}
		case k == 'q':
			cancel = true
			break loop
		case k == '\n':
			break loop
		}
	}
	write("\n")
	if cancel {
		return 0, false
	}
	return sel, true
}

// removeSpace is ${s//[[:space:]]/} with the C locale's classes.
func removeSpace(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '\n', '\v', '\f', '\r':
			return -1
		}
		return r
	}, s)
}

// decimalInRange is [[ $s =~ ^[0-9]+$ ]] && ((10#$s >= 1 && 10#$s <= n)).
func decimalInRange(s string, n int) (int, bool) {
	if !isDigits(s) {
		return 0, false
	}
	t := strings.TrimLeft(s, "0")
	if len(t) > 18 {
		return 0, false
	}
	v, err := strconv.Atoi("0" + t)
	if err != nil || v < 1 || v > n {
		return 0, false
	}
	return v, true
}
