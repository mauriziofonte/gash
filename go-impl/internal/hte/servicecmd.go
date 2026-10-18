// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package hte

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/google/renameio/v2"

	"github.com/mauriziofonte/gash/internal/hte/child"
	"github.com/mauriziofonte/gash/internal/hte/hteerr"
	"github.com/mauriziofonte/gash/internal/hte/service"
	"github.com/mauriziofonte/gash/internal/hte/state"
)

// ServiceResult is the outcome of service start, stop, restart, reload.
type ServiceResult struct {
	Via     string   `json:"via"` // "direct" or "systemd"
	Applied *Applied `json:"applied,omitempty"`
	Running bool     `json:"running"`
}

// systemdUnit reports whether service commands go through the user unit:
// autostart systemd and the unit file in place.
func (e *Engine) systemdUnit(v *env, set state.Settings) bool {
	_, err := os.Stat(service.UnitPath(v.dirs.ConfigHome))
	return set.Autostart == service.AutostartSystemd && err == nil
}

// userEnv is the environment of systemctl --user: the base one plus what
// it needs to reach the user manager.
func (e *Engine) userEnv(v *env) []string {
	out := child.BaseEnv(v.dirs.Home)
	if v.dirs.RuntimeDir != "" {
		out = append(out, "XDG_RUNTIME_DIR="+v.dirs.RuntimeDir)
	}
	if bus := e.Getenv("DBUS_SESSION_BUS_ADDRESS"); bus != "" {
		out = append(out, "DBUS_SESSION_BUS_ADDRESS="+bus)
	}
	return out
}

func (e *Engine) systemctlUser(ctx context.Context, v *env, args ...string) error {
	bin := e.find("systemctl")
	if bin == "" {
		return hteerr.New(hteerr.PlatformUnsupported, "systemctl is not installed", "")
	}
	c := child.Cmd(bin, e.userEnv(v), append([]string{"--user"}, args...)...)
	c.Timeout = 3 * time.Minute
	res, err := child.Run(ctx, e.Runner, c)
	if err != nil {
		return err
	}
	if !res.Success() {
		return hteerr.Newf(hteerr.ReloadFailed, "run gash hte service logs", "systemctl --user %v: %s", args, child.Output(res))
	}
	return nil
}

// unitJob runs systemctl --user args from a command that holds the state
// lock. The lease names this process while the job runs, so the hidden
// __unit start and __unit stop that systemd runs meanwhile work under
// this command's lock instead of waiting for it (LEDGER HTE-73).
func (e *Engine) unitJob(ctx context.Context, v *env, args ...string) error {
	lease := v.paths.UnitLease()
	if err := renameio.WriteFile(lease, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o600); err != nil {
		return err
	}
	defer os.Remove(lease)
	return e.systemctlUser(ctx, v, args...)
}

// underLease reports whether a command that holds the state lock waits
// for this unit job: the lease names a live process and the lock is held.
// A lease left by a command that died does not count: its lock went with
// it.
func (e *Engine) underLease(v *env) bool {
	data, err := os.ReadFile(v.paths.UnitLease())
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid < 1 {
		return false
	}
	if err := syscall.Kill(pid, 0); err != nil && !errors.Is(err, syscall.EPERM) {
		return false
	}
	l, err := state.AcquireLock(v.paths.Lock(), 0, nil)
	if err != nil {
		return hteerr.CodeOf(err) == hteerr.LockTimeout
	}
	l.Unlock()
	return false
}

// unitLocked runs fn under the state lock, or under the lock of the
// command whose unit job this is.
func (e *Engine) unitLocked(v *env, command string, fn func() (any, error)) (any, error) {
	if e.underLease(v) {
		res, err := fn()
		e.audit(v, command, "", res, err)
		return res, err
	}
	return e.locked(v, command, "", fn)
}

// unitStart starts the stack through the user unit: a unit that stayed
// active while its masters ended is restarted. locked says whether the
// caller holds the state lock.
func (e *Engine) unitStart(ctx context.Context, v *env, locked bool) error {
	run := e.systemctlUser
	if locked {
		run = e.unitJob
	}
	if err := run(ctx, v, "start", service.UnitName); err != nil {
		return err
	}
	if all, err := e.supervisor(v).AllRunning(); err != nil || all {
		return err
	}
	return run(ctx, v, "restart", service.UnitName)
}

// startStack starts the stack from a command that holds the state lock:
// through systemctl --user when the systemd unit is in use, else directly
// (LEDGER HTE-73).
func (e *Engine) startStack(ctx context.Context, v *env, set state.Settings) error {
	if e.systemdUnit(v, set) {
		return e.unitStart(ctx, v, true)
	}
	return e.supervisor(v).Start(ctx)
}

// stopStack stops the stack from a command that holds the state lock:
// through systemctl --user when the systemd unit is in use, then any
// master still running (one the unit did not start) by its PID file.
func (e *Engine) stopStack(ctx context.Context, v *env, set state.Settings) error {
	if e.systemdUnit(v, set) {
		if err := e.unitJob(ctx, v, "stop", service.UnitName); err != nil {
			return err
		}
	}
	return e.supervisor(v).Stop(ctx)
}

// ServiceStart is service start: through systemctl --user when the unit
// is in use, else UnitStart.
func (e *Engine) ServiceStart(ctx context.Context) (*ServiceResult, error) {
	return e.serviceVia(ctx, "service start", "start", e.UnitStart)
}

// ServiceStop is service stop.
func (e *Engine) ServiceStop(ctx context.Context) (*ServiceResult, error) {
	return e.serviceVia(ctx, "service stop", "stop", e.UnitStop)
}

// ServiceRestart is service restart: stop, then start.
func (e *Engine) ServiceRestart(ctx context.Context) (*ServiceResult, error) {
	return e.serviceVia(ctx, "service restart", "restart", func(ctx context.Context) (*ServiceResult, error) {
		if _, err := e.UnitStop(ctx); err != nil {
			return nil, err
		}
		return e.UnitStart(ctx)
	})
}

func (e *Engine) serviceVia(ctx context.Context, command, verb string, direct func(context.Context) (*ServiceResult, error)) (*ServiceResult, error) {
	v, err := e.ready(command)
	if err != nil {
		return nil, err
	}
	set, err := e.installed(v)
	if err != nil {
		return nil, err
	}
	if !e.systemdUnit(v, set) {
		return direct(ctx)
	}
	if verb == "start" {
		err = e.unitStart(ctx, v, false)
	} else {
		err = e.systemctlUser(ctx, v, verb, service.UnitName)
	}
	if err != nil {
		return nil, err
	}
	running, err := e.supervisor(v).Running()
	return &ServiceResult{Via: "systemd", Running: running}, err
}

// UnitStart starts the stack directly (the hidden command the unit
// runs): render when the state changed, start every master, write the
// hosts block. Run by the unit for a command that holds the lock, it
// works under that command's lock.
func (e *Engine) UnitStart(ctx context.Context) (*ServiceResult, error) {
	v, err := e.ready("service start")
	if err != nil {
		return nil, err
	}
	set, err := e.installed(v)
	if err != nil {
		return nil, err
	}
	res, err := e.unitLocked(v, "service start", func() (any, error) {
		sites, err := e.sites(v)
		if err != nil {
			return nil, err
		}
		applied, err := e.apply(ctx, v, set, sites)
		if err != nil {
			return nil, err
		}
		if err := e.supervisor(v).Start(ctx); err != nil {
			return nil, err
		}
		return &ServiceResult{Via: "direct", Applied: &applied, Running: true}, nil
	})
	if err != nil {
		return nil, err
	}
	return res.(*ServiceResult), nil
}

// UnitStop stops the stack directly.
func (e *Engine) UnitStop(ctx context.Context) (*ServiceResult, error) {
	v, err := e.ready("service stop")
	if err != nil {
		return nil, err
	}
	res, err := e.unitLocked(v, "service stop", func() (any, error) {
		return &ServiceResult{Via: "direct"}, e.supervisor(v).Stop(ctx)
	})
	if err != nil {
		return nil, err
	}
	return res.(*ServiceResult), nil
}

// ServiceReload is service reload: render, test, swap, reload, roll back
// on failure.
func (e *Engine) ServiceReload(ctx context.Context) (*ServiceResult, error) {
	v, err := e.ready("service reload")
	if err != nil {
		return nil, err
	}
	set, err := e.installed(v)
	if err != nil {
		return nil, err
	}
	res, err := e.locked(v, "service reload", "", func() (any, error) {
		sites, err := e.sites(v)
		if err != nil {
			return nil, err
		}
		applied, err := e.apply(ctx, v, set, sites)
		if err != nil {
			return nil, err
		}
		// The same rendering (Apache overlays included): the masters
		// still read their files again, as a reload promises.
		if !applied.Changed {
			if applied.Reloaded, err = e.supervisor(v).Reload(ctx); err != nil {
				return nil, err
			}
		}
		running, err := e.supervisor(v).Running()
		return &ServiceResult{Via: "direct", Applied: &applied, Running: running}, err
	})
	if err != nil {
		return nil, err
	}
	return res.(*ServiceResult), nil
}

// ServiceStatus is service status, with every site probed.
func (e *Engine) ServiceStatus(ctx context.Context) (*service.Status, error) {
	v, err := e.ready("service status")
	if err != nil {
		return nil, err
	}
	st, err := e.supervisor(v).Status(ctx, true)
	return &st, err
}

// ServiceLogs is service logs: the last lines of the logs, of one site
// with site.
func (e *Engine) ServiceLogs(ctx context.Context, site string, lines int) ([]service.LogFile, error) {
	v, err := e.ready("service logs")
	if err != nil {
		return nil, err
	}
	if site != "" {
		s, err := v.store.LoadSite(site)
		if err != nil {
			return nil, err
		}
		site = s.Domain
	}
	_, m, _, err := e.supervisor(v).Current()
	if err != nil {
		return nil, err
	}
	return service.Logs(v.paths, m, site, lines)
}

// Autostart is what the first interactive shell runs when autostart is
// "shell": start the stack when it is down; nothing otherwise.
func (e *Engine) Autostart(ctx context.Context) error {
	v, err := e.ready("service start")
	if err != nil {
		return err
	}
	set, err := e.installed(v)
	if err != nil || set.Autostart != service.AutostartShell {
		return err
	}
	if running, err := e.supervisor(v).Running(); err != nil || running {
		return err
	}
	_, err = e.UnitStart(ctx)
	return err
}
