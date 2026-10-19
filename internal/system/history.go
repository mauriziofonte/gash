// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package system

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"unicode/utf8"

	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/ui"
)

// historyGrep is history_grep: the stub override pipes the caller's
// `history` here. The binary runs the 2.x pipeline `grep -i -- "$@" |
// grep -v history_grep | awk | awk | __gash_table`: with more than one
// word, grep reads the named files instead of the history (SYS-08), and
// the two awk passes keep their column and dedup defects (SYS-09).
func (m *Module) historyGrep(c *invoke.Call) int {
	if help.NeedsOf(c, c.Arg(0)) {
		return 0
	}
	p := ui.NewPrinter(c.Stderr, c.Term, false)
	if c.Arg(0) == "" {
		p.Error("Missing pattern. Usage: history_grep <pattern>")
		return 1
	}
	lines, ok := m.grepI(c, p, c.Args[0], c.Args[1:])
	if !ok {
		return 1
	}
	var kept []string
	for _, l := range lines {
		if !strings.Contains(l, "history_grep") {
			kept = append(kept, l)
		}
	}
	// awk '{ $1=""; seen[$0]++; if (seen[$0]==1) print $0 }'
	seen := map[string]bool{}
	var uniq []string
	for _, l := range kept {
		f := awkFields(l)
		rebuilt := ""
		if len(f) > 1 {
			rebuilt = " " + strings.Join(f[1:], " ")
		}
		if !seen[rebuilt] {
			seen[rebuilt] = true
			uniq = append(uniq, rebuilt)
		}
	}
	// awk '{ printf "%s\t%s\t%s\n", NR, $1" "$2, substr($0, index($0,$3)) }'
	rows := make([]string, 0, len(uniq))
	for i, l := range uniq {
		f := awkFields(l)
		f1, f2, f3 := field(f, 0), field(f, 1), field(f, 2)
		rest := l
		if j := strings.Index(l, f3); f3 != "" && j >= 0 {
			rest = l[j:]
		}
		rows = append(rows, strconv.Itoa(i+1)+"\t"+f1+" "+f2+"\t"+rest)
	}
	ui.StyleOf(c.Term, false).Table(c.Stdout, rows,
		"--no-header", "--gap", "1", "--min", "1:5", "--min", "2:20", "--color", "1:muted", "--color", "2:accent")
	return 0
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

// grepI is `grep -i -- PATTERN [FILE...]` (GNU grep 3.11, the BRE of
// compileBRE): the matching lines (prefixed with "FILE:" for two or more
// files), with GNU grep's messages on stderr. Without FILE it reads
// stdin. A pattern grep refuses prints grep's error and yields no lines;
// one Go cannot express prints "Error: Invalid pattern: P" and ok is
// false (LEDGER SYS-38).
func (m *Module) grepI(c *invoke.Call, p ui.Printer, pattern string, files []string) (lines []string, ok bool) {
	re, err := compileBRE(pattern)
	var ge *grepError
	switch {
	case errors.As(err, &ge):
		write(c.Stderr, "grep: "+ge.msg+"\n")
		return nil, true
	case err != nil:
		p.Error("Invalid pattern: " + pattern)
		return nil, false
	}
	if len(files) == 0 {
		data, _ := readAll(c.Stdin)
		return grepData(c.Stderr, re, data, "(standard input)", ""), true
	}
	for _, f := range files {
		prefix := ""
		if len(files) > 1 {
			prefix = f + ":"
		}
		if f == "-" {
			data, _ := readAll(c.Stdin)
			lines = append(lines, grepData(c.Stderr, re, data, "(standard input)", prefix)...)
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			write(c.Stderr, "grep: "+f+": "+grepReason(err)+"\n")
			continue
		}
		lines = append(lines, grepData(c.Stderr, re, string(data), f, prefix)...)
	}
	return lines, true
}

// grepData matches the lines of one input. Input with a NUL byte is
// binary: no line is printed. A line that is not UTF-8 is never printed.
// Either way a match is reported as "grep: NAME: binary file matches".
func grepData(stderr io.Writer, re *regexp.Regexp, data, name, prefix string) []string {
	if data == "" {
		return nil
	}
	binary := strings.IndexByte(data, 0) >= 0
	data = strings.TrimSuffix(data, "\n")
	var out []string
	suppressed := false
	for _, l := range strings.Split(data, "\n") {
		if !re.MatchString(l) {
			continue
		}
		if binary || !utf8.ValidString(l) {
			suppressed = true
			continue
		}
		out = append(out, prefix+l)
	}
	if suppressed {
		write(stderr, "grep: "+name+": binary file matches\n")
	}
	return out
}

// grepReason is the strerror text of a file grep could not read.
func grepReason(err error) string {
	var errno syscall.Errno
	if errors.As(err, &errno) {
		switch errno {
		case syscall.ENOENT:
			return "No such file or directory"
		case syscall.EISDIR:
			return "Is a directory"
		case syscall.EACCES:
			return "Permission denied"
		case syscall.ENOTDIR:
			return "Not a directory"
		}
		return errno.Error()
	}
	if errors.Is(err, fs.ErrNotExist) {
		return "No such file or directory"
	}
	return err.Error()
}

// readAll reads a whole stream; nil is empty.
func readAll(r io.Reader) (string, error) {
	if r == nil {
		return "", nil
	}
	b, err := io.ReadAll(r)
	return string(b), err
}
