// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package hte

import (
	"context"
	"maps"
	"os"
	"slices"

	"github.com/mauriziofonte/gash/internal/hte/certs"
	"github.com/mauriziofonte/gash/internal/hte/htaccess"
	"github.com/mauriziofonte/gash/internal/hte/hteerr"
	"github.com/mauriziofonte/gash/internal/hte/state"
)

// SiteResult is the outcome of a site command.
type SiteResult struct {
	Site     *state.Site `json:"site,omitempty"`
	Changed  bool        `json:"changed"`
	Applied  *Applied    `json:"applied,omitempty"`
	Warnings []string    `json:"warnings"`
}

func (e *Engine) siteEnv(v *env, set state.Settings, installed []string) state.Env {
	return state.Env{Cwd: e.Cwd, Installed: installed, DefaultPHP: set.DefaultPHP, Ports: set.Ports}
}

// htaccessWarnings are the .htaccess HTTPS redirect warnings of a site
// (design/hte.md 3).
func htaccessWarnings(s state.Site) []string {
	out := []string{}
	if s.Kind != state.KindManaged || s.Docroot == "" {
		return out
	}
	found, err := htaccess.Check(s.Docroot, os.ReadFile)
	if err != nil {
		return out
	}
	for _, r := range found {
		out = append(out, r.Warning())
	}
	return out
}

// mutate is the frame of every site change: identity, install check,
// lock, then fn computes the next site list; the state is saved and
// applied only when it changed. A failed apply puts the previous state
// files back, so state and rendering never disagree.
func (e *Engine) mutate(ctx context.Context, command, site string, fn func(v *env, set state.Settings, sites []state.Site) ([]state.Site, *state.Site, bool, error)) (*SiteResult, error) {
	if err := e.CheckIdentity(command); err != nil {
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
	res, err := e.locked(v, command, site, func() (any, error) {
		sites, err := e.sites(v)
		if err != nil {
			return nil, err
		}
		next, s, changed, err := fn(v, set, sites)
		if err != nil {
			return nil, err
		}
		out := &SiteResult{Site: s, Changed: changed, Warnings: []string{}}
		if s != nil {
			out.Warnings = htaccessWarnings(*s)
		}
		if !changed {
			return out, nil
		}
		if err := e.saveSites(v, sites, next); err != nil {
			return nil, err
		}
		applied, err := e.apply(ctx, v, set, next)
		if err != nil {
			if rerr := e.saveSites(v, next, sites); rerr != nil {
				return nil, hteerr.Newf(hteerr.InvalidState, "", "%v; putting the previous state back failed: %v", err, rerr)
			}
			return nil, err
		}
		out.Applied = &applied
		return out, nil
	})
	if err != nil {
		return nil, err
	}
	return res.(*SiteResult), nil
}

// saveSites writes the files of to and removes the sites of from that to
// no longer holds.
func (e *Engine) saveSites(v *env, from, to []state.Site) error {
	for _, s := range to {
		i := slices.IndexFunc(from, func(f state.Site) bool { return f.Domain == s.Domain })
		if i >= 0 && state.Equal(from[i], s) {
			continue
		}
		if err := v.store.SaveSite(s); err != nil {
			return err
		}
	}
	for _, f := range from {
		if !slices.ContainsFunc(to, func(s state.Site) bool { return s.Domain == f.Domain }) {
			if err := v.store.RemoveSite(f.Domain); err != nil {
				return err
			}
		}
	}
	return nil
}

func (e *Engine) installedPHP(ctx context.Context, v *env) []string {
	return e.discoverPHP(ctx, v).Installed()
}

// SiteCreate is site create.
func (e *Engine) SiteCreate(ctx context.Context, f state.Flags) (*SiteResult, error) {
	return e.mutate(ctx, "site create", f.Domain, func(v *env, set state.Settings, sites []state.Site) ([]state.Site, *state.Site, bool, error) {
		s, err := state.NewSite(f, e.siteEnv(v, set, e.installedPHP(ctx, v)), sites)
		if err != nil {
			return nil, nil, false, err
		}
		return append(slices.Clone(sites), s), &s, true, nil
	})
}

// SiteModify is site modify; with no effective change it changes nothing
// and reports changed=false (LEDGER HTE-12, option a).
func (e *Engine) SiteModify(ctx context.Context, f state.Flags) (*SiteResult, error) {
	return e.mutate(ctx, "site modify", f.Domain, func(v *env, set state.Settings, sites []state.Site) ([]state.Site, *state.Site, bool, error) {
		cur, err := v.store.LoadSite(f.Domain)
		if err != nil {
			return nil, nil, false, err
		}
		next, changed, err := state.ModifySite(cur, f, e.siteEnv(v, set, e.installedPHP(ctx, v)), sites)
		if err != nil {
			return nil, nil, false, err
		}
		if !slices.Equal(sortedNames(cur.Aliases), sortedNames(next.Aliases)) {
			v.reissue = append(v.reissue, next.Domain)
		}
		return replace(sites, next), &next, changed, nil
	})
}

// sortedNames is a sorted copy of names.
func sortedNames(names []string) []string {
	out := slices.Clone(names)
	slices.Sort(out)
	return out
}

func replace(sites []state.Site, s state.Site) []state.Site {
	out := slices.Clone(sites)
	for i := range out {
		if out[i].Domain == s.Domain {
			out[i] = s
		}
	}
	return out
}

// SiteRemove is site remove: the state, rendered files, pool, certificate
// and hosts entries of exactly that site; never the document root, never
// its overlay. Without yes it is confirmation_required.
func (e *Engine) SiteRemove(ctx context.Context, domain string, yes bool) (*SiteResult, error) {
	if !yes {
		if err := e.CheckIdentity("site remove"); err != nil {
			return nil, err
		}
		return nil, hteerr.Newf(hteerr.ConfirmationRequired, "add --yes", "removing %s needs --yes", domain)
	}
	var removed state.Site
	res, err := e.mutate(ctx, "site remove", domain, func(v *env, set state.Settings, sites []state.Site) ([]state.Site, *state.Site, bool, error) {
		cur, err := v.store.LoadSite(domain)
		if err != nil {
			return nil, nil, false, err
		}
		removed = cur
		return slices.DeleteFunc(slices.Clone(sites), func(s state.Site) bool { return s.Domain == cur.Domain }), &cur, true, nil
	})
	if err != nil {
		return nil, err
	}
	v, err := e.resolve()
	if err != nil {
		return nil, err
	}
	if err := e.certs(v).Remove(removed.Domain); err != nil {
		res.Warnings = append(res.Warnings, "the certificate directory was not removed: "+err.Error())
	}
	return res, nil
}

// SiteEnable and SiteDisable switch a site on and off.
func (e *Engine) SiteEnable(ctx context.Context, domain string) (*SiteResult, error) {
	return e.setEnabled(ctx, "site enable", domain, true)
}

// SiteDisable is site disable: the site stops being served; its state,
// certificate and overlay stay.
func (e *Engine) SiteDisable(ctx context.Context, domain string) (*SiteResult, error) {
	return e.setEnabled(ctx, "site disable", domain, false)
}

func (e *Engine) setEnabled(ctx context.Context, command, domain string, on bool) (*SiteResult, error) {
	return e.mutate(ctx, command, domain, func(v *env, set state.Settings, sites []state.Site) ([]state.Site, *state.Site, bool, error) {
		cur, err := v.store.LoadSite(domain)
		if err != nil {
			return nil, nil, false, err
		}
		next := cur
		next.Enabled = on
		return replace(sites, next), &next, cur.Enabled != on, nil
	})
}

// SiteList is site list: the sites and the files that do not load.
type SiteList struct {
	Sites    []state.Site `json:"sites"`
	Problems []string     `json:"problems"`
}

// SiteList reads every site.
func (e *Engine) SiteList(ctx context.Context) (*SiteList, error) {
	v, err := e.ready("site list")
	if err != nil {
		return nil, err
	}
	sites, problems, err := v.store.LoadSites()
	if err != nil {
		return nil, err
	}
	out := &SiteList{Sites: append([]state.Site{}, sites...), Problems: []string{}}
	for _, p := range problems {
		out.Problems = append(out.Problems, p.Error())
	}
	return out, nil
}

// SiteShow is site show: the state, the certificate and the warnings.
type SiteShow struct {
	Site        state.Site  `json:"site"`
	Certificate *certs.Info `json:"certificate,omitempty"`
	Warnings    []string    `json:"warnings"`
}

// SiteShow reads one site.
func (e *Engine) SiteShow(ctx context.Context, domain string) (*SiteShow, error) {
	v, err := e.ready("site show")
	if err != nil {
		return nil, err
	}
	s, err := v.store.LoadSite(domain)
	if err != nil {
		return nil, err
	}
	out := &SiteShow{Site: s, Warnings: htaccessWarnings(s)}
	if info, err := e.certs(v).Show(s.Domain); err == nil {
		out.Certificate = &info
	}
	if s.PHPIni != nil {
		out.Site.PHPIni = maps.Clone(s.PHPIni)
	}
	return out, nil
}

// ready is the frame of a read-only command: identity and install check.
func (e *Engine) ready(command string) (*env, error) {
	if err := e.CheckIdentity(command); err != nil {
		return nil, err
	}
	v, err := e.resolve()
	if err != nil {
		return nil, err
	}
	if _, err := e.installed(v); err != nil {
		return nil, err
	}
	return v, nil
}
