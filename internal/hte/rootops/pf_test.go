// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package rootops

import (
	"flag"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/mauriziofonte/gash/internal/hte/hteerr"
	"github.com/mauriziofonte/gash/internal/hte/receipt"
	"github.com/mauriziofonte/gash/internal/platform"
)

var update = flag.Bool("update", false, "rewrite the golden files under testdata/hte/golden/pf")

// stockPFConf is the /etc/pf.conf macOS ships.
const stockPFConf = `#
# Default PF configuration file.
#
scrub-anchor "com.apple/*"
nat-anchor "com.apple/*"
rdr-anchor "com.apple/*"
dummynet-anchor "com.apple/*"
anchor "com.apple/*"
load anchor "com.apple" from "/etc/pf.anchors/com.apple"
`

// newMacFixture is the fixture as macOS: pf.conf and its anchors
// directory, /Library/LaunchDaemons, pfctl, plutil and launchctl.
func newMacFixture(t *testing.T) (*fixture, Env) {
	f := newFixture(t)
	f.write(PFConf, stockPFConf, 0o644)
	f.write("/etc/pf.anchors/com.apple", "anchor \"250.ApplicationFirewall/*\"\n", 0o644)
	f.write("/Library/LaunchDaemons/com.example.other.plist", "<plist/>\n", 0o644)
	for _, tool := range []string{pfctlPath, "/usr/bin/plutil", "/bin/launchctl"} {
		f.write(tool, "", 0o755)
	}
	env := f.env()
	env.OS = platform.OS{GOOS: "darwin", ID: "macos", VersionID: "15.1", Family: platform.FamilyDarwin}
	return f, env
}

func pfPlan() []Op {
	return []Op{{ID: 1, Op: OpPFRedirect, Ports: []int{8080, 8443}}, {ID: 2, Op: OpRecordAutostart, Mode: "launchd"}}
}

func run(t *testing.T, env Env, plan []Op) error {
	t.Helper()
	c := session(t, env, plan)
	defer c.Close()
	for _, o := range plan {
		if _, err := c.Run(o.ID); err != nil {
			return err
		}
	}
	return nil
}

func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("../../../testdata/hte/golden/pf", name)
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("golden file missing (run go test -update): %v", err)
	}
	if diff := cmp.Diff(string(want), got); diff != "" {
		t.Errorf("%s (-golden +written):\n%s", name, diff)
	}
}

// TestPFRedirect: on macOS the pf redirect writes the anchor file and the
// LaunchDaemon (golden files, both checked before they are put in place),
// loads the anchor with pfctl -E, and records itself after its files; a
// second install records nothing; uninstall empties the anchor, unloads
// the daemon and leaves the tree byte for byte as it was. /etc/pf.conf is
// never written.
func TestPFRedirect(t *testing.T) {
	f, env := newMacFixture(t)
	before := snapshot(t, f.root)
	if err := run(t, env, pfPlan()); err != nil {
		t.Fatal(err)
	}
	golden(t, "gash-hte", f.read(PFAnchorFile))
	golden(t, PFDaemonLabel+".plist", f.read(PFDaemonPath))
	for _, p := range []string{PFAnchorFile, PFDaemonPath} {
		if fi, err := os.Stat(filepath.Join(f.root, p)); err != nil || fi.Mode().Perm() != 0o644 {
			t.Errorf("%s: %v %v", p, fi, err)
		}
	}
	if f.read(PFConf) != stockPFConf {
		t.Error("pf.conf written")
	}
	want := []string{
		"pfctl -n -a com.apple/gash-hte -f " + filepath.Join(f.root, pfAnchorTmp),
		"plutil -lint " + filepath.Join(f.root, pfDaemonTmp),
		"pfctl -E -a com.apple/gash-hte -f /etc/pf.anchors/gash-hte",
	}
	if diff := cmp.Diff(want, f.lines); diff != "" {
		t.Errorf("commands (-want +got):\n%s", diff)
	}
	r, _, err := receipt.Load(filepath.Join(f.root, receipt.RootPath("me")))
	if err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for _, c := range r.Changes {
		kinds = append(kinds, string(c.Kind)+" "+c.Path)
	}
	if diff := cmp.Diff([]string{"file_created " + PFAnchorFile, "file_created " + PFDaemonPath, "pf_redirect " + PFAnchorFile, "autostart "}, kinds); diff != "" {
		t.Errorf("changes:\n%s", diff)
	}
	if c, _ := r.Find(receipt.PFRedirect, ""); c.Name != PFAnchor || string(c.Applied) != `{"http":8080,"https":8443}` {
		t.Errorf("pf change %+v", c)
	}

	// A second install finds its redirect.
	f.lines = nil
	if err := run(t, env, pfPlan()); err != nil {
		t.Fatal(err)
	}
	if again, _, _ := receipt.Load(filepath.Join(f.root, receipt.RootPath("me"))); len(again.Changes) != len(r.Changes) || len(f.lines) != 0 {
		t.Errorf("second install: %d changes, commands %v", len(again.Changes), f.lines)
	}

	f.lines = nil
	uninstall(t, f)
	if !slices.Contains(f.lines, "launchctl bootout system/"+PFDaemonLabel) || !slices.Contains(f.lines, "pfctl -a com.apple/gash-hte -F nat") {
		t.Errorf("uninstall commands %v", f.lines)
	}
	if diff := cmp.Diff(before, snapshot(t, f.root)); diff != "" {
		t.Fatalf("tree after uninstall (-before +after):\n%s", diff)
	}
}

// The redirect is refused off macOS, without the com.apple anchors in
// pf.conf, and when pfctl refuses the rules, writing nothing; a load that
// fails leaves a pending change that uninstall undoes.
func TestPFRedirectRefusals(t *testing.T) {
	f, env := newMacFixture(t)
	before := snapshot(t, f.root)
	linux := env
	linux.OS = f.env().OS
	if err := run(t, linux, pfPlan()); hteerr.CodeOf(err) != hteerr.PlatformUnsupported {
		t.Errorf("linux: %v", err)
	}
	f.write(PFConf, "anchor \"mine\"\n", 0o644)
	err := run(t, env, pfPlan())
	if he, ok := hteerr.As(err); !ok || he.Code != hteerr.PlatformUnsupported || !strings.Contains(he.Hint, "--ports high") {
		t.Errorf("pf.conf without the com.apple anchors: %v", err)
	}
	f.write(PFConf, stockPFConf, 0o644)
	f.failOn = "pfctl -n"
	if err := run(t, env, pfPlan()); hteerr.CodeOf(err) != hteerr.PrivilegedStepRequired {
		t.Errorf("rules refused: %v", err)
	}
	snap := snapshot(t, f.root)
	delete(snap, strings.TrimPrefix(receipt.RootDir, "/"))
	delete(snap, strings.TrimPrefix(receipt.RootPath("me"), "/"))
	if diff := cmp.Diff(before, snap); diff != "" {
		t.Errorf("refused rules wrote (-before +after):\n%s", diff)
	}
	f.failOn = "pfctl -E"
	if err := run(t, env, pfPlan()); hteerr.CodeOf(err) != hteerr.PrivilegedStepRequired {
		t.Errorf("load refused: %v", err)
	}
	r, _, _ := receipt.Load(filepath.Join(f.root, receipt.RootPath("me")))
	if c, ok := r.Find(receipt.PFRedirect, ""); !ok || !c.Pending {
		t.Errorf("failed load: %+v", r.Changes)
	}
	f.failOn = ""
	uninstall(t, f)
	if diff := cmp.Diff(before, snapshot(t, f.root)); diff != "" {
		t.Fatalf("tree after uninstall (-before +after):\n%s", diff)
	}
	for _, ports := range [][]int{{80, 443}, {8080}, {8080, 8080}, {8080, 70000}} {
		c := []Op{{ID: 1, Op: OpPFRedirect, Ports: ports}}
		if err := c[0].Check(); hteerr.CodeOf(err) != hteerr.InvalidState {
			t.Errorf("ports %v accepted: %v", ports, err)
		}
	}
}
