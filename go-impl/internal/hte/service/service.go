// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package service supervises the userland stack (design/hte.md 5): one
// Apache master and one PHP-FPM master per PHP version, started as the
// user, daemonized by themselves, driven through their PID files, and the
// change protocol: render a new generation, config-test it, swap, reload
// gracefully (FPM before Apache; Apache first when sites only went away),
// check health, and roll back to the previous generation on any failure.
// gash never runs a supervisor process of its own.
package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mauriziofonte/gash/internal/hte/child"
	"github.com/mauriziofonte/gash/internal/hte/hteerr"
	"github.com/mauriziofonte/gash/internal/hte/render"
	"github.com/mauriziofonte/gash/internal/runner"
)

// Log markers, verified on Apache 2.4.58 and PHP-FPM 8.3 (Ubuntu 24.04):
// Apache logs AH00489 at notice level whatever LogLevel says, after every
// start and graceful restart; FPM logs its readiness at its default
// log_level (notice) after every start and SIGUSR2 re-exec.
const (
	apacheReady = "resuming normal operations"
	fpmReady    = "ready to handle connections"
)

// Lines after which a master is gone for good: configuration failure, a
// port another process holds (the master exits during a graceful
// restart), logs it cannot open, FPM's failed initialization.
var (
	apacheFailed = []string{"AH00016", "no listening sockets available", "AH00019", "AH00015"}
	fpmFailed    = []string{"FPM initialization failed"}
)

// Default timeouts.
const (
	DefaultStartTimeout = 20 * time.Second
	DefaultStopTimeout  = 10 * time.Second
	killWait            = 3 * time.Second
	pollEvery           = 100 * time.Millisecond
)

// Supervisor starts, stops, reloads and inspects the masters of the
// generations in Layout. Callers hold the state lock around every method
// that changes something, and create the runtime and log directories the
// generation names before Apply and Start.
type Supervisor struct {
	Runner runner.Runner
	Layout render.Layout
	Procs  Procs
	// Env is the environment of the masters (child.BaseEnv(home)); FPM
	// masters get PHP_INI_SCAN_DIR on top of it.
	Env []string
	// Dial reports whether addr accepts connections; nil dials for real.
	Dial         func(ctx context.Context, network, addr string) error
	Sleep        func(time.Duration)
	StartTimeout time.Duration
	StopTimeout  time.Duration
}

func (s *Supervisor) sleep(d time.Duration) {
	if s.Sleep != nil {
		s.Sleep(d)
		return
	}
	time.Sleep(d)
}

func (s *Supervisor) startTimeout() time.Duration {
	if s.StartTimeout > 0 {
		return s.StartTimeout
	}
	return DefaultStartTimeout
}

func (s *Supervisor) stopTimeout() time.Duration {
	if s.StopTimeout > 0 {
		return s.StopTimeout
	}
	return DefaultStopTimeout
}

func (s *Supervisor) dial(ctx context.Context, network, addr string) error {
	if s.Dial != nil {
		return s.Dial(ctx, network, addr)
	}
	d := net.Dialer{Timeout: time.Second}
	c, err := d.DialContext(ctx, network, addr)
	if err != nil {
		return err
	}
	return c.Close()
}

// currentPath is where masters are started: render/current/<rel>, so a
// reload reads the generation current then points to.
func (s *Supervisor) currentPath(rel string) string {
	return filepath.Join(s.Layout.Dir, "current", rel)
}

// master is one daemon of a generation.
type master struct {
	name    string // "apache", "php-fpm 8.3"
	pidFile string
	log     string
	ident   func(cmdline string) bool
	ready   string
	failed  []string
	network string
	addrs   []string // Listen addresses or FPM sockets
}

func (s *Supervisor) apache(a render.ApacheMaster) master {
	conf := s.currentPath(a.Config)
	return master{
		name: "apache", pidFile: a.PidFile, log: a.ErrorLog, ready: apacheReady, failed: apacheFailed,
		network: "tcp", addrs: a.Listen,
		ident: func(c string) bool { return strings.Contains(c+" ", " -f "+conf+" ") },
	}
}

func (s *Supervisor) fpm(f render.FPMMaster) master {
	conf := s.currentPath(f.Config)
	return master{
		name: "php-fpm " + f.Version, pidFile: f.PidFile, log: f.ErrorLog, ready: fpmReady, failed: fpmFailed,
		network: "unix", addrs: f.Sockets,
		ident: func(c string) bool {
			return strings.Contains(c, "("+conf+")") || strings.Contains(c+" ", " --fpm-config "+conf+" ")
		},
	}
}

// pid returns the master's PID when its PID file names a live process
// whose command line names our configuration.
func (s *Supervisor) pid(m master) (int, bool) {
	data, err := os.ReadFile(m.pidFile)
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid < 2 {
		return 0, false
	}
	cmd, err := s.Procs.Cmdline(pid)
	if err != nil || !m.ident(cmd) {
		return pid, false
	}
	return pid, true
}

// clearStale removes a PID file whose process is not ours: Apache refuses
// to start while the PID it names is alive, even when it was recycled.
func (s *Supervisor) clearStale(m master) error {
	if _, ok := s.pid(m); ok {
		return nil
	}
	if err := os.Remove(m.pidFile); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

func logSize(path string) int64 {
	fi, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return fi.Size()
}

// logSince is what path gained after offset (all of it when it shrank).
func logSince(path string, offset int64) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	if fi, err := f.Stat(); err == nil && fi.Size() < offset {
		offset = 0
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return ""
	}
	data, _ := io.ReadAll(io.LimitReader(f, 1<<20))
	return string(data)
}

func failedLines(text string, markers []string) string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		for _, m := range markers {
			if strings.Contains(line, m) {
				out = append(out, strings.TrimSpace(line))
				break
			}
		}
	}
	return strings.Join(out, "; ")
}

// lastLines are the last n lines of text, for error details.
func lastLines(text string, n int) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// waitReady waits until the master logged its readiness after offset,
// its PID file names it, and every address accepts connections.
func (s *Supervisor) waitReady(ctx context.Context, m master, offset int64) error {
	deadline := time.Now().Add(s.startTimeout())
	for {
		text := logSince(m.log, offset)
		if bad := failedLines(text, m.failed); bad != "" {
			return fmt.Errorf("%s failed: %s", m.name, bad)
		}
		if strings.Contains(text, m.ready) {
			if _, ok := s.pid(m); ok && s.accepting(ctx, m) == "" {
				return nil
			}
		}
		if time.Now().After(deadline) {
			why := "it did not log that it is ready"
			if strings.Contains(text, m.ready) {
				if _, ok := s.pid(m); !ok {
					why = "its PID file names no running master"
				} else {
					why = s.accepting(ctx, m) + " does not accept connections"
				}
			}
			return fmt.Errorf("%s was not ready after %s: %s (log: %s)", m.name, s.startTimeout(), why, lastLines(text, 3))
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		s.sleep(pollEvery)
	}
}

// accepting returns the first address that refuses connections, or "".
func (s *Supervisor) accepting(ctx context.Context, m master) string {
	for _, a := range m.addrs {
		if s.dial(ctx, m.network, a) != nil {
			return a
		}
	}
	return ""
}

func (s *Supervisor) startApache(ctx context.Context, a render.ApacheMaster) error {
	m := s.apache(a)
	if err := s.clearStale(m); err != nil {
		return err
	}
	offset := logSize(m.log)
	res, err := child.Run(ctx, s.Runner, child.Cmd(a.Binary, s.Env, "-f", s.currentPath(a.Config), "-k", "start"))
	if err != nil {
		return fmt.Errorf("apache: %w", err)
	}
	if !res.Success() {
		return fmt.Errorf("apache did not start: %s", firstNonEmpty(child.Output(res), lastLines(logSince(m.log, offset), 3)))
	}
	return s.waitReady(ctx, m, offset)
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func (s *Supervisor) fpmEnv(f render.FPMMaster) []string {
	env := slices.Clone(s.Env)
	if env == nil {
		env = child.BaseEnv("")
	}
	return append(env, f.Env()...)
}

func (s *Supervisor) startFPM(ctx context.Context, f render.FPMMaster) error {
	m := s.fpm(f)
	if err := s.clearStale(m); err != nil {
		return err
	}
	offset := logSize(m.log)
	res, err := child.Run(ctx, s.Runner, child.Cmd(f.Binary, s.fpmEnv(f), "--fpm-config", s.currentPath(f.Config), "-c", f.INI))
	if err != nil {
		return fmt.Errorf("%s: %w", m.name, err)
	}
	if !res.Success() {
		return fmt.Errorf("%s did not start: %s", m.name, firstNonEmpty(child.Output(res), lastLines(logSince(m.log, offset), 3)))
	}
	return s.waitReady(ctx, m, offset)
}

// signalReady sends sig to a running master and waits until it is ready
// again (Apache SIGUSR1: graceful restart; FPM SIGUSR2: re-exec).
func (s *Supervisor) signalReady(ctx context.Context, m master, sig syscall.Signal) error {
	pid, ok := s.pid(m)
	if !ok {
		return fmt.Errorf("%s is not running", m.name)
	}
	offset := logSize(m.log)
	if err := s.Procs.Signal(pid, sig); err != nil {
		return fmt.Errorf("%s: signal %s: %w", m.name, sig, err)
	}
	return s.waitReady(ctx, m, offset)
}

// stop ends a master: SIGTERM, then SIGKILL after the stop timeout. A PID
// whose command line does not name our configuration is never signalled.
func (s *Supervisor) stop(m master) error {
	pid, ok := s.pid(m)
	if !ok {
		return s.clearStale(m)
	}
	if err := s.Procs.Signal(pid, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
		return fmt.Errorf("%s: %w", m.name, err)
	}
	if s.gone(m, pid, s.stopTimeout()) {
		return s.clearStale(m)
	}
	if err := s.Procs.Signal(pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		return fmt.Errorf("%s: %w", m.name, err)
	}
	if s.gone(m, pid, killWait) {
		return s.clearStale(m)
	}
	return fmt.Errorf("%s (pid %d) did not stop", m.name, pid)
}

func (s *Supervisor) gone(m master, pid int, wait time.Duration) bool {
	deadline := time.Now().Add(wait)
	for {
		cmd, err := s.Procs.Cmdline(pid)
		if err != nil || !m.ident(cmd) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		s.sleep(pollEvery)
	}
}

// Current returns the current generation and its manifest; ok is false
// before the first generation.
func (s *Supervisor) Current() (n int, m *render.Manifest, ok bool, err error) {
	n, ok, err = s.Layout.Current()
	if err != nil || !ok {
		return 0, nil, false, err
	}
	m, err = render.ReadManifest(s.Layout.GenDir(n))
	if err != nil {
		return 0, nil, false, err
	}
	return n, m, true, nil
}

// up is which masters of a manifest run.
type up struct {
	apache bool
	fpm    map[string]bool
}

func (u up) any() bool {
	if u.apache {
		return true
	}
	for _, v := range u.fpm {
		if v {
			return true
		}
	}
	return false
}

func (s *Supervisor) running(m *render.Manifest) up {
	u := up{fpm: map[string]bool{}}
	if m == nil {
		return u
	}
	_, u.apache = s.pid(s.apache(m.Apache))
	for _, f := range m.FPM {
		_, u.fpm[f.Version] = s.pid(s.fpm(f))
	}
	return u
}

// Running reports whether any master of the current generation runs.
func (s *Supervisor) Running() (bool, error) {
	_, m, ok, err := s.Current()
	if err != nil || !ok {
		return false, err
	}
	return s.running(m).any(), nil
}

// Start starts every master of the current generation that is not
// running: FPM first, then Apache.
func (s *Supervisor) Start(ctx context.Context) error {
	_, m, ok, err := s.Current()
	if err != nil {
		return err
	}
	if !ok {
		return hteerr.New(hteerr.InvalidState, "nothing is rendered yet", "run gash hte service reload")
	}
	u := s.running(m)
	for _, f := range m.FPM {
		if !u.fpm[f.Version] {
			if err := s.startFPM(ctx, f); err != nil {
				return hteerr.Newf(hteerr.ReloadFailed, "run gash hte service logs", "%v", err)
			}
		}
	}
	if !u.apache {
		if err := s.startApache(ctx, m.Apache); err != nil {
			return startError(err, m.Apache)
		}
	}
	return nil
}

// startError maps a failed Apache start: a port another process holds is
// port_in_use.
func startError(err error, a render.ApacheMaster) error {
	if strings.Contains(err.Error(), "Address already in use") || strings.Contains(err.Error(), "could not bind") {
		return hteerr.Newf(hteerr.PortInUse, "run gash hte doctor: it names the process holding the port", "%v", err)
	}
	return hteerr.Newf(hteerr.ReloadFailed, "run gash hte service logs", "%v", err)
}

// Graceful makes a running Apache read its configuration and
// certificates again (a renewed pair under the same paths).
func (s *Supervisor) Graceful(ctx context.Context) error {
	_, m, ok, err := s.Current()
	if err != nil || !ok {
		return err
	}
	if _, up := s.pid(s.apache(m.Apache)); !up {
		return nil
	}
	if err := s.signalReady(ctx, s.apache(m.Apache), syscall.SIGUSR1); err != nil {
		return hteerr.Newf(hteerr.ReloadFailed, "run gash hte service logs", "%v", err)
	}
	return nil
}

// Reload makes the running stack read the current generation again
// (the Apache overlays are copies inside it; the global PHP overlay
// directories are read by the FPM masters, outside the rendering):
// config test first, then FPM SIGUSR2, then Apache graceful. A failed
// test changes nothing; it reports whether anything ran.
func (s *Supervisor) Reload(ctx context.Context) (bool, error) {
	_, m, ok, err := s.Current()
	if err != nil || !ok {
		return false, err
	}
	if err := s.Test(ctx, filepath.Join(s.Layout.Dir, "current"), m); err != nil {
		return false, err
	}
	u := s.running(m)
	for _, f := range m.FPM {
		if u.fpm[f.Version] {
			if err := s.signalReady(ctx, s.fpm(f), syscall.SIGUSR2); err != nil {
				return true, hteerr.Newf(hteerr.ReloadFailed, "run gash hte service logs", "%v", err)
			}
		}
	}
	if u.apache {
		if err := s.signalReady(ctx, s.apache(m.Apache), syscall.SIGUSR1); err != nil {
			return true, hteerr.Newf(hteerr.ReloadFailed, "run gash hte service logs, then gash hte service start", "%v", err)
		}
	}
	return u.any(), nil
}

// ReloadFPM makes the running FPM master of version re-read its ini
// files (SIGUSR2): a change of the global overlay, which the rendering
// does not see. A master that is not running is left alone.
func (s *Supervisor) ReloadFPM(ctx context.Context, version string) error {
	_, m, ok, err := s.Current()
	if err != nil || !ok {
		return err
	}
	for _, f := range m.FPM {
		if f.Version != version {
			continue
		}
		if _, up := s.pid(s.fpm(f)); !up {
			return nil
		}
		if err := s.signalReady(ctx, s.fpm(f), syscall.SIGUSR2); err != nil {
			return hteerr.Newf(hteerr.ReloadFailed, "run gash hte service logs", "%v", err)
		}
	}
	return nil
}

// Stop stops Apache, then every FPM master: those of the current
// generation and any other gash master whose PID file is in the runtime
// directory (a version dropped by a failed reload).
func (s *Supervisor) Stop(ctx context.Context) error {
	_, m, ok, err := s.Current()
	if err != nil || !ok {
		return err
	}
	var errs []error
	if err := s.stop(s.apache(m.Apache)); err != nil {
		errs = append(errs, err)
	}
	for _, f := range s.allFPM(m) {
		if err := s.stop(s.fpm(f)); err != nil {
			errs = append(errs, err)
		}
	}
	if err := errors.Join(errs...); err != nil {
		return hteerr.Newf(hteerr.ReloadFailed, "run gash hte service status", "%v", err)
	}
	return nil
}

// allFPM is m's FPM masters plus those of the previous generation.
func (s *Supervisor) allFPM(m *render.Manifest) []render.FPMMaster {
	out := slices.Clone(m.FPM)
	if n, ok, err := s.Layout.Previous(); err == nil && ok {
		if pm, err := render.ReadManifest(s.Layout.GenDir(n)); err == nil {
			for _, f := range pm.FPM {
				if _, has := m.Master(f.Version); !has {
					out = append(out, f)
				}
			}
		}
	}
	return out
}

// Test runs the config tests of the generation in dir: httpd -t, and
// php-fpm -t for each master with its php.ini and PHP_INI_SCAN_DIR.
func (s *Supervisor) Test(ctx context.Context, dir string, m *render.Manifest) error {
	res, err := child.Run(ctx, s.Runner, child.Cmd(m.Apache.Binary, s.Env, "-t", "-f", filepath.Join(dir, m.Apache.Config)))
	if err != nil {
		return err
	}
	if !res.Success() {
		return hteerr.Newf(hteerr.ConfigTestFailed, "fix the site state or the overlay named in the message",
			"apache: %s", child.Output(res))
	}
	for _, f := range m.FPM {
		res, err := child.Run(ctx, s.Runner, child.Cmd(f.Binary, s.fpmEnv(f), "-t", "--fpm-config", filepath.Join(dir, f.Config), "-c", f.INI))
		if err != nil {
			return err
		}
		if !res.Success() {
			return hteerr.Newf(hteerr.ConfigTestFailed, "fix the php_ini values or the global PHP overlay named in the message",
				"php-fpm %s: %s", f.Version, child.Output(res))
		}
	}
	return nil
}

// Outcome is what Apply did.
type Outcome struct {
	Generation int      `json:"generation"`
	Changed    bool     `json:"changed"`  // a new generation was made current
	Reloaded   bool     `json:"reloaded"` // the running stack moved to it
	Pruned     []string `json:"pruned,omitempty"`
}

// Apply renders in as a new generation and makes it current, following
// the change protocol. When the rendering equals the current generation
// nothing changes. When nothing runs, the new generation becomes current
// and nothing is started. When the stack runs, it moves to the new
// generation; on any failure the previous generation is made current
// again and the stack is moved back (reload_failed says whether that
// worked).
func (s *Supervisor) Apply(ctx context.Context, in render.Input) (Outcome, error) {
	cur, curM, hasCur, err := s.Current()
	if err != nil {
		return Outcome{}, err
	}
	if hasCur {
		same := in
		same.GenDir = s.Layout.GenDir(cur)
		g, err := render.Render(same)
		if err != nil {
			return Outcome{}, err
		}
		if equal, err := sameAsDisk(same.GenDir, g); err != nil {
			return Outcome{}, err
		} else if equal {
			return Outcome{Generation: cur}, nil
		}
	}
	n, err := s.Layout.Next()
	if err != nil {
		return Outcome{}, err
	}
	in.GenDir = s.Layout.GenDir(n)
	g, err := render.Render(in)
	if err != nil {
		return Outcome{}, err
	}
	if _, err := s.Layout.Write(n, g); err != nil {
		return Outcome{}, err
	}
	newM, err := render.ReadManifest(in.GenDir)
	if err == nil {
		err = s.Test(ctx, in.GenDir, newM)
	}
	if err != nil {
		os.RemoveAll(in.GenDir)
		return Outcome{}, err
	}
	was := s.running(curM)
	if err := s.Layout.Swap(n); err != nil {
		return Outcome{}, err
	}
	out := Outcome{Generation: n, Changed: true}
	if was.any() {
		if err := s.transition(ctx, curM, newM, was); err != nil {
			return out, s.rollback(ctx, newM, curM, hasCur, err)
		}
		out.Reloaded = true
	}
	out.Pruned, err = s.Layout.Prune()
	return out, err
}

// rollback makes the previous generation current again and moves the
// stack back to it.
func (s *Supervisor) rollback(ctx context.Context, failed, prev *render.Manifest, hasPrev bool, cause error) error {
	hint := "run gash hte service logs; gash hte doctor names the remedy"
	if !hasPrev {
		return hteerr.Newf(hteerr.ReloadFailed, hint, "%v; no previous generation to roll back to", cause)
	}
	if _, err := s.Layout.Rollback(); err != nil {
		return hteerr.Newf(hteerr.ReloadFailed, hint, "%v; rollback failed: %v", cause, err)
	}
	// What runs now: a master the failure ended is started again, since
	// the stack ran before.
	if err := s.transition(ctx, failed, prev, s.running(failed)); err != nil {
		return hteerr.Newf(hteerr.ReloadFailed, hint, "%v; rollback to the previous generation failed: %v", cause, err)
	}
	return hteerr.Newf(hteerr.ReloadFailed, hint, "%v; rolled back to the previous generation, which is running", cause)
}

func sameMaster(a, b render.FPMMaster) bool {
	return a.Binary == b.Binary && a.INI == b.INI && a.ScanDir == b.ScanDir && a.PidFile == b.PidFile && a.Config == b.Config
}

// lostOnly reports a socket set that only shrank.
func lostOnly(from, to []string) bool {
	if len(to) >= len(from) {
		return false
	}
	for _, t := range to {
		if !slices.Contains(from, t) {
			return false
		}
	}
	return true
}

// transition moves the masters that run (u) from one manifest to the
// next. Phase 1: FPM masters that gain or keep pools are reloaded, or
// started, so that every socket the new Apache configuration names
// exists. Phase 2: Apache restarts gracefully (or starts). Phase 3: FPM
// masters that only lost pools reload, masters no longer used stop: a
// removed site's pool goes away only after Apache stopped using it
// (HTE-27).
func (s *Supervisor) transition(ctx context.Context, from, to *render.Manifest, u up) error {
	fromFPM := map[string]render.FPMMaster{}
	if from != nil {
		for _, f := range from.FPM {
			fromFPM[f.Version] = f
		}
	}
	var later []render.FPMMaster
	for _, f := range to.FPM {
		old, had := fromFPM[f.Version]
		var err error
		switch {
		case !u.fpm[f.Version]:
			err = s.startFPM(ctx, f)
		case !had || !sameMaster(old, f):
			if err = s.stop(s.fpm(old)); err == nil {
				err = s.startFPM(ctx, f)
			}
		case lostOnly(old.Sockets, f.Sockets):
			later = append(later, f)
		default:
			err = s.signalReady(ctx, s.fpm(f), syscall.SIGUSR2)
		}
		if err != nil {
			return err
		}
	}
	sameApache := from != nil && from.Apache.Binary == to.Apache.Binary && from.Apache.PidFile == to.Apache.PidFile
	switch {
	case u.apache && sameApache:
		if err := s.signalReady(ctx, s.apache(to.Apache), syscall.SIGUSR1); err != nil {
			return err
		}
	case u.apache:
		if err := s.stop(s.apache(from.Apache)); err != nil {
			return err
		}
		fallthrough
	default:
		if err := s.startApache(ctx, to.Apache); err != nil {
			return err
		}
	}
	for _, f := range later {
		if err := s.signalReady(ctx, s.fpm(f), syscall.SIGUSR2); err != nil {
			return err
		}
	}
	for v, old := range fromFPM {
		if _, keep := to.Master(v); !keep && u.fpm[v] {
			if err := s.stop(s.fpm(old)); err != nil {
				return err
			}
		}
	}
	return nil
}

// sameAsDisk reports whether dir holds exactly the files of g.
func sameAsDisk(dir string, g *render.Generation) (bool, error) {
	count := 0
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			count++
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	if count != len(g.Files) {
		return false, nil
	}
	for _, f := range g.Files {
		data, err := os.ReadFile(filepath.Join(dir, f.Path))
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if !bytes.Equal(data, f.Data) {
			return false, nil
		}
	}
	return true, nil
}
