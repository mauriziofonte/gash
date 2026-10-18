// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package hosts

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/mauriziofonte/gash/internal/hostshelper"
	"github.com/mauriziofonte/gash/internal/hte/hteerr"
	"github.com/mauriziofonte/gash/internal/hte/proc"
	"github.com/mauriziofonte/gash/internal/hte/state"
	"github.com/mauriziofonte/gash/internal/platform"
)

func site(domain string, enabled bool, aliases ...string) state.Site {
	return state.Site{Domain: domain, Aliases: aliases, Enabled: enabled, Kind: state.KindManaged}
}

// TestLedger_HTE54: every enabled site's names are in the block, as
// 127.0.0.1 lines only (HTE-37, option a), then the extra names.
func TestLedger_HTE54(t *testing.T) {
	sites := []state.Site{site("shop.test", true, "www.shop.test"), site("api.test", true), site("off.test", false)}
	got := string(hostshelper.FormatBlock(Desired(sites, []string{"legacy.test", "api.test", "legacy.test"})))
	want := "127.0.0.1 api.test\n127.0.0.1 shop.test www.shop.test\n127.0.0.1 legacy.test\n"
	if got != want {
		t.Fatalf("block:\n%s", got)
	}
	if len(Desired(nil, nil)) != 0 {
		t.Fatal("empty state, non-empty block")
	}
}

func TestLedger_HTE37(t *testing.T) {
	for _, e := range Desired([]state.Site{site("a.test", true)}, []string{"b.test"}) {
		if e.Address != "127.0.0.1" {
			t.Fatalf("address %s", e.Address)
		}
	}
}

func fakeRoot(t *testing.T, hosts string) platform.Sysroot {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "etc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "etc/hosts"), []byte(hosts), 0o644); err != nil {
		t.Fatal(err)
	}
	sr, err := platform.NewSysroot(dir)
	if err != nil {
		t.Fatal(err)
	}
	return sr
}

func TestCheck(t *testing.T) {
	desired := Desired([]state.Site{site("a.test", true, "www.a.test"), site("b.test", true)}, nil)
	inSync := "127.0.0.1 localhost\n" + hostshelper.BeginMarker + "\n127.0.0.1 a.test www.a.test\n127.0.0.1 b.test\n" + hostshelper.EndMarker + "\n"
	if d := Check(fakeRoot(t, inSync), desired); !d.InSync || !d.Found {
		t.Fatalf("in sync: %+v", d)
	}
	// WSL regenerated /etc/hosts: no block at all.
	d := Check(fakeRoot(t, "127.0.0.1 localhost\n"), desired)
	if d.InSync || d.Found || !cmp.Equal(d.Missing, []string{"a.test", "www.a.test", "b.test"}) {
		t.Fatalf("no block: %+v", d)
	}
	drifted := hostshelper.BeginMarker + "\n127.0.0.1 a.test\n127.0.0.1 gone.test\n" + hostshelper.EndMarker + "\n"
	d = Check(fakeRoot(t, drifted), desired)
	if d.InSync || !cmp.Equal(d.Missing, []string{"www.a.test", "b.test"}) || !cmp.Equal(d.Unexpected, []string{"gone.test"}) {
		t.Fatalf("drift: %+v", d)
	}
	if d := Check(fakeRoot(t, hostshelper.BeginMarker+"\n"), desired); d.InSync || d.Problem == "" {
		t.Fatalf("corrupt markers: %+v", d)
	}
	if d := Check(fakeRoot(t, "127.0.0.1 localhost\n"), nil); !d.InSync {
		t.Fatalf("nothing desired, no block: %+v", d)
	}
}

func TestApply(t *testing.T) {
	desired := Desired([]state.Site{site("a.test", true)}, nil)
	f := &proc.Fake{}
	a := Applier{Runner: f, Sudo: "/usr/bin/sudo", Helper: HelperPath}
	if err := a.Apply(context.Background(), desired); err != nil {
		t.Fatal(err)
	}
	c := f.Calls()[0]
	if c.String() != "/usr/bin/sudo -n /usr/local/libexec/gash/gash-hosts-helper apply" || string(c.Stdin) != "127.0.0.1 a.test\n" || c.Env != nil {
		t.Fatalf("%s %q %v", c, c.Stdin, c.Env)
	}
	for _, tc := range []struct {
		res  proc.Result
		want hteerr.Code
	}{
		{proc.Result{ExitCode: 1, Stderr: []byte("sudo: a password is required\n")}, hteerr.PrivilegedStepRequired},
		{proc.Result{ExitCode: 2, Stderr: []byte("gash-hosts-helper: line 1: bad\n")}, hteerr.InvalidState},
		{proc.Result{ExitCode: 1, Stderr: []byte("gash-hosts-helper: writing /etc/hosts: read-only\n")}, hteerr.PrivilegedStepRequired},
	} {
		f := &proc.Fake{Handler: func(proc.Cmd) (proc.Result, error) { return tc.res, nil }}
		err := Applier{Runner: f, Sudo: "/usr/bin/sudo", Helper: HelperPath}.Apply(context.Background(), desired)
		if hteerr.CodeOf(err) != tc.want {
			t.Errorf("%s: %v", tc.res.Stderr, err)
		}
	}
}

func TestState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hte", "hosts.toml")
	st, err := LoadState(path)
	if err != nil || len(st.Names) != 0 {
		t.Fatalf("missing: %v %v", st, err)
	}
	if err := SaveState(path, State{Names: []string{"legacy.test"}}); err != nil {
		t.Fatal(err)
	}
	if st, err := LoadState(path); err != nil || !cmp.Equal(st.Names, []string{"legacy.test"}) {
		t.Fatalf("round trip: %v %v", st, err)
	}
	for _, bad := range []string{"names = [\"Bad Name\"]\n", "names = []\nx = 1\n", "\n", "names = ["} {
		if err := os.WriteFile(path, []byte(bad), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadState(path); hteerr.CodeOf(err) != hteerr.InvalidState {
			t.Errorf("%q: %v", bad, err)
		}
	}
	if err := SaveState(path, State{Names: []string{"*.test"}}); hteerr.CodeOf(err) != hteerr.InvalidDomain {
		t.Fatalf("wildcard saved: %v", err)
	}
}

func TestWindows(t *testing.T) {
	f := &proc.Fake{Handler: func(c proc.Cmd) (proc.Result, error) {
		return proc.Result{Stdout: []byte("ok a.test\r\nfail b.test\r\n")}, nil
	}}
	got, err := WindowsUnresolved(context.Background(), f, PowerShell, []string{"a.test", "b.test", "c.test"})
	if err != nil || !cmp.Equal(got, []string{"b.test", "c.test"}) {
		t.Fatalf("%v %v", got, err)
	}
	c := f.Calls()[0]
	if string(c.Stdin) != "a.test\nb.test\nc.test\n" || strings.Contains(strings.Join(c.Args, " "), "a.test") {
		t.Fatalf("names must be data on stdin: %q %q", c.Args, c.Stdin)
	}
	if diff := cmp.Diff([]string{"127.0.0.1 b.test"}, WindowsLines([]string{"b.test"})); diff != "" {
		t.Fatal(diff)
	}
}
