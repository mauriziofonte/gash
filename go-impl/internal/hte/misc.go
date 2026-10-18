// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package hte

import (
	"context"

	"github.com/mauriziofonte/gash/internal/hostshelper"
	"github.com/mauriziofonte/gash/internal/hte/certs"
	"github.com/mauriziofonte/gash/internal/hte/hosts"
	"github.com/mauriziofonte/gash/internal/hte/hteerr"
	"github.com/mauriziofonte/gash/internal/hte/state"
)

// CertShow is cert show.
func (e *Engine) CertShow(ctx context.Context, domain string) (*certs.Info, error) {
	v, err := e.ready("cert show")
	if err != nil {
		return nil, err
	}
	s, err := v.store.LoadSite(domain)
	if err != nil {
		return nil, err
	}
	info, err := e.certs(v).Show(s.Domain)
	return &info, err
}

// CertRenew is cert renew: a new pair, then a reload.
func (e *Engine) CertRenew(ctx context.Context, domain string) (*SiteResult, error) {
	if err := e.CheckIdentity("cert renew"); err != nil {
		return nil, err
	}
	v, err := e.resolve()
	if err != nil {
		return nil, err
	}
	set, err := e.installed(v)
	if err != nil {
		return nil, err
	}
	res, err := e.locked(v, "cert renew", domain, func() (any, error) {
		s, err := v.store.LoadSite(domain)
		if err != nil {
			return nil, err
		}
		if s.Kind != state.KindManaged || !s.HTTPS {
			return nil, hteerr.Newf(hteerr.ConflictingOptions, "", "%s has no gash certificate (kind %s, https %v)", s.Domain, s.Kind, s.HTTPS)
		}
		if err := e.certs(v).Issue(ctx, s); err != nil {
			return nil, err
		}
		sites, err := e.sites(v)
		if err != nil {
			return nil, err
		}
		// The rendering is the same (the paths did not change): a graceful
		// restart makes Apache read the new pair.
		applied, err := e.apply(ctx, v, set, sites)
		if err != nil {
			return nil, err
		}
		if !applied.Changed {
			if err := e.supervisor(v).Graceful(ctx); err != nil {
				return nil, err
			}
		}
		return &SiteResult{Site: &s, Changed: true, Applied: &applied, Warnings: []string{}}, nil
	})
	if err != nil {
		return nil, err
	}
	return res.(*SiteResult), nil
}

// HostsList is hosts list: the block gash wants.
func (e *Engine) HostsList(ctx context.Context) ([]hostshelper.Entry, error) {
	v, err := e.ready("hosts list")
	if err != nil {
		return nil, err
	}
	sites, err := e.sites(v)
	if err != nil {
		return nil, err
	}
	return desiredHosts(v, sites)
}

// HostsCheck is hosts check: the drift of /etc/hosts.
func (e *Engine) HostsCheck(ctx context.Context) (*hosts.Drift, error) {
	v, err := e.ready("hosts check")
	if err != nil {
		return nil, err
	}
	sites, err := e.sites(v)
	if err != nil {
		return nil, err
	}
	desired, err := desiredHosts(v, sites)
	if err != nil {
		return nil, err
	}
	d := hosts.Check(e.Sysroot, desired)
	return &d, nil
}

// HostsApply is hosts apply: write the block through the helper.
func (e *Engine) HostsApply(ctx context.Context) (*hosts.Drift, error) {
	v, err := e.ready("hosts apply")
	if err != nil {
		return nil, err
	}
	_, err = e.locked(v, "hosts apply", "", func() (any, error) {
		sites, err := e.sites(v)
		if err != nil {
			return nil, err
		}
		if !e.Sysroot.IsRegular(hosts.HelperPath) {
			return nil, hteerr.New(hteerr.PrivilegedStepRequired, hosts.HelperPath+" is not installed", "run gash hte install")
		}
		return nil, e.applyHosts(ctx, v, sites)
	})
	if err != nil {
		return nil, err
	}
	return e.HostsCheck(ctx)
}

// HostsWindows is hosts windows: on WSL, the lines to add to the Windows
// hosts file for the names Windows cannot resolve.
func (e *Engine) HostsWindows(ctx context.Context) ([]string, error) {
	v, err := e.ready("hosts windows")
	if err != nil {
		return nil, err
	}
	if !v.os.WSL {
		return nil, hteerr.New(hteerr.PlatformUnsupported, "hosts windows is for WSL", "")
	}
	sites, err := e.sites(v)
	if err != nil {
		return nil, err
	}
	desired, err := desiredHosts(v, sites)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, d := range desired {
		names = append(names, d.Names...)
	}
	if !e.Sysroot.IsRegular(hosts.PowerShell) {
		return hosts.WindowsLines(names), nil
	}
	missing, err := hosts.WindowsUnresolved(ctx, e.Runner, hosts.PowerShell, names)
	if err != nil {
		return nil, err
	}
	return hosts.WindowsLines(missing), nil
}
