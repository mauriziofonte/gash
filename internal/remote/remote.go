// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package remote holds the remote module functions of 2.x
// (lib/modules/remote.sh): remote_hosts, remote_exec, remote_push,
// remote_pull, remote_sync_push and remote_sync_pull.
//
// ssh, scp and rsync are started through internal/runner with the
// caller's exported environment (c.Env.Child()) and the 2.x argv. Hosts
// come from the HOST: entries of ~/.gash_env; headless, only those are
// accepted. Remote paths go through the 2.x DENY/ASK/ALLOW tables on the
// path as written; a remote path the remote side could expand (globs,
// $, quotes, ...) is refused (LEDGER REM-03). Targets are strict
// user@host (REM-02, REM-08), unknown flags are refused (REM-01), and
// script arguments are quoted POSIX-style (REM-04).
package remote

import (
	"context"
	"io"
	"net"
	"regexp"
	"strings"

	"github.com/mauriziofonte/gash/internal/config"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/runner"
	"github.com/mauriziofonte/gash/internal/ui"
)

// Module is the remote handler set with its process seam.
type Module struct {
	// Run starts ssh, scp, rsync and cat.
	Run runner.Runner
}

// New returns the module wired to the real system.
func New() *Module { return &Module{Run: runner.Exec{}} }

// Register binds the remote functions to t.
func Register(t *invoke.Table) { New().Register(t) }

// Register binds the functions of m to t.
func (m *Module) Register(t *invoke.Table) {
	t.Handle("remote_hosts", m.hosts)
	t.Handle("remote_exec", m.exec)
	t.Handle("remote_push", m.push)
	t.Handle("remote_pull", m.pull)
	t.Handle("remote_sync_push", m.syncPush)
	t.Handle("remote_sync_pull", m.syncPull)
}

// call is one invocation with its style, printer and process helpers.
type call struct {
	*invoke.Call
	m *Module
	s ui.Style
	p ui.Printer
}

func (m *Module) begin(c *invoke.Call) *call {
	s := ui.StyleOf(c.Term, false)
	return &call{Call: c, m: m, s: s, p: s.Printer(c.Stderr)}
}

func (x *call) ctx() context.Context {
	if x.Ctx != nil {
		return x.Ctx
	}
	return context.Background()
}

// run starts NAME ARGS... with the caller's exported environment and the
// given streams (nil stdout captures). A program that cannot start gets
// Bash's status (127 or 126) without Bash's message (LEDGER DOCK-19).
func (x *call) run(stdin io.Reader, stdout, stderr io.Writer, name string, args ...string) runner.Result {
	res, _ := x.m.Run.Run(x.ctx(), runner.Cmd{Path: name, Args: args, Options: runner.Options{
		Env: x.Env.Child(), Stdin: stdin, Stdout: stdout, Stderr: stderr,
	}})
	return res
}

// home is the caller's $HOME.
func (x *call) home() string { return x.Env.Get("HOME") }

// headless is __gash_headless_gate_active.
func (x *call) headless() bool { return x.Env.Get("GASH_HEADLESS") == "1" }

// have is `command -v NAME >/dev/null`: a regular file NAME on the PATH,
// executable or not (Bash's command -v also names a file it cannot run).
func (x *call) have(name string) bool {
	_, err := runner.LookPath(name, x.Env.Child(), "")
	if err == nil {
		return true
	}
	se, ok := err.(*runner.StartError)
	return ok && se.Status == 126
}

// requireArg is __gash_require_arg VALUE NAME USAGE.
func (x *call) requireArg(value, name, usage string) bool {
	if value != "" {
		return true
	}
	x.p.Error("Missing " + name + ". Usage: " + usage)
	return false
}

// loadConfig is __gash_load_env: the caller's ~/.gash_env (or
// GASH_ENV_FILE). Its warnings were printed by the shell's own load.
func (x *call) loadConfig() *config.File { return config.LoadCaller(x.Env.Get) }

// resolveTarget is __remote_resolve_target: a registered alias, or,
// interactively, a raw user@host. Both must pass validTarget (LEDGER
// REM-02, REM-08): an alias whose HOST: target fails it is skipped with a
// warning, as an unregistered name.
func (x *call) resolveTarget(input string) (string, bool) {
	f := x.loadConfig()
	t, ok := f.HostTarget(input)
	if !ok {
		// The load skipped it (config.ValidHostTarget is validTarget's
		// rule); say why the alias is unknown.
		t, ok = f.RejectedHost(input)
	}
	if ok {
		if validTarget(t) {
			return t, true
		}
		x.p.Warning("HOST target of '" + input + "' must be user@host (skipping)")
		if strings.Contains(t, ":") {
			x.p.Hint("put the port in a Host entry of ~/.ssh/config")
		}
	}
	if x.headless() {
		x.p.Error("Unknown host alias '" + input + "'. In headless mode only aliases registered in ~/.gash_env (HOST:alias=user@host) are accepted.")
		return "", false
	}
	if validTarget(input) {
		return input, true
	}
	x.p.Error("Unknown host alias '" + input + "' and not a valid user@host target.")
	return "", false
}

var (
	reTargetUser = regexp.MustCompile(`^[A-Za-z0-9._][A-Za-z0-9._-]*$`)
	reTargetHost = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]*$`)
)

// validTarget is the user@host test of raw targets and HOST: entries: one
// @, a user of letters, digits, '.', '_' and '-' not starting with '-',
// and a host name of letters, digits, '.' and '-' starting with a letter
// or digit, or a bracketed IPv6 literal. So neither part reads as an ssh
// option and the host holds no ':' port that ssh, scp and the tier check
// would read differently (LEDGER REM-02, REM-08).
func validTarget(t string) bool {
	user, host, ok := strings.Cut(t, "@")
	if !ok || !reTargetUser.MatchString(user) {
		return false
	}
	if reTargetHost.MatchString(host) {
		return true
	}
	if len(host) > 2 && host[0] == '[' && host[len(host)-1] == ']' {
		ip := net.ParseIP(host[1 : len(host)-1])
		return ip != nil && strings.Contains(host, ":")
	}
	return false
}

// sshOpts are the ssh options of remote_exec, with the multiplexing
// socket under the caller's ~/.ssh.
func sshOpts(home string) []string {
	return []string{
		"-o", "ConnectTimeout=10",
		"-o", "BatchMode=yes",
		"-o", "ServerAliveInterval=15",
		"-o", "ServerAliveCountMax=4",
		"-o", "ControlMaster=auto",
		"-o", "ControlPath=" + home + "/.ssh/cm-%C",
		"-o", "ControlPersist=60s",
	}
}

// rsyncRSH is the -e string of the rsync functions. rsync splits it on
// spaces itself, honoring single and double quotes, where a doubled quote
// is a literal one (a backslash is not special), so the ControlPath
// option is single-quoted with every ' of HOME doubled: one word for any
// HOME (LEDGER REM-05).
func rsyncRSH(home string) string {
	return "ssh -o BatchMode=yes -o ConnectTimeout=10 -o ServerAliveInterval=15 -o ServerAliveCountMax=4 -o ControlMaster=auto -o '" +
		strings.ReplaceAll("ControlPath="+home+"/.ssh/cm-%C", "'", "''") + "' -o ControlPersist=60s"
}
