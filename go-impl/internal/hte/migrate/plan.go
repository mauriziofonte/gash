// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package migrate builds the plan of `gash hte migrate` (design/hte.md
// section 10) from what package legacy read: which vhosts become managed
// or raw sites, with which state, pool profile and certificate, which
// root steps --apply would run, and everything hte-cli never noticed
// (duplicates, shared sockets, custom ports, orphans). Building a plan
// reads only; there is no apply step here.
package migrate

import (
	"io/fs"
	"net/netip"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/mauriziofonte/gash/internal/hostshelper"
	"github.com/mauriziofonte/gash/internal/hte/hosts"
	"github.com/mauriziofonte/gash/internal/hte/htaccess"
	"github.com/mauriziofonte/gash/internal/hte/hteerr"
	"github.com/mauriziofonte/gash/internal/hte/legacy"
	"github.com/mauriziofonte/gash/internal/hte/profiles"
	"github.com/mauriziofonte/gash/internal/hte/state"
	"github.com/mauriziofonte/gash/internal/platform"
)

// Options are the flags of `gash hte migrate`.
type Options struct {
	Sites []string // --site D: import only these domains
	// Exclude are the --exclude values: a domain (every vhost serving it)
	// or an absolute vhost file path as the plan lists it (that file only,
	// whatever its class). Nothing is excluded implicitly.
	Exclude        []string
	KeepPoolValues bool // --keep-pool-values: pool differences go to [php_ini]
	ReissueCerts   bool // --reissue-certs: new pairs instead of copying the legacy ones
}

// Env is what the plan is built from.
type Env struct {
	Sysroot   platform.Sysroot
	OS        platform.OS
	Layout    legacy.Layout
	User      string // the invoking user's name
	Paths     state.Paths
	Ports     state.Ports
	Listen    []string
	Installed []string     // discovered PHP-FPM versions
	Existing  []state.Site // sites already managed (migrate is idempotent)
	// HostsExtra are the names of hosts.toml (an earlier migrate kept them).
	HostsExtra []string
	// Stat checks document roots; nil means os.Stat.
	Stat func(string) (fs.FileInfo, error)
	// ReadFile reads a document root's .htaccess; nil means os.ReadFile.
	ReadFile func(string) ([]byte, error)
}

// Action is what --apply would do with a vhost.
type Action string

// The actions of a site plan.
const (
	ActionImport    Action = "import"          // a managed site
	ActionImportRaw Action = "import_raw"      // kind = raw
	ActionSkip      Action = "skip"            // stays in the system, reported
	ActionExcluded  Action = "excluded"        // --exclude: never imported, no longer served once the system Apache stops
	ActionUnchosen  Action = "not_selected"    // left out by --site: not imported in this run
	ActionManaged   Action = "already_managed" // a site of that name exists
)

// SitePlan is the outcome of one vhost file.
type SitePlan struct {
	Path     string       `json:"path"`
	Enabled  bool         `json:"enabled"`
	Class    legacy.Class `json:"class"`
	Action   Action       `json:"action"`
	Reason   string       `json:"reason,omitempty"`
	Code     hteerr.Code  `json:"code,omitempty"` // the validation error that made it a skip
	Domain   string       `json:"domain,omitempty"`
	Site     *state.Site  `json:"site,omitempty"` // the state --apply writes
	Pool     *PoolPlan    `json:"pool,omitempty"`
	Cert     *CertPlan    `json:"certificate,omitempty"`
	Notes    []string     `json:"notes,omitempty"`
	Warnings []string     `json:"warnings,omitempty"`
}

// PoolPlan is the legacy pool of an imported site.
type PoolPlan struct {
	Path    string               `json:"path"`
	Found   bool                 `json:"found"`
	Profile profiles.Name        `json:"profile"`
	Source  legacy.ProfileSource `json:"source"`
	Diffs   []legacy.PoolDiff    `json:"diffs,omitempty"`
	Kept    []string             `json:"kept,omitempty"`     // php_ini keys taken with --keep-pool-values
	NotKept []string             `json:"not_kept,omitempty"` // differences that have no php_ini form
	User    string               `json:"user,omitempty"`     // the pool's user
	Match   *legacy.ProfileMatch `json:"-"`
}

// CertAction is what happens to a site's certificate.
type CertAction string

// The certificate actions (LEDGER HTE-67).
const (
	CertCopy    CertAction = "copy"    // the legacy pair is copied; the key by the root step
	CertReissue CertAction = "reissue" // a new ECDSA pair
)

// CertPlan is the certificate of an imported HTTPS site.
type CertPlan struct {
	Action CertAction `json:"action"`
	Cert   string     `json:"cert,omitempty"` // legacy source
	Key    string     `json:"key,omitempty"`
	Dest   string     `json:"dest"` // the site's userland certificate directory
}

// RootOp is a step --apply runs through `sudo gash hte __root`.
type RootOp struct {
	Op     string `json:"op"` // copy_key
	Domain string `json:"domain"`
	Src    string `json:"src"`
	Dst    string `json:"dst"`
}

// Report kinds.
const (
	ReportDuplicate    = "duplicate"
	ReportSharedSocket = "shared_socket"
	ReportCustomPort   = "custom_port"
	ReportOrphanPool   = "orphan_pool"
	ReportOrphanCert   = "orphan_certificate"
	ReportOrphanInfo   = "orphan_info"
	ReportIndexGap     = "index_gap"
	ReportRunUser      = "apache_run_user"
	ReportUnreadable   = "unreadable"
	ReportPoolError    = "pool_error"
	ReportHostsLine    = "hosts_line"
)

// Report is one finding that is not a site.
type Report struct {
	Kind    string   `json:"kind"`
	Paths   []string `json:"paths,omitempty"`
	Message string   `json:"message"`
}

// Hosts is the hosts part of the plan (design step 7, LEDGER HTE-45).
type Hosts struct {
	Entries []hostshelper.Entry `json:"entries"` // the managed block after --apply
	Block   string              `json:"block"`
	// Extra are the legacy names no site serves: --apply keeps them in
	// hosts.toml, so they stay in the block.
	Extra []string `json:"extra_names"`
	// Legacy are the "# hte-cli" lines; they stay untouched.
	Legacy []legacy.HostsLine `json:"legacy_lines"`
}

// Plan is the dry-run result.
type Plan struct {
	Platform string     `json:"platform"`
	Sites    []SitePlan `json:"sites"`
	Hosts    Hosts      `json:"hosts"`
	RootOps  []RootOp   `json:"root_steps"`
	Reports  []Report   `json:"reports"`
	// NoLongerServed are the enabled vhosts excluded with --exclude and
	// the enabled conf-enabled snippets: the system Apache stops serving
	// them when --apply stops it (HTE-64).
	NoLongerServed []string `json:"no_longer_served"`
	// Blocking are the enabled vhosts neither imported (now or before)
	// nor excluded: while any is left, --apply imports what it can but
	// does not stop the system Apache.
	Blocking []Blocking `json:"blocking_stop"`
}

// Blocking is an enabled vhost that keeps the system Apache running, with
// the exact --exclude argument that unblocks the stop.
type Blocking struct {
	Path    string `json:"path"`
	Action  Action `json:"action"`
	Reason  string `json:"reason,omitempty"`
	Exclude string `json:"exclude"` // "--exclude PATH", the path shell-quoted when needed
}

// candidate is a vhost with its identity resolved.
type candidate struct {
	v      legacy.Vhost
	plan   *SitePlan
	domain string // normalized
}

// BuildPlan reads the system through env and builds the plan.
func BuildPlan(env Env, opt Options) (*Plan, error) {
	sel, err := normalizeAll(opt.Sites)
	if err != nil {
		return nil, err
	}
	exc, excPaths, err := splitExcludes(opt.Exclude)
	if err != nil {
		return nil, err
	}
	for _, d := range sel {
		if slices.Contains(exc, d) {
			return nil, hteerr.Newf(hteerr.ConflictingOptions, "", "%s is both selected with --site and excluded", d)
		}
	}
	inv := legacy.Scan(env.Sysroot, env.Layout)
	p := &Plan{Platform: env.OS.Platform(), Sites: make([]SitePlan, 0, len(inv.Vhosts)), RootOps: []RootOp{}, Reports: []Report{},
		NoLongerServed: []string{}, Blocking: []Blocking{}}
	b := builder{env: env, opt: opt, inv: inv, plan: p, claimed: map[string]bool{}}

	// Identity first, then exclusions by path, duplicates, selection, and
	// the import.
	var cands []*candidate
	byDomain := map[string][]*candidate{}
	for _, v := range inv.Vhosts {
		sp := SitePlan{Path: v.Path, Enabled: v.Enabled, Class: v.Class, Warnings: v.Warnings}
		p.Sites = append(p.Sites, sp)
		c := &candidate{v: v, plan: &p.Sites[len(p.Sites)-1]}
		cands = append(cands, c)
		b.identify(c)
		if slices.Contains(excPaths, v.Path) {
			c.plan.Action, c.plan.Reason, c.plan.Code = ActionExcluded, "excluded with --exclude "+v.Path, ""
			continue
		}
		if c.domain != "" {
			byDomain[c.domain] = append(byDomain[c.domain], c)
		}
	}
	for _, ep := range excPaths {
		if !slices.ContainsFunc(cands, func(c *candidate) bool { return c.v.Path == ep }) {
			return nil, hteerr.Newf(hteerr.SiteNotFound, "give the path as the plan lists it",
				"no legacy vhost file %s", ep)
		}
	}
	for _, d := range append(slices.Clone(sel), exc...) {
		if _, ok := byDomain[d]; !ok {
			hint := "the plan without --site and --exclude lists the vhosts"
			if slices.Contains(sel, d) && slices.ContainsFunc(cands, func(c *candidate) bool { return c.domain == d }) {
				return nil, hteerr.Newf(hteerr.ConflictingOptions, "", "%s is selected with --site, but every vhost serving it is excluded", d)
			}
			return nil, hteerr.Newf(hteerr.SiteNotFound, hint, "no legacy vhost serves %s", d)
		}
	}
	for _, d := range sortedKeys(byDomain) {
		cs := byDomain[d]
		if len(cs) < 2 {
			continue
		}
		var paths []string
		for _, c := range cs {
			paths = append(paths, c.v.Path)
			c.plan.Action, c.plan.Reason = ActionSkip, "duplicate: another vhost also serves "+d
		}
		slices.Sort(paths)
		p.Reports = append(p.Reports, Report{Kind: ReportDuplicate, Paths: paths,
			Message: "several vhosts serve " + d + " (names compared case-insensitively); none is imported"})
	}
	for _, c := range cands {
		if c.plan.Action != "" {
			continue
		}
		switch {
		case slices.Contains(exc, c.domain):
			c.plan.Action, c.plan.Reason = ActionExcluded, "excluded with --exclude"
		case len(sel) > 0 && !slices.Contains(sel, c.domain):
			c.plan.Action, c.plan.Reason = ActionUnchosen, "not selected with --site"
		default:
			b.build(c)
		}
	}
	b.reportSockets()
	b.reportOrphans()
	b.reportSystem()
	b.hosts(cands)
	for _, c := range cands {
		sp := c.plan
		if !sp.Enabled || sp.Action == ActionImport || sp.Action == ActionImportRaw || sp.Action == ActionManaged {
			continue
		}
		if sp.Action == ActionExcluded {
			p.NoLongerServed = append(p.NoLongerServed, sp.Path)
		} else {
			p.Blocking = append(p.Blocking, Blocking{Path: sp.Path, Action: sp.Action, Reason: sp.Reason,
				Exclude: "--exclude " + shellWord(sp.Path)})
		}
	}
	for _, s := range inv.Snippets {
		p.NoLongerServed = append(p.NoLongerServed, s.Path)
	}
	return p, nil
}

// splitExcludes sorts the --exclude values into domains and absolute
// vhost file paths (cleaned). A value with a "/" that is not absolute is
// refused rather than taken for a domain.
func splitExcludes(in []string) (domains, paths []string, err error) {
	var names []string
	for _, e := range in {
		switch {
		case path.IsAbs(e):
			if p := path.Clean(e); !slices.Contains(paths, p) {
				paths = append(paths, p)
			}
		case strings.Contains(e, "/"):
			return nil, nil, hteerr.Newf(hteerr.InvalidDomain, "--exclude takes a domain or an absolute vhost file path",
				"--exclude %q is a relative path", e)
		default:
			names = append(names, e)
		}
	}
	domains, err = normalizeAll(names)
	return domains, paths, err
}

// shellWord quotes a path for a command line the user copies: as is when
// it holds only safe characters, else in single quotes.
func shellWord(s string) string {
	safe := s != "" && strings.IndexFunc(s, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("/._-+@%=:,", r))
	}) < 0
	if safe {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func normalizeAll(in []string) ([]string, error) {
	var out []string
	for _, d := range in {
		n, err := state.NormalizeDomain(d)
		if err != nil {
			return nil, err
		}
		if !slices.Contains(out, n) {
			out = append(out, n)
		}
	}
	return out, nil
}

type builder struct {
	env     Env
	opt     Options
	inv     *legacy.Inventory
	plan    *Plan
	planned []state.Site
	claimed map[string]bool // pool and certificate files that belong to a vhost
}

func (b *builder) skip(c *candidate, reason string) {
	c.plan.Action, c.plan.Reason = ActionSkip, reason
}

func (b *builder) skipErr(c *candidate, err error) {
	c.plan.Action = ActionSkip
	if e, ok := hteerr.As(err); ok {
		c.plan.Code, c.plan.Reason = e.Code, e.Details
		if e.Hint != "" {
			c.plan.Reason += " (" + e.Hint + ")"
		}
		return
	}
	c.plan.Reason = err.Error()
}

// identify resolves the site name of a vhost; a vhost without a usable
// one is a skip now.
func (b *builder) identify(c *candidate) {
	v := c.v
	if v.Class == legacy.ClassMalformed {
		b.skip(c, v.Reason)
		return
	}
	if v.Conf == nil {
		b.skip(c, v.Reason)
		return
	}
	name := v.Domain
	if !v.Marked() {
		name = v.ServerName
		if name == "" {
			b.skip(c, "no ServerName: nothing names the site")
			return
		}
	}
	host := strings.TrimSuffix(name, ".")
	if h, port, ok := strings.Cut(host, ":"); ok && !strings.Contains(port, ":") && !v.Marked() {
		host = h
	}
	if _, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil {
		b.skip(c, "the site name "+name+" is an IP address; a site name is a hostname such as app.test")
		return
	}
	domain, err := state.NormalizeDomain(host)
	if err != nil {
		b.skipErr(c, err)
		return
	}
	c.domain, c.plan.Domain = domain, domain
	if v.Marked() {
		b.claimMarked(v)
		switch sn := strings.ToLower(v.ServerName); {
		case sn == "":
			c.domain = ""
			b.skip(c, "no ServerName; the socket names "+v.Domain)
		case sn != domain:
			c.domain = ""
			b.skip(c, "ServerName "+v.ServerName+" differs from the socket's domain "+v.Domain)
		}
	}
}

// claimMarked records the pool and certificate files a marked vhost owns,
// imported or not, so that they are not reported as orphans.
func (b *builder) claimMarked(v legacy.Vhost) {
	l := b.env.Layout
	if p := l.PoolPath(v.PHP, v.Domain); p != "" {
		b.claimed[p] = true
	}
	if cert, key := l.CertPair(v.Domain); cert != "" {
		b.claimed[cert], b.claimed[key] = true, true
		b.claimed[strings.TrimSuffix(cert, ".crt")+".info"] = true
	}
}

func (b *builder) existing() []state.Site {
	return append(slices.Clone(b.env.Existing), b.planned...)
}

func (b *builder) build(c *candidate) {
	for _, s := range b.env.Existing {
		if s.Domain == c.domain {
			c.plan.Action, c.plan.Reason = ActionManaged, "site "+c.domain+" is already managed"
			return
		}
	}
	if c.v.Marked() {
		b.buildManaged(c)
	} else {
		b.buildRaw(c)
	}
	if s := c.plan.Site; s != nil {
		b.planned = append(b.planned, *s)
		for _, l := range []struct {
			proto string
			ports []int
		}{{"HTTP", s.ExtraHTTPPorts}, {"HTTPS", s.ExtraHTTPSPorts}} {
			for _, p := range l.ports {
				msg := c.domain + " listens on port " + strconv.Itoa(p) + "; kept as an extra " + l.proto + " port of the site"
				if p < 1024 {
					msg += " (below 1024: binding it needs net.ipv4.ip_unprivileged_port_start at most " + strconv.Itoa(p) + ")"
				}
				b.plan.Reports = append(b.plan.Reports, Report{Kind: ReportCustomPort, Paths: []string{c.v.Path}, Message: msg})
			}
		}
	}
}

func (b *builder) buildManaged(c *candidate) {
	v, env := c.v, b.env
	pp := b.poolPlan(c)
	c.plan.Pool = pp
	profile := string(pp.Profile)
	php := v.PHP
	https, force, bypass, http2 := v.HTTPS, v.Forced && v.HTTPS, v.Bypass, true
	if v.Forced && !v.HTTPS {
		c.plan.Warnings = append(c.plan.Warnings, "forced HTTPS without a usable HTTPS vhost; imported as not forced")
	}
	var extra []string
	noWWW := !slices.Contains(lower(v.Aliases), "www."+c.domain)
	for _, a := range lower(v.Aliases) {
		if a != "www."+c.domain {
			extra = append(extra, a)
		}
	}
	ports, err := customPorts(v.Conf, env)
	if err != nil {
		b.skip(c, err.Error())
		return
	}
	if len(ports.https) > 0 && !https {
		c.plan.Warnings = append(c.plan.Warnings, "the HTTPS ports "+joinInts(ports.https)+" are not kept: the site has no usable HTTPS vhost")
		ports.https = nil
	}
	docroot := v.Docroot
	f := state.Flags{
		Domain: c.domain, Docroot: &docroot, PHP: &php, Profile: &profile,
		HTTPS: &https, ForceHTTPS: &force, ForceHTTPSBypass: &bypass, HTTP2: &http2,
		Aliases: extra, NoWWW: noWWW, ExtraHTTPPorts: &ports.http, ExtraHTTPSPorts: &ports.https,
	}
	site, err := state.NewSite(f, state.Env{Cwd: "/", Installed: env.Installed, Ports: env.Ports, Stat: env.Stat}, b.existing())
	if err != nil {
		b.skipErr(c, err)
		return
	}
	site.Enabled = v.Enabled
	if b.opt.KeepPoolValues && pp.Match != nil {
		for _, d := range pp.Diffs {
			k, ok := legacy.INIKey(d.Key)
			val := iniText(d.Pool)
			// Line is 0 for a profile value the pool lacks: nothing to keep.
			if ok && d.Line > 0 && state.CheckINI(k, val) == nil {
				if site.PHPIni == nil {
					site.PHPIni = map[string]string{}
				}
				site.PHPIni[k] = val
				pp.Kept = append(pp.Kept, k)
			} else {
				pp.NotKept = append(pp.NotKept, d.Key)
			}
		}
	}
	if v.DocrootRecovered {
		c.plan.Notes = append(c.plan.Notes, "the unquoted DocumentRoot with spaces was recovered from <Directory>")
	}
	switch {
	case !force:
	case v.Class == legacy.ClassV1:
		c.plan.Notes = append(c.plan.Notes, "V1 force-HTTPS: every plain-HTTP request keeps being redirected, now with 307 instead of 301")
	default:
		c.plan.Notes = append(c.plan.Notes, "force-HTTPS with the loopback bypass, as hte-cli 1.1.x: requests from 127.0.0.1 and ::1 are not redirected; 301 becomes 307")
	}
	c.plan.Action, c.plan.Site = ActionImport, &site
	if site.HTTPS {
		c.plan.Cert = b.certPlan(c, v.CertFile, v.KeyFile, true)
	}
	b.htaccess(c)
}

// htaccess warns about an HTTPS redirect of the document root's .htaccess
// (LEDGER HTE-26).
func (b *builder) htaccess(c *candidate) {
	if c.plan.Site.Docroot == "" {
		return
	}
	redirects, err := htaccess.Check(c.plan.Site.Docroot, b.env.ReadFile)
	if err != nil {
		c.plan.Warnings = append(c.plan.Warnings, "cannot read the .htaccess of "+c.plan.Site.Docroot+": "+err.Error())
	}
	for _, r := range redirects {
		c.plan.Warnings = append(c.plan.Warnings, r.Warning())
	}
}

func joinInts(in []int) string {
	out := make([]string, len(in))
	for i, n := range in {
		out[i] = strconv.Itoa(n)
	}
	return strings.Join(out, ", ")
}

// iniText returns the value an INI line means: the text inside a leading
// pair of double quotes, else the text before a ";" comment, trimmed.
func iniText(raw string) string {
	raw = strings.TrimSpace(raw)
	if rest, ok := strings.CutPrefix(raw, `"`); ok {
		if v, _, closed := strings.Cut(rest, `"`); closed {
			return v
		}
		return raw
	}
	v, _, _ := strings.Cut(raw, ";")
	return strings.TrimSpace(v)
}

func lower(in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = strings.ToLower(s)
	}
	return out
}

// poolPlan finds the exact pool of a marked vhost and its profile.
func (b *builder) poolPlan(c *candidate) *PoolPlan {
	v := c.v
	pp := &PoolPlan{Path: b.env.Layout.PoolPath(v.PHP, v.Domain), Profile: profiles.Dev, Source: legacy.ProfileFromDefault}
	pool, ok := b.inv.PoolByPath(pp.Path)
	switch {
	case pp.Path == "":
		return pp
	case !ok:
		c.plan.Notes = append(c.plan.Notes, "no pool at "+pp.Path+"; profile dev")
		return pp
	case pool.Section != v.Domain:
		c.plan.Warnings = append(c.plan.Warnings, pp.Path+" starts with ["+pool.Section+"], not ["+v.Domain+"]; it is not this site's pool; profile dev")
		return pp
	}
	m := legacy.DetectProfile(pool, v.Docroot)
	pp.Found, pp.Profile, pp.Source, pp.Diffs, pp.Match = true, m.Profile, m.Source, m.Diffs, &m
	if m.Warning != "" {
		c.plan.Warnings = append(c.plan.Warnings, m.Warning)
	}
	if m.Source == legacy.ProfileFromValues && pool.ProfileMarker == "" {
		c.plan.Notes = append(c.plan.Notes, "the pool has no \"; Profile:\" line; its values are those of "+string(m.Profile))
	}
	if len(m.Diffs) > 0 && !b.opt.KeepPoolValues {
		c.plan.Notes = append(c.plan.Notes, "the pool differs from the "+string(m.Profile)+" profile; --keep-pool-values moves the php_* differences to [php_ini]")
	}
	if u, ok := pool.Get("user"); ok {
		pp.User = u
		if b.env.User != "" && u != b.env.User {
			c.plan.Warnings = append(c.plan.Warnings, "the pool ran as "+u+"; the userland pool runs as "+b.env.User)
		}
	}
	return pp
}

// certPlan decides the certificate of an HTTPS site: the legacy pair is
// copied (the key through the root step) unless --reissue-certs or the
// pair is not where it must be.
func (b *builder) certPlan(c *candidate, cert, key string, marked bool) *CertPlan {
	cp := &CertPlan{Action: CertReissue, Dest: b.env.Paths.CertDir(c.domain)}
	if b.opt.ReissueCerts {
		return cp
	}
	if marked {
		wantCert, wantKey := b.env.Layout.CertPair(c.v.Domain)
		if wantCert == "" || cert != wantCert || key != wantKey {
			c.plan.Warnings = append(c.plan.Warnings, "the vhost names "+cert+" and "+key+", not the hte-cli pair; a new certificate is issued")
			return cp
		}
	} else if dir := b.env.Layout.CertDir; dir != "" && (path.Dir(cert) == dir || path.Dir(key) == dir) {
		b.claimed[cert], b.claimed[key] = true, true
		c.plan.Notes = append(c.plan.Notes, "uses hte-cli certificates from "+dir)
	}
	sr := b.env.Sysroot
	if !sr.IsRegular(cert) || !sr.IsRegular(key) {
		c.plan.Warnings = append(c.plan.Warnings, "the certificate pair "+cert+" / "+key+" is missing; a new certificate is issued")
		return cp
	}
	cp.Action, cp.Cert, cp.Key = CertCopy, cert, key
	if fi, err := sr.Stat(key); err == nil && fi.Mode().Perm()&0o077 == 0 {
		c.plan.Notes = append(c.plan.Notes, "the key "+key+" is readable by its owner only; the root step copies it")
	}
	b.plan.RootOps = append(b.plan.RootOps, RootOp{Op: "copy_key", Domain: c.domain, Src: key, Dst: b.env.Paths.KeyFile(c.domain)})
	return cp
}

func (b *builder) buildRaw(c *candidate) {
	v, env := c.v, b.env
	names := map[string]bool{}
	for _, vh := range v.Conf.Vhosts {
		for _, d := range vh.Find("ServerName") {
			if len(d.Args) == 1 {
				h, _, _ := strings.Cut(strings.ToLower(d.Args[0]), ":")
				names[h] = true
			}
		}
	}
	if len(names) > 1 {
		b.skip(c, "the file serves several ServerName values; one site has one file")
		return
	}
	var aliases []string
	for _, a := range lower(v.Aliases) {
		if hostshelper.ValidHostname(a) && a != c.domain && !slices.Contains(aliases, a) {
			aliases = append(aliases, a)
		} else if !hostshelper.ValidHostname(a) {
			c.plan.Warnings = append(c.plan.Warnings, "ServerAlias "+a+" is not a hostname; it stays in the vhost but not in the hosts block")
		}
	}
	res, err := rewriteRaw(v, c.domain, env)
	if err != nil {
		b.skip(c, "cannot import as raw: "+err.Error())
		return
	}
	site := state.Site{Domain: c.domain, Aliases: aliases, Kind: state.KindRaw, Enabled: v.Enabled,
		RawConf: res.conf, HTTPS: res.certFile != "",
		ExtraHTTPPorts: append([]int{}, res.ports.http...), ExtraHTTPSPorts: append([]int{}, res.ports.https...)}
	if aliases == nil {
		site.Aliases = []string{}
	}
	if v.Docroot != "" && state.CheckDocrootText(v.Docroot) == nil && path.IsAbs(v.Docroot) && path.Clean(v.Docroot) == v.Docroot {
		site.Docroot = v.Docroot
	}
	if res.php != "" {
		if !slices.Contains(env.Installed, res.php) {
			b.skipErr(c, hteerr.Newf(hteerr.PHPVersionNotInstalled, "", "the vhost uses PHP %s, which is not installed", res.php))
			return
		}
		site.PHP, site.Profile = res.php, string(profiles.Dev)
		c.plan.Notes = append(c.plan.Notes, "its PHP-FPM socket becomes the site's own pool (PHP "+res.php+", profile dev)")
	}
	if err := site.Validate(); err != nil {
		b.skipErr(c, err)
		return
	}
	if err := state.CheckUnique(site, b.existing()); err != nil {
		b.skipErr(c, err)
		return
	}
	if err := state.CheckPortProtocols(site, b.existing()); err != nil {
		b.skipErr(c, err)
		return
	}
	c.plan.Notes = append(c.plan.Notes, res.notes...)
	c.plan.Action, c.plan.Site = ActionImportRaw, &site
	if site.HTTPS {
		c.plan.Cert = b.certPlan(c, res.certFile, res.keyFile, false)
	}
	b.htaccess(c)
}

// reportSockets reports a legacy socket used by several vhost files: each
// imported site gets its own pool.
func (b *builder) reportSockets() {
	users := map[string][]string{}
	for _, v := range b.inv.Vhosts {
		for _, s := range v.Sockets {
			users[s] = append(users[s], v.Path)
		}
	}
	for _, s := range sortedKeys(users) {
		if len(users[s]) > 1 {
			b.plan.Reports = append(b.plan.Reports, Report{Kind: ReportSharedSocket, Paths: users[s],
				Message: "the vhosts share the socket " + s + "; each imported site gets its own pool"})
		}
	}
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// reportOrphans lists pools and certificate files no vhost owns (never
// deleted) and the index gaps.
func (b *builder) reportOrphans() {
	for _, p := range sortedKeys(b.inv.Pools) {
		if b.claimed[p] || path.Base(p) == "www.conf" {
			continue
		}
		b.plan.Reports = append(b.plan.Reports, Report{Kind: ReportOrphanPool, Paths: []string{p},
			Message: "pool [" + b.inv.Pools[p].Section + "] belongs to no vhost; left in place"})
	}
	for _, e := range b.inv.PoolErrors {
		b.plan.Reports = append(b.plan.Reports, Report{Kind: ReportPoolError, Paths: []string{e.Path}, Message: e.Err})
	}
	for _, f := range b.inv.CertFiles {
		if b.claimed[f] {
			continue
		}
		kind := ReportOrphanCert
		if strings.HasSuffix(f, ".info") {
			kind = ReportOrphanInfo
		}
		b.plan.Reports = append(b.plan.Reports, Report{Kind: kind, Paths: []string{f},
			Message: "belongs to no vhost; left in place"})
	}
	if gaps := b.inv.IndexGaps(); len(gaps) > 0 {
		b.plan.Reports = append(b.plan.Reports, Report{Kind: ReportIndexGap, Paths: b.env.Layout.VhostDirs,
			Message: "unused NNN- numbers " + strings.Join(gaps, ", ") + " (information only: gash names files by domain)"})
	}
}

func (b *builder) reportSystem() {
	if b.inv.RunUser != "" && b.inv.RunUser == b.env.User {
		b.plan.Reports = append(b.plan.Reports, Report{Kind: ReportRunUser, Paths: []string{b.env.Layout.Envvars},
			Message: "APACHE_RUN_USER is " + b.inv.RunUser + ": the system Apache's workers already run as you, but its master runs as root; the userland stack replaces both"})
	}
	for _, u := range b.inv.Unreadable {
		b.plan.Reports = append(b.plan.Reports, Report{Kind: ReportUnreadable, Paths: []string{u.Path},
			Message: "cannot be read (" + u.Err + "); left out of the plan"})
	}
}

// hosts builds the managed block after --apply: the sites already
// managed and the imported ones (enabled only, as hosts.Desired), then the
// names of "# hte-cli" lines no site covers, kept in hosts.toml. The legacy
// lines stay as they are.
func (b *builder) hosts(cands []*candidate) {
	h := &b.plan.Hosts
	h.Legacy = b.inv.Hosts
	if h.Legacy == nil {
		h.Legacy = []legacy.HostsLine{}
	}
	sites := slices.Clone(b.env.Existing)
	for _, c := range cands {
		if c.plan.Site != nil {
			sites = append(sites, *c.plan.Site)
		}
	}
	covered := map[string]bool{}
	for _, s := range sites {
		for _, n := range s.Names() {
			covered[n] = true
		}
	}
	h.Extra = []string{}
	for _, n := range b.env.HostsExtra {
		if !covered[n] && !slices.Contains(h.Extra, n) {
			h.Extra = append(h.Extra, n)
		}
	}
	for _, l := range b.inv.Hosts {
		if l.Addr != state.LoopbackIPv4 {
			b.plan.Reports = append(b.plan.Reports, Report{Kind: ReportHostsLine, Paths: []string{b.env.Layout.Hosts},
				Message: "line " + strconv.Itoa(l.Line) + " maps hte-cli names to " + l.Addr + "; not taken"})
			continue
		}
		for _, n := range l.Names {
			n = strings.ToLower(n)
			switch {
			case !hostshelper.ValidHostname(n):
				b.plan.Reports = append(b.plan.Reports, Report{Kind: ReportHostsLine, Paths: []string{b.env.Layout.Hosts},
					Message: "line " + strconv.Itoa(l.Line) + ": " + n + " is not a hostname; not taken"})
			case !covered[n] && !slices.Contains(h.Extra, n):
				h.Extra = append(h.Extra, n)
			}
		}
	}
	h.Entries = hosts.Desired(sites, h.Extra)
	h.Block = string(hostshelper.FormatBlock(h.Entries))
}
