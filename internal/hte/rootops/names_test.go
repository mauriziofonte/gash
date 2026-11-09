// Gash 4.1. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package rootops

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/mauriziofonte/gash/internal/hostshelper"
	"github.com/mauriziofonte/gash/internal/hte/hteerr"
	"github.com/mauriziofonte/gash/internal/hte/receipt"
)

// legacyNames maps what Gash 4.1 writes to what Gash 4.0 wrote, in the
// receipt and in the files gash owns.
var legacyNames = []string{
	receipt.RootDir, receipt.LegacyRootDir,
	SudoersPath, LegacySudoersPath,
	SysctlFile, LegacySysctlFile,
	PFAnchorFile, LegacyPFAnchorFile,
	PFDaemonPath, LegacyPFDaemonPath,
	PFDaemonLabel, LegacyPFDaemonLabel,
	PFAnchor, LegacyPFAnchor,
	"gash-localweb.service", "gash-hte.service",
	"gash localweb", "gash hte",
}

// legacyize turns the tree an install of Gash 4.1 left into the one Gash
// 4.0 left: the receipt under /var/lib/gash-hte with its backups and
// archives, the gash-hte names and content of the sudoers and sysctl
// drop-ins and of the pf files, the legacy markers of the hosts block.
func legacyize(t *testing.T, f *fixture) {
	t.Helper()
	old := strings.NewReplacer(legacyNames...)
	if err := os.Rename(filepath.Join(f.root, receipt.RootDir), filepath.Join(f.root, receipt.LegacyRootDir)); err != nil {
		t.Fatal(err)
	}
	moved := map[string]string{}
	for _, p := range [][2]string{{SudoersPath, LegacySudoersPath}, {SysctlFile, LegacySysctlFile}, {PFAnchorFile, LegacyPFAnchorFile}, {PFDaemonPath, LegacyPFDaemonPath}} {
		fi, err := os.Stat(filepath.Join(f.root, p[0]))
		if err != nil {
			continue
		}
		content := old.Replace(f.read(p[0]))
		if err := os.Remove(filepath.Join(f.root, p[0])); err != nil {
			t.Fatal(err)
		}
		f.write(p[1], content, fi.Mode().Perm())
		moved[p[1]] = content
	}
	if err := hostshelper.Rewrite(filepath.Join(f.root, hostshelper.HostsFile), func(h []byte) ([]byte, error) {
		s := strings.Replace(string(h), hostshelper.BeginMarker, hostshelper.LegacyBeginMarker, 1)
		return []byte(strings.Replace(s, hostshelper.EndMarker, hostshelper.LegacyEndMarker, 1)), nil
	}); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(f.root, receipt.LegacyRootPath("me"))
	r, err := receipt.Parse([]byte(old.Replace(f.read(receipt.LegacyRootPath("me")))), file)
	if err != nil {
		t.Fatal(err)
	}
	for i, c := range r.Changes {
		if content, ok := moved[c.Path]; ok && (c.Kind == receipt.FileCreated || c.Kind == receipt.FileModified) {
			r.Changes[i].SHA256 = sha(content)
		}
	}
	if err := receipt.Save(file, r, 0o644); err != nil {
		t.Fatal(err)
	}
}

// legacyPaths lists every path of the receipt that still carries a legacy
// name.
func legacyPaths(r *receipt.Receipt) []string {
	var out []string
	for _, c := range r.Changes {
		refs := []string{c.Path, c.Backup, c.Archive, c.Name}
		if c.File != nil {
			refs = append(refs, c.File.Path)
			if c.File.Prior != nil {
				refs = append(refs, *c.File.Prior)
			}
		}
		for _, p := range refs {
			if strings.Contains(p, "gash-hte") || strings.Contains(p, "gash.hte") {
				out = append(out, p)
			}
		}
	}
	return out
}

func rootReceipt(t *testing.T, f *fixture) *receipt.Receipt {
	t.Helper()
	data, p, err := receipt.ReadRoot(f.sr.ReadFile, "me")
	if err != nil {
		t.Fatal(err)
	}
	r, err := receipt.Parse(data, p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// TestMigrateNames: a tree Gash 4.0 installed (the receipt under
// /var/lib/gash-hte with an archived vhost, the gash-hte drop-ins, the
// legacy hosts markers) moves to the gash-localweb names with one root
// operation; the copies of the receipt stay equal, the kernel setting and
// the system units are not touched, a second run moves nothing, and
// uninstall then gives back the tree as it was before install.
func TestMigrateNames(t *testing.T) {
	f := newFixture(t)
	before := snapshot(t, f.root)
	if err := run(t, f.env(), installPlan()); err != nil {
		t.Fatal(err)
	}
	if err := hostshelper.Rewrite(filepath.Join(f.root, "etc/hosts"), func(h []byte) ([]byte, error) {
		return hostshelper.Splice(h, []byte("127.0.0.1 http.test\n"))
	}); err != nil {
		t.Fatal(err)
	}
	legacyize(t, f)
	if legacy := legacyPaths(rootReceipt(t, f)); len(legacy) == 0 {
		t.Fatal("legacyize left no legacy path in the receipt")
	}

	f.lines = nil
	var userCopy string
	plan := []Op{{ID: 1, Op: OpMigrateNames}}
	c := session(t, f.env(), plan)
	c.OnReceipt = func(text string) error { userCopy = text; return nil }
	if _, err := c.Run(1); err != nil {
		t.Fatal(err)
	}
	c.Close()

	if userCopy == "" || userCopy != f.read(receipt.RootPath("me")) {
		t.Fatalf("user copy %q differs from the root copy", userCopy)
	}
	r := rootReceipt(t, f)
	if legacy := legacyPaths(r); len(legacy) > 0 {
		t.Fatalf("legacy paths left in the receipt: %v", legacy)
	}
	for _, p := range []string{receipt.LegacyRootDir, LegacySudoersPath, LegacySysctlFile} {
		if _, err := os.Lstat(filepath.Join(f.root, p)); !os.IsNotExist(err) {
			t.Errorf("%s is still there", p)
		}
	}
	if got := f.read(SudoersPath); got != string(SudoersContent("me")) {
		t.Errorf("sudoers %q", got)
	}
	if fi, err := os.Stat(filepath.Join(f.root, SudoersPath)); err != nil || fi.Mode().Perm() != 0o440 {
		t.Errorf("sudoers mode %v %v", fi, err)
	}
	if got := f.read(SysctlFile); got != string(sysctlContent("80")) {
		t.Errorf("sysctl drop-in %q", got)
	}
	if fi, err := os.Stat(filepath.Join(f.root, receipt.RootDir)); err != nil || fi.Mode().Perm() != 0o755 {
		t.Errorf("%s: %v %v", receipt.RootDir, fi, err)
	}
	if c, ok := r.Find(receipt.LegacyVhost, ""); !ok || !strings.HasPrefix(c.Archive, receipt.ArchiveDir+"/") || f.read(c.Archive) == "" {
		t.Errorf("archived vhost %+v", c)
	}
	if c, ok := r.Find(receipt.FileCreated, SudoersPath); !ok || c.SHA256 != sha(string(SudoersContent("me"))) {
		t.Errorf("sudoers change %+v", c)
	}
	// The hosts block keeps its markers: the installed helper may still be
	// the one of Gash 4.0; the user side gives it the new ones later.
	hosts := f.read(hostshelper.HostsFile)
	if block, found, err := hostshelper.Extract([]byte(hosts)); err != nil || !found || string(block) != "127.0.0.1 http.test\n" || !hostshelper.Legacy([]byte(hosts)) {
		t.Errorf("hosts block %q %v %v legacy %v", block, found, err, hostshelper.Legacy([]byte(hosts)))
	}
	if diff := cmp.Diff([]string{"visudo -c -f " + filepath.Join(f.root, sudoersTmp)}, f.lines); diff != "" {
		t.Errorf("commands (-want +got):\n%s", diff)
	}

	// Run again: nothing moves.
	moved := snapshot(t, f.root)
	f.lines = nil
	if err := run(t, f.env(), plan); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(moved, snapshot(t, f.root)); diff != "" || len(f.lines) > 0 {
		t.Fatalf("second run changed the tree (-before +after):\n%s\ncommands %v", diff, f.lines)
	}

	// Install again (4.1) records nothing new.
	if err := run(t, f.env(), installPlan()[:4]); err != nil {
		t.Fatal(err)
	}
	if again := rootReceipt(t, f); len(again.Changes) != len(r.Changes) {
		t.Fatalf("install after the move recorded %d changes, had %d", len(again.Changes), len(r.Changes))
	}

	uninstall(t, f)
	if diff := cmp.Diff(before, snapshot(t, f.root)); diff != "" {
		t.Fatalf("tree after uninstall (-before +after):\n%s", diff)
	}
}

// TestMigrateNamesInterrupted: a move that stops half way (visudo refuses)
// leaves a receipt that names every file where it is: the next run
// finishes the move, and so does an uninstall instead of a new run.
func TestMigrateNamesInterrupted(t *testing.T) {
	for _, then := range []string{"run again", "uninstall"} {
		t.Run(then, func(t *testing.T) {
			f := newFixture(t)
			before := snapshot(t, f.root)
			if err := run(t, f.env(), installPlan()); err != nil {
				t.Fatal(err)
			}
			legacyize(t, f)
			f.failOn = "visudo"
			if err := run(t, f.env(), []Op{{ID: 1, Op: OpMigrateNames}}); err == nil {
				t.Fatal("the move succeeded with visudo failing")
			}
			f.failOn = ""
			if _, err := os.Lstat(filepath.Join(f.root, receipt.LegacyRootPath("me"))); err != nil {
				t.Fatalf("the legacy receipt is gone after a failed move: %v", err)
			}
			if f.read(LegacySudoersPath) == "" {
				t.Fatal("the legacy sudoers drop-in is gone after a failed move")
			}
			r := rootReceipt(t, f)
			if c, ok := r.Find(receipt.LegacyVhost, ""); !ok || !strings.HasPrefix(c.Archive, receipt.ArchiveDir+"/") || f.read(c.Archive) == "" {
				t.Fatalf("the receipt does not name the moved archive: %+v", c)
			}
			if then == "run again" {
				if err := run(t, f.env(), []Op{{ID: 1, Op: OpMigrateNames}}); err != nil {
					t.Fatal(err)
				}
				if legacy := legacyPaths(rootReceipt(t, f)); len(legacy) > 0 {
					t.Fatalf("legacy paths left: %v", legacy)
				}
			}
			uninstall(t, f)
			if diff := cmp.Diff(before, snapshot(t, f.root)); diff != "" {
				t.Fatalf("tree after uninstall (-before +after):\n%s", diff)
			}
		})
	}
}

// TestMigrateNamesForeignFiles: a legacy sudoers drop-in that changed since
// install (another user installed later) stays, with a warning; a drop-in
// removed by hand is not written again.
func TestMigrateNamesForeignFiles(t *testing.T) {
	f := newFixture(t)
	if err := run(t, f.env(), installPlan()[:2]); err != nil {
		t.Fatal(err)
	}
	legacyize(t, f)
	f.write(LegacySudoersPath, "# another user\nbob ALL=(root) NOPASSWD: "+HelperPath+" apply\n", 0o440)
	c := session(t, f.env(), []Op{{ID: 1, Op: OpMigrateNames}})
	resp, err := c.Run(1)
	c.Close()
	if err != nil || len(resp.Warnings) != 1 || !strings.Contains(resp.Warnings[0], LegacySudoersPath) {
		t.Fatalf("move: %v warnings %q", err, resp.Warnings)
	}
	if !strings.Contains(f.read(LegacySudoersPath), "bob") || f.read(SudoersPath) != string(SudoersContent("me")) {
		t.Error("the other user's drop-in was touched, or mine was not written")
	}

	g := newFixture(t)
	if err := run(t, g.env(), installPlan()[:2]); err != nil {
		t.Fatal(err)
	}
	legacyize(t, g)
	must(t, os.Remove(filepath.Join(g.root, LegacySudoersPath)))
	if err := run(t, g.env(), []Op{{ID: 1, Op: OpMigrateNames}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(g.root, SudoersPath)); !os.IsNotExist(err) {
		t.Error("a drop-in removed by hand was written again")
	}
	if c, ok := rootReceipt(t, g).Find(receipt.FileCreated, SudoersPath); !ok || c.SHA256 != sha(string(SudoersContent("me"))) {
		t.Errorf("sudoers change %+v", c)
	}
}

// TestMigrateNamesLegacyCopy: a legacy receipt next to the current one is
// removed when it is the copy an interrupted move left, and refused when it
// records another install.
func TestMigrateNamesLegacyCopy(t *testing.T) {
	f := newFixture(t)
	if err := run(t, f.env(), installPlan()[:2]); err != nil {
		t.Fatal(err)
	}
	legacyize(t, f)
	stale := f.read(receipt.LegacyRootPath("me"))
	if err := run(t, f.env(), []Op{{ID: 1, Op: OpMigrateNames}}); err != nil {
		t.Fatal(err)
	}
	f.write(receipt.LegacyRootPath("me"), stale, 0o644)
	if err := run(t, f.env(), []Op{{ID: 1, Op: OpMigrateNames}}); err != nil {
		t.Fatalf("the stale copy: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(f.root, receipt.LegacyRootDir)); !os.IsNotExist(err) {
		t.Fatal("the stale copy is still there")
	}
	r, err := receipt.Parse([]byte(stale), "stale")
	if err != nil {
		t.Fatal(err)
	}
	r.Created = r.Created.Add(time.Hour)
	must(t, receipt.Save(filepath.Join(f.root, receipt.LegacyRootPath("me")), r, 0o644))
	if err := run(t, f.env(), []Op{{ID: 1, Op: OpMigrateNames}}); hteerr.CodeOf(err) != hteerr.InvalidState {
		t.Fatalf("another install's receipt: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(f.root, receipt.LegacyRootPath("me"))); err != nil {
		t.Fatal("another install's receipt was removed")
	}
}

// TestUninstallLegacyReceipt: uninstall of a tree Gash 4.0 installed, never
// moved, undoes every change where it is and removes /var/lib/gash-hte; on
// macOS it empties the legacy pf anchor and unloads the legacy daemon.
func TestUninstallLegacyReceipt(t *testing.T) {
	f := newFixture(t)
	before := snapshot(t, f.root)
	if err := run(t, f.env(), installPlan()); err != nil {
		t.Fatal(err)
	}
	legacyize(t, f)
	uninstall(t, f)
	if diff := cmp.Diff(before, snapshot(t, f.root)); diff != "" {
		t.Fatalf("tree after uninstall (-before +after):\n%s", diff)
	}

	m, env := newMacFixture(t)
	before = snapshot(t, m.root)
	if err := run(t, env, pfPlan()); err != nil {
		t.Fatal(err)
	}
	legacyize(t, m)
	m.lines = nil
	data, p, err := receipt.ReadRoot(m.sr.ReadFile, "me")
	if err != nil {
		t.Fatal(err)
	}
	r, err := receipt.Parse(data, p)
	if err != nil {
		t.Fatal(err)
	}
	var plan []Op
	for i := len(r.Changes) - 1; i >= 0; i-- {
		plan = append(plan, Op{ID: len(plan) + 1, Op: OpRestore, Change: r.Changes[i].ID})
	}
	plan = append(plan, Op{ID: len(plan) + 1, Op: OpFinish})
	if err := run(t, env, plan); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"launchctl bootout system/" + LegacyPFDaemonLabel, "pfctl -a " + LegacyPFAnchor + " -F all"} {
		if !slices.Contains(m.lines, want) {
			t.Errorf("uninstall commands %v lack %q", m.lines, want)
		}
	}
	if diff := cmp.Diff(before, snapshot(t, m.root)); diff != "" {
		t.Fatalf("macOS tree after uninstall (-before +after):\n%s", diff)
	}
}

// TestMigrateNamesPF: on macOS the pf anchor and its LaunchDaemon move to
// their current names; the rules are loaded into the current anchor before
// the legacy one is emptied and its daemon unloaded.
func TestMigrateNamesPF(t *testing.T) {
	f, env := newMacFixture(t)
	before := snapshot(t, f.root)
	if err := run(t, env, pfPlan()); err != nil {
		t.Fatal(err)
	}
	legacyize(t, f)
	f.lines = nil
	if err := run(t, env, []Op{{ID: 1, Op: OpMigrateNames}}); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"pfctl -n -a " + PFAnchor + " -f " + filepath.Join(f.root, pfAnchorTmp),
		"plutil -lint " + filepath.Join(f.root, pfDaemonTmp),
		"pfctl -E -a " + PFAnchor + " -f " + PFAnchorFile,
		"launchctl bootout system/" + LegacyPFDaemonLabel,
		"pfctl -a " + LegacyPFAnchor + " -F all",
	}
	if diff := cmp.Diff(want, f.lines); diff != "" {
		t.Errorf("commands (-want +got):\n%s", diff)
	}
	if f.read(PFAnchorFile) != string(PFRules(8080, 8443)) || f.read(PFDaemonPath) != string(PFDaemon()) {
		t.Error("the pf files do not hold the current content")
	}
	for _, p := range []string{LegacyPFAnchorFile, LegacyPFDaemonPath} {
		if _, err := os.Lstat(filepath.Join(f.root, p)); !os.IsNotExist(err) {
			t.Errorf("%s is still there", p)
		}
	}
	r := rootReceipt(t, f)
	if c, ok := r.Find(receipt.PFRedirect, ""); !ok || c.Name != PFAnchor || c.Path != PFAnchorFile {
		t.Errorf("pf change %+v", c)
	}
	if c, ok := r.Find(receipt.Autostart, ""); !ok || c.AutoMode != "launchd" {
		t.Errorf("autostart change %+v", c)
	}
	f.lines = nil
	uninstall(t, f)
	if !slices.Contains(f.lines, "pfctl -a "+PFAnchor+" -F all") {
		t.Errorf("uninstall commands %v", f.lines)
	}
	if diff := cmp.Diff(before, snapshot(t, f.root)); diff != "" {
		t.Fatalf("tree after uninstall (-before +after):\n%s", diff)
	}
}

// TestAutostartPath: the unit and the LaunchAgent of Gash 4.0 get their
// current names; any other path stays.
func TestAutostartPath(t *testing.T) {
	for in, want := range map[string]string{
		"/home/me/.config/systemd/user/gash-hte.service":                        "/home/me/.config/systemd/user/gash-localweb.service",
		"/Users/me/Library/LaunchAgents/io.github.mauriziofonte.gash.hte.plist": "/Users/me/Library/LaunchAgents/io.github.mauriziofonte.gash.localweb.plist",
		"/home/me/.config/systemd/user/other.service":                           "/home/me/.config/systemd/user/other.service",
	} {
		if got := autostartPath(in); got != want {
			t.Errorf("%s: %s, want %s", in, got, want)
		}
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
