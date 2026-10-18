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
func TestLedger_EXEC07(t *testing.T) {
	cwd := t.TempDir()
	mustWrite(t, filepath.Join(cwd, "x"), "validated\n")
	mustWrite(t, filepath.Join(cwd, `x\`), "unchecked\n")

	// The defect: 2.x checked x, and bash printed x\ instead.
	if out, exit := run2x(t, `cat x\`, cwd, cwd); out != "unchecked\n" || exit != 0 {
		t.Errorf("2.x cat x\\: got %q (exit %d), want the unchecked file", out, exit)
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
	for _, line := range []string{
		`rm x \`,
		`cat .env \`,
		`echo $HOME \`,
		`echo "a\`,
		`echo 'a\`,
	} {
		_, err := build(t, line, cwd)
		r, ok := err.(*Refusal)
		ref := refusal2x(t, line, cwd)
		if !ok || r.Code != ref.Error || r.Details != ref.Details || r.Action != ref.Action || r.Hint != ref.Hint ||
			ref.Recoverable != (r.Action != "FATAL") {
			t.Errorf("%q:\n  mine = %+v\n  2.x  = %+v", line, err, ref)
		}
	}

	// No trailing backslash: an escaped one, one inside single quotes, one
	// in a comment, a final continuation. These run as in 2.x, byte for byte.
	for _, line := range []string{
		`echo a\\`,
		`echo "a\\"`,
		`echo 'a\'`,
		`echo a # note\`,
		"echo a \\\n",
	} {
		mineOut, mineExit, err := runMineCombined(t, line, cwd, cwd)
		if err != nil {
			t.Errorf("%q: Build refused: %v", line, err)
			continue
		}
		refOut, refExit := run2x(t, line, cwd, cwd)
		if mineOut != refOut || mineExit != refExit {
			t.Errorf("%q:\n  mine = %q (exit %d)\n  2.x  = %q (exit %d)", line, mineOut, mineExit, refOut, refExit)
		}
	}
}

// TestLedger_EXEC08 (defect, reproduced): the 2.x parser reads a pipe, a
// line continuation and a pipe as two pipes around an empty command, while
// bash drops the continuation first and reads ||. The words are checked
// either way; what runs is the || list. The canonical script keeps the
// continuation, so bash runs exactly what 2.x ran.
func TestLedger_EXEC08(t *testing.T) {
	cwd := t.TempDir()
	mustWrite(t, filepath.Join(cwd, "list.txt"), "b\na\n")

	lc := newLineChecker(testEnv(cwd), permissiveProbe(), map[string]bool{})
	script, err := lc.checkAndRender("echo a |\\\n| echo b")
	if err != nil {
		t.Fatal(err)
	}
	if want := "'echo' 'a' |\\\n| 'echo' 'b'"; script != want {
		t.Errorf("script = %q, want %q", script, want)
	}
	// Both commands were parsed and checked, as two simple commands.
	if len(lc.cmdWords) != 2 {
		t.Errorf("checked commands = %v, want 2", lc.cmdWords)
	}

	for _, tc := range []struct {
		line, out string
		exit      int
	}{
		// A pipeline would print b; the || list prints a.
		{"echo a |\\\n| echo b", "a\n", 0},
		{"false |\\\n| echo b", "b\n", 0},
		{"ls |\\\n| wc -l", "list.txt\n", 0},
	} {
		mineOut, mineExit, err := runMineCombined(t, tc.line, cwd, cwd)
		if err != nil {
			t.Errorf("%q: Build refused: %v", tc.line, err)
			continue
		}
		refOut, refExit := run2x(t, tc.line, cwd, cwd)
		if mineOut != refOut || mineExit != refExit || mineOut != tc.out || mineExit != tc.exit {
			t.Errorf("%q:\n  mine = %q (exit %d)\n  2.x  = %q (exit %d)\n  want = %q (exit %d)",
				tc.line, mineOut, mineExit, refOut, refExit, tc.out, tc.exit)
		}
	}
}

// TestLedger_EXEC04_2x (decision, kept): the secret-file check runs on the
// first positional before it is skipped as a grep/rg pattern, so a pattern
// that looks like a secret file is refused, while one that looks like a
// protected path is searched for, as in 2.x.
func TestLedger_EXEC04_2x(t *testing.T) {
	cwd := t.TempDir()
	mustWrite(t, filepath.Join(cwd, "notes.txt"), "see /etc/shadow\nand .env\n")
	for _, line := range []string{"grep .env notes.txt", "grep id_rsa notes.txt", "rg -n .env notes.txt"} {
		want := refusal2x(t, line, cwd)
		_, err := build(t, line, cwd)
		r, ok := err.(*Refusal)
		if !ok || r.Code != want.Error || r.Details != want.Details || r.Action != want.Action || r.Hint != want.Hint {
			t.Errorf("%q: got %v, want %+v", line, err, want)
		}
		if want.Error != "secret_file_blocked" {
			t.Errorf("%q: 2.x refused with %q, want secret_file_blocked", line, want.Error)
		}
	}
	for _, line := range []string{"grep /etc/shadow notes.txt", "grep -c /etc/passwd notes.txt"} {
		want, wantExit := run2x(t, line, cwd, cwd)
		plan, err := build(t, line, cwd)
		if err != nil {
			t.Fatalf("%q: refused: %v", line, err)
		}
		got, exit := runPlan(t, plan, cwd)
		if got != want || exit != wantExit {
			t.Errorf("%q: got %q (exit %d), want %q (exit %d)", line, got, exit, want, wantExit)
		}
	}
}

// TestLedger_EXEC06 (forced divergence): globs expand in byte order
// whatever the caller's collation; in the C locale that is the 2.x order.
func TestLedger_EXEC06(t *testing.T) {
	cwd := t.TempDir()
	for _, n := range []string{"b", "B", "a", "_u", "Z1", "é", "e", "a b"} {
		mustWrite(t, filepath.Join(cwd, n), "")
	}
	want := "B Z1 _u a a b b e é\n"
	if got, exit := run2x(t, "echo *", cwd, cwd); got != want || exit != 0 {
		t.Fatalf("2.x in C: got %q (exit %d), want %q", got, exit, want)
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
