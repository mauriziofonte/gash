// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package files

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
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
// order (FILE-16: matched in Go, no shell option touched; FILE-13: .tar.Z is
// the single uncompress branch, .taz/.tbz/.tar.lz unsupported).
func TestArchiveSuffixMatch(t *testing.T) {
	cases := map[string]kind{
		"a.tar.bz2": kTarBz2, "A.TAR.BZ2": kTarBz2, "a.tar.gz": kTarGz, "a.Tar.Gz": kTarGz,
		"a.tar.xz": kTarXz, "a.txz": kTarXz, "a.tar.zst": kTarZst, "a.TZST": kTarZst,
		"a.xz": kSingleXz, "a.ZST": kSingleZst, "a.Bz2": kSingleBz2, "a.rar": kRar,
		"a.GZ": kSingleGz, "a.tar": kTarPlain, "a.tbz2": kTbz2, "a.tgz": kTgz, "a.ZIP": kZip,
		"a.Z": kSingleZ, "a.tar.Z": kSingleZ, "a.7z": kSevenZip,
		"a.taz": kUnsupported, "a.tbz": kUnsupported, "a.tar.lz": kUnsupported, "f.abc": kUnsupported,
	}
	for name, want := range cases {
		if got, _ := classify(name); got != want {
			t.Errorf("classify(%q) = %d, want %d", name, got, want)
		}
	}
}

// TestBasenameSuffix pins `basename NAME SUFFIX` (case-sensitive strip).
func TestBasenameSuffix(t *testing.T) {
	cases := []struct{ name, suffix, want string }{
		{"foo.Z", ".z", "foo.Z"}, {"UP.GZ", ".gz", "UP.GZ"}, {"a.gz", ".gz", "a"},
		{".gz", ".gz", ".gz"}, {"dir/foo.gz", ".gz", "foo"}, {"./-s.gz", ".gz", "-s"},
		{"foo\n.gz", ".gz", "foo\n"}, {"dir/", ".gz", "dir"},
	}
	for _, c := range cases {
		if got := basenameSuffix(c.name, c.suffix); got != c.want {
			t.Errorf("basenameSuffix(%q, %q) = %q, want %q", c.name, c.suffix, got, c.want)
		}
	}
}

// TestLedger_FILE01_2x: a .Z archive in the working directory is emptied.
func TestLedger_FILE01_2x(t *testing.T) {
	need(t, "uncompress")
	d := t.TempDir()
	write(t, filepath.Join(d, "foo.Z"), gzipBytes(t, "hi\n"))
	_, _, rc := runIn(t, d, nil, nil, "archive_extract", "foo.Z")
	if rc != 1 || size(t, filepath.Join(d, "foo.Z")) != 0 {
		t.Errorf("rc=%d size=%d, want rc 1 and an emptied archive", rc, size(t, filepath.Join(d, "foo.Z")))
	}
}

// TestLedger_FILE02_2x: an upper-case suffix keeps the archive's name.
func TestLedger_FILE02_2x(t *testing.T) {
	need(t, "gunzip")
	d := t.TempDir()
	write(t, filepath.Join(d, "UP.GZ"), gzipBytes(t, "hi\n"))
	_, _, rc := runIn(t, d, nil, nil, "archive_extract", "UP.GZ")
	if rc != 1 || size(t, filepath.Join(d, "UP.GZ")) != 0 {
		t.Errorf("rc=%d, want rc 1 and an emptied archive", rc)
	}
}

// TestLedger_FILE03_2x: a corrupt archive truncates the existing output.
func TestLedger_FILE03_2x(t *testing.T) {
	need(t, "gunzip")
	d := t.TempDir()
	write(t, filepath.Join(d, "s.txt.gz"), []byte("not gzip"))
	write(t, filepath.Join(d, "s.txt"), []byte("PRECIOUS\n"))
	_, errb, rc := runIn(t, d, nil, nil, "archive_extract", "s.txt.gz")
	if rc != 1 || size(t, filepath.Join(d, "s.txt")) != 0 || !strings.Contains(errb, "not in gzip format") {
		t.Errorf("rc=%d err=%q, want rc 1, gzip's error and an empty s.txt", rc, errb)
	}
}

// TestLedger_FILE15_2x: a trailing newline is dropped from the output name.
func TestLedger_FILE15_2x(t *testing.T) {
	need(t, "gunzip")
	d := t.TempDir()
	write(t, filepath.Join(d, "foo\n.gz"), gzipBytes(t, "hi\n"))
	_, _, rc := runIn(t, d, nil, nil, "archive_extract", "foo\n.gz")
	got, err := os.ReadFile(filepath.Join(d, "foo"))
	if rc != 0 || err != nil || string(got) != "hi\n" {
		t.Errorf("rc=%d foo=%q err=%v", rc, got, err)
	}
}

// TestLedger_FILE07_2x: OUTPUT_DIR is created before the type is checked.
func TestLedger_FILE07_2x(t *testing.T) {
	d := t.TempDir()
	write(t, filepath.Join(d, "f.abc"), []byte("x"))
	_, errb, rc := runIn(t, d, nil, nil, "archive_extract", "f.abc", "newdir")
	if rc != 1 || errb != "Error: Cannot extract 'f.abc', unsupported file type.\n" {
		t.Errorf("rc=%d err=%q", rc, errb)
	}
	if st, err := os.Stat(filepath.Join(d, "newdir")); err != nil || !st.IsDir() {
		t.Errorf("newdir not created: %v", err)
	}
}

// TestLedger_FILE12_2x: existing non-regular paths get "does not exist".
func TestLedger_FILE12_2x(t *testing.T) {
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
			if rc != 1 || errb != "Error: File '"+n+"' does not exist.\n" {
				t.Errorf("%s %s: rc=%d err=%q", fn, n, rc, errb)
			}
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

// TestLedger_FILE19 pins the missing-tool lines, after the 2.x file effects.
func TestLedger_FILE19(t *testing.T) {
	t.Run("gunzip", func(t *testing.T) {
		d := t.TempDir()
		write(t, filepath.Join(d, "s.txt.gz"), gzipBytes(t, "hi\n"))
		_, errb, rc := runIn(t, d, map[string]string{"PATH": t.TempDir()}, nil, "archive_extract", "s.txt.gz")
		if rc != 1 || errb != "Error: Command 'gunzip' is not installed or not in PATH.\n" {
			t.Errorf("rc=%d err=%q", rc, errb)
		}
		if size(t, filepath.Join(d, "s.txt")) != 0 {
			t.Error("the empty output should stay (FILE-03)")
		}
	})
	t.Run("unzip", func(t *testing.T) {
		d := t.TempDir()
		writeZip(t, filepath.Join(d, "a.zip"), map[string]string{"src/a.txt": "a"}, nil)
		_, errb, rc := runIn(t, d, map[string]string{"PATH": linkBin(t, "mkdir")}, nil, "archive_extract", "a.zip", "newdir")
		if rc != 1 || errb != "Error: Command 'unzip' is not installed or not in PATH.\n" {
			t.Errorf("rc=%d err=%q", rc, errb)
		}
		if _, err := os.Stat(filepath.Join(d, "newdir")); err != nil {
			t.Error("OUTPUT_DIR should be created first (FILE-07)")
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
	cases := []struct {
		archive string
		expects []runner.Expect
		want    int
	}{
		{"a.tgz", []runner.Expect{{Argv: []string{"tar", "xvzf", "a.tgz", "-C", "o", "--no-same-owner", "--no-same-permissions"}, Status: 2}}, 1},
		{"a.zip", []runner.Expect{{Argv: []string{"unzip", "a.zip", "-d", "o"}, Status: 1}}, 1},
		{"a.7z", []runner.Expect{
			{Argv: []string{"7z", "l", "-slt", "-ba", "--", "a.7z"}},
			{Argv: []string{"7z", "x", "a.7z", "-oo"}, Status: 0}}, 0},
		{"a.7z", []runner.Expect{
			{Argv: []string{"7z", "l", "-slt", "-ba", "--", "a.7z"}},
			{Argv: []string{"7z", "x", "a.7z", "-oo"}, Status: 2}}, 1},
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

// TestLedger_FILE08_09_11_13_14 pins the reproduced 2.x argv: mkdir without
// --, unrar's dash OUTPUT_DIR, the inherited stdin, .tar.Z through
// uncompress and `uncompress F -c`.
func TestLedger_FILE08_09_11_13_14(t *testing.T) {
	d := t.TempDir()
	for _, n := range []string{"a.tgz", "a.rar", "a.zip", "s.z", "a.tar.Z"} {
		write(t, filepath.Join(d, n), []byte("x"))
	}
	for _, n := range []string{"-x", "o"} {
		if err := os.Mkdir(filepath.Join(d, n), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	path := stubBin(t, "tar", "unrar", "unzip", "uncompress", "mkdir")
	run := func(stdin io.Reader, expects []runner.Expect, args ...string) int {
		withRunner(t, runner.NewFake(t, expects...))
		_, _, rc := runIn(t, d, map[string]string{"PATH": path}, stdin, "archive_extract", args...)
		return rc
	}
	// FILE-08: `mkdir -p --help` exits 0, then tar fails on -C --help.
	if rc := run(nil, []runner.Expect{
		{Argv: []string{"mkdir", "-p", "--help"}},
		{Argv: []string{"tar", "xvzf", "a.tgz", "-C", "--help", "--no-same-owner", "--no-same-permissions"}, Status: 2},
	}, "a.tgz", "--help"); rc != 1 {
		t.Errorf("FILE-08 rc=%d", rc)
	}
	// FILE-09: unrar x F -x (no --).
	if rc := run(nil, []runner.Expect{
		{Argv: []string{"unrar", "lt", "--", "a.rar"}},
		{Argv: []string{"unrar", "x", "a.rar", "-x"}},
	}, "a.rar", "-x"); rc != 0 {
		t.Errorf("FILE-09 rc=%d", rc)
	}
	// FILE-11: the caller's stdin reaches unzip (prompt answers).
	if rc := run(strings.NewReader("A\n"), []runner.Expect{
		{Argv: []string{"unzip", "a.zip", "-d", "o"}, Stdin: runner.Text("A\n")},
	}, "a.zip", "o"); rc != 0 {
		t.Errorf("FILE-11 rc=%d", rc)
	}
	// FILE-14: uncompress F -c, output o/s.
	if rc := run(nil, []runner.Expect{{Argv: []string{"uncompress", "s.z", "-c"}, Stdout: "hi\n"}}, "s.z", "o"); rc != 0 {
		t.Errorf("FILE-14 rc=%d", rc)
	}
	if b, _ := os.ReadFile(filepath.Join(d, "o", "s")); string(b) != "hi\n" {
		t.Errorf("FILE-14 output %q", b)
	}
	// FILE-13: .tar.Z is decompressed under its own name.
	if rc := run(nil, []runner.Expect{{Argv: []string{"uncompress", "a.tar.Z", "-c"}, Stdout: "TAR"}}, "a.tar.Z", "o"); rc != 0 {
		t.Errorf("FILE-13 rc=%d", rc)
	}
	if b, _ := os.ReadFile(filepath.Join(d, "o", "a.tar.Z")); string(b) != "TAR" {
		t.Errorf("FILE-13 output %q", b)
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

// TestLedger_FILE04_05_06_2x pins the reproduced file_backup defects with a
// fixed `date` on PATH: same-second overwrite, write-through of a symlink,
// and cp's status ignored.
func TestLedger_FILE04_05_06_2x(t *testing.T) {
	need(t, "cp")
	path := scriptBin(t, map[string]string{"date": "echo 20240101000000"})
	t.Run("FILE-04", func(t *testing.T) {
		d := t.TempDir()
		write(t, filepath.Join(d, "a"), []byte("1"))
		runIn(t, d, map[string]string{"PATH": path}, nil, "file_backup", "a")
		write(t, filepath.Join(d, "a"), []byte("2"))
		_, _, rc := runIn(t, d, map[string]string{"PATH": path}, nil, "file_backup", "a")
		m, _ := filepath.Glob(filepath.Join(d, "a_backup_*"))
		b, _ := os.ReadFile(filepath.Join(d, "a_backup_20240101000000"))
		if rc != 0 || len(m) != 1 || string(b) != "2" {
			t.Errorf("rc=%d backups=%v content=%q", rc, m, b)
		}
	})
	t.Run("FILE-05", func(t *testing.T) {
		d := t.TempDir()
		write(t, filepath.Join(d, "a"), []byte("NEW"))
		write(t, filepath.Join(d, "v"), []byte("OLD"))
		if err := os.Symlink("v", filepath.Join(d, "a_backup_20240101000000")); err != nil {
			t.Fatal(err)
		}
		_, _, rc := runIn(t, d, map[string]string{"PATH": path}, nil, "file_backup", "a")
		if b, _ := os.ReadFile(filepath.Join(d, "v")); rc != 0 || string(b) != "NEW" {
			t.Errorf("rc=%d v=%q, want the target rewritten", rc, b)
		}
	})
	t.Run("FILE-06", func(t *testing.T) {
		failing := scriptBin(t, map[string]string{"date": "echo 20240101000000", "cp": "exit 1"})
		d := t.TempDir()
		write(t, filepath.Join(d, "a"), []byte("x"))
		write(t, filepath.Join(d, "a_backup_20240101000000"), []byte("old"))
		if _, errb, rc := runIn(t, d, map[string]string{"PATH": failing}, nil, "file_backup", "a"); rc != 0 || errb != "" {
			t.Errorf("rc=%d err=%q, want 0: cp's status is ignored", rc, errb)
		}
		e := t.TempDir()
		write(t, filepath.Join(e, "a"), []byte("x"))
		if _, errb, rc := runIn(t, e, map[string]string{"PATH": failing}, nil, "file_backup", "a"); rc != 1 ||
			errb != "Error: Failed to create backup file.\n" {
			t.Errorf("no backup: rc=%d err=%q", rc, errb)
		}
	})
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
	write(t, filepath.Join(d, "a_backup_20251018182200"), nil)
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
