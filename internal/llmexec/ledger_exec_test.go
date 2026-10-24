// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llmexec

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// envelope2x is the 2.x JSON error line.
type envelope2x struct {
	Error       string `json:"error"`
	Details     string `json:"details"`
	Action      string `json:"action"`
	Recoverable bool   `json:"recoverable"`
	Hint        string `json:"hint"`
}

// refusal2x runs line through 2.x llm_exec and decodes the error it printed.
func refusal2x(t *testing.T, line, cwd string) envelope2x {
	t.Helper()
	out, exit := run2x(t, line, cwd, cwd)
	var e envelope2x
	if exit != 1 || json.Unmarshal([]byte(strings.TrimSpace(out)), &e) != nil {
		t.Fatalf("%q: 2.x did not refuse: %q (exit %d)", line, out, exit)
	}
	return e
}

// TestLedger_EXEC07 (forced divergence): the 2.x parser dropped a backslash
// at the end of the input, but bash keeps it as a literal character, so 2.x
// checked one word and bash ran another. rc.A refuses such a line with
// construct_not_allowed, value \, RETRY (recoverable), and runs nothing.
// The 2.x reference is compared only when its tree is there; the 3.0
// verdicts are pinned either way.
func TestLedger_EXEC07(t *testing.T) {
	cwd := t.TempDir()
	mustWrite(t, filepath.Join(cwd, "x"), "validated\n")
	mustWrite(t, filepath.Join(cwd, `x\`), "unchecked\n")
	with2x := have2x()

	// The defect: 2.x checked x, and bash printed x\ instead.
	if with2x {
		if out, exit := run2x(t, `cat x\`, cwd, cwd); out != "unchecked\n" || exit != 0 {
			t.Errorf("2.x cat x\\: got %q (exit %d), want the unchecked file", out, exit)
		}
	}

	want := Refusal{Code: "construct_not_allowed", Details: `\`, Action: "RETRY",
		Hint: "Remove the trailing backslash or quote it"}
	for _, line := range []string{
		`cat x\`,
		`\`,
		`echo a \`,
		`echo 'a'\`,
		`echo "a"\`,
		"echo a \\\n\\",        // after a line continuation
		"echo \"a\\\nb\"\\",    // after a continuation inside double quotes
		"ls\n\\",               // alone on the last line
		`ls | wc \`,            // in the last command of a pipeline
		`ls; echo a\`,          // in the last of two commands
		`echo a 2>/dev/null \`, // after a redirection
	} {
		plan, err := build(t, line, cwd)
		r, ok := err.(*Refusal)
		if plan != nil || !ok || *r != want {
			t.Errorf("%q: got plan=%v err=%v, want %+v and no plan", line, plan != nil, err, want)
		}
	}

	// A line 2.x refused for another reason keeps that refusal: the
	// trailing backslash is checked once the rest of the line has passed.
	// Inside quotes a final backslash leaves the quote open, as in 2.x.
	for line, code := range map[string]string{
		`rm x \`:       "command_not_allowed",
		`cat .env \`:   "secret_file_blocked",
		`echo $HOME \`: "construct_not_allowed",
		`echo "a\`:     "unterminated_quote",
		`echo 'a\`:     "unterminated_quote",
	} {
		_, err := build(t, line, cwd)
		r, ok := err.(*Refusal)
		if !ok || r.Code != code {
			t.Errorf("%q: got %v, want %s", line, err, code)
			continue
		}
		if !with2x {
			continue
		}
		ref := refusal2x(t, line, cwd)
		if r.Code != ref.Error || r.Details != ref.Details || r.Action != ref.Action || r.Hint != ref.Hint ||
			ref.Recoverable != (r.Action != "FATAL") {
			t.Errorf("%q:\n  mine = %+v\n  2.x  = %+v", line, err, ref)
		}
	}

	// No trailing backslash: an escaped one, one inside single quotes, one
	// in a comment, a final continuation. These run as in 2.x, byte for byte.
	for _, tc := range []struct{ line, out string }{
		{`echo a\\`, "a\\\n"},
		{`echo "a\\"`, "a\\\n"},
		{`echo 'a\'`, "a\\\n"},
		{`echo a # note\`, "a\n"},
		{"echo a \\\n", "a\n"},
	} {
		mineOut, mineExit, err := runMineCombined(t, tc.line, cwd, cwd)
		if err != nil {
			t.Errorf("%q: Build refused: %v", tc.line, err)
			continue
		}
		if mineOut != tc.out || mineExit != 0 {
			t.Errorf("%q: got %q (exit %d), want %q", tc.line, mineOut, mineExit, tc.out)
		}
		if with2x {
			refOut, refExit := run2x(t, tc.line, cwd, cwd)
			if mineOut != refOut || mineExit != refExit {
				t.Errorf("%q:\n  mine = %q (exit %d)\n  2.x  = %q (exit %d)", tc.line, mineOut, mineExit, refOut, refExit)
			}
		}
	}
}

// TestLedger_EXEC08: the 2.x parser read a pipe, a line continuation and a
// pipe as two pipes around an empty command, while bash drops the
// continuation first and reads ||. 3.0: EXEC-08 fixed, the parser drops
// the continuation before it reads the operator, so the checked operator is
// the one bash runs; the script renders || whole with the continuation
// after it. What runs is what 2.x ran (bash read || there too).
func TestLedger_EXEC08(t *testing.T) {
	cwd := t.TempDir()
	mustWrite(t, filepath.Join(cwd, "list.txt"), "b\na\n")

	for line, want := range map[string]string{
		"echo a |\\\n| echo b":     "'echo' 'a' ||\\\n 'echo' 'b'",
		"echo a |\\\n\\\n| echo b": "'echo' 'a' ||\\\n\\\n 'echo' 'b'",
		"echo a &\\\n& echo b":     "'echo' 'a' &&\\\n 'echo' 'b'",
		"echo a |\\\n wc -l":       "'echo' 'a' |\\\n 'wc' '-l'",
	} {
		lc := newLineChecker(testEnv(cwd), permissiveProbe(), map[string]bool{})
		script, err := lc.checkAndRender(line)
		if err != nil {
			t.Fatalf("%q: %v", line, err)
		}
		if script != want {
			t.Errorf("%q: script = %q, want %q", line, script, want)
		}
		// Both commands were parsed and checked, as two simple commands.
		if len(lc.cmdWords) != 2 {
			t.Errorf("%q: checked commands = %v, want 2", line, lc.cmdWords)
		}
	}
	// A continuation inside |& is still |&, refused as 2.x refused |&.
	if _, err := build(t, "echo a |\\\n& echo b", cwd); errCode(err) != "construct_not_allowed" {
		t.Errorf("|\\<newline>&: got %v, want construct_not_allowed", err)
	}

	with2x := have2x()
	for _, tc := range []struct {
		line, out string
		exit      int
	}{
		// A pipeline would print b; the || list prints a.
		{"echo a |\\\n| echo b", "a\n", 0},
		{"false |\\\n| echo b", "b\n", 0},
		{"ls |\\\n| wc -l", "list.txt\n", 0},
		{"true &\\\n& echo b", "b\n", 0},
	} {
		mineOut, mineExit, err := runMineCombined(t, tc.line, cwd, cwd)
		if err != nil {
			t.Errorf("%q: Build refused: %v", tc.line, err)
			continue
		}
		if mineOut != tc.out || mineExit != tc.exit {
			t.Errorf("%q: got %q (exit %d), want %q (exit %d)", tc.line, mineOut, mineExit, tc.out, tc.exit)
		}
		if with2x && !strings.Contains(tc.line, "&") {
			refOut, refExit := run2x(t, tc.line, cwd, cwd)
			if mineOut != refOut || mineExit != refExit {
				t.Errorf("%q:\n  mine = %q (exit %d)\n  2.x  = %q (exit %d)", tc.line, mineOut, mineExit, refOut, refExit)
			}
		}
	}
}

// TestLedger_EXEC04 (decision, kept): the secret-file check runs on the
// first positional before it is skipped as a grep/rg pattern, so a pattern
// that looks like a secret file is refused, while one that looks like a
// protected path is searched for, as in 2.x (operator 2026-10-24: keep).
func TestLedger_EXEC04(t *testing.T) {
	cwd := t.TempDir()
	mustWrite(t, filepath.Join(cwd, "notes.txt"), "see /etc/shadow\nand .env\n")
	with2x := have2x()
	for line, details := range map[string]string{
		"grep .env notes.txt": ".env", "grep id_rsa notes.txt": "id_rsa", "rg -n .env notes.txt": ".env",
	} {
		_, err := build(t, line, cwd)
		r, ok := err.(*Refusal)
		if !ok || r.Code != "secret_file_blocked" || r.Details != details || r.Action != "FATAL" {
			t.Errorf("%q: got %v, want secret_file_blocked %s", line, err, details)
			continue
		}
		if with2x {
			want := refusal2x(t, line, cwd)
			if r.Code != want.Error || r.Details != want.Details || r.Action != want.Action || r.Hint != want.Hint {
				t.Errorf("%q: got %v, want %+v", line, err, want)
			}
		}
	}
	for line, want := range map[string]string{
		"grep /etc/shadow notes.txt":    "see /etc/shadow\n",
		"grep -c /etc/passwd notes.txt": "0\n",
	} {
		plan, err := build(t, line, cwd)
		if err != nil {
			t.Fatalf("%q: refused: %v", line, err)
		}
		got, exit := runPlan(t, plan, cwd)
		wantExit := 0
		if want == "0\n" {
			wantExit = 1
		}
		if got != want || exit != wantExit {
			t.Errorf("%q: got %q (exit %d), want %q (exit %d)", line, got, exit, want, wantExit)
		}
		if with2x {
			ref, refExit := run2x(t, line, cwd, cwd)
			if got != ref || exit != refExit {
				t.Errorf("%q: got %q (exit %d), 2.x %q (exit %d)", line, got, exit, ref, refExit)
			}
		}
	}
}

// TestLedger_EXEC06 (forced divergence): globs expand in byte order
// whatever the caller's collation; in the C locale that is the 2.x order.
func TestLedger_EXEC06(t *testing.T) {
	cwd := t.TempDir()
	// No two names differ only in case: a macOS runner's file system is
	// case-insensitive.
	for _, n := range []string{"c", "B", "a", "_u", "Z1", "é", "e", "a b"} {
		mustWrite(t, filepath.Join(cwd, n), "")
	}
	want := "B Z1 _u a a b c e é\n"
	if have2x() {
		if got, exit := run2x(t, "echo *", cwd, cwd); got != want || exit != 0 {
			t.Fatalf("2.x in C: got %q (exit %d), want %q", got, exit, want)
		}
	}
	for _, vars := range []map[string]string{
		nil,
		{"LC_ALL": "C.UTF-8"},
		{"LC_COLLATE": "en_US.UTF-8", "LANG": "de_DE.UTF-8"},
	} {
		env := testEnv(cwd)
		for k, v := range vars {
			env.Vars[k] = v
		}
		plan, err := Build("echo *", Settings{}, env, permissiveProbe())
		if err != nil {
			t.Fatalf("%v: %v", vars, err)
		}
		if got, exit := runPlan(t, plan, cwd); got != want || exit != 0 {
			t.Errorf("%v: got %q (exit %d), want %q", vars, got, exit, want)
		}
	}
}
