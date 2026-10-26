// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package files

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/platform"
	"github.com/mauriziofonte/gash/internal/prereq"
	"github.com/mauriziofonte/gash/internal/runner"
	"github.com/mauriziofonte/gash/internal/term"
)

// withBSDTar sets bsdTar (tar decompresses by itself, as on macOS) for
// the test.
func withBSDTar(t *testing.T, on bool) {
	t.Helper()
	old := bsdTar
	bsdTar = on
	t.Cleanup(func() { bsdTar = old })
}

// GNU tar runs gzip, bzip2 and xz: a missing one is the prerequisite
// report before OUTPUT_DIR is made and nothing starts (3.4: tar's own
// "Cannot exec" lines, after OUTPUT_DIR was made).
func TestTarDecompressorMissing(t *testing.T) {
	defer pinSystem()()
	withBSDTar(t, false)
	d := t.TempDir()
	for _, n := range []string{"a.tar.gz", "a.tgz", "a.tar.bz2", "a.tar.xz"} {
		write(t, filepath.Join(d, n), []byte("x"))
	}
	withRunner(t, runner.NewFake(t)) // nothing may start
	path := stubBin(t, "tar", "mkdir")
	for archive, want := range map[string]string{
		"a.tar.gz":  gzipReport(".tar.gz"),
		"a.tgz":     gzipReport(".tgz"),
		"a.tar.bz2": report(".tar.bz2", "bzip2", "extracts .bz2 archives", "bzip2"),
		"a.tar.xz": report(".tar.xz", "xz", "extracts .xz archives, and lets archive_extract check a .tar.xz before extracting it",
			"xz-utils"),
	} {
		_, errb, rc := runIn(t, d, map[string]string{"PATH": path}, nil, "archive_extract", archive, "out")
		if rc != 1 || errb != want {
			t.Errorf("%s: rc=%d err=%q", archive, rc, errb)
		}
		if _, err := os.Lstat(filepath.Join(d, "out")); err == nil {
			t.Errorf("%s: out created", archive)
		}
	}
}

// bsdtar decompresses .gz and .bz2 itself, but the .tar.xz member guard
// needs xz: without it the archive is refused, never extracted unchecked
// (3.4: the guard stood down and bsdtar went ahead).
func TestTarXzRefusedWithoutXz(t *testing.T) {
	defer prereq.SetSystem(&prereq.System{OS: platform.OS{GOOS: "darwin", ID: "macos", Name: "macOS 15.1"},
		Manager: prereq.Brew, Brew: "/opt/homebrew/bin/brew"})()
	withBSDTar(t, true)
	d := t.TempDir()
	writeTar(t, filepath.Join(d, "a.tbz2"), false, []tarEntry{{name: "s", flag: '0'}})
	write(t, filepath.Join(d, "a.tar.xz"), []byte("x"))
	if err := os.Mkdir(filepath.Join(d, "o"), 0o755); err != nil {
		t.Fatal(err)
	}
	withRunner(t, runner.NewFake(t,
		runner.Expect{Argv: []string{"tar", "xvjf", "a.tbz2", "-C", "o", "--no-same-owner", "--no-same-permissions"}}))
	path := stubBin(t, "tar")
	if _, errb, rc := runIn(t, d, map[string]string{"PATH": path}, nil, "archive_extract", "a.tbz2", "o"); rc != 0 {
		t.Errorf("a.tbz2 with bsdtar: rc=%d err=%q", rc, errb)
	}
	_, errb, rc := runIn(t, d, map[string]string{"PATH": path}, nil, "archive_extract", "a.tar.xz", "o")
	want := "Error: archive_extract (.tar.xz) needs xz, which is not installed.\n" +
		"  xz extracts .xz archives, and lets archive_extract check a .tar.xz before extracting it (package xz on macOS 15.1).\n" +
		"  Install it:  brew install xz\n" +
		"  Or run:      gash_doctor --install xz\n"
	if rc != 1 || errb != want {
		t.Errorf("a.tar.xz: rc=%d err=%q", rc, errb)
	}
}

// The guard itself fails closed: a decompressor that is not there is an
// error, not an empty member list (3.4).
func TestGuardMissingDecompressor(t *testing.T) {
	d := t.TempDir()
	write(t, filepath.Join(d, "a.tar.xz"), []byte("x"))
	for path, want := range map[string]string{stubBin(t, "tar"): "xz", stubBin(t, "xz"): "tar"} {
		e := invoke.EnvMap(map[string]string{"PATH": path})
		c := &invoke.Call{Name: "archive_extract", Env: e, Term: term.Fixed(e, false, false, false)}
		_, unsafe, err := memberGuard(c, kTarXz, filepath.Join(d, "a.tar.xz"), "o")
		var me *missingErr
		if unsafe || !errors.As(err, &me) || me.cmd != want {
			t.Errorf("PATH with %s missing: unsafe=%v err=%v", want, unsafe, err)
		}
	}
}

// file_backup without date stamps the backup with the local time read in
// Go, and names a missing cp (3.4: the name ended in "_backup_", and a
// missing cp was "Failed to create backup file.").
func TestFileBackupWithoutDateOrCp(t *testing.T) {
	old := now
	now = func() time.Time { return time.Date(2026, 10, 26, 9, 8, 7, 0, time.Local) }
	t.Cleanup(func() { now = old })
	d := t.TempDir()
	write(t, filepath.Join(d, "a"), []byte("x"))
	withRunner(t, runner.NewFake(t,
		runner.Expect{Argv: []string{"date", "+%Y%m%d%H%M%S"}, Err: &runner.StartError{Name: "date", Status: 127, Err: errors.New("not found")}},
		runner.Expect{Argv: []string{"cp", "-v", "--", "a", "a_backup_20261026090807"}, Stdout: "'a' -> 'a_backup_20261026090807'\n"},
		runner.Expect{Argv: []string{"date", "+%Y%m%d%H%M%S"}, Stdout: "20240101000000\n"},
		runner.Expect{Argv: []string{"cp", "-v", "--", "a", "a_backup_20240101000000"}, Err: &runner.StartError{Name: "cp", Status: 127, Err: errors.New("not found")}},
	))
	out, errb, rc := runIn(t, d, nil, nil, "file_backup", "a")
	if rc != 0 || errb != "" || out != "'a' -> 'a_backup_20261026090807'\n" {
		t.Errorf("no date: rc=%d out=%q err=%q", rc, out, errb)
	}
	_, errb, rc = runIn(t, d, nil, nil, "file_backup", "a")
	if rc != 1 || errb != "Error: file_backup needs cp, which is not on PATH.\n" {
		t.Errorf("no cp: rc=%d err=%q", rc, errb)
	}
	if _, err := os.Lstat(filepath.Join(d, "a_backup_20240101000000")); err == nil {
		t.Error("no cp: the reserved backup was left behind")
	}
}
