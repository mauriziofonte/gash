// Gash 4.1. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package hte

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mauriziofonte/gash/internal/hostshelper"
	"github.com/mauriziofonte/gash/internal/hte/hteerr"
	"github.com/mauriziofonte/gash/internal/hte/receipt"
	"github.com/mauriziofonte/gash/internal/hte/render"
	"github.com/mauriziofonte/gash/internal/hte/rootops"
	"github.com/mauriziofonte/gash/internal/hte/service"
	"github.com/mauriziofonte/gash/internal/hte/state"
	"github.com/mauriziofonte/gash/internal/platform"
)

// layouts are the current and the legacy layout of the rig's user.
func (m *rig) layouts() (cur, old state.Paths) {
	m.t.Helper()
	dirs, err := platform.ResolveDirs(m.e.Getenv)
	if err != nil {
		m.t.Fatal(err)
	}
	return state.NewPaths(dirs, m.e.UID), state.LegacyPaths(dirs, m.e.UID)
}

// legacyTree turns the stopped stack an install of Gash 4.1 left into the
// tree Gash 4.0 left (gash/hte, hte.toml, a raw site naming those
// directories), then starts it from there as the unit of Gash 4.0 would
// (KeepLayout): the rendering and the masters use the old paths.
func (m *rig) legacyTree(ctx context.Context) (cur, old state.Paths) {
	t := m.t
	t.Helper()
	cur, old = m.layouts()
	if running, _ := m.e.supervisor(&env{paths: cur}).Running(); running {
		t.Fatal("legacyTree needs the stack stopped")
	}
	must(t, os.RemoveAll(cur.RenderDir()))
	must(t, os.RemoveAll(cur.RuntimeDir))
	for _, d := range [][2]string{{cur.ConfigDir, old.ConfigDir}, {cur.StateDir, old.StateDir}, {cur.DataDir, old.DataDir}} {
		if fileExists(d[0]) {
			must(t, os.Rename(d[0], d[1]))
		}
	}
	must(t, os.Rename(filepath.Join(old.ConfigDir, state.SettingsName), old.Settings()))
	raw := state.Site{Domain: "raw.test", Aliases: []string{}, Kind: state.KindRaw, Enabled: true,
		RawConf: "<VirtualHost 127.0.0.1:8080>\n    ServerName raw.test\n    ErrorLog " + old.SiteLogDir("raw.test") + "/error.log\n" +
			"    SSLCertificateFile " + old.CertFile("raw.test") + "\n</VirtualHost>\n"}
	must(t, state.Store{Paths: old}.SaveSite(raw))
	must(t, os.MkdirAll(old.SitesDir(), 0o700))
	must(t, os.WriteFile(old.SiteOverlay("http.test"), []byte("CustomLog "+old.LogsDir()+"/extra.log combined\n"), 0o600))
	m.e.KeepLayout = true
	if _, err := m.e.ServiceStart(ctx); err != nil {
		t.Fatalf("start from the legacy tree: %v", err)
	}
	m.e.KeepLayout = false
	if man := m.manifest(old); !strings.HasPrefix(man.Apache.PidFile, old.RuntimeDir+"/") {
		t.Fatalf("the legacy stack runs from %s", man.Apache.PidFile)
	}
	return cur, old
}

// manifest is the current generation's manifest of a layout.
func (m *rig) manifest(p state.Paths) *render.Manifest {
	m.t.Helper()
	l := render.Layout{Dir: p.RenderDir()}
	n, ok, err := l.Current()
	if err != nil || !ok {
		m.t.Fatalf("no current generation in %s: %v", p.RenderDir(), err)
	}
	man, err := render.ReadManifest(l.GenDir(n))
	if err != nil {
		m.t.Fatal(err)
	}
	return man
}

func gone(t *testing.T, paths ...string) {
	t.Helper()
	for _, p := range paths {
		if _, err := os.Lstat(p); !os.IsNotExist(err) {
			t.Errorf("%s is still there", p)
		}
	}
}

// TestMoveLayout: the first command of Gash 4.1 on a tree Gash 4.0 left
// moves it: the stack is stopped, the directories and hte.toml get the
// localweb names, the raw site names the new directories, the rendering of
// the old paths goes, and the stack runs again from the new tree. doctor
// before it only reports the tree; the second command moves nothing.
func TestMoveLayout(t *testing.T) {
	ctx := context.Background()
	m := newRig(t)
	if _, err := m.e.Install(ctx, InstallOptions{Yes: true, Ports: "high", Autostart: "none"}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.e.Migrate(ctx, MigrateOptions{Apply: true, Yes: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.e.ServiceStop(ctx); err != nil {
		t.Fatal(err)
	}
	cur, old := m.legacyTree(ctx)
	var moves []LayoutMove
	m.e.OnLayoutMove = func(mv LayoutMove) { moves = append(moves, mv) }

	doc, err := m.e.Doctor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if i := slices.IndexFunc(doc.Checks, func(c Check) bool { return c.Name == "layout" }); i < 0 || doc.Checks[i].Status != CheckWarn {
		t.Errorf("doctor does not report the legacy tree: %+v", doc.Checks)
	}
	if len(moves) > 0 || !fileExists(old.Settings()) {
		t.Fatal("doctor moved the tree")
	}

	list, err := m.e.SiteList(ctx)
	if err != nil {
		t.Fatalf("site list: %v", err)
	}
	var domains []string
	for _, s := range list.Sites {
		domains = append(domains, s.Domain)
	}
	if !slices.Contains(domains, "raw.test") || !slices.Contains(domains, "http.test") {
		t.Errorf("sites after the move: %v", domains)
	}
	if len(moves) != 1 || !moves[0].Restarted || len(moves[0].Moved) != 3 {
		t.Fatalf("moves %+v", moves)
	}
	if len(moves[0].Warnings) != 1 || !strings.Contains(moves[0].Warnings[0], cur.SiteOverlay("http.test")) {
		t.Errorf("warnings %q, want the overlay that names the old directory", moves[0].Warnings)
	}
	gone(t, old.ConfigDir, old.StateDir, old.DataDir, old.RuntimeDir, filepath.Join(cur.ConfigDir, state.LegacySettingsName))
	if !fileExists(cur.Settings()) {
		t.Fatal("localweb.toml is missing")
	}
	raw, err := state.Store{Paths: cur}.LoadSite("raw.test")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw.RawConf, old.DataDir) || !strings.Contains(raw.RawConf, cur.CertFile("raw.test")) || !strings.Contains(raw.RawConf, cur.SiteLogDir("raw.test")) {
		t.Errorf("raw site:\n%s", raw.RawConf)
	}
	man := m.manifest(cur)
	if !strings.HasPrefix(man.Apache.PidFile, cur.RuntimeDir+"/") {
		t.Errorf("the stack runs from %s", man.Apache.PidFile)
	}
	if gens, _ := (render.Layout{Dir: cur.RenderDir()}).Generations(); len(gens) != 1 {
		t.Errorf("generations after the move: %v", gens)
	}
	if m.stack.Running() == 0 {
		t.Error("the stack does not run after the move")
	}
	audit, _ := os.ReadFile(cur.Audit())
	if !strings.Contains(string(audit), `"command":"install"`) || !strings.Contains(string(audit), `"command":"layout move"`) {
		t.Errorf("audit log:\n%s", audit)
	}

	if _, err := m.e.SiteList(ctx); err != nil || len(moves) != 1 {
		t.Fatalf("second command: %v, moves %d", err, len(moves))
	}
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// TestMoveLayoutSystemd: with the systemd unit of Gash 4.0 the move stops
// the stack through that unit, replaces it with gash-localweb.service and
// starts the stack through the new one.
func TestMoveLayoutSystemd(t *testing.T) {
	ctx := context.Background()
	m := newRig(t)
	m.systemd = true
	must(t, os.MkdirAll(filepath.Join(m.root, "run/systemd/system"), 0o755))
	if _, err := m.e.Install(ctx, InstallOptions{Yes: true, Ports: "high", Autostart: "systemd"}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.e.ServiceStop(ctx); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(m.home, ".config")
	must(t, os.Rename(service.UnitPath(cfg), service.LegacyUnitPath(cfg)))
	cur, _ := m.legacyTree(ctx)
	if !m.unitActive || m.stack.Running() == 0 {
		t.Fatal("the legacy unit did not start the stack")
	}
	_, before := m.lines()

	if _, err := m.e.SiteList(ctx); err != nil {
		t.Fatalf("site list: %v", err)
	}
	_, user := m.lines()
	var jobs []string
	for _, l := range user[len(before):] {
		if verb, ok := strings.CutPrefix(l, "systemctl --user "); ok {
			jobs = append(jobs, verb)
		}
	}
	for _, want := range []string{"stop " + service.LegacyUnitName, "disable " + service.LegacyUnitName, "daemon-reload",
		"enable " + service.UnitName, "start " + service.UnitName} {
		if !slices.Contains(jobs, want) {
			t.Errorf("systemctl --user %s missing from %q", want, jobs)
		}
	}
	gone(t, service.LegacyUnitPath(cfg))
	unit, _ := service.SystemdUnit(m.e.Self)
	if got, _ := os.ReadFile(service.UnitPath(cfg)); string(got) != string(unit) {
		t.Errorf("unit file:\n%s", got)
	}
	if !m.unitActive || m.stack.Running() == 0 || !strings.HasPrefix(m.manifest(cur).Apache.PidFile, cur.RuntimeDir+"/") {
		t.Error("the new unit does not run the stack from the new tree")
	}
	m.mu.Lock()
	direct := slices.Clone(m.direct)
	m.mu.Unlock()
	if len(direct) > 0 {
		t.Errorf("masters started or stopped outside a unit job: %q", direct)
	}
}

// TestMoveLayoutUndone: a move that cannot finish (the new configuration
// directory already holds a sites directory) undoes every rename and
// starts the stack again from the old tree.
func TestMoveLayoutUndone(t *testing.T) {
	ctx := context.Background()
	m := newRig(t)
	if _, err := m.e.Install(ctx, InstallOptions{Yes: true, Ports: "high", Autostart: "none"}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.e.ServiceStop(ctx); err != nil {
		t.Fatal(err)
	}
	cur, old := m.legacyTree(ctx)
	must(t, os.MkdirAll(cur.SitesDir(), 0o700))
	must(t, os.WriteFile(cur.SiteFile("other.test"), []byte("x"), 0o600))
	if _, err := m.e.SiteList(ctx); code(err) != "invalid_state" || !strings.Contains(err.Error(), cur.SitesDir()) {
		t.Fatalf("site list: %v", err)
	}
	for _, p := range []string{old.Settings(), old.SiteFile("raw.test"), old.Audit()} {
		if !fileExists(p) {
			t.Errorf("%s is missing after the undo", p)
		}
	}
	if fileExists(cur.Settings()) || fileExists(cur.Audit()) {
		t.Error("the new tree kept moved files")
	}
	if m.stack.Running() == 0 || !strings.HasPrefix(m.manifest(old).Apache.PidFile, old.RuntimeDir+"/") {
		t.Error("the stack does not run from the old tree again")
	}
}

// TestKeepLayout: root and the hidden commands read a tree of Gash 4.0
// where it is.
func TestKeepLayout(t *testing.T) {
	ctx := context.Background()
	m := newRig(t)
	if _, err := m.e.Install(ctx, InstallOptions{Yes: true, Ports: "high", Autostart: "none"}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.e.ServiceStop(ctx); err != nil {
		t.Fatal(err)
	}
	_, old := m.legacyTree(ctx)
	m.e.KeepLayout = true
	if _, err := m.e.UnitStop(ctx); err != nil {
		t.Fatal(err)
	}
	if !fileExists(old.Settings()) || m.stack.Running() != 0 {
		t.Fatal("the hidden stop moved the tree or left the stack running")
	}
	m.e.KeepLayout = false
	m.e.EUID = 0
	if v, err := m.e.resolve(ctx); err != nil || !v.paths.Legacy {
		t.Fatalf("root resolves %+v %v", v, err)
	}
}

// TestInstallMovesLegacyRoot: install over a stack whose root receipt is
// still under /var/lib/gash-hte moves it first (one root operation at the
// head of the plan): both copies of the receipt name the new paths.
func TestInstallMovesLegacyRoot(t *testing.T) {
	ctx := context.Background()
	m := newRig(t)
	if _, err := m.e.Install(ctx, InstallOptions{Yes: true, Ports: "high", Autostart: "none"}); err != nil {
		t.Fatal(err)
	}
	cur, _ := m.layouts()
	// What Gash 4.0 left: the receipt and the sudoers drop-in under their
	// gash-hte names, both copies of the receipt naming them.
	must(t, os.Rename(filepath.Join(m.root, receipt.RootDir), filepath.Join(m.root, receipt.LegacyRootDir)))
	sudoers := strings.ReplaceAll(m.read(rootops.SudoersPath), "gash localweb", "gash hte")
	must(t, os.Remove(filepath.Join(m.root, rootops.SudoersPath)))
	m.write(rootops.LegacySudoersPath, sudoers, 0o440)
	text := strings.NewReplacer(receipt.RootDir, receipt.LegacyRootDir, rootops.SudoersPath, rootops.LegacySudoersPath).Replace(m.read(receipt.LegacyRootPath(m.e.User)))
	r, err := receipt.Parse([]byte(text), "legacy")
	if err != nil {
		t.Fatal(err)
	}
	for i, c := range r.Changes {
		if c.Path == rootops.LegacySudoersPath {
			r.Changes[i].SHA256 = sumBytes([]byte(sudoers))
		}
	}
	must(t, receipt.Save(filepath.Join(m.root, receipt.LegacyRootPath(m.e.User)), r, 0o644))
	data, _ := os.ReadFile(filepath.Join(m.root, receipt.LegacyRootPath(m.e.User)))
	must(t, os.WriteFile(cur.Receipt(), data, 0o600))

	doc, err := m.e.Doctor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if i := slices.IndexFunc(doc.Checks, func(c Check) bool { return c.Name == "system files" }); i < 0 {
		t.Errorf("doctor does not report the legacy system files: %+v", doc.Checks)
	}

	res, err := m.e.Install(ctx, InstallOptions{Yes: true, Ports: "high", Autostart: "none"})
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if len(res.Plan) == 0 || !strings.HasPrefix(res.Plan[0], "move the files Gash 4.0 installed") {
		t.Errorf("plan %q", res.Plan)
	}
	gone(t, filepath.Join(m.root, receipt.LegacyRootDir), filepath.Join(m.root, rootops.LegacySudoersPath))
	root := m.read(receipt.RootPath(m.e.User))
	if user, _ := os.ReadFile(cur.Receipt()); root == "" || string(user) != root {
		t.Fatalf("receipt copies differ:\nroot %s\nuser %s", root, user)
	}
	if strings.Contains(root, "gash-hte") || m.read(rootops.SudoersPath) != string(rootops.SudoersContent(m.e.User)) {
		t.Errorf("after install: receipt %s, sudoers %q", root, m.read(rootops.SudoersPath))
	}
	if _, err := m.e.Uninstall(ctx, UninstallOptions{Yes: true}); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	gone(t, filepath.Join(m.root, receipt.RootDir), filepath.Join(m.root, rootops.SudoersPath))
}

// legacyMarkers gives the hosts block the markers of Gash 4.0.
func (m *rig) legacyMarkers() {
	m.t.Helper()
	h := m.read(hostshelper.HostsFile)
	if !strings.Contains(h, hostshelper.BeginMarker) {
		m.t.Fatalf("no hosts block:\n%s", h)
	}
	h = strings.Replace(strings.Replace(h, hostshelper.BeginMarker, hostshelper.LegacyBeginMarker, 1), hostshelper.EndMarker, hostshelper.LegacyEndMarker, 1)
	m.write(hostshelper.HostsFile, h, 0o644)
}

// TestHostsMarkersMove: a hosts block between the markers of Gash 4.0
// gets the current markers at the next write once the installed helper is
// this gash's own; with the helper of Gash 4.0 still installed, which
// knows only the old markers, it is left as it is.
func TestHostsMarkersMove(t *testing.T) {
	ctx := context.Background()
	m := newRig(t)
	if _, err := m.e.Install(ctx, InstallOptions{Yes: true, Ports: "high", Autostart: "none"}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.e.Migrate(ctx, MigrateOptions{Apply: true, Yes: true}); err != nil {
		t.Fatal(err)
	}
	m.legacyMarkers()
	helper := m.read(rootops.HelperPath)
	m.write(rootops.HelperPath, "#!the helper of Gash 4.0\n", 0o755)
	if _, err := m.e.ServiceReload(ctx); err != nil {
		t.Fatal(err)
	}
	if !hostshelper.Legacy([]byte(m.read(hostshelper.HostsFile))) {
		t.Fatal("the block got the new markers while the old helper is installed")
	}
	m.write(rootops.HelperPath, helper, 0o755)
	if _, err := m.e.ServiceReload(ctx); err != nil {
		t.Fatal(err)
	}
	h := []byte(m.read(hostshelper.HostsFile))
	if block, found, err := hostshelper.Extract(h); hostshelper.Legacy(h) || err != nil || !found || !strings.Contains(string(block), "http.test") {
		t.Fatalf("hosts after the reload with the current helper:\n%s", h)
	}
}

// TestInstallMovesHostsMarkers: install gives a block of Gash 4.0 the
// current markers once it put the current helper in place, also when the
// start that follows writes nothing (a systemd unit already active).
func TestInstallMovesHostsMarkers(t *testing.T) {
	ctx := context.Background()
	m := newRig(t)
	m.systemd = true
	must(t, os.MkdirAll(filepath.Join(m.root, "run/systemd/system"), 0o755))
	if _, err := m.e.Install(ctx, InstallOptions{Yes: true, Ports: "high", Autostart: "systemd"}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.e.Migrate(ctx, MigrateOptions{Apply: true, Yes: true}); err != nil {
		t.Fatal(err)
	}
	m.legacyMarkers()
	m.write(rootops.HelperPath, "#!the helper of Gash 4.0\n", 0o755)
	if !m.unitActive {
		t.Fatal("the unit is not active")
	}
	if _, err := m.e.Install(ctx, InstallOptions{Yes: true, Ports: "high", Autostart: "systemd"}); err != nil {
		t.Fatal(err)
	}
	if h := []byte(m.read(hostshelper.HostsFile)); hostshelper.Legacy(h) {
		t.Fatalf("hosts after install:\n%s", h)
	}
}

// TestMoveLayoutResumed: a move interrupted after some entries moved (the
// old hte.toml still in place) is finished by the next command.
func TestMoveLayoutResumed(t *testing.T) {
	ctx := context.Background()
	m := newRig(t)
	if _, err := m.e.Install(ctx, InstallOptions{Yes: true, Ports: "high", Autostart: "none"}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.e.ServiceStop(ctx); err != nil {
		t.Fatal(err)
	}
	cur, old := m.legacyTree(ctx)
	m.e.KeepLayout = true
	if _, err := m.e.ServiceStop(ctx); err != nil {
		t.Fatal(err)
	}
	m.e.KeepLayout = false
	// What a move killed half way leaves: the sites and the state moved,
	// hte.toml and the overlay still in the old directory.
	must(t, os.MkdirAll(cur.ConfigDir, 0o700))
	must(t, os.Rename(old.SitesDir(), cur.SitesDir()))
	must(t, os.MkdirAll(old.SitesDir(), 0o700))
	must(t, os.Rename(cur.SiteOverlay("http.test"), old.SiteOverlay("http.test")))
	must(t, os.Rename(old.StateDir, cur.StateDir))
	if _, err := m.e.SiteList(ctx); err == nil {
		// sites/ exists on both sides: the move cannot merge it and stops,
		// changing nothing more.
		t.Fatal("the move merged two sites directories")
	}
	must(t, os.Rename(old.SiteOverlay("http.test"), cur.SiteOverlay("http.test")))
	must(t, os.Remove(old.SitesDir()))
	list, err := m.e.SiteList(ctx)
	if err != nil {
		t.Fatalf("site list: %v", err)
	}
	if len(list.Sites) == 0 || !fileExists(cur.Settings()) {
		t.Fatalf("after the resumed move: %+v", list)
	}
	gone(t, old.ConfigDir, old.StateDir, old.DataDir)
	raw, err := state.Store{Paths: cur}.LoadSite("raw.test")
	if err != nil || strings.Contains(raw.RawConf, old.DataDir) {
		t.Errorf("raw site after the resumed move: %v\n%s", err, raw.RawConf)
	}
}

// TestKeepLayoutWaited: a command that read the tree of Gash 4.0 where it
// is, and waited for the lock while another command moved it, does not
// work on what is left: lock_timeout (RETRY), and the state directory its
// lock made again goes.
func TestKeepLayoutWaited(t *testing.T) {
	ctx := context.Background()
	m := newRig(t)
	if _, err := m.e.Install(ctx, InstallOptions{Yes: true, Ports: "high", Autostart: "none"}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.e.ServiceStop(ctx); err != nil {
		t.Fatal(err)
	}
	_, old := m.legacyTree(ctx)
	m.e.KeepLayout = true
	v, err := m.e.resolve(ctx)
	if err != nil || !v.paths.Legacy {
		t.Fatalf("resolve: %+v %v", v, err)
	}
	m.e.KeepLayout = false
	if _, err := m.e.SiteList(ctx); err != nil {
		t.Fatal(err)
	}
	ran := false
	_, err = m.e.locked(v, "service start", "", func() (any, error) { ran = true; return nil, nil })
	if code(err) != hteerr.LockTimeout || ran {
		t.Fatalf("locked on the moved tree: %v, ran %v", err, ran)
	}
	gone(t, old.StateDir)
}
