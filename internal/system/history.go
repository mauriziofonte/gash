// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package system

import (
	"errors"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/ui"
)

// historyGrep is history_grep: the stub override pipes the caller's
// `history` here. Every word joins, with single spaces, one pattern (GNU
// BRE, case-insensitive, LEDGER SYS-38) that is matched against the
// command of each `NUM  CMD` line of the piped history, never a file named
// by a word (SYS-08). Commands are deduplicated on CMD, the first one
// kept; column 2 is its first two words and column 3 the rest of it by
// position; a command whose first word is history_grep or hg is left out
// (SYS-09).
func (m *Module) historyGrep(c *invoke.Call) int {
	if help.NeedsOf(c, c.Arg(0)) {
		return 0
	}
	p := ui.NewPrinter(c.Stderr, c.Term, false)
	if c.Arg(0) == "" {
		p.Error("Missing pattern. Usage: history_grep <pattern>")
		return 1
	}
	pattern := strings.Join(c.Args, " ")
	re, err := compileBRE(pattern)
	var ge *grepError
	switch {
	case errors.As(err, &ge):
		write(c.Stderr, "grep: "+ge.msg+"\n")
		return 0
	case err != nil:
		p.Error("Invalid pattern: " + pattern)
		return 1
	}
	data, _ := readAll(c.Stdin)
	seen := map[string]bool{}
	var rows []string
	suppressed := false
	for _, l := range strings.Split(strings.TrimSuffix(data, "\n"), "\n") {
		cmd := historyCommand(l)
		if cmd == "" || !re.MatchString(cmd) {
			continue
		}
		if strings.IndexByte(cmd, 0) >= 0 || !utf8.ValidString(cmd) {
			suppressed = true
			continue
		}
		f := awkFields(cmd)
		if f[0] == "history_grep" || f[0] == "hg" || seen[cmd] {
			continue
		}
		seen[cmd] = true
		col2, rest := f[0], ""
		if len(f) > 1 {
			col2 += " " + f[1]
			rest = strings.TrimLeft(cmd[strings.Index(cmd, f[0])+len(f[0]):], " \t")
			rest = strings.TrimLeft(rest[len(f[1]):], " \t")
		}
		row := strconv.Itoa(len(rows)+1) + "\t" + col2
		if rest != "" {
			row += "\t" + rest
		}
		rows = append(rows, row)
	}
	if suppressed {
		write(c.Stderr, "grep: (standard input): binary file matches\n")
	}
	ui.StyleOf(c.Term, false).Table(c.Stdout, rows,
		"--no-header", "--gap", "1", "--min", "1:5", "--min", "2:20", "--color", "1:muted", "--color", "2:accent")
	return 0
}

// historyCommand is CMD of a `history` line "  NUM  CMD" (the shim runs
// `history` with HISTTIMEFORMAT empty, so no timestamp precedes CMD): the
// text after the number and its blanks, trailing blanks dropped. A line
// without a leading number is all command.
func historyCommand(l string) string {
	t := strings.TrimLeft(l, " \t")
	i := 0
	for i < len(t) && t[i] >= '0' && t[i] <= '9' {
		i++
	}
	if i > 0 && i < len(t) && (t[i] == ' ' || t[i] == '\t' || t[i] == '*') {
		t = strings.TrimLeft(t[i:], "* \t")
	} else if i > 0 && i == len(t) {
		return ""
	}
	return strings.TrimRight(t, " \t\r")
}

// awkFields splits a line as awk's default FS does: runs of spaces, tabs
// and newlines, none at either end.
func awkFields(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return r == ' ' || r == '\t' || r == '\n' })
}

func field(f []string, i int) string {
	if i < len(f) {
		return f[i]
	}
	return ""
}

// readAll reads a whole stream; nil is empty.
func readAll(r io.Reader) (string, error) {
	if r == nil {
		return "", nil
	}
	b, err := io.ReadAll(r)
	return string(b), err
}
