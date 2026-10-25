// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package wincmd

import (
	"strconv"
	"strings"

	"github.com/mauriziofonte/gash/internal/winbridge"
)

// ---- policy ----

type policyRoot struct {
	Key    string `json:"key"`
	Values int    `json:"values"`
}

// join is the device's identity posture from dsregcmd /status.
type join struct {
	AzureADJoined  string `json:"azure_ad_joined"`
	EnterpriseJoin string `json:"enterprise_joined"`
	DomainJoined   string `json:"domain_joined"`
	TenantName     string `json:"tenant_name"`
}

func (r *run) doPolicy() error {
	roots, e := itemsOf[policyRoot](r, winbridge.ScriptPolicy, nil)
	if e != nil {
		return r.winErr(e)
	}
	j := r.deviceJoin() // best-effort; nil when dsregcmd is absent
	if r.asJSON {
		out := map[string]any{"policy_roots": roots}
		if j != nil {
			out["join"] = j
		}
		return r.emitJSON(out)
	}
	s := r.style()
	if j != nil {
		s.KV(r.c.Stdout,
			"Entra (Azure AD) joined", orDash(j.AzureADJoined),
			"Enterprise joined", orDash(j.EnterpriseJoin),
			"Domain joined", orDash(j.DomainJoined),
			"Tenant", orDash(j.TenantName))
	}
	if len(roots) == 0 {
		r.say().Info("no policy values are set under the known policy roots")
		return nil
	}
	rows := []string{"policy key\tvalues\t"}
	for _, p := range roots {
		rows = append(rows, p.Key+"\t"+strconv.Itoa(p.Values))
	}
	s.Table(r.c.Stdout, rows, "--gap", "2")
	return nil
}

// deviceJoin runs dsregcmd /status and reads the join lines; nil when the
// program is missing or failed (not every Windows has it reachable).
func (r *run) deviceJoin() *join {
	out, e := r.bridge().Native(r.c.Ctx, "dsregcmd.exe", "/status")
	if e != nil {
		return nil
	}
	text := winbridge.Decode(out)
	get := func(key string) string {
		for _, line := range strings.Split(text, "\n") {
			if i := strings.Index(line, ":"); i >= 0 && strings.EqualFold(strings.TrimSpace(line[:i]), key) {
				return strings.TrimSpace(line[i+1:])
			}
		}
		return ""
	}
	j := &join{
		AzureADJoined:  get("AzureAdJoined"),
		EnterpriseJoin: get("EnterpriseJoined"),
		DomainJoined:   get("DomainJoined"),
		TenantName:     get("TenantName"),
	}
	return j
}

// ---- doctor ----

// doDoctor composes a health view from several read-only calls. A section
// whose call fails is marked, not fatal, so one unavailable area does not
// hide the rest. The command exits 1 when a check is a warning or worse.
func (r *run) doDoctor() error {
	br := r.bridge()
	if e := br.Available(); e != nil {
		return r.winErr(e)
	}
	type line struct {
		Status  string `json:"status"`
		Name    string `json:"name"`
		Details string `json:"details"`
	}
	var lines []line
	add := func(status, name, details string) { lines = append(lines, line{status, name, details}) }

	var info winInfo
	if e := br.DecodeJSON(r.c.Ctx, winbridge.ScriptInfo, nil, &info); e == nil {
		add("ok", "windows", info.Caption+" (build "+info.Build+")")
		add(boolMark(info.Elevated, "info", "info"), "token", elevWord(info.Elevated))
	} else {
		add("fail", "windows", e.Details)
	}

	var up winUpdates
	if e := br.DecodeJSON(r.c.Ctx, winbridge.ScriptUpdates, nil, &up); e == nil {
		if up.RebootPending {
			add("warn", "reboot", "a reboot is pending")
		} else {
			add("ok", "reboot", "none pending")
		}
	}

	var d winDefender
	if e := br.DecodeJSON(r.c.Ctx, winbridge.ScriptDefender, nil, &d); e == nil && d.Available {
		switch {
		case !d.Realtime:
			add("warn", "defender", "real-time protection is off")
		case d.SignatureAge > 7:
			add("warn", "defender", "virus signatures are "+strconv.Itoa(d.SignatureAge)+" days old")
		default:
			add("ok", "defender", "real-time on, signatures "+strconv.Itoa(d.SignatureAge)+"d old")
		}
	}

	if vols, e := itemsOf[winVolume](r, winbridge.ScriptDisk, nil); e == nil {
		for _, v := range vols {
			if v.Size == 0 {
				continue
			}
			pct := int(v.Free * 100 / v.Size)
			status := "ok"
			if pct < 10 {
				status = "warn"
			}
			add(status, "disk "+v.Drive, humanFree(v.Free)+" free ("+strconv.Itoa(pct)+"%)")
		}
	}

	if evs, e := itemsOf[winEvent](r, winbridge.ScriptEvents, map[string]string{"log": "System", "level": "2", "max": "5"}); e == nil {
		if len(evs) == 0 {
			add("ok", "errors", "no recent System errors")
		} else {
			add("warn", "errors", strconv.Itoa(len(evs))+" recent System error(s); see: gash win events --level error")
		}
	}

	failed := 0
	for _, l := range lines {
		if l.Status == "warn" || l.Status == "fail" {
			failed++
		}
	}
	if r.asJSON {
		r.emitJSON(map[string]any{"checks": lines, "attention": failed})
		if failed > 0 {
			r.status = 1
		}
		return nil
	}
	s := r.style()
	s.Section(r.c.Stdout, "Windows doctor")
	for _, l := range lines {
		s.Mark(r.c.Stdout, markWord(l.Status), l.Name+": "+l.Details)
	}
	if failed > 0 {
		r.say().Warning(strconv.Itoa(failed) + " item(s) want attention")
		r.status = 1
		return nil
	}
	r.say().Success("Windows looks healthy")
	return nil
}

func boolMark(b bool, yes, no string) string {
	if b {
		return yes
	}
	return no
}

func elevWord(elevated bool) string {
	if elevated {
		return "elevated (administrator)"
	}
	return "normal (some data needs an elevated terminal)"
}

func markWord(status string) string {
	switch status {
	case "ok":
		return "ok"
	case "warn":
		return "warning"
	case "fail":
		return "error"
	}
	return "info"
}

func humanFree(n int64) string {
	const g = 1 << 30
	if n >= g {
		return strconv.FormatInt(n/g, 10) + " GiB"
	}
	return strconv.FormatInt(n/(1<<20), 10) + " MiB"
}
