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

	"github.com/mauriziofonte/gash/internal/hte/hteerr"
	"github.com/mauriziofonte/gash/internal/hte/proc"
	"github.com/mauriziofonte/gash/internal/hte/state"
	"github.com/mauriziofonte/gash/internal/platform"
)

const sysroots = "../../testdata/hte/sysroot"

// fixture is an engine on the Debian sysroot fixture with a fake runner:
// the FPM binaries answer as PHP 8.3, Apache lists no built-in modules,
// every config test passes unless failTest names the binary.
type fixture struct {
	e        *Engine
	run      *proc.Fake
	home     string
	failTest string
	env      map[string]string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join(sysroots, "debian"))
	if err != nil {
		t.Fatal(err)
	}
	sr, err := platform.NewSysroot(dir)
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{home: t.TempDir()}
	f.run = &proc.Fake{Handler: func(c proc.Cmd) (proc.Result, error) {
		switch {
		case strings.HasPrefix(filepath.Base(c.Path), "php-fpm") && slices.Equal(c.Args, []string{"-v"}):
			v := strings.TrimPrefix(filepath.Base(c.Path), "php-fpm")
			return proc.Result{Stdout: []byte("PHP " + v + ".6 (fpm-fcgi) (built: Oct  1 2026 10:00:00)\n")}, nil
		case strings.HasPrefix(filepath.Base(c.Path), "php-fpm") && slices.Equal(c.Args, []string{"-i"}):
			v := strings.TrimPrefix(filepath.Base(c.Path), "php-fpm")
			return proc.Result{Stdout: []byte("Scan this dir for additional .ini files => /etc/php/" + v + "/fpm/conf.d\n")}, nil
		case filepath.Base(c.Path) == "apache2" && slices.Equal(c.Args, []string{"-l"}):
			return proc.Result{Stdout: []byte("Compiled in modules:\n  core.c\n  mod_so.c\n  mod_watchdog.c\n  http_core.c\n  mod_log_config.c\n  mod_logio.c\n  mod_version.c\n  mod_unixd.c\n")}, nil
		case len(c.Args) > 0 && c.Args[0] == "-t" && f.failTest != "" && filepath.Base(c.Path) == f.failTest:
			return proc.Result{ExitCode: 1, Stderr: []byte("AH00526: Syntax error on line 1")}, nil
		}
		return proc.Result{}, nil
	}}
	f.env = map[string]string{
		"HOME": f.home, "XDG_RUNTIME_DIR": filepath.Join(f.home, "run"),
		// Who the user is never comes from these (LEDGER HTE-19).
		"USER": "root", "LOGNAME": "root", "SUDO_USER": "alice", "SUDO_UID": "0", "SUDO_GID": "0",
	}
	f.e = &Engine{
		Sysroot: sr, GOOS: "linux", Getenv: func(k string) string { return f.env[k] },
		// UID owns the temporary tree; EUID is only ever compared with 0.
		UID: os.Getuid(), EUID: 4242, GID: os.Getgid(), User: "dev", Cwd: f.home,
		Self: "/usr/local/bin/gash", Helper: "/usr/local/bin/gash-hosts-helper",
		Runner: f.run,
	}
	return f
}

// install writes hte.toml as install would, without the root steps.
func (f *fixture) install(t *testing.T) *env {
	t.Helper()
	if err := os.MkdirAll(f.env["XDG_RUNTIME_DIR"], 0o700); err != nil {
		t.Fatal(err)
	}
	v, err := f.e.resolve()
	if err != nil {
		t.Fatal(err)
	}
	if err := v.store.SaveSettings(state.DefaultSettings(state.HighPorts, "none")); err != nil {
		t.Fatal(err)
	}
	return v
}

func (f *fixture) docroot(t *testing.T) string {
	t.Helper()
	d := filepath.Join(f.home, "www", "app", "public")
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	return d
}

func ptr[T any](v T) *T { return &v }

func code(err error) hteerr.Code {
	if he, ok := hteerr.As(err); ok {
		return he.Code
	}
	return ""
}

// TestLedger_HTE19: the user is the process's uid; USER, LOGNAME and
// SUDO_* change nothing. With euid 0 every command but install,
// uninstall, migrate and doctor is root_not_allowed, before it reads or
// writes anything.
func TestLedger_HTE19(t *testing.T) {
	f := newFixture(t)
	delete(f.env, "XDG_RUNTIME_DIR")
	f.e.UID = 4242
	v, err := f.e.resolve()
	if err != nil {
		t.Fatal(err)
	}
	if v.paths.RuntimeDir != "/tmp/gash-hte-4242" {
		t.Errorf("runtime dir %q, want the one of uid 4242", v.paths.RuntimeDir)
	}
	if v.paths.ConfigDir != filepath.Join(f.home, ".config", "gash", "hte") {
		t.Errorf("config dir %q", v.paths.ConfigDir)
	}

	f.e.EUID = 0
	ctx := context.Background()
	calls := map[string]func() error{
		"site create": func() error {
			_, err := f.e.SiteCreate(ctx, state.Flags{Domain: "a.test", Docroot: ptr(f.home)})
			return err
		},
		"site modify":    func() error { _, err := f.e.SiteModify(ctx, state.Flags{Domain: "a.test"}); return err },
		"site remove":    func() error { _, err := f.e.SiteRemove(ctx, "a.test", false); return err },
		"site list":      func() error { _, err := f.e.SiteList(ctx); return err },
		"site show":      func() error { _, err := f.e.SiteShow(ctx, "a.test"); return err },
		"service start":  func() error { _, err := f.e.ServiceStart(ctx); return err },
		"service status": func() error { _, err := f.e.ServiceStatus(ctx); return err },
		"service logs":   func() error { _, err := f.e.ServiceLogs(ctx, "", 10); return err },
		"cert renew":     func() error { _, err := f.e.CertRenew(ctx, "a.test"); return err },
		"hosts apply":    func() error { _, err := f.e.HostsApply(ctx); return err },
	}
	for name, call := range calls {
		if err := call(); code(err) != hteerr.RootNotAllowed {
			t.Errorf("%s as euid 0: %v, want root_not_allowed", name, err)
		}
	}
	for _, c := range []string{"install", "uninstall", "migrate", "doctor"} {
		if err := f.e.CheckIdentity(c); err != nil {
			t.Errorf("%s as euid 0: %v", c, err)
		}
	}
	if entries, _ := os.ReadDir(f.home); len(entries) != 0 {
		t.Errorf("a refused command wrote under HOME: %v", entries)
	}
	if len(f.run.Calls()) != 0 {
		t.Errorf("a refused command ran %v", f.run.Lines())
	}
	if he, _ := hteerr.As(f.e.CheckIdentity("site list")); he == nil || he.Action() != hteerr.Fatal {
		t.Errorf("root_not_allowed is not FATAL: %v", he)
	}
}

// TestLedger_HTE12: a step that fails makes the command fail and leaves
// the state as it was; site modify with no effective change succeeds
// with changed=false and runs nothing.
func TestLedger_HTE12(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	v := f.install(t)
	root := f.docroot(t)
	flags := state.Flags{Domain: "app.test", Docroot: ptr(root), HTTPS: ptr(false)}

	// The Apache config test fails: no site file, no generation.
	f.failTest = "apache2"
	_, err := f.e.SiteCreate(ctx, flags)
	if code(err) != hteerr.ConfigTestFailed {
		t.Fatalf("site create with a failing config test: %v, want config_test_failed", err)
	}
	if _, err := os.Stat(v.paths.SiteFile("app.test")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the site file stayed after the failure: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(v.paths.RenderDir(), "current")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a generation became current after the failure: %v", err)
	}

	// The same create, passing.
	f.failTest = ""
	res, err := f.e.SiteCreate(ctx, flags)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Changed || res.Applied == nil || res.Site.PHP != "8.3" {
		t.Fatalf("site create: %+v", res)
	}
	if _, err := os.Stat(v.paths.SiteFile("app.test")); err != nil {
		t.Fatal(err)
	}
	gen := res.Applied.Generation

	// A modify that changes nothing: changed=false, no command run.
	before := len(f.run.Calls())
	res, err = f.e.SiteModify(ctx, state.Flags{Domain: "app.test", Docroot: ptr(root), HTTPS: ptr(false)})
	if err != nil {
		t.Fatal(err)
	}
	if res.Changed || res.Applied != nil {
		t.Errorf("no-op modify: %+v", res)
	}
	if got := f.run.Calls()[before:]; len(got) != 0 {
		t.Errorf("no-op modify ran %d commands", len(got))
	}

	// A modify whose config test fails: the previous site file is back.
	f.failTest = "php-fpm8.3"
	old, _ := os.ReadFile(v.paths.SiteFile("app.test"))
	_, err = f.e.SiteModify(ctx, state.Flags{Domain: "app.test", Profile: ptr("hardened")})
	if code(err) != hteerr.ConfigTestFailed {
		t.Fatalf("modify with a failing FPM test: %v", err)
	}
	if now, _ := os.ReadFile(v.paths.SiteFile("app.test")); string(now) != string(old) {
		t.Errorf("the site file changed after a failed modify:\n%s", now)
	}
	n, _, _, err := f.e.supervisor(v).Current()
	if err != nil || n != gen {
		t.Errorf("current generation %d (%v), want %d", n, err, gen)
	}

	// Remove needs --yes; with it the site and its files go.
	if _, err := f.e.SiteRemove(ctx, "app.test", false); code(err) != hteerr.ConfirmationRequired {
		t.Errorf("remove without --yes: %v", err)
	}
	f.failTest = ""
	if _, err := f.e.SiteRemove(ctx, "app.test", true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(v.paths.SiteFile("app.test")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("site file after remove: %v", err)
	}
	if _, err := os.Stat(root); err != nil {
		t.Errorf("remove touched the document root: %v", err)
	}
}

// Before install every command but install, migrate and doctor is
// hte_not_installed.
func TestNotInstalled(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	if _, err := f.e.SiteList(ctx); code(err) != hteerr.HTENotInstalled {
		t.Errorf("site list: %v", err)
	}
	if _, err := f.e.ServiceStart(ctx); code(err) != hteerr.HTENotInstalled {
		t.Errorf("service start: %v", err)
	}
	if _, err := f.e.Uninstall(ctx, UninstallOptions{Yes: true}); code(err) != hteerr.HTENotInstalled {
		t.Errorf("uninstall: %v", err)
	}
}

// php ini and php global: the site's [php_ini] reaches its pool; the
// global overlay is a file of key lines, edited in place.
func TestPHPCommands(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	v := f.install(t)
	if _, err := f.e.SiteCreate(ctx, state.Flags{Domain: "app.test", Docroot: ptr(f.docroot(t)), HTTPS: ptr(false)}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.e.PHPIniSet(ctx, "app.test", "memory_limit", `5"M`); code(err) != hteerr.InvalidState {
		t.Errorf("a quote in a value: %v", err)
	}
	res, err := f.e.PHPIniSet(ctx, "app.test", "memory_limit", "321M")
	if err != nil || !res.Changed {
		t.Fatalf("php ini set: %+v %v", res, err)
	}
	pool, err := os.ReadFile(filepath.Join(v.paths.RenderDir(), "current", "fpm", "8.3", "pools", "app.test.conf"))
	if err != nil || !strings.Contains(string(pool), `php_admin_value[memory_limit] = "321M"`) {
		t.Errorf("pool after php ini set (%v):\n%s", err, pool)
	}
	if val, ok, _ := f.e.PHPIniGet(ctx, "app.test", "memory_limit"); !ok || val != "321M" {
		t.Errorf("php ini get: %q %v", val, ok)
	}
	if res, err := f.e.PHPIniSet(ctx, "app.test", "memory_limit", "321M"); err != nil || res.Changed {
		t.Errorf("the same value again: %+v %v", res, err)
	}
	if res, err := f.e.PHPIniUnset(ctx, "app.test", "memory_limit"); err != nil || !res.Changed || res.Site.PHPIni != nil {
		t.Errorf("php ini unset: %+v %v", res, err)
	}

	if _, err := f.e.PHPGlobalSet(ctx, "7.4", "max_input_vars", "1"); code(err) != hteerr.PHPVersionNotInstalled {
		t.Errorf("an absent version: %v", err)
	}
	for _, kv := range [][2]string{{"max_input_vars", "1234"}, {"display_errors", "On"}, {"max_input_vars", "4321"}} {
		if _, err := f.e.PHPGlobalSet(ctx, "8.3", kv[0], kv[1]); err != nil {
			t.Fatal(err)
		}
	}
	data, _ := os.ReadFile(v.paths.PHPOverlay("8.3"))
	if string(data) != "max_input_vars = \"4321\"\ndisplay_errors = On\n" {
		t.Errorf("overlay:\n%s", data)
	}
	if val, ok, _ := f.e.PHPGlobalGet(ctx, "8.3", "max_input_vars"); !ok || val != "4321" {
		t.Errorf("php global get: %q %v", val, ok)
	}
	if _, err := f.e.PHPGlobalUnset(ctx, "8.3", "max_input_vars"); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(v.paths.PHPOverlay("8.3")); string(data) != "display_errors = On\n" {
		t.Errorf("overlay after unset:\n%s", data)
	}
}

// newLegacyFixture is the Debian fixture plus the hte-cli V2 tree.
func newLegacyFixture(t *testing.T) *fixture {
	t.Helper()
	f := newFixture(t)
	dir := t.TempDir()
	for _, src := range []string{filepath.Join(sysroots, "debian"), "../../testdata/hte/legacy/V2"} {
		if err := os.CopyFS(dir, os.DirFS(src)); err != nil && !errors.Is(err, fs.ErrExist) {
			t.Fatal(err)
		}
	}
	for _, s := range []string{"http", "https", "forced"} {
		if err := os.MkdirAll(filepath.Join(dir, "home/dev/sites", s, "public"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	sr, err := platform.NewSysroot(dir)
	if err != nil {
		t.Fatal(err)
	}
	f.e.Sysroot = sr
	return f
}

// migrate --apply decides its root steps from the plan: copies first, the
// stop and its conditional rollback, the archives, then the FPM units; a
// blocking vhost leaves only the copies; the stop and archives need --yes.
func TestMigrateRootSteps(t *testing.T) {
	ctx := context.Background()
	f := newLegacyFixture(t)
	v := f.install(t)
	set, err := f.e.installed(v)
	if err != nil {
		t.Fatal(err)
	}
	// https.test uses PHP 7.4, which the fixture lacks: it blocks the stop.
	w, err := f.e.planMigration(ctx, v, set, MigrateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !w.blocking {
		t.Fatalf("not blocking: %+v", w.plan.Blocking)
	}
	if got := w.lines(); len(got) != 1 || !strings.Contains(got[0], "forced.test.key") {
		t.Errorf("blocked root steps: %v", got)
	}

	https := "/etc/apache2/sites-available/002-https.test.conf"
	w, err = f.e.planMigration(ctx, v, set, MigrateOptions{Exclude: []string{https}})
	if err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for _, o := range w.ops(nil) {
		kinds = append(kinds, o.Op+" "+o.Path+o.Name)
	}
	want := []string{
		"copy_key ",
		"archive_vhost /etc/apache2/sites-available/001-http.test.conf",
		"archive_vhost /etc/apache2/sites-available/003-forced.test.conf",
		"archive_pool /etc/php/8.3/fpm/pool.d/http.test.conf",
		"archive_pool /etc/php/8.3/fpm/pool.d/forced.test.conf",
		"unit_stop php8.3-fpm",
	}
	if !slices.Equal(kinds, want) {
		t.Errorf("root steps:\n%s\nwant\n%s", strings.Join(kinds, "\n"), strings.Join(want, "\n"))
	}
	_, err = f.e.Migrate(ctx, MigrateOptions{Apply: true, Exclude: []string{https}})
	if code(err) != hteerr.ConfirmationRequired {
		t.Errorf("apply without --yes: %v", err)
	}
	if sites, _, _ := v.store.LoadSites(); len(sites) != 0 {
		t.Errorf("a refused apply created %d sites", len(sites))
	}
	f.e.EUID = 0
	if _, err := f.e.Migrate(ctx, MigrateOptions{Apply: true}); code(err) != hteerr.RootNotAllowed {
		t.Errorf("apply as euid 0: %v", err)
	}
	if _, err := f.e.Migrate(ctx, MigrateOptions{}); err != nil {
		t.Errorf("dry run as euid 0: %v", err)
	}
}
