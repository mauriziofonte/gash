// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package dev

import (
	"bufio"
	"os"
	"strings"
)

// The text tools of 2.x (head -1, tr -d, grep -oE, grep "^X\.") evaluated
// natively with GNU behavior.

// lines splits text as grep reads it: newline-terminated lines, the last
// one without its newline.
func lines(in string) []string {
	if in == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(in, "\n"), "\n")
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
