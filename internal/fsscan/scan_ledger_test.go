// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package fsscan

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/term"
)

// handlers maps a scanner name to its handler for the in-process tests.
var scanHandlers = map[string]func(*invoke.Call) int{
	"files_largest":   filesLargest,
	"dirs_largest":    dirsLargest,
	"dirs_find_large": dirsFindLarge,
	"dirs_list_empty": dirsListEmpty,
	"tree_stats":      treeStats,
}

// runGo invokes a scanner in-process with a non-terminal style (plain/JSON)
// and returns its stdout, stderr and exit status.
func runGo(fn, home, pwd string, args ...string) (string, string, int) {
	e := invoke.EnvMap(map[string]string{"HOME": home, "PWD": pwd, "GASH__HELP": "0"})
	var out, errb bytes.Buffer
	c := &invoke.Call{
		Ctx: context.Background(), Name: fn, Args: args, Env: e,
		Stdout: &out, Stderr: &errb, Term: term.Fixed(e, false, false, false),
	}
	rc := scanHandlers[fn](c)
	return out.String(), errb.String(), rc
}

var scanTimeRe = regexp.MustCompile(`"scan_time":"[^"]*"`)

// topArrRe matches a tree_stats top_by_count / top_by_size JSON array so its
// element order can be canonicalized before a differential comparison. The
// objects inside hold no literal ']' for the scanner's own output.
var topArrRe = regexp.MustCompile(`("top_by_(?:count|size)":\[)([^\]]*)(\])`)

// canonTops sorts the entries inside each top_by_count / top_by_size array so
// two runs that agree on the entries but not their order compare equal. The
// order of ties in those arrays is a documented forced divergence (FS-28):
// 2.x follows awk's hash iteration order (mawk, gawk-UTF-8 and gawk-C all
// differ), which rc.A does not reproduce. The differential tests still check
// the entries themselves and every other field exactly.
func canonTops(s string) string {
	return topArrRe.ReplaceAllStringFunc(s, func(m string) string {
		g := topArrRe.FindStringSubmatch(m)
		if g[2] == "" {
			return m
		}
		parts := strings.Split(g[2], "},{")
		for i := range parts {
			if !strings.HasPrefix(parts[i], "{") {
				parts[i] = "{" + parts[i]
			}
			if !strings.HasSuffix(parts[i], "}") {
				parts[i] += "}"
			}
		}
		sort.Strings(parts)
		return g[1] + strings.Join(parts, ",") + g[3]
	})
}

func normScan(s, fx string) string {
	s = scanTimeRe.ReplaceAllString(s, `"scan_time":"T"`)
	s = canonTops(s)
	return strings.ReplaceAll(s, fx, "FX")
}

// run2x runs a 2.x scanner by sourcing the checkout's Bash sources, for the
// differential tests. ok is false (the test skips) when bash or the sources
// are unavailable.
func run2x(t *testing.T, fn, pwd string, args ...string) (string, string, int, bool) {
	t.Helper()
	bash, err := exec.LookPath("bash")
	if err != nil {
		return "", "", 0, false
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		return "", "", 0, false
	}
	for _, f := range []string{"lib/core/output.sh", "lib/core/utils.sh", "lib/core/validation.sh", "lib/modules/files.sh"} {
		if _, err := os.Stat(filepath.Join(root, f)); err != nil {
			return "", "", 0, false
		}
	}
	script := `set +e
source "$R/lib/core/output.sh"
source "$R/lib/core/utils.sh"
source "$R/lib/core/validation.sh"
source "$R/lib/modules/files.sh"
"$@"`
	callArgs := append([]string{"-c", script, "bash", fn}, args...)
	cmd := exec.Command(bash, callArgs...)
	cmd.Dir = pwd
	cmd.Env = append(os.Environ(), "R="+root, "GASH_HEADLESS=1", "LC_ALL=C", "GASH_NO_COLOR=1", "PWD="+pwd)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err = cmd.Run()
	rc := 0
	if ee, ok := err.(*exec.ExitError); ok {
		rc = ee.ExitCode()
	} else if err != nil {
		return "", "", 0, false
	}
	return out.String(), errb.String(), rc, true
}

// mkfix builds the shared fixture tree and returns its path.
func mkfix(t *testing.T) string {
	t.Helper()
	fx := t.TempDir()
	mustMkdir(t, fx, "src", "logs", "empty", "node_modules/pkg", "a/b/c")
	writeN(t, filepath.Join(fx, "logs/app.log"), 524288)
	writeN(t, filepath.Join(fx, "src/logo.png"), 70000)
	mustWrite(t, filepath.Join(fx, "src/app.js"), "xx")
	writeN(t, filepath.Join(fx, "node_modules/pkg/big.js"), 3000)
	writeN(t, filepath.Join(fx, "a/b/c/deep.bin"), 9000)
	return fx
}

func mustMkdir(t *testing.T, base string, rels ...string) {
	t.Helper()
	for _, r := range rels {
		if err := os.MkdirAll(filepath.Join(base, r), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}
func writeN(t *testing.T, path string, n int) {
	t.Helper()
	if err := os.WriteFile(path, make([]byte, n), 0o644); err != nil {
		t.Fatal(err)
	}
}
func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestDifferential2x compares the Go scanners to the live 2.x sources on the
// JSON path, where rc.A reproduces 2.x byte for byte (the tree_stats tops
// order is left to TestLedger_FS28, a forced divergence).
func TestDifferential2x(t *testing.T) {
	fx := mkfix(t)
	home := t.TempDir()
	cases := [][]string{
		{"files_largest", "--json", fx},
		{"files_largest", "--json", "-n", "2", fx},
		{"files_largest", "--json", "--min-size", "50K", fx},
		{"files_largest", "--null", fx},
		{"dirs_largest", "--json", "--depth", "3", fx},
		{"dirs_largest", "--json", "--exclude", "logs", fx},
		{"dirs_find_large", "--json", "--size", "1K", fx},
		{"dirs_find_large", "--json", "--size", "1K", "--with-mtime", fx},
		{"dirs_list_empty", "--json", fx},
		{"dirs_list_empty", "--count", fx},
		{"dirs_list_empty", fx},
		{"files_largest", "--json", "--bogus", fx},
		{"dirs_find_large", "--json", "--size", "nonsense", fx},
		{"files_largest", "--exclude", "--json", fx}, // FS-19
	}
	for _, args := range cases {
		fn, rest := args[0], args[1:]
		o2, e2, rc2, ok := run2x(t, fn, home, rest...)
		if !ok {
			t.Skip("2.x sources or bash unavailable")
		}
		og, eg, rcg := runGo(fn, home, fx, rest...)
		if normScan(og, fx) != normScan(o2, fx) {
			t.Errorf("%v stdout:\n go: %q\n 2x: %q", args, normScan(og, fx), normScan(o2, fx))
		}
		if normScan(eg, fx) != normScan(e2, fx) {
			t.Errorf("%v stderr:\n go: %q\n 2x: %q", args, normScan(eg, fx), normScan(e2, fx))
		}
		if rcg != rc2 {
			t.Errorf("%v rc: go=%d 2x=%d", args, rcg, rc2)
		}
	}
}

// TestLedger_FS20 pins the forced divergence: a newline in an --exclude value
// gets the invalid_filter contract in all five scanners.
func TestLedger_FS20(t *testing.T) {
	fx := mkfix(t)
	want := `{"error":"invalid_filter","details":"a\nb","action":"RETRY","recoverable":true,` +
		`"hint":"--exclude takes a glob without newlines, e.g. '*/node_modules'"}` + "\n"
	for fn := range scanHandlers {
		out, errb, rc := runGo(fn, t.TempDir(), fx, "--json", "--exclude", "a\nb", fx)
		if rc != 1 || out != "" || errb != want {
			t.Errorf("%s: rc=%d out=%q err=%q", fn, rc, out, errb)
		}
	}
}

// TestLedger_FS21 pins the du-depth forced divergence: a leading zero, 08/09
// and values above 2^63-1 are invalid_depth; decimal values scan.
func TestLedger_FS21(t *testing.T) {
	fx := mkfix(t)
	for _, bad := range []string{"010", "08", "09", "9223372036854775808", "99999999999999999999"} {
		for _, fn := range []string{"dirs_largest", "dirs_find_large"} {
			out, errb, rc := runGo(fn, t.TempDir(), fx, "--json", "--depth", bad, fx)
			if rc != 1 || out != "" || !strings.Contains(errb, `"error":"invalid_depth"`) {
				t.Errorf("%s --depth %s: rc=%d out=%q err=%q", fn, bad, rc, out, errb)
			}
		}
	}
	for _, good := range []string{"0", "1", "2147483648"} {
		for _, fn := range []string{"dirs_largest", "dirs_find_large"} {
			out, _, rc := runGo(fn, t.TempDir(), fx, "--json", "--depth", good, fx)
			if rc != 0 || !strings.Contains(out, `"kind":"directories"`) {
				t.Errorf("%s --depth %s should scan: rc=%d out=%q", fn, good, rc, out)
			}
		}
	}
}

// TestLedger_FS22 pins the size overflow forced divergence.
func TestLedger_FS22(t *testing.T) {
	fx := mkfix(t)
	for _, sz := range []string{"8E", "9223372036854775807", "8192P"} {
		_, errb, rc := runGo("dirs_find_large", t.TempDir(), fx, "--json", "--size", sz, fx)
		if rc != 1 || !strings.Contains(errb, `"error":"invalid_size"`) {
			t.Errorf("dirs_find_large --size %s: rc=%d err=%q", sz, rc, errb)
		}
	}
	_, errb, rc := runGo("files_largest", t.TempDir(), fx, "--json", "--min-size", "9999999P", fx)
	if rc != 1 || !strings.Contains(errb, `"error":"invalid_size"`) {
		t.Errorf("files_largest --min-size 9999999P: rc=%d err=%q", rc, errb)
	}
	// A numfmt Big value (8E) is kept by --min-size and filters everything out.
	out, _, rc := runGo("files_largest", t.TempDir(), fx, "--json", "--min-size", "8E", fx)
	if rc != 0 || !strings.Contains(out, `"count":0`) {
		t.Errorf("files_largest --min-size 8E: rc=%d out=%q", rc, out)
	}
}

// TestLedger_FS28 pins the deterministic tie order of the tree_stats tops:
// top_by_count by count desc, size desc, extension asc; top_by_size by size
// desc, count desc, extension asc, zero-size extensions excluded.
func TestLedger_FS28(t *testing.T) {
	fx := t.TempDir()
	// three .a files (counts tie at 3), two .b files, sizes arranged so the
	// tie-breaks are exercised.
	writeN(t, filepath.Join(fx, "1.a"), 10)
	writeN(t, filepath.Join(fx, "2.a"), 10)
	writeN(t, filepath.Join(fx, "3.a"), 10)
	writeN(t, filepath.Join(fx, "1.b"), 100)
	writeN(t, filepath.Join(fx, "2.b"), 100)
	mustWrite(t, filepath.Join(fx, "z.c"), "") // size 0: in count, not in size
	out, _, rc := runGo("tree_stats", t.TempDir(), fx, "--json", fx)
	if rc != 0 {
		t.Fatalf("rc=%d", rc)
	}
	// count: a(3) before b(2) before c(1); size: b(200) before a(30) [c excluded]
	wantCount := `"top_by_count":[{"ext":"a","count":3,"size":30},{"ext":"b","count":2,"size":200},{"ext":"c","count":1,"size":0}]`
	wantSize := `"top_by_size":[{"ext":"b","size":200},{"ext":"a","size":30}]`
	if !strings.Contains(out, wantCount) {
		t.Errorf("top_by_count order:\n got %q\n want substring %q", out, wantCount)
	}
	if !strings.Contains(out, wantSize) {
		t.Errorf("top_by_size order:\n got %q\n want substring %q", out, wantSize)
	}
}

// TestLedger_FS18 pins the one-blank-line of dirs_list_empty text mode when
// nothing matches.
func TestLedger_FS18(t *testing.T) {
	fx := t.TempDir()
	mustWrite(t, filepath.Join(fx, "f"), "x")
	out, _, rc := runGo("dirs_list_empty", t.TempDir(), fx, fx)
	if rc != 0 || out != "\n" {
		t.Errorf("rc=%d out=%q, want one newline", rc, out)
	}
}

// TestLedger_FS17 pins the tab-in-extension reproduction: the extension is
// split into ext, count and size as the 2.x `read` did (invalid JSON kept).
func TestLedger_FS17(t *testing.T) {
	fx := t.TempDir()
	mustWrite(t, filepath.Join(fx, "f.x\ty"), "z")
	out, _, rc := runGo("tree_stats", t.TempDir(), fx, "--json", fx)
	if rc != 0 {
		t.Fatalf("rc=%d", rc)
	}
	if !strings.Contains(out, `{"ext":"x","count":y,"size":1`+"\t"+`1}`) {
		t.Errorf("FS-17 not reproduced in top_by_count: %q", out)
	}
	if !strings.Contains(out, `{"ext":"x","size":y`+"\t"+`1}`) {
		t.Errorf("FS-17 not reproduced in top_by_size: %q", out)
	}
}
