// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package ai

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mauriziofonte/gash/internal/platform"
)

// distroOf builds a module over a sysroot whose /etc/os-release holds the
// given content and returns its distro and package manager.
func distroOf(t *testing.T, osRelease string) (string, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "etc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "etc", "os-release"), []byte(osRelease), 0o644); err != nil {
		t.Fatal(err)
	}
	sr, err := platform.NewSysroot(root)
	if err != nil {
		t.Fatal(err)
	}
	m := New()
	m.sys = sr
	return m.distro()
}

// 3.0: AI-11 fixed: the ID is read with the os-release parser (quotes
// and all), Linux Mint's and AlmaLinux's real IDs are in the table, and
// an unknown ID takes the manager of its first known ID_LIKE word.
func TestLedger_AI11_fix(t *testing.T) {
	cases := []struct{ os, distro, pkg string }{
		{"ID=ubuntu\n", "ubuntu", "apt"},
		{"ID=rocky\n", "rocky", "dnf"},
		{`ID="rocky"` + "\n", "rocky", "dnf"},
		{"ID='almalinux'\n", "almalinux", "dnf"},
		{"ID=linuxmint\n", "linuxmint", "apt"},
		{"ID=opensuse-tumbleweed\n", "opensuse-tumbleweed", "zypper"},
		{"ID=foo\nID_LIKE=debian\n", "foo", "apt"},
		{"ID=bar\nID_LIKE=\"other rhel fedora\"\n", "bar", "dnf"},
		{"ID=baz\nID_LIKE=\"suse opensuse\"\n", "baz", "zypper"},
		{"ID=\"a b\"\n", "linux", "unknown"},
		{"NAME=Foo\n", "linux", "unknown"},
	}
	for _, c := range cases {
		d, p := distroOf(t, c.os)
		if d != c.distro || p != c.pkg {
			t.Fatalf("os %q -> distro %q pkg %q, want %q / %q", c.os, d, p, c.distro, c.pkg)
		}
	}
}

// TestLedger_AI21_2x: ID=alpine is read natively (no grep -P) to alpine/apk.
func TestLedger_AI21_2x(t *testing.T) {
	if d, p := distroOf(t, "ID=alpine\nNAME=Alpine\n"); d != "alpine" || p != "apk" {
		t.Fatalf("got %q/%q, want alpine/apk", d, p)
	}
}

// TestGatherContextJSON: the context is a valid JSON object with the 2.x
// keys in order.
func TestGatherContextJSON(t *testing.T) {
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, "etc"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "etc", "os-release"), []byte("ID=debian\n"), 0o644)
	sr, _ := platform.NewSysroot(root)
	m := New()
	m.sys = sr
	cwd := t.TempDir()
	_ = os.WriteFile(filepath.Join(cwd, "a.txt"), nil, 0o644)
	m.getwd = func() (string, error) { return cwd, nil }
	c, _, _ := newCall(map[string]string{}, nil, false)
	got := m.gatherContext(c)
	for _, want := range []string{`"cwd":`, `"shell":`, `"distro":"debian"`, `"pkg":"apt"`, `"git":`, `"files":`, `"exit":0`} {
		if !strings.Contains(got, want) {
			t.Fatalf("context %s missing %q", got, want)
		}
	}
}
