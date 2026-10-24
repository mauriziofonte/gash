// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package htecmd

import (
	"fmt"
	"io"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mauriziofonte/gash/internal/hte"
	"github.com/mauriziofonte/gash/internal/hte/certs"
	"github.com/mauriziofonte/gash/internal/hte/discover"
	"github.com/mauriziofonte/gash/internal/hte/hosts"
	"github.com/mauriziofonte/gash/internal/hte/receipt"
	"github.com/mauriziofonte/gash/internal/hte/service"
	"github.com/mauriziofonte/gash/internal/hte/state"
)

// The text output: the data on stdout (key-value blocks, tables, lists),
// the messages on stderr (OK, warnings, hints), as the other 3.x commands.

func (r *run) out() io.Writer { return r.c.Stdout }

func (r *run) line(format string, a ...any) { fmt.Fprintf(r.out(), format+"\n", a...) }

// list prints a title and its items, nothing when there are none.
func (r *run) list(title string, items []string) {
	if len(items) == 0 {
		return
	}
	r.line("%s", title)
	r.style().List(r.out(), items...)
}

// table prints rows (tab-separated cells, the first one the header).
func (r *run) table(rows []string) { r.style().Table(r.out(), rows) }

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func ints(n []int) string {
	s := make([]string, len(n))
	for i, v := range n {
		s[i] = strconv.Itoa(v)
	}
	return orDash(strings.Join(s, ", "))
}

func changeLine(c receipt.Change) string {
	what := c.Path
	if what == "" {
		what = c.Name
	}
	if what == "" {
		return string(c.Kind)
	}
	return string(c.Kind) + " " + what
}

// applied reports what a change did to the running stack.
func (r *run) applied(a *hte.Applied) {
	if a == nil {
		return
	}
	for _, n := range a.Notes {
		r.say().Info(n)
	}
	switch {
	case a.Reloaded:
		r.say().Info(fmt.Sprintf("the stack reloaded (generation %d)", a.Generation))
	case a.Changed:
		r.say().Info("the stack is not running: the change applies at the next gash hte service start")
	}
	if h := a.Hosts; h != nil && !h.InSync {
		r.say().Warning("the hosts block does not match the sites: run gash hte hosts apply")
	}
}

func (r *run) printInstall(res *hte.InstallResult) {
	ports := fmt.Sprintf("%d and %d", res.Settings.Ports.HTTP, res.Settings.Ports.HTTPS)
	if res.Redirect == hte.RedirectPF {
		ports = "80 and 443, redirected by pf to " + ports
	}
	kv := []string{"Ports", ports, "Autostart", orDash(res.Settings.Autostart), "Started", yesNo(res.Started)}
	if res.Restored != "" {
		kv = append(kv, "Restored", res.Restored)
	}
	r.style().KV(r.out(), kv...)
	changes := make([]string, len(res.Changes))
	for i, c := range res.Changes {
		changes[i] = changeLine(c)
	}
	r.list("Recorded in the receipt:", changes)
	r.warn(res.Warnings)
	switch {
	case res.Started:
		r.say().Success("gash hte is installed and the stack runs")
	case res.Next != "":
		r.say().Success("gash hte is installed; the stack starts once the system vhosts are migrated")
		r.say().Hint("Next: " + res.Next)
	default:
		r.say().Success("gash hte is installed")
		r.say().Hint("Start the stack: gash hte service start")
	}
}

func (r *run) printUninstall(res *hte.UninstallResult) {
	r.style().KV(r.out(), "Stage", orDash(res.Stage), "Restored", plural(len(res.Restored), "change", "changes"))
	r.warn(res.Warnings)
	if res.Stage != "" {
		r.say().Success("gash hte is uninstalled; the sites are staged for a later install")
		r.say().Hint("gash hte install offers to restore " + res.Stage)
	}
}

func (r *run) printDoctor(res *hte.DoctorResult) {
	s := r.style()
	failed := 0
	for _, c := range res.Checks {
		mark, text := "info", c.Name+": "+c.Details
		switch c.Status {
		case "ok":
			mark = "ok"
		case "fixed":
			mark, text = "ok", text+" (fixed)"
		case "warn":
			mark = "warning"
		case "fail":
			mark = "error"
			failed++
		case "skip":
			text += " (skipped)"
		}
		s.Mark(r.out(), mark, text)
		if c.Remedy != "" {
			r.line("  remedy: %s", c.Remedy)
		}
	}
	if failed > 0 {
		r.say().Error(plural(failed, "check failed", "checks failed"))
		return
	}
	r.say().Success("every check passed")
}

func (r *run) printMigrate(res *hte.MigrateResult) {
	if !res.Applied {
		if res.Plan != nil {
			_, _ = io.WriteString(r.out(), res.Plan.Text())
		}
		if len(res.RootPlan) > 0 {
			r.line("")
			r.list("Every root step of --apply, in order (one sudo):", res.RootPlan)
		}
		r.warn(res.Warnings)
		r.say().Hint("Apply it: gash hte migrate --apply")
		return
	}
	kv := []string{"Imported", orDash(strings.Join(res.Imported, ", ")), "Switched", yesNo(res.Switched),
		"Archived", plural(len(res.Archived), "file", "files")}
	if res.Staging != "" {
		kv = append(kv, "Staging", res.Staging)
	}
	r.style().KV(r.out(), kv...)
	// The staging check and the check after the switch may report the
	// same answers.
	var verified []string
	for _, v := range res.Verified {
		if l := v.Domain + ": " + probes(v); !slices.Contains(verified, l) {
			verified = append(verified, l)
		}
	}
	r.list("Verified:", verified)
	r.list("Archived:", res.Archived)
	r.warn(res.Warnings)
	switch {
	case res.Switched:
		r.say().Success("the stack serves the migrated sites")
	case len(res.Imported) > 0:
		r.say().Success("the sites are imported; the system Apache keeps running")
	}
	if res.Plan != nil && len(res.Plan.Blocking) > 0 {
		r.say().Hint("These vhosts keep the system Apache running: migrate them or add --exclude PATH")
	}
}

// probes is "HTTP 200, HTTPS 200" for a site's answers.
func probes(s service.SiteStatus) string {
	var parts []string
	for _, p := range []struct {
		name string
		list []service.Probe
	}{{"HTTP", s.HTTP}, {"HTTPS", s.HTTPS}} {
		for _, pr := range p.list {
			v := strconv.Itoa(pr.Status)
			if pr.Error != "" {
				v = pr.Error
			}
			parts = append(parts, p.name+" "+v)
		}
	}
	return orDash(strings.Join(parts, ", "))
}

// printSite prints a site, then the pairs of more.
func (r *run) printSite(s state.Site, more ...string) {
	php := s.PHP
	if php == "" {
		php = "none"
	}
	kv := []string{
		"Domain", s.Domain,
		"Aliases", orDash(strings.Join(s.Aliases, " ")),
		"Document root", s.Docroot,
		"PHP", php,
		"Profile", orDash(s.Profile),
		"HTTPS", onOff(s.HTTPS),
		"Force HTTPS", onOff(s.ForceHTTPS),
		"Loopback bypass", onOff(s.ForceHTTPSBypassLoopback),
		"HTTP/2", onOff(s.HTTP2),
		"Extra HTTP ports", ints(s.ExtraHTTPPorts),
		"Extra HTTPS ports", ints(s.ExtraHTTPSPorts),
		"Enabled", yesNo(s.Enabled),
		"Kind", string(s.Kind),
	}
	if s.Kind == state.KindRaw {
		kv = append(kv, "Raw config", s.RawConf)
	}
	for _, k := range slices.Sorted(maps.Keys(s.PHPIni)) {
		kv = append(kv, "php.ini "+k, s.PHPIni[k])
	}
	r.style().KV(r.out(), append(kv, more...)...)
}

// printSiteResult reports a site command: the site, then what changed.
func (r *run) printSiteResult(domain, verb string) func(*hte.SiteResult) {
	return func(res *hte.SiteResult) {
		if res.Site != nil && verb != "removed" {
			r.printSite(*res.Site)
		}
		r.warn(res.Warnings)
		if !res.Changed {
			r.say().Info(domain + ": nothing to change")
			return
		}
		r.say().Success(domain + " " + verb)
		r.applied(res.Applied)
	}
}

func (r *run) printSiteShow(sh *hte.SiteShow) {
	var more []string
	if c := sh.Certificate; c != nil {
		more = []string{"Certificate", c.Path, "Valid until", stamp(c.NotAfter)}
	}
	r.printSite(sh.Site, more...)
	r.warn(sh.Warnings)
}

func (r *run) printSiteList(l *hte.SiteList) {
	if len(l.Sites) == 0 {
		r.warn(l.Problems)
		r.say().Info("no sites yet: gash hte site create --domain D --docroot PATH")
		return
	}
	rows := []string{"DOMAIN\tPHP\tHTTPS\tENABLED\tDOCUMENT ROOT"}
	for _, s := range l.Sites {
		https := onOff(s.HTTPS)
		if s.HTTPS && s.ForceHTTPS {
			https = "forced"
		}
		rows = append(rows, strings.Join([]string{s.Domain, orDash(s.PHP), https, yesNo(s.Enabled), s.Docroot}, "\t"))
	}
	r.table(rows)
	r.warn(l.Problems)
}

func (r *run) printPHPVersions(res *discover.PHPResult) {
	if len(res.Versions) == 0 {
		r.warn(res.Problems)
		r.say().Info("no PHP-FPM found")
		return
	}
	rows := []string{"VERSION\tRELEASE\tFPM\tPHP.INI"}
	for _, p := range res.Versions {
		rows = append(rows, strings.Join([]string{p.Version, p.Full, p.FPM, p.INI}, "\t"))
	}
	r.table(rows)
	r.warn(res.Problems)
}

func (r *run) printIniValue(v *iniValue) {
	if !v.Set {
		r.say().Info(v.Key + " is not set")
		return
	}
	r.line("%s", v.Value)
}

// printChange reports php ini set|unset and php global set|unset.
func (r *run) printChange(what string) func(any) {
	return func(v any) {
		switch res := v.(type) {
		case *hte.SiteResult:
			r.warn(res.Warnings)
			if !res.Changed {
				r.say().Info(what + ": nothing to change")
				return
			}
			r.say().Success(what)
			r.applied(res.Applied)
		case *hte.GlobalResult:
			r.style().KV(r.out(), "File", res.File)
			if !res.Changed {
				r.say().Info(what + ": nothing to change")
				return
			}
			r.say().Success(what)
			if res.Reloaded {
				r.say().Info("PHP-FPM reloaded")
			}
		}
	}
}

func (r *run) printModules(m *hte.ModulesResult) {
	rows := []string{"MODULE\tSTATE\tFILE"}
	for _, mod := range m.Modules {
		file := mod.Path
		switch {
		case mod.Builtin:
			file = "built in"
		case mod.Missing:
			file = "missing"
		}
		rows = append(rows, strings.Join([]string{mod.Name, mod.State, orDash(file)}, "\t"))
	}
	r.table(rows)
}

func (r *run) printModule(use string) func(*hte.ModuleResult) {
	return func(res *hte.ModuleResult) {
		r.warn(res.Warnings)
		if !res.Changed {
			r.say().Info(res.Module + ": nothing to change")
			return
		}
		r.say().Success("module " + res.Module + " " + use + "d")
		r.applied(res.Applied)
	}
}

func (r *run) printService(use string) func(*hte.ServiceResult) {
	done := map[string]string{"start": "the stack runs", "stop": "the stack is stopped",
		"restart": "the stack restarted", "reload": "the stack reloaded"}
	return func(res *hte.ServiceResult) {
		msg := done[use]
		if use == "reload" && res.Applied != nil {
			msg = fmt.Sprintf("%s (generation %d)", msg, res.Applied.Generation)
		}
		if res.Via == "systemd" {
			msg += ", through systemd"
		}
		if use != "stop" && !res.Running {
			r.say().Warning("the stack is not running")
			return
		}
		r.say().Success(msg)
		if res.Applied != nil {
			for _, n := range res.Applied.Notes {
				r.say().Info(n)
			}
		}
	}
}

func (r *run) printStatus(st *service.Status) {
	r.style().KV(r.out(), "Generation", strconv.Itoa(st.Generation), "Running", yesNo(st.Running))
	r.line("")
	rows := []string{"MASTER\tSTATE\tPID\tUPTIME\tADDRESSES"}
	for _, m := range append([]service.MasterStatus{st.Apache}, st.FPM...) {
		state, pid, up := "stopped", "-", "-"
		if m.Running {
			state, pid = "running", strconv.Itoa(m.PID)
			if m.Started != nil {
				up = (time.Duration(m.Uptime) * time.Second).String()
			}
		}
		var addrs []string
		for _, a := range m.Addrs {
			if a.Accepts {
				addrs = append(addrs, a.Addr)
			} else {
				addrs = append(addrs, a.Addr+" (refused)")
			}
		}
		rows = append(rows, strings.Join([]string{m.Name, state, pid, up, orDash(strings.Join(addrs, " "))}, "\t"))
	}
	r.table(rows)
	if len(st.Sites) > 0 {
		r.line("")
		rows = []string{"SITE\tANSWERS"}
		for _, s := range st.Sites {
			rows = append(rows, s.Domain+"\t"+probes(s))
		}
		r.table(rows)
	}
}

func (r *run) printLogs(l *logFiles) {
	for i, f := range l.Logs {
		if i > 0 {
			r.line("")
		}
		r.line("==> %s <==", f.Path)
		for _, ln := range f.Lines {
			r.line("%s", ln)
		}
	}
}

func stamp(t time.Time) string { return t.UTC().Format("2006-01-02 15:04 UTC") }

func (r *run) printCert(i *certs.Info) {
	r.style().KV(r.out(),
		"Domain", i.Domain,
		"File", i.Path,
		"Subject", i.Subject,
		"Names", strings.Join(i.Names, " "),
		"Valid from", stamp(i.NotBefore),
		"Valid until", stamp(i.NotAfter),
		"Algorithm", i.Algorithm,
		"Serial", i.Serial,
		"SHA-256", i.SHA256)
	if i.Legacy {
		r.say().Hint("A certificate copied from hte-cli: gash hte cert renew --domain " + i.Domain + " makes an ECDSA one")
	}
}

func (r *run) printDrift(use string) func(*hosts.Drift) {
	return func(d *hosts.Drift) {
		r.list("Missing from the block:", d.Missing)
		r.list("In the block, served by no site:", d.Unexpected)
		if d.Problem != "" {
			r.say().Warning(d.Problem)
		}
		switch {
		case d.InSync && use == "apply":
			r.say().Success("the hosts block is written")
		case d.InSync:
			r.say().Success("the hosts block matches the sites")
		default:
			r.say().Warning("the hosts block does not match the sites")
			if use == "check" {
				r.say().Hint("Write it: gash hte hosts apply")
			}
		}
	}
}

func (r *run) printHosts(h *hostsEntries) {
	if len(h.Entries) == 0 {
		r.say().Info("the block is empty: no site serves a name yet")
		return
	}
	for _, e := range h.Entries {
		r.line("%s %s", e.Address, strings.Join(e.Names, " "))
	}
}
