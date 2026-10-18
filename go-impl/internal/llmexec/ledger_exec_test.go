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
