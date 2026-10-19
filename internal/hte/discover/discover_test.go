// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package discover

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/mauriziofonte/gash/internal/hte/hteerr"
	"github.com/mauriziofonte/gash/internal/hte/render"
	"github.com/mauriziofonte/gash/internal/platform"
)

const sysroots = "../../../testdata/hte/sysroot"

func fixture(t *testing.T, name string) platform.Sysroot {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join(sysroots, name))
	if err != nil {
		t.Fatal(err)
	}
	sr, err := platform.NewSysroot(dir)
	if err != nil {
		t.Fatal(err)
	}
	return sr
}

// copyFixture copies a fixture without symlinks into a temporary root.
func copyFixture(t *testing.T, name string) (platform.Sysroot, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS(filepath.Join(sysroots, name))); err != nil {
		t.Fatal(err)
	}
	sr, err := platform.NewSysroot(dir)
	if err != nil {
		t.Fatal(err)
	}
	return sr, dir
}

const debianBuiltins = "Compiled in modules:\n  core.c\n  mod_so.c\n  mod_watchdog.c\n  http_core.c\n  mod_log_config.c\n  mod_logio.c\n  mod_version.c\n  mod_unixd.c\n"
const plainBuiltins = "Compiled in modules:\n  core.c\n  mod_so.c\n  http_core.c\n"

// fakeProbe answers from a table and counts the calls.
type fakeProbe struct {
	out   map[string]string
	calls map[string]int
}

func newProbe(out map[string]string) *fakeProbe {
	return &fakeProbe{out: out, calls: map[string]int{}}
}

func (p *fakeProbe) run(bin string, args ...string) (string, error) {
	key := strings.Join(append([]string{bin}, args...), " ")
	p.calls[key]++
	if out, ok := p.out[key]; ok {
		return out, nil
	}
	return "", errors.New("exit status 127")
}

func fpmInfo(scan string) string {
	return "phpinfo()\nPHP Version => 8.3.6\n\nConfiguration File (php.ini) Path => /etc/php\n" +
		"Scan this dir for additional .ini files => " + scan + "\nAdditional .ini files parsed => (none)\n"
}

func fpmV(full string) string {
	return "PHP " + full + " (fpm-fcgi) (built: Oct  1 2026 10:00:00)\nCopyright (c) The PHP Group\nZend Engine v4\n"
}

type platformCase struct {
	name     string
	goos     string
	family   platform.Family
	apache   Apache
	builtins string
	probe    map[string]string
	versions []PHP
	problems []string
}

var noWrite = map[string]bool{"/var/lib/php/session": true, "/var/opt/remi/php74/lib/php/session": true}

func writable(sys string) bool { return !noWrite[sys] }

func platformCases() []platformCase {
	return []platformCase{
		{
			name: "debian", goos: "linux", family: platform.FamilyDebian,
			apache:   Apache{Binary: "/usr/sbin/apache2", ModuleDir: "/usr/lib/apache2/modules", MimeTypes: "/etc/mime.types"},
			builtins: debianBuiltins,
			probe: map[string]string{
				"/usr/sbin/php-fpm8.3 -v": fpmV("8.3.6"),
				"/usr/sbin/php-fpm8.3 -i": fpmInfo("/etc/php/8.3/fpm/conf.d"),
				"/usr/sbin/php-fpm8.2 -v": "PHP 8.2.20 (cli) (built: Jun  1 2026)\n",
				"/usr/sbin/php-fpm7.4 -v": fpmV("7.4.33"),
			},
			versions: []PHP{{Version: "8.3", Full: "8.3.6", FPM: "/usr/sbin/php-fpm8.3", INI: "/etc/php/8.3/fpm/php.ini",
				Layout: LayoutDebian, SessionPath: "/var/lib/php/sessions", ScanDir: "/etc/php/8.3/fpm/conf.d"}},
			problems: []string{
				"/usr/sbin/php-fpm7.4 has no php.ini at /etc/php/7.4/fpm/php.ini",
				`/usr/sbin/php-fpm8.2 -v does not report a PHP-FPM version: "PHP 8.2.20 (cli) (built: Jun  1 2026)"`,
			},
		},
		{
			name: "fedora", goos: "linux", family: platform.FamilyFedora,
			apache:   Apache{Binary: "/usr/sbin/httpd", ModuleDir: "/usr/lib64/httpd/modules", MimeTypes: "/etc/mime.types"},
			builtins: plainBuiltins,
			probe: map[string]string{
				"/usr/sbin/php-fpm -v":                     fpmV("8.3.12"),
				"/usr/sbin/php-fpm -i":                     fpmInfo("/etc/php.d"),
				"/opt/remi/php74/root/usr/sbin/php-fpm -v": fpmV("7.4.33"),
				"/opt/remi/php83/root/usr/sbin/php-fpm -v": fpmV("8.3.13"),
			},
			versions: []PHP{
				{Version: "7.4", Full: "7.4.33", FPM: "/opt/remi/php74/root/usr/sbin/php-fpm", INI: "/etc/opt/remi/php74/php.ini",
					Layout: LayoutRemi, SessionPath: "/var/opt/remi/php74/lib/php/session", NeedsSessionDir: true,
					ScanDir: "/etc/opt/remi/php74/php.d"},
				{Version: "8.3", Full: "8.3.12", FPM: "/usr/sbin/php-fpm", INI: "/etc/php.ini",
					Layout: LayoutFedora, SessionPath: "/var/lib/php/session", NeedsSessionDir: true, ScanDir: "/etc/php.d"},
			},
			problems: []string{"PHP 8.3 is also at /opt/remi/php83/root/usr/sbin/php-fpm; using /usr/sbin/php-fpm"},
		},
		{
			name: "arch", goos: "linux", family: platform.FamilyArch,
			apache:   Apache{Binary: "/usr/bin/httpd", ModuleDir: "/usr/lib/httpd/modules", MimeTypes: "/etc/httpd/conf/mime.types"},
			builtins: plainBuiltins,
			probe: map[string]string{
				"/usr/bin/php-fpm -v":        fpmV("8.4.2"),
				"/usr/bin/php-fpm-legacy -v": fpmV("8.2.27"),
				"/usr/bin/php-fpm74 -v":      fpmV("7.4.33"),
			},
			versions: []PHP{
				{Version: "7.4", Full: "7.4.33", FPM: "/usr/bin/php-fpm74", INI: "/etc/php74/php.ini", Layout: LayoutAUR, ScanDir: "/etc/php74/conf.d"},
				{Version: "8.2", Full: "8.2.27", FPM: "/usr/bin/php-fpm-legacy", INI: "/etc/php-legacy/php.ini", Layout: LayoutArchLegacy,
					ScanDir: "/etc/php-legacy/conf.d"},
				{Version: "8.4", Full: "8.4.2", FPM: "/usr/bin/php-fpm", INI: "/etc/php/php.ini", Layout: LayoutArch, ScanDir: "/etc/php/conf.d"},
			},
		},
		{
			name: "macos", goos: "darwin", family: platform.FamilyDarwin,
			apache: Apache{Binary: "/opt/homebrew/bin/httpd", ModuleDir: "/opt/homebrew/lib/httpd/modules",
				MimeTypes: "/opt/homebrew/etc/httpd/mime.types", Prefix: "/opt/homebrew"},
			builtins: plainBuiltins,
			probe: map[string]string{
				"/opt/homebrew/opt/php/sbin/php-fpm -v":     fpmV("8.4.1"),
				"/opt/homebrew/opt/php@8.3/sbin/php-fpm -v": fpmV("8.3.14"),
			},
			versions: []PHP{
				{Version: "8.3", Full: "8.3.14", FPM: "/opt/homebrew/opt/php@8.3/sbin/php-fpm", INI: "/opt/homebrew/etc/php/8.3/php.ini", Layout: LayoutHomebrew,
					ScanDir: "/opt/homebrew/etc/php/8.3/conf.d"},
				{Version: "8.4", Full: "8.4.1", FPM: "/opt/homebrew/opt/php/sbin/php-fpm", INI: "/opt/homebrew/etc/php/8.4/php.ini", Layout: LayoutHomebrew,
					ScanDir: "/opt/homebrew/etc/php/8.4/conf.d"},
			},
		},
	}
}

// TestLedger_HTE58: per platform fake, the Apache binary, module
// directory and PHP layouts.
func TestLedger_HTE58(t *testing.T) {
	for _, tc := range platformCases() {
		t.Run(tc.name, func(t *testing.T) {
			sr := fixture(t, tc.name)
			o := platform.DetectOS(sr, tc.goos)
			if o.Family != tc.family {
				t.Fatalf("family %s, want %s", o.Family, tc.family)
			}
			a, err := FindApache(sr, o)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(tc.apache, a); diff != "" {
				t.Fatal(diff)
			}
			probe := newProbe(tc.probe)
			probe.out[a.Binary+" -l"] = tc.builtins
			builtin, err := BuiltinModules(a, probe.run)
			if err != nil {
				t.Fatal(err)
			}
			mods := ResolveModules(sr, a, builtin, nil)
			if len(mods.Missing) != 0 || len(mods.Warnings) != 0 {
				t.Fatalf("missing %v, warnings %v", mods.Missing, mods.Warnings)
			}
			if err := mods.MissingError(a, o.Family); err != nil {
				t.Fatal(err)
			}
			var loaded []string
			for _, m := range mods.Load {
				loaded = append(loaded, m.Name)
				if m.Path != a.ModuleDir+"/mod_"+m.Name+".so" {
					t.Errorf("module path %s", m.Path)
				}
			}
			want := slices.Clone(render.BaseModules)
			if tc.name == "debian" {
				want = slices.DeleteFunc(want, func(m string) bool { return m == "unixd" || m == "log_config" || m == "logio" })
				if diff := cmp.Diff([]string{"unixd", "log_config", "logio"}, mods.Builtin); diff != "" {
					t.Errorf("builtin: %s", diff)
				}
			}
			if diff := cmp.Diff(want, loaded); diff != "" {
				t.Errorf("loaded modules: %s", diff)
			}
			res := DiscoverPHP(PHPEnv{Sysroot: sr, OS: o, Probe: probe.run, Writable: writable})
			if diff := cmp.Diff(tc.versions, res.Versions); diff != "" {
				t.Errorf("versions (-want +got):\n%s", diff)
			}
			got := slices.Clone(res.Problems)
			slices.Sort(got)
			if diff := cmp.Diff(tc.problems, got); diff != "" {
				t.Errorf("problems (-want +got):\n%s", diff)
			}
		})
	}
}

// TestLedger_HTE22: a bare /etc/php/X.Y (here 8.4 with only cli/) is not a
// version; a binary answering (fpm-fcgi) is; the cache is keyed by size
// and mtime.
func TestLedger_HTE22(t *testing.T) {
	sr, dir := copyFixture(t, "debian")
	o := platform.DetectOS(sr, "linux")
	probe := newProbe(platformCases()[0].probe)
	cachePath := filepath.Join(t.TempDir(), "state", "php-versions.json")
	discover := func() PHPResult {
		c := LoadCache(cachePath)
		res := DiscoverPHP(PHPEnv{Sysroot: sr, OS: o, Probe: probe.run, Cache: c, Writable: writable})
		if err := c.Save(cachePath); err != nil {
			t.Fatal(err)
		}
		return res
	}
	res := discover()
	if diff := cmp.Diff([]string{"8.3"}, res.Installed()); diff != "" {
		t.Fatal(diff)
	}
	if _, ok := res.Find("8.4"); ok {
		t.Fatal("8.4 (cli only) listed")
	}
	if probe.calls["/usr/sbin/php-fpm8.4 -v"] != 0 {
		t.Fatal("probed a binary that does not exist")
	}
	if fi, err := os.Stat(cachePath); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("cache file: %v %v", fi, err)
	}

	discover()
	if n, i := probe.calls["/usr/sbin/php-fpm8.3 -v"], probe.calls["/usr/sbin/php-fpm8.3 -i"]; n != 1 || i != 1 {
		t.Fatalf("cached binary probed %d times (-i %d)", n, i)
	}
	if p, _ := discover().Find("8.3"); p.ScanDir != "/etc/php/8.3/fpm/conf.d" {
		t.Fatalf("cached scan dir %q", p.ScanDir)
	}
	// An upgrade changes the mtime: probed again, new version seen.
	probe.out["/usr/sbin/php-fpm8.3 -v"] = fpmV("8.3.7")
	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(filepath.Join(dir, "usr/sbin/php-fpm8.3"), future, future); err != nil {
		t.Fatal(err)
	}
	res = discover()
	if n := probe.calls["/usr/sbin/php-fpm8.3 -v"]; n != 2 {
		t.Fatalf("after an mtime change probed %d times", n)
	}
	if p, _ := res.Find("8.3"); p.Full != "8.3.7" {
		t.Fatalf("full version %s", p.Full)
	}

	// A corrupt or foreign cache is an empty one.
	for _, bad := range []string{"{", `{"schema":1,"entries":{}}`, `{"schema":2,"entries":{},"x":1}`} {
		if err := os.WriteFile(cachePath, []byte(bad), 0o600); err != nil {
			t.Fatal(err)
		}
		before := probe.calls["/usr/sbin/php-fpm8.3 -v"]
		if got := discover().Installed(); !slices.Equal(got, []string{"8.3"}) {
			t.Fatalf("with cache %q: %v", bad, got)
		}
		if probe.calls["/usr/sbin/php-fpm8.3 -v"] != before+1 {
			t.Fatalf("cache %q was trusted", bad)
		}
	}

	// A version mismatch under a versioned name does not count.
	probe.out["/usr/sbin/php-fpm8.3 -v"] = fpmV("8.2.1")
	res = DiscoverPHP(PHPEnv{Sysroot: sr, OS: o, Probe: probe.run, Writable: writable})
	if len(res.Versions) != 0 || !slices.Contains(res.Problems, "/usr/sbin/php-fpm8.3 reports PHP 8.2, not 8.3") {
		t.Fatalf("mismatch: %+v", res)
	}
	// A failing probe and a non-executable binary are problems, not versions.
	delete(probe.out, "/usr/sbin/php-fpm8.3 -v")
	if err := os.Chmod(filepath.Join(dir, "usr/sbin/php-fpm7.4"), 0o644); err != nil {
		t.Fatal(err)
	}
	res = DiscoverPHP(PHPEnv{Sysroot: sr, OS: o, Probe: probe.run, Writable: writable})
	if len(res.Versions) != 0 ||
		!slices.Contains(res.Problems, "/usr/sbin/php-fpm8.3 -v failed: exit status 127") ||
		!slices.Contains(res.Problems, "/usr/sbin/php-fpm7.4 is not an executable file") {
		t.Fatalf("failures: %+v", res.Problems)
	}
}

func TestFindApacheFailures(t *testing.T) {
	sr := fixture(t, "debian")
	_, err := FindApache(sr, platform.OS{ID: "alpine", VersionID: "3.20", Family: platform.FamilyUnknown})
	if hteerr.CodeOf(err) != hteerr.PlatformUnsupported {
		t.Fatalf("unknown platform: %v", err)
	}
	// The Debian tree read as Fedora has no httpd.
	_, err = FindApache(sr, platform.OS{Family: platform.FamilyFedora})
	if e, ok := hteerr.As(err); !ok || e.Code != hteerr.ApacheNotFound || e.Hint != "sudo dnf install httpd mod_ssl mod_http2" {
		t.Fatalf("missing httpd: %v", err)
	}
	// No Homebrew on a Linux tree read as macOS.
	if _, err := FindApache(sr, platform.OS{Family: platform.FamilyDarwin}); hteerr.CodeOf(err) != hteerr.ApacheNotFound {
		t.Fatalf("no Homebrew: %v", err)
	}
	for _, rm := range []string{"usr/sbin/apache2", "usr/lib/apache2/modules", "etc/mime.types"} {
		sr, dir := copyFixture(t, "debian")
		if err := os.RemoveAll(filepath.Join(dir, rm)); err != nil {
			t.Fatal(err)
		}
		if _, err := FindApache(sr, platform.OS{Family: platform.FamilyDebian}); hteerr.CodeOf(err) != hteerr.ApacheNotFound {
			t.Errorf("without %s: %v", rm, err)
		}
	}
	sr, dir := copyFixture(t, "debian")
	if err := os.Chmod(filepath.Join(dir, "usr/sbin/apache2"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := FindApache(sr, platform.OS{Family: platform.FamilyDebian}); hteerr.CodeOf(err) != hteerr.ApacheNotFound {
		t.Errorf("non-executable apache2: %v", err)
	}
}

func TestParseBuiltinModules(t *testing.T) {
	got, err := ParseBuiltinModules(debianBuiltins)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff([]string{"core", "so", "watchdog", "http_core", "log_config", "logio", "version", "unixd"}, got); diff != "" {
		t.Fatal(diff)
	}
	got, err = ParseBuiltinModules("Compiled in modules:\n  core.c\n  prefork.c\n  event.c\n  worker.c\n")
	if err != nil || !slices.Equal(got, []string{"core", "mpm_prefork", "mpm_event", "mpm_worker"}) {
		t.Fatalf("%v %v", got, err)
	}
	for _, bad := range []string{"", "core.c\n", "Compiled in modules:\n  mod_x.so\n", "Compiled in modules:\n  a b.c\n", "Compiled in modules:\n  .c\n"} {
		if _, err := ParseBuiltinModules(bad); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	a := Apache{Binary: "/usr/sbin/apache2"}
	if _, err := BuiltinModules(a, newProbe(nil).run); hteerr.CodeOf(err) != hteerr.ApacheNotFound {
		t.Fatalf("failing httpd -l: %v", err)
	}
}

func TestResolveModules(t *testing.T) {
	sr, dir := copyFixture(t, "fedora")
	a, err := FindApache(sr, platform.DetectOS(sr, "linux"))
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range []string{"ssl", "http2"} {
		if err := os.Remove(filepath.Join(dir, "usr/lib64/httpd/modules/mod_"+m+".so")); err != nil {
			t.Fatal(err)
		}
	}
	mods := ResolveModules(sr, a, []string{"core", "so", "http_core"}, []string{"status", "rewrite", "nosuch"})
	if diff := cmp.Diff([]string{"ssl", "http2", "nosuch"}, mods.Missing); diff != "" {
		t.Fatal(diff)
	}
	if last := mods.Load[len(mods.Load)-1]; last.Name != "status" {
		t.Fatalf("extra module not appended: %v", last)
	}
	if n := slices.IndexFunc(mods.Load, func(m render.Module) bool { return m.Name == "rewrite" }); n < 0 ||
		slices.IndexFunc(mods.Load[n+1:], func(m render.Module) bool { return m.Name == "rewrite" }) >= 0 {
		t.Fatal("rewrite loaded twice or not at all")
	}
	if e, ok := hteerr.As(mods.MissingError(a, platform.FamilyFedora)); !ok || e.Code != hteerr.ApacheModuleMissing ||
		e.Action() != hteerr.Stop || !strings.Contains(e.Details, "ssl, http2, nosuch") ||
		e.Hint != "sudo dnf install mod_ssl mod_http2 httpd" {
		t.Fatalf("missing error: %v", e)
	}
	if h := ModuleInstallHint(platform.FamilyDebian, []string{"ssl"}); h != "sudo apt install --reinstall apache2" {
		t.Fatalf("debian hint: %s", h)
	}
	// A statically built prefork MPM replaces mpm_event, with a warning.
	mods = ResolveModules(sr, a, []string{"core", "mpm_prefork"}, nil)
	if slices.ContainsFunc(mods.Load, func(m render.Module) bool { return strings.HasPrefix(m.Name, "mpm_") }) || len(mods.Warnings) != 1 {
		t.Fatalf("static prefork: %+v", mods)
	}

	files, err := ModuleFiles(fixture(t, "debian"), Apache{ModuleDir: "/usr/lib/apache2/modules"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(files, "status") || !slices.Contains(files, "cgi") || slices.Contains(files, "log_config") || !slices.IsSorted(files) {
		t.Fatalf("module files %v", files)
	}
}

func TestSessionPath(t *testing.T) {
	for _, tc := range []struct {
		ini  string
		want string
		set  bool
	}{
		{"", "", false},
		{";session.save_path = \"/tmp\"\n", "", false},
		{"session.save_path = \"/var/lib/php/sessions\"\n", "/var/lib/php/sessions", true},
		{"session.save_path = /srv/s ; comment\n", "/srv/s", true},
		{"session.save_path = \"2;0700;/srv/s\"\n", "/srv/s", true},
		{"session.save_path = \"/a\"\nsession.save_path = \"/b\"\n", "/b", true},
		{"session.save_path =\n", "", true},
		{"session.save_handler = files\n", "", false},
	} {
		got, set := iniSessionPath([]byte(tc.ini))
		if got != tc.want || set != tc.set {
			t.Errorf("%q: %q %v, want %q %v", tc.ini, got, set, tc.want, tc.set)
		}
	}
}

func TestParseScanDir(t *testing.T) {
	for _, tc := range []struct {
		info, dir string
		found     bool
	}{
		{fpmInfo("/etc/php/8.3/fpm/conf.d"), "/etc/php/8.3/fpm/conf.d", true},
		{fpmInfo("(none)"), "", true},
		{"PHP Version => 8.3.6\n", "", false},
	} {
		if dir, found := ParseScanDir(tc.info); dir != tc.dir || found != tc.found {
			t.Errorf("%q: %q %v", tc.info, dir, found)
		}
	}
}

func TestParseFPMVersion(t *testing.T) {
	for _, tc := range []struct{ out, short, full string }{
		{fpmV("8.3.6"), "8.3", "8.3.6"},
		{"PHP 8.4.0RC1 (fpm-fcgi) (built: x)\n", "8.4", "8.4.0"},
		{"PHP 7.4.33-1+ubuntu24.04.1+deb.sury.org+1 (fpm-fcgi) (built: x)", "7.4", "7.4.33"},
		{"PHP 8.3.6 (cli) (built: x)", "", ""},
		{"Zend Engine\nPHP 8.3.6 (fpm-fcgi)", "", ""},
		{"", "", ""},
	} {
		s, f, ok := ParseFPMVersion(tc.out)
		if s != tc.short || f != tc.full || ok != (tc.short != "") {
			t.Errorf("%q: %q %q %v", tc.out, s, f, ok)
		}
	}
	if PHPInstallHint(platform.FamilyDebian, "8.3") != "sudo apt install php8.3-fpm" || PHPInstallHint(platform.FamilyDarwin, "8.3") != "brew install php@8.3" {
		t.Fatal("install hints")
	}
}
