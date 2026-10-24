// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package hte

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mauriziofonte/gash/internal/hte/child"
	"github.com/mauriziofonte/gash/internal/hte/rootops"
	"github.com/mauriziofonte/gash/internal/platform"
	"github.com/mauriziofonte/gash/internal/runner"
)

func TestProcAddr(t *testing.T) {
	for in, want := range map[string]string{
		"0100007F":                         "127.0.0.1",
		"00000000":                         "0.0.0.0",
		"00000000000000000000000001000000": "[::1]",
		"0000000000000000FFFF00000100007F": "[127.0.0.1]",
	} {
		if got := procAddr(in); got != want {
			t.Errorf("procAddr(%s) = %s, want %s", in, got, want)
		}
	}
}

func TestListeners(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "proc/net"), 0o755); err != nil {
		t.Fatal(err)
	}
	tcp := "  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n" +
		"   0: 00000000:0050 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 1234 1 0 100 0 0 10 0\n" +
		"   1: 0100007F:1F90 0100007F:9C40 01 00000000:00000000 00:00000000 00000000  1000        0 5678 1 0 100 0 0 10 0\n"
	if err := os.WriteFile(filepath.Join(dir, "proc/net/tcp"), []byte(tcp), 0o644); err != nil {
		t.Fatal(err)
	}
	sr, err := platform.NewSysroot(dir)
	if err != nil {
		t.Fatal(err)
	}
	ls := listeners(sr)
	if got := ls[80]; len(got) != 1 || got[0].addr != "0.0.0.0:80" || got[0].uid != 0 || got[0].inode != "1234" {
		t.Errorf("port 80: %+v", got)
	}
	if len(ls[8080]) != 0 {
		t.Errorf("an established socket counted as listening: %+v", ls[8080])
	}
}

// Before install doctor reports the machine and fails on the install.
func TestDoctorNotInstalled(t *testing.T) {
	f := newFixture(t)
	res, err := f.e.Doctor(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.OK {
		t.Error("doctor OK before install")
	}
	i := slices.IndexFunc(res.Checks, func(c Check) bool { return c.Name == "install" })
	if i < 0 || res.Checks[i].Status != CheckFail || res.Checks[i].Remedy != "run gash hte install" {
		t.Errorf("install check: %+v", res.Checks)
	}
	if i := slices.IndexFunc(res.Checks, func(c Check) bool { return c.Name == "php" }); i < 0 || res.Checks[i].Status != CheckOK {
		t.Errorf("php check: %+v", res.Checks)
	}
}

// Fedora and Arch ship /etc/sudoers.d as 0750: the drop-in is there but
// the user cannot stat it. Doctor asks sudo instead of calling it missing.
func TestDoctorSudoersUnreadableDir(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads a 0750 directory; run as a regular user")
	}
	dir := t.TempDir()
	for rel, mode := range map[string]os.FileMode{"usr/bin/sudo": 0o755, "etc/sudoers.d/gash-hte": 0o440} {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x\n"), mode); err != nil {
			t.Fatal(err)
		}
	}
	sudoersD := filepath.Join(dir, "etc/sudoers.d")
	sr, err := platform.NewSysroot(dir)
	if err != nil {
		t.Fatal(err)
	}
	check := func(t *testing.T, sudoOK bool) Check {
		t.Helper()
		run := child.Fake(t, func(c runner.Call) runner.Expect {
			if filepath.Base(c.Argv[0]) == "sudo" && slices.Contains(c.Argv, "-l") && !sudoOK {
				return runner.Expect{Status: 1, Stderr: "sudo: a password is required"}
			}
			return runner.Expect{}
		})
		r := &report{}
		(&Engine{Sysroot: sr, Runner: run}).doctorSudoers(context.Background(), r)
		if len(r.checks) != 1 {
			t.Fatalf("checks %+v", r.checks)
		}
		return r.checks[0]
	}

	// The user owns the temporary tree, so 0750 would not hide anything
	// from them; 0000 is what root:root 0750 is to a user outside the group.
	if err := os.Chmod(sudoersD, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(sudoersD, 0o755) })
	if _, err := os.Stat(filepath.Join(sudoersD, "gash-hte")); !errors.Is(err, fs.ErrPermission) {
		t.Skipf("the directory does not hide the file: %v", err)
	}
	if c := check(t, true); c.Status != CheckOK || !strings.Contains(c.Details, "not readable by you") {
		t.Errorf("hidden, sudo allows: %+v", c)
	}
	if c := check(t, false); c.Status != CheckFail || strings.Contains(c.Details, "missing") {
		t.Errorf("hidden, sudo refuses: %+v", c)
	}
	if err := os.Chmod(sudoersD, 0o755); err != nil {
		t.Fatal(err)
	}
	if c := check(t, true); c.Status != CheckOK || c.Details != rootops.SudoersPath {
		t.Errorf("readable: %+v", c)
	}
	if err := os.Remove(filepath.Join(sudoersD, "gash-hte")); err != nil {
		t.Fatal(err)
	}
	if c := check(t, true); c.Status != CheckFail || !strings.Contains(c.Details, "is missing") {
		t.Errorf("missing: %+v", c)
	}
}
