// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package hte

import (
	"context"
	"errors"
	"fmt"
	"net"
	"path"
	"slices"
	"strings"

	"github.com/mauriziofonte/gash/internal/hte/hosts"
	"github.com/mauriziofonte/gash/internal/hte/hteerr"
	"github.com/mauriziofonte/gash/internal/hte/legacy"
	"github.com/mauriziofonte/gash/internal/hte/migrate"
	"github.com/mauriziofonte/gash/internal/hte/receipt"
	"github.com/mauriziofonte/gash/internal/hte/render"
	"github.com/mauriziofonte/gash/internal/hte/rootops"
	"github.com/mauriziofonte/gash/internal/hte/service"
	"github.com/mauriziofonte/gash/internal/hte/state"
)

// MigrateOptions are the flags of migrate.
type MigrateOptions struct {
	Apply          bool
	Sites          []string // --site D
	Exclude        []string // --exclude D|PATH
	KeepPoolValues bool
	ReissueCerts   bool
	Yes            bool
}

// MigrateResult is the plan, and with --apply what was done.
type MigrateResult struct {
	Plan    *migrate.Plan `json:"plan"`
	Applied bool          `json:"applied"`
	// RootPlan are the root steps --apply runs (or would run), as shown
	// before sudo.
	RootPlan []string `json:"root_plan"`
	Imported []string `json:"imported"`
	// Staging is where the new sites were verified before the switch
	// ("127.0.0.1:<http>,<https>"), "" when the stack already ran.
	Staging  string               `json:"staging,omitempty"`
	Verified []service.SiteStatus `json:"verified"`
	// Switched: the system Apache is stopped and the stack serves the
	// sites on its own ports.
	Switched bool             `json:"switched"`
	Archived []string         `json:"archived"`
	Changes  []receipt.Change `json:"changes"`
	Warnings []string         `json:"warnings"`
}

// probeSite is Engine.Probe or the real probe.
func (e *Engine) probeSite(ctx context.Context, ep render.Endpoint) service.SiteStatus {
	if e.Probe != nil {
		return e.Probe(ctx, ep)
	}
	return service.ProbeSite(ctx, ep)
}

// answered reports whether every address of a site gave an HTTP answer
// that is not a gateway error (502, 503, 504: the FPM pool is not
// serving).
func answered(st service.SiteStatus) error {
	for _, p := range append(slices.Clone(st.HTTP), st.HTTPS...) {
		switch {
		case p.Error != "":
			return fmt.Errorf("%s: %s", p.URL, p.Error)
		case p.Status == 502 || p.Status == 503 || p.Status == 504:
			return fmt.Errorf("%s: HTTP %d", p.URL, p.Status)
		}
	}
	return nil
}

// verify probes the sites named in domains on the current generation.
func (e *Engine) verify(ctx context.Context, v *env, domains []string, res *MigrateResult) error {
	_, m, ok, err := e.supervisor(v).Current()
	if err != nil || !ok {
		return err
	}
	var failed []string
	for _, d := range domains {
		if !slices.ContainsFunc(m.Sites, func(ep render.Endpoint) bool { return ep.Domain == d }) {
			failed = append(failed, d+" (not in the rendered generation)")
		}
	}
	for _, ep := range m.Sites {
		if !slices.Contains(domains, ep.Domain) {
			continue
		}
		st := e.probeSite(ctx, ep)
		res.Verified = append(res.Verified, st)
		if err := answered(st); err != nil {
			failed = append(failed, ep.Domain+" ("+err.Error()+")")
		}
	}
	if len(failed) > 0 {
		return hteerr.Newf(hteerr.ReloadFailed, "run gash hte service logs --site DOMAIN",
			"the migrated sites do not answer: %s", strings.Join(failed, "; "))
	}
	return nil
}

// freePorts returns two loopback ports nothing listens on, for staging.
func freePorts() (state.Ports, error) {
	var ls []net.Listener
	defer func() {
		for _, l := range ls {
			l.Close()
		}
	}()
	var ports []int
	for range 2 {
		l, err := net.Listen("tcp4", state.LoopbackIPv4+":0")
		if err != nil {
			return state.Ports{}, hteerr.Newf(hteerr.PortInUse, "", "no free loopback port for staging: %v", err)
		}
		ls = append(ls, l)
		ports = append(ports, l.Addr().(*net.TCPAddr).Port)
	}
	return state.Ports{HTTP: ports[0], HTTPS: ports[1]}, nil
}

// Migrate is migrate: the plan, and with --apply the import (design/hte.md
// 10): new sites verified first (on staging ports while the system Apache
// holds the real ones), then, when no enabled system vhost is left
// unhandled, the system Apache stopped and the sites verified on the real
// ports, and only then the legacy files archived. A failed verification
// after the switch starts the system Apache again.
func (e *Engine) Migrate(ctx context.Context, opt MigrateOptions) (*MigrateResult, error) {
	if err := e.CheckIdentity("migrate"); err != nil {
		return nil, err
	}
	if opt.Apply && e.EUID == 0 {
		return nil, hteerr.New(hteerr.RootNotAllowed, "migrate --apply moves the sites into your own userland stack",
			"Run gash hte as your own user; it asks for sudo itself when a step needs root")
	}
	v, err := e.resolve()
	if err != nil {
		return nil, err
	}
	res := &MigrateResult{RootPlan: []string{}, Imported: []string{}, Verified: []service.SiteStatus{}, Archived: []string{},
		Changes: []receipt.Change{}, Warnings: []string{}}
	set, err := e.installed(v)
	if err != nil {
		if opt.Apply || !errors.Is(err, state.ErrNotInstalled) {
			return nil, err
		}
		set = state.DefaultSettings(state.StandardPorts, service.AutostartNone)
	}
	if !opt.Apply {
		w, err := e.planMigration(ctx, v, set, opt)
		if err != nil {
			return nil, err
		}
		res.Plan, res.RootPlan = w.plan, w.lines()
		return res, nil
	}
	if _, err := e.locked(v, "migrate", "", func() (any, error) { return res, e.migrateApply(ctx, v, set, opt, res) }); err != nil {
		return res, err
	}
	// The unit becomes active once the lock is released: its start finds
	// the stack running.
	if res.Switched && e.systemdUnit(v, set) {
		if err := e.systemctlUser(ctx, v, "start", service.UnitName); err != nil {
			res.Warnings = append(res.Warnings, err.Error())
		}
	}
	return res, nil
}

// migration is a plan with what --apply does with it.
type migration struct {
	plan     *migrate.Plan
	inv      *legacy.Inventory
	layout   legacy.Layout
	sites    []state.Site // the current sites
	create   []state.Site // the sites --apply creates
	copies   map[string]*migrate.CertPlan
	reissue  []state.Site // raw HTTPS sites that get a new pair
	vhosts   []string     // legacy vhosts to archive
	pools    []string     // legacy pools to archive
	apache   string       // the system Apache unit, "" when none
	keep     bool         // the system Apache keeps running: reloaded, not stopped (HTE-70)
	fpmStop  []string     // system FPM units left without pools
	fpmKeep  []string     // system FPM units that keep pools (reloaded)
	blocking bool
	extra    []string // hosts.toml names after --apply
	keyFile  func(domain string) string
}

// lines are the root steps as the user sees them before sudo.
func (w *migration) lines() []string {
	var out []string
	for _, o := range w.ops(nil) {
		out = append(out, o.String())
	}
	if out == nil {
		out = []string{}
	}
	return out
}

// ops are the root steps; keys maps a domain to the temporary key path
// of its copy (the final key path when nil, for the dry run).
func (w *migration) ops(keys map[string]string) []rootops.Op {
	var out []rootops.Op
	add := func(o rootops.Op) {
		o.ID = len(out) + 1
		out = append(out, o)
	}
	for _, d := range sortedKeys(w.copies) {
		dst := w.keyFile(d)
		if keys != nil {
			dst = keys[d]
		}
		add(rootops.Op{Op: rootops.OpCopyKey, Src: w.copies[d].Key, Dst: dst})
	}
	if w.blocking {
		return out
	}
	if w.apache != "" && !w.keep {
		add(rootops.Op{Op: rootops.OpUnitStop, Name: w.apache})
		add(rootops.Op{Op: rootops.OpUndoUnitStop, Name: w.apache})
	}
	for _, p := range w.vhosts {
		add(rootops.Op{Op: rootops.OpArchiveVhost, Path: p})
	}
	if w.apache != "" && w.keep && len(w.vhosts) > 0 {
		add(rootops.Op{Op: rootops.OpUnitReload, Name: w.apache})
	}
	for _, p := range w.pools {
		add(rootops.Op{Op: rootops.OpArchivePool, Path: p})
	}
	for _, u := range w.fpmStop {
		add(rootops.Op{Op: rootops.OpUnitStop, Name: u})
	}
	for _, u := range w.fpmKeep {
		add(rootops.Op{Op: rootops.OpUnitReload, Name: u})
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// planMigration builds the plan and decides what --apply does.
func (e *Engine) planMigration(ctx context.Context, v *env, set state.Settings, opt MigrateOptions) (*migration, error) {
	m, err := e.discover(ctx, v, set.Modules)
	if err != nil {
		return nil, err
	}
	layout, err := legacyLayout(v, m.apache)
	if err != nil {
		return nil, err
	}
	sites, problems, err := v.store.LoadSites()
	if err != nil {
		return nil, err
	}
	if len(problems) > 0 {
		return nil, problems[0]
	}
	hst, err := hosts.LoadState(v.paths.HostsState())
	if err != nil {
		return nil, err
	}
	plan, err := migrate.BuildPlan(migrate.Env{
		Sysroot: e.Sysroot, OS: v.os, Layout: layout, User: e.User, Paths: v.paths, Ports: set.Ports, Listen: listenAddrs(set),
		Installed: m.php.Installed(), Existing: sites, HostsExtra: hst.Names, Stat: e.Sysroot.Stat, ReadFile: e.Sysroot.ReadFile,
	}, migrate.Options{Sites: opt.Sites, Exclude: opt.Exclude, KeepPoolValues: opt.KeepPoolValues, ReissueCerts: opt.ReissueCerts})
	if err != nil {
		return nil, err
	}
	w := &migration{plan: plan, inv: legacy.Scan(e.Sysroot, layout), layout: layout, sites: sites, copies: map[string]*migrate.CertPlan{},
		blocking: len(plan.Blocking) > 0, extra: slices.Clone(hst.Names), keyFile: v.paths.KeyFile}
	for _, n := range plan.Hosts.Extra {
		if !slices.Contains(w.extra, n) {
			w.extra = append(w.extra, n)
		}
	}
	slices.Sort(w.extra)
	for _, sp := range plan.Sites {
		switch sp.Action {
		case migrate.ActionImport, migrate.ActionImportRaw:
			w.create = append(w.create, *sp.Site)
			if sp.Cert != nil && sp.Cert.Action == migrate.CertCopy {
				w.copies[sp.Site.Domain] = sp.Cert
			} else if sp.Cert != nil && sp.Site.Kind == state.KindRaw {
				w.reissue = append(w.reissue, *sp.Site)
			}
		case migrate.ActionManaged:
		default:
			continue
		}
		w.vhosts = append(w.vhosts, sp.Path)
		if p := w.pool(sp.Path); p != "" {
			w.pools = append(w.pools, p)
		}
	}
	if len(w.vhosts) > 0 || len(w.create) > 0 {
		if e.systemApacheInstalled(v) {
			w.apache = systemUnit(v.os.Family)
		}
	}
	w.keep = plan.SystemApache != nil
	w.fpmUnits()
	return w, nil
}

// pool is the hte-cli pool of a marked legacy vhost: the exact path whose
// first section is the vhost's domain (design/hte.md 10, step 5).
func (w *migration) pool(vhost string) string {
	i := slices.IndexFunc(w.inv.Vhosts, func(vh legacy.Vhost) bool { return vh.Path == vhost })
	if i < 0 {
		return ""
	}
	vh := w.inv.Vhosts[i]
	if !vh.Marked() || vh.PHP == "" {
		return ""
	}
	p := w.layout.PoolPath(vh.PHP, vh.Domain)
	if pool, ok := w.inv.PoolByPath(p); ok && pool.Section == vh.Domain {
		return p
	}
	return ""
}

// fpmUnits sorts the system FPM units of the archived pools: a unit left
// without pools stops, the others reload.
func (w *migration) fpmUnits() {
	versions := map[string]bool{}
	for _, p := range w.pools {
		versions[path.Base(path.Dir(path.Dir(path.Dir(p))))] = true
	}
	for _, ver := range sortedKeys(versions) {
		unit := "php" + ver + "-fpm"
		dir := path.Join(w.layout.PHPRoot, ver, "fpm", "pool.d")
		left := false
		for p := range w.inv.Pools {
			if path.Dir(p) == dir && !slices.Contains(w.pools, p) {
				left = true
			}
		}
		for _, pe := range w.inv.PoolErrors {
			if path.Dir(pe.Path) == dir {
				left = true
			}
		}
		if left {
			w.fpmKeep = append(w.fpmKeep, unit)
		} else {
			w.fpmStop = append(w.fpmStop, unit)
		}
	}
}

func (e *Engine) migrateApply(ctx context.Context, v *env, set state.Settings, opt MigrateOptions, res *MigrateResult) error {
	w, err := e.planMigration(ctx, v, set, opt)
	if err != nil {
		return err
	}
	res.Plan, res.RootPlan = w.plan, w.lines()
	res.Applied = true
	if len(w.create) == 0 && len(w.vhosts) == 0 {
		res.Warnings = append(res.Warnings, "nothing to migrate")
		return nil
	}
	ops := w.ops(nil)
	if !opt.Yes && slices.ContainsFunc(ops, func(o rootops.Op) bool {
		return o.Op == rootops.OpUnitStop || o.Op == rootops.OpArchiveVhost || o.Op == rootops.OpArchivePool
	}) {
		res.Applied = false
		what := "stops the system Apache and archives the legacy files"
		if w.keep {
			what = "archives the legacy files and reloads the system Apache"
		}
		return hteerr.Newf(hteerr.ConfirmationRequired, "add --yes",
			"migrate --apply %s; the root steps: %s", what, strings.Join(res.RootPlan, "; "))
	}

	// The temporary key paths, then one sudo for every root step.
	cm := e.certs(v)
	keys := map[string]string{}
	for _, d := range sortedKeys(w.copies) {
		if keys[d], err = cm.Prepare(d); err != nil {
			return err
		}
	}
	ops = w.ops(keys)
	res.RootPlan = nil
	for _, o := range ops {
		res.RootPlan = append(res.RootPlan, o.String())
	}
	var c *rootops.Client
	if len(ops) > 0 {
		if c, err = e.rootSession(ctx, v, ops); err != nil {
			return err
		}
		defer c.Close()
	}
	run := func(kind, arg string) error {
		i := slices.IndexFunc(ops, func(o rootops.Op) bool {
			return o.Op == kind && (o.Name == arg || o.Path == arg || o.Src == arg)
		})
		if i < 0 {
			return nil
		}
		r, err := c.Run(ops[i].ID)
		res.Changes = append(res.Changes, r.Changes...)
		res.Warnings = append(res.Warnings, r.Warnings...)
		return err
	}

	// 1. Certificates: the copies (root hands each key to openssl running
	// as the user), new pairs for raw sites that cannot copy theirs.
	for _, d := range sortedKeys(w.copies) {
		cp := w.copies[d]
		if err := run(rootops.OpCopyKey, cp.Key); err != nil {
			return err
		}
		pem, err := e.Sysroot.ReadFile(cp.Cert)
		if err != nil {
			return hteerr.Newf(hteerr.InvalidState, "run migrate --apply --reissue-certs to issue new pairs", "reading %s: %v", cp.Cert, err)
		}
		if err := cm.Finish(ctx, d, pem, keys[d]); err != nil {
			return err
		}
	}
	for _, s := range w.reissue {
		if err := cm.Issue(ctx, s); err != nil {
			return err
		}
	}

	// 2. The state.
	next := append(slices.Clone(w.sites), w.create...)
	for _, s := range w.create {
		res.Imported = append(res.Imported, s.Domain)
	}
	hostsBefore, err := hosts.LoadState(v.paths.HostsState())
	if err != nil {
		return err
	}
	undoState := func(cause error) error {
		errs := []error{cause}
		errs = append(errs, e.saveSites(v, next, w.sites), hosts.SaveState(v.paths.HostsState(), hostsBefore))
		for _, s := range w.create {
			errs = append(errs, cm.Remove(s.Domain))
		}
		return errors.Join(errs...)
	}
	if err := e.saveSites(v, w.sites, next); err != nil {
		return err
	}
	if err := hosts.SaveState(v.paths.HostsState(), hosts.State{Names: w.extra}); err != nil {
		return undoState(err)
	}

	// 3. The new sites answer: on the running stack, or on a staging
	// generation on free loopback ports while the system Apache keeps the
	// real ones.
	sup := e.supervisor(v)
	running, err := sup.Running()
	if err != nil {
		return undoState(err)
	}
	created := enabledDomains(w.create)
	if running {
		if _, err := e.applyStack(ctx, v, set, next); err != nil {
			return undoState(err)
		}
		if err := e.verify(ctx, v, created, res); err != nil {
			e.applyStack(ctx, v, set, w.sites)
			return undoState(err)
		}
	} else if len(created) > 0 {
		if err := e.staging(ctx, v, set, next, created, res); err != nil {
			return undoState(err)
		}
	}
	if w.blocking {
		res.Warnings = append(res.Warnings, fmt.Sprintf("%d enabled system vhosts are neither migrated nor excluded: the system Apache keeps running "+
			"and nothing is archived; migrate or exclude them (the plan lists the --exclude to add), then run migrate --apply again", len(w.plan.Blocking)))
		return e.applyHosts(ctx, v, next)
	}

	// 4. The switch: the system Apache stops (unless it keeps running next
	// to a stack on other ports), the stack serves every site on its own
	// ports; a site that does not answer starts it again.
	if !w.keep {
		if err := run(rootops.OpUnitStop, w.apache); err != nil {
			return err
		}
	}
	switchErr := func() error {
		if _, err := e.applyStack(ctx, v, set, next); err != nil {
			return err
		}
		if err := sup.Start(ctx); err != nil {
			return err
		}
		return e.verify(ctx, v, enabledDomains(next), res)
	}()
	if switchErr != nil {
		errs := []error{switchErr}
		if !running {
			errs = append(errs, sup.Stop(ctx))
		}
		after := "the system Apache was started again"
		if w.keep {
			after = "the system Apache was not touched"
		} else if w.apache != "" {
			errs = append(errs, run(rootops.OpUndoUnitStop, w.apache))
		}
		return hteerr.Newf(hteerr.ReloadFailed, "the sites stay imported; fix the cause (gash hte doctor), then run gash hte migrate --apply again",
			"after the switch: %v; %s", errors.Join(errs...), after)
	}
	res.Switched = true

	// 5. Only now the legacy files go into the archive. A system Apache
	// that keeps running is then tested and reloaded; a failed test puts
	// the vhosts back (root does it) and stops here.
	for _, p := range w.vhosts {
		if err := run(rootops.OpArchiveVhost, p); err != nil {
			return err
		}
		res.Archived = append(res.Archived, p)
	}
	if w.keep {
		if err := run(rootops.OpUnitReload, w.apache); err != nil {
			if hteerr.CodeOf(err) == hteerr.ConfigTestFailed {
				res.Archived = slices.DeleteFunc(res.Archived, func(p string) bool { return slices.Contains(w.vhosts, p) })
				res.Changes = slices.DeleteFunc(res.Changes, func(c receipt.Change) bool {
					return c.Kind == receipt.LegacyVhost && slices.Contains(w.vhosts, c.Path)
				})
			}
			return err
		}
	}
	for _, p := range w.pools {
		if err := run(rootops.OpArchivePool, p); err != nil {
			return err
		}
		res.Archived = append(res.Archived, p)
	}
	for _, u := range w.fpmStop {
		if err := run(rootops.OpUnitStop, u); err != nil {
			return err
		}
	}
	for _, u := range w.fpmKeep {
		if err := run(rootops.OpUnitReload, u); err != nil {
			res.Warnings = append(res.Warnings, err.Error())
		}
	}
	return e.applyHosts(ctx, v, next)
}

func enabledDomains(sites []state.Site) []string {
	var out []string
	for _, s := range sites {
		if s.Enabled {
			out = append(out, s.Domain)
		}
	}
	return out
}

// staging verifies the sites on a staging generation: the stack's state on
// two free loopback ports, IPv4 only, without the extra ports (the system
// Apache may hold them), started, probed, stopped.
func (e *Engine) staging(ctx context.Context, v *env, set state.Settings, sites []state.Site, domains []string, res *MigrateResult) error {
	ports, err := freePorts()
	if err != nil {
		return err
	}
	staging := set
	staging.Ports, staging.Listen = ports, []string{state.LoopbackIPv4}
	var staged []state.Site
	for _, s := range sites {
		s.ExtraHTTPPorts, s.ExtraHTTPSPorts = []int{}, []int{}
		staged = append(staged, s)
	}
	res.Staging = fmt.Sprintf("%s:%d,%d", state.LoopbackIPv4, ports.HTTP, ports.HTTPS)
	if _, err := e.applyStack(ctx, v, staging, staged); err != nil {
		return err
	}
	sup := e.supervisor(v)
	err = sup.Start(ctx)
	if err == nil {
		err = e.verify(ctx, v, domains, res)
	}
	return errors.Join(err, sup.Stop(ctx))
}
