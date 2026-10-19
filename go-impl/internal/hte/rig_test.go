// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package hte

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/mauriziofonte/gash/internal/hostshelper"
	"github.com/mauriziofonte/gash/internal/hte/child"
	"github.com/mauriziofonte/gash/internal/hte/hteerr"
	"github.com/mauriziofonte/gash/internal/hte/legacy"
	"github.com/mauriziofonte/gash/internal/hte/receipt"
	"github.com/mauriziofonte/gash/internal/hte/render"
	"github.com/mauriziofonte/gash/internal/hte/rootops"
	"github.com/mauriziofonte/gash/internal/hte/service"
	"github.com/mauriziofonte/gash/internal/hte/service/servicetest"
	"github.com/mauriziofonte/gash/internal/hte/state"
	"github.com/mauriziofonte/gash/internal/platform"
	"github.com/mauriziofonte/gash/internal/runner"
)

// rig is an engine on a writable copy of the Debian sysroot and the
// hte-cli V2 tree, with every process played: the masters by
// servicetest.Stack, the root session by rootops.Serve in-process (its
// tools by rootAnswer, against the same tree), the hosts helper by
// hostshelper.Run on the tree's /etc/hosts, systemctl --user recorded,
// openssl for real (certificates are real files).
type rig struct {
	*fixture
	t     *testing.T
	root  string // the sysroot directory
	stack *servicetest.Stack
	mu    sync.Mutex
	// active and enabled are the system units.
	active, enabled map[string]bool
	// failRoot is a root command line prefix that fails.
	failRoot string
	// probeFail names a domain whose probe gets a 503.
	probeFail string
	// systemd: the machine runs systemd (/run/systemd/system); the user
	// manager runs the unit's hidden commands in-process.
	systemd   bool
	rootLines []string
	userLines []string // the commands of the user side, masters excepted
	// unitActive is the user unit's state, inUnit true while a job of it
	// runs, direct the master starts and stops outside one.
	unitActive, inUnit bool
	direct             []string
	// mac: the tree is the Homebrew macOS one; pfLoaded is the anchor's
	// state (pfctl -E loads it, -F empties it).
	mac, pfLoaded bool
}

// rigProcs records the stops of masters outside a unit job.
type rigProcs struct {
	*servicetest.Stack
	m *rig
}

func (p rigProcs) Signal(pid int, sig syscall.Signal) error {
	if sig == syscall.SIGTERM || sig == syscall.SIGKILL {
		if cmd, err := p.Cmdline(pid); err == nil {
			what := "stop apache"
			if strings.HasPrefix(cmd, "php-fpm") {
				what = "stop php-fpm"
			}
			p.m.mu.Lock()
			if !p.m.inUnit {
				p.m.direct = append(p.m.direct, what)
			}
			p.m.mu.Unlock()
		}
	}
	return p.Stack.Signal(pid, sig)
}

// userRunner is the user side's runner: openssl runs for real, the rest
// is answered by the rig.
type userRunner struct {
	*runner.Fake
	m *rig
}

func (u userRunner) Run(ctx context.Context, c runner.Cmd) (runner.Result, error) {
	if filepath.Base(c.Path) == "openssl" {
		bin := child.Find("openssl", func(p string) bool { _, err := os.Stat(p); return err == nil })
		if bin == "" {
			u.m.t.Skip("no openssl")
		}
		c.Path = bin
		return runner.Exec{}.Run(ctx, c)
	}
	return u.Fake.Run(ctx, c)
}

func newRig(t *testing.T) *rig {
	t.Helper()
	return newRigOn(t, false)
}

// newMacRig is the rig on the Homebrew macOS tree: pf.conf as macOS ships
// it, launchd, no system Apache, no hte-cli tree.
func newMacRig(t *testing.T) *rig {
	t.Helper()
	return newRigOn(t, true)
}

// stockPFConf is the /etc/pf.conf macOS ships.
const stockPFConf = "scrub-anchor \"com.apple/*\"\nnat-anchor \"com.apple/*\"\nrdr-anchor \"com.apple/*\"\n" +
	"dummynet-anchor \"com.apple/*\"\nanchor \"com.apple/*\"\nload anchor \"com.apple\" from \"/etc/pf.anchors/com.apple\"\n"

func newRigOn(t *testing.T, mac bool) *rig {
	t.Helper()
	f := newFixture(t)
	// A short runtime directory: socket paths have at most 103 bytes.
	rt, err := os.MkdirTemp("", "rt")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(rt) })
	f.env["XDG_RUNTIME_DIR"] = rt
	m := &rig{fixture: f, t: t, root: t.TempDir(), stack: servicetest.New(t), mac: mac,
		active: map[string]bool{"apache2": true, "php8.3-fpm": true}, enabled: map[string]bool{"apache2": true, "php8.3-fpm": true}}
	if mac {
		m.macTree()
	} else {
		m.linuxTree()
	}
	m.write("/etc/sudoers", "root ALL=(ALL:ALL) ALL\n@includedir /etc/sudoers.d\n", 0o440)
	return m.finish(f)
}

// macTree is the macOS part of the tree.
func (m *rig) macTree() {
	m.t.Helper()
	m.active, m.enabled = map[string]bool{}, map[string]bool{}
	if err := os.CopyFS(m.root, os.DirFS(filepath.Join(sysroots, "macos"))); err != nil {
		m.t.Fatal(err)
	}
	m.write("/etc/hosts", "127.0.0.1\tlocalhost\n::1\tlocalhost\n", 0o644)
	m.write(rootops.PFConf, stockPFConf, 0o644)
	m.write("/etc/pf.anchors/com.apple", "anchor \"250.ApplicationFirewall/*\"\n", 0o644)
	m.write("/Library/LaunchDaemons/com.example.other.plist", "<plist/>\n", 0o644)
	if err := os.MkdirAll(filepath.Join(m.root, "var/lib"), 0o755); err != nil {
		m.t.Fatal(err)
	}
	for _, tool := range []string{"/usr/bin/sudo", "/usr/sbin/visudo", "/usr/bin/openssl", "/sbin/pfctl", "/usr/bin/plutil", "/bin/launchctl"} {
		m.write(tool, "", 0o755)
	}
}

// linuxTree is the Debian tree with the hte-cli V2 sites, the system
// units and the tools.
func (m *rig) linuxTree() {
	t := m.t
	t.Helper()
	for _, src := range []string{filepath.Join(sysroots, "debian"), "../../testdata/hte/legacy/V2"} {
		if err := os.CopyFS(m.root, os.DirFS(src)); err != nil && !errors.Is(err, fs.ErrExist) {
			t.Fatal(err)
		}
	}
	for _, s := range []string{"http", "https", "forced"} {
		m.write("/home/dev/sites/"+s+"/public/index.php", "<?php echo 1;\n", 0o644)
	}
	m.write("/proc/sys/net/ipv4/ip_unprivileged_port_start", "1024\n", 0o644)
	m.write("/etc/init.d/apache2", "#!/bin/sh\n", 0o755)
	if err := os.MkdirAll(filepath.Join(m.root, "var/lib"), 0o755); err != nil {
		t.Fatal(err)
	}
	m.write("/etc/apache2/ports.conf", "Listen 80\n<IfModule ssl_module>\n\tListen 443\n</IfModule>\n", 0o644)
	for _, tool := range []string{"/usr/bin/sudo", "/usr/sbin/visudo", "/usr/sbin/sysctl", "/usr/sbin/service", "/usr/sbin/update-rc.d",
		"/usr/sbin/apachectl", "/usr/bin/openssl", "/usr/bin/loginctl", "/usr/bin/systemctl"} {
		m.write(tool, "", 0o755)
	}
	for _, rc := range []string{"rc2.d", "rc3.d"} {
		for _, u := range []string{"apache2", "php8.3-fpm"} {
			if err := os.MkdirAll(filepath.Join(m.root, "etc", rc), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink("../init.d/"+u, filepath.Join(m.root, "etc", rc, "S01"+u)); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// finish adds the hosts helper and the engine's fakes.
func (m *rig) finish(f *fixture) *rig {
	t := m.t
	t.Helper()
	// The hosts helper: the user side hashes the real file, root copies
	// it from the tree.
	helper := filepath.Join(t.TempDir(), "gash-hosts-helper")
	if err := os.WriteFile(helper, []byte("#!helper\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	m.write(helper, "#!helper\n", 0o755)
	sr, err := platform.NewSysroot(m.root)
	if err != nil {
		t.Fatal(err)
	}
	e := f.e
	e.Sysroot, e.Helper, e.Self = sr, helper, "/usr/local/bin/gash"
	if m.mac {
		e.GOOS = "darwin"
	}
	e.Runner = userRunner{Fake: runner.NewFake(t).Respond(m.userAnswer), m: m}
	e.Starter = child.Fake(t, func(runner.Call) runner.Expect {
		return runner.Expect{Serve: func(in io.Reader, out io.Writer) error {
			return rootops.Serve(context.Background(), in, out, m.rootEnv())
		}}
	})
	e.Procs = rigProcs{m.stack, m}
	e.Dial = func(context.Context, string, string) error { return nil }
	e.Sleep = func(time.Duration) {}
	e.StartTimeout = time.Second
	e.Probe = func(_ context.Context, ep render.Endpoint) service.SiteStatus {
		st := service.SiteStatus{Domain: ep.Domain, HTTP: []service.Probe{}, HTTPS: []service.Probe{}}
		code := 200
		if ep.Domain == m.probeFail {
			code = 503
		}
		for _, a := range ep.HTTP {
			st.HTTP = append(st.HTTP, service.Probe{URL: "http://" + a + "/", Status: code})
		}
		for _, a := range ep.HTTPS {
			st.HTTPS = append(st.HTTPS, service.Probe{URL: "https://" + a + "/", Status: code})
		}
		return st
	}
	return m
}

// write creates a file of the tree.
func (m *rig) write(sys, data string, mode os.FileMode) {
	m.t.Helper()
	p := filepath.Join(m.root, sys)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		m.t.Fatal(err)
	}
	os.Remove(p)
	if err := os.WriteFile(p, []byte(data), mode); err != nil {
		m.t.Fatal(err)
	}
}

func (m *rig) read(sys string) string {
	data, _ := os.ReadFile(filepath.Join(m.root, sys))
	return string(data)
}

func (m *rig) exists(sys string) bool {
	_, err := os.Lstat(filepath.Join(m.root, sys))
	return err == nil
}

// lines returns the recorded root and user commands.
func (m *rig) lines() (root, user []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.rootLines), slices.Clone(m.userLines)
}

func (m *rig) rootEnv() rootops.Env {
	o := platform.OS{GOOS: "linux", ID: "ubuntu", VersionID: "24.04", Family: platform.FamilyDebian}
	prefix := ""
	if m.mac {
		o, prefix = platform.OS{GOOS: "darwin", ID: "macos", VersionID: "15.1", Family: platform.FamilyDarwin}, "/opt/homebrew"
	}
	l, err := legacy.LayoutFor(o.Family, prefix)
	if err != nil {
		m.t.Fatal(err)
	}
	// The invoking user: the test's own ids, never root's (a root test run
	// plays a user 4242).
	uid, gid := os.Getuid(), os.Getgid()
	if uid == 0 {
		uid, gid = 4242, 4242
	}
	return rootops.Env{
		Sysroot: m.e.Sysroot, Runner: runner.NewFake(m.t).Respond(m.rootAnswer), Euid: 0,
		Getenv: func(k string) string {
			return map[string]string{"SUDO_USER": m.e.User, "SUDO_UID": strconv.Itoa(uid), "SUDO_GID": strconv.Itoa(gid)}[k]
		},
		Now:     m.e.now,
		OS:      o,
		Layout:  l,
		Systemd: m.systemd,
		Groups:  func(string) ([]uint32, error) { return []uint32{uint32(gid)}, nil },
		Home:    func(string) (string, error) { return m.home, nil },
	}
}

// userAnswer plays the user side's commands: discovery and the masters
// as the fixture does, the hosts helper behind sudo -n, systemctl --user.
func (m *rig) userAnswer(c runner.Call) (runner.Expect, bool) {
	if e, ok := m.stack.Answer(c); ok {
		if start := !slices.Contains(c.Argv, "-t"); start {
			what := "start " + filepath.Base(c.Argv[0])
			if len(c.Argv) > 2 && c.Argv[1] == "-f" {
				// Apache: the addresses of the generation it starts.
				if man, err := render.ReadManifest(filepath.Dir(c.Argv[2])); err == nil {
					what += " " + strings.Join(man.Apache.Listen, " ")
				}
			}
			m.mu.Lock()
			if !m.inUnit {
				m.direct = append(m.direct, what)
			}
			m.mu.Unlock()
		}
		return e, true
	}
	bin, args := filepath.Base(c.Argv[0]), c.Argv[1:]
	if bin == "php-fpm" || strings.HasPrefix(bin, "php-fpm") || bin == "apache2" || bin == "httpd" {
		e, _ := m.fixtureAnswer(c)
		return e, true
	}
	m.mu.Lock()
	m.userLines = append(m.userLines, bin+" "+strings.Join(args, " "))
	m.mu.Unlock()
	if bin == "systemctl" && m.systemd && len(args) == 3 && args[0] == "--user" && args[2] == "gash-hte.service" {
		return m.unitJob(args[1]), true
	}
	if bin == "sudo" && len(args) == 3 && args[0] == "-n" && args[2] == "apply" {
		var stderr bytes.Buffer
		rc := hostshelper.Run([]string{"apply"}, bytes.NewReader(c.Stdin), io.Discard, &stderr,
			hostshelper.Config{HostsPath: filepath.Join(m.root, "etc/hosts"), GOOS: "linux"})
		return runner.Expect{Status: rc, Stderr: stderr.String()}, true
	}
	return runner.Expect{}, true
}

// unitJob plays the user manager for gash-hte.service: start runs
// ExecStart (gash hte __unit start) unless the unit is active, stop runs
// ExecStop when it is, restart both, as Type=oneshot with RemainAfterExit
// does.
func (m *rig) unitJob(verb string) runner.Expect {
	run := func(start bool) error {
		m.mu.Lock()
		m.inUnit = true
		m.mu.Unlock()
		defer func() {
			m.mu.Lock()
			m.inUnit = false
			m.mu.Unlock()
		}()
		var err error
		if start {
			_, err = m.e.UnitStart(context.Background())
		} else {
			_, err = m.e.UnitStop(context.Background())
		}
		return err
	}
	var err error
	switch verb {
	case "start":
		if !m.unitActive {
			err = run(true)
			m.unitActive = err == nil
		}
	case "stop":
		if m.unitActive {
			err = run(false)
			m.unitActive = false
		}
	case "restart":
		if m.unitActive {
			err = run(false)
		}
		if err == nil {
			err = run(true)
		}
		m.unitActive = err == nil
	}
	if err != nil {
		return runner.Expect{Status: 1, Stderr: "Job for gash-hte.service failed: " + err.Error()}
	}
	return runner.Expect{}
}

// fixtureAnswer is the discovery answer of the plain fixture.
func (m *rig) fixtureAnswer(c runner.Call) (runner.Expect, bool) {
	bin, args := filepath.Base(c.Argv[0]), c.Argv[1:]
	switch {
	case bin == "php-fpm" && slices.Equal(args, []string{"-v"}):
		// Homebrew: php@X.Y, and php for the newest.
		v := map[string]string{"/opt/homebrew/opt/php@8.3/sbin/php-fpm": "8.3.14", "/opt/homebrew/opt/php/sbin/php-fpm": "8.4.1"}[c.Argv[0]]
		return runner.Expect{Stdout: "PHP " + v + " (fpm-fcgi) (built: Oct  1 2026 10:00:00)\n"}, true
	case strings.HasPrefix(bin, "php-fpm") && slices.Equal(args, []string{"-v"}):
		v := strings.TrimPrefix(bin, "php-fpm")
		return runner.Expect{Stdout: "PHP " + v + ".6 (fpm-fcgi) (built: Oct  1 2026 10:00:00)\n"}, true
	case strings.HasPrefix(bin, "php-fpm") && slices.Equal(args, []string{"-i"}):
		v := strings.TrimPrefix(bin, "php-fpm")
		return runner.Expect{Stdout: "Scan this dir for additional .ini files => /etc/php/" + v + "/fpm/conf.d\n"}, true
	case (bin == "apache2" || bin == "httpd") && slices.Equal(args, []string{"-l"}):
		return runner.Expect{Stdout: "Compiled in modules:\n  core.c\n  mod_so.c\n  mod_watchdog.c\n  http_core.c\n  mod_log_config.c\n  mod_logio.c\n  mod_version.c\n  mod_unixd.c\n"}, true
	}
	return runner.Expect{}, true
}

// rootAnswer plays the tools of the root step against the tree.
func (m *rig) rootAnswer(c runner.Call) (runner.Expect, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	bin, args := filepath.Base(c.Argv[0]), c.Argv[1:]
	line := bin + " " + strings.Join(args, " ")
	m.rootLines = append(m.rootLines, line)
	if m.failRoot != "" && strings.HasPrefix(line, m.failRoot) {
		return runner.Expect{Status: 1, Stderr: "simulated failure"}, true
	}
	status := func(ok bool) runner.Expect {
		if ok {
			return runner.Expect{Stdout: "enabled\n"}
		}
		return runner.Expect{Status: 3, Stdout: "disabled\n"}
	}
	unit := func(s string) string { return strings.TrimSuffix(s, ".service") }
	switch bin {
	case "sysctl":
		_, v, _ := strings.Cut(args[len(args)-1], "=")
		os.WriteFile(filepath.Join(m.root, "proc/sys/net/ipv4/ip_unprivileged_port_start"), []byte(v+"\n"), 0o644)
	case "service":
		switch args[1] {
		case "status":
			return status(m.active[args[0]]), true
		case "stop":
			m.active[args[0]] = false
		case "start":
			m.active[args[0]] = true
		}
	case "update-rc.d":
		m.enabled[args[0]] = args[1] == "enable"
		from, to := "S", "K"
		if args[1] == "enable" {
			from, to = "K", "S"
		}
		for _, rc := range []string{"rc2.d", "rc3.d"} {
			dir := filepath.Join(m.root, "etc", rc)
			os.Rename(filepath.Join(dir, from+"01"+args[0]), filepath.Join(dir, to+"01"+args[0]))
		}
	case "systemctl":
		switch args[0] {
		case "is-enabled":
			return status(m.enabled[unit(args[1])]), true
		case "is-active":
			return status(m.active[unit(args[1])]), true
		case "disable":
			m.enabled[unit(args[2])], m.active[unit(args[2])] = false, false
		case "enable":
			m.enabled[unit(args[1])] = true
		case "start":
			m.active[unit(args[1])] = true
		}
	case "openssl":
		os.WriteFile(args[2], c.Stdin, 0o600)
	case "pfctl":
		switch args[0] {
		case "-E":
			m.pfLoaded = true
		case "-a":
			m.pfLoaded = m.pfLoaded && args[2] != "-F"
		}
	}
	return runner.Expect{}, true
}

// TestLedger_HTE70: with the high ports, migrate --apply keeps the system
// Apache running. The plan says which ports stay with it; the migrated
// vhosts are archived, the system configuration tested and the system
// Apache reloaded gracefully, never stopped. A failed test puts the
// vhosts back, reloads nothing, archives no pool, and reports
// config_test_failed; the imported sites stay on the stack.
func TestLedger_HTE70(t *testing.T) {
	ctx := context.Background()
	sa := "/etc/apache2/sites-available/"
	for _, fail := range []bool{false, true} {
		m := newRig(t)
		m.install(t)
		if fail {
			m.failRoot = "apachectl configtest"
		}
		res, err := m.e.Migrate(ctx, MigrateOptions{Apply: true, Yes: true})
		root, _ := m.lines()
		if slices.ContainsFunc(root, func(l string) bool {
			return strings.HasPrefix(l, "service apache2 stop") || strings.HasPrefix(l, "update-rc.d apache2")
		}) || !m.active["apache2"] || !m.enabled["apache2"] {
			t.Errorf("fail=%v: the system Apache was stopped: %v", fail, root)
		}
		if sa := res.Plan.SystemApache; sa == nil || !slices.Equal(sa.Ports, []int{80, 443}) ||
			!slices.Contains(sa.Serves, "/etc/apache2/sites-available/002-https.test.conf") || len(res.Plan.Blocking) != 0 {
			t.Fatalf("fail=%v: plan %+v blocking %v", fail, res.Plan.SystemApache, res.Plan.Blocking)
		}
		if !slices.ContainsFunc(res.RootPlan, func(l string) bool { return strings.Contains(l, "apachectl configtest") }) {
			t.Errorf("fail=%v: root plan %v", fail, res.RootPlan)
		}
		if !slices.Contains(root, "apachectl configtest") {
			t.Errorf("fail=%v: no configtest: %v", fail, root)
		}
		if m.stack.Running() == 0 || len(res.Imported) != 2 {
			t.Errorf("fail=%v: imported %v, %d masters", fail, res.Imported, m.stack.Running())
		}
		enabled := m.exists("/etc/apache2/sites-enabled/001-http.test.conf") && m.exists(sa+"001-http.test.conf")
		reloaded := slices.Contains(root, "service apache2 reload")
		if !fail {
			if err != nil || !res.Switched || !reloaded || enabled || len(res.Archived) != 4 {
				t.Errorf("pass: %v switched %v reloaded %v enabled %v archived %v", err, res.Switched, reloaded, enabled, res.Archived)
			}
			continue
		}
		if code(err) != hteerr.ConfigTestFailed || reloaded || !enabled || len(res.Archived) != 0 ||
			!m.exists(sa+"003-forced.test.conf") || !m.exists("/etc/php/8.3/fpm/pool.d/http.test.conf") {
			t.Errorf("fail: %v reloaded %v enabled %v archived %v", err, reloaded, enabled, res.Archived)
		}
		if slices.ContainsFunc(res.Changes, func(c receipt.Change) bool { return c.Kind == receipt.LegacyVhost }) {
			t.Errorf("fail: the put-back vhosts are still reported as changes: %+v", res.Changes)
		}
	}
}

// After a high-ports migrate, uninstall puts the vhosts back and reloads
// the system Apache that kept running; the tree is as before.
func TestUninstallReloadsKeptApache(t *testing.T) {
	ctx := context.Background()
	m := newRig(t)
	m.install(t)
	before := m.snapshot()
	if _, err := m.e.Migrate(ctx, MigrateOptions{Apply: true, Yes: true}); err != nil {
		t.Fatal(err)
	}
	res, err := m.e.Uninstall(ctx, UninstallOptions{Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	root, _ := m.lines()
	if n := len(root); n < 3 || !slices.Equal(root[n-3:], []string{"apachectl configtest", "service apache2 status", "service apache2 reload"}) {
		t.Errorf("root lines %v", root)
	}
	if len(res.Warnings) != 0 || !m.active["apache2"] {
		t.Errorf("warnings %v, apache2 active %v", res.Warnings, m.active["apache2"])
	}
	after := m.snapshot()
	for p, v := range before {
		if after[p] != v {
			t.Errorf("%s: %q, before %q", p, after[p], v)
		}
	}
	for p := range after {
		if _, ok := before[p]; !ok && !strings.HasPrefix(p, "/etc/hosts") {
			t.Errorf("%s left behind", p)
		}
	}
}

// snapshot is every entry of /etc, /usr/local, /var/lib and /Library of
// the tree.
func (m *rig) snapshot() map[string]string {
	m.t.Helper()
	out := map[string]string{}
	for _, top := range []string{"etc", "usr/local", "var/lib", "Library"} {
		filepath.WalkDir(filepath.Join(m.root, top), func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			rel := strings.TrimPrefix(p, m.root)
			switch fi, _ := d.Info(); {
			case d.Type()&fs.ModeSymlink != 0:
				target, _ := os.Readlink(p)
				out[rel] = "link " + target
			case d.IsDir():
				out[rel] = "dir"
			default:
				data, _ := os.ReadFile(p)
				out[rel] = fi.Mode().String() + " " + string(data)
			}
			return nil
		})
	}
	return out
}

// TestLedger_HTE71: a generation includes copies of the overlays. An
// overlay edit whose reload fails at run time, which the config test does
// not see (in the global overlay a Listen on a port another process
// holds, in a site overlay a log file the user cannot open), leaves the
// previous generation running with its own copies; the user's overlay
// file is never written, and apache config path still names the global
// one.
func TestLedger_HTE71(t *testing.T) {
	ctx := context.Background()
	for _, global := range []bool{false, true} {
		m := newRig(t)
		v := m.install(t)
		docroot := m.docroot(t)
		if _, err := m.e.SiteCreate(ctx, state.Flags{Domain: "app.test", Docroot: &docroot, HTTPS: ptr(false)}); err != nil {
			t.Fatal(err)
		}
		if _, err := m.e.ServiceStart(ctx); err != nil {
			t.Fatal(err)
		}
		overlay, copyRel := v.paths.SiteOverlay("app.test"), "overlays/sites/app.test.apache.conf"
		edit := []byte("Header set X-Gen 2\nCustomLog \"/etc/app-access.log\" combined\n")
		m.stack.Unopenable = "/etc/app-access.log"
		if global {
			overlay, copyRel = v.paths.ApacheOverlay(), "overlays/apache.conf"
			edit = []byte("Header set X-Gen 2\nListen 127.0.0.1:3000\n")
			m.stack.Unopenable, m.stack.Busy = "", "127.0.0.1:3000"
		}
		must(t, os.MkdirAll(filepath.Dir(overlay), 0o700))
		must(t, os.WriteFile(overlay, []byte("Header set X-Gen 1\n"), 0o600))
		if res, err := m.e.ServiceReload(ctx); err != nil || !res.Applied.Changed {
			t.Fatalf("global=%v: first overlay: %+v %v", global, res, err)
		}
		layout := render.Layout{Dir: v.paths.RenderDir()}
		good, _, err := layout.Current()
		must(t, err)

		must(t, os.WriteFile(overlay, edit, 0o600))
		before, err := os.Stat(overlay)
		must(t, err)
		_, err = m.e.ServiceReload(ctx)
		if code(err) != hteerr.ReloadFailed || !strings.Contains(err.Error(), "rolled back to the previous generation, which is running") {
			t.Fatalf("global=%v: reload: %v", global, err)
		}
		cur, _, err := layout.Current()
		must(t, err)
		current := filepath.Join(v.paths.RenderDir(), "current")
		if cur != good || m.stack.Running() != 2 {
			t.Errorf("global=%v: current gen-%d (want gen-%d), %d masters", global, cur, good, m.stack.Running())
		}
		if data, _ := os.ReadFile(filepath.Join(current, copyRel)); string(data) != "Header set X-Gen 1\n" {
			t.Errorf("global=%v: the running generation's copy is %q", global, data)
		}
		failed, _ := os.ReadFile(filepath.Join(layout.GenDir(good+1), copyRel))
		if !bytes.Equal(failed, edit) {
			t.Errorf("global=%v: the failed generation's copy is %q", global, failed)
		}
		after, err := os.Stat(overlay)
		must(t, err)
		if data, _ := os.ReadFile(overlay); !bytes.Equal(data, edit) || !after.ModTime().Equal(before.ModTime()) || after.Mode() != before.Mode() {
			t.Errorf("global=%v: the overlay file changed: %q", global, data)
		}
		if p, err := m.e.ApacheConfigPath(ctx); err != nil || p != v.paths.ApacheOverlay() {
			t.Errorf("apache config path: %q %v", p, err)
		}
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// TestLedger_HTE73: with the systemd user unit, every start, stop and
// restart of the stack (install, migrate, uninstall, the service
// commands) is a job of systemctl --user, whose hidden __unit start and
// __unit stop run the masters; the masters are never started or stopped
// directly, but for migrate's staging generation (on its own ports, while
// the stack is down). A job a locked command waits for runs under that
// command's lock (the lease) instead of waiting for it.
func TestLedger_HTE73(t *testing.T) {
	ctx := context.Background()
	m := newRig(t)
	m.systemd = true
	must(t, os.MkdirAll(filepath.Join(m.root, "run/systemd/system"), 0o755))
	m.e.LockWait = time.Second // a job that waited for the lock would fail
	ins, err := m.e.Install(ctx, InstallOptions{Yes: true, Ports: "high", Autostart: "systemd"})
	if err != nil || !ins.Started || ins.Settings.Autostart != "systemd" {
		t.Fatalf("install: %+v %v", ins, err)
	}
	var staging string
	steps := []struct {
		name string
		run  func() error
		jobs []string
	}{
		{"install", func() error { return nil }, []string{"start"}},
		{"service stop", func() error { _, err := m.e.ServiceStop(ctx); return err }, []string{"stop"}},
		{"service start", func() error { _, err := m.e.ServiceStart(ctx); return err }, []string{"start"}},
		{"service restart", func() error { _, err := m.e.ServiceRestart(ctx); return err }, []string{"restart"}},
		{"service stop", func() error { _, err := m.e.ServiceStop(ctx); return err }, []string{"stop"}},
		{"migrate --apply", func() error {
			res, err := m.e.Migrate(ctx, MigrateOptions{Apply: true, Yes: true})
			if err == nil && (!res.Switched || len(res.Imported) != 2) {
				err = fmt.Errorf("migrate: %+v", res)
			}
			if res != nil {
				staging = res.Staging
			}
			return err
		}, []string{"start"}},
		{"uninstall", func() error { _, err := m.e.Uninstall(ctx, UninstallOptions{Yes: true}); return err }, []string{"stop", "disable"}},
	}
	seen := 0
	for _, st := range steps {
		if err := st.run(); err != nil {
			t.Fatalf("%s: %v", st.name, err)
		}
		_, user := m.lines()
		var jobs []string
		for _, l := range user[seen:] {
			if verb, ok := strings.CutPrefix(l, "systemctl --user "); ok && strings.HasSuffix(verb, " gash-hte.service") {
				jobs = append(jobs, strings.TrimSuffix(verb, " gash-hte.service"))
			}
		}
		seen = len(user)
		if st.name == "install" {
			jobs = slices.DeleteFunc(jobs, func(j string) bool { return j == "enable" })
		}
		if !slices.Equal(jobs, st.jobs) {
			t.Errorf("%s: systemctl --user jobs %v, want %v", st.name, jobs, st.jobs)
		}
		running := m.stack.Running() > 0
		if wantUp := st.name != "service stop" && st.name != "uninstall"; running != wantUp || m.unitActive != wantUp {
			t.Errorf("%s: masters running %v, unit active %v", st.name, running, m.unitActive)
		}
	}
	// The only masters gash started or stopped itself: the staging pair.
	host, ports, _ := strings.Cut(staging, ":")
	httpPort, httpsPort, _ := strings.Cut(ports, ",")
	want := []string{"start php-fpm8.3", "start apache2 " + host + ":" + httpPort + " " + host + ":" + httpsPort, "stop apache", "stop php-fpm"}
	m.mu.Lock()
	direct := slices.Clone(m.direct)
	m.mu.Unlock()
	if !slices.Equal(direct, want) {
		t.Errorf("masters started or stopped outside a unit job: %q, want the staging pair %q", direct, want)
	}
	if _, err := os.Stat(service.UnitPath(filepath.Join(m.home, ".config"))); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the unit file: %v", err)
	}
}

// TestLedger_HTE75: install on the standard ports next to a system Apache
// that serves enabled vhosts succeeds without starting the stack: started
// false, next "gash hte migrate", and a warning that names the vhosts and
// the command. The system Apache keeps running.
func TestLedger_HTE75(t *testing.T) {
	m := newRig(t)
	res, err := m.e.Install(context.Background(), InstallOptions{Yes: true, Ports: "standard", Autostart: "none"})
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	vhosts := []string{"/etc/apache2/sites-available/001-http.test.conf", "/etc/apache2/sites-available/002-https.test.conf",
		"/etc/apache2/sites-available/003-forced.test.conf"}
	if res.Started || res.Next != "gash hte migrate" || !slices.Equal(res.Blocking, vhosts) {
		t.Fatalf("started %v next %q blocking %v", res.Started, res.Next, res.Blocking)
	}
	warned := slices.ContainsFunc(res.Warnings, func(w string) bool {
		for _, v := range vhosts {
			if !strings.Contains(w, v) {
				return false
			}
		}
		return strings.Contains(w, "run gash hte migrate")
	})
	if !warned {
		t.Errorf("warnings %q", res.Warnings)
	}
	data, err := json.Marshal(res)
	must(t, err)
	if !bytes.Contains(data, []byte(`"started":false`)) || !bytes.Contains(data, []byte(`"next":"gash hte migrate"`)) {
		t.Errorf("JSON %s", data)
	}
	if m.stack.Running() != 0 || !m.active["apache2"] || slices.ContainsFunc(res.Plan, func(l string) bool { return strings.Contains(l, "stop") }) {
		t.Errorf("masters %d, system Apache active %v, plan %q", m.stack.Running(), m.active["apache2"], res.Plan)
	}
}
