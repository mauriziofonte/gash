// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package fsscan

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
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

// envOf decodes one JSON line, failing the test when it is not valid JSON.
func envOf(t *testing.T, label, out string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("%s: invalid JSON %q: %v", label, out, err)
	}
	return m
}

// scanArgs adds the flags a scanner needs to report everything on a small
// fixture (dirs_find_large: a zero threshold; dirs_largest: depth 3).
func scanArgs(fn string, args ...string) []string {
	switch fn {
	case "dirs_find_large":
		return append([]string{"--size", "0"}, args...)
	case "dirs_largest":
		return append([]string{"--depth", "3"}, args...)
	}
	return args
}

// TestLedger_FS02_fix: -L never enters a protected tree; tree_stats -L
// counts what the tree without the link holds (3.0: FS-02 fixed).
func TestLedger_FS02_fix(t *testing.T) {
	prot := physical(t, t.TempDir())
	mustWrite(t, filepath.Join(prot, "secret"), "0123456789")
	mustMkdir(t, prot, "sub")
	d := physical(t, t.TempDir())
	mustMkdir(t, d, "own")
	mustWrite(t, filepath.Join(d, "own", "f"), "x")
	if err := os.Symlink(prot, filepath.Join(d, "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(prot, "secret"), filepath.Join(d, "own", "flink")); err != nil {
		t.Fatal(err)
	}
	withForbidden(t, prot)

	out, errb, rc := runGo("files_largest", t.TempDir(), d, "-L", "--json", "--depth", "2", d)
	m := envOf(t, "files_largest -L", out)
	if rc != 0 || errb != "" || m["count"] != 1.0 || m["errors"] != 0.0 || !strings.Contains(out, `"path":"`+d+`/own/f"`) {
		t.Errorf("files_largest -L: rc=%d out=%q err=%q", rc, out, errb)
	}
	out, _, rc = runGo("tree_stats", t.TempDir(), d, "-L", "--json", d)
	data := envOf(t, "tree_stats -L", out)["data"].(map[string]any)
	if rc != 0 || data["files_total"] != 1.0 || data["dirs_total"] != 2.0 || data["empty_dirs"] != 0.0 {
		t.Errorf("tree_stats -L: rc=%d out=%q", rc, out)
	}
	// The PATH argument itself is still guarded, with the resolved path.
	out, errb, rc = runGo("files_largest", t.TempDir(), d, "--json", filepath.Join(d, "link"))
	if rc == 0 || out != "" || !strings.Contains(errb, `"error":"forbidden_path","details":"`+prot+`"`) {
		t.Errorf("guarded link: rc=%d out=%q err=%q", rc, out, errb)
	}
}

// TestLedger_FS03_fix: a walk never descends into a protected tree, also
// from an allowed root above it, in all five scanners (3.0: FS-03 fixed).
func TestLedger_FS03_fix(t *testing.T) {
	d := physical(t, t.TempDir())
	for _, top := range []string{"proc", "root", "home"} {
		mustMkdir(t, d, top+"/sub", top+"/e")
		writeN(t, filepath.Join(d, top, "sub", "f"), 3000)
	}
	withForbidden(t, filepath.Join(d, "proc"), filepath.Join(d, "root"))
	for fn := range scanHandlers {
		out, errb, rc := runGo(fn, t.TempDir(), d, scanArgs(fn, "--allow-root", "--json", d)...)
		envOf(t, fn, out)
		if rc != 0 || errb != "" || strings.Contains(out, d+"/proc") || strings.Contains(out, d+"/root") {
			t.Errorf("%s: rc=%d out=%q err=%q", fn, rc, out, errb)
		}
		if fn != "tree_stats" && !strings.Contains(out, d+"/home") {
			t.Errorf("%s: home/ entries missing: %q", fn, out)
		}
	}
	out, _, _ := runGo("tree_stats", t.TempDir(), d, "--json", d)
	data := envOf(t, "tree_stats", out)["data"].(map[string]any)
	if data["files_total"] != 1.0 || data["dirs_total"] != 4.0 || data["empty_dirs"] != 1.0 {
		t.Errorf("tree_stats counts the protected trees: %q", out)
	}
	// "/" is blocked without --allow-root and scans silently with it.
	_, errb, rc := runGo("files_largest", t.TempDir(), d, "--json", "--depth", "0", "/")
	if rc == 0 || !strings.Contains(errb, `"error":"root_scan_blocked"`) {
		t.Errorf("/ without --allow-root: rc=%d err=%q", rc, errb)
	}
	out, errb, rc = runGo("dirs_list_empty", t.TempDir(), d, "--allow-root", "--json", "--depth", "0", "/")
	if rc != 0 || errb != "" || !strings.HasPrefix(out, `{"data":[],"count":0,"path":"/"`) {
		t.Errorf("/ with --allow-root: rc=%d out=%q err=%q", rc, out, errb)
	}
}

// TestLedger_FS04_fix: a second PATH, before or after "--", fails with
// unexpected_argument; "-- A" still scans A (3.0: FS-04 fixed).
func TestLedger_FS04_fix(t *testing.T) {
	d := t.TempDir()
	mustMkdir(t, d, "a", "b")
	mustWrite(t, filepath.Join(d, "b", "f"), "x")
	a, b := filepath.Join(d, "a"), filepath.Join(d, "b")
	want := `{"error":"unexpected_argument","details":"` + b + `","action":"RETRY","recoverable":true,"hint":"Pass at most one PATH"}` + "\n"
	for fn := range scanHandlers {
		for _, args := range [][]string{{"--json", a, b}, {"--json", "--", a, b}, {"--json", a, "--", b}} {
			out, errb, rc := runGo(fn, t.TempDir(), d, args...)
			if rc != 1 || out != "" || errb != want {
				t.Errorf("%s %q: rc=%d out=%q err=%q", fn, args, rc, out, errb)
			}
		}
		out, errb, rc := runGo(fn, t.TempDir(), d, "--json", "--", b)
		if rc != 0 || errb != "" || !json.Valid([]byte(out)) {
			t.Errorf("%s -- B: rc=%d out=%q err=%q", fn, rc, out, errb)
		}
	}
	_, errb, rc := runGo("files_largest", t.TempDir(), d, a, b)
	if rc != 1 || errb != "Error: unexpected_argument: "+b+"\nHint: Pass at most one PATH\n" {
		t.Errorf("text mode: rc=%d err=%q", rc, errb)
	}
}

// TestLedger_FS05_fix: every integer flag takes 1 to 9 ASCII digits, base
// 10, with the FS-49 hint (3.0: FS-05 and FS-49 fixed).
func TestLedger_FS05_fix(t *testing.T) {
	d := t.TempDir()
	mustWrite(t, filepath.Join(d, "f"), "x")
	flags := []struct{ fn, flag, code string }{
		{"files_largest", "-n", "invalid_limit"}, {"files_largest", "--depth", "invalid_depth"},
		{"dirs_largest", "-n", "invalid_limit"}, {"dirs_largest", "--depth", "invalid_depth"},
		{"dirs_find_large", "-n", "invalid_limit"}, {"dirs_find_large", "--depth", "invalid_depth"},
		{"dirs_list_empty", "--min-depth", "invalid_depth"}, {"dirs_list_empty", "--depth", "invalid_depth"},
		{"tree_stats", "--depth", "invalid_depth"}, {"tree_stats", "--top", "invalid_top"},
	}
	for _, f := range flags {
		want := `{"error":"` + f.code + `","details":"1234567890","action":"RETRY","recoverable":true,` +
			`"hint":"Provide a non-negative integer (at most 9 digits)"}` + "\n"
		out, errb, rc := runGo(f.fn, t.TempDir(), d, "--json", f.flag, "1234567890", d)
		if rc != 1 || out != "" || errb != want {
			t.Errorf("%s %s: rc=%d out=%q err=%q", f.fn, f.flag, rc, out, errb)
		}
		if _, _, rc := runGo(f.fn, t.TempDir(), d, "--json", f.flag, "999999999", d); rc != 0 {
			t.Errorf("%s %s 999999999: rc=%d", f.fn, f.flag, rc)
		}
	}
	// --depth 010 is ten levels.
	deep := t.TempDir()
	p := deep
	for i := 1; i <= 14; i++ {
		p = filepath.Join(p, "d")
	}
	mustMkdir(t, p, ".")
	writeN(t, filepath.Join(p, "f"), 5000)
	out, _, rc := runGo("dirs_largest", t.TempDir(), deep, "--json", "--depth", "010", deep)
	if n := strings.Count(out, `"path":`); rc != 0 || n != 10 {
		t.Errorf("--depth 010: rc=%d rows=%d, want 10", rc, n)
	}
}

// fixTabNL builds the tab/newline fixture of FS-01: dirs t<TAB>x, n<NL>x,
// empty et<TAB>x and en<NL>x, files ft<TAB>y and fn<NL>y, and norm/f.
func fixTabNL(t *testing.T) string {
	t.Helper()
	d := physical(t, t.TempDir())
	mustMkdir(t, d, "t\tx", "n\nx", "et\tx", "en\nx", "norm")
	writeN(t, filepath.Join(d, "t\tx", "f"), 5000)
	writeN(t, filepath.Join(d, "n\nx", "f"), 5000)
	writeN(t, filepath.Join(d, "ft\ty"), 700)
	writeN(t, filepath.Join(d, "fn\ny"), 700)
	writeN(t, filepath.Join(d, "norm", "f"), 1000)
	return d
}

// TestLedger_FS01_fix: no scanner, in any mode, reports a path below the
// root that holds a tab or a newline, nor a name cut from one; tree_stats
// leaves them out of every count (3.0: FS-01 applied uniformly).
func TestLedger_FS01_fix(t *testing.T) {
	d := fixTabNL(t)
	bad := func(out string) bool {
		rest := strings.ReplaceAll(out, d, "")
		for _, cut := range []string{"/t\"", "/et\"", "/n\"", "/t\x00", "/et\x00", "/n\x00", "/t\n", "/et\n", "/n\n"} {
			if strings.Contains(rest, cut) {
				return true
			}
		}
		return strings.Contains(rest, `\t`) || strings.Contains(rest, `\n`) ||
			strings.Contains(strings.TrimSuffix(rest, "\n"), "\n/") || strings.Contains(rest, "\t")
	}
	for fn := range scanHandlers {
		modes := [][]string{{"--json"}, {"--null"}, {}}
		switch fn {
		case "tree_stats":
			modes = [][]string{{"--json"}}
		case "dirs_list_empty":
			modes = append(modes, []string{"--count"})
		case "dirs_find_large":
			modes = append(modes, []string{"--json", "--with-mtime"}, []string{"--null", "--with-mtime"})
		}
		for _, m := range modes {
			args := scanArgs(fn, append(append([]string{}, m...), d)...)
			out, errb, rc := runGo(fn, t.TempDir(), d, args...)
			if rc != 0 || errb != "" || bad(out) {
				t.Errorf("%s %q: rc=%d out=%q err=%q", fn, m, rc, out, errb)
			}
			if len(m) > 0 && m[0] == "--json" {
				envOf(t, fn, out)
			}
		}
	}
	out, _, _ := runGo("tree_stats", t.TempDir(), d, "--json", d)
	data := envOf(t, "tree_stats", out)["data"].(map[string]any)
	if data["files_total"] != 1.0 || data["dirs_total"] != 2.0 || data["empty_dirs"] != 0.0 || data["size_total"] != 1000.0 {
		t.Errorf("tree_stats counts: %q", out)
	}
	out, _, _ = runGo("dirs_list_empty", t.TempDir(), d, "--count", d)
	if out != "0\n" {
		t.Errorf("dirs_list_empty --count: %q", out)
	}
}

// TestLedger_FS06_fix: dirs_largest leaves a tab directory out in every
// mode, and out of its parent's total (3.0: FS-06 fixed).
func TestLedger_FS06_fix(t *testing.T) {
	d := physical(t, t.TempDir())
	mustMkdir(t, d, "p/t\tx", "q")
	writeN(t, filepath.Join(d, "p", "t\tx", "f"), 50000)
	writeN(t, filepath.Join(d, "q", "f"), 5000)
	for _, mode := range []string{"--json", "--null", "--no-color"} {
		out, _, rc := runGo("dirs_largest", t.TempDir(), d, mode, "--depth", "3", d)
		if rc != 0 || strings.Contains(out, "t\tx") || strings.Contains(out, `t\tx`) {
			t.Errorf("%s: rc=%d out=%q", mode, rc, out)
		}
	}
	out, _, _ := runGo("dirs_largest", t.TempDir(), d, "--json", d)
	var env struct {
		Data []struct {
			Size int64
			Path string
		}
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil || len(env.Data) != 2 || env.Data[0].Path != d+"/q" {
		t.Errorf("p must be smaller than q without its tab child: %q", out)
	}
}

// TestLedger_FS07_fix: dirs_list_empty leaves an empty tab directory out
// (3.0: FS-07 fixed).
func TestLedger_FS07_fix(t *testing.T) {
	d := t.TempDir()
	mustMkdir(t, d, "et\tx")
	mustWrite(t, filepath.Join(d, "f"), "x")
	for _, c := range []struct{ mode, want string }{{"--count", "0\n"}, {"--null", ""}, {"", ""}} {
		args := []string{d}
		if c.mode != "" {
			args = []string{c.mode, d}
		}
		if out, _, rc := runGo("dirs_list_empty", t.TempDir(), d, args...); rc != 0 || out != c.want {
			t.Errorf("%q: rc=%d out=%q", c.mode, rc, out)
		}
	}
	out, _, _ := runGo("dirs_list_empty", t.TempDir(), d, "--json", d)
	if envOf(t, "json", out)["count"] != 0.0 {
		t.Errorf("--json: %q", out)
	}
}

// TestLedger_FS08_fix: tree_stats counts neither tab paths nor newline
// directories, in both counts (3.0: FS-08 fixed).
func TestLedger_FS08_fix(t *testing.T) {
	d := t.TempDir()
	mustMkdir(t, d, "en\nx", "et\tx", "t\tx")
	mustWrite(t, filepath.Join(d, "t\tx", "f"), "x")
	mustWrite(t, filepath.Join(d, "ft\ty"), "x")
	out, _, _ := runGo("tree_stats", t.TempDir(), d, "--json", d)
	data := envOf(t, "tree_stats", out)["data"].(map[string]any)
	if data["empty_dirs"] != 0.0 || data["dirs_total"] != 1.0 || data["files_total"] != 0.0 {
		t.Errorf("FS-08 counts: %q", out)
	}
}

// TestLedger_FS09_fix: dirs_find_large never reports a path cut at a tab;
// every --null record is an existing directory (3.0: FS-09 fixed).
func TestLedger_FS09_fix(t *testing.T) {
	d := t.TempDir()
	mustMkdir(t, d, "t\tx", "ok")
	writeN(t, filepath.Join(d, "t\tx", "f"), 5000)
	writeN(t, filepath.Join(d, "ok", "f"), 5000)
	for _, extra := range [][]string{nil, {"--with-mtime"}} {
		out, _, rc := runGo("dirs_find_large", t.TempDir(), d, append([]string{"--size", "1", "--json"}, append(extra, d)...)...)
		if rc != 0 || strings.Contains(out, `/t"`) || strings.Contains(out, `t\tx`) || !strings.Contains(out, `/ok"`) {
			t.Errorf("%q: rc=%d out=%q", extra, rc, out)
		}
	}
	out, _, _ := runGo("dirs_find_large", t.TempDir(), d, "--size", "1", "--null", d)
	recs := strings.Split(strings.TrimSuffix(out, "\x00"), "\x00")
	for _, r := range recs {
		if st, err := os.Stat(r); err != nil || !st.IsDir() || strings.HasSuffix(r, "/t") {
			t.Errorf("--null record %q", r)
		}
	}
}

// TestLedger_FS10_fix: every scanner's JSON is valid for names holding
// control characters, and the names round-trip (3.0: FS-10 fixed).
func TestLedger_FS10_fix(t *testing.T) {
	d := physical(t, t.TempDir())
	names := []string{"esc\033x", "bs\bx", "ff\fx", "soh\001x", "del\x7fx"}
	for _, n := range names {
		writeN(t, filepath.Join(d, "f"+n), 2000)
		mustMkdir(t, d, "d"+n+"/e")
		writeN(t, filepath.Join(d, "d"+n, "g"), 2000)
	}
	for fn := range scanHandlers {
		out, _, rc := runGo(fn, t.TempDir(), d, scanArgs(fn, "--json", d)...)
		if rc != 0 || !json.Valid([]byte(out)) {
			t.Errorf("%s: rc=%d invalid JSON %q", fn, rc, out)
			continue
		}
		if fn == "tree_stats" {
			continue
		}
		var decoded struct{ Data json.RawMessage }
		_ = json.Unmarshal([]byte(out), &decoded)
		for _, n := range names {
			want := "/f" + n
			if fn != "files_largest" {
				want = "/d" + n
			}
			if !strings.Contains(string(decodeStrings(t, decoded.Data)), want) {
				t.Errorf("%s: %q does not round-trip", fn, want)
			}
		}
	}
}

// decodeStrings returns every string of a JSON value, joined by NULs.
func decodeStrings(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	var walk func(any)
	walk = func(x any) {
		switch y := x.(type) {
		case string:
			b.WriteString(y)
			b.WriteByte(0)
		case []any:
			for _, e := range y {
				walk(e)
			}
		case map[string]any:
			for _, e := range y {
				walk(e)
			}
		}
	}
	walk(v)
	return b.String()
}

// TestLedger_FS11_fix: the size flags use the native grammar: the fallback
// oddities are refused, units are case-insensitive (3.0: FS-11 fixed).
func TestLedger_FS11_fix(t *testing.T) {
	d := t.TempDir()
	writeN(t, filepath.Join(d, "f"), 2000)
	for _, v := range []string{"1K2", "K1", "iB5", "1 0K", "5i"} {
		for _, c := range []struct{ fn, flag string }{{"files_largest", "--min-size"}, {"dirs_largest", "--min-size"}, {"dirs_find_large", "--size"}} {
			_, errb, rc := runGo(c.fn, t.TempDir(), d, "--json", c.flag, v, d)
			if rc != 1 || !strings.Contains(errb, `"error":"invalid_size","details":"`+v+`"`) {
				t.Errorf("%s %s %q: rc=%d err=%q", c.fn, c.flag, v, rc, errb)
			}
		}
	}
	for _, c := range []struct {
		v     string
		count float64
	}{{"1kb", 1}, {"1KIB", 1}, {"1.5K", 1}, {"2K", 0}, {"1999B", 1}} {
		out, _, rc := runGo("files_largest", t.TempDir(), d, "--json", "--min-size", c.v, d)
		if rc != 0 || envOf(t, c.v, out)["count"] != c.count {
			t.Errorf("--min-size %s: rc=%d out=%q", c.v, rc, out)
		}
	}
}

// TestLedger_FS12_fix: one --exclude rule in all five scanners: the base
// name or the path below PATH, '*' never crossing '/', never the root's own
// path, no '|' splitting (3.0: FS-12 fixed).
func TestLedger_FS12_fix(t *testing.T) {
	base := physical(t, t.TempDir())
	d := filepath.Join(base, "proj")
	mustMkdir(t, base, "proj/src/e", "proj/mysrcdir/e", "proj/logs/e", "proj/x/src/e")
	writeN(t, filepath.Join(d, "mysrcdir", "f"), 3000)
	writeN(t, filepath.Join(d, "logs", "a.log"), 3000)
	writeN(t, filepath.Join(d, "src", "m.go"), 2000)
	writeN(t, filepath.Join(d, "x", "src", "n.go"), 1000)
	hidden := func(out, p string) bool { return !strings.Contains(out, d+"/"+p) }
	for fn := range scanHandlers {
		all, _, _ := runGo(fn, t.TempDir(), base, scanArgs(fn, "--json", d)...)
		for _, c := range []struct {
			pat          string
			hide, keep   []string
			sameAsNoExcl bool
		}{
			{pat: "src", hide: []string{"src/", "x/src/"}, keep: []string{"mysrcdir/"}},
			{pat: "*/src", hide: []string{"x/src/"}, keep: []string{"src/", "mysrcdir/"}},
			{pat: "s*c", hide: []string{"src/", "x/src/"}, keep: []string{"mysrcdir/"}},
			{pat: "*.log", keep: []string{"src/"}},
			{pat: "proj", sameAsNoExcl: true},
			{pat: "src|logs", sameAsNoExcl: true},
			{pat: d + "/src", sameAsNoExcl: true},
		} {
			out, _, rc := runGo(fn, t.TempDir(), base, scanArgs(fn, "--json", "--exclude", c.pat, d)...)
			if rc != 0 {
				t.Errorf("%s --exclude %q: rc=%d", fn, c.pat, rc)
				continue
			}
			if c.sameAsNoExcl {
				if normScan(out, d) != normScan(all, d) {
					t.Errorf("%s --exclude %q hid something:\n got %q\nwant %q", fn, c.pat, out, all)
				}
				continue
			}
			if fn == "tree_stats" {
				continue // no paths in its output; counts checked below
			}
			for _, h := range c.hide {
				if !hidden(out, h) {
					t.Errorf("%s --exclude %q: %s still listed: %q", fn, c.pat, h, out)
				}
			}
			for _, k := range c.keep {
				if hidden(out, k) {
					t.Errorf("%s --exclude %q: %s hidden: %q", fn, c.pat, k, out)
				}
			}
		}
	}
	out, _, _ := runGo("tree_stats", t.TempDir(), base, "--json", "--exclude", "src", d)
	data := envOf(t, "tree_stats", out)["data"].(map[string]any)
	if data["files_total"] != 2.0 || data["empty_dirs"] != 2.0 {
		t.Errorf("tree_stats --exclude src: %q", out)
	}
	out, _, _ = runGo("tree_stats", t.TempDir(), base, "--json", "--exclude", "*.log", d)
	if envOf(t, "tree_stats", out)["data"].(map[string]any)["files_total"] != 3.0 {
		t.Errorf("tree_stats --exclude *.log: %q", out)
	}
}

// TestLedger_FS13_fix: the default prunes and excludes never apply to the
// root, and --min-depth only filters the output (3.0: FS-13 fixed).
func TestLedger_FS13_fix(t *testing.T) {
	base := t.TempDir()
	v := filepath.Join(base, "vendor")
	mustMkdir(t, base, "vendor/e")
	mustWrite(t, filepath.Join(v, "f"), "x")
	for fn := range scanHandlers {
		out, _, rc := runGo(fn, t.TempDir(), base, scanArgs(fn, "--json", v)...)
		if rc != 0 || strings.Contains(out, `"count":0`) || strings.Contains(out, `"files_total":0`) {
			t.Errorf("%s scanning vendor/: rc=%d out=%q", fn, rc, out)
		}
	}
	out, _, _ := runGo("files_largest", t.TempDir(), base, "--json", "--exclude", "vendor", v)
	if !strings.Contains(out, `"count":1`) {
		t.Errorf("--exclude matched the root: %q", out)
	}
	d := t.TempDir()
	mustMkdir(t, d, "node_modules/a/b", "src/c")
	out, _, _ = runGo("dirs_list_empty", t.TempDir(), d, "--json", "--min-depth", "2", d)
	if strings.Contains(out, "node_modules") || !strings.Contains(out, "/src/c\"") {
		t.Errorf("--min-depth 2: %q", out)
	}
}

// TestLedger_FS14_fix: the argument cases are refused by validation; an
// unlistable root is path_unreadable (3.0: FS-14 fixed).
func TestLedger_FS14_fix(t *testing.T) {
	d := t.TempDir()
	_, errb, rc := runGo("dirs_list_empty", t.TempDir(), d, "--json", "--depth", "2147483648", d)
	if rc != 1 || !strings.Contains(errb, `"error":"invalid_depth"`) {
		t.Errorf("--depth 2147483648: rc=%d err=%q", rc, errb)
	}
	if os.Geteuid() == 0 {
		t.Skip("needs an unprivileged user: root lists mode-0111 directories")
	}
	x := filepath.Join(d, "x")
	mustMkdir(t, d, "x/sub")
	if err := os.Chmod(x, 0o111); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(x, 0o755) })
	want := `{"error":"path_unreadable","details":"Path '` + x + `' is not readable","action":"RETRY","recoverable":true,"hint":"Check directory permissions"}` + "\n"
	for fn := range scanHandlers {
		out, errb, rc := runGo(fn, t.TempDir(), d, scanArgs(fn, "--json", x)...)
		if rc != 1 || out != "" || errb != want {
			t.Errorf("%s: rc=%d out=%q err=%q", fn, rc, out, errb)
		}
	}
}

// TestLedger_FS15_fix: errors counts each unreadable entry once, in all
// five scanners (3.0: FS-15 fixed).
func TestLedger_FS15_fix(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("needs an unprivileged user: root reads mode-000 directories")
	}
	d := t.TempDir()
	mustMkdir(t, d, "x")
	if err := os.Chmod(filepath.Join(d, "x"), 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(d, "x"), 0o755) })
	for fn := range scanHandlers {
		out, _, rc := runGo(fn, t.TempDir(), d, scanArgs(fn, "--json", d)...)
		if rc != 0 || !strings.Contains(out, `"errors":1}`) {
			t.Errorf("%s: rc=%d out=%q", fn, rc, out)
		}
	}
}

// TestLedger_FS15_loop: a directory loop under -L is one error, and the walk
// does not run around it.
func TestLedger_FS15_loop(t *testing.T) {
	d := t.TempDir()
	mustMkdir(t, d, "a")
	mustWrite(t, filepath.Join(d, "a", "f"), "x")
	if err := os.Symlink("..", filepath.Join(d, "a", "up")); err != nil {
		t.Fatal(err)
	}
	out, _, rc := runGo("files_largest", t.TempDir(), d, "-L", "--json", d)
	if rc != 0 || !strings.Contains(out, `"count":1`) || !strings.Contains(out, `"errors":1}`) {
		t.Errorf("rc=%d out=%q", rc, out)
	}
}

// TestLedger_FS16_fix: tree_stats empty_dirs counts what dirs_list_empty
// --count counts, with the same prunes and excludes (3.0: FS-16 fixed).
func TestLedger_FS16_fix(t *testing.T) {
	d := t.TempDir()
	mustMkdir(t, d, "node_modules/p", "src", "logs")
	mustWrite(t, filepath.Join(d, "node_modules", "p", "i.js"), "x")
	mustWrite(t, filepath.Join(d, "src", "a"), "x")
	mustWrite(t, filepath.Join(d, "logs", "x.log"), "x")
	fixtures := []string{d, mkfix(t), fixTabNL(t)}
	for _, fx := range fixtures {
		for _, extra := range [][]string{nil, {"--exclude", "*.log"}, {"--no-ignore"}, {"--exclude", "src"}} {
			out, _, _ := runGo("tree_stats", t.TempDir(), fx, append(append([]string{"--json"}, extra...), fx)...)
			cnt, _, _ := runGo("dirs_list_empty", t.TempDir(), fx, append(append([]string{"--count"}, extra...), fx)...)
			got := envOf(t, "tree_stats", out)["data"].(map[string]any)["empty_dirs"]
			if strconv.Itoa(int(got.(float64)))+"\n" != cnt {
				t.Errorf("%s %q: empty_dirs %v, dirs_list_empty --count %q", fx, extra, got, cnt)
			}
		}
	}
	out, _, _ := runGo("tree_stats", t.TempDir(), d, "--json", "--exclude", "*.log", d)
	if !strings.Contains(out, `"empty_dirs":0`) {
		t.Errorf("FS-16 empty_dirs: %q", out)
	}
}

// TestLedger_FS19_fix: the --json pre-scan skips flag values, so
// `--exclude --json` takes "--json" as the glob only (3.0: FS-19 fixed).
func TestLedger_FS19_fix(t *testing.T) {
	d := t.TempDir()
	mustWrite(t, filepath.Join(d, "f"), "x")
	mustMkdir(t, d, "e")
	out, _, rc := runGo("files_largest", t.TempDir(), d, "--exclude", "--json", d)
	if rc != 0 || strings.HasPrefix(out, "{") || !strings.Contains(out, d) {
		t.Errorf("files_largest: rc=%d out=%q", rc, out)
	}
	out, _, rc = runGo("dirs_list_empty", t.TempDir(), d, "--exclude", "--json", d)
	if rc != 0 || strings.HasPrefix(out, "{") || !strings.HasSuffix(out, "/e\n") {
		t.Errorf("dirs_list_empty: rc=%d out=%q", rc, out)
	}
	// An error met before a real --json still gets the JSON contract.
	_, errb, rc := runGo("tree_stats", t.TempDir(), d, "--top", "x", "--json", d)
	if rc != 1 || !strings.HasPrefix(errb, `{"error":"invalid_top"`) {
		t.Errorf("error before --json: rc=%d err=%q", rc, errb)
	}
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
	// The --exclude value reaches the glob matcher unchanged: an escaped
	// backslash matches a literal one (3.0: FS-12 glob rule; a real tab
	// path is left out anyway, FS-01).
	d := t.TempDir()
	mustMkdir(t, d, `x\ty`, "keep")
	writeN(t, filepath.Join(d, `x\ty`, "f"), 5000)
	writeN(t, filepath.Join(d, "keep", "f"), 5000)
	out, _, _ := runGo("dirs_largest", t.TempDir(), d, "--json", "--exclude", `x\\ty`, d)
	if strings.Contains(out, `x\\ty"`) || !strings.Contains(out, `/keep"`) {
		t.Errorf(`--exclude 'x\\ty' did not hide the backslash directory: %q`, out)
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
	d := physical(t, t.TempDir())
	mustMkdir(t, d, "n\nx", "n", "ok")
	writeN(t, filepath.Join(d, "n\nx", "f"), 9000)
	writeN(t, filepath.Join(d, "ok", "f"), 5000)
	env := map[string]string{"PATH": bin + ":/usr/bin:/bin"}
	out, _, _ := runGoEnv("dirs_largest", t.TempDir(), d, env, "--json", d)
	var e struct{ Data []struct{ Path string } }
	if err := json.Unmarshal([]byte(out), &e); err != nil || len(e.Data) != 2 ||
		e.Data[0].Path != d+"/ok" || e.Data[1].Path != d+"/n" {
		t.Errorf("FS-27 rows: %q", out)
	}
	out, _, _ = runGoEnv("dirs_largest", t.TempDir(), d, env, "--null", d)
	if out != d+"/ok\x00"+d+"/n\x00" {
		t.Errorf("FS-27 --null: %q", out)
	}
	out, _, _ = runGoEnv("dirs_find_large", t.TempDir(), d, env, "--json", "--size", "0", d)
	if strings.Contains(out, `\n`) || strings.Count(out, `"path":`) != 2 {
		t.Errorf("FS-27 dirs_find_large: %q", out)
	}
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
// 3.0: FS-05 fixed: the oversized --depth that produced the error is now
// refused, so the error comes from a directory loop under -L instead.
func TestLedger_FS30(t *testing.T) {
	d := t.TempDir()
	if err := os.Symlink(".", filepath.Join(d, "loop")); err != nil {
		t.Fatal(err)
	}
	out, errb, rc := runGoEnv("files_largest", t.TempDir(), d, map[string]string{"TMPDIR": "/nonexistent"},
		"-L", "--json", d)
	if rc != 0 || errb != "" || !strings.Contains(out, `"errors":1`) {
		t.Errorf("FS-30: rc=%d out=%q err=%q", rc, out, errb)
	}
}
