// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestScriptSignalsOnlyItsOwnPIDs is the LEDGER TEST-04 rule: no script may
// name a PID it did not create. The binary signals with a real kill(2), so a
// pgrep, lsof, pidof or fuser stub that prints a literal number would have
// the binary signal whatever process holds that PID. Stubs must print the
// PID of a victim the script started (sleep 300 & vic=$!).
func TestScriptSignalsOnlyItsOwnPIDs(t *testing.T) {
	files, err := filepath.Glob("../../testdata/script/*.txtar")
	if err != nil {
		t.Fatal(err)
	}
	more, err := filepath.Glob("../../testdata/script/*/*.txtar")
	if err != nil {
		t.Fatal(err)
	}
	files = append(files, more...)
	if len(files) < 50 {
		t.Fatalf("found only %d scripts", len(files))
	}
	stub := regexp.MustCompile(`(?:>|/)\s*"?[^ ]*\b(pgrep|pidof|lsof|fuser)\b`)
	literal := regexp.MustCompile(`(?:\\n|\b)echo\s+(?:\\?")?[0-9]{2,}\b`)
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(data), "\n") {
			if stub.MatchString(line) && literal.MatchString(line) {
				t.Errorf("%s:%d: a %s stub prints a fixed PID; report a victim the script started instead (TEST-04)",
					filepath.Base(f), i+1, stub.FindStringSubmatch(line)[1])
			}
		}
	}
}
