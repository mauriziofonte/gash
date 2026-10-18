// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package service

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/mauriziofonte/gash/internal/hte/hteerr"
	"github.com/mauriziofonte/gash/internal/hte/proc"
	"github.com/mauriziofonte/gash/internal/hte/render"
	"github.com/mauriziofonte/gash/internal/hte/state"
)

// fakeStack plays Apache and PHP-FPM: it is the runner of their commands
// and the process table, and it writes PID files and log lines the way
// the real masters do (verified in the Ubuntu 24.04 container).
type fakeStack struct {
	t      *testing.T
	mu     sync.Mutex
	procs  map[int]string // pid -> command line
	next   int
	events []string
	// failTest makes the config test of the named master fail.
	failTest string
	// failUSR1 makes Apache exit during a graceful restart (a busy port).
	failUSR1 bool
	// deaf masters ignore SIGTERM.
	deaf bool
}

func newStack(t *testing.T) *fakeStack {
	return &fakeStack{t: t, procs: map[int]string{}, next: 100}
}

func (f *fakeStack) event(format string, args ...any) {
	f.events = append(f.events, fmt.Sprintf(format, args...))
}

func (f *fakeStack) Events() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := slices.Clone(f.events)
	f.events = nil
	return out
}

func appendLog(t *testing.T, path, line string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	fh, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprintln(fh, line)
	fh.Close()
}

func (f *fakeStack) manifestOf(conf string) *render.Manifest {
	dir := filepath.Dir(conf)
	for !strings.HasSuffix(dir, "/current") && !strings.Contains(filepath.Base(dir), "gen-") {
		dir = filepath.Dir(dir)
	}
	m, err := render.ReadManifest(dir)
	if err != nil {
		f.t.Fatal(err)
	}
	return m
}

func (f *fakeStack) spawn(cmdline, pidFile string) int {
	f.next++
	f.procs[f.next] = cmdline
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(f.next)+"\n"), 0o600); err != nil {
		f.t.Fatal(err)
	}
	return f.next
}

func (f *fakeStack) Run(_ context.Context, c proc.Cmd) (proc.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	apache := strings.HasSuffix(c.Path, "apache2")
	switch {
	case c.Args[0] == "-t":
		name := "fpm"
		if apache {
			name = "apache"
		}
		f.event("test %s", name)
		if f.failTest == name {
			return proc.Result{ExitCode: 1, Stderr: []byte("AH00526: Syntax error on line 3")}, nil
		}
		if !apache && !slices.ContainsFunc(c.Env, func(e string) bool { return strings.HasPrefix(e, "PHP_INI_SCAN_DIR=") }) {
			f.t.Errorf("fpm -t without PHP_INI_SCAN_DIR: %v", c.Env)
		}
		return proc.Result{}, nil
	case apache:
		m := f.manifestOf(c.Args[1])
		f.event("start apache")
		f.spawn(c.Path+" "+strings.Join(c.Args, " "), m.Apache.PidFile)
		appendLog(f.t, m.Apache.ErrorLog, "AH00489: Apache/2.4.58 configured -- resuming normal operations")
		return proc.Result{}, nil
	default:
		m := f.manifestOf(c.Args[1])
		v := filepath.Base(filepath.Dir(c.Args[1]))
		fm, _ := m.Master(v)
		f.event("start fpm %s", v)
		if !slices.Contains(c.Env, "PHP_INI_SCAN_DIR="+fm.ScanDir) {
			f.t.Errorf("fpm start without PHP_INI_SCAN_DIR: %v", c.Env)
		}
		f.spawn("php-fpm: master process ("+c.Args[1]+")", fm.PidFile)
		appendLog(f.t, fm.ErrorLog, "NOTICE: ready to handle connections")
		return proc.Result{}, nil
	}
}

func (f *fakeStack) Cmdline(pid int) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.procs[pid]
	if !ok {
		return "", os.ErrNotExist
	}
	return c, nil
}

func (f *fakeStack) Started(int) (time.Time, error) { return time.Now().Add(-time.Minute), nil }

func (f *fakeStack) Signal(pid int, sig syscall.Signal) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cmd, ok := f.procs[pid]
	if !ok {
		return syscall.ESRCH
	}
	apache := !strings.HasPrefix(cmd, "php-fpm")
	name := "apache"
	conf := ""
	if apache {
		conf = strings.Fields(cmd)[2]
	} else {
		conf = strings.TrimSuffix(strings.TrimPrefix(cmd, "php-fpm: master process ("), ")")
		name = "fpm " + filepath.Base(filepath.Dir(conf))
	}
	m := f.manifestOf(conf)
	switch sig {
	case syscall.SIGUSR1:
		f.event("graceful apache")
		if f.failUSR1 {
			delete(f.procs, pid)
			appendLog(f.t, m.Apache.ErrorLog, "(98)Address already in use: AH00072: make_sock: could not bind to address 127.0.0.1:3000")
			appendLog(f.t, m.Apache.ErrorLog, "no listening sockets available, shutting down")
			return nil
		}
		appendLog(f.t, m.Apache.ErrorLog, "AH00489: resuming normal operations")
	case syscall.SIGUSR2:
		f.event("reload %s", name)
		fm, _ := m.Master(filepath.Base(filepath.Dir(conf)))
		delete(f.procs, pid)
		f.spawn(cmd, fm.PidFile) // FPM re-execs and daemonizes: a new PID
		appendLog(f.t, fm.ErrorLog, "NOTICE: ready to handle connections")
	case syscall.SIGTERM:
		f.event("stop %s", name)
		if f.deaf {
			return nil
		}
		delete(f.procs, pid)
	case syscall.SIGKILL:
		f.event("kill %s", name)
		delete(f.procs, pid)
	}
	return nil
}

func testPaths(t *testing.T) state.Paths {
	d := t.TempDir()
	return state.Paths{ConfigDir: d + "/c", StateDir: d + "/s", DataDir: d + "/d", RuntimeDir: d + "/r"}
}

func site(domain, php string) state.Site {
	return state.Site{Domain: domain, Aliases: []string{}, Docroot: "/srv/" + domain, PHP: php, Profile: "dev",
		HTTPS: false, HTTP2: true, Enabled: true, Kind: state.KindManaged, PHPIni: map[string]string{},
		ExtraHTTPPorts: []int{}, ExtraHTTPSPorts: []int{}}
}

func input(p state.Paths, sites ...state.Site) render.Input {
	php := func(v string) render.PHPVersion {
		return render.PHPVersion{FPM: "/usr/sbin/php-fpm" + v, INI: "/etc/php/" + v + "/fpm/php.ini", ScanDir: "/etc/php/" + v + "/fpm/conf.d"}
	}
	return render.Input{
		Paths:  p,
		Apache: render.Apache{Binary: "/usr/sbin/apache2", MimeTypes: "/etc/mime.types"},
		Ports:  state.HighPorts, Listen: []string{state.LoopbackIPv4},
		PHP:   map[string]render.PHPVersion{"8.3": php("8.3"), "8.4": php("8.4")},
		Sites: sites,
	}
}

func supervisor(t *testing.T, f *fakeStack, p state.Paths) *Supervisor {
	if err := os.MkdirAll(p.RuntimeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	return &Supervisor{Runner: f, Procs: f, Layout: render.Layout{Dir: p.RenderDir()},
		Dial: func(context.Context, string, string) error { return nil }, Sleep: func(time.Duration) {},
		StartTimeout: 2 * time.Second, StopTimeout: 200 * time.Millisecond}
}

func TestApplyWhileStopped(t *testing.T) {
	f, p := newStack(t), testPaths(t)
	s := supervisor(t, f, p)
	ctx := context.Background()
	in := input(p, site("a.test", "8.3"))
	out, err := s.Apply(ctx, in)
	if err != nil || out.Generation != 1 || !out.Changed || out.Reloaded {
		t.Fatalf("%+v %v", out, err)
	}
	if diff := cmp.Diff([]string{"test apache", "test fpm"}, f.Events()); diff != "" {
		t.Fatal(diff)
	}
	// The same state renders the same files: nothing happens.
	if out, err := s.Apply(ctx, in); err != nil || out.Changed || out.Generation != 1 {
		t.Fatalf("unchanged: %+v %v", out, err)
	}
	if ev := f.Events(); len(ev) != 0 {
		t.Fatalf("unchanged ran %v", ev)
	}
}

func TestStartStatusStop(t *testing.T) {
	f, p := newStack(t), testPaths(t)
	s := supervisor(t, f, p)
	ctx := context.Background()
	if err := s.Start(ctx); hteerr.CodeOf(err) != hteerr.InvalidState {
		t.Fatalf("start before render: %v", err)
	}
	if _, err := s.Apply(ctx, input(p, site("a.test", "8.3"), site("b.test", "8.4"))); err != nil {
		t.Fatal(err)
	}
	f.Events()
	if err := s.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff([]string{"start fpm 8.3", "start fpm 8.4", "start apache"}, f.Events()); diff != "" {
		t.Fatalf("FPM before Apache: %s", diff)
	}
	if err := s.Start(ctx); err != nil || len(f.Events()) != 0 {
		t.Fatal("second start started something")
	}
	st, err := s.Status(ctx, false)
	if err != nil || !st.Running || len(st.FPM) != 2 || !st.Apache.Running || st.Apache.Uptime < 59 || st.Generation != 1 {
		t.Fatalf("%+v %v", st, err)
	}
	if err := s.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff([]string{"stop apache", "stop fpm 8.3", "stop fpm 8.4"}, f.Events()); diff != "" {
		t.Fatalf("Apache before FPM: %s", diff)
	}
	if st, _ := s.Status(ctx, false); st.Running || st.Apache.Running {
		t.Fatalf("after stop: %+v", st)
	}
	for _, pf := range []string{"httpd.pid", "php-fpm-8.3.pid"} {
		if _, err := os.Stat(filepath.Join(p.RuntimeDir, pf)); !os.IsNotExist(err) {
			t.Errorf("%s left", pf)
		}
	}
}

func running(t *testing.T, f *fakeStack, p state.Paths, sites ...state.Site) *Supervisor {
	t.Helper()
	s := supervisor(t, f, p)
	if _, err := s.Apply(context.Background(), input(p, sites...)); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	f.Events()
	return s
}

// TestLedger_HTE27: FPM reloads before Apache when pools are added or
// changed; Apache first when they only go away; an unused master stops
// after Apache.
func TestLedger_HTE27(t *testing.T) {
	f, p := newStack(t), testPaths(t)
	a, b := site("a.test", "8.3"), site("b.test", "8.3")
	s := running(t, f, p, a)
	ctx := context.Background()

	// A new site: its pool must exist before Apache routes to it.
	out, err := s.Apply(ctx, input(p, a, b))
	if err != nil || !out.Reloaded || out.Generation != 2 {
		t.Fatalf("%+v %v", out, err)
	}
	if diff := cmp.Diff([]string{"test apache", "test fpm", "reload fpm 8.3", "graceful apache"}, f.Events()); diff != "" {
		t.Fatal(diff)
	}
	// A changed pool only: same order.
	a.PHPIni = map[string]string{"memory_limit": "256M"}
	if _, err := s.Apply(ctx, input(p, a, b)); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff([]string{"test apache", "test fpm", "reload fpm 8.3", "graceful apache"}, f.Events()); diff != "" {
		t.Fatal(diff)
	}
	// A removed site: Apache stops using the socket first.
	if _, err := s.Apply(ctx, input(p, a)); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff([]string{"test apache", "test fpm", "graceful apache", "reload fpm 8.3"}, f.Events()); diff != "" {
		t.Fatal(diff)
	}
	// The site moves to 8.4: the new master starts first, the old one
	// stops after Apache left it.
	a.PHP = "8.4"
	if _, err := s.Apply(ctx, input(p, a)); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff([]string{"test apache", "test fpm", "start fpm 8.4", "graceful apache", "stop fpm 8.3"}, f.Events()); diff != "" {
		t.Fatal(diff)
	}
	// Only the current and previous generations are kept.
	gens, _ := s.Layout.Generations()
	if !slices.Equal(gens, []int{4, 5}) {
		t.Fatalf("generations %v", gens)
	}
}

// TestLedger_HTE09: a failed config test changes nothing; a failed
// reload brings the previous generation back (TestRollback).
func TestLedger_HTE09(t *testing.T) {
	f, p := newStack(t), testPaths(t)
	a := site("a.test", "8.3")
	s := running(t, f, p, a)
	f.failTest = "fpm"
	a.PHPIni = map[string]string{"memory_limit": "1G"}
	_, err := s.Apply(context.Background(), input(p, a))
	if hteerr.CodeOf(err) != hteerr.ConfigTestFailed || !strings.Contains(err.Error(), "php-fpm 8.3") {
		t.Fatalf("%v", err)
	}
	if cur, _, _ := s.Layout.Current(); cur != 1 {
		t.Fatalf("current moved to %d", cur)
	}
	if _, err := os.Stat(s.Layout.GenDir(2)); !os.IsNotExist(err) {
		t.Fatal("failed generation kept")
	}
	if diff := cmp.Diff([]string{"test apache", "test fpm"}, f.Events()); diff != "" {
		t.Fatalf("nothing signalled: %s", diff)
	}
}

// TestRollback: Apache exits during the graceful restart (a port held by
// another process); the previous generation becomes current again and
// Apache is started on it.
func TestRollback(t *testing.T) {
	f, p := newStack(t), testPaths(t)
	a := site("a.test", "8.3")
	s := running(t, f, p, a)
	f.failUSR1 = true
	a.ExtraHTTPPorts = []int{3000}
	_, err := s.Apply(context.Background(), input(p, a))
	if hteerr.CodeOf(err) != hteerr.ReloadFailed || !strings.Contains(err.Error(), "rolled back") || !strings.Contains(err.Error(), "no listening sockets") {
		t.Fatalf("%v", err)
	}
	if diff := cmp.Diff([]string{"test apache", "test fpm", "reload fpm 8.3", "graceful apache", "reload fpm 8.3", "start apache"}, f.Events()); diff != "" {
		t.Fatal(diff)
	}
	if cur, _, _ := s.Layout.Current(); cur != 1 {
		t.Fatalf("current is %d, not the previous generation", cur)
	}
	if st, _ := s.Status(context.Background(), false); !st.Running {
		t.Fatalf("not running after rollback: %+v", st)
	}
}

// TestLedger_HTE32: a PID file naming another process is never signalled.
func TestLedger_HTE32(t *testing.T) {
	f, p := newStack(t), testPaths(t)
	s := running(t, f, p, site("a.test", "8.3"))
	if err := s.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	f.Events()
	f.procs[4242] = "/usr/bin/vim notes.txt"
	if err := os.WriteFile(filepath.Join(p.RuntimeDir, "httpd.pid"), []byte("4242\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := s.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if ev := f.Events(); len(ev) != 0 {
		t.Fatalf("signalled a foreign process: %v", ev)
	}
	if _, ok := f.procs[4242]; !ok {
		t.Fatal("foreign process gone")
	}
	// Start clears the stale PID file instead of failing on it.
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if st, _ := s.Status(context.Background(), false); !st.Running || st.Apache.PID == 4242 {
		t.Fatalf("%+v", st)
	}
}

func TestStopKillsAfterTimeout(t *testing.T) {
	f, p := newStack(t), testPaths(t)
	s := running(t, f, p, site("a.test", "8.3"))
	f.deaf = true
	if err := s.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff([]string{"stop apache", "kill apache", "stop fpm 8.3", "kill fpm 8.3"}, f.Events()); diff != "" {
		t.Fatal(diff)
	}
}

func TestStartFailureNamesTheCause(t *testing.T) {
	f, p := newStack(t), testPaths(t)
	s := supervisor(t, f, p)
	if _, err := s.Apply(context.Background(), input(p, site("a.test", "8.3"))); err != nil {
		t.Fatal(err)
	}
	s.Runner = &proc.Fake{Handler: func(c proc.Cmd) (proc.Result, error) {
		if strings.HasSuffix(c.Path, "apache2") {
			return proc.Result{ExitCode: 1, Stderr: []byte("(98)Address already in use: AH00072: make_sock: could not bind to address 127.0.0.1:8080")}, nil
		}
		return f.Run(context.Background(), c)
	}}
	if err := s.Start(context.Background()); hteerr.CodeOf(err) != hteerr.PortInUse {
		t.Fatalf("%v", err)
	}
}

func TestLinuxProcs(t *testing.T) {
	root := t.TempDir()
	mk := func(rel, data string) {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, rel)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, rel), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mk("stat", "cpu 1 2 3\nbtime 1760000000\nprocesses 5\n")
	mk("42/cmdline", "php-fpm: master process (/a b/php-fpm.conf)\x00\x00")
	mk("42/stat", "42 (php-fpm8.3 x) S 1 42 42 0 -1 4194624 0 0 0 0 0 0 0 0 20 0 1 0 12345 0 0")
	lp := LinuxProcs{Root: root}
	if c, err := lp.Cmdline(42); err != nil || c != "php-fpm: master process (/a b/php-fpm.conf)" {
		t.Fatalf("%q %v", c, err)
	}
	if st, err := lp.Started(42); err != nil || !st.Equal(time.Unix(1760000000, 0).Add(123450*time.Millisecond)) {
		t.Fatalf("%v %v", st, err)
	}
	if _, err := lp.Cmdline(7); err == nil {
		t.Fatal("missing process")
	}
}

func TestParseEtime(t *testing.T) {
	for in, want := range map[string]time.Duration{
		"00:05": 5 * time.Second, "01:02:03": time.Hour + 2*time.Minute + 3*time.Second, "2-00:00:01": 48*time.Hour + time.Second,
	} {
		if got, err := ParseEtime(in); err != nil || got != want {
			t.Errorf("%s: %v %v", in, got, err)
		}
	}
	if _, err := ParseEtime("x"); err == nil {
		t.Fatal("bad etime")
	}
}

func TestTail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log")
	if got, err := Tail(path, 5); err != nil || len(got) != 0 {
		t.Fatalf("missing: %v %v", got, err)
	}
	var b strings.Builder
	for i := 1; i <= 10; i++ {
		fmt.Fprintf(&b, "line %d\r\n", i)
	}
	os.WriteFile(path, []byte(b.String()), 0o600)
	if got, _ := Tail(path, 3); !slices.Equal(got, []string{"line 8", "line 9", "line 10"}) {
		t.Fatalf("%q", got)
	}
	if got, _ := Tail(path, 0); len(got) != 0 {
		t.Fatalf("%q", got)
	}
}

var update = flag.Bool("update", false, "rewrite the golden files under testdata/hte/golden")

// TestLedger_HTE57: the user unit and the LaunchAgent (golden files),
// with the program quoted for systemd and escaped for XML.
func TestLedger_HTE57(t *testing.T) {
	dir := "../../../testdata/hte/golden/autostart"
	for name, gen := range map[string]func(string) ([]byte, error){UnitName: SystemdUnit, LaunchAgentLabel + ".plist": LaunchAgent} {
		got, err := gen("/home/me/.local/bin/gash")
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, name)
		if *update {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, got, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("golden file missing (run go test -update): %v", err)
		}
		if diff := cmp.Diff(string(want), string(got)); diff != "" {
			t.Errorf("%s (-golden +rendered):\n%s", name, diff)
		}
	}
	unit, err := SystemdUnit("/home/me/.local/bin/ga sh%$")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Type=oneshot\n", "RemainAfterExit=yes\n", "WantedBy=default.target\n",
		`ExecStart="/home/me/.local/bin/ga sh%%$$" "hte" "__unit" "start"` + "\n",
		`ExecStop="/home/me/.local/bin/ga sh%%$$" "hte" "__unit" "stop"` + "\n"} {
		if !strings.Contains(string(unit), want) {
			t.Errorf("unit lacks %q:\n%s", want, unit)
		}
	}
	plist, err := LaunchAgent("/Users/me/bin/gash&<x>")
	if err != nil || !strings.Contains(string(plist), "<string>/Users/me/bin/gash&amp;&lt;x&gt;</string>") || !strings.Contains(string(plist), "<key>RunAtLoad</key>\n  <true/>") {
		t.Fatalf("%s %v", plist, err)
	}
	if _, err := SystemdUnit("gash"); hteerr.CodeOf(err) != hteerr.PlatformUnsupported {
		t.Fatal("relative program accepted")
	}
	if _, err := SystemdUnit("/a\nb"); err == nil {
		t.Fatal("newline accepted")
	}
}

// Reload re-reads the overlays of an unchanged rendering: test first,
// then FPM, then Apache; a failed test signals nothing.
func TestReloadSameRendering(t *testing.T) {
	f, p := newStack(t), testPaths(t)
	s := running(t, f, p, site("a.test", "8.3"))
	ctx := context.Background()
	f.failTest = "apache"
	if _, err := s.Reload(ctx); hteerr.CodeOf(err) != hteerr.ConfigTestFailed {
		t.Fatalf("broken overlay: %v", err)
	}
	if diff := cmp.Diff([]string{"test apache"}, f.Events()); diff != "" {
		t.Fatalf("a failed test signalled: %s", diff)
	}
	f.failTest = ""
	ran, err := s.Reload(ctx)
	if err != nil || !ran {
		t.Fatalf("reload: %v %v", ran, err)
	}
	if diff := cmp.Diff([]string{"test apache", "test fpm", "reload fpm 8.3", "graceful apache"}, f.Events()); diff != "" {
		t.Fatalf("order: %s", diff)
	}
}
