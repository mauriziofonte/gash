// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llmexec

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// The one-parser-decides property (TestRenderRoundTrip) over any line: a
// line the checker accepts renders to a script in which every word of
// every command is one single-quoted literal, so bash expands nothing, and
// an independent shell parser reads back exactly the words that were
// validated. Build never panics, in either mode.
func FuzzRenderRoundTrip(f *testing.F) {
	for _, s := range []string{
		"echo hello", "sort list.txt | head -n 1", `echo 'a b' "c d" e\;f`, "echo one\necho two # comment",
		"ls nothing 2>/dev/null || echo missing", "cat *.log", "grep -n x ./*.log", "printf '%s-%s' a b",
		"echo a \\\n b", "echo \"a\\\nb\" c", "ls |\nwc -l", "echo $(id)", "echo `id`", "echo ${HOME}",
		"cat <(ls)", "ls > out", "echo a >&2", "find . -exec rm {} \\;", "git log --output=x",
		"echo ~root", "cat ../x", "cat /etc/shadow", "sed -n 1w/tmp/x in.txt", "awk 'BEGIN{system(\"id\")}'",
		"echo $'\\x41'", "ls &", "echo a; rm -rf /", "env X=1 ls", "X=1 ls", "echo {a,b}", "ls ?", "echo [ab]",
	} {
		f.Add(s)
	}
	cwd := f.TempDir()
	for _, n := range []string{"a.log", "b.log", "notes.txt", "list.txt"} {
		mustWriteF(f, filepath.Join(cwd, n), "x\n")
	}
	f.Fuzz(func(t *testing.T, line string) {
		for _, mode := range []string{"", "unrestricted"} {
			_, _ = Build(line, Settings{Exec: mode}, testEnv(cwd), permissiveProbe())
		}
		lc := newLineChecker(testEnv(cwd), permissiveProbe(), map[string]bool{})
		script, err := lc.checkAndRender(line)
		if err != nil {
			return
		}
		file, perr := syntax.NewParser().Parse(strings.NewReader(script), "")
		if perr != nil {
			// A line bash rejects stays rejected (TestRenderRoundTrip_SyntaxErrors).
			return
		}
		var got [][]string
		syntax.Walk(file, func(node syntax.Node) bool {
			switch n := node.(type) {
			case *syntax.CallExpr:
				if len(n.Assigns) > 0 {
					t.Fatalf("%q: rendered an assignment: %q", line, script)
				}
				words := make([]string, 0, len(n.Args))
				for _, a := range n.Args {
					w, ok := quotedLiteral(a)
					if !ok {
						t.Fatalf("%q: word %d of the script %q is not a quoted literal", line, len(words), script)
					}
					words = append(words, w)
				}
				got = append(got, words)
			case *syntax.CmdSubst, *syntax.ProcSubst, *syntax.ArithmExp, *syntax.ParamExp:
				t.Fatalf("%q: the script %q holds an expansion", line, script)
			}
			return true
		})
		// The oracle parser drops a CR before an LF even inside quotes,
		// where bash keeps it; the script carries the CR (bash reads the
		// single-quoted word as it is), so compare without it.
		want := make([][]string, len(lc.cmdWords))
		for i, ws := range lc.cmdWords {
			for _, w := range ws {
				want[i] = append(want[i], strings.ReplaceAll(w, "\r\n", "\n"))
			}
		}
		if !reflect.DeepEqual(got, want) && !(len(got) == 0 && len(want) == 0) {
			t.Fatalf("%q:\n  validated = %q\n  re-parsed = %q\n  script = %q", line, lc.cmdWords, got, script)
		}
	})
}

// quotedLiteral is the value of a rendered word: single-quoted segments
// joined by escaped quotes (\'), the only shape the renderer emits and one
// in which bash expands nothing.
func quotedLiteral(w *syntax.Word) (string, bool) {
	var b strings.Builder
	for _, p := range w.Parts {
		switch p := p.(type) {
		case *syntax.SglQuoted:
			if p.Dollar {
				return "", false
			}
			b.WriteString(p.Value)
		case *syntax.Lit:
			if strings.Trim(p.Value, "\\'") != "" || strings.Count(p.Value, "\\'")*2 != len(p.Value) {
				return "", false
			}
			b.WriteString(strings.Repeat("'", len(p.Value)/2))
		default:
			return "", false
		}
	}
	return b.String(), len(w.Parts) > 0
}

func mustWriteF(f *testing.F, path, content string) {
	f.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		f.Fatal(err)
	}
}

// 3.1: a NUL byte is refused, not rendered into a word bash would cut.
func TestRefuse_NUL(t *testing.T) {
	_, err := Build("printf 'a\x00b'", Settings{}, testEnv(t.TempDir()), permissiveProbe())
	if errCode(err) != "construct_not_allowed" {
		t.Fatalf("got %v, want construct_not_allowed", err)
	}
}
