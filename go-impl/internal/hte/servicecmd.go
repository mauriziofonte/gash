// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package hte

import (
	"context"
	"os"
	"time"

	"github.com/mauriziofonte/gash/internal/hte/hteerr"
	"github.com/mauriziofonte/gash/internal/hte/proc"
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
	out := proc.BaseEnv(v.dirs.Home)
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
	res, err := e.Runner.Run(ctx, proc.Cmd{Path: bin, Args: append([]string{"--user"}, args...), Env: e.userEnv(v), Setsid: true,
		Timeout: 3 * time.Minute})
	if err != nil {
		return err
	}
	if !res.OK() {
		return hteerr.Newf(hteerr.ReloadFailed, "run gash hte service logs", "systemctl --user %v: %s", args, res.Output())
	}
	return nil
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
	if err := e.systemctlUser(ctx, v, verb, service.UnitName); err != nil {
		return nil, err
	}
	running, err := e.supervisor(v).Running()
	return &ServiceResult{Via: "systemd", Running: running}, err
}

// UnitStart starts the stack directly (the hidden command the unit
// runs): render when the state changed, start every master, write the
// hosts block.
func (e *Engine) UnitStart(ctx context.Context) (*ServiceResult, error) {
	v, err := e.ready("service start")
	if err != nil {
		return nil, err
	}
	set, err := e.installed(v)
	if err != nil {
		return nil, err
	}
	res, err := e.locked(v, "service start", "", func() (any, error) {
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
	res, err := e.locked(v, "service stop", "", func() (any, error) {
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
