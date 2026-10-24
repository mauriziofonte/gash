// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package system holds the functions of lib/modules/system.sh: disk
// usage, history search, the public IP, process and port tools, the
// service stopper, sudo_last, mkdir_cd and the WSL helpers.
package system

import (
	"context"
	"io"
	"net/http"
	"os"
	"syscall"
	"time"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/platform"
	"github.com/mauriziofonte/gash/internal/runner"
	"github.com/mauriziofonte/gash/internal/system/handoff"
	"github.com/mauriziofonte/gash/internal/ui"
)

// Module is the system handler set with its seams: tests replace the
// runner, the sysroot, the signal sender, the clock and the hand-off
// descriptor.
type Module struct {
	run  runner.Runner
	sys  platform.Sysroot
	kill func(pid int, sig syscall.Signal) error
	// sleep waits between two liveness polls of the kill functions.
	sleep func(time.Duration)
	// self is the binary's own PID, left out of every process list.
	self int
	// loc is the time zone of hgrep's timestamps (the caller's TZ).
	loc *time.Location
	// client makes ip_public's request.
	client *http.Client
	// ipURL is the address ip_public asks.
	ipURL string
	// ipTimeout bounds ip_public's request.
	ipTimeout time.Duration
	// open opens the hand-off descriptor of a stub override.
	open handoff.Opener
	// readable is [[ -r PATH ]].
	readable func(path string) bool
}

// New returns the module wired to the real system.
func New() *Module {
	return &Module{
		run:       runner.Exec{},
		sys:       platform.Real(),
		kill:      func(pid int, sig syscall.Signal) error { return syscall.Kill(pid, sig) },
		sleep:     time.Sleep,
		self:      os.Getpid(),
		loc:       time.Local,
		client:    http.DefaultClient,
		ipURL:     "https://ipinfo.io/ip",
		ipTimeout: ipTimeout,
		open:      handoff.OpenFD,
		readable:  readableOS,
	}
}

// Register binds the system functions to t.
func Register(t *invoke.Table) { New().Register(t) }

// Register binds the functions of m to t.
func (m *Module) Register(t *invoke.Table) {
	t.Handle("disk_usage", m.diskUsage)
	t.Handle("history_grep", m.historyGrep)
	t.Handle("hgrep", m.hgrep)
	t.Handle("ip_public", m.ipPublic)
	t.Handle("process_find", m.processFind)
	t.Handle("process_kill", m.processKill)
	t.Handle("port_kill", m.portKill)
	t.Handle("services_stop", m.servicesStop)
	t.Handle("sudo_last", m.sudoLast)
	t.Handle("mkdir_cd", m.mkdirCD)
	t.Handle("wsl_restart", m.wslRestart)
	t.Handle("wsl_shutdown", m.wslShutdown)
	t.Handle("wsl_explorer", m.wslExplorer)
	t.Handle("wsl_taskmanager", m.wslTaskmanager)
	t.Handle("wsl_compact", m.wslCompact)
}

// ctx is the context of a call (Background when the caller set none).
func ctx(c *invoke.Call) context.Context {
	if c.Ctx != nil {
		return c.Ctx
	}
	return context.Background()
}

// unattended reports a run where sudo cannot ask for a password:
// headless, or no controlling terminal.
func unattended(c *invoke.Call) bool {
	if c.Term.Headless() {
		return true
	}
	tty, err := c.Term.OpenTTY()
	if err != nil {
		return true
	}
	_ = tty.Close()
	return false
}

// sudoReady is the check before an unattended sudo (SYS-25): `sudo -n
// true` must pass, else the call is refused with one clear error rather
// than a password prompt nobody sees. On a terminal it is always ready;
// a sudo that does not start is left to the real call's message.
func (m *Module) sudoReady(c *invoke.Call, p ui.Printer) bool {
	if !unattended(c) {
		return true
	}
	res, err := m.run.Run(ctx(c), runner.Cmd{Path: "sudo", Args: []string{"-n", "true"},
		Options: runner.Options{Env: c.Env.Child(), Stdout: io.Discard, Stderr: io.Discard}})
	if _, ok := err.(*runner.StartError); ok || (err == nil && res.ExitCode == 0) {
		return true
	}
	p.Error("sudo needs a password: run interactively or configure NOPASSWD.")
	return false
}

// sudoArgv is the argv of `sudo ARGS`, `sudo -n ARGS` in an unattended
// run.
func sudoArgv(c *invoke.Call, args ...string) []string {
	if unattended(c) {
		return append([]string{"-n"}, args...)
	}
	return args
}

// found is `command -v NAME` for a program on the caller's PATH.
func found(c *invoke.Call, name string) bool {
	_, err := runner.LookPath(name, c.Env.Child(), "")
	return err == nil
}

// output runs argv with the caller's environment, stdin closed and stderr
// discarded, and returns its stdout as $(...) keeps it and its status.
func (m *Module) output(c *invoke.Call, argv ...string) (string, int) {
	res, err := m.run.Run(ctx(c), runner.Cmd{Path: argv[0], Args: argv[1:],
		Options: runner.Options{Env: c.Env.Child(), Stderr: io.Discard}})
	if err != nil && res.ExitCode == 0 {
		res.ExitCode = 1
	}
	return runner.CommandSubst(res.Stdout), res.ExitCode
}

// readableOS is the [[ -r PATH ]] test: access(2) with the effective IDs.
func readableOS(path string) bool {
	return faccess(path, 4)
}

// write writes s to w, ignoring errors as 2.x printf did.
func write(w io.Writer, s string) { _, _ = io.WriteString(w, s) }
