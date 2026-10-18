// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llmexec

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// TestRenderRoundTrip is the one-parser-decides property (MUST NOT 7): the
// canonical script, re-parsed by an independent shell parser
// (mvdan.cc/sh/v3/syntax), yields exactly the words Gash validated. If bash
// could find anything else to expand in the rendered script, this would fail.
func TestRenderRoundTrip(t *testing.T) {
	cwd := t.TempDir()
	// Fixtures for the glob lines.
	for _, n := range []string{"a.log", "b.log", "notes.txt"} {
		mustWrite(t, filepath.Join(cwd, n), "x\n")
	}
	mustWrite(t, filepath.Join(cwd, "list.txt"), "b\na\n")

	lines := []string{
		"echo hello",
		"sort list.txt | head -n 1",
		`echo 'a b' "c d" e\;f`,
		"echo one\necho two # comment",
		"echo err >&2",
		"ls nothing 2>/dev/null || echo missing",
		"cat /dev/null && echo ok",
		"echo '*'",
		"cat *.log",
		"grep -n x ./*.log",
		"echo /proc/x",
		"printf '%s-%s' a b",
		// Redirections stay in place, also in a command with no words.
		"2>/dev/null; echo a",
		"echo a 2>/dev/null b",
		"ls | 2>/dev/null",
		// Line continuations and comments between and inside words.
		"echo a \\\n b",
		"echo a\\\nb c",
		"echo \"a\\\nb\" c",
		// Not here: "ls |\\\n| wc -l" (EXEC-08). Bash drops a line
		// continuation before it reads tokens, so |\<newline>| is || to
		// bash (and was in 2.x); this parser keeps the two | apart.
		// TestLedger_EXEC08 runs such lines against bash and 2.x instead.
		"echo a # note\necho b",
		// Operators across newlines.
		"ls |\nwc -l",
		"ls &&\necho b",
		"echo a;\n",
	}
	for _, line := range lines {
		lc := newLineChecker(testEnv(cwd), permissiveProbe(), map[string]bool{})
		script, err := lc.checkAndRender(line)
		if err != nil {
			t.Errorf("%q: validate/render refused: %v", line, err)
			continue
		}
		got, err := reparseWords(script)
		if err != nil {
			t.Errorf("%q: re-parse failed: %v (script=%q)", line, err, script)
			continue
		}
		if !reflect.DeepEqual(got, lc.cmdWords) {
			t.Errorf("%q:\n  validated = %v\n  re-parsed = %v\n  script = %q", line, lc.cmdWords, got, script)
		}
	}
}

// TestRenderRoundTrip_SyntaxErrors: a line 2.x handed to bash with a
// dangling operator or an empty command between operators renders to a script
// the independent parser rejects too, so bash rejects it as it rejected the
// raw line and runs none of its words.
func TestRenderRoundTrip_SyntaxErrors(t *testing.T) {
	cwd := t.TempDir()
	for _, line := range []string{
		"ls |", "ls &&", "ls ||", "echo a |  ", "ls 2>/dev/null |",
		"| ls", "&& ls", "; ls", "ls ;; ls", "ls | | ls", "ls ; ; ls", "ls\n;",
	} {
		lc := newLineChecker(testEnv(cwd), permissiveProbe(), map[string]bool{})
		script, err := lc.checkAndRender(line)
		if err != nil {
			t.Errorf("%q: validate/render refused: %v", line, err)
			continue
		}
		if _, err := syntax.NewParser().Parse(strings.NewReader(script), ""); err == nil {
			t.Errorf("%q: script %q parses, want a syntax error", line, script)
		}
	}
}

// reparseWords parses the canonical script with mvdan's shell parser and
// returns the literal words of each simple command, in source order.
func reparseWords(script string) ([][]string, error) {
	f, err := syntax.NewParser().Parse(strings.NewReader(script), "")
	if err != nil {
		return nil, err
	}
	var out [][]string
	syntax.Walk(f, func(node syntax.Node) bool {
		if ce, ok := node.(*syntax.CallExpr); ok {
			if len(ce.Assigns) > 0 {
				return true
			}
			words := make([]string, 0, len(ce.Args))
			for _, a := range ce.Args {
				words = append(words, wordLiteral(a))
			}
			out = append(out, words)
		}
		return true
	})
	return out, nil
}

// wordLiteral returns the literal value of a word that is a single
// single-quoted segment (every rendered word is exactly that).
func wordLiteral(w *syntax.Word) string {
	if len(w.Parts) == 1 {
		if sq, ok := w.Parts[0].(*syntax.SglQuoted); ok {
			return sq.Value
		}
	}
	// Fallback: print it (should not happen for a canonical word).
	var b strings.Builder
	_ = syntax.NewPrinter().Print(&b, w)
	return b.String()
}
