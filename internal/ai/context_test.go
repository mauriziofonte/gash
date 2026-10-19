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
	d := m.distro()
	return d, pkgManager(d)
}

// TestLedger_AI11_2x: the quoting gap and the table are reproduced; quoted
// IDs and Linux Mint fall to unknown, opensuse-* truncates to opensuse.
func TestLedger_AI11_2x(t *testing.T) {
	cases := []struct{ os, distro, pkg string }{
		{"ID=ubuntu\n", "ubuntu", "apt"},
		{"ID=rocky\n", "rocky", "dnf"},
		{`ID="rocky"` + "\n", "linux", "unknown"},          // quoted value: no match
		{"ID=linuxmint\n", "linuxmint", "unknown"},         // real Mint ID not in table
		{"ID=opensuse-tumbleweed\n", "opensuse", "zypper"}, // truncated at '-'
		{"NAME=Foo\n", "linux", "unknown"},                 // no ID line
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
