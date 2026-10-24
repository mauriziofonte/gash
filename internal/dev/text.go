// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package dev

import (
	"bufio"
	"io"
	"os"
	"regexp"
	"strings"
	"unicode/utf8"
)

// The text tools of 2.x (head -1, tr -d, grep -oE, grep "^X\.") evaluated
// natively with GNU behavior.

// grepBinaryMsg is what GNU grep 3.5 and later prints on stderr, instead of
// the matching lines, when its input holds a NUL byte or, in a UTF-8
// locale, invalid UTF-8.
const grepBinaryMsg = "grep: (standard input): binary file matches\n"

// grepper is one grep run on piped text: utf8 is the caller's locale.
type grepper struct {
	utf8   bool
	stderr io.Writer
}

// binary is GNU grep's test of its input.
func (g grepper) binary(in string) bool {
	return strings.IndexByte(in, 0) >= 0 || (g.utf8 && !utf8.ValidString(in))
}

// lines splits text as grep reads it: newline-terminated lines, the last
// one without its newline.
func lines(in string) []string {
	if in == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(in, "\n"), "\n")
}

// only is `grep -oE RE` on in: every non-empty match of every line, in
// order.
func (g grepper) only(in string, re *regexp.Regexp) []string {
	var out []string
	for _, l := range lines(in) {
		for _, m := range re.FindAllString(l, -1) {
			if m != "" {
				out = append(out, m)
			}
		}
	}
	if out != nil && g.binary(in) {
		_, _ = io.WriteString(g.stderr, grepBinaryMsg)
		return nil
	}
	return out
}

// match is `grep RE` on in: the lines that match.
func (g grepper) match(in string, re *regexp.Regexp) []string {
	var out []string
	for _, l := range lines(in) {
		if re.MatchString(l) {
			out = append(out, l)
		}
	}
	if out != nil && g.binary(in) {
		_, _ = io.WriteString(g.stderr, grepBinaryMsg)
		return nil
	}
	return out
}

// headLine is `head -1 FILE`: the first line with its newline; "" when the
// file cannot be read.
func headLine(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	l, _ := bufio.NewReader(f).ReadString('\n')
	return l
}

// trimSpaceAll is `tr -d '[:space:]'`: every ASCII white-space byte
// removed (tr works on bytes).
func trimSpaceAll(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '\n', '\v', '\f', '\r':
			return -1
		}
		return r
	}, s)
}

// subst is a command substitution: NUL bytes dropped (Bash warns, rc.A
// does not; LEDGER AI-20 is the same divergence) and trailing newlines
// removed.
func subst(s string) string {
	return strings.TrimRight(strings.ReplaceAll(s, "\x00", ""), "\n")
}

// breLine is the regexp of the basic regular expression `^TEXT\.`, where
// TEXT is a major version taken from a directory name: . and * keep
// their meaning, a bracket expression passes through, a backslash escapes
// the next character, and everything else is literal. ok is false for a
// pattern grep refuses (an unmatched [).
func breLine(text string) (*regexp.Regexp, bool) {
	var b strings.Builder
	b.WriteString(`^`)
	for i := 0; i < len(text); i++ {
		c := text[i]
		switch {
		case c == '.':
			b.WriteString(`.`)
		case c == '*':
			if i == 0 {
				b.WriteString(`\*`)
			} else {
				b.WriteString(`*`)
			}
		case c == '\\' && i+1 < len(text):
			i++
			b.WriteString(regexp.QuoteMeta(text[i : i+1]))
		case c == '[':
			j := i + 1
			if j < len(text) && text[j] == '^' {
				j++
			}
			if j < len(text) && text[j] == ']' {
				j++
			}
			for j < len(text) && text[j] != ']' {
				j++
			}
			if j >= len(text) {
				return nil, false
			}
			b.WriteString(text[i : j+1])
			i = j
		default:
			b.WriteString(regexp.QuoteMeta(text[i : i+1]))
		}
	}
	b.WriteString(`\.`)
	re, err := regexp.Compile(b.String())
	return re, err == nil
}
