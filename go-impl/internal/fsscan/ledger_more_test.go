// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package fsscan

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/term"
)

// runGoEnv is runGo with extra caller variables (PATH, TMPDIR, CDPATH...).
func runGoEnv(fn, home, pwd string, extra map[string]string, args ...string) (string, string, int) {
	m := map[string]string{"HOME": home, "PWD": pwd, "GASH__HELP": "0"}
	for k, v := range extra {
		m[k] = v
	}
	e := invoke.EnvMap(m)
	var out, errb bytes.Buffer
	c := &invoke.Call{
		Ctx: context.Background(), Name: fn, Args: args, Env: e,
		Stdout: &out, Stderr: &errb, Term: term.Fixed(e, false, false, false),
	}
	rc := scanHandlers[fn](c)
	return out.String(), errb.String(), rc
}

// same2x runs fn on the live 2.x sources and in Go (with extra caller
// variables on the Go side only), fails on any byte difference after the
// scan_time and fixture-path normalization, and returns the Go result.
func same2x(t *testing.T, fx string, extra map[string]string, fn string, args ...string) (string, string, int) {
	t.Helper()
	home := t.TempDir()
	o2, e2, rc2, ok := run2x(t, fn, home, args...)
	if !ok {
		t.Skip("2.x sources or bash unavailable")
	}
	og, eg, rcg := runGoEnv(fn, home, home, extra, args...)
	if normScan(og, fx) != normScan(o2, fx) {
		t.Errorf("%s %q stdout:\n go: %q\n 2x: %q", fn, args, normScan(og, fx), normScan(o2, fx))
	}
	if normScan(eg, fx) != normScan(e2, fx) {
		t.Errorf("%s %q stderr:\n go: %q\n 2x: %q", fn, args, normScan(eg, fx), normScan(e2, fx))
	}
	if rcg != rc2 {
		t.Errorf("%s %q rc: go=%d 2x=%d", fn, args, rcg, rc2)
	}
	return og, eg, rcg
}

// physical returns the symlink-free absolute form of a test directory, as
// the scanners print it.
func physical(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// withForbidden points the scanners' protected list at fixture directories
// for one test (the sysroot indirection of FS-02/FS-03).
func withForbidden(t *testing.T, dirs ...string) {
	t.Helper()
	orig := scannerForbidden
	scannerForbidden = append(append([]string{}, orig...), dirs...)
	t.Cleanup(func() { scannerForbidden = orig })
}

// fakeTool writes an executable that prints garbage and fails, so a scanner
// that ran it would change its result.
func fakeTool(t *testing.T, dir, name string) {
	t.Helper()
	script := "#!/bin/sh\necho 'fake " + name + ": unsupported' >&2\necho 'garbage line'\nexit 1\n"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestLedger_FS02_2x(t *testing.T) {
	prot := physical(t, t.TempDir())
	mustWrite(t, filepath.Join(prot, "secret"), "0123456789")
	d := physical(t, t.TempDir())
	if err := os.Symlink(prot, filepath.Join(d, "link")); err != nil {
		t.Fatal(err)
	}
	withForbidden(t, prot)

	// -L walks into the protected tree below ROOT (not re-checked).
	out, _, rc := runGo("files_largest", t.TempDir(), d, "-L", "--json", "--depth", "2", d)
	if rc != 0 || !strings.Contains(out, `"path":"`+d+`/link/secret"`) {
		t.Errorf("files_largest -L: rc=%d out=%q", rc, out)
	}
	out, _, rc = runGo("tree_stats", t.TempDir(), d, "-L", "--json", d)
	if rc != 0 || !strings.Contains(out, `"files_total":1`) {
		t.Errorf("tree_stats -L: rc=%d out=%q", rc, out)
	}
	// The PATH argument itself is guarded, with the resolved path.
	out, errb, rc := runGo("files_largest", t.TempDir(), d, "--json", filepath.Join(d, "link"))
	if rc == 0 || out != "" || !strings.Contains(errb, `"error":"forbidden_path","details":"`+prot+`"`) {
		t.Errorf("guarded link: rc=%d out=%q err=%q", rc, out, errb)
	}
}

func TestLedger_FS03_2x(t *testing.T) {
	d := physical(t, t.TempDir())
	mustMkdir(t, d, "proc")
	mustWrite(t, filepath.Join(d, "proc", "f"), "x")
	withForbidden(t, filepath.Join(d, "proc"))
	// The walk from an allowed root enters a protected subtree, no warning.
	out, errb, rc := runGo("files_largest", t.TempDir(), d, "--json", d)
	if rc != 0 || errb != "" || !strings.Contains(out, `"path":"`+d+`/proc/f"`) {
		t.Errorf("walk below root: rc=%d out=%q err=%q", rc, out, errb)
	}
	// "/" is blocked without --allow-root and scans silently with it.
	_, errb, rc = runGo("files_largest", t.TempDir(), d, "--json", "--depth", "0", "/")
	if rc == 0 || !strings.Contains(errb, `"error":"root_scan_blocked"`) {
		t.Errorf("/ without --allow-root: rc=%d err=%q", rc, errb)
	}
	same2x(t, d, nil, "files_largest", "--allow-root", "--json", "--depth", "0", "/")
	same2x(t, d, nil, "dirs_list_empty", "--allow-root", "--json", "--depth", "0", "/")
}

func TestLedger_FS04_2x(t *testing.T) {
	d := t.TempDir()
	mustMkdir(t, d, "a", "b")
	mustWrite(t, filepath.Join(d, "b", "f"), "x")
	a, b := filepath.Join(d, "a"), filepath.Join(d, "b")
	for fn := range scanHandlers {
		args := []string{"--json", a, b}
		if fn == "dirs_find_large" {
			args = append([]string{"--size", "1"}, args...)
		}
		same2x(t, d, nil, fn, args...)
		same2x(t, d, nil, fn, append(args[:len(args)-2:len(args)-2], "--", b, a)...)
	}
	out, _, rc := runGo("files_largest", t.TempDir(), d, "--json", a, b)
	if rc != 0 || !strings.Contains(out, `"path":"`+physical(t, b)+`/f"`) {
		t.Errorf("last PATH wins: rc=%d out=%q", rc, out)
	}
}

func TestLedger_FS05_2x(t *testing.T) {
	d := t.TempDir()
	mustWrite(t, filepath.Join(d, "f"), "x")
	out, _, rc := same2x(t, d, nil, "files_largest", "--json", "--depth", "2147483648", d)
	if rc != 0 || !strings.Contains(out, `"count":0`) || !strings.Contains(out, `"errors":1`) {
		t.Errorf("--depth 2147483648: rc=%d out=%q", rc, out)
	}
	same2x(t, d, nil, "files_largest", "--json", "-n", "99999999999999999999", d)
	same2x(t, d, nil, "files_largest", "--json", "-n", "1234567890", d)
	same2x(t, d, nil, "dirs_largest", "--json", "--depth", "9223372036854775807", d)
	same2x(t, d, nil, "dirs_list_empty", "--json", "--depth", "1234567890", d)
	same2x(t, d, nil, "tree_stats", "--json", "--top", "1234567890", d)
}

func TestLedger_FS06_2x(t *testing.T) {
	d := t.TempDir()
	tab := filepath.Join(d, "t\tx")
	mustMkdir(t, d, "t\tx")
	writeN(t, filepath.Join(tab, "f"), 5000)
	out, _, _ := same2x(t, d, nil, "dirs_largest", "--json", d)
	if !strings.Contains(out, `t\tx"`) {
		t.Errorf("tab path not listed: %q", out)
	}
	same2x(t, d, nil, "dirs_largest", "--null", d)
	same2x(t, d, nil, "dirs_largest", d)
}

func TestLedger_FS07_2x(t *testing.T) {
	d := t.TempDir()
	mustMkdir(t, d, "et\tx")
	out, _, _ := same2x(t, d, nil, "dirs_list_empty", "--json", d)
	if !strings.Contains(out, `"count":1`) {
		t.Errorf("tab dir not counted: %q", out)
	}
	for _, mode := range []string{"--null", "--count", ""} {
		args := []string{d}
		if mode != "" {
			args = []string{mode, d}
		}
		same2x(t, d, nil, "dirs_list_empty", args...)
	}
}

func TestLedger_FS08_2x(t *testing.T) {
	d := t.TempDir()
	mustMkdir(t, d, "en\nx")
	out, _, _ := same2x(t, d, nil, "tree_stats", "--json", d)
	if !strings.Contains(out, `"empty_dirs":2`) || !strings.Contains(out, `"dirs_total":1`) {
		t.Errorf("FS-08 counts: %q", out)
	}
}

func TestLedger_FS09_2x(t *testing.T) {
	d := t.TempDir()
	mustMkdir(t, d, "t\tx")
	writeN(t, filepath.Join(d, "t\tx", "f"), 5000)
	out, _, _ := same2x(t, d, nil, "dirs_find_large", "--size", "1", "--json", d)
	if !strings.Contains(out, `"path":"`+physical(t, d)+`/t"`) {
		t.Errorf("path not cut at the tab: %q", out)
	}
	out, _, _ = same2x(t, d, nil, "dirs_find_large", "--size", "1", "--json", "--with-mtime", d)
	if !strings.Contains(out, `t\tx"`) {
		t.Errorf("--with-mtime path not intact: %q", out)
	}
	same2x(t, d, nil, "dirs_find_large", "--size", "1", "--null", d)
}

func TestLedger_FS10_2x(t *testing.T) {
	d := t.TempDir()
	for _, n := range []string{"esc\033x", "bs\bx", "ff\fx", "soh\001x"} {
		mustWrite(t, filepath.Join(d, n), "x")
		mustMkdir(t, d, "d"+n)
	}
	same2x(t, d, nil, "files_largest", "--json", d)
	same2x(t, d, nil, "dirs_list_empty", "--json", d)
	same2x(t, d, nil, "dirs_largest", "--json", d)
	same2x(t, d, nil, "dirs_find_large", "--size", "0", "--json", d)
}

func TestLedger_FS11_2x(t *testing.T) {
	d := t.TempDir()
	writeN(t, filepath.Join(d, "f"), 2000)
	_, _, rc := same2x(t, d, nil, "files_largest", "--json", "--min-size", "K1", d)
	if rc != 0 {
		t.Errorf("--min-size K1: rc=%d", rc)
	}
	_, errb, _ := same2x(t, d, nil, "files_largest", "--json", "--min-size", "1kb", d)
	if !strings.Contains(errb, `"error":"invalid_size"`) {
		t.Errorf("--min-size 1kb: %q", errb)
	}
	for _, v := range []string{"1.5K", "2K", "1Ki", "1KiB", "1KB", "abc", "1x2", "0", "3M", "1e3"} {
		same2x(t, d, nil, "files_largest", "--json", "--min-size", v, d)
		same2x(t, d, nil, "dirs_find_large", "--json", "--size", v, d)
	}
}

func TestLedger_FS12_2x(t *testing.T) {
	base := t.TempDir()
	d := filepath.Join(base, "proj")
	mustMkdir(t, base, "proj/src", "proj/mysrcdir", "proj/logs", "proj/s1c/e", "proj/x/src")
	mustWrite(t, filepath.Join(d, "mysrcdir", "f"), "x")
	writeN(t, filepath.Join(d, "logs", "a.log"), 3000)
	writeN(t, filepath.Join(d, "src", "m.go"), 2000)
	writeN(t, filepath.Join(d, "x", "src", "n.go"), 1000)
	for _, pat := range []string{"src", "*/src", physical(t, d) + "/src", "*.log", "s*c", "src|logs", "proj"} {
		same2x(t, base, nil, "dirs_largest", "--json", "--exclude", pat, d)
		same2x(t, base, nil, "dirs_find_large", "--json", "--size", "0", "--exclude", pat, d)
		same2x(t, base, nil, "files_largest", "--json", "--exclude", pat, d)
		same2x(t, base, nil, "dirs_list_empty", "--json", "--exclude", pat, d)
	}
}

func TestLedger_FS13_2x(t *testing.T) {
	base := t.TempDir()
	v := filepath.Join(base, "vendor")
	mustMkdir(t, base, "vendor")
	mustWrite(t, filepath.Join(v, "f"), "x")
	out, _, _ := same2x(t, base, nil, "files_largest", "--json", v)
	if !strings.Contains(out, `"count":0`) {
		t.Errorf("pruned root still scanned: %q", out)
	}
	d := t.TempDir()
	mustMkdir(t, d, "node_modules/a/b", "src/c")
	same2x(t, d, nil, "dirs_list_empty", "--json", "--min-depth", "2", d)
	same2x(t, d, nil, "dirs_list_empty", "--json", d)
}

func TestLedger_FS14_2x(t *testing.T) {
	d := t.TempDir()
	out, _, rc := same2x(t, d, nil, "dirs_list_empty", "--json", "--depth", "2147483648", d)
	if rc != 0 || !strings.Contains(out, `"data":[],"count":0`) || !strings.Contains(out, `"errors":1`) {
		t.Errorf("FS-14: rc=%d out=%q", rc, out)
	}
	same2x(t, d, nil, "files_largest", "--json", "--depth", "99999999999", d)
	same2x(t, d, nil, "tree_stats", "--json", "--depth", "2147483648", d)
}

func TestLedger_FS15_2x(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("needs an unprivileged user: root reads mode-000 directories")
	}
	d := t.TempDir()
	mustMkdir(t, d, "x")
	if err := os.Chmod(filepath.Join(d, "x"), 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(d, "x"), 0o755) })
	out, _, _ := same2x(t, d, nil, "dirs_list_empty", "--json", d)
	if !strings.Contains(out, `"errors":2`) {
		t.Errorf("dirs_list_empty errors: %q", out)
	}
	out, _, _ = same2x(t, d, nil, "files_largest", "--json", d)
	if !strings.Contains(out, `"errors":1`) {
		t.Errorf("files_largest errors: %q", out)
	}
}

func TestLedger_FS16_2x(t *testing.T) {
	d := t.TempDir()
	mustMkdir(t, d, "node_modules/p", "src")
	mustWrite(t, filepath.Join(d, "node_modules", "p", "i.js"), "x")
	mustWrite(t, filepath.Join(d, "src", "a"), "x")
	out, _, _ := same2x(t, d, nil, "tree_stats", "--json", d)
	if !strings.Contains(out, `"empty_dirs":1`) {
		t.Errorf("FS-16 empty_dirs: %q", out)
	}
	same2x(t, d, nil, "tree_stats", "--json", "--exclude", "*/src", d)
}

func TestLedger_FS19_2x(t *testing.T) {
	d := t.TempDir()
	mustWrite(t, filepath.Join(d, "f"), "x")
	out, _, _ := same2x(t, d, nil, "files_largest", "--exclude", "--json", d)
	if !strings.HasPrefix(out, `{"data":`) {
		t.Errorf("FS-19 not JSON: %q", out)
	}
	same2x(t, d, nil, "dirs_list_empty", "--exclude", "--json", d)
}

// TestLedger_FS23: the root and --exclude values are compared literally
// (no awk escape processing).
func TestLedger_FS23(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, `a\tb`)
	mustMkdir(t, base, `a\tb/sub`)
	writeN(t, filepath.Join(root, "sub", "f"), 5000)
	pr := physical(t, root)
	for _, fn := range []string{"dirs_largest", "dirs_find_large"} {
		args := []string{"--json", root}
		if fn == "dirs_find_large" {
			args = []string{"--json", "--size", "1", root}
		}
		out, _, rc := runGo(fn, t.TempDir(), base, args...)
		if rc != 0 || strings.Contains(out, `"path":"`+strings.ReplaceAll(pr, `\`, `\\`)+`"`) ||
			!strings.Contains(out, `/sub"`) {
			t.Errorf("%s lists its own root: rc=%d out=%q", fn, rc, out)
		}
	}
	d := t.TempDir()
	mustMkdir(t, d, "x\ty")
	writeN(t, filepath.Join(d, "x\ty", "f"), 5000)
	out, _, _ := runGo("dirs_largest", t.TempDir(), d, "--json", "--exclude", `x\ty`, d)
	if !strings.Contains(out, `x\ty"`) {
		t.Errorf(`--exclude 'x\ty' hid the tab directory: %q`, out)
	}
}

// TestLedger_FS24: GNU du semantics whatever du is on PATH.
func TestLedger_FS24(t *testing.T) {
	bin := t.TempDir()
	fakeTool(t, bin, "du")
	d := t.TempDir()
	mustMkdir(t, d, "xgit", ".git/o", "agit/q", "Xcache", "proj/.cache")
	writeN(t, filepath.Join(d, "xgit", "f"), 5000)
	writeN(t, filepath.Join(d, ".git", "o", "f"), 7000)
	writeN(t, filepath.Join(d, "proj", ".cache", "f"), 20000)
	writeN(t, filepath.Join(d, "Xcache", "f"), 3000)
	env := map[string]string{"PATH": bin + ":/usr/bin:/bin", "__GASH_FS_DU_HAS_EXCLUDE": "0"}
	out, _, _ := same2x(t, d, env, "dirs_largest", "--json", d)
	if !strings.Contains(out, `/xgit"`) || !strings.Contains(out, `/Xcache"`) {
		t.Errorf("prune names matched as regexes: %q", out)
	}
	same2x(t, d, env, "dirs_find_large", "--json", "--size", "1", d)
}

// TestLedger_FS25: a relative PATH resolves against the working directory;
// CDPATH is not read.
func TestLedger_FS25(t *testing.T) {
	d := physical(t, t.TempDir())
	mustMkdir(t, d, "a/src/sub", "b/src/other")
	writeN(t, filepath.Join(d, "a", "src", "sub", "f"), 5000)
	t.Chdir(filepath.Join(d, "a"))
	for fn := range scanHandlers {
		args := []string{"--json", "src"}
		if fn == "dirs_find_large" {
			args = []string{"--json", "--size", "0", "src"}
		}
		out, _, rc := runGoEnv(fn, t.TempDir(), filepath.Join(d, "a"),
			map[string]string{"CDPATH": filepath.Join(d, "b")}, args...)
		if rc != 0 || !strings.Contains(out, d+"/a/src") || strings.Contains(out, d+"/b/src") {
			t.Errorf("%s: rc=%d out=%q", fn, rc, out)
		}
	}
	out, _, _ := runGoEnv("files_largest", t.TempDir(), filepath.Join(d, "a"),
		map[string]string{"CDPATH": filepath.Join(d, "b")}, "--json", "src")
	if !strings.Contains(out, `"path":"`+d+`/a/src/sub/f"`) {
		t.Errorf("files_largest src: %q", out)
	}
}

// TestLedger_FS26: the GNU-path results whatever find is on PATH.
func TestLedger_FS26(t *testing.T) {
	bin := t.TempDir()
	fakeTool(t, bin, "find")
	d := t.TempDir()
	mustMkdir(t, d, "s")
	mustWrite(t, filepath.Join(d, "f"), "x")
	writeN(t, filepath.Join(d, "s", "g.txt"), 300)
	env := map[string]string{"PATH": bin + ":/usr/bin:/bin"}
	out, _, _ := same2x(t, d, env, "files_largest", "--json", d)
	if !strings.Contains(out, `"count":2`) || !strings.Contains(out, `"errors":0`) {
		t.Errorf("files_largest under a foreign find: %q", out)
	}
	same2x(t, d, env, "tree_stats", "--json", d)
	same2x(t, d, env, "dirs_list_empty", "--json", d)
}

// TestLedger_FS27: newline directories are left out of dirs_largest and
// dirs_find_large, also from the parent totals, whatever du is on PATH.
func TestLedger_FS27(t *testing.T) {
	bin := t.TempDir()
	fakeTool(t, bin, "du")
	d := t.TempDir()
	mustMkdir(t, d, "n\nx", "n", "ok")
	writeN(t, filepath.Join(d, "n\nx", "f"), 9000)
	writeN(t, filepath.Join(d, "ok", "f"), 5000)
	env := map[string]string{"PATH": bin + ":/usr/bin:/bin"}
	out, _, _ := same2x(t, d, env, "dirs_largest", "--json", d)
	if strings.Count(out, `"path":`) != 2 || !strings.Contains(out, `/ok"`) || !strings.Contains(out, `/n"`) {
		t.Errorf("FS-27 rows: %q", out)
	}
	same2x(t, d, env, "dirs_largest", "--null", d)
	same2x(t, d, env, "dirs_find_large", "--json", "--size", "0", d)
}

// TestLedger_FS29: non-ASCII digits get the scanner's invalid_* envelope in
// every locale.
func TestLedger_FS29(t *testing.T) {
	d := t.TempDir()
	cases := []struct{ fn, flag, code string }{
		{"files_largest", "-n", "invalid_limit"},
		{"files_largest", "--depth", "invalid_depth"},
		{"files_largest", "--min-size", "invalid_size"},
		{"dirs_largest", "-n", "invalid_limit"},
		{"dirs_largest", "--depth", "invalid_depth"},
		{"dirs_largest", "--min-size", "invalid_size"},
		{"dirs_find_large", "-n", "invalid_limit"},
		{"dirs_find_large", "--depth", "invalid_depth"},
		{"dirs_find_large", "--size", "invalid_size"},
		{"dirs_list_empty", "--depth", "invalid_depth"},
		{"dirs_list_empty", "--min-depth", "invalid_depth"},
		{"tree_stats", "--depth", "invalid_depth"},
		{"tree_stats", "--top", "invalid_top"},
	}
	for _, loc := range []string{"en_US.UTF-8", "it_IT.UTF-8", "C.UTF-8"} {
		for _, digit := range []string{"١", "１", "²", "𝟙"} {
			for _, k := range cases {
				out, errb, rc := runGoEnv(k.fn, t.TempDir(), d, map[string]string{"LC_ALL": loc},
					"--json", k.flag, digit, d)
				if rc != 1 || out != "" || !strings.Contains(errb, `"error":"`+k.code+`"`) {
					t.Errorf("%s %s %q (%s): rc=%d out=%q err=%q", k.fn, k.flag, digit, loc, rc, out, errb)
				}
			}
		}
	}
}

// TestLedger_FS30: TMPDIR is not read; errors are counted in-process.
func TestLedger_FS30(t *testing.T) {
	d := t.TempDir()
	out, errb, rc := runGoEnv("files_largest", t.TempDir(), d, map[string]string{"TMPDIR": "/nonexistent"},
		"--json", "--depth", "2147483648", d)
	if rc != 0 || errb != "" || !strings.Contains(out, `"errors":1`) {
		t.Errorf("FS-30: rc=%d out=%q err=%q", rc, out, errb)
	}
}
