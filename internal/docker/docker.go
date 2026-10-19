// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package docker holds the docker and docker-compose module functions of
// 2.x (lib/modules/docker.sh, lib/modules/docker-compose.sh):
// docker_stop_all, docker_start_all, docker_prune_all,
// docker_compose_check, docker_compose_upgrade and docker_compose_scan.
//
// docker, docker compose and docker-compose are started through
// internal/runner with the caller's exported environment (c.Env.Child())
// and the 2.x argv, never through a shell. Registry digests are asked
// over net/http with the 2.x protocol (registry.go). The 2.x behaviors and
// defects are kept as the LEDGER DOCK items say; the forced divergences
// are DOCK-02 (paths resolved natively, GNU realpath -m semantics), DOCK-08
// (one struct per service), DOCK-11 (nothing is left in the caller's
// shell), DOCK-12 (interpolation from DIR/.env and the exported
// environment, literal replacement), DOCK-19 (no Bash diagnostics),
// DOCK-23 to DOCK-25 (net/http instead of curl), DOCK-26 (compose v2
// config parsed with encoding/json) and DOCK-27 (byte order).
package docker

import (
	"context"
	"io"
	"net/http"
	"os"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/runner"
	"github.com/mauriziofonte/gash/internal/ui"
)

// Module is the docker handler set with its process and network seams.
type Module struct {
	// Run starts docker and docker-compose.
	Run runner.Runner
	// HTTP is the client of the registry calls; nil means one with the
	// proxies and CAs of the environment and the 2.x timeouts.
	HTTP *http.Client
}

// New returns the module wired to the real system.
func New() *Module { return &Module{Run: runner.Exec{}} }

// Register binds the docker functions to t.
func Register(t *invoke.Table) { New().Register(t) }

// Register binds the functions of m to t.
func (m *Module) Register(t *invoke.Table) {
	t.Handle("docker_stop_all", m.stopAll)
	t.Handle("docker_start_all", m.startAll)
	t.Handle("docker_prune_all", m.pruneAll)
	t.Handle("docker_compose_check", m.check)
	t.Handle("docker_compose_upgrade", m.upgrade)
	t.Handle("docker_compose_scan", m.scan)
}

// call is one invocation with its style, printer and process helpers.
type call struct {
	*invoke.Call
	m *Module
	s ui.Style
	p ui.Printer
	// exported is the caller's exported environment (what ${!VAR} and
	// ${GITHUB_TOKEN} can see from a binary, LEDGER DOCK-12).
	exported invoke.Env
}

func (m *Module) begin(c *invoke.Call) *call {
	s := ui.StyleOf(c.Term, false)
	return &call{Call: c, m: m, s: s, p: s.Printer(c.Stderr), exported: invoke.EnvFrom(c.Env.Child())}
}

// colors is __gash_want_colors NO_COLOR || eval "$__GASH_COLOR_OFF_LOCALS":
// from here on the call prints with the palette off when noColor is set.
// Messages printed before keep the caller's palette, as in 2.x.
func (x *call) colors(noColor bool) {
	x.s = ui.StyleOf(x.Term, noColor)
	x.p = x.s.Printer(x.Stderr)
}

func (x *call) ctx() context.Context {
	if x.Ctx != nil {
		return x.Ctx
	}
	return context.Background()
}

// run starts NAME ARGS... with the caller's exported environment, the
// caller's stdin and the given output streams (nil captures). A program
// that cannot start gets Bash's status (127 or 126) without Bash's
// message (LEDGER DOCK-19).
func (x *call) run(stdout, stderr io.Writer, name string, args ...string) runner.Result {
	return x.runIn(x.Stdin, stdout, stderr, name, args...)
}

// runIn is run with another stdin.
func (x *call) runIn(stdin io.Reader, stdout, stderr io.Writer, name string, args ...string) runner.Result {
	res, _ := x.m.Run.Run(x.ctx(), runner.Cmd{Path: name, Args: args, Options: runner.Options{
		Env: x.Env.Child(), Stdin: stdin, Stdout: stdout, Stderr: stderr,
	}})
	return res
}

// subst is VAR=$(NAME ARGS... 2>/dev/null): the captured output without
// its trailing newlines, and the status.
func (x *call) subst(name string, args ...string) (string, int) {
	res := x.run(nil, io.Discard, name, args...)
	return runner.CommandSubst(res.Stdout), res.ExitCode
}

// have is `command -v NAME` and `type -P NAME`: a file NAME on the PATH,
// executable or not (both name a file Bash could not run, too). Shell
// functions and aliases of the caller are out of a binary's sight.
func (x *call) have(name string) bool {
	_, err := runner.LookPath(name, x.Env.Child(), "")
	if err == nil {
		return true
	}
	se, ok := err.(*runner.StartError)
	return ok && se.Status == 126
}

// requireDocker is __gash_require_docker.
func (x *call) requireDocker() bool {
	if x.have("docker") {
		return true
	}
	x.p.Error("Docker is not installed.")
	return false
}

// headless is GASH_HEADLESS=1.
func (x *call) headless() bool { return x.Env.Get("GASH_HEADLESS") == "1" }

// pwd is the caller's $PWD (the fallback of a path realpath -m refuses),
// the working directory of the binary when the caller has none.
func (x *call) pwd() string {
	if p := x.Env.Get("PWD"); p != "" {
		return p
	}
	wd, _ := os.Getwd()
	return wd
}
