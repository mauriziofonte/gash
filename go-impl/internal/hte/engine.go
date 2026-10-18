// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package hte is the engine of gash hte: one method per command of
// design/hte.md section 9, returning a result value for the CLI to print
// (text or --json) or an error of the HTE contract. The CLI parses flags,
// builds an Engine from the process facts, and calls it; nothing here
// prints or prompts.
package hte

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/mauriziofonte/gash/internal/hostshelper"
	"github.com/mauriziofonte/gash/internal/hte/certs"
	"github.com/mauriziofonte/gash/internal/hte/child"
	"github.com/mauriziofonte/gash/internal/hte/discover"
	"github.com/mauriziofonte/gash/internal/hte/hosts"
	"github.com/mauriziofonte/gash/internal/hte/hteerr"
	"github.com/mauriziofonte/gash/internal/hte/render"
	"github.com/mauriziofonte/gash/internal/hte/service"
	"github.com/mauriziofonte/gash/internal/hte/state"
	"github.com/mauriziofonte/gash/internal/platform"
	"github.com/mauriziofonte/gash/internal/runner"
)

// Engine holds the process facts every command needs. The CLI fills it
// from the real process; tests from fakes.
type Engine struct {
	Sysroot platform.Sysroot    // platform.Real() outside tests
	GOOS    string              // runtime.GOOS
	Getenv  func(string) string // the environment snapshot (HOME, XDG_*)
	// UID and EUID are the process's own ids (os.Getuid, os.Geteuid): who
	// the user is never comes from USER or SUDO_USER (LEDGER HTE-19).
	UID, EUID int
	GID       int
	User      string // the name of UID (os/user), for the sudoers line
	Cwd       string // a relative --docroot resolves against it
	// Self is the absolute path of the gash binary: the root session and
	// the autostart units run it. Helper is the hosts helper install copies
	// (gash-hosts-helper next to Self).
	Self, Helper string
	Runner       runner.Runner
	Starter      runner.SessionStarter
	Procs        service.Procs
	// Interactive allows sudo to prompt (a terminal, not headless).
	Interactive bool
	Now         func() time.Time
	// LockWait bounds the wait for the state lock (30 s by default).
	LockWait time.Duration
	// Dial, StartTimeout: supervision overrides for tests.
	Dial         func(ctx context.Context, network, addr string) error
	StartTimeout time.Duration
	Sleep        func(time.Duration)
	// Probe replaces the HTTP probe of a site (migrate's verification)
	// in tests.
	Probe func(ctx context.Context, e render.Endpoint) service.SiteStatus
}

func (e *Engine) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now()
}

// rootOnly lists the commands euid 0 may run (HTE-19, option a).
var rootOnly = []string{"install", "uninstall", "migrate", "doctor"}

// CheckIdentity refuses euid 0 for every command but install, uninstall,
// migrate and doctor (root_not_allowed, LEDGER HTE-19).
func (e *Engine) CheckIdentity(command string) error {
	if e.EUID != 0 || slices.Contains(rootOnly, command) {
		return nil
	}
	return hteerr.New(hteerr.RootNotAllowed, "gash hte "+command+" runs as your own user, not as root",
		"Run gash hte as your own user; it asks for sudo itself when a step needs root")
}

// env is what one command resolved about the machine.
type env struct {
	dirs  platform.Dirs
	paths state.Paths
	os    platform.OS
	store state.Store
}

func (e *Engine) resolve() (*env, error) {
	dirs, err := platform.ResolveDirs(e.Getenv)
	if err != nil {
		return nil, hteerr.New(hteerr.PlatformUnsupported, "HOME is not set to an absolute path", "")
	}
	paths := state.NewPaths(dirs, e.UID)
	return &env{dirs: dirs, paths: paths, os: platform.DetectOS(e.Sysroot, e.GOOS), store: state.Store{Paths: paths}}, nil
}

// probe runs a system binary with fixed arguments for discovery.
func (e *Engine) probe(ctx context.Context) discover.Probe {
	return func(bin string, args ...string) (string, error) {
		c := child.Cmd(bin, nil, args...)
		c.Timeout = 20 * time.Second
		res, err := child.Run(ctx, e.Runner, c)
		if err != nil {
			return "", err
		}
		if !res.Success() {
			return "", fmt.Errorf("%s %s: exit %d: %s", bin, strings.Join(args, " "), res.ExitCode, child.Output(res))
		}
		return string(res.Stdout), nil
	}
}

// machine is the discovered Apache, modules and PHP versions.
type machine struct {
	apache  discover.Apache
	modules discover.Modules
	php     discover.PHPResult
}

func (e *Engine) discoverPHP(ctx context.Context, v *env) discover.PHPResult {
	cache := discover.LoadCache(v.paths.PHPCache())
	res := discover.DiscoverPHP(discover.PHPEnv{Sysroot: e.Sysroot, OS: v.os, Probe: e.probe(ctx), Cache: cache})
	if err := os.MkdirAll(filepath.Dir(v.paths.PHPCache()), 0o700); err == nil {
		cache.Save(v.paths.PHPCache())
	}
	return res
}

func (e *Engine) discover(ctx context.Context, v *env, extra []string) (*machine, error) {
	a, err := discover.FindApache(e.Sysroot, v.os)
	if err != nil {
		return nil, err
	}
	builtin, err := discover.BuiltinModules(a, e.probe(ctx))
	if err != nil {
		return nil, err
	}
	m := &machine{apache: a, modules: discover.ResolveModules(e.Sysroot, a, builtin, extra)}
	if len(m.modules.Missing) > 0 {
		return m, m.modules.MissingError(a, v.os.Family)
	}
	m.php = e.discoverPHP(ctx, v)
	return m, nil
}

// ipv6Loopback reports whether ::1 can be bound (containers may lack it).
func ipv6Loopback() bool {
	l, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		return false
	}
	l.Close()
	return true
}

func listenAddrs(set state.Settings) []string {
	out := []string{}
	for _, a := range set.Listen {
		if a == state.LoopbackIPv6 && !ipv6Loopback() {
			continue
		}
		out = append(out, a)
	}
	return out
}

// input builds the render input of the state.
func (e *Engine) input(v *env, m *machine, set state.Settings, sites []state.Site) render.Input {
	php := map[string]render.PHPVersion{}
	for _, p := range m.php.Versions {
		pv := render.PHPVersion{FPM: p.FPM, INI: p.INI, ScanDir: p.ScanDir}
		if p.NeedsSessionDir {
			pv.SessionDir = v.paths.SessionDir(p.Version)
		}
		php[p.Version] = pv
	}
	return render.Input{
		Paths:  v.paths,
		Apache: render.Apache{Binary: m.apache.Binary, Modules: m.modules.Load, MimeTypes: m.apache.MimeTypes},
		Ports:  set.Ports,
		Listen: listenAddrs(set),
		PHP:    php,
		Sites:  sites,
	}
}

func (e *Engine) supervisor(v *env) *service.Supervisor {
	return &service.Supervisor{
		Runner: e.Runner, Layout: render.Layout{Dir: v.paths.RenderDir()}, Procs: e.Procs,
		Env: child.BaseEnv(v.dirs.Home), Dial: e.Dial, Sleep: e.Sleep, StartTimeout: e.StartTimeout,
	}
}

func (e *Engine) certs(v *env) certs.Manager {
	return certs.Manager{Runner: e.Runner, OpenSSL: e.find("openssl"), OpenSSLHint: discover.OpenSSLInstallHint(v.os.Family),
		Paths: v.paths, Env: child.BaseEnv(v.dirs.Home), Now: e.Now}
}

// find returns a system binary in the base PATH, as seen through the
// sysroot ("" when absent).
func (e *Engine) find(name string) string { return child.Find(name, e.Sysroot.IsRegular) }

// prepare creates the directories a generation names: the runtime
// directory (0700, owner checked), the logs, the per-site log
// directories and the session directories.
func (e *Engine) prepare(v *env, in render.Input) error {
	if err := os.MkdirAll(filepath.Dir(v.paths.RuntimeDir), 0o700); err != nil {
		return err
	}
	if err := platform.EnsurePrivateDir(v.paths.RuntimeDir, e.UID); err != nil {
		return hteerr.Newf(hteerr.InvalidState, "remove it; gash hte creates it again", "%s: %v", v.paths.RuntimeDir, err)
	}
	dirs := []string{v.paths.LogsDir()}
	for _, s := range in.Sites {
		dirs = append(dirs, v.paths.SiteLogDir(s.Domain))
	}
	for _, p := range in.PHP {
		if p.SessionDir != "" {
			dirs = append(dirs, p.SessionDir)
		}
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return err
		}
	}
	return nil
}

// ensureCerts gives every enabled HTTPS managed site a pair that covers
// its names: a missing pair, or one that does not cover a new alias, is
// issued (HTE-47); a pair whose key does not match is an error.
func (e *Engine) ensureCerts(ctx context.Context, v *env, sites []state.Site) ([]string, error) {
	cm := e.certs(v)
	var notes []string
	for _, s := range sites {
		if !s.Enabled || !s.HTTPS || s.Kind != state.KindManaged {
			continue
		}
		info, err := cm.Show(s.Domain)
		switch {
		case err != nil:
			if err := cm.Issue(ctx, s); err != nil {
				return notes, err
			}
			notes = append(notes, "issued a certificate for "+s.Domain)
		case len(info.Uncovered(s.Names())) > 0:
			if err := cm.Issue(ctx, s); err != nil {
				return notes, err
			}
			notes = append(notes, "reissued the certificate of "+s.Domain+" for its names")
		default:
			if h := cm.Check(ctx, s); len(h.Problems) > 0 {
				return notes, hteerr.Newf(hteerr.InvalidState, "run gash hte cert renew --domain "+s.Domain,
					"the certificate of %s: %s", s.Domain, strings.Join(h.Problems, "; "))
			}
		}
	}
	return notes, nil
}

// Applied is what a change did to the running stack.
type Applied struct {
	service.Outcome
	Notes []string     `json:"notes,omitempty"`
	Hosts *hosts.Drift `json:"hosts,omitempty"`
}

// apply renders the state into a new generation and moves the stack to
// it (applyStack), then writes the hosts block.
func (e *Engine) apply(ctx context.Context, v *env, set state.Settings, sites []state.Site) (Applied, error) {
	out, err := e.applyStack(ctx, v, set, sites)
	if err != nil {
		return out, err
	}
	return out, e.applyHosts(ctx, v, sites)
}

// applyStack issues missing certificates, renders the state into a new
// generation and moves the stack to it (service.Supervisor.Apply).
func (e *Engine) applyStack(ctx context.Context, v *env, set state.Settings, sites []state.Site) (Applied, error) {
	m, err := e.discover(ctx, v, set.Modules)
	if err != nil {
		return Applied{}, err
	}
	var out Applied
	if out.Notes, err = e.ensureCerts(ctx, v, sites); err != nil {
		return out, err
	}
	in := e.input(v, m, set, sites)
	if in.Overlays, err = render.ReadOverlays(v.paths, sites); err != nil {
		return out, err
	}
	if err := e.prepare(v, in); err != nil {
		return out, err
	}
	out.Outcome, err = e.supervisor(v).Apply(ctx, in)
	return out, err
}

// desiredHosts is the block of the state.
func desiredHosts(v *env, sites []state.Site) ([]hostshelper.Entry, error) {
	st, err := hosts.LoadState(v.paths.HostsState())
	if err != nil {
		return nil, err
	}
	return hosts.Desired(sites, st.Names), nil
}

// applyHosts writes the block through the helper when it differs; before
// install (no helper) it does nothing.
func (e *Engine) applyHosts(ctx context.Context, v *env, sites []state.Site) error {
	if !e.Sysroot.IsRegular(hosts.HelperPath) {
		return nil
	}
	desired, err := desiredHosts(v, sites)
	if err != nil {
		return err
	}
	if hosts.Check(e.Sysroot, desired).InSync {
		return nil
	}
	sudo := e.find("sudo")
	if sudo == "" {
		return hteerr.New(hteerr.PrivilegedStepRequired, "sudo is not installed", "install sudo")
	}
	return hosts.Applier{Runner: e.Runner, Sudo: sudo, Helper: hosts.HelperPath}.Apply(ctx, desired)
}

// locked runs fn under the state lock and appends the audit line.
func (e *Engine) locked(v *env, command, site string, fn func() (any, error)) (any, error) {
	wait := e.LockWait
	if wait == 0 {
		wait = 30 * time.Second
	}
	l, err := state.AcquireLock(v.paths.Lock(), wait, nil)
	if err != nil {
		return nil, err
	}
	defer l.Unlock()
	res, err := fn()
	e.audit(v, command, site, res, err)
	return res, err
}

type auditLine struct {
	TS      string `json:"ts"`
	Command string `json:"command"`
	Site    string `json:"site,omitempty"`
	Outcome string `json:"outcome"`
	Details any    `json:"details,omitempty"`
}

func (e *Engine) audit(v *env, command, site string, res any, err error) {
	line := auditLine{TS: e.now().UTC().Format(time.RFC3339), Command: command, Site: site, Outcome: "ok", Details: res}
	if err != nil {
		line.Outcome, line.Details = "error", err.Error()
		if he, ok := hteerr.As(err); ok {
			line.Outcome = string(he.Code)
		}
	}
	data, mErr := json.Marshal(line)
	if mErr != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(v.paths.Audit()), 0o700); err != nil {
		return
	}
	f, oErr := os.OpenFile(v.paths.Audit(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if oErr != nil {
		return
	}
	defer f.Close()
	f.Write(append(data, '\n'))
}

// installed loads hte.toml: hte_not_installed when it is missing.
func (e *Engine) installed(v *env) (state.Settings, error) {
	return v.store.LoadSettings()
}

// sites loads every site; a file that does not load stops a mutation
// (invalid_state), since the rendering would drop that site.
func (e *Engine) sites(v *env) ([]state.Site, error) {
	sites, problems, err := v.store.LoadSites()
	if err != nil {
		return nil, err
	}
	if len(problems) > 0 {
		return nil, problems[0]
	}
	return sites, nil
}
