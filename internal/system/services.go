// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package system

import (
	"errors"
	"io"
	"os"
	"strings"

	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/prereq"
	"github.com/mauriziofonte/gash/internal/runner"
	"github.com/mauriziofonte/gash/internal/ui"
)

// knownServices are the services services_stop looks for, in order.
var knownServices = []string{"apache2", "nginx", "mysql", "mariadb", "postgresql", "mongodb", "redis", "redis-server", "memcached", "docker"}

// servicesStop is services_stop: systemctl when systemd runs (else the
// SysV service wrapper), the running ones of the fixed list, the question
// unless -y, then `sudo systemctl stop S` (or `sudo service S stop`) on
// the caller's terminal, plain sudo as 2.x (SYS-25).
func (m *Module) servicesStop(c *invoke.Call) int {
	if help.NeedsOf(c, c.Arg(0)) {
		return 0
	}
	p := ui.NewPrinter(c.Stderr, c.Term, false)
	assumeYes, dryRun := false, false
	for _, a := range c.Args {
		switch a {
		case "-y", "--yes", "-f", "--force":
			assumeYes = true
		case "--dry-run":
			dryRun = true
		default:
			p.Error("Unknown option: " + a + " (usage: services_stop [-y] [--dry-run])")
			return 1
		}
	}
	rundir := c.Env.Get("__GASH_SYSTEMD_RUNDIR")
	isDir := false
	if rundir == "" {
		isDir = m.sys.IsDir("/run/systemd/system")
	} else if st, err := os.Stat(rundir); err == nil && st.IsDir() {
		isDir = true
	}
	manager := ""
	switch {
	case isDir && found(c, "systemctl"):
		manager = "systemd"
	case found(c, "service"):
		manager = "service"
	default:
		p.Error("No service manager found (systemd or service).")
		return 1
	}
	// `systemctl is-active --quiet S 2>/dev/null`; `service S status
	// >/dev/null 2>&1`.
	var running []string
	for _, s := range knownServices {
		cmd := runner.Cmd{Path: "systemctl", Args: []string{"is-active", "--quiet", s},
			Options: runner.Options{Env: c.Env.Child(), Stdout: c.Stdout, Stderr: io.Discard}}
		if manager == "service" {
			cmd = runner.Cmd{Path: "service", Args: []string{s, "status"},
				Options: runner.Options{Env: c.Env.Child(), Stdout: io.Discard, Stderr: io.Discard}}
		}
		if res, err := m.run.Run(ctx(c), cmd); err == nil && res.ExitCode == 0 {
			running = append(running, s)
		}
	}
	if len(running) == 0 {
		p.Info("None of the known services is running.")
		return 0
	}
	p.Info("Running: " + strings.Join(running, " "))
	if dryRun {
		return 0
	}
	if !assumeYes && !ui.StyleOf(c.Term, false).Confirm(c.Term, c.Stderr, "Stop these services?") {
		p.Info("Aborted.")
		return 1
	}
	if !m.sudoReady(c, p, "services_stop") {
		return 1
	}
	rc := 0
	for _, s := range running {
		p.Info("Stopping " + s + "...")
		args := []string{"systemctl", "stop", s}
		if manager == "service" {
			args = []string{"service", s, "stop"}
		}
		res, err := m.run.Run(ctx(c), runner.Cmd{Path: "sudo", Args: sudoArgv(c, args...),
			Options: runner.Options{Env: c.Env.Child(), Stdin: c.Stdin, Stdout: c.Stdout, Stderr: c.Stderr}})
		switch se := (*runner.StartError)(nil); {
		case notInstalled(err):
			// No sudo: the report once, not one failure per service.
			return prereq.Fail(c, "sudo", "services_stop", false)
		case errors.As(err, &se):
			write(c.Stderr, "bash: "+se.Error()+"\n")
			rc = 1
		case err != nil || res.ExitCode != 0:
			rc = 1
		}
	}
	return rc
}

// passthrough runs a program on the caller's terminal (stdin, stdout and
// stderr as they are) and returns its status; a program that does not
// start prints the shell's message.
func (m *Module) passthrough(c *invoke.Call, name string, args ...string) int {
	res, err := m.run.Run(ctx(c), runner.Cmd{Path: name, Args: args,
		Options: runner.Options{Env: c.Env.Child(), Stdin: c.Stdin, Stdout: c.Stdout, Stderr: c.Stderr}})
	if err != nil {
		if se := (*runner.StartError)(nil); errors.As(err, &se) {
			write(c.Stderr, "bash: "+se.Error()+"\n")
			return se.Status
		}
		if res.ExitCode == 0 {
			return 1
		}
	}
	return res.ExitCode
}
