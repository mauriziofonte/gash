// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package state

import (
	"io/fs"
	"maps"
	"slices"
	"strings"

	"github.com/mauriziofonte/gash/internal/hostshelper"
	"github.com/mauriziofonte/gash/internal/hte/hteerr"
	"github.com/mauriziofonte/gash/internal/hte/profiles"
)

// Kind is how a site's vhost is produced.
type Kind string

// The two kinds of design/hte.md section 2.
const (
	KindManaged Kind = "managed" // rendered from this state
	KindRaw     Kind = "raw"     // an imported vhost kept verbatim in RawConf
)

// Site is the state of one site, sites/<domain>.toml. The domain is the
// identity and never changes.
type Site struct {
	Domain                   string            `toml:"domain" json:"domain"`
	Aliases                  []string          `toml:"aliases" json:"aliases"`
	Docroot                  string            `toml:"docroot" json:"docroot"`
	PHP                      string            `toml:"php" json:"php"`
	Profile                  string            `toml:"profile" json:"profile"`
	HTTPS                    bool              `toml:"https" json:"https"`
	ForceHTTPS               bool              `toml:"force_https" json:"force_https"`
	ForceHTTPSBypassLoopback bool              `toml:"force_https_bypass_loopback" json:"force_https_bypass_loopback"`
	HTTP2                    bool              `toml:"http2" json:"http2"`
	Enabled                  bool              `toml:"enabled" json:"enabled"`
	Kind                     Kind              `toml:"kind" json:"kind"`
	RawConf                  string            `toml:"raw_conf" json:"raw_conf"`
	PHPIni                   map[string]string `toml:"php_ini,omitempty" json:"php_ini,omitempty"`
}

// Names returns the domain followed by the aliases: every name the site
// serves (ServerName, ServerAlias, the hosts block, the certificate SAN).
func (s Site) Names() []string { return append([]string{s.Domain}, s.Aliases...) }

// HasPool reports whether the site gets a PHP-FPM pool.
func (s Site) HasPool() bool { return s.PHP != "" }

// Env carries the machine facts site validation needs.
type Env struct {
	Cwd        string                            // a relative --docroot resolves against it
	Installed  []string                          // discovered PHP-FPM versions ("8.3")
	DefaultPHP string                            // default_php of hte.toml, "" when unset
	Stat       func(string) (fs.FileInfo, error) // os.Stat when nil
}

// Flags are the site options of `site create` and `site modify`. A nil
// pointer is an absent flag. Aliases and NoWWW describe the alias set: on
// modify, giving either recomputes it as create does.
type Flags struct {
	Domain           string
	Docroot          *string
	PHP              *string
	Profile          *string
	HTTPS            *bool
	ForceHTTPS       *bool
	ForceHTTPSBypass *bool
	HTTP2            *bool
	Aliases          []string
	NoWWW            bool
}

func (f Flags) aliasesGiven() bool { return len(f.Aliases) > 0 || f.NoWWW }

func pick(flag *bool, current bool) bool {
	if flag != nil {
		return *flag
	}
	return current
}

// checkHTTPS refuses force-HTTPS on a site without HTTPS (LEDGER HTE-01).
func checkHTTPS(https, force bool, f Flags) error {
	if !https && force {
		hint := "drop --force-https, or add --https"
		if f.ForceHTTPS == nil {
			hint = "the site forces HTTPS: add --no-force-https"
		}
		return hteerr.New(hteerr.ConflictingOptions, "force-HTTPS needs HTTPS", hint)
	}
	return nil
}

// NewSite validates the flags of `site create` and returns the new state.
// Every value is checked before anything is written, in argument order:
// domain, docroot, php, profile, the HTTPS pair, aliases, then the names
// already served by existing sites (LEDGER HTE-02). Absent flags take the
// defaults of LEDGER HTE-55 (https on, force-https off, bypass off, http2
// on, profile dev, PHP from hte.toml else the highest installed).
func NewSite(f Flags, env Env, existing []Site) (Site, error) {
	domain, err := NormalizeDomain(f.Domain)
	if err != nil {
		return Site{}, err
	}
	if f.Docroot == nil {
		return Site{}, hteerr.New(hteerr.MissingValue, "a document root is required", "pass --docroot PATH")
	}
	docroot, err := ResolveDocroot(*f.Docroot, env.Cwd, env.Stat)
	if err != nil {
		return Site{}, err
	}
	php, err := ResolvePHP(f.PHP, env)
	if err != nil {
		return Site{}, err
	}
	profile, err := ParseProfile(f.Profile)
	if err != nil {
		return Site{}, err
	}
	https, force := pick(f.HTTPS, true), pick(f.ForceHTTPS, false)
	if err := checkHTTPS(https, force, f); err != nil {
		return Site{}, err
	}
	aliases, err := BuildAliases(domain, f.NoWWW, f.Aliases)
	if err != nil {
		return Site{}, err
	}
	s := Site{
		Domain:                   domain,
		Aliases:                  aliases,
		Docroot:                  docroot,
		PHP:                      php,
		Profile:                  string(profile),
		HTTPS:                    https,
		ForceHTTPS:               force,
		ForceHTTPSBypassLoopback: pick(f.ForceHTTPSBypass, false),
		HTTP2:                    pick(f.HTTP2, true),
		Enabled:                  true,
		Kind:                     KindManaged,
	}
	if err := CheckUnique(s, existing); err != nil {
		return Site{}, err
	}
	return s, nil
}

// ModifySite applies the flags of `site modify` to cur. Absent flags keep
// the current values; changed reports whether anything differs.
func ModifySite(cur Site, f Flags, env Env, existing []Site) (next Site, changed bool, err error) {
	domain, err := NormalizeDomain(f.Domain)
	if err != nil {
		return Site{}, false, err
	}
	if domain != cur.Domain {
		return Site{}, false, hteerr.Newf(hteerr.SiteNotFound, "gash hte site list", "site %s is not %s", cur.Domain, domain)
	}
	next = cur
	next.Aliases = slices.Clone(cur.Aliases)
	next.PHPIni = maps.Clone(cur.PHPIni)
	if f.Docroot != nil {
		if next.Docroot, err = ResolveDocroot(*f.Docroot, env.Cwd, env.Stat); err != nil {
			return Site{}, false, err
		}
	}
	if f.PHP != nil {
		if next.PHP, err = ResolvePHP(f.PHP, env); err != nil {
			return Site{}, false, err
		}
	}
	if f.Profile != nil {
		p, err := ParseProfile(f.Profile)
		if err != nil {
			return Site{}, false, err
		}
		next.Profile = string(p)
	}
	next.HTTPS = pick(f.HTTPS, cur.HTTPS)
	next.ForceHTTPS = pick(f.ForceHTTPS, cur.ForceHTTPS)
	next.ForceHTTPSBypassLoopback = pick(f.ForceHTTPSBypass, cur.ForceHTTPSBypassLoopback)
	next.HTTP2 = pick(f.HTTP2, cur.HTTP2)
	if err := checkHTTPS(next.HTTPS, next.ForceHTTPS, f); err != nil {
		return Site{}, false, err
	}
	if f.aliasesGiven() {
		if next.Aliases, err = BuildAliases(domain, f.NoWWW, f.Aliases); err != nil {
			return Site{}, false, err
		}
	}
	if err := CheckUnique(next, without(existing, cur.Domain)); err != nil {
		return Site{}, false, err
	}
	return next, !Equal(cur, next), nil
}

// Equal reports whether two states are the same.
func Equal(a, b Site) bool {
	return a.Domain == b.Domain && slices.Equal(a.Aliases, b.Aliases) && a.Docroot == b.Docroot &&
		a.PHP == b.PHP && a.Profile == b.Profile && a.HTTPS == b.HTTPS && a.ForceHTTPS == b.ForceHTTPS &&
		a.ForceHTTPSBypassLoopback == b.ForceHTTPSBypassLoopback && a.HTTP2 == b.HTTP2 &&
		a.Enabled == b.Enabled && a.Kind == b.Kind && a.RawConf == b.RawConf && maps.Equal(a.PHPIni, b.PHPIni)
}

// CheckUnique fails with site_exists when s is one of others (same
// domain), or when a name s serves is served by one of them. Names are
// stored lowercased, so App.test and app.test are the same site (LEDGER
// HTE-20). On modify, others leaves out the site being modified.
func CheckUnique(s Site, others []Site) error {
	for _, o := range others {
		if o.Domain == s.Domain {
			return hteerr.Newf(hteerr.SiteExists, "change it with gash hte site modify --domain "+s.Domain, "site %s already exists", s.Domain)
		}
		for _, n := range s.Names() {
			if slices.Contains(o.Names(), n) {
				return hteerr.Newf(hteerr.SiteExists, "", "%s is already served by site %s", n, o.Domain)
			}
		}
	}
	return nil
}

// without returns sites minus the one named domain.
func without(sites []Site, domain string) []Site {
	return slices.DeleteFunc(slices.Clone(sites), func(o Site) bool { return o.Domain == domain })
}

// Validate checks a stored state with the rules of create, except the
// ones that need the machine (docroot existence, installed PHP versions).
func (s Site) Validate() error {
	if !hostshelper.ValidHostname(s.Domain) {
		return hteerr.Newf(hteerr.InvalidDomain, hintDomain, "domain %q is not a valid site name", s.Domain)
	}
	seen := map[string]bool{s.Domain: true}
	for _, a := range s.Aliases {
		if !hostshelper.ValidHostname(a) {
			return hteerr.Newf(hteerr.InvalidDomain, hintDomain, "alias %q is not a valid site name", a)
		}
		if seen[a] {
			return hteerr.Newf(hteerr.InvalidDomain, "", "name %s is listed twice", a)
		}
		seen[a] = true
	}
	switch s.Kind {
	case KindManaged:
		if s.RawConf != "" {
			return hteerr.New(hteerr.InvalidState, "raw_conf is set on a managed site", "")
		}
		if err := checkStoredDocroot(s.Docroot); err != nil {
			return err
		}
		if !phpVersionRE.MatchString(s.PHP) {
			return hteerr.Newf(hteerr.PHPVersionNotInstalled, "", "php %q is not a PHP version", s.PHP)
		}
	case KindRaw:
		if strings.TrimSpace(s.RawConf) == "" {
			return hteerr.New(hteerr.InvalidState, "raw_conf is empty on a raw site", "")
		}
		if s.Docroot != "" {
			if err := checkStoredDocroot(s.Docroot); err != nil {
				return err
			}
		}
		if s.PHP != "" && !phpVersionRE.MatchString(s.PHP) {
			return hteerr.Newf(hteerr.PHPVersionNotInstalled, "", "php %q is not a PHP version", s.PHP)
		}
	default:
		return hteerr.Newf(hteerr.InvalidState, "", "kind %q is not managed or raw", s.Kind)
	}
	if s.HasPool() {
		if s.Docroot == "" {
			return hteerr.New(hteerr.InvalidState, "a site with a PHP pool has no docroot", "")
		}
		if _, ok := profiles.Parse(s.Profile); !ok {
			return hteerr.Newf(hteerr.InvalidProfile, "profiles: dev, staging, hardened", "unknown profile %q", s.Profile)
		}
	}
	if !s.HTTPS && s.ForceHTTPS {
		return hteerr.New(hteerr.ConflictingOptions, "force_https is true while https is false", "")
	}
	for _, k := range slices.Sorted(maps.Keys(s.PHPIni)) {
		if err := CheckINI(k, s.PHPIni[k]); err != nil {
			return err
		}
	}
	return nil
}
