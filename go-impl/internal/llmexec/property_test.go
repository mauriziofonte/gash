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
