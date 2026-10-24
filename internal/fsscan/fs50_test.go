// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package fsscan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLedger_FS50 pins the forced divergence: text taken from a scanned file
// name never reaches an evaluator. The name below puts a tab and a Bash
// array subscript holding a command substitution into the extension; 2.x
// evaluated that subscript in the rich renderer's arithmetic. The payload
// only creates an empty marker file, so the test is harmless on any build.
func TestLedger_FS50(t *testing.T) {
	fx := t.TempDir()
	cwd := t.TempDir()
	t.Chdir(cwd)
	mustWrite(t, filepath.Join(fx, "f.x\ta[$(touch MARKER)]"), "z")
	mustWrite(t, filepath.Join(fx, "ok.txt"), "zz")
	for _, run := range []struct {
		name string
		do   func() (string, int)
	}{
		{"rich", func() (string, int) { return runGoRich("tree_stats", t.TempDir(), cwd, fx) }},
		{"plain", func() (string, int) { o, _, rc := runGo("tree_stats", t.TempDir(), cwd, fx); return o, rc }},
		{"json", func() (string, int) { o, _, rc := runGo("tree_stats", t.TempDir(), cwd, "--json", fx); return o, rc }},
	} {
		out, rc := run.do()
		if rc != 0 {
			t.Errorf("%s: rc=%d out=%q", run.name, rc, out)
		}
		for _, dir := range []string{cwd, fx} {
			if _, err := os.Lstat(filepath.Join(dir, "MARKER")); err == nil {
				t.Fatalf("%s: the payload in a file name ran a command", run.name)
			}
		}
		if !strings.Contains(out, "txt") {
			t.Errorf("%s: the ordinary extension row is missing: %q", run.name, out)
		}
		// 3.0: FS-01 and FS-17 fixed: the tab name is left out, so no row
		// carries any part of it.
		if strings.Contains(out, "touch") || strings.Contains(out, "MARKER") {
			t.Errorf("%s: a row carries the file name's text: %q", run.name, out)
		}
	}
}
