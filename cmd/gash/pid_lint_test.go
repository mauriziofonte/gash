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

// TestScriptFakesProgramsOnPATH is the LEDGER TEST-05 rule: a script fakes
// an external program with a stub on PATH (or httpsfake for the network),
// never with a shell function, which only the 2.x Bash implementation would
// call: the binary runs the real program and would reach the network or the
// real tool. Shell-side names (history, fc, nvm, needs_confirm_prompt) are
// shell functions in the caller's shell and stay function overrides.
func TestScriptFakesProgramsOnPATH(t *testing.T) {
	files, err := filepath.Glob("../../testdata/script/*.txtar")
	if err != nil {
		t.Fatal(err)
	}
	more, err := filepath.Glob("../../testdata/script/*/*.txtar")
	if err != nil {
		t.Fatal(err)
	}
	files = append(files, more...)
	fake := regexp.MustCompile(`(?:^|[\s;{(&|'"])(curl|wget|ssh|scp|rsync|docker|git|lsof|pgrep|pkill|ps|ss|netstat|kill|sudo|` +
		`systemctl|jq|sqlite3|mysql|psql|pg_isready|uname|ip|dig|apt|dpkg|node|npm|php|composer|tar|unzip|gzip|7z|unrar|` +
		`zstd|xz|sed|awk|gawk|grep|find|du|df|free|whoami|id|hostname|openssl|gpg)\s*\(\)\s*\{`)
	// Known cases owned by another slice, reported to the lead: the :99
	// render case of files_largest fakes find, which the binary's native
	// walk never calls (the case passes without exercising a find error).
	known := map[string]bool{"files_largest.txtar:find": true}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(data), "\n") {
			if m := fake.FindStringSubmatch(line); m != nil && !known[filepath.Base(f)+":"+m[1]] {
				t.Errorf("%s:%d: %s is faked with a shell function; put a stub on PATH instead (TEST-05)", filepath.Base(f), i+1, m[1])
			}
		}
	}
}
