// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package sysinfo holds sysinfo of lib/modules/sysinfo.sh: the overview
// card and the ten /etc enumerations, verbose or compact (--llm), with
// lazy sub-headers, and the collectors ai_sysinfo reuses through a
// Session (Begin, Collect, Deep for the six drill-downs, Sudo, Release). Files
// are read through a platform.Sysroot; the tools whose text 2.x showed
// (hostname, uname, df, uptime, systemctl, ss, systemd-detect-virt, sudo)
// stay CLIs. Defects of 2.x are reproduced (LEDGER SYS-15 to SYS-20).
package sysinfo

import (
	"context"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/platform"
	"github.com/mauriziofonte/gash/internal/runner"
	"github.com/mauriziofonte/gash/internal/ui"
)

// Sections are the words sysinfo takes, in the order of its help.
var Sections = []string{"overview", "identity", "storage", "services", "auth", "network", "security", "webstack", "mail", "infra", "system", "all"}

// needsSudo are the sections that read anything as root; sysinfo asks
// for sudo only for those, and for all (SYS-15).
var needsSudo = map[string]bool{"identity": true, "storage": true, "services": true, "auth": true, "security": true, "system": true, "all": true}

// Module is the sysinfo handler with its seams: the runner, the sysroot,
// the user id and the period of the sudo keep-alive (0: none).
type Module struct {
	run  runner.Runner
	sys  platform.Sysroot
	uid  func() int
	tick time.Duration
}

// New returns the module wired to the real system.
func New() *Module {
	return &Module{run: runner.Exec{}, sys: platform.Real(), uid: os.Getuid, tick: 50 * time.Second}
}

// Register binds sysinfo to t.
func Register(t *invoke.Table) { New().Register(t) }

// Register binds the function of m to t.
func (m *Module) Register(t *invoke.Table) { t.Handle("sysinfo", m.sysinfo) }

// sysinfo is sysinfo [section] [--llm] [--no-color].
func (m *Module) sysinfo(c *invoke.Call) int {
	if help.NeedsOf(c, c.Arg(0)) {
		return 0
	}
	section, llm, noColor := "all", false, false
	for _, a := range c.Args {
		switch a {
		case "--llm":
			llm = true
		case "--no-color":
			noColor = true
		case "overview", "identity", "storage", "services", "auth", "network", "security", "webstack", "mail", "infra", "system", "all":
			// The last one wins (SYS-20).
			section = a
		default:
			p := ui.NewPrinter(c.Stderr, c.Term, false)
			p.Error("Unknown argument: " + a)
			p.Info("Sections: overview identity storage services auth network security webstack mail infra system all")
			p.Info("Flags: --llm, --no-color")
			return 1
		}
	}
	// --llm implies --no-color.
	if llm {
		noColor = true
	}
	s := m.session(c, noColor)
	if needsSudo[section] {
		s = m.Begin(c, noColor, !llm)
	}
	defer s.Release()
	// The lazy headers keep their state in memory: no state directory.
	o := s.output(c.Stdout, llm, true)
	if section == "all" {
		for _, name := range Sections[:len(Sections)-1] {
			s.collect(o, name)
		}
	} else {
		s.collect(o, section)
	}
	o.close()
	return 0
}

// Session is one enumeration: the caller, the style and the sudo state of
// __sysinfo_ensure_sudo, until Release.
type Session struct {
	m     *Module
	c     *invoke.Call
	ctx   context.Context
	env   []string
	style ui.Style
	f     files
	// sudo is __SYSINFO_HAS_SUDO.
	sudo bool
	stop context.CancelFunc
	done chan struct{}
}

// Begin is __sysinfo_ensure_sudo for one run of c: root needs nothing;
// cached credentials (`sudo -n true`) are kept alive; otherwise, on a
// terminal outside headless mode, `sudo -v` asks once, with the Info and
// Warning lines when verbose. sysinfo calls it only for the sections that
// read as root (SYS-15). noColor is the --no-color flag of the run.
func (m *Module) Begin(c *invoke.Call, noColor, verbose bool) *Session {
	s := m.session(c, noColor)
	if s.sudo {
		return s
	}
	if s.status("sudo", "-n", "true") != 0 {
		if c.Term.Headless() || !c.Term.StdinTTY() {
			return s
		}
		p := s.style.Printer(c.Stderr)
		if verbose {
			p.Info("Elevated privileges needed for full system enumeration.")
		}
		res, err := m.run.Run(s.ctx, runner.Cmd{Path: "sudo", Args: []string{"-v"},
			Options: runner.Options{Env: s.env, Stdin: c.Stdin, Stdout: c.Stdout, Stderr: io.Discard}})
		if err != nil || res.ExitCode != 0 {
			if verbose {
				p.Warning("Could not obtain sudo. Some data will be limited.")
			}
			return s
		}
	}
	s.sudo = true
	s.keepAlive()
	return s
}

// session is a Session that asks for nothing: root reads as root, any
// other user reads as itself.
func (m *Module) session(c *invoke.Call, noColor bool) *Session {
	ctx := c.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	s := &Session{m: m, c: c, ctx: ctx, env: c.Env.Child(), style: ui.StyleOf(c.Term, noColor)}
	s.f = files{sys: m.sys, u8: s.style.UTF8}
	s.sudo = m.uid() == 0
	return s
}

// keepAlive refreshes the credentials every tick (`sudo -n true`, the
// first one at once) until Release.
func (s *Session) keepAlive() {
	if s.m.tick <= 0 {
		return
	}
	ctx, cancel := context.WithCancel(s.ctx)
	s.stop, s.done = cancel, make(chan struct{})
	go func() {
		defer close(s.done)
		t := time.NewTicker(s.m.tick)
		defer t.Stop()
		for {
			_, _ = s.m.run.Run(ctx, runner.Cmd{Path: "sudo", Args: []string{"-n", "true"},
				Options: runner.Options{Env: s.env, Stdout: io.Discard, Stderr: io.Discard}})
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
}

// Release is __sysinfo_release_sudo: the keep-alive ends.
func (s *Session) Release() {
	if s.stop != nil {
		s.stop()
		<-s.done
		s.stop = nil
	}
}

// Collect writes one collector (a section name, overview included) to w
// in the compact format, with every header printed at once, as ai_sysinfo
// runs them outside sysinfo (no state directory).
func (s *Session) Collect(w io.Writer, section string) {
	s.collect(s.output(w, true, false), section)
}

func (s *Session) collect(o *out, section string) {
	switch section {
	case "overview":
		s.overview(o)
	case "identity":
		s.identity(o)
	case "storage":
		s.storage(o)
	case "services":
		s.services(o)
	case "auth":
		s.auth(o)
	case "network":
		s.network(o)
	case "security":
		s.security(o)
	case "webstack":
		s.webstack(o)
	case "mail":
		s.mail(o)
	case "infra":
		s.infra(o)
	case "system":
		s.system(o)
	}
}

// elevated is the test of __sysinfo_sudo: cached sudo and not root.
func (s *Session) elevated() bool { return s.sudo && s.m.uid() != 0 }

// status runs a program with its output discarded.
func (s *Session) status(name string, args ...string) int {
	res, err := s.m.run.Run(s.ctx, runner.Cmd{Path: name, Args: args,
		Options: runner.Options{Env: s.env, Stdout: io.Discard, Stderr: io.Discard}})
	if se, ok := err.(*runner.StartError); ok {
		return se.Status
	}
	return res.ExitCode
}

// tool is `NAME ARGS 2>/dev/null`: its stdout and status.
func (s *Session) tool(name string, args ...string) ([]byte, int) {
	res, err := s.m.run.Run(s.ctx, runner.Cmd{Path: name, Args: args,
		Options: runner.Options{Env: s.env, Stderr: io.Discard}})
	if se, ok := err.(*runner.StartError); ok {
		return nil, se.Status
	}
	return res.Stdout, res.ExitCode
}

// toolShown is `NAME ARGS` with its stderr on the caller's, a missing
// program reported as Bash does.
func (s *Session) toolShown(name string, args ...string) ([]byte, int) {
	res, err := s.m.run.Run(s.ctx, runner.Cmd{Path: name, Args: args,
		Options: runner.Options{Env: s.env, Stderr: s.c.Stderr}})
	if se, ok := err.(*runner.StartError); ok {
		_, _ = io.WriteString(s.c.Stderr, "bash: "+se.Error()+"\n")
		return nil, se.Status
	}
	return res.Stdout, res.ExitCode
}

// has is `command -v NAME >/dev/null 2>&1`.
func (s *Session) has(name string) bool {
	_, err := runner.LookPath(name, s.env, "")
	return err == nil
}

// either is `$(A 2>/dev/null || B ...)`: A's output, then B's when A
// failed; b shows its stderr when shown.
func (s *Session) either(a []string, b []string, shown bool) string {
	out, rc := s.tool(a[0], a[1:]...)
	if rc != 0 {
		var more []byte
		if shown {
			more, _ = s.toolShown(b[0], b[1:]...)
		} else {
			more, _ = s.tool(b[0], b[1:]...)
		}
		out = append(append([]byte(nil), out...), more...)
	}
	return runner.CommandSubst(out)
}

// The privileged reads of __sysinfo_sudo: through `sudo` when elevated,
// else as the user. Globs and paths stay those of the user (SYS-19).

// sudoRead is `__sysinfo_sudo cat FILE`.
func (s *Session) sudoRead(p string) []byte {
	if !s.elevated() {
		return s.f.read(p)
	}
	out, _ := s.tool("sudo", "cat", s.m.sys.Path(p))
	return out
}

// sudoFind is `__sysinfo_sudo find ROOT EXPR` read by a while loop: the
// paths in the order of a lexical walk (SYS-41).
func (s *Session) sudoFind(root string, q query) []string {
	if !s.elevated() {
		return s.f.find(root, q)
	}
	args := []string{"find", s.m.sys.Path(root)}
	if q.maxDepth > 0 {
		args = append(args, "-maxdepth", strconv.Itoa(q.maxDepth))
	}
	if q.typ != 0 {
		args = append(args, "-type", string(q.typ))
	}
	for i, n := range q.names {
		if i > 0 {
			args = append(args, "-o")
		}
		args = append(args, "-name", n)
	}
	out, _ := s.tool("sudo", args...)
	var ps []string
	for _, l := range readLines(out) {
		ps = append(ps, s.sysPath(l))
	}
	sortPaths(ps)
	return ps
}

// sudoLs is `__sysinfo_sudo ls DIR/`.
func (s *Session) sudoLs(dir string) []string {
	if !s.elevated() {
		return s.f.ls(dir)
	}
	out, _ := s.tool("sudo", "ls", s.m.sys.Path(dir)+"/")
	return sorted(lines(out))
}

// sysPath maps a host path a root-side tool printed back to its system
// path (the same path outside tests).
func (s *Session) sysPath(h string) string {
	if p, ok := s.m.sys.SystemPath(h); ok {
		return p
	}
	return h
}
