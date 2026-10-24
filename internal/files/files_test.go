// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package files

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"hash/crc32"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"testing"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/runner"
	"github.com/mauriziofonte/gash/internal/term"
)

var fileHandlers = map[string]func(*invoke.Call) int{
	"archive_extract": archiveExtract,
	"file_backup":     fileBackup,
}

// runIn runs a handler with dir as the working directory. env adds to (or
// overrides) PATH and HOME; stdin may be nil.
func runIn(t *testing.T, dir string, env map[string]string, stdin io.Reader, fn string, args ...string) (string, string, int) {
	t.Helper()
	t.Chdir(dir)
	m := map[string]string{"PATH": os.Getenv("PATH"), "HOME": t.TempDir(), "PWD": dir, "GASH__HELP": "0"}
	for k, v := range env {
		m[k] = v
	}
	e := invoke.EnvMap(m)
	var out, errb bytes.Buffer
	c := &invoke.Call{
		Ctx: context.Background(), Name: fn, Args: args, Env: e, Stdin: stdin,
		Stdout: &out, Stderr: &errb, Term: term.Fixed(e, false, false, false),
	}
	rc := fileHandlers[fn](c)
	return out.String(), errb.String(), rc
}

// withRunner swaps the runner for the test.
func withRunner(t *testing.T, r runner.Runner) {
	t.Helper()
	old := runnerFor
	runnerFor = func(*invoke.Call) runner.Runner { return r }
	t.Cleanup(func() { runnerFor = old })
}

// stubBin makes a PATH directory holding empty executables of the given
// names (found by LookPath, never run: the Fake runner answers instead).
func stubBin(t *testing.T, names ...string) string {
	t.Helper()
	d := t.TempDir()
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(d, n), nil, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return d
}

// linkBin makes a PATH directory with symlinks to the named real tools.
func linkBin(t *testing.T, names ...string) string {
	t.Helper()
	d := t.TempDir()
	for _, n := range names {
		p, err := exec.LookPath(n)
		if err != nil {
			t.Skipf("%s not installed", n)
		}
		if err := os.Symlink(p, filepath.Join(d, n)); err != nil {
			t.Fatal(err)
		}
	}
	return d
}

// scriptBin makes a PATH directory with constant test scripts (a fixed
// `date`, a failing `cp`), placed before the real PATH.
func scriptBin(t *testing.T, scripts map[string]string) string {
	t.Helper()
	d := t.TempDir()
	for n, body := range scripts {
		if err := os.WriteFile(filepath.Join(d, n), []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return d + ":" + os.Getenv("PATH")
}

func need(t *testing.T, tools ...string) {
	t.Helper()
	for _, n := range tools {
		if _, err := exec.LookPath(n); err != nil {
			t.Skipf("%s not installed", n)
		}
	}
}

func gzipBytes(t *testing.T, s string) []byte {
	t.Helper()
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	_, _ = w.Write([]byte(s))
	_ = w.Close()
	return b.Bytes()
}

func write(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func size(t *testing.T, path string) int64 {
	t.Helper()
	st, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return st.Size()
}

// TestArchiveSuffixMatch pins the case-insensitive suffix table in the 2.x
// order (FILE-16: matched in Go, no shell option touched). 3.0: FILE-13
// fixed: .tar.Z and .taz are unpacked, not decompressed under their own
// name; .tbz and .tar.lz stay unsupported.
func TestArchiveSuffixMatch(t *testing.T) {
	cases := map[string]kind{
		"a.tar.bz2": kTarBz2, "A.TAR.BZ2": kTarBz2, "a.tar.gz": kTarGz, "a.Tar.Gz": kTarGz,
		"a.tar.xz": kTarXz, "a.txz": kTarXz, "a.tar.zst": kTarZst, "a.TZST": kTarZst,
		"a.xz": kSingleXz, "a.ZST": kSingleZst, "a.Bz2": kSingleBz2, "a.rar": kRar,
		"a.GZ": kSingleGz, "a.tar": kTarPlain, "a.tbz2": kTbz2, "a.tgz": kTgz, "a.ZIP": kZip,
		"a.Z": kSingleZ, "a.z": kSingleZ, "a.tar.Z": kTarZ, "a.TAZ": kTarZ, "a.7z": kSevenZip,
		"a.tbz": kUnsupported, "a.tar.lz": kUnsupported, "f.abc": kUnsupported,
	}
	for name, want := range cases {
		if got, _ := classify(name); got != want {
			t.Errorf("classify(%q) = %d, want %d", name, got, want)
		}
	}
}

// TestSingleFileName pins the output name of the single-file formats.
// 3.0: FILE-01, FILE-02 and FILE-15 fixed: the suffix is stripped
// case-insensitively, only when something remains, and a trailing newline
// stays in the name.
func TestSingleFileName(t *testing.T) {
	cases := []struct{ name, suffix, want string }{
		{"foo.Z", ".z", "foo"}, {"UP.GZ", ".gz", "UP"}, {"Cap.Bz2", ".bz2", "Cap"}, {"a.gz", ".gz", "a"},
		{".gz", ".gz", ".gz"}, {"dir/foo.gz", ".gz", "foo"}, {"-s.gz", ".gz", "-s"},
		{"foo\n.gz", ".gz", "foo\n"}, {"S.ZST", ".zst", "S"},
	}
	for _, c := range cases {
		if got := singleFileName(c.name, c.suffix); got != c.want {
			t.Errorf("singleFileName(%q, %q) = %q, want %q", c.name, c.suffix, got, c.want)
		}
	}
}

// compressBytes returns s in the format `uncompress` reads: real compress
// output when compress is installed (ncompress, or BSD compress on macOS),
// else gzip output, which gzip's uncompress wrapper (Linux) also reads.
func compressBytes(t *testing.T, s string) []byte {
	t.Helper()
	if _, err := exec.LookPath("compress"); err == nil {
		cmd := exec.Command("compress", "-c", "-f")
		cmd.Stdin = strings.NewReader(s)
		if out, err := cmd.Output(); err == nil {
			return out
		}
	}
	return gzipBytes(t, s)
}

// TestLedger_FILE01_fix: a .Z archive in the working directory is
// decompressed to its name without .Z and is left unchanged (3.0: FILE-01
// fixed; rc.A emptied it).
func TestLedger_FILE01_fix(t *testing.T) {
	need(t, "uncompress")
	d := t.TempDir()
	foo := compressBytes(t, "hi\n")
	write(t, filepath.Join(d, "foo.Z"), foo)
	_, errb, rc := runIn(t, d, nil, nil, "archive_extract", "foo.Z")
	got, _ := os.ReadFile(filepath.Join(d, "foo"))
	if rc != 0 || string(got) != "hi\n" || size(t, filepath.Join(d, "foo.Z")) != int64(len(foo)) {
		t.Errorf("foo.Z: rc=%d err=%q foo=%q", rc, errb, got)
	}
	if _, err := exec.LookPath("compress"); err != nil {
		t.Skip("compress not installed: real .Z data not tested")
	}
	cmd := exec.Command("compress", "-c", "-f")
	cmd.Stdin = strings.NewReader("real\n")
	real, err := cmd.Output()
	if err != nil {
		t.Skip("compress cannot write .Z data here")
	}
	write(t, filepath.Join(d, "real.Z"), real)
	_, errb, rc = runIn(t, d, nil, nil, "archive_extract", "real.Z", ".")
	got, _ = os.ReadFile(filepath.Join(d, "real"))
	if rc != 0 || string(got) != "real\n" || size(t, filepath.Join(d, "real.Z")) != int64(len(real)) {
		t.Errorf("real.Z: rc=%d err=%q real=%q", rc, errb, got)
	}
}

// TestLedger_FILE02_fix: upper- and mixed-case suffixes are stripped; a
// file named only like a suffix is refused, unchanged (3.0: FILE-02 fixed).
func TestLedger_FILE02_fix(t *testing.T) {
	type tc struct{ archive, out, tool string }
	for _, c := range []tc{{"UP.GZ", "UP", "gzip"}, {"BIG.XZ", "BIG", "xz"}, {"Cap.Bz2", "Cap", "bzip2"}, {"S.ZST", "S", "zstd"}} {
		if _, err := exec.LookPath(c.tool); err != nil {
			t.Logf("%s not installed: %s skipped", c.tool, c.archive)
			continue
		}
		d := t.TempDir()
		cmd := exec.Command(c.tool, "-c")
		cmd.Stdin = strings.NewReader("hi\n")
		data, err := cmd.Output()
		if err != nil {
			t.Fatal(err)
		}
		write(t, filepath.Join(d, c.archive), data)
		_, errb, rc := runIn(t, d, nil, nil, "archive_extract", c.archive)
		got, _ := os.ReadFile(filepath.Join(d, c.out))
		if rc != 0 || string(got) != "hi\n" || size(t, filepath.Join(d, c.archive)) != int64(len(data)) {
			t.Errorf("%s: rc=%d err=%q out=%q", c.archive, rc, errb, got)
		}
	}
	d := t.TempDir()
	data := gzipBytes(t, "hi\n")
	write(t, filepath.Join(d, ".gz"), data)
	withRunner(t, runner.NewFake(t)) // nothing may start
	_, errb, rc := runIn(t, d, map[string]string{"PATH": stubBin(t, "gunzip")}, nil, "archive_extract", ".gz")
	if rc != 1 || errb != "Error: Output './.gz' would overwrite the archive '.gz'.\n" ||
		size(t, filepath.Join(d, ".gz")) != int64(len(data)) {
		t.Errorf(".gz: rc=%d err=%q", rc, errb)
	}
}

// TestLedger_FILE03_fix: a corrupt archive or a missing tool never touches
// an existing output; a symlink at the output is replaced, not written
// through; a good archive still replaces the output (3.0: FILE-03 fixed).
func TestLedger_FILE03_fix(t *testing.T) {
	need(t, "gunzip")
	d := t.TempDir()
	write(t, filepath.Join(d, "s.txt.gz"), []byte("not gzip"))
	write(t, filepath.Join(d, "s.txt"), []byte("PRECIOUS\n"))
	_, errb, rc := runIn(t, d, nil, nil, "archive_extract", "s.txt.gz")
	if b, _ := os.ReadFile(filepath.Join(d, "s.txt")); rc != 1 || string(b) != "PRECIOUS\n" ||
		!strings.Contains(errb, "not in gzip format") {
		t.Errorf("corrupt: rc=%d err=%q s.txt=%q", rc, errb, b)
	}
	if m, _ := filepath.Glob(filepath.Join(d, ".axe-*")); len(m) != 0 {
		t.Errorf("temporary output left behind: %v", m)
	}

	e := t.TempDir()
	write(t, filepath.Join(e, "s.txt.gz"), gzipBytes(t, "hi\n"))
	_, errb, rc = runIn(t, e, map[string]string{"PATH": t.TempDir()}, nil, "archive_extract", "s.txt.gz")
	if _, err := os.Lstat(filepath.Join(e, "s.txt")); rc != 1 || err == nil ||
		errb != "Error: Command 'gunzip' is not installed or not in PATH.\n" {
		t.Errorf("missing gunzip: rc=%d err=%q created=%v", rc, errb, err == nil)
	}

	f := t.TempDir()
	mustMkdirs(t, f, "out", "victim")
	write(t, filepath.Join(f, "victim", "s.txt"), []byte("VICTIM\n"))
	if err := os.Symlink("../victim/s.txt", filepath.Join(f, "out", "s.txt")); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(f, "s.txt.gz"), gzipBytes(t, "hi\n"))
	_, errb, rc = runIn(t, f, nil, nil, "archive_extract", "s.txt.gz", "out")
	v, _ := os.ReadFile(filepath.Join(f, "victim", "s.txt"))
	st, err := os.Lstat(filepath.Join(f, "out", "s.txt"))
	o, _ := os.ReadFile(filepath.Join(f, "out", "s.txt"))
	if rc != 0 || string(v) != "VICTIM\n" || err != nil || !st.Mode().IsRegular() || string(o) != "hi\n" {
		t.Errorf("symlink target: rc=%d err=%q victim=%q out=%q", rc, errb, v, o)
	}

	write(t, filepath.Join(d, "s.txt.gz"), gzipBytes(t, "NEW\n"))
	if _, errb, rc := runIn(t, d, nil, nil, "archive_extract", "s.txt.gz"); rc != 0 {
		t.Errorf("good archive: rc=%d err=%q", rc, errb)
	}
	if b, _ := os.ReadFile(filepath.Join(d, "s.txt")); string(b) != "NEW\n" {
		t.Errorf("good archive did not replace s.txt: %q", b)
	}
}

// TestLedger_FILE15_fix: a trailing newline stays in the output name, so an
// existing name without it is untouched (3.0: FILE-15 fixed).
func TestLedger_FILE15_fix(t *testing.T) {
	need(t, "gunzip")
	d := t.TempDir()
	write(t, filepath.Join(d, "foo\n.gz"), gzipBytes(t, "hi\n"))
	write(t, filepath.Join(d, "foo"), []byte("KEEP"))
	_, _, rc := runIn(t, d, nil, nil, "archive_extract", "foo\n.gz")
	got, err := os.ReadFile(filepath.Join(d, "foo\n"))
	keep, _ := os.ReadFile(filepath.Join(d, "foo"))
	if rc != 0 || err != nil || string(got) != "hi\n" || string(keep) != "KEEP" {
		t.Errorf("rc=%d foo\\n=%q err=%v foo=%q", rc, got, err, keep)
	}
}

// TestLedger_FILE07_fix: nothing is created when the type is unsupported or
// the branch's tool is missing (3.0: FILE-07 fixed).
func TestLedger_FILE07_fix(t *testing.T) {
	d := t.TempDir()
	write(t, filepath.Join(d, "f.abc"), []byte("x"))
	write(t, filepath.Join(d, "a.tar.zst"), []byte("x"))
	write(t, filepath.Join(d, "a.zip"), []byte("x"))
	withRunner(t, runner.NewFake(t)) // nothing may start, mkdir included
	for _, c := range []struct{ archive, path, err string }{
		{"f.abc", os.Getenv("PATH"), "Error: Cannot extract 'f.abc', unsupported file type.\n"},
		{"a.tar.zst", stubBin(t, "tar", "mkdir"), "Error: Extracting .zst needs zstd.\n"},
		{"a.zip", stubBin(t, "mkdir"), "Error: Command 'unzip' is not installed or not in PATH.\n"},
	} {
		_, errb, rc := runIn(t, d, map[string]string{"PATH": c.path}, nil, "archive_extract", c.archive, "newdir")
		if rc != 1 || errb != c.err {
			t.Errorf("%s: rc=%d err=%q", c.archive, rc, errb)
		}
		if _, err := os.Lstat(filepath.Join(d, "newdir")); err == nil {
			t.Errorf("%s: newdir created", c.archive)
		}
	}
}

// TestLedger_FILE12_fix: existing non-regular paths are named as such; a
// missing path keeps "does not exist" (3.0: FILE-12 fixed).
func TestLedger_FILE12_fix(t *testing.T) {
	d := t.TempDir()
	if err := os.Mkdir(filepath.Join(d, "d.zip"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(d, "p.zip"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("missing", filepath.Join(d, "l.zip")); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"d.zip", "p.zip", "l.zip"} {
		for fn := range fileHandlers {
			_, errb, rc := runIn(t, d, nil, nil, fn, n)
			if rc != 1 || errb != "Error: '"+n+"' is not a regular file.\n" {
				t.Errorf("%s %s: rc=%d err=%q", fn, n, rc, errb)
			}
		}
	}
	for fn := range fileHandlers {
		_, errb, rc := runIn(t, d, nil, nil, fn, "nope.zip")
		if rc != 1 || errb != "Error: File 'nope.zip' does not exist.\n" {
			t.Errorf("%s missing: rc=%d err=%q", fn, rc, errb)
		}
	}
	_, errb, rc := runIn(t, d, nil, nil, "archive_extract")
	if rc != 1 || errb != "Error: Missing archive file. Usage: archive_extract <archive_file> [output_dir]\n" {
		t.Errorf("missing arg: rc=%d err=%q", rc, errb)
	}
	_, errb, rc = runIn(t, d, nil, nil, "file_backup")
	if rc != 1 || errb != "Error: Missing file. Usage: file_backup <file>\n" {
		t.Errorf("missing arg: rc=%d err=%q", rc, errb)
	}
}

func mustMkdirs(t *testing.T, base string, rels ...string) {
	t.Helper()
	for _, r := range rels {
		if err := os.MkdirAll(filepath.Join(base, r), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

// TestLedger_FILE17 pins pipefail semantics for .tar.zst.
func TestLedger_FILE17(t *testing.T) {
	for _, c := range []struct{ zstd, tar, want int }{{1, 0, 1}, {0, 1, 1}, {0, 0, 0}} {
		d := t.TempDir()
		write(t, filepath.Join(d, "a.tar.zst"), []byte("x"))
		if err := os.Mkdir(filepath.Join(d, "out"), 0o755); err != nil {
			t.Fatal(err)
		}
		f := runner.NewFake(t,
			runner.Expect{Argv: []string{"zstd", "-dc", "--", "a.tar.zst"}}, // FILE-22 listing
			runner.Expect{Argv: []string{"zstd", "-dc", "--", "a.tar.zst"}, Status: c.zstd},
			runner.Expect{Argv: []string{"tar", "xvf", "-", "-C", "out", "--no-same-owner", "--no-same-permissions"}, Status: c.tar},
		)
		withRunner(t, f)
		_, _, rc := runIn(t, d, map[string]string{"PATH": stubBin(t, "zstd", "tar")}, nil, "archive_extract", "a.tar.zst", "out")
		if rc != c.want {
			t.Errorf("zstd %d / tar %d: rc=%d, want %d", c.zstd, c.tar, rc, c.want)
		}
	}
}

// TestLedger_FILE19 pins the missing-tool lines. 3.0: FILE-03 and FILE-07
// fixed: the tools are checked before anything is created.
func TestLedger_FILE19(t *testing.T) {
	t.Run("gunzip", func(t *testing.T) {
		d := t.TempDir()
		write(t, filepath.Join(d, "s.txt.gz"), gzipBytes(t, "hi\n"))
		_, errb, rc := runIn(t, d, map[string]string{"PATH": t.TempDir()}, nil, "archive_extract", "s.txt.gz")
		if rc != 1 || errb != "Error: Command 'gunzip' is not installed or not in PATH.\n" {
			t.Errorf("rc=%d err=%q", rc, errb)
		}
		if _, err := os.Lstat(filepath.Join(d, "s.txt")); err == nil {
			t.Error("no output may be created when the tool is missing")
		}
	})
	t.Run("unzip", func(t *testing.T) {
		d := t.TempDir()
		writeZip(t, filepath.Join(d, "a.zip"), map[string]string{"src/a.txt": "a"}, nil)
		_, errb, rc := runIn(t, d, map[string]string{"PATH": linkBin(t, "mkdir")}, nil, "archive_extract", "a.zip", "newdir")
		if rc != 1 || errb != "Error: Command 'unzip' is not installed or not in PATH.\n" {
			t.Errorf("rc=%d err=%q", rc, errb)
		}
		if _, err := os.Lstat(filepath.Join(d, "newdir")); err == nil {
			t.Error("OUTPUT_DIR may not be created when the tool is missing")
		}
	})
	t.Run("zstd", func(t *testing.T) {
		d := t.TempDir()
		write(t, filepath.Join(d, "s.zst"), []byte("x"))
		_, errb, rc := runIn(t, d, map[string]string{"PATH": t.TempDir()}, nil, "archive_extract", "s.zst")
		if rc != 1 || errb != "Error: Extracting .zst needs zstd.\n" {
			t.Errorf("rc=%d err=%q", rc, errb)
		}
		if _, err := os.Stat(filepath.Join(d, "s")); err == nil {
			t.Error("no output may be opened before the zstd check")
		}
	})
	t.Run("tar for .tar.zst", func(t *testing.T) {
		d := t.TempDir()
		write(t, filepath.Join(d, "a.tar.zst"), []byte("x"))
		if err := os.Mkdir(filepath.Join(d, "out"), 0o755); err != nil {
			t.Fatal(err)
		}
		withRunner(t, runner.NewFake(t)) // no start at all: zstd is never run
		_, errb, rc := runIn(t, d, map[string]string{"PATH": stubBin(t, "zstd")}, nil, "archive_extract", "a.tar.zst", "out")
		if rc != 1 || errb != "Error: Command 'tar' is not installed or not in PATH.\n" {
			t.Errorf("rc=%d err=%q", rc, errb)
		}
	})
}

// TestLedger_FILE23 pins option (a): every non-zero tool status is 1.
func TestLedger_FILE23(t *testing.T) {
	d := t.TempDir()
	for _, n := range []string{"a.tgz", "a.zip", "a.7z"} {
		write(t, filepath.Join(d, n), []byte("x"))
	}
	if err := os.Mkdir(filepath.Join(d, "o"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := stubBin(t, "tar", "unzip", "7z")
	sevenList := runner.Expect{Argv: []string{"7z", "l", "-slt", "-ba", "--", "a.7z"}}
	sevenX := []string{"7z", "x", "-spd", "-aos", "-oo", "--", "a.7z"}
	cases := []struct {
		archive string
		expects []runner.Expect
		want    int
	}{
		{"a.tgz", []runner.Expect{{Argv: []string{"tar", "xvzf", "a.tgz", "-C", "o", "--no-same-owner", "--no-same-permissions"}, Status: 2}}, 1},
		{"a.zip", []runner.Expect{{Argv: []string{"unzip", "-n", "a.zip", "-d", "o"}, Status: 1}}, 1},
		{"a.7z", []runner.Expect{sevenList, {Argv: sevenX, Status: 0}}, 0},
		{"a.7z", []runner.Expect{sevenList, {Argv: sevenX, Status: 2}}, 1},
	}
	for _, c := range cases {
		withRunner(t, runner.NewFake(t, c.expects...))
		if _, _, rc := runIn(t, d, map[string]string{"PATH": path}, nil, "archive_extract", c.archive, "o"); rc != c.want {
			t.Errorf("%s: rc=%d, want %d", c.archive, rc, c.want)
		}
	}
}

// TestLedger_FILE24_2x: the tools get the caller's environment and HOME
// (option a), minus the stub's protocol variables.
func TestLedger_FILE24_2x(t *testing.T) {
	d := t.TempDir()
	write(t, filepath.Join(d, "a.tgz"), []byte("x"))
	if err := os.Mkdir(filepath.Join(d, "o"), 0o755); err != nil {
		t.Fatal(err)
	}
	withRunner(t, runner.NewFake(t, runner.Expect{
		Argv:     []string{"tar", "xvzf", "a.tgz", "-C", "o", "--no-same-owner", "--no-same-permissions"},
		EnvHas:   []string{"TAR_OPTIONS=--keep-old-files", "HOME=/home/u", "UNZIP=-o"},
		EnvLacks: []string{"GASH__HELP"},
	}))
	_, _, rc := runIn(t, d, map[string]string{
		"PATH": stubBin(t, "tar"), "HOME": "/home/u", "TAR_OPTIONS": "--keep-old-files", "UNZIP": "-o",
	}, nil, "archive_extract", "a.tgz", "o")
	if rc != 0 {
		t.Errorf("rc=%d", rc)
	}
}

// TestLedger_FILE08_fix: an option-like OUTPUT_DIR is a directory name for
// mkdir and for every tool (3.0: FILE-08 fixed).
func TestLedger_FILE08_fix(t *testing.T) {
	d := t.TempDir()
	write(t, filepath.Join(d, "a.tgz"), []byte("x"))
	f := runner.NewFake(t,
		runner.Expect{Argv: []string{"mkdir", "-p", "--", "--help"}},
		runner.Expect{Argv: []string{"tar", "xvzf", "a.tgz", "-C", "./--help", "--no-same-owner", "--no-same-permissions"}},
	)
	withRunner(t, f)
	if _, _, rc := runIn(t, d, map[string]string{"PATH": stubBin(t, "tar", "mkdir")}, nil, "archive_extract", "a.tgz", "--help"); rc != 0 {
		t.Errorf("fake: rc=%d", rc)
	}

	withRunner(t, runner.Exec{})
	need(t, "tar", "gzip", "mkdir")
	e := t.TempDir()
	writeTar(t, filepath.Join(e, "a.tgz"), true, []tarEntry{{name: "s", flag: tar.TypeReg}})
	for _, dir := range []string{"--help", "-x"} {
		out, errb, rc := runIn(t, e, nil, nil, "archive_extract", "a.tgz", dir)
		if rc != 0 || strings.Contains(out, "Usage: mkdir") {
			t.Errorf("%s: rc=%d out=%q err=%q", dir, rc, out, errb)
		}
		if _, err := os.Stat(filepath.Join(e, dir, "s")); err != nil {
			t.Errorf("%s/s not extracted: %v", dir, err)
		}
	}
}

// writeRar writes a RAR 4 archive of stored (uncompressed) Unix members:
// the minimal block layout unrar reads, so the tests need no rar binary.
// A member whose mode is a symlink stores its target as the data.
func writeRar(t *testing.T, path string, members []rarMember) {
	t.Helper()
	crc16 := func(b []byte) uint16 { return uint16(crc32.ChecksumIEEE(b)) }
	var out bytes.Buffer
	out.WriteString("Rar!\x1a\x07\x00")
	main := []byte{0x73, 0, 0, 13, 0, 0, 0, 0, 0, 0, 0}
	_ = binary.Write(&out, binary.LittleEndian, crc16(main))
	out.Write(main)
	for _, m := range members {
		var h bytes.Buffer
		data := []byte(m.data)
		h.WriteByte(0x74)
		_ = binary.Write(&h, binary.LittleEndian, uint16(0x8000))
		_ = binary.Write(&h, binary.LittleEndian, uint16(32+len(m.name)))
		_ = binary.Write(&h, binary.LittleEndian, uint32(len(data)))
		_ = binary.Write(&h, binary.LittleEndian, uint32(len(data)))
		h.WriteByte(3) // host OS: Unix
		_ = binary.Write(&h, binary.LittleEndian, crc32.ChecksumIEEE(data))
		_ = binary.Write(&h, binary.LittleEndian, uint32(0x5a210000)) // DOS time
		h.WriteByte(20)                                               // version needed
		h.WriteByte(0x30)                                             // method: store
		_ = binary.Write(&h, binary.LittleEndian, uint16(len(m.name)))
		_ = binary.Write(&h, binary.LittleEndian, m.mode)
		h.WriteString(m.name)
		_ = binary.Write(&out, binary.LittleEndian, crc16(h.Bytes()))
		out.Write(h.Bytes())
		out.Write(data)
	}
	out.Write([]byte{0xc4, 0x3d, 0x7b, 0x00, 0x40, 0x07, 0x00})
	write(t, path, out.Bytes())
}

type rarMember struct {
	name, data string
	mode       uint32
}

// TestLedger_FILE09_fix: unrar reads an existing dash-named OUTPUT_DIR as
// the destination, not as a switch (3.0: FILE-09 fixed).
func TestLedger_FILE09_fix(t *testing.T) {
	d := t.TempDir()
	write(t, filepath.Join(d, "a.rar"), []byte("x"))
	if err := os.Mkdir(filepath.Join(d, "-x"), 0o755); err != nil {
		t.Fatal(err)
	}
	withRunner(t, runner.NewFake(t,
		runner.Expect{Argv: []string{"unrar", "lt", "--", "a.rar"}},
		runner.Expect{Argv: []string{"unrar", "x", "-o-", "-p-", "--", "a.rar", "./-x/"}},
	))
	if _, _, rc := runIn(t, d, map[string]string{"PATH": stubBin(t, "unrar")}, nil, "archive_extract", "a.rar", "-x"); rc != 0 {
		t.Errorf("fake: rc=%d", rc)
	}

	withRunner(t, runner.Exec{})
	need(t, "unrar")
	e := t.TempDir()
	writeRar(t, filepath.Join(e, "plain.rar"), []rarMember{{name: "src/a.txt", data: "hello\n", mode: 0o100644}})
	if err := os.Mkdir(filepath.Join(e, "-x"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, errb, rc := runIn(t, e, nil, nil, "archive_extract", "plain.rar", "-x")
	if b, err := os.ReadFile(filepath.Join(e, "-x", "src", "a.txt")); rc != 0 || err != nil || string(b) != "hello\n" {
		t.Errorf("rc=%d err=%q content=%q", rc, errb, b)
	}
	if _, err := os.Lstat(filepath.Join(e, "src")); err == nil {
		t.Error("unrar extracted into the working directory")
	}
}

// TestLedger_FILE10_fix: unzip and unrar names with wildcard characters are
// refused before anything runs; 7z reads the name literally (3.0: FILE-10
// fixed).
func TestLedger_FILE10_fix(t *testing.T) {
	d := t.TempDir()
	writeZip(t, filepath.Join(d, "p[1].zip"), map[string]string{"A/a": "a"}, nil)
	writeZip(t, filepath.Join(d, "p1.zip"), map[string]string{"Bd/b": "b"}, nil)
	write(t, filepath.Join(d, "r?.rar"), []byte("x"))
	withRunner(t, runner.NewFake(t)) // nothing may start
	path := stubBin(t, "unzip", "unrar", "mkdir")
	for _, c := range []struct{ archive, tool string }{{"p[1].zip", "unzip"}, {"r?.rar", "unrar"}} {
		_, errb, rc := runIn(t, d, map[string]string{"PATH": path}, nil, "archive_extract", c.archive, "o")
		want := "Error: Archive name '" + c.archive + "' contains wildcard characters (* ? [) that " + c.tool +
			" would expand; rename it.\n"
		if rc != 1 || errb != want {
			t.Errorf("%s: rc=%d err=%q", c.archive, rc, errb)
		}
	}
	if _, err := os.Lstat(filepath.Join(d, "o")); err == nil {
		t.Error("OUTPUT_DIR created for a refused name")
	}

	withRunner(t, runner.Exec{})
	need(t, "7z")
	e := t.TempDir()
	// y?.7z first: p7zip 17.05 (a macOS runner's 7z) matches its own archive
	// name as a wildcard when a yb.7z already exists, whatever -spd says.
	for _, a := range []struct{ name, member string }{{"y?.7z", "Y"}, {"yb.7z", "B"}} {
		src := t.TempDir()
		write(t, filepath.Join(src, a.member), []byte(a.member))
		cmd := exec.Command("7z", "a", "-bd", "-spd", "--", filepath.Join(e, a.name), filepath.Join(src, a.member))
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("7z a: %v %s", err, out)
		}
	}
	_, errb, rc := runIn(t, e, nil, nil, "archive_extract", "y?.7z", "o")
	if rc != 0 {
		t.Errorf("y?.7z: rc=%d err=%q", rc, errb)
	}
	if _, err := os.Stat(filepath.Join(e, "o", "Y")); err != nil {
		t.Error("y?.7z not extracted")
	}
	if _, err := os.Lstat(filepath.Join(e, "o", "B")); err == nil {
		t.Error("yb.7z extracted through a wildcard")
	}
}

// TestLedger_FILE11_fix: headless, or with stdin not a terminal, the
// prompting tools get /dev/null, a new session and their never-overwrite,
// never-ask switches; on a terminal outside headless the 2.x argv and
// stdin stay (3.0: FILE-11 fixed).
func TestLedger_FILE11_fix(t *testing.T) {
	d := t.TempDir()
	for _, n := range []string{"a.rar", "a.zip", "a.7z"} {
		write(t, filepath.Join(d, n), []byte("x"))
	}
	if err := os.Mkdir(filepath.Join(d, "o"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := stubBin(t, "unrar", "unzip", "7z")
	headless := []runner.Expect{
		{Argv: []string{"unrar", "lt", "--", "a.rar"}},
		{Argv: []string{"unrar", "x", "-o-", "-p-", "--", "a.rar", "o/"}},
		{Argv: []string{"unzip", "-n", "a.zip", "-d", "o"}},
		{Argv: []string{"7z", "l", "-slt", "-ba", "--", "a.7z"}},
		{Argv: []string{"7z", "x", "-spd", "-aos", "-oo", "--", "a.7z"}},
	}
	f := runner.NewFake(t, headless...)
	withRunner(t, f)
	for _, a := range []string{"a.rar", "a.zip", "a.7z"} {
		if _, _, rc := runIn(t, d, map[string]string{"PATH": path, "GASH_HEADLESS": "1"}, strings.NewReader("A\n"), "archive_extract", a, "o"); rc != 0 {
			t.Errorf("%s: rc=%d", a, rc)
		}
	}
	for _, call := range f.Calls() {
		if call.Argv[1] == "lt" || call.Argv[1] == "l" {
			continue
		}
		if !call.Setsid || len(call.Stdin) != 0 {
			t.Errorf("%v: setsid=%v stdin=%q, want a new session and no input", call.Argv, call.Setsid, call.Stdin)
		}
	}

	// A terminal outside headless keeps the prompts and the caller's stdin.
	tty := runner.NewFake(t,
		runner.Expect{Argv: []string{"unzip", "a.zip", "-d", "o"}, Stdin: runner.Text("A\n")},
	)
	withRunner(t, tty)
	t.Chdir(d)
	e := invoke.EnvMap(map[string]string{"PATH": path, "HOME": t.TempDir(), "PWD": d, "GASH__HELP": "0"})
	var out, errb bytes.Buffer
	c := &invoke.Call{
		Ctx: context.Background(), Name: "archive_extract", Args: []string{"a.zip", "o"}, Env: e,
		Stdin: strings.NewReader("A\n"), Stdout: &out, Stderr: &errb, Term: term.Fixed(e, true, true, true),
	}
	if rc := archiveExtract(c); rc != 0 {
		t.Errorf("tty: rc=%d err=%q", rc, errb.String())
	}
	if calls := tty.Calls(); len(calls) != 1 || calls[0].Setsid {
		t.Errorf("tty: calls %+v", calls)
	}
}

// TestLedger_FILE13_fix: .tar.Z and .taz are unpacked into OUTPUT_DIR and
// left unchanged (3.0: FILE-13 fixed).
func TestLedger_FILE13_fix(t *testing.T) {
	need(t, "uncompress", "tar")
	d := t.TempDir()
	var tb bytes.Buffer
	tw := tar.NewWriter(&tb)
	for _, h := range []*tar.Header{
		{Name: "src/", Mode: 0o755, Typeflag: tar.TypeDir},
		{Name: "src/a.txt", Mode: 0o644, Size: 2, Typeflag: tar.TypeReg},
	} {
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if h.Size > 0 {
			_, _ = tw.Write([]byte("A\n"))
		}
	}
	_ = tw.Close()
	data := compressBytes(t, tb.String())
	for _, n := range []string{"a.tar.Z", "a.taz"} {
		write(t, filepath.Join(d, n), data)
		out := "o-" + n
		_, errb, rc := runIn(t, d, nil, nil, "archive_extract", n, out)
		b, err := os.ReadFile(filepath.Join(d, out, "src", "a.txt"))
		if rc != 0 || err != nil || string(b) != "A\n" || size(t, filepath.Join(d, n)) != int64(len(data)) {
			t.Errorf("%s: rc=%d err=%q content=%q", n, rc, errb, b)
		}
	}
}

// TestLedger_FILE14_fix: uncompress gets -c before the operand and --, so a
// BSD getopt never reads -c as a file (3.0: FILE-14 fixed). A name without
// the .Z suffix goes on stdin, since a BSD uncompress would look for NAME.Z.
func TestLedger_FILE14_fix(t *testing.T) {
	for _, tc := range []struct {
		name string
		want runner.Expect
	}{
		{"s.Z", runner.Expect{Argv: []string{"uncompress", "-c", "--", "s.Z"}, Stdout: "hi\n"}},
		{"s.z", runner.Expect{Argv: []string{"uncompress", "-c"}, Stdin: runner.Text("x"), Stdout: "hi\n"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := t.TempDir()
			write(t, filepath.Join(d, tc.name), []byte("x"))
			if err := os.Mkdir(filepath.Join(d, "o"), 0o755); err != nil {
				t.Fatal(err)
			}
			withRunner(t, runner.NewFake(t, tc.want))
			if _, _, rc := runIn(t, d, map[string]string{"PATH": stubBin(t, "uncompress")}, nil, "archive_extract", tc.name, "o"); rc != 0 {
				t.Errorf("rc=%d", rc)
			}
			if b, _ := os.ReadFile(filepath.Join(d, "o", "s")); string(b) != "hi\n" {
				t.Errorf("output %q", b)
			}
			if b, _ := os.ReadFile(filepath.Join(d, tc.name)); string(b) != "x" {
				t.Errorf("archive changed: %q", b)
			}
		})
	}
}

type tarEntry struct {
	name, link string
	flag       byte
}

func writeTar(t *testing.T, path string, gz bool, entries []tarEntry) {
	t.Helper()
	var buf bytes.Buffer
	var w io.Writer = &buf
	var zw *gzip.Writer
	if gz {
		zw = gzip.NewWriter(&buf)
		w = zw
	}
	tw := tar.NewWriter(w)
	for _, e := range entries {
		h := &tar.Header{Name: e.name, Mode: 0o644, Typeflag: e.flag, Linkname: e.link}
		if e.flag == tar.TypeReg {
			h.Size = 1
		}
		if e.flag == tar.TypeDir {
			h.Mode = 0o755
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if e.flag == tar.TypeReg {
			_, _ = tw.Write([]byte("M"))
		}
	}
	_ = tw.Close()
	if zw != nil {
		_ = zw.Close()
	}
	write(t, path, buf.Bytes())
}

func writeZip(t *testing.T, path string, files map[string]string, links map[string]string) {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for n, body := range files {
		w, err := zw.Create(n)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(body))
	}
	for n, target := range links {
		h := &zip.FileHeader{Name: n, Method: zip.Store}
		h.SetMode(os.ModeSymlink | 0o777)
		w, err := zw.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(target))
	}
	_ = zw.Close()
	write(t, path, buf.Bytes())
}

// TestLedger_FILE22_fix pins option (b): hostile members are refused before
// OUTPUT_DIR exists and nothing lands outside it; clean archives extract.
func TestLedger_FILE22_fix(t *testing.T) {
	reg, sym, hard := byte(tar.TypeReg), byte(tar.TypeSymlink), byte(tar.TypeLink)
	hostile := []struct {
		archive, member string
		make            func(path string)
	}{
		{"dotdot.tar", "../M", func(p string) { writeTar(t, p, false, []tarEntry{{name: "../M", flag: reg}}) }},
		{"abs.tar", "/tmp/gash-guard-M", func(p string) {
			writeTar(t, p, false, []tarEntry{{name: "/tmp/gash-guard-M", flag: reg}})
		}},
		{"symout.tar", "l", func(p string) { writeTar(t, p, false, []tarEntry{{name: "l", link: "../..", flag: sym}}) }},
		{"symabs.tgz", "l", func(p string) { writeTar(t, p, true, []tarEntry{{name: "l", link: "/etc", flag: sym}}) }},
		{"below.tar", "l/M", func(p string) {
			writeTar(t, p, false, []tarEntry{{name: "d/", flag: tar.TypeDir}, {name: "l", link: "d", flag: sym}, {name: "l/M", flag: reg}})
		}},
		{"hard.tar", "h", func(p string) { writeTar(t, p, false, []tarEntry{{name: "h", link: "../x", flag: hard}}) }},
		{"dotdot.tgz", "a/../../M", func(p string) { writeTar(t, p, true, []tarEntry{{name: "a/../../M", flag: reg}}) }},
		{"dotdot.zip", "../M", func(p string) { writeZip(t, p, map[string]string{"../M": "M"}, nil) }},
		{"abs.zip", "/tmp/gash-guard-M", func(p string) { writeZip(t, p, map[string]string{"/tmp/gash-guard-M": "M"}, nil) }},
		{"symout.zip", "l", func(p string) { writeZip(t, p, nil, map[string]string{"l": "../../outside"}) }},
	}
	sandbox := t.TempDir()
	run := filepath.Join(sandbox, "run")
	if err := os.Mkdir(run, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, h := range hostile {
		h.make(filepath.Join(run, h.archive))
		_, errb, rc := runIn(t, run, nil, nil, "archive_extract", h.archive, "out")
		want := "Error: Archive '" + h.archive + "' has an unsafe member '" + h.member +
			"' (absolute path, .. component or link outside the destination); nothing extracted.\n"
		if rc != 1 || errb != want {
			t.Errorf("%s: rc=%d err=%q", h.archive, rc, errb)
		}
		if _, err := os.Lstat(filepath.Join(run, "out")); err == nil {
			t.Errorf("%s: OUTPUT_DIR was created", h.archive)
		}
	}
	if _, err := os.Lstat(filepath.Join(sandbox, "M")); err == nil {
		t.Error("a marker escaped the destination")
	}

	// 7z with absolute stored paths (-spf), when 7-Zip is installed.
	if _, err := exec.LookPath("7z"); err == nil {
		src := filepath.Join(sandbox, "abs-src")
		_ = os.Mkdir(src, 0o755)
		write(t, filepath.Join(src, "M"), []byte("M"))
		cmd := exec.Command("7z", "a", "-bd", "-spf", filepath.Join(run, "abs.7z"), filepath.Join(src, "M"))
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("7z a: %v %s", err, out)
		}
		_, errb, rc := runIn(t, run, nil, nil, "archive_extract", "abs.7z", "out")
		if rc != 1 || !strings.HasPrefix(errb, "Error: Archive 'abs.7z' has an unsafe member '/") {
			t.Errorf("abs.7z: rc=%d err=%q", rc, errb)
		}
	}

	// 7z with a stored symlink pointing outside (-snl): its target is the
	// member's content, read before anything is extracted.
	if _, err := exec.LookPath("7z"); err == nil {
		src := filepath.Join(sandbox, "snl-src")
		_ = os.Mkdir(src, 0o755)
		if err := os.Symlink("../../outside", filepath.Join(src, "l")); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("7z", "a", "-bd", "-snl", filepath.Join(run, "snl.7z"), filepath.Join(src, "l"))
		if out, err := cmd.CombinedOutput(); err != nil {
			// p7zip 17.05 (Homebrew, macOS) follows the link despite -snl
			// and cannot store a dangling one: no fixture to test with.
			t.Logf("7z a -snl cannot store the symlink here, skipping the snl.7z case: %v %s", err, out)
		} else {
			_, errb, rc := runIn(t, run, nil, nil, "archive_extract", "snl.7z", "out")
			if rc != 1 || errb != "Error: Archive 'snl.7z' has an unsafe member 'l' (absolute path, .. component or link outside the destination); nothing extracted.\n" {
				t.Errorf("snl.7z: rc=%d err=%q", rc, errb)
			}
		}
	}

	// rar 4 with a stored Unix symlink pointing outside.
	if _, err := exec.LookPath("unrar"); err == nil {
		writeRar(t, filepath.Join(run, "sym.rar"), []rarMember{
			{name: "l", data: "../../outside", mode: 0o120777},
			{name: "l/M", data: "M", mode: 0o100644},
		})
		_, errb, rc := runIn(t, run, nil, nil, "archive_extract", "sym.rar", "out")
		if rc != 1 || errb != "Error: Archive 'sym.rar' has an unsafe member 'l' (absolute path, .. component or link outside the destination); nothing extracted.\n" {
			t.Errorf("sym.rar: rc=%d err=%q", rc, errb)
		}
	}
	if _, err := os.Lstat(filepath.Join(run, "out")); err == nil {
		t.Error("OUTPUT_DIR was created for a refused archive")
	}
	if _, err := os.Lstat(filepath.Join(filepath.Dir(sandbox), "outside")); err == nil {
		t.Error("a link target outside the sandbox was created")
	}

	// Clean archives still extract.
	need(t, "tar", "unzip", "gzip")
	writeTar(t, filepath.Join(run, "clean.tgz"), true, []tarEntry{
		{name: "src/", flag: tar.TypeDir}, {name: "src/a.txt", flag: reg}, {name: "src/l", link: "a.txt", flag: sym},
	})
	writeZip(t, filepath.Join(run, "clean.zip"), map[string]string{"z/a.txt": "M"}, nil)
	for _, a := range []string{"clean.tgz", "clean.zip"} {
		if _, errb, rc := runIn(t, run, nil, nil, "archive_extract", a, "ok"); rc != 0 {
			t.Errorf("%s: rc=%d err=%q", a, rc, errb)
		}
	}
	for _, p := range []string{"ok/src/a.txt", "ok/z/a.txt"} {
		if _, err := os.Stat(filepath.Join(run, p)); err != nil {
			t.Errorf("%s not extracted", p)
		}
	}
}

var stampRe = regexp.MustCompile(`^a_backup_[0-9]{14}$`)

// TestLedger_FILE04_fix: backups within one stamp never overwrite each
// other: the second gets _1, the third _2 (3.0: FILE-04 fixed). The fixed
// `date` stands for both the same second and the two instants of a DST
// fall-back hour, which print the same stamp.
func TestLedger_FILE04_fix(t *testing.T) {
	need(t, "cp")
	path := scriptBin(t, map[string]string{"date": "echo 20240101000000"})
	d := t.TempDir()
	for i, content := range []string{"1", "2", "3"} {
		write(t, filepath.Join(d, "a"), []byte(content))
		out, errb, rc := runIn(t, d, map[string]string{"PATH": path + ":" + os.Getenv("PATH")}, nil, "file_backup", "a")
		name := "a_backup_20240101000000"
		if i > 0 {
			name += "_" + string(rune('0'+i))
		}
		// cp -v: GNU quotes both names, BSD (macOS) does not.
		if rc != 0 || (out != "'a' -> '"+name+"'\n" && out != "a -> "+name+"\n") || errb != "" {
			t.Errorf("call %d: rc=%d out=%q err=%q", i+1, rc, out, errb)
		}
		if b, _ := os.ReadFile(filepath.Join(d, name)); string(b) != content {
			t.Errorf("%s holds %q, want %q", name, b, content)
		}
	}
	if m, _ := filepath.Glob(filepath.Join(d, "a_backup_*")); len(m) != 3 {
		t.Errorf("backups %v, want 3", m)
	}
}

// TestLedger_FILE05_fix: an entry already at the backup name (a symlink, a
// hard link, a directory, a dangling symlink) is never opened: the backup
// goes to the next suffix (3.0: FILE-05 fixed).
func TestLedger_FILE05_fix(t *testing.T) {
	need(t, "cp")
	path := scriptBin(t, map[string]string{"date": "echo 20240101000000"})
	env := map[string]string{"PATH": path + ":" + os.Getenv("PATH")}
	const name = "a_backup_20240101000000"
	for _, c := range []struct {
		kind string
		make func(d string)
	}{
		{"symlink", func(d string) { must(t, os.Symlink("victim", filepath.Join(d, name))) }},
		{"hard link", func(d string) { must(t, os.Link(filepath.Join(d, "victim"), filepath.Join(d, name))) }},
		{"directory", func(d string) { must(t, os.Mkdir(filepath.Join(d, name), 0o755)) }},
		{"dangling symlink", func(d string) { must(t, os.Symlink("missing", filepath.Join(d, name))) }},
	} {
		d := t.TempDir()
		write(t, filepath.Join(d, "a"), []byte("NEW"))
		write(t, filepath.Join(d, "victim"), []byte("OLD"))
		c.make(d)
		_, errb, rc := runIn(t, d, env, nil, "file_backup", "a")
		v, _ := os.ReadFile(filepath.Join(d, "victim"))
		b, _ := os.ReadFile(filepath.Join(d, name+"_1"))
		if rc != 0 || string(v) != "OLD" || string(b) != "NEW" {
			t.Errorf("%s: rc=%d err=%q victim=%q backup=%q", c.kind, rc, errb, v, b)
		}
		if c.kind == "directory" {
			if m, _ := filepath.Glob(filepath.Join(d, name, "*")); len(m) != 0 {
				t.Errorf("directory: copied into it: %v", m)
			}
		}
		if _, err := os.Lstat(filepath.Join(d, "missing")); err == nil {
			t.Errorf("%s: the dangling symlink was written through", c.kind)
		}
	}
}

// TestLedger_FILE06_fix: a failing cp is a failure, and its partial file is
// removed (3.0: FILE-06 fixed).
func TestLedger_FILE06_fix(t *testing.T) {
	d := t.TempDir()
	write(t, filepath.Join(d, "a"), []byte("x"))
	write(t, filepath.Join(d, "a_backup_20240101000000"), []byte("old"))
	withRunner(t, runner.NewFake(t,
		runner.Expect{Argv: []string{"date", "+%Y%m%d%H%M%S"}, Stdout: "20240101000000\n"},
		runner.Expect{Argv: []string{"cp", "-v", "--", "a", "a_backup_20240101000000_1"}, Status: 1},
	))
	_, errb, rc := runIn(t, d, nil, nil, "file_backup", "a")
	if rc != 1 || errb != "Error: Failed to create backup file.\n" {
		t.Errorf("fake: rc=%d err=%q", rc, errb)
	}
	if _, err := os.Lstat(filepath.Join(d, "a_backup_20240101000000_1")); err == nil {
		t.Error("fake: the reserved backup was left behind")
	}

	// A cp that writes one byte and fails.
	withRunner(t, runner.Exec{})
	failing := scriptBin(t, map[string]string{
		"date": "echo 20240101000000",
		"cp":   `printf X > "$4"; exit 1`,
	})
	e := t.TempDir()
	write(t, filepath.Join(e, "a"), []byte("xyz"))
	write(t, filepath.Join(e, "a_backup_20240101000000"), []byte("old"))
	_, errb, rc = runIn(t, e, map[string]string{"PATH": failing}, nil, "file_backup", "a")
	if rc != 1 || errb != "Error: Failed to create backup file.\n" {
		t.Errorf("partial copy: rc=%d err=%q", rc, errb)
	}
	if m, _ := filepath.Glob(filepath.Join(e, "a_backup_*")); len(m) != 1 {
		t.Errorf("partial copy left %v", m)
	}
	if b, _ := os.ReadFile(filepath.Join(e, "a_backup_20240101000000")); string(b) != "old" {
		t.Errorf("the earlier backup changed: %q", b)
	}
}

// TestFileBackupMode: the backup gets the source's permission bits minus
// the umask (setuid, setgid and sticky dropped), also for a read-only
// source, as the new file of `cp` would.
func TestFileBackupMode(t *testing.T) {
	need(t, "cp")
	old := syscall.Umask(0o022)
	t.Cleanup(func() { syscall.Umask(old) })
	path := scriptBin(t, map[string]string{"date": "echo 20240101000000"})
	for _, m := range []os.FileMode{0o644, 0o444, 0o755, 0o666} {
		d := t.TempDir()
		write(t, filepath.Join(d, "a"), []byte("x"))
		must(t, os.Chmod(filepath.Join(d, "a"), m))
		if _, errb, rc := runIn(t, d, map[string]string{"PATH": path + ":" + os.Getenv("PATH")}, nil, "file_backup", "a"); rc != 0 {
			t.Fatalf("%o: rc=%d err=%q", m, rc, errb)
		}
		st, err := os.Stat(filepath.Join(d, "a_backup_20240101000000"))
		if err != nil || st.Mode().Perm() != m&^0o022 {
			t.Errorf("source %o: backup mode %v, want %o", m, st.Mode().Perm(), m&^0o022)
		}
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// TestLedger_FILE21: cp runs with LC_ALL=C added, so a non-ASCII name is
// quoted the C way under a UTF-8 caller locale.
func TestLedger_FILE21(t *testing.T) {
	need(t, "cp")
	if out, _ := exec.Command("cp", "--version").Output(); !bytes.Contains(out, []byte("GNU coreutils")) {
		t.Skip("GNU cp required")
	}
	d := t.TempDir()
	write(t, filepath.Join(d, "più"), []byte("x"))
	out, _, rc := runIn(t, d, map[string]string{"LANG": "C.UTF-8", "LC_ALL": "C.UTF-8"}, nil, "file_backup", "più")
	re := regexp.MustCompile(`^'pi'\$'\\303\\271' -> 'pi'\$'\\303\\271''_backup_[0-9]{14}'\n$`)
	if rc != 0 || !re.MatchString(out) {
		t.Errorf("rc=%d out=%q", rc, out)
	}
}

// TestLedger_FILE25_2x: the stamp is `date +%Y%m%d%H%M%S` from PATH with
// the caller's TZ (option a), so POSIX TZ strings match 2.x.
func TestLedger_FILE25_2x(t *testing.T) {
	d := t.TempDir()
	write(t, filepath.Join(d, "a"), []byte("x"))
	// 3.0: FILE-04 fixed: the backup name is reserved before cp runs, so the
	// fake cp finds the regular file already there.
	withRunner(t, runner.NewFake(t,
		runner.Expect{Argv: []string{"date", "+%Y%m%d%H%M%S"}, EnvHas: []string{"TZ=JST-9"}, Stdout: "20251018182200\n"},
		runner.Expect{Argv: []string{"cp", "-v", "--", "a", "a_backup_20251018182200"}, EnvHas: []string{"LC_ALL=C", "TZ=JST-9"}},
	))
	if _, _, rc := runIn(t, d, map[string]string{"TZ": "JST-9"}, nil, "file_backup", "a"); rc != 0 {
		t.Errorf("rc=%d", rc)
	}

	// With the real tools, the stamp agrees with date under the same TZ.
	withRunner(t, runner.Exec{})
	need(t, "date", "cp")
	for _, tz := range []string{"JST-9", "EST5EDT,M3.2.0,M11.1.0"} {
		e := t.TempDir()
		write(t, filepath.Join(e, "a"), []byte("x"))
		before := dateIn(t, tz)
		runIn(t, e, map[string]string{"TZ": tz}, nil, "file_backup", "a")
		after := dateIn(t, tz)
		m, _ := filepath.Glob(filepath.Join(e, "a_backup_*"))
		if len(m) != 1 || !stampRe.MatchString(filepath.Base(m[0])) {
			t.Fatalf("TZ=%s: backups %v", tz, m)
		}
		s := strings.TrimPrefix(filepath.Base(m[0]), "a_backup_")
		if s != before && s != after {
			t.Errorf("TZ=%s: stamp %s, date said %s / %s", tz, s, before, after)
		}
	}
}

func dateIn(t *testing.T, tz string) string {
	t.Helper()
	cmd := exec.Command("date", "+%Y%m%d%H%M%S")
	cmd.Env = append(os.Environ(), "TZ="+tz)
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}
