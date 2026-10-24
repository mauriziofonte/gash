// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package ssh holds gash_ssh_auto_unlock of lib/modules/ssh.sh: the keys
// of the settings file, the agent search, and the terminal driver of
// ssh-add that replaces the generated expect script (LEDGER SYS-37).
package ssh

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"time"

	"mvdan.cc/sh/v3/syntax"

	"github.com/mauriziofonte/gash/internal/config"
	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/runner"
	"github.com/mauriziofonte/gash/internal/system/handoff"
	"github.com/mauriziofonte/gash/internal/ui"
)

// noAgent is the ssh-add message 2.x looked for in `ssh-add -l`.
const noAgent = "Could not open a connection to your authentication agent"

// Module is the ssh handler with its seams: tests replace the runner, the
// hand-off descriptor and the two timeouts.
type Module struct {
	run  runner.Runner
	open handoff.Opener
	// wait is expect's `set timeout 20`: how long one wait for ssh-add's
	// output lasts.
	wait time.Duration
	// settle is how long an ssh-add has to exit once its terminal is
	// closed, before it is signaled.
	settle time.Duration
}

// New returns the module wired to the real system.
func New() *Module {
	return &Module{run: runner.Exec{}, open: handoff.OpenFD, wait: 20 * time.Second, settle: 5 * time.Second}
}

// Register binds gash_ssh_auto_unlock to t.
func Register(t *invoke.Table) { New().Register(t) }

// Register binds the function of m to t.
func (m *Module) Register(t *invoke.Table) {
	t.Handle("gash_ssh_auto_unlock", m.autoUnlock)
}

// session is one run: the child environment, which gets the agent
// variables as 2.x set them in the shell, and the variables to export.
type session struct {
	c    *invoke.Call
	env  []string
	vars map[string]string
}

// set is an assignment of the shell: later children see it, and the
// stub override exports it.
func (s *session) set(name, value string) {
	env := s.env[:0:0]
	for _, kv := range s.env {
		if !strings.HasPrefix(kv, name+"=") {
			env = append(env, kv)
		}
	}
	s.env = append(env, name+"="+value)
	s.vars[name] = value
}

// with is the child environment with extra NAME=value entries in front
// (`SSH_AUTH_SOCK="$sock" ssh-add -l`).
func (s *session) with(kv ...string) []string {
	return append(append([]string(nil), kv...), s.env...)
}

func (s *session) ctx() context.Context {
	if s.c.Ctx != nil {
		return s.c.Ctx
	}
	return context.Background()
}

// exports is the hand-off: quoted export lines of the agent variables
// that were set and, once the unlock ran, of the run-once marker.
func (s *session) exports(ran bool) string {
	var b strings.Builder
	for _, name := range []string{"SSH_AUTH_SOCK", "SSH_AGENT_PID"} {
		v, ok := s.vars[name]
		if !ok {
			continue
		}
		if q, err := syntax.Quote(v, syntax.LangBash); err == nil {
			b.WriteString("export " + name + "=" + q + "\n")
		}
	}
	if ran {
		b.WriteString("export GASH_SSH_AUTOUNLOCK_RAN=1\n")
	}
	return b.String()
}

// autoUnlock is gash_ssh_auto_unlock. The stub override evaluates the
// export lines the binary hands back (SSH_AUTH_SOCK, SSH_AGENT_PID,
// GASH_SSH_AUTOUNLOCK_RAN); a direct run does the work without them.
// Status 0 whatever happens, as 2.x.
func (m *Module) autoUnlock(c *invoke.Call) int {
	if help.NeedsOf(c, c.Arg(0)) {
		return 0
	}
	if c.Env.Get("GASH_SSH_AUTOUNLOCK_RAN") != "" {
		return 0
	}
	// First, so that no child (the agent daemonizes) inherits the capture.
	ch := handoff.Open(c.Env, m.open)
	s := &session{c: c, env: c.Env.Child(), vars: map[string]string{}}
	ran := m.unlock(c, s)
	ch.Send(s.exports(ran))
	return 0
}

// unlock is the body of 2.x gash_ssh_auto_unlock; it reports whether the
// run-once marker is set.
func (m *Module) unlock(c *invoke.Call, s *session) bool {
	p := ui.NewPrinter(c.Stderr, c.Term, false)
	// The settings file as the caller's shell has it; its warnings were
	// printed when the shell loaded it.
	keys := config.LoadCaller(c.Env.Get).SSHKeys
	if len(keys) == 0 {
		return true
	}
	if !m.found(s, "ssh-add") {
		p.Error("ssh-add is not available; cannot auto-unlock SSH keys.")
		return false
	}
	// ssh-add -l exits 2 when it reaches no agent, whatever it says: an
	// unset SSH_AUTH_SOCK or a socket a dead agent left (SYS-21).
	list, rc := m.list(s)
	if rc == 2 || strings.Contains(list, noAgent) {
		m.startAgent(s)
		list, rc = m.list(s)
		if rc == 2 || strings.Contains(list, noAgent) {
			p.Error("ssh-agent is not running and could not be started automatically.")
			return false
		}
	}
	// The missing-expect warning and its hint are unreachable (SYS-37).

	// Fingerprints already in the agent: those keys are skipped. The
	// entries are the parsed ones, never shell words (SYS-35).
	loaded := ""
	if strings.Contains(list, "SHA256:") || strings.Contains(list, "MD5:") {
		loaded = list
	}
	var todo []config.Entry
	for _, k := range keys {
		if loaded != "" && m.found(s, "ssh-keygen") {
			if fp := m.fingerprint(s, k.Name); fp != "" && strings.Contains(loaded, " "+fp+" ") {
				continue
			}
		}
		todo = append(todo, k)
	}
	if len(todo) == 0 {
		return true
	}
	printTranscript(p, m.addKeys(s, todo))
	return true
}

// found is `command -v NAME` on the caller's PATH.
func (m *Module) found(s *session, name string) bool {
	_, err := runner.LookPath(name, s.env, "")
	return err == nil
}

// list is `$(ssh-add -l 2>&1)` and its exit status: 0 keys, 1 no keys,
// 2 no agent.
func (m *Module) list(s *session) (string, int) {
	var out bytes.Buffer
	res, err := m.run.Run(s.ctx(), runner.Cmd{Path: "ssh-add", Args: []string{"-l"},
		Options: runner.Options{Env: s.env, Stdout: &out, Stderr: &out}})
	rc := res.ExitCode
	if se, ok := err.(*runner.StartError); ok {
		rc = se.Status
	}
	return runner.CommandSubst(out.Bytes()), rc
}

// fingerprint is the field 2 of `ssh-keygen -lf KEY.pub` (KEY when there
// is no .pub file), "" when ssh-keygen fails.
func (m *Module) fingerprint(s *session, key string) string {
	file := key
	if st, err := os.Stat(key + ".pub"); err == nil && st.Mode().IsRegular() {
		file = key + ".pub"
	}
	res, err := m.run.Run(s.ctx(), runner.Cmd{Path: "ssh-keygen", Args: []string{"-lf", file},
		Options: runner.Options{Env: s.env, Stderr: io.Discard}})
	if err != nil || res.ExitCode != 0 {
		return ""
	}
	fp := runner.CommandSubst(res.Stdout)
	if _, after, ok := strings.Cut(fp, " "); ok {
		fp = after
	}
	if before, _, ok := strings.Cut(fp, " "); ok {
		fp = before
	}
	return fp
}

// printTranscript is __gash_format_expect_output on `echo "$transcript"`:
// one SSH: line per non-empty line, the spawn lines left out, the level
// from the line's start.
func printTranscript(p ui.Printer, transcript string) {
	for _, line := range strings.Split(runner.CommandSubst([]byte(transcript)), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if line == "" || strings.HasPrefix(line, "spawn ssh-add") {
			continue
		}
		level := "info"
		switch {
		case strings.HasPrefix(line, "Agent pid"):
		case strings.HasPrefix(line, "Identity added:"):
			level = "success"
		case strings.HasPrefix(line, "The agent has no identities."):
			level = "warning"
		case strings.HasPrefix(line, "Bad passphrase"),
			strings.HasPrefix(line, noAgent+"."):
			level = "error"
		}
		p.SSH(line, level)
	}
}
